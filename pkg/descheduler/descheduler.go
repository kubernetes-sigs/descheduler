/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package descheduler

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"k8s.io/client-go/discovery"
	componentbaseconfig "k8s.io/component-base/config"
	"k8s.io/klog/v2"

	policy "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	listersv1 "k8s.io/client-go/listers/core/v1"
	schedulingv1 "k8s.io/client-go/listers/scheduling/v1"
	core "k8s.io/client-go/testing"

	"sigs.k8s.io/descheduler/cmd/descheduler/app/options"
	"sigs.k8s.io/descheduler/metrics"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/client"
	eutils "sigs.k8s.io/descheduler/pkg/descheduler/evictions/utils"
	nodeutil "sigs.k8s.io/descheduler/pkg/descheduler/node"
	"sigs.k8s.io/descheduler/pkg/framework/pluginregistry"
	"sigs.k8s.io/descheduler/pkg/utils"
	"sigs.k8s.io/descheduler/pkg/version"
)

func Run(ctx context.Context, rs *options.DeschedulerServer) error {
	metrics.Register()

	clientConnection := rs.ClientConnection
	if rs.KubeconfigFile != "" && clientConnection.Kubeconfig == "" {
		clientConnection.Kubeconfig = rs.KubeconfigFile
	}
	rsclient, eventClient, err := createClients(clientConnection)
	if err != nil {
		return err
	}
	rs.Client = rsclient
	rs.EventClient = eventClient

	deschedulerPolicy, err := LoadPolicyConfig(rs.PolicyConfigFile, rs.Client, pluginregistry.PluginRegistry)
	if err != nil {
		return err
	}
	if deschedulerPolicy == nil {
		return fmt.Errorf("deschedulerPolicy is nil")
	}

	// Add k8s compatibility warnings to logs
	if err := validateVersionCompatibility(rs.Client.Discovery(), version.Get()); err != nil {
		klog.Warning(err.Error())
	}

	evictionPolicyGroupVersion, err := eutils.SupportEviction(rs.Client)
	if err != nil || len(evictionPolicyGroupVersion) == 0 {
		return err
	}

	runFn := func() error {
		return RunDeschedulerStrategies(ctx, rs, deschedulerPolicy, evictionPolicyGroupVersion)
	}

	if rs.LeaderElection.LeaderElect && rs.DeschedulingInterval.Seconds() == 0 {
		return fmt.Errorf("leaderElection must be used with deschedulingInterval")
	}

	if rs.LeaderElection.LeaderElect && rs.DryRun {
		klog.V(1).Info("Warning: DryRun is set to True. You need to disable it to use Leader Election.")
	}

	if rs.LeaderElection.LeaderElect && !rs.DryRun {
		if err := NewLeaderElection(runFn, rsclient, &rs.LeaderElection, ctx); err != nil {
			return fmt.Errorf("leaderElection: %w", err)
		}
		return nil
	}

	return runFn()
}

func validateVersionCompatibility(discovery discovery.DiscoveryInterface, versionInfo version.Info) error {
	serverVersion, serverErr := discovery.ServerVersion()
	if serverErr != nil {
		return errors.New("failed to get Kubernetes server version")
	}

	deschedulerMinorVersion := strings.Split(versionInfo.Minor, ".")[0]
	deschedulerMinorVersionFloat, err := strconv.ParseFloat(deschedulerMinorVersion, 64)
	if err != nil {
		return errors.New("failed to convert Descheduler minor version to float")
	}

	kubernetesMinorVersionFloat, err := strconv.ParseFloat(serverVersion.Minor, 64)
	if err != nil {
		return errors.New("failed to convert Kubernetes server minor version to float")
	}

	if math.Abs(deschedulerMinorVersionFloat-kubernetesMinorVersionFloat) > 3 {
		return fmt.Errorf(
			"descheduler minor version %v is not supported on your version of Kubernetes %v.%v. "+
				"See compatibility docs for more info: https://github.com/kubernetes-sigs/descheduler#compatibility-matrix",
			deschedulerMinorVersion,
			serverVersion.Major,
			serverVersion.Minor,
		)
	}

	return nil
}

func cachedClient(
	realClient clientset.Interface,
	podLister listersv1.PodLister,
	nodeLister listersv1.NodeLister,
	namespaceLister listersv1.NamespaceLister,
	priorityClassLister schedulingv1.PriorityClassLister,
) (clientset.Interface, error) {
	fakeClient := fakeclientset.NewSimpleClientset()
	// simulate a pod eviction by deleting a pod
	fakeClient.PrependReactor("create", "pods", func(action core.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() == "eviction" {
			createAct, matched := action.(core.CreateActionImpl)
			if !matched {
				return false, nil, fmt.Errorf("unable to convert action to core.CreateActionImpl")
			}
			eviction, matched := createAct.Object.(*policy.Eviction)
			if !matched {
				return false, nil, fmt.Errorf("unable to convert action object into *policy.Eviction")
			}
			if err := fakeClient.Tracker().Delete(action.GetResource(), eviction.GetNamespace(), eviction.GetName()); err != nil {
				return false, nil, fmt.Errorf("unable to delete pod %v/%v: %v", eviction.GetNamespace(), eviction.GetName(), err)
			}
			return true, nil, nil
		}
		// fallback to the default reactor
		return false, nil, nil
	})

	klog.V(3).Infof("Pulling resources for the cached client from the cluster")
	pods, err := podLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("unable to list pods: %v", err)
	}

	for _, item := range pods {
		if _, err := fakeClient.CoreV1().Pods(item.Namespace).Create(context.TODO(), item, metav1.CreateOptions{}); err != nil {
			return nil, fmt.Errorf("unable to copy pod: %v", err)
		}
	}

	nodes, err := nodeLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("unable to list nodes: %v", err)
	}

	for _, item := range nodes {
		if _, err := fakeClient.CoreV1().Nodes().Create(context.TODO(), item, metav1.CreateOptions{}); err != nil {
			return nil, fmt.Errorf("unable to copy node: %v", err)
		}
	}

	namespaces, err := namespaceLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("unable to list namespaces: %v", err)
	}

	for _, item := range namespaces {
		if _, err := fakeClient.CoreV1().Namespaces().Create(context.TODO(), item, metav1.CreateOptions{}); err != nil {
			return nil, fmt.Errorf("unable to copy namespace: %v", err)
		}
	}

	priorityClasses, err := priorityClassLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("unable to list priorityclasses: %v", err)
	}

	for _, item := range priorityClasses {
		if _, err := fakeClient.SchedulingV1().PriorityClasses().Create(context.TODO(), item, metav1.CreateOptions{}); err != nil {
			return nil, fmt.Errorf("unable to copy priorityclass: %v", err)
		}
	}

	return fakeClient, nil
}

func RunDeschedulerStrategies(ctx context.Context, rs *options.DeschedulerServer, deschedulerPolicy *api.DeschedulerPolicy, evictionPolicyGroupVersion string) error {
	var eventClient clientset.Interface
	if rs.DryRun {
		eventClient = fakeclientset.NewSimpleClientset()
	} else {
		eventClient = rs.Client
	}
	eventBroadcaster, eventRecorder := utils.GetRecorderAndBroadcaster(ctx, eventClient)
	defer eventBroadcaster.Shutdown()

	descheduler, err := NewDescheduler(ctx, rs, deschedulerPolicy, evictionPolicyGroupVersion, eventRecorder)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	wait.NonSlidingUntil(func() {
		nodes, err := nodeutil.ReadyNodes(ctx, rs.Client, descheduler.NodeLister, descheduler.NodeSelector)
		if err != nil {
			klog.Error(err)
			cancel()
			return
		}
		err = descheduler.RunDeschedulerLoop(ctx, nodes)
		if err != nil {
			klog.Error(err)
			cancel()
			return
		}
		// If there was no interval specified, send a signal to the stopChannel to end the wait.Until loop after 1 iteration
		if rs.DeschedulingInterval.Seconds() == 0 {
			cancel()
		}
	}, rs.DeschedulingInterval, ctx.Done())

	return nil
}

func GetPluginConfig(pluginName string, pluginConfigs []api.PluginConfig) (*api.PluginConfig, int) {
	for idx, pluginConfig := range pluginConfigs {
		if pluginConfig.Name == pluginName {
			return &pluginConfig, idx
		}
	}
	return nil, 0
}

func createClients(clientConnection componentbaseconfig.ClientConnectionConfiguration) (clientset.Interface, clientset.Interface, error) {
	kClient, err := client.CreateClient(clientConnection, "descheduler")
	if err != nil {
		return nil, nil, err
	}

	eventClient, err := client.CreateClient(clientConnection, "")
	if err != nil {
		return nil, nil, err
	}

	return kClient, eventClient, nil
}
