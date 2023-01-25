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

package v1alpha1

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/framework"
	"sigs.k8s.io/descheduler/pkg/framework/pluginregistry"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	"sigs.k8s.io/descheduler/pkg/utils"
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

func V1alpha1ToInternal(
	client clientset.Interface,
	deschedulerPolicy *DeschedulerPolicy,
	registry pluginregistry.Registry,
) (*api.DeschedulerPolicy, error) {
	var evictLocalStoragePods bool
	if deschedulerPolicy.EvictLocalStoragePods != nil {
		evictLocalStoragePods = *deschedulerPolicy.EvictLocalStoragePods
	}

	evictBarePods := false
	if deschedulerPolicy.EvictFailedBarePods != nil {
		evictBarePods = *deschedulerPolicy.EvictFailedBarePods
		if evictBarePods {
			klog.V(1).Info("Warning: EvictFailedBarePods is set to True. This could cause eviction of pods without ownerReferences.")
		}
	}

	evictSystemCriticalPods := false
	if deschedulerPolicy.EvictSystemCriticalPods != nil {
		evictSystemCriticalPods = *deschedulerPolicy.EvictSystemCriticalPods
		if evictSystemCriticalPods {
			klog.V(1).Info("Warning: EvictSystemCriticalPods is set to True. This could cause eviction of Kubernetes system pods.")
		}
	}

	ignorePvcPods := false
	if deschedulerPolicy.IgnorePVCPods != nil {
		ignorePvcPods = *deschedulerPolicy.IgnorePVCPods
	}

	var profiles []api.Profile

	// Build profiles
	for name, strategy := range deschedulerPolicy.Strategies {
		if _, ok := pluginregistry.PluginRegistry[string(name)]; ok {
			if strategy.Enabled {
				params := strategy.Params
				if params == nil {
					params = &StrategyParameters{}
				}

				nodeFit := false
				if name != "PodLifeTime" {
					nodeFit = params.NodeFit
				}

				if params.ThresholdPriority != nil && params.ThresholdPriorityClassName != "" {
					klog.ErrorS(fmt.Errorf("priority threshold misconfigured"), "only one of priorityThreshold fields can be set", "pluginName", name)
					return nil, fmt.Errorf("priority threshold misconfigured for plugin %v", name)
				}

				var priorityThreshold *api.PriorityThreshold
				if strategy.Params != nil {
					priorityThreshold = &api.PriorityThreshold{
						Value: strategy.Params.ThresholdPriority,
						Name:  strategy.Params.ThresholdPriorityClassName,
					}
				}
				thresholdPriority, err := utils.GetPriorityFromStrategyParams(context.TODO(), client, priorityThreshold)
				if err != nil {
					klog.ErrorS(err, "Failed to get threshold priority from strategy's params")
					return nil, fmt.Errorf("failed to get threshold priority from strategy's params: %v", err)
				}

				var pluginConfig *api.PluginConfig
				if pcFnc, exists := StrategyParamsToPluginArgs[string(name)]; exists {
					pluginConfig, err = pcFnc(params)
					if err != nil {
						klog.ErrorS(err, "skipping strategy", "strategy", name)
						return nil, fmt.Errorf("failed to get plugin config for strategy %v: %v", name, err)
					}
				} else {
					klog.ErrorS(fmt.Errorf("unknown strategy name"), "skipping strategy", "strategy", name)
					return nil, fmt.Errorf("unknown strategy name: %v", name)
				}

				profile := api.Profile{
					Name: fmt.Sprintf("strategy-%v-profile", name),
					PluginConfigs: []api.PluginConfig{
						{
							Name: defaultevictor.PluginName,
							Args: &defaultevictor.DefaultEvictorArgs{
								EvictLocalStoragePods:   evictLocalStoragePods,
								EvictSystemCriticalPods: evictSystemCriticalPods,
								IgnorePvcPods:           ignorePvcPods,
								EvictFailedBarePods:     evictBarePods,
								NodeFit:                 nodeFit,
								PriorityThreshold: &api.PriorityThreshold{
									Value: &thresholdPriority,
								},
							},
						},
						*pluginConfig,
					},
					Plugins: api.Plugins{
						Evict: api.PluginSet{
							Enabled: []string{defaultevictor.PluginName},
						},
					},
				}

				pluginArgs := registry[string(name)].PluginArgInstance
				pluginInstance, err := registry[string(name)].PluginBuilder(pluginArgs, &handleImpl{})
				if err != nil {
					klog.ErrorS(fmt.Errorf("could not build plugin"), "plugin build error", "plugin", name)
					return nil, fmt.Errorf("could not build plugin: %v", name)
				}

				// pluginInstance can be of any of each type, or both
				profilePlugins := profile.Plugins
				profile.Plugins = enableProfilePluginsByType(profilePlugins, pluginInstance, pluginConfig)
				profiles = append(profiles, profile)
			}
		} else {
			klog.ErrorS(fmt.Errorf("unknown strategy name"), "skipping strategy", "strategy", name)
			return nil, fmt.Errorf("unknown strategy name: %v", name)
		}
	}

	return &api.DeschedulerPolicy{
		Profiles:                       profiles,
		NodeSelector:                   deschedulerPolicy.NodeSelector,
		MaxNoOfPodsToEvictPerNode:      deschedulerPolicy.MaxNoOfPodsToEvictPerNode,
		MaxNoOfPodsToEvictPerNamespace: deschedulerPolicy.MaxNoOfPodsToEvictPerNamespace,
	}, nil
}

func enableProfilePluginsByType(profilePlugins api.Plugins, pluginInstance framework.Plugin, pluginConfig *api.PluginConfig) api.Plugins {
	profilePlugins = checkBalance(profilePlugins, pluginInstance, pluginConfig)
	profilePlugins = checkDeschedule(profilePlugins, pluginInstance, pluginConfig)
	return profilePlugins
}

func checkBalance(profilePlugins api.Plugins, pluginInstance framework.Plugin, pluginConfig *api.PluginConfig) api.Plugins {
	_, ok := pluginInstance.(framework.BalancePlugin)
	if ok {
		klog.V(3).Info("converting Balance plugin: %s", pluginInstance.Name())
		profilePlugins.Balance.Enabled = []string{pluginConfig.Name}
	}
	return profilePlugins
}

func checkDeschedule(profilePlugins api.Plugins, pluginInstance framework.Plugin, pluginConfig *api.PluginConfig) api.Plugins {
	_, ok := pluginInstance.(framework.DeschedulePlugin)
	if ok {
		klog.V(3).Info("converting Deschedule plugin: %s", pluginInstance.Name())
		profilePlugins.Deschedule.Enabled = []string{pluginConfig.Name}
	}
	return profilePlugins
}
