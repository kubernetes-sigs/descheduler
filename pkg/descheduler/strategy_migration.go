/*
Copyright 2022 The Kubernetes Authors.

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
	"fmt"

	"k8s.io/klog/v2"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/api/v1alpha1"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/nodeutilization"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/podlifetime"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removeduplicates"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removefailedpods"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodshavingtoomanyrestarts"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodsviolatinginterpodantiaffinity"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodsviolatingnodeaffinity"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodsviolatingnodetaints"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodsviolatingtopologyspreadconstraint"
)

// Once all strategies are migrated the arguments get read from the configuration file
// without any wiring. Keeping the wiring here so the descheduler can still use
// the v1alpha1 configuration during the strategy migration to plugins.

var strategyParamsToPluginArgs = map[string]func(params *v1alpha1.StrategyParameters) (*api.PluginConfig, error){
	"RemovePodsViolatingNodeTaints": func(params *v1alpha1.StrategyParameters) (*api.PluginConfig, error) {
		args := &removepodsviolatingnodetaints.RemovePodsViolatingNodeTaintsArgs{
			Namespaces:              v1alpha1NamespacesToInternal(params.Namespaces),
			LabelSelector:           params.LabelSelector,
			IncludePreferNoSchedule: params.IncludePreferNoSchedule,
			ExcludedTaints:          params.ExcludedTaints,
		}
		if err := removepodsviolatingnodetaints.ValidateRemovePodsViolatingNodeTaintsArgs(args); err != nil {
			klog.ErrorS(err, "unable to validate plugin arguments", "pluginName", removepodsviolatingnodetaints.PluginName)
			return nil, fmt.Errorf("strategy %q param validation failed: %v", removepodsviolatingnodetaints.PluginName, err)
		}
		return &api.PluginConfig{
			Name: removepodsviolatingnodetaints.PluginName,
			Args: args,
		}, nil
	},
	"RemoveFailedPods": func(params *v1alpha1.StrategyParameters) (*api.PluginConfig, error) {
		failedPodsParams := params.FailedPods
		if failedPodsParams == nil {
			failedPodsParams = &v1alpha1.FailedPods{}
		}
		args := &removefailedpods.RemoveFailedPodsArgs{
			Namespaces:              v1alpha1NamespacesToInternal(params.Namespaces),
			LabelSelector:           params.LabelSelector,
			IncludingInitContainers: failedPodsParams.IncludingInitContainers,
			MinPodLifetimeSeconds:   failedPodsParams.MinPodLifetimeSeconds,
			ExcludeOwnerKinds:       failedPodsParams.ExcludeOwnerKinds,
			Reasons:                 failedPodsParams.Reasons,
		}
		if err := removefailedpods.ValidateRemoveFailedPodsArgs(args); err != nil {
			klog.ErrorS(err, "unable to validate plugin arguments", "pluginName", removefailedpods.PluginName)
			return nil, fmt.Errorf("strategy %q param validation failed: %v", removefailedpods.PluginName, err)
		}
		return &api.PluginConfig{
			Name: removefailedpods.PluginName,
			Args: args,
		}, nil
	},
	"RemovePodsViolatingNodeAffinity": func(params *v1alpha1.StrategyParameters) (*api.PluginConfig, error) {
		args := &removepodsviolatingnodeaffinity.RemovePodsViolatingNodeAffinityArgs{
			Namespaces:       v1alpha1NamespacesToInternal(params.Namespaces),
			LabelSelector:    params.LabelSelector,
			NodeAffinityType: params.NodeAffinityType,
		}
		if err := removepodsviolatingnodeaffinity.ValidateRemovePodsViolatingNodeAffinityArgs(args); err != nil {
			klog.ErrorS(err, "unable to validate plugin arguments", "pluginName", removepodsviolatingnodeaffinity.PluginName)
			return nil, fmt.Errorf("strategy %q param validation failed: %v", removepodsviolatingnodeaffinity.PluginName, err)
		}
		return &api.PluginConfig{
			Name: removepodsviolatingnodeaffinity.PluginName,
			Args: args,
		}, nil
	},
	"RemovePodsViolatingInterPodAntiAffinity": func(params *v1alpha1.StrategyParameters) (*api.PluginConfig, error) {
		args := &removepodsviolatinginterpodantiaffinity.RemovePodsViolatingInterPodAntiAffinityArgs{
			Namespaces:    v1alpha1NamespacesToInternal(params.Namespaces),
			LabelSelector: params.LabelSelector,
		}
		if err := removepodsviolatinginterpodantiaffinity.ValidateRemovePodsViolatingInterPodAntiAffinityArgs(args); err != nil {
			klog.ErrorS(err, "unable to validate plugin arguments", "pluginName", removepodsviolatinginterpodantiaffinity.PluginName)
			return nil, fmt.Errorf("strategy %q param validation failed: %v", removepodsviolatinginterpodantiaffinity.PluginName, err)
		}
		return &api.PluginConfig{
			Name: removepodsviolatinginterpodantiaffinity.PluginName,
			Args: args,
		}, nil
	},
	"RemovePodsHavingTooManyRestarts": func(params *v1alpha1.StrategyParameters) (*api.PluginConfig, error) {
		tooManyRestartsParams := params.PodsHavingTooManyRestarts
		if tooManyRestartsParams == nil {
			tooManyRestartsParams = &v1alpha1.PodsHavingTooManyRestarts{}
		}
		args := &removepodshavingtoomanyrestarts.RemovePodsHavingTooManyRestartsArgs{
			Namespaces:              v1alpha1NamespacesToInternal(params.Namespaces),
			LabelSelector:           params.LabelSelector,
			PodRestartThreshold:     tooManyRestartsParams.PodRestartThreshold,
			IncludingInitContainers: tooManyRestartsParams.IncludingInitContainers,
		}
		if err := removepodshavingtoomanyrestarts.ValidateRemovePodsHavingTooManyRestartsArgs(args); err != nil {
			klog.ErrorS(err, "unable to validate plugin arguments", "pluginName", removepodshavingtoomanyrestarts.PluginName)
			return nil, fmt.Errorf("strategy %q param validation failed: %v", removepodshavingtoomanyrestarts.PluginName, err)
		}
		return &api.PluginConfig{
			Name: removepodshavingtoomanyrestarts.PluginName,
			Args: args,
		}, nil
	},
	"PodLifeTime": func(params *v1alpha1.StrategyParameters) (*api.PluginConfig, error) {
		podLifeTimeParams := params.PodLifeTime
		if podLifeTimeParams == nil {
			podLifeTimeParams = &v1alpha1.PodLifeTime{}
		}

		var states []string
		if podLifeTimeParams.PodStatusPhases != nil {
			states = append(states, podLifeTimeParams.PodStatusPhases...)
		}
		if podLifeTimeParams.States != nil {
			states = append(states, podLifeTimeParams.States...)
		}

		args := &podlifetime.PodLifeTimeArgs{
			Namespaces:            v1alpha1NamespacesToInternal(params.Namespaces),
			LabelSelector:         params.LabelSelector,
			MaxPodLifeTimeSeconds: podLifeTimeParams.MaxPodLifeTimeSeconds,
			States:                states,
		}
		if err := podlifetime.ValidatePodLifeTimeArgs(args); err != nil {
			klog.ErrorS(err, "unable to validate plugin arguments", "pluginName", podlifetime.PluginName)
			return nil, fmt.Errorf("strategy %q param validation failed: %v", podlifetime.PluginName, err)
		}
		return &api.PluginConfig{
			Name: podlifetime.PluginName,
			Args: args,
		}, nil
	},
	"RemoveDuplicates": func(params *v1alpha1.StrategyParameters) (*api.PluginConfig, error) {
		args := &removeduplicates.RemoveDuplicatesArgs{
			Namespaces: v1alpha1NamespacesToInternal(params.Namespaces),
		}
		if params.RemoveDuplicates != nil {
			args.ExcludeOwnerKinds = params.RemoveDuplicates.ExcludeOwnerKinds
		}
		if err := removeduplicates.ValidateRemoveDuplicatesArgs(args); err != nil {
			klog.ErrorS(err, "unable to validate plugin arguments", "pluginName", removeduplicates.PluginName)
			return nil, fmt.Errorf("strategy %q param validation failed: %v", removeduplicates.PluginName, err)
		}
		return &api.PluginConfig{
			Name: removeduplicates.PluginName,
			Args: args,
		}, nil
	},
	"RemovePodsViolatingTopologySpreadConstraint": func(params *v1alpha1.StrategyParameters) (*api.PluginConfig, error) {
		args := &removepodsviolatingtopologyspreadconstraint.RemovePodsViolatingTopologySpreadConstraintArgs{
			Namespaces:             v1alpha1NamespacesToInternal(params.Namespaces),
			LabelSelector:          params.LabelSelector,
			IncludeSoftConstraints: params.IncludeSoftConstraints,
		}
		if err := removepodsviolatingtopologyspreadconstraint.ValidateRemovePodsViolatingTopologySpreadConstraintArgs(args); err != nil {
			klog.ErrorS(err, "unable to validate plugin arguments", "pluginName", removepodsviolatingtopologyspreadconstraint.PluginName)
			return nil, fmt.Errorf("strategy %q param validation failed: %v", removepodsviolatingtopologyspreadconstraint.PluginName, err)
		}
		return &api.PluginConfig{
			Name: removepodsviolatingtopologyspreadconstraint.PluginName,
			Args: args,
		}, nil
	},
	"HighNodeUtilization": func(params *v1alpha1.StrategyParameters) (*api.PluginConfig, error) {
		if params.NodeResourceUtilizationThresholds == nil {
			params.NodeResourceUtilizationThresholds = &v1alpha1.NodeResourceUtilizationThresholds{}
		}
		args := &nodeutilization.HighNodeUtilizationArgs{
			EvictableNamespaces: v1alpha1NamespacesToInternal(params.Namespaces),
			Thresholds:          v1alpha1ThresholdToInternal(params.NodeResourceUtilizationThresholds.Thresholds),
			NumberOfNodes:       params.NodeResourceUtilizationThresholds.NumberOfNodes,
		}
		if err := nodeutilization.ValidateHighNodeUtilizationArgs(args); err != nil {
			klog.ErrorS(err, "unable to validate plugin arguments", "pluginName", nodeutilization.HighNodeUtilizationPluginName)
			return nil, fmt.Errorf("strategy %q param validation failed: %v", nodeutilization.HighNodeUtilizationPluginName, err)
		}
		return &api.PluginConfig{
			Name: nodeutilization.HighNodeUtilizationPluginName,
			Args: args,
		}, nil
	},
	"LowNodeUtilization": func(params *v1alpha1.StrategyParameters) (*api.PluginConfig, error) {
		if params.NodeResourceUtilizationThresholds == nil {
			params.NodeResourceUtilizationThresholds = &v1alpha1.NodeResourceUtilizationThresholds{}
		}
		args := &nodeutilization.LowNodeUtilizationArgs{
			EvictableNamespaces:    v1alpha1NamespacesToInternal(params.Namespaces),
			Thresholds:             v1alpha1ThresholdToInternal(params.NodeResourceUtilizationThresholds.Thresholds),
			TargetThresholds:       v1alpha1ThresholdToInternal(params.NodeResourceUtilizationThresholds.TargetThresholds),
			UseDeviationThresholds: params.NodeResourceUtilizationThresholds.UseDeviationThresholds,
			NumberOfNodes:          params.NodeResourceUtilizationThresholds.NumberOfNodes,
		}

		if err := nodeutilization.ValidateLowNodeUtilizationArgs(args); err != nil {
			klog.ErrorS(err, "unable to validate plugin arguments", "pluginName", nodeutilization.LowNodeUtilizationPluginName)
			return nil, fmt.Errorf("strategy %q param validation failed: %v", nodeutilization.LowNodeUtilizationPluginName, err)
		}
		return &api.PluginConfig{
			Name: nodeutilization.LowNodeUtilizationPluginName,
			Args: args,
		}, nil
	},
}

func v1alpha1NamespacesToInternal(namespaces *v1alpha1.Namespaces) *api.Namespaces {
	internal := &api.Namespaces{}
	if namespaces != nil {
		if namespaces.Exclude != nil {
			internal.Exclude = namespaces.Exclude
		}
		if namespaces.Include != nil {
			internal.Include = namespaces.Include
		}
	} else {
		internal = nil
	}
	return internal
}

func v1alpha1ThresholdToInternal(thresholds v1alpha1.ResourceThresholds) api.ResourceThresholds {
	internal := make(api.ResourceThresholds, len(thresholds))
	for k, v := range thresholds {
		internal[k] = api.Percentage(float64(v))
	}
	return internal
}

type extensionPoint string

const (
	descheduleEP extensionPoint = "deschedule"
	balanceEP    extensionPoint = "balance"
)

var pluginToExtensionPoint = map[string]extensionPoint{
	removepodsviolatingnodetaints.PluginName:               descheduleEP,
	removefailedpods.PluginName:                            descheduleEP,
	removepodsviolatingnodeaffinity.PluginName:             descheduleEP,
	removepodsviolatinginterpodantiaffinity.PluginName:     descheduleEP,
	removepodshavingtoomanyrestarts.PluginName:             descheduleEP,
	podlifetime.PluginName:                                 descheduleEP,
	removeduplicates.PluginName:                            balanceEP,
	removepodsviolatingtopologyspreadconstraint.PluginName: balanceEP,
	nodeutilization.HighNodeUtilizationPluginName:          balanceEP,
	nodeutilization.LowNodeUtilizationPluginName:           balanceEP,
}
