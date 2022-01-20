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
	"fmt"

	v1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/klog/v2"

	corev1informers "k8s.io/client-go/informers/core/v1"
	schedulingv1informers "k8s.io/client-go/informers/scheduling/v1"

	"sigs.k8s.io/descheduler/cmd/descheduler/app/options"
	"sigs.k8s.io/descheduler/metrics"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/client"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	eutils "sigs.k8s.io/descheduler/pkg/descheduler/evictions/utils"
	nodeutil "sigs.k8s.io/descheduler/pkg/descheduler/node"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/descheduler/strategies"
	"sigs.k8s.io/descheduler/pkg/descheduler/strategies/nodeutilization"
)

func Run(rs *options.DeschedulerServer) error {
	metrics.Register()

	ctx := context.Background()
	rsclient, err := client.CreateClient(rs.KubeconfigFile)
	if err != nil {
		return err
	}
	rs.Client = rsclient

	deschedulerPolicy, err := LoadPolicyConfig(rs.PolicyConfigFile)
	if err != nil {
		return err
	}
	if deschedulerPolicy == nil {
		return fmt.Errorf("deschedulerPolicy is nil")
	}

	evictionPolicyGroupVersion, err := eutils.SupportEviction(rs.Client)
	if err != nil || len(evictionPolicyGroupVersion) == 0 {
		return err
	}

	stopChannel := make(chan struct{})
	return RunDeschedulerStrategies(ctx, rs, deschedulerPolicy, evictionPolicyGroupVersion, stopChannel)
}

type strategyFunction func(ctx context.Context, client clientset.Interface, strategy api.DeschedulerStrategy, nodes []*v1.Node, podEvictor *evictions.PodEvictor, getPodsAssignedToNode podutil.GetPodsAssignedToNodeFunc)

func cachedClient(
	realClient clientset.Interface,
	podInformer corev1informers.PodInformer,
	nodeInformer corev1informers.NodeInformer,
	namespaceInformer corev1informers.NamespaceInformer,
	priorityClassInformer schedulingv1informers.PriorityClassInformer,
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
	pods, err := podInformer.Lister().List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("unable to list pods: %v", err)
	}

	for _, item := range pods {
		if _, err := fakeClient.CoreV1().Pods(item.Namespace).Create(context.TODO(), item, metav1.CreateOptions{}); err != nil {
			return nil, fmt.Errorf("unable to copy pod: %v", err)
		}
	}

	nodes, err := nodeInformer.Lister().List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("unable to list nodes: %v", err)
	}

	for _, item := range nodes {
		if _, err := fakeClient.CoreV1().Nodes().Create(context.TODO(), item, metav1.CreateOptions{}); err != nil {
			return nil, fmt.Errorf("unable to copy node: %v", err)
		}
	}

	namespaces, err := namespaceInformer.Lister().List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("unable to list namespaces: %v", err)
	}

	for _, item := range namespaces {
		if _, err := fakeClient.CoreV1().Namespaces().Create(context.TODO(), item, metav1.CreateOptions{}); err != nil {
			return nil, fmt.Errorf("unable to copy node: %v", err)
		}
	}

	priorityClasses, err := priorityClassInformer.Lister().List(labels.Everything())
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

func RunDeschedulerStrategies(ctx context.Context, rs *options.DeschedulerServer, deschedulerPolicy *api.DeschedulerPolicy, evictionPolicyGroupVersion string, stopChannel chan struct{}) error {
	sharedInformerFactory := informers.NewSharedInformerFactory(rs.Client, 0)
	nodeInformer := sharedInformerFactory.Core().V1().Nodes()
	podInformer := sharedInformerFactory.Core().V1().Pods()
	namespaceInformer := sharedInformerFactory.Core().V1().Namespaces()
	priorityClassInformer := sharedInformerFactory.Scheduling().V1().PriorityClasses()

	// create the informers
	namespaceInformer.Informer()
	priorityClassInformer.Informer()

	getPodsAssignedToNode, err := podutil.BuildGetPodsAssignedToNodeFunc(podInformer)
	if err != nil {
		return fmt.Errorf("build get pods assigned to node function error: %v", err)
	}

	sharedInformerFactory.Start(stopChannel)
	sharedInformerFactory.WaitForCacheSync(stopChannel)

	strategyFuncs := map[api.StrategyName]strategyFunction{
		"RemoveDuplicates":                            strategies.RemoveDuplicatePods,
		"LowNodeUtilization":                          nodeutilization.LowNodeUtilization,
		"HighNodeUtilization":                         nodeutilization.HighNodeUtilization,
		"RemovePodsViolatingInterPodAntiAffinity":     strategies.RemovePodsViolatingInterPodAntiAffinity,
		"RemovePodsViolatingNodeAffinity":             strategies.RemovePodsViolatingNodeAffinity,
		"RemovePodsViolatingNodeTaints":               strategies.RemovePodsViolatingNodeTaints,
		"RemovePodsHavingTooManyRestarts":             strategies.RemovePodsHavingTooManyRestarts,
		"PodLifeTime":                                 strategies.PodLifeTime,
		"RemovePodsViolatingTopologySpreadConstraint": strategies.RemovePodsViolatingTopologySpreadConstraint,
		"RemoveFailedPods":                            strategies.RemoveFailedPods,
	}

	var nodeSelector string
	if deschedulerPolicy.NodeSelector != nil {
		nodeSelector = *deschedulerPolicy.NodeSelector
	}

	var evictLocalStoragePods bool
	if deschedulerPolicy.EvictLocalStoragePods != nil {
		evictLocalStoragePods = *deschedulerPolicy.EvictLocalStoragePods
	}

	evictBarePods := false
	if deschedulerPolicy.EvictFailedBarePods != nil {
		evictBarePods = *deschedulerPolicy.EvictFailedBarePods
		if evictBarePods {
			klog.V(1).InfoS("Warning: EvictFailedBarePods is set to True. This could cause eviction of pods without ownerReferences.")
		}
	}

	evictSystemCriticalPods := false
	if deschedulerPolicy.EvictSystemCriticalPods != nil {
		evictSystemCriticalPods = *deschedulerPolicy.EvictSystemCriticalPods
		if evictSystemCriticalPods {
			klog.V(1).InfoS("Warning: EvictSystemCriticalPods is set to True. This could cause eviction of Kubernetes system pods.")
		}
	}

	ignorePvcPods := false
	if deschedulerPolicy.IgnorePVCPods != nil {
		ignorePvcPods = *deschedulerPolicy.IgnorePVCPods
	}

	wait.NonSlidingUntil(func() {
		nodes, err := nodeutil.ReadyNodes(ctx, rs.Client, nodeInformer, nodeSelector)
		if err != nil {
			klog.V(1).InfoS("Unable to get ready nodes", "err", err)
			close(stopChannel)
			return
		}

		if len(nodes) <= 1 {
			klog.V(1).InfoS("The cluster size is 0 or 1 meaning eviction causes service disruption or degradation. So aborting..")
			close(stopChannel)
			return
		}

		var podEvictorClient clientset.Interface
		// When the dry mode is enable, collect all the relevant objects (mostly pods) under a fake client.
		// So when evicting pods while running multiple strategies in a row have the cummulative effect
		// as is when evicting pods for real.
		if rs.DryRun {
			klog.V(3).Infof("Building a cached client from the cluster for the dry run")
			// Create a new cache so we start from scratch without any leftovers
			fakeClient, err := cachedClient(rs.Client, podInformer, nodeInformer, namespaceInformer, priorityClassInformer)
			if err != nil {
				klog.Error(err)
				return
			}

			fakeSharedInformerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
			getPodsAssignedToNode, err = podutil.BuildGetPodsAssignedToNodeFunc(fakeSharedInformerFactory.Core().V1().Pods())
			if err != nil {
				klog.Errorf("build get pods assigned to node function error: %v", err)
				return
			}

			fakeCtx, cncl := context.WithCancel(context.TODO())
			defer cncl()
			fakeSharedInformerFactory.Start(fakeCtx.Done())
			fakeSharedInformerFactory.WaitForCacheSync(fakeCtx.Done())

			podEvictorClient = fakeClient
		} else {
			podEvictorClient = rs.Client
		}

		klog.V(3).Infof("Building a pod evictor")
		podEvictor := evictions.NewPodEvictor(
			podEvictorClient,
			evictionPolicyGroupVersion,
			rs.DryRun,
			deschedulerPolicy.MaxNoOfPodsToEvictPerNode,
			deschedulerPolicy.MaxNoOfPodsToEvictPerNamespace,
			nodes,
			evictLocalStoragePods,
			evictSystemCriticalPods,
			ignorePvcPods,
			evictBarePods,
		)

		for name, strategy := range deschedulerPolicy.Strategies {
			if f, ok := strategyFuncs[name]; ok {
				if strategy.Enabled {
					f(ctx, rs.Client, strategy, nodes, podEvictor, getPodsAssignedToNode)
				}
			} else {
				klog.ErrorS(fmt.Errorf("unknown strategy name"), "skipping strategy", "strategy", name)
			}
		}

		klog.V(1).InfoS("Number of evicted pods", "totalEvicted", podEvictor.TotalEvicted())

		// If there was no interval specified, send a signal to the stopChannel to end the wait.Until loop after 1 iteration
		if rs.DeschedulingInterval.Seconds() == 0 {
			close(stopChannel)
		}
	}, rs.DeschedulingInterval, stopChannel)

	return nil
}
