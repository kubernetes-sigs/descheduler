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
	"time"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/tools/events"
	componentbaseconfig "k8s.io/component-base/config"
	"k8s.io/klog/v2"

	v1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	listersv1 "k8s.io/client-go/listers/core/v1"
	schedulingv1 "k8s.io/client-go/listers/scheduling/v1"
	core "k8s.io/client-go/testing"

	"sigs.k8s.io/descheduler/cmd/descheduler/app/options"
	"sigs.k8s.io/descheduler/metrics"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/client"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	eutils "sigs.k8s.io/descheduler/pkg/descheduler/evictions/utils"
	nodeutil "sigs.k8s.io/descheduler/pkg/descheduler/node"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/framework/pluginregistry"
	frameworkprofile "sigs.k8s.io/descheduler/pkg/framework/profile"
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

func RunDeschedulerLoop(ctx context.Context, rs *options.DeschedulerServer, nodes []*v1.Node, nodeSelector string,
	podLister listersv1.PodLister, nodeLister listersv1.NodeLister, namespaceLister listersv1.NamespaceLister,
	priorityClassLister schedulingv1.PriorityClassLister, getPodsAssignedToNode podutil.GetPodsAssignedToNodeFunc,
	cycleSharedInformerFactory informers.SharedInformerFactory, evictionPolicyGroupVersion string, deschedulerPolicy *api.DeschedulerPolicy, eventRecorder events.EventRecorder,
) error {
	loopStartDuration := time.Now()
	defer metrics.DeschedulerLoopDuration.With(map[string]string{}).Observe(time.Since(loopStartDuration).Seconds())

	// if len is still <= 1 error out
	if len(nodes) <= 1 {
		klog.V(1).InfoS("The cluster size is 0 or 1 meaning eviction causes service disruption or degradation. So aborting..")
		return fmt.Errorf("the cluster size is 0 or 1")
	}

	var client clientset.Interface
	// When the dry mode is enable, collect all the relevant objects (mostly pods) under a fake client.
	// So when evicting pods while running multiple strategies in a row have the cummulative effect
	// as is when evicting pods for real.
	if rs.DryRun {
		klog.V(3).Infof("Building a cached client from the cluster for the dry run")
		// Create a new cache so we start from scratch without any leftovers
		fakeClient, err := cachedClient(rs.Client, podLister, nodeLister, namespaceLister, priorityClassLister)
		if err != nil {
			return err
		}

		// create a new instance of the shared informer factor from the cached client
		fakeSharedInformerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
		// register the pod informer, otherwise it will not get running
		getPodsAssignedToNode, err = podutil.BuildGetPodsAssignedToNodeFunc(fakeSharedInformerFactory.Core().V1().Pods().Informer())
		if err != nil {
			return fmt.Errorf("build get pods assigned to node function error: %v", err)
		}

		fakeCtx, cncl := context.WithCancel(context.TODO())
		defer cncl()
		fakeSharedInformerFactory.Start(fakeCtx.Done())
		fakeSharedInformerFactory.WaitForCacheSync(fakeCtx.Done())

		client = fakeClient
		cycleSharedInformerFactory = fakeSharedInformerFactory
	} else {
		client = rs.Client
	}

	klog.V(3).Infof("Building a pod evictor")
	podEvictor := evictions.NewPodEvictor(
		client,
		evictionPolicyGroupVersion,
		rs.DryRun,
		deschedulerPolicy.MaxNoOfPodsToEvictPerNode,
		deschedulerPolicy.MaxNoOfPodsToEvictPerNamespace,
		nodes,
		!rs.DisableMetrics,
		eventRecorder,
	)

	RunProfiles(ctx, nodes, deschedulerPolicy, client, cycleSharedInformerFactory, podEvictor, getPodsAssignedToNode)

	klog.V(1).InfoS("Number of evicted pods", "totalEvicted", podEvictor.TotalEvicted())

	return nil
}

// RunProfiles runs all the deschedule plugins of all profiles and
// later runs through all balance plugins of all profiles. (All Balance plugins should come after all Deschedule plugins)
// see https://github.com/kubernetes-sigs/descheduler/issues/979
func RunProfiles(ctx context.Context, nodes []*v1.Node, deschedulerPolicy *api.DeschedulerPolicy, client clientset.Interface,
	cycleSharedInformerFactory informers.SharedInformerFactory, podEvictor *evictions.PodEvictor, getPodsAssignedToNode podutil.GetPodsAssignedToNodeFunc,
) {
	frameworkprofiles := []*frameworkprofile.ProfileImpl{}
	for _, profile := range deschedulerPolicy.Profiles {
		currProfile, err := frameworkprofile.NewProfile(
			profile,
			pluginregistry.PluginRegistry,
			frameworkprofile.WithClientSet(client),
			frameworkprofile.WithSharedInformerFactory(cycleSharedInformerFactory),
			frameworkprofile.WithPodEvictor(podEvictor),
			frameworkprofile.WithGetPodsAssignedToNodeFnc(getPodsAssignedToNode),
		)
		if err != nil {
			klog.ErrorS(err, "unable to create a profile", "profile", profile.Name)
			continue
		}
		frameworkprofiles = append(frameworkprofiles, currProfile)
	}

	for _, profileImpl := range frameworkprofiles {
		// First deschedule
		status := profileImpl.RunDeschedulePlugins(ctx, nodes)
		if status != nil && status.Err != nil {
			klog.ErrorS(status.Err, "running deschedule extension point failed with error", "profile", profileImpl.ProfileName)
			continue
		}
	}

	for _, profileImpl := range frameworkprofiles {
		// Then balance
		status := profileImpl.RunBalancePlugins(ctx, nodes)
		if status != nil && status.Err != nil {
			klog.ErrorS(status.Err, "running balance extension point failed with error", "profile", profileImpl.ProfileName)
			continue
		}
	}
}

func RunDeschedulerStrategies(ctx context.Context, rs *options.DeschedulerServer, deschedulerPolicy *api.DeschedulerPolicy, evictionPolicyGroupVersion string) error {
	sharedInformerFactory := informers.NewSharedInformerFactory(rs.Client, 0)
	podInformer := sharedInformerFactory.Core().V1().Pods().Informer()
	podLister := sharedInformerFactory.Core().V1().Pods().Lister()
	nodeLister := sharedInformerFactory.Core().V1().Nodes().Lister()
	namespaceLister := sharedInformerFactory.Core().V1().Namespaces().Lister()
	priorityClassLister := sharedInformerFactory.Scheduling().V1().PriorityClasses().Lister()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	getPodsAssignedToNode, err := podutil.BuildGetPodsAssignedToNodeFunc(podInformer)
	if err != nil {
		return fmt.Errorf("build get pods assigned to node function error: %v", err)
	}

	sharedInformerFactory.Start(ctx.Done())
	sharedInformerFactory.WaitForCacheSync(ctx.Done())

	var nodeSelector string
	if deschedulerPolicy.NodeSelector != nil {
		nodeSelector = *deschedulerPolicy.NodeSelector
	}

	var eventClient clientset.Interface
	if rs.DryRun {
		eventClient = fakeclientset.NewSimpleClientset()
	} else {
		eventClient = rs.Client
	}

	eventBroadcaster, eventRecorder := utils.GetRecorderAndBroadcaster(ctx, eventClient)
	defer eventBroadcaster.Shutdown()

	cycleSharedInformerFactory := sharedInformerFactory

	wait.NonSlidingUntil(func() {
		nodes, err := nodeutil.ReadyNodes(ctx, rs.Client, nodeLister, nodeSelector)
		if err != nil {
			klog.Error(err)
			cancel()
			return
		}
		err = RunDeschedulerLoop(ctx, rs, nodes, nodeSelector, podLister, nodeLister, namespaceLister, priorityClassLister, getPodsAssignedToNode, cycleSharedInformerFactory, evictionPolicyGroupVersion, deschedulerPolicy, eventRecorder)
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
