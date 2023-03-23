/*
Copyright 2023 The Kubernetes Authors.
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

package profile

import (
	"context"
	"fmt"
	"time"

	"sigs.k8s.io/descheduler/metrics"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/framework"
	"sigs.k8s.io/descheduler/pkg/framework/pluginregistry"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"

	"k8s.io/klog/v2"
)

// evictorImpl implements the Evictor interface so plugins
// can evict a pod without importing a specific pod evictor
type evictorImpl struct {
	podEvictor    *evictions.PodEvictor
	evictorFilter framework.EvictorPlugin
}

var _ framework.Evictor = &evictorImpl{}

// Filter checks if a pod can be evicted
func (ei *evictorImpl) Filter(pod *v1.Pod) bool {
	return ei.evictorFilter.Filter(pod)
}

// PreEvictionFilter checks if pod can be evicted right before eviction
func (ei *evictorImpl) PreEvictionFilter(pod *v1.Pod) bool {
	return ei.evictorFilter.PreEvictionFilter(pod)
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

type profileImpl struct {
	profileName string
	podEvictor  *evictions.PodEvictor

	deschedulePlugins []framework.DeschedulePlugin
	balancePlugins    []framework.BalancePlugin
}

// Option for the handleImpl.
type Option func(*handleImplOpts)

type handleImplOpts struct {
	clientSet                 clientset.Interface
	sharedInformerFactory     informers.SharedInformerFactory
	getPodsAssignedToNodeFunc podutil.GetPodsAssignedToNodeFunc
	podEvictor                *evictions.PodEvictor
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

func WithPodEvictor(podEvictor *evictions.PodEvictor) Option {
	return func(o *handleImplOpts) {
		o.podEvictor = podEvictor
	}
}

func WithGetPodsAssignedToNodeFnc(getPodsAssignedToNodeFunc podutil.GetPodsAssignedToNodeFunc) Option {
	return func(o *handleImplOpts) {
		o.getPodsAssignedToNodeFunc = getPodsAssignedToNodeFunc
	}
}

func getPluginConfig(pluginName string, pluginConfigs []api.PluginConfig) (*api.PluginConfig, int) {
	for idx, pluginConfig := range pluginConfigs {
		if pluginConfig.Name == pluginName {
			return &pluginConfig, idx
		}
	}
	return nil, 0
}

func buildPlugin(config api.DeschedulerProfile, pluginName string, handle *handleImpl, reg pluginregistry.Registry) (framework.Plugin, error) {
	pc, _ := getPluginConfig(pluginName, config.PluginConfigs)
	if pc == nil {
		klog.ErrorS(fmt.Errorf("unable to get plugin config"), "skipping plugin", "plugin", pluginName, "profile", config.Name)
		return nil, fmt.Errorf("unable to find %q plugin config", pluginName)
	}

	registryPlugin, ok := reg[pluginName]
	if !ok {
		klog.ErrorS(fmt.Errorf("unable to find plugin in the pluginsMap"), "skipping plugin", "plugin", pluginName)
		return nil, fmt.Errorf("unable to find %q plugin in the pluginsMap", pluginName)
	}
	pg, err := registryPlugin.PluginBuilder(pc.Args, handle)
	if err != nil {
		klog.ErrorS(err, "unable to initialize a plugin", "pluginName", pluginName)
		return nil, fmt.Errorf("unable to initialize %q plugin: %v", pluginName, err)
	}
	return pg, nil
}

func NewProfile(config api.DeschedulerProfile, reg pluginregistry.Registry, opts ...Option) (*profileImpl, error) {
	hOpts := &handleImplOpts{}
	for _, optFnc := range opts {
		optFnc(hOpts)
	}

	if hOpts.clientSet == nil {
		return nil, fmt.Errorf("clientSet missing")
	}

	if hOpts.sharedInformerFactory == nil {
		return nil, fmt.Errorf("sharedInformerFactory missing")
	}

	if hOpts.podEvictor == nil {
		return nil, fmt.Errorf("podEvictor missing")
	}

	evictorPlugin, err := buildPlugin(config, defaultevictor.PluginName, &handleImpl{
		clientSet:                 hOpts.clientSet,
		getPodsAssignedToNodeFunc: hOpts.getPodsAssignedToNodeFunc,
		sharedInformerFactory:     hOpts.sharedInformerFactory,
	}, reg)
	if err != nil {
		return nil, fmt.Errorf("unable to build %v plugin: %v", defaultevictor.PluginName, err)
	}
	if evictorPlugin == nil {
		return nil, fmt.Errorf("empty plugin build for %v plugin: %v", defaultevictor.PluginName, err)
	}

	handle := &handleImpl{
		clientSet:                 hOpts.clientSet,
		getPodsAssignedToNodeFunc: hOpts.getPodsAssignedToNodeFunc,
		sharedInformerFactory:     hOpts.sharedInformerFactory,
		evictor: &evictorImpl{
			podEvictor:    hOpts.podEvictor,
			evictorFilter: evictorPlugin.(framework.EvictorPlugin),
		},
	}

	deschedulePlugins := []framework.DeschedulePlugin{}
	balancePlugins := []framework.BalancePlugin{}

	descheduleEnabled := make(map[string]struct{})
	balanceEnabled := make(map[string]struct{})
	for _, name := range config.Plugins.Deschedule.Enabled {
		descheduleEnabled[name] = struct{}{}
	}
	for _, name := range config.Plugins.Balance.Enabled {
		balanceEnabled[name] = struct{}{}
	}

	// Assuming only a list of enabled extension points.
	// Later, when a default list of plugins and their extension points is established,
	// compute the list of enabled extension points as (DefaultEnabled + Enabled - Disabled)
	for _, plugin := range append(config.Plugins.Deschedule.Enabled, config.Plugins.Balance.Enabled...) {
		pg, err := buildPlugin(config, plugin, handle, reg)
		if err != nil {
			return nil, fmt.Errorf("unable to build %v plugin: %v", plugin, err)
		}
		if pg != nil {
			// pg can be of any of each type, or both

			if _, exists := descheduleEnabled[plugin]; exists {
				_, ok := pg.(framework.DeschedulePlugin)
				if ok {
					deschedulePlugins = append(deschedulePlugins, pg.(framework.DeschedulePlugin))
				}
			}

			if _, exists := balanceEnabled[plugin]; exists {
				_, ok := pg.(framework.BalancePlugin)
				if ok {
					balancePlugins = append(balancePlugins, pg.(framework.BalancePlugin))
				}
			}
		}
	}

	return &profileImpl{
		profileName:       config.Name,
		podEvictor:        hOpts.podEvictor,
		deschedulePlugins: deschedulePlugins,
		balancePlugins:    balancePlugins,
	}, nil
}

func (d profileImpl) RunDeschedulePlugins(ctx context.Context, nodes []*v1.Node) *framework.Status {
	errs := []error{}
	for _, pl := range d.deschedulePlugins {
		evicted := d.podEvictor.TotalEvicted()
		// TODO: strategyName should be accessible from within the strategy using a framework
		// handle or function which the Evictor has access to. For migration/in-progress framework
		// work, we are currently passing this via context. To be removed
		// (See discussion thread https://github.com/kubernetes-sigs/descheduler/pull/885#discussion_r919962292)
		strategyStart := time.Now()
		childCtx := context.WithValue(ctx, "strategyName", pl.Name())
		status := pl.Deschedule(childCtx, nodes)
		metrics.DeschedulerStrategyDuration.With(map[string]string{"strategy": pl.Name(), "profile": d.profileName}).Observe(time.Since(strategyStart).Seconds())

		if status != nil && status.Err != nil {
			errs = append(errs, fmt.Errorf("plugin %q finished with error: %v", pl.Name(), status.Err))
		}
		klog.V(1).InfoS("Total number of pods evicted", "extension point", "Deschedule", "evictedPods", d.podEvictor.TotalEvicted()-evicted)
	}

	aggrErr := errors.NewAggregate(errs)
	if aggrErr == nil {
		return &framework.Status{}
	}

	return &framework.Status{
		Err: fmt.Errorf("%v", aggrErr.Error()),
	}
}

func (d profileImpl) RunBalancePlugins(ctx context.Context, nodes []*v1.Node) *framework.Status {
	errs := []error{}
	for _, pl := range d.balancePlugins {
		evicted := d.podEvictor.TotalEvicted()
		// TODO: strategyName should be accessible from within the strategy using a framework
		// handle or function which the Evictor has access to. For migration/in-progress framework
		// work, we are currently passing this via context. To be removed
		// (See discussion thread https://github.com/kubernetes-sigs/descheduler/pull/885#discussion_r919962292)
		strategyStart := time.Now()
		childCtx := context.WithValue(ctx, "strategyName", pl.Name())
		status := pl.Balance(childCtx, nodes)
		metrics.DeschedulerStrategyDuration.With(map[string]string{"strategy": pl.Name(), "profile": d.profileName}).Observe(time.Since(strategyStart).Seconds())

		if status != nil && status.Err != nil {
			errs = append(errs, fmt.Errorf("plugin %q finished with error: %v", pl.Name(), status.Err))
		}
		klog.V(1).InfoS("Total number of pods evicted", "extension point", "Balance", "evictedPods", d.podEvictor.TotalEvicted()-evicted)
	}

	aggrErr := errors.NewAggregate(errs)
	if aggrErr == nil {
		return &framework.Status{}
	}

	return &framework.Status{
		Err: fmt.Errorf("%v", aggrErr.Error()),
	}
}
