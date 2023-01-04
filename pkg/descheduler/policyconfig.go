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
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/api/v1alpha1"
	"sigs.k8s.io/descheduler/pkg/descheduler/scheme"
	"sigs.k8s.io/descheduler/pkg/framework"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/pluginbuilder"
	"sigs.k8s.io/descheduler/pkg/utils"
)

func LoadPolicyConfig(policyConfigFile string, client clientset.Interface, registry pluginbuilder.Registry) (*api.DeschedulerPolicy, error) {
	if policyConfigFile == "" {
		klog.V(1).InfoS("Policy config file not specified")
		return nil, nil
	}

	policy, err := os.ReadFile(policyConfigFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read policy config file %q: %+v", policyConfigFile, err)
	}

	versionedPolicy := &v1alpha1.DeschedulerPolicy{}

	decoder := scheme.Codecs.UniversalDecoder(v1alpha1.SchemeGroupVersion)
	if err := runtime.DecodeInto(decoder, policy, versionedPolicy); err != nil {
		return nil, fmt.Errorf("failed decoding descheduler's policy config %q: %v", policyConfigFile, err)
	}

	// Build profiles
	internalPolicy, err := V1alpha1ToInternal(client, versionedPolicy, registry)
	if err != nil {
		return nil, fmt.Errorf("failed converting versioned policy to internal policy version: %v", err)
	}

	return internalPolicy, nil
}

func V1alpha1ToInternal(
	client clientset.Interface,
	deschedulerPolicy *v1alpha1.DeschedulerPolicy,
	registry pluginbuilder.Registry,
) (*api.DeschedulerPolicy, error) {
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

	var profiles []api.Profile

	// Build profiles
	for name, strategy := range deschedulerPolicy.Strategies {
		if _, ok := pluginbuilder.PluginRegistry[string(name)]; ok {
			if strategy.Enabled {
				params := strategy.Params
				if params == nil {
					params = &v1alpha1.StrategyParameters{}
				}

				nodeFit := false
				if name != "PodLifeTime" {
					nodeFit = params.NodeFit
				}

				// TODO(jchaloup): once all strategies are migrated move this check under
				// the default evictor args validation
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
				if pcFnc, exists := strategyParamsToPluginArgs[string(name)]; exists {
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
	switch p := pluginInstance.(type) {
	case framework.BalancePlugin:
		klog.V(3).Info("converting Balance plugin: %s", p.Name())
		profilePlugins.Balance.Enabled = []string{pluginConfig.Name}
	}
	return profilePlugins
}

func checkDeschedule(profilePlugins api.Plugins, pluginInstance framework.Plugin, pluginConfig *api.PluginConfig) api.Plugins {
	switch p := pluginInstance.(type) {
	case framework.DeschedulePlugin:
		klog.V(3).Info("converting Deschedule plugin: %s", p.Name())
		profilePlugins.Deschedule.Enabled = []string{pluginConfig.Name}
	}
	return profilePlugins
}
