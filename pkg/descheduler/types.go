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
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	listersv1 "k8s.io/client-go/listers/core/v1"
	schedulingv1 "k8s.io/client-go/listers/scheduling/v1"
	"k8s.io/client-go/tools/events"
	"k8s.io/klog/v2"
	"sigs.k8s.io/descheduler/cmd/descheduler/app/options"
	"sigs.k8s.io/descheduler/metrics"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/framework/pluginregistry"
	frameworkprofile "sigs.k8s.io/descheduler/pkg/framework/profile"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
	"sigs.k8s.io/descheduler/pkg/utils"
)

type eprunner func(ctx context.Context, nodes []*v1.Node) *frameworktypes.Status

type profileRunner struct {
	name                      string
	descheduleEPs, balanceEPs eprunner
}

type Descheduler struct {
	rs                         *options.DeschedulerServer
	NodeSelector               string
	PodLister                  listersv1.PodLister
	NodeLister                 listersv1.NodeLister
	NamespaceLister            listersv1.NamespaceLister
	PriorityClassLister        schedulingv1.PriorityClassLister
	GetPodsAssignedToNode      podutil.GetPodsAssignedToNodeFunc
	CycleSharedInformerFactory informers.SharedInformerFactory
	EvictionPolicyGroupVersion string
	DeschedulerPolicy          *api.DeschedulerPolicy
	EventRecorder              events.EventRecorder
}

func NewDescheduler(ctx context.Context, rs *options.DeschedulerServer, deschedulerPolicy *api.DeschedulerPolicy, evictionPolicyGroupVersion string) (*Descheduler, error) {
	sharedInformerFactory := informers.NewSharedInformerFactory(rs.Client, 0)
	podInformer := sharedInformerFactory.Core().V1().Pods().Informer()
	podLister := sharedInformerFactory.Core().V1().Pods().Lister()
	nodeLister := sharedInformerFactory.Core().V1().Nodes().Lister()
	namespaceLister := sharedInformerFactory.Core().V1().Namespaces().Lister()
	priorityClassLister := sharedInformerFactory.Scheduling().V1().PriorityClasses().Lister()

	getPodsAssignedToNode, err := podutil.BuildGetPodsAssignedToNodeFunc(podInformer)
	if err != nil {
		return nil, fmt.Errorf("build get pods assigned to node function error: %v", err)
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

	return &Descheduler{
		rs:                         rs,
		NodeSelector:               nodeSelector,
		PodLister:                  podLister,
		NodeLister:                 nodeLister,
		NamespaceLister:            namespaceLister,
		PriorityClassLister:        priorityClassLister,
		GetPodsAssignedToNode:      getPodsAssignedToNode,
		CycleSharedInformerFactory: cycleSharedInformerFactory,
		EvictionPolicyGroupVersion: evictionPolicyGroupVersion,
		DeschedulerPolicy:          deschedulerPolicy,
		EventRecorder:              eventRecorder,
	}, nil
}

func (d *Descheduler) RunDeschedulerLoop(ctx context.Context, nodes []*v1.Node) error {
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
	if d.rs.DryRun {
		klog.V(3).Infof("Building a cached client from the cluster for the dry run")
		// Create a new cache so we start from scratch without any leftovers
		fakeClient, err := cachedClient(d.rs.Client, d.PodLister, d.NodeLister, d.NamespaceLister, d.PriorityClassLister)
		if err != nil {
			return err
		}

		// create a new instance of the shared informer factor from the cached client
		fakeSharedInformerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
		// register the pod informer, otherwise it will not get running
		d.GetPodsAssignedToNode, err = podutil.BuildGetPodsAssignedToNodeFunc(fakeSharedInformerFactory.Core().V1().Pods().Informer())
		if err != nil {
			return fmt.Errorf("build get pods assigned to node function error: %v", err)
		}

		fakeCtx, cncl := context.WithCancel(context.TODO())
		defer cncl()
		fakeSharedInformerFactory.Start(fakeCtx.Done())
		fakeSharedInformerFactory.WaitForCacheSync(fakeCtx.Done())

		client = fakeClient
		d.CycleSharedInformerFactory = fakeSharedInformerFactory
	} else {
		client = d.rs.Client
	}

	klog.V(3).Infof("Building a pod evictor")
	podEvictor := evictions.NewPodEvictor(
		client,
		d.EvictionPolicyGroupVersion,
		d.rs.DryRun,
		d.DeschedulerPolicy.MaxNoOfPodsToEvictPerNode,
		d.DeschedulerPolicy.MaxNoOfPodsToEvictPerNamespace,
		nodes,
		!d.rs.DisableMetrics,
		d.EventRecorder,
	)

	d.RunProfiles(ctx, client, nodes, podEvictor)

	klog.V(1).InfoS("Number of evicted pods", "totalEvicted", podEvictor.TotalEvicted())

	return nil
}

// RunProfiles runs all the deschedule plugins of all profiles and
// later runs through all balance plugins of all profiles. (All Balance plugins should come after all Deschedule plugins)
// see https://github.com/kubernetes-sigs/descheduler/issues/979
func (d *Descheduler) RunProfiles(ctx context.Context, client clientset.Interface, nodes []*v1.Node, podEvictor *evictions.PodEvictor) {
	var profileRunners []profileRunner
	for _, profile := range d.DeschedulerPolicy.Profiles {
		currProfile, err := frameworkprofile.NewProfile(
			profile,
			pluginregistry.PluginRegistry,
			frameworkprofile.WithClientSet(client),
			frameworkprofile.WithSharedInformerFactory(d.CycleSharedInformerFactory),
			frameworkprofile.WithPodEvictor(podEvictor),
			frameworkprofile.WithGetPodsAssignedToNodeFnc(d.GetPodsAssignedToNode),
		)
		if err != nil {
			klog.ErrorS(err, "unable to create a profile", "profile", profile.Name)
			continue
		}
		profileRunners = append(profileRunners, profileRunner{profile.Name, currProfile.RunDeschedulePlugins, currProfile.RunBalancePlugins})
	}

	for _, profileR := range profileRunners {
		// First deschedule
		status := profileR.descheduleEPs(ctx, nodes)
		if status != nil && status.Err != nil {
			klog.ErrorS(status.Err, "running deschedule extension point failed with error", "profile", profileR.name)
			continue
		}
	}

	for _, profileR := range profileRunners {
		// Balance Later
		status := profileR.balanceEPs(ctx, nodes)
		if status != nil && status.Err != nil {
			klog.ErrorS(status.Err, "running balance extension point failed with error", "profile", profileR.name)
			continue
		}
	}
}
