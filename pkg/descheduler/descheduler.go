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
	policyv1 "k8s.io/api/policy/v1"
	policy "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	corev1informers "k8s.io/client-go/informers/core/v1"
	schedulingv1informers "k8s.io/client-go/informers/scheduling/v1"
	clientset "k8s.io/client-go/kubernetes"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/cmd/descheduler/app/options"
	"sigs.k8s.io/descheduler/metrics"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/api/v1alpha2"
	"sigs.k8s.io/descheduler/pkg/descheduler/client"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	eutils "sigs.k8s.io/descheduler/pkg/descheduler/evictions/utils"
	nodeutil "sigs.k8s.io/descheduler/pkg/descheduler/node"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/framework"
	"sigs.k8s.io/descheduler/pkg/framework/registry"
	frameworkruntime "sigs.k8s.io/descheduler/pkg/framework/runtime"
)

func Run(ctx context.Context, rs *options.DeschedulerServer) error {
	metrics.Register()

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

	runFn := func() error {
		return RunDeschedulerStrategies(ctx, rs, deschedulerPolicy, evictionPolicyGroupVersion)
	}

	if rs.LeaderElection.LeaderElect && rs.DeschedulingInterval.Seconds() == 0 {
		return fmt.Errorf("leaderElection must be used with deschedulingInterval")
	}

	if rs.LeaderElection.LeaderElect && !rs.DryRun {
		if err := NewLeaderElection(runFn, rsclient, &rs.LeaderElection, ctx); err != nil {
			return fmt.Errorf("leaderElection: %w", err)
		}
		return nil
	}

	return runFn()
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

type Descheduler struct {
	podEvictor   *framework.PodEvictor
	framework    *frameworkruntime.Framework
	nodeSelector string
	clientSet    clientset.Interface
	nodeInformer corev1informers.NodeInformer

	nodepodCount      map[string]uint
	namespacePodCount map[string]uint
	evicted           uint

	maxPodsToEvictPerNode      *uint
	maxPodsToEvictPerNamespace *uint
}

func (d *Descheduler) Evict(ctx context.Context, pod *v1.Pod) bool {
	if d.maxPodsToEvictPerNode != nil && d.nodepodCount[pod.Spec.NodeName]+1 > *d.maxPodsToEvictPerNode {
		klog.ErrorS(fmt.Errorf("Maximum number of evicted pods per node reached"), "limit", *d.maxPodsToEvictPerNode, "node", pod.Spec.NodeName)
		return false
	}
	if d.maxPodsToEvictPerNamespace != nil && d.namespacePodCount[pod.Namespace]+1 > *d.maxPodsToEvictPerNamespace {
		klog.ErrorS(fmt.Errorf("Maximum number of evicted pods per namespace reached"), "limit", *d.maxPodsToEvictPerNamespace, "namespace", pod.Namespace)
	}
	if d.podEvictor.Evict(ctx, pod) {
		d.nodepodCount[pod.Spec.NodeName]++
		d.namespacePodCount[pod.Namespace]++
		d.evicted++
	}
	return false
}

func (d *Descheduler) deschedulerOnce(ctx context.Context) error {
	d.nodepodCount = make(map[string]uint)
	d.namespacePodCount = make(map[string]uint)

	nodes, err := nodeutil.ReadyNodes(ctx, d.clientSet, d.nodeInformer, d.nodeSelector)
	if err != nil {
		return fmt.Errorf("unable to get ready nodes: %v", err)
	}

	if len(nodes) <= 1 {
		return fmt.Errorf("the cluster size is 0 or 1 meaning eviction causes service disruption or degradation")
	}

	if status := d.framework.RunDeschedulePlugins(ctx, nodes); status != nil && status.Err != nil {
		return status.Err
	}

	if status := d.framework.RunBalancePlugins(ctx, nodes); status != nil && status.Err != nil {
		return status.Err
	}

	return nil
}

func resetFramework(
	desch *Descheduler,
	config v1alpha2.DeschedulerConfiguration,
	pluginReg registry.Registry,
	realClient clientset.Interface,
	podInformer corev1informers.PodInformer,
	nodeInformer corev1informers.NodeInformer,
	namespaceInformer corev1informers.NamespaceInformer,
	priorityClassInformer schedulingv1informers.PriorityClassInformer,
) (
	context.CancelFunc,
	error,
) {
	// When the dry mode is enable, collect all the relevant objects (mostly pods) under a fake client.
	// So when evicting pods while running multiple strategies in a row have the cummulative effect
	// as is when evicting pods for real.
	klog.V(3).Infof("Building a cached client from the cluster for the dry run")
	// Create a new cache so we start from scratch without any leftovers
	fakeClient, err := cachedClient(realClient, podInformer, nodeInformer, namespaceInformer, priorityClassInformer)
	if err != nil {
		return nil, err
	}

	fakeSharedInformerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
	fakeCtx, cncl := context.WithCancel(context.TODO())
	fakeSharedInformerFactory.Start(fakeCtx.Done())
	fakeSharedInformerFactory.WaitForCacheSync(fakeCtx.Done())

	frmwrk, err := frameworkruntime.NewFramework(config,
		frameworkruntime.WithClientSet(fakeClient),
		frameworkruntime.WithSharedInformerFactory(fakeSharedInformerFactory),
		frameworkruntime.WithPodEvictor(desch),
		frameworkruntime.WithRegistry(pluginReg),
	)
	if err != nil {
		cncl()
		return cncl, err
	}

	desch.framework = frmwrk
	return cncl, nil
}

func RunDeschedulerStrategies(ctx context.Context, rs *options.DeschedulerServer, deschedulerPolicy *api.DeschedulerPolicy, evictionPolicyGroupVersion string) error {
	sharedInformerFactory := informers.NewSharedInformerFactory(rs.Client, 0)
	nodeInformer := sharedInformerFactory.Core().V1().Nodes()
	podInformer := sharedInformerFactory.Core().V1().Pods()
	namespaceInformer := sharedInformerFactory.Core().V1().Namespaces()
	priorityClassInformer := sharedInformerFactory.Scheduling().V1().PriorityClasses()

	// create the informers before starting the informer factory
	namespaceInformer.Informer()
	priorityClassInformer.Informer()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sharedInformerFactory.Start(ctx.Done())
	sharedInformerFactory.WaitForCacheSync(ctx.Done())

	var nodeSelector string
	if deschedulerPolicy.NodeSelector != nil {
		nodeSelector = *deschedulerPolicy.NodeSelector
	}

	klog.V(3).Infof("Building a pod evictor")
	podEvictor := framework.NewPodEvictor(
		rs.Client,
		policyv1.SchemeGroupVersion.String(),
		rs.DryRun,
		nil,
		nil,
		!rs.DisableMetrics,
	)

	desch := &Descheduler{
		podEvictor:                 podEvictor,
		clientSet:                  rs.Client,
		nodeInformer:               nodeInformer,
		nodeSelector:               nodeSelector,
		nodepodCount:               make(map[string]uint),
		namespacePodCount:          make(map[string]uint),
		maxPodsToEvictPerNode:      deschedulerPolicy.MaxNoOfPodsToEvictPerNode,
		maxPodsToEvictPerNamespace: deschedulerPolicy.MaxNoOfPodsToEvictPerNamespace,
	}

	pluginReg := registry.NewRegistry()

	config := v1alpha2.DeschedulerConfiguration{}

	if !rs.DryRun {
		frmwrk, err := frameworkruntime.NewFramework(config,
			frameworkruntime.WithClientSet(rs.Client),
			frameworkruntime.WithSharedInformerFactory(sharedInformerFactory),
			frameworkruntime.WithPodEvictor(desch),
			frameworkruntime.WithRegistry(pluginReg),
		)
		if err != nil {
			return fmt.Errorf("Unable to initialize framework: %v", err)
		}
		desch.framework = frmwrk
	}

	wait.NonSlidingUntil(func() {
		if rs.DryRun {
			cncl, err := resetFramework(desch, config, pluginReg, rs.Client, podInformer, nodeInformer, namespaceInformer, priorityClassInformer)
			if err != nil {
				klog.Error(err)
				cancel()
				return
			}
			defer cncl()
		}
		if err := desch.deschedulerOnce(ctx); err != nil {
			klog.Errorf("Error descheduling pods: %v", err)
		}

		klog.V(1).InfoS("Number of evicted pods", "totalEvicted", desch.evicted)

		// If there was no interval specified, send a signal to the stopChannel to end the wait.Until loop after 1 iteration
		if rs.DeschedulingInterval.Seconds() == 0 {
			cancel()
			return
		}
	}, rs.DeschedulingInterval, ctx.Done())

	return nil
}
