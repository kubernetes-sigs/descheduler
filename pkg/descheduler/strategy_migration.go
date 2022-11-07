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

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/framework"
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

var strategyParamsToPluginArgs = map[string]func(params *api.StrategyParameters) (*api.PluginConfig, error){
	"RemovePodsViolatingNodeTaints": func(params *api.StrategyParameters) (*api.PluginConfig, error) {
		args := &removepodsviolatingnodetaints.RemovePodsViolatingNodeTaintsArgs{
			Namespaces:              params.Namespaces,
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
	"RemoveFailedPods": func(params *api.StrategyParameters) (*api.PluginConfig, error) {
		failedPodsParams := params.FailedPods
		if failedPodsParams == nil {
			failedPodsParams = &api.FailedPods{}
		}
		args := &removefailedpods.RemoveFailedPodsArgs{
			Namespaces:              params.Namespaces,
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
	"RemovePodsViolatingNodeAffinity": func(params *api.StrategyParameters) (*api.PluginConfig, error) {
		args := &removepodsviolatingnodeaffinity.RemovePodsViolatingNodeAffinityArgs{
			Namespaces:       params.Namespaces,
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
	"RemovePodsViolatingInterPodAntiAffinity": func(params *api.StrategyParameters) (*api.PluginConfig, error) {
		args := &removepodsviolatinginterpodantiaffinity.RemovePodsViolatingInterPodAntiAffinityArgs{
			Namespaces:    params.Namespaces,
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
	"RemovePodsHavingTooManyRestarts": func(params *api.StrategyParameters) (*api.PluginConfig, error) {
		tooManyRestartsParams := params.PodsHavingTooManyRestarts
		if tooManyRestartsParams == nil {
			tooManyRestartsParams = &api.PodsHavingTooManyRestarts{}
		}
		args := &removepodshavingtoomanyrestarts.RemovePodsHavingTooManyRestartsArgs{
			Namespaces:              params.Namespaces,
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
	"PodLifeTime": func(params *api.StrategyParameters) (*api.PluginConfig, error) {
		podLifeTimeParams := params.PodLifeTime
		if podLifeTimeParams == nil {
			podLifeTimeParams = &api.PodLifeTime{}
		}

		var states []string
		if podLifeTimeParams.PodStatusPhases != nil {
			states = append(states, podLifeTimeParams.PodStatusPhases...)
		}
		if podLifeTimeParams.States != nil {
			states = append(states, podLifeTimeParams.States...)
		}

		args := &podlifetime.PodLifeTimeArgs{
			Namespaces:            params.Namespaces,
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
	"RemoveDuplicates": func(params *api.StrategyParameters) (*api.PluginConfig, error) {
		args := &removeduplicates.RemoveDuplicatesArgs{
			Namespaces: params.Namespaces,
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
	"RemovePodsViolatingTopologySpreadConstraint": func(params *api.StrategyParameters) (*api.PluginConfig, error) {
		args := &removepodsviolatingtopologyspreadconstraint.RemovePodsViolatingTopologySpreadConstraintArgs{
			Namespaces:             params.Namespaces,
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
	"HighNodeUtilization": func(params *api.StrategyParameters) (*api.PluginConfig, error) {
		args := &nodeutilization.HighNodeUtilizationArgs{
			Thresholds:    params.NodeResourceUtilizationThresholds.Thresholds,
			NumberOfNodes: params.NodeResourceUtilizationThresholds.NumberOfNodes,
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
	"LowNodeUtilization": func(params *api.StrategyParameters) (*api.PluginConfig, error) {
		args := &nodeutilization.LowNodeUtilizationArgs{
			Thresholds:             params.NodeResourceUtilizationThresholds.Thresholds,
			TargetThresholds:       params.NodeResourceUtilizationThresholds.TargetThresholds,
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

var pluginsMap = map[string]func(args runtime.Object, handle *handleImpl) framework.Plugin{
	"RemovePodsViolatingNodeTaints": func(args runtime.Object, handle *handleImpl) framework.Plugin {
		pg, err := removepodsviolatingnodetaints.New(args, handle)
		if err != nil {
			klog.ErrorS(err, "unable to initialize a plugin", "pluginName", removepodsviolatingnodetaints.PluginName)
			return nil
		}
		return pg
	},
	"RemoveFailedPods": func(args runtime.Object, handle *handleImpl) framework.Plugin {
		pg, err := removefailedpods.New(args, handle)
		if err != nil {
			klog.ErrorS(err, "unable to initialize a plugin", "pluginName", removefailedpods.PluginName)
			return nil
		}
		return pg
	},
	"RemovePodsViolatingNodeAffinity": func(args runtime.Object, handle *handleImpl) framework.Plugin {
		pg, err := removepodsviolatingnodeaffinity.New(args, handle)
		if err != nil {
			klog.ErrorS(err, "unable to initialize a plugin", "pluginName", removepodsviolatingnodeaffinity.PluginName)
			return nil
		}
		return pg
	},
	"RemovePodsViolatingInterPodAntiAffinity": func(args runtime.Object, handle *handleImpl) framework.Plugin {
		pg, err := removepodsviolatinginterpodantiaffinity.New(args, handle)
		if err != nil {
			klog.ErrorS(err, "unable to initialize a plugin", "pluginName", removepodsviolatinginterpodantiaffinity.PluginName)
			return nil
		}
		return pg
	},
	"RemovePodsHavingTooManyRestarts": func(args runtime.Object, handle *handleImpl) framework.Plugin {
		pg, err := removepodshavingtoomanyrestarts.New(args, handle)
		if err != nil {
			klog.ErrorS(err, "unable to initialize a plugin", "pluginName", removepodshavingtoomanyrestarts.PluginName)
			return nil
		}
		return pg
	},
	"PodLifeTime": func(args runtime.Object, handle *handleImpl) framework.Plugin {
		pg, err := podlifetime.New(args, handle)
		if err != nil {
			klog.ErrorS(err, "unable to initialize a plugin", "pluginName", podlifetime.PluginName)
			return nil
		}
		return pg
	},
	"RemoveDuplicates": func(args runtime.Object, handle *handleImpl) framework.Plugin {
		pg, err := removeduplicates.New(args, handle)
		if err != nil {
			klog.ErrorS(err, "unable to initialize a plugin", "pluginName", removeduplicates.PluginName)
			return nil
		}
		return pg
	},
	"RemovePodsViolatingTopologySpreadConstraint": func(args runtime.Object, handle *handleImpl) framework.Plugin {
		pg, err := removepodsviolatingtopologyspreadconstraint.New(args, handle)
		if err != nil {
			klog.ErrorS(err, "unable to initialize a plugin", "pluginName", removepodsviolatingtopologyspreadconstraint.PluginName)
			return nil
		}
		return pg
	},
	"HighNodeUtilization": func(args runtime.Object, handle *handleImpl) framework.Plugin {
		pg, err := nodeutilization.NewHighNodeUtilization(args, handle)
		if err != nil {
			klog.ErrorS(err, "unable to initialize a plugin", "pluginName", nodeutilization.HighNodeUtilizationPluginName)
			return nil
		}
		return pg
	},
	"LowNodeUtilization": func(args runtime.Object, handle *handleImpl) framework.Plugin {
		pg, err := nodeutilization.NewLowNodeUtilization(args, handle)
		if err != nil {
			klog.ErrorS(err, "unable to initialize a plugin", "pluginName", nodeutilization.LowNodeUtilizationPluginName)
			return nil
		}
		return pg
	},
}
