package framework

import (
	"context"
	"fmt"
	"sort"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/pkg/api/v1alpha2"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/framework"
	"sigs.k8s.io/descheduler/pkg/framework/registry"
)

// Option for the handleImpl.
type Option func(*handleImplOpts)

type handleImplOpts struct {
	clientSet             clientset.Interface
	sharedInformerFactory informers.SharedInformerFactory
	podEvictor            *framework.PodEvictor
}

// WithClientSet sets clientSet for the scheduling frameworkImpl.
func WithClientSet(clientSet clientset.Interface) Option {
	return func(o *handleImplOpts) {
		o.clientSet = clientSet
	}
}

func WithSharedInformerFactory(sharedInformerFactory informers.SharedInformerFactory) Option {
	return func(o *handleImplOpts) {
		o.sharedInformerFactory = sharedInformerFactory
	}
}

func WithPodEvictor(podEvictor *framework.PodEvictor) Option {
	return func(o *handleImplOpts) {
		o.podEvictor = podEvictor
	}
}

type handleImpl struct {
	clientSet                 clientset.Interface
	getPodsAssignedToNodeFunc podutil.GetPodsAssignedToNodeFunc
	sharedInformerFactory     informers.SharedInformerFactory

	evictorImpl *evictorImpl
}

type profileImpl struct {
	handleImpl        *handleImpl
	deschedulePlugins []framework.DeschedulePlugin
	balancePlugins    []framework.BalancePlugin
}

type evictorImpl struct {
	podEvictor  *framework.PodEvictor
	evictPlugin framework.EvictPlugin
	sortPlugin  framework.SortPlugin
}

// Sort pods from the most to the least suitable for eviction
func (ei *evictorImpl) Sort(pods []*v1.Pod) {
	sort.Slice(pods, func(i int, j int) bool {
		return ei.sortPlugin.Less(pods[i], pods[j])
	})
}

// Filter checks if a pod can be evicted
func (ei *evictorImpl) Filter(pod *v1.Pod) bool {
	return ei.evictPlugin.Filter(pod)
}

// Evict evicts a pod (no pre-check performed)
func (ei *evictorImpl) Evict(ctx context.Context, pod *v1.Pod) bool {
	return ei.podEvictor.Evict(ctx, pod)
}

func (d *handleImpl) ClientSet() clientset.Interface {
	return d.clientSet
}

func (d *handleImpl) Evictor() framework.Evictor {
	return d.evictorImpl
}

func (d *handleImpl) GetPodsAssignedToNodeFunc() podutil.GetPodsAssignedToNodeFunc {
	return d.getPodsAssignedToNodeFunc
}

func (d *handleImpl) SharedInformerFactory() informers.SharedInformerFactory {
	return d.sharedInformerFactory
}

func (d profileImpl) runDeschedulePlugins(ctx context.Context, nodes []*v1.Node) *framework.Status {
	errs := []error{}
	for _, pl := range d.deschedulePlugins {
		status := pl.Deschedule(context.WithValue(ctx, "pluginName", pl.Name()), nodes)
		if status != nil && status.Err != nil {
			errs = append(errs, status.Err)
		}
		klog.V(1).InfoS("Total number of pods evicted", "evictedPods", d.handleImpl.evictorImpl.podEvictor.TotalEvicted())
	}

	aggrErr := errors.NewAggregate(errs)
	if aggrErr == nil {
		return &framework.Status{}
	}

	return &framework.Status{
		Err: fmt.Errorf("%v", aggrErr.Error()),
	}
}

func (d profileImpl) runBalancePlugins(ctx context.Context, nodes []*v1.Node) *framework.Status {
	errs := []error{}
	for _, pl := range d.balancePlugins {
		status := pl.Balance(context.WithValue(ctx, "pluginName", pl.Name()), nodes)
		if status != nil && status.Err != nil {
			errs = append(errs, status.Err)
		}
		klog.V(1).InfoS("Total number of pods evicted", "evictedPods", d.handleImpl.evictorImpl.podEvictor.TotalEvicted())
	}

	aggrErr := errors.NewAggregate(errs)
	if aggrErr == nil {
		return &framework.Status{}
	}

	return &framework.Status{
		Err: fmt.Errorf("%v", aggrErr.Error()),
	}
}

func newProfile(config v1alpha2.Profile, reg registry.Registry, opts ...Option) (*profileImpl, error) {
	hOpts := &handleImplOpts{}
	for _, optFnc := range opts {
		optFnc(hOpts)
	}

	pluginArgs := map[string]runtime.Object{}
	for _, plConfig := range config.PluginConfig {
		pluginArgs[plConfig.Name] = plConfig.Args
	}

	// Assumption: Enabled and Disabled sets are mutually exclusive

	enabled := sets.NewString()
	// disabled := sets.NewString()

	// for _, plName := range config.Plugins.Deschedule.Disabled {
	// 	disabled.Insert(plName)
	// }

	for _, plName := range config.Plugins.PreSort.Enabled {
		enabled.Insert(plName)
	}

	for _, plName := range config.Plugins.Deschedule.Enabled {
		enabled.Insert(plName)
	}

	for _, plName := range config.Plugins.Balance.Enabled {
		enabled.Insert(plName)
	}

	for _, plName := range config.Plugins.Sort.Enabled {
		enabled.Insert(plName)
	}

	for _, plName := range config.Plugins.Evict.Enabled {
		enabled.Insert(plName)
	}

	podInformer := hOpts.sharedInformerFactory.Core().V1().Pods()
	getPodsAssignedToNode, err := podutil.BuildGetPodsAssignedToNodeFunc(podInformer)
	if err != nil {
		return nil, fmt.Errorf("unable to create BuildGetPodsAssignedToNodeFunc: %v", err)
	}

	handle := &handleImpl{
		clientSet:                 hOpts.clientSet,
		getPodsAssignedToNodeFunc: getPodsAssignedToNode,
		sharedInformerFactory:     hOpts.sharedInformerFactory,
		evictorImpl: &evictorImpl{
			podEvictor: hOpts.podEvictor,
		},
	}

	pluginsMap := map[string]framework.Plugin{}
	for plName := range enabled {
		fmt.Printf("plName: %v\n", plName)
		var pl framework.Plugin
		var err error
		if args, ok := pluginArgs[plName]; ok {
			pl, err = reg[plName](args, handle)
		} else {
			pl, err = reg[plName](nil, handle)
		}
		if err != nil {
			return nil, fmt.Errorf("unable to initialize %q plugin: %v", plName, err)
		}
		pluginsMap[plName] = pl
	}

	deschedulePlugins := []framework.DeschedulePlugin{}
	balancePlugins := []framework.BalancePlugin{}

	for _, plName := range config.Plugins.Deschedule.Enabled {
		deschedulePlugins = append(deschedulePlugins, pluginsMap[plName].(framework.DeschedulePlugin))
	}

	for _, plName := range config.Plugins.Balance.Enabled {
		balancePlugins = append(balancePlugins, pluginsMap[plName].(framework.BalancePlugin))
	}

	if len(config.Plugins.Sort.Enabled) != 1 {
		return nil, fmt.Errorf("expected only a single sort plugin, have %v", len(config.Plugins.Sort.Enabled))
	}

	if len(config.Plugins.Evict.Enabled) != 1 {
		return nil, fmt.Errorf("expected only a single evict plugin, have %v", len(config.Plugins.Evict.Enabled))
	}

	evictPluginName := config.Plugins.Evict.Enabled[0]
	handle.evictorImpl.evictPlugin = pluginsMap[evictPluginName].(framework.EvictPlugin)

	sortPluginName := config.Plugins.Evict.Enabled[0]
	handle.evictorImpl.sortPlugin = pluginsMap[sortPluginName].(framework.SortPlugin)

	return &profileImpl{
		handleImpl:        handle,
		deschedulePlugins: deschedulePlugins,
		balancePlugins:    balancePlugins,
	}, nil
}
