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
	"sigs.k8s.io/descheduler/pkg/framework"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	"sigs.k8s.io/descheduler/pkg/utils"
)

func Run(ctx context.Context, rs *options.DeschedulerServer) error {
	metrics.Register()

	rsclient, eventClient, err := createClients(rs.KubeconfigFile)
	if err != nil {
		return err
	}
	rs.Client = rsclient
	rs.EventClient = eventClient

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

type strategyFunction func(ctx context.Context, client clientset.Interface, strategy api.DeschedulerPolicy, nodes []*v1.Node, podEvictor *evictions.PodEvictor, evictorFilter framework.EvictorPlugin, getPodsAssignedToNode podutil.GetPodsAssignedToNodeFunc)

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
			return nil, fmt.Errorf("unable to copy node: %v", err)
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

// evictorImpl implements the Evictor interface so plugins
// can evict a pod without importing a specific pod evictor
type evictorImpl struct {
	podEvictor              *evictions.PodEvictor
	evictorFiltersOrSorters []framework.EvictorPlugin
}

var _ framework.Evictor = &evictorImpl{}

// Filter checks if a pod can be evicted
func (ei *evictorImpl) Filter(pod *v1.Pod) bool {
	var filterFuncs []podutil.FilterFunc
	for _, filterPlugin := range ei.evictorFiltersOrSorters {
		filterFuncs = append(filterFuncs, filterPlugin.Filter)
	}
	combinedFilter := podutil.WrapFilterFuncs(filterFuncs...)
	return combinedFilter(pod)
}

// PreEvictionFilter checks if pod can be evicted right before eviction
func (ei *evictorImpl) PreEvictionFilter(pod *v1.Pod) bool {
	var preEvictionFilterFuncs []podutil.FilterFunc
	for _, preEvictionFilterPlugin := range ei.evictorFiltersOrSorters {
		preEvictionFilterFuncs = append(preEvictionFilterFuncs, preEvictionFilterPlugin.PreEvictionFilter)
	}
	combinedFilter := podutil.WrapFilterFuncs(preEvictionFilterFuncs...)
	return combinedFilter(pod)
}

// Evict evicts a pod (no pre-check performed)
func (ei *evictorImpl) Evict(ctx context.Context, pod *v1.Pod, opts evictions.EvictOptions) bool {
	return ei.podEvictor.EvictPod(ctx, pod, opts)
}

func (ei *evictorImpl) NodeLimitExceeded(node *v1.Node) bool {
	return ei.podEvictor.NodeLimitExceeded(node)
}

// handleImpl implements the framework handle which gets passed to plugins
type handleImpl struct {
	clientSet                 clientset.Interface
	getPodsAssignedToNodeFunc podutil.GetPodsAssignedToNodeFunc
	sharedInformerFactory     informers.SharedInformerFactory
	evictor                   *evictorImpl
}

var _ framework.Handle = &handleImpl{}

// ClientSet retrieves kube client set
func (hi *handleImpl) ClientSet() clientset.Interface {
	return hi.clientSet
}

// GetPodsAssignedToNodeFunc retrieves GetPodsAssignedToNodeFunc implementation
func (hi *handleImpl) GetPodsAssignedToNodeFunc() podutil.GetPodsAssignedToNodeFunc {
	return hi.getPodsAssignedToNodeFunc
}

// SharedInformerFactory retrieves shared informer factory
func (hi *handleImpl) SharedInformerFactory() informers.SharedInformerFactory {
	return hi.sharedInformerFactory
}

// Evictor retrieves evictor so plugins can filter and evict pods
func (hi *handleImpl) Evictor() framework.Evictor {
	return hi.evictor
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

	wait.NonSlidingUntil(func() {
		nodes, err := nodeutil.ReadyNodes(ctx, rs.Client, nodeLister, nodeSelector)
		if err != nil {
			klog.V(1).InfoS("Unable to get ready nodes", "err", err)
			cancel()
			return
		}

		if len(nodes) <= 1 {
			klog.V(1).InfoS("The cluster size is 0 or 1 meaning eviction causes service disruption or degradation. So aborting..")
			cancel()
			return
		}

		var podEvictorClient clientset.Interface
		// When the dry mode is enable, collect all the relevant objects (mostly pods) under a fake client.
		// So when evicting pods while running multiple strategies in a row have the cummulative effect
		// as is when evicting pods for real.
		if rs.DryRun {
			klog.V(3).Infof("Building a cached client from the cluster for the dry run")
			// Create a new cache so we start from scratch without any leftovers
			fakeClient, err := cachedClient(rs.Client, podLister, nodeLister, namespaceLister, priorityClassLister)
			if err != nil {
				klog.Error(err)
				return
			}

			fakeSharedInformerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
			getPodsAssignedToNode, err = podutil.BuildGetPodsAssignedToNodeFunc(fakeSharedInformerFactory.Core().V1().Pods().Informer())
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
			!rs.DisableMetrics,
			eventRecorder,
		)

		for _, profile := range deschedulerPolicy.Profiles {
			_, filterConfigs, _ := getSortersFiltersAndEvictorConfigFromProfile(profile)
			var filterArgs []runtime.Object
			for _, filterConfig := range filterConfigs {
				filterArgs = append(filterArgs, filterConfig.Args.(*defaultevictor.DefaultEvictorArgs))
			}
			evictorFilters, err := buildEvictorFilterPlugins(filterArgs, rs.Client, getPodsAssignedToNode, sharedInformerFactory)
			if err != nil {
				klog.Errorf("could not build evictor filters", err)
			}
			handle := &handleImpl{
				clientSet:                 rs.Client,
				getPodsAssignedToNodeFunc: getPodsAssignedToNode,
				sharedInformerFactory:     sharedInformerFactory,
				evictor: &evictorImpl{
					podEvictor:              podEvictor,
					evictorFiltersOrSorters: evictorFilters,
				},
			}

			// for now running sequentially
			// TODO(knelasevero): implement profiles in parallel
			enabledDeschedulePlugins := profile.Plugins.Deschedule.Enabled
			enabledBalancePlugins := profile.Plugins.Balance.Enabled

			for _, pluginName := range enabledDeschedulePlugins {
				err := runPlugin(ctx, evictorFilters, handle, profile, pluginName, filterArgs, rs.Client, getPodsAssignedToNode, sharedInformerFactory, podEvictor, nodes)
				if err != nil {
					klog.Errorf("could not run plugin", err)
				}
			}

			for _, pluginName := range enabledBalancePlugins {
				err := runPlugin(ctx, evictorFilters, handle, profile, pluginName, filterArgs, rs.Client, getPodsAssignedToNode, sharedInformerFactory, podEvictor, nodes)
				if err != nil {
					klog.Errorf("could not run plugin", err)
				}
			}

			klog.V(1).InfoS("Number of evicted pods for profile", "profile", profile.Name, "totalEvicted", podEvictor.TotalEvicted())

		}

		// If there was no interval specified, send a signal to the stopChannel to end the wait.Until loop after 1 iteration
		if rs.DeschedulingInterval.Seconds() == 0 {
			cancel()
		}
	}, rs.DeschedulingInterval, ctx.Done())

	return nil
}

func runPlugin(ctx context.Context, evictorFilters []framework.EvictorPlugin, handle *handleImpl, profile api.Profile, pluginName string, filterArgs []runtime.Object, clientSet clientset.Interface, getPodsAssignedToNode podutil.GetPodsAssignedToNodeFunc, sharedInformerFactory informers.SharedInformerFactory, podEvictor *evictions.PodEvictor, nodes []*v1.Node) error {
	pluginConfig := getPluginConfig(pluginName, profile.PluginConfig)
	if pluginConfig == nil {
		return fmt.Errorf("could not get config for plugin: %s", pluginName)
	}
	PluginArgs := pluginConfig.Args
	// TODO: strategyName should be accessible from within the strategy using a framework
	// handle or function which the Evictor has access to. For migration/in-progress framework
	// work, we are currently passing this via context. To be removed
	// (See discussion thread https://github.com/kubernetes-sigs/descheduler/pull/885#discussion_r919962292)
	childCtx := context.WithValue(ctx, "strategyName", string(pluginName))
	if pgFnc, exists := pluginsMap[string(pluginName)]; exists {
		pgFnc(childCtx, nodes, PluginArgs, handle)
	}
	return nil
}

func buildEvictorFilterPlugins(filterArgs []runtime.Object, clientSet clientset.Interface, getPodsAssignedToNode podutil.GetPodsAssignedToNodeFunc, sharedInformerFactory informers.SharedInformerFactory) ([]framework.EvictorPlugin, error) {
	var evictorFilters []framework.EvictorPlugin
	for _, args := range filterArgs {
		newEvictorFilter, err := defaultevictor.New(
			args,
			&handleImpl{
				clientSet:                 clientSet,
				getPodsAssignedToNodeFunc: getPodsAssignedToNode,
				sharedInformerFactory:     sharedInformerFactory,
			},
		)
		if err != nil {
			return nil, err
		}
		evictorFilters = append(evictorFilters, newEvictorFilter.(framework.EvictorPlugin))
	}
	return evictorFilters, nil
}

func getPluginConfig(pluginName string, pluginConfigs []api.PluginConfig) *api.PluginConfig {
	for _, pluginConfig := range pluginConfigs {
		if pluginConfig.Name == pluginName {
			return &pluginConfig
		}
	}
	return nil
}

func getSortersFiltersAndEvictorConfigFromProfile(profile api.Profile) (api.PluginConfig, []api.PluginConfig, []api.PluginConfig) {
	var evictorConfig api.PluginConfig
	var filterConfigs []api.PluginConfig
	var preEvictionFilterConfigs []api.PluginConfig

	evictorPluginName := profile.Plugins.Evict.Enabled[0]
	filterPluginNames := profile.Plugins.Filter.Enabled
	preEvictionFilterPluginNames := profile.Plugins.PreEvictionFilter.Enabled
	for _, pluginConfig := range profile.PluginConfig {
		if pluginConfig.Name == evictorPluginName {
			evictorConfig = pluginConfig
		}
		if contains(filterPluginNames, pluginConfig.Name) {
			filterConfigs = append(filterConfigs, pluginConfig)
		}
		if contains(preEvictionFilterPluginNames, pluginConfig.Name) {
			preEvictionFilterConfigs = append(preEvictionFilterConfigs, pluginConfig)
		}
	}
	return evictorConfig, filterConfigs, preEvictionFilterConfigs
}

func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}

func createClients(kubeconfig string) (clientset.Interface, clientset.Interface, error) {
	kClient, err := client.CreateClient(kubeconfig, "descheduler")
	if err != nil {
		return nil, nil, err
	}

	eventClient, err := client.CreateClient(kubeconfig, "")
	if err != nil {
		return nil, nil, err
	}

	return kClient, eventClient, nil
}
