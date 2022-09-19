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
	"context"

	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/apis/componentconfig"
	"sigs.k8s.io/descheduler/pkg/apis/componentconfig/validation"
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

var pluginsMap = map[string]func(ctx context.Context, nodes []*v1.Node, params *api.StrategyParameters, handle *handleImpl){
	"RemovePodsViolatingNodeTaints": func(ctx context.Context, nodes []*v1.Node, params *api.StrategyParameters, handle *handleImpl) {
		args := &componentconfig.RemovePodsViolatingNodeTaintsArgs{
			Namespaces:              params.Namespaces,
			LabelSelector:           params.LabelSelector,
			IncludePreferNoSchedule: params.IncludePreferNoSchedule,
			ExcludedTaints:          params.ExcludedTaints,
		}
		if err := validation.ValidateRemovePodsViolatingNodeTaintsArgs(args); err != nil {
			klog.V(1).ErrorS(err, "unable to validate plugin arguments", "pluginName", removepodsviolatingnodetaints.PluginName)
			return
		}
		pg, err := removepodsviolatingnodetaints.New(args, handle)
		if err != nil {
			klog.V(1).ErrorS(err, "unable to initialize a plugin", "pluginName", removepodsviolatingnodetaints.PluginName)
			return
		}
		status := pg.(framework.DeschedulePlugin).Deschedule(ctx, nodes)
		if status != nil && status.Err != nil {
			klog.V(1).ErrorS(err, "plugin finished with error", "pluginName", removepodsviolatingnodetaints.PluginName)
		}
	},
	"RemoveFailedPods": func(ctx context.Context, nodes []*v1.Node, params *api.StrategyParameters, handle *handleImpl) {
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
			klog.V(1).ErrorS(err, "unable to validate plugin arguments", "pluginName", removefailedpods.PluginName)
			return
		}
		pg, err := removefailedpods.New(args, handle)
		if err != nil {
			klog.V(1).ErrorS(err, "unable to initialize a plugin", "pluginName", removefailedpods.PluginName)
			return
		}
		status := pg.(framework.DeschedulePlugin).Deschedule(ctx, nodes)
		if status != nil && status.Err != nil {
			klog.V(1).ErrorS(err, "plugin finished with error", "pluginName", removefailedpods.PluginName)
		}
	},
	"RemovePodsViolatingNodeAffinity": func(ctx context.Context, nodes []*v1.Node, params *api.StrategyParameters, handle *handleImpl) {
		args := &componentconfig.RemovePodsViolatingNodeAffinityArgs{
			Namespaces:       params.Namespaces,
			LabelSelector:    params.LabelSelector,
			NodeAffinityType: params.NodeAffinityType,
		}
		if err := validation.ValidateRemovePodsViolatingNodeAffinityArgs(args); err != nil {
			klog.V(1).ErrorS(err, "unable to validate plugin arguments", "pluginName", removepodsviolatingnodeaffinity.PluginName)
			return
		}
		pg, err := removepodsviolatingnodeaffinity.New(args, handle)
		if err != nil {
			klog.V(1).ErrorS(err, "unable to initialize a plugin", "pluginName", removepodsviolatingnodeaffinity.PluginName)
			return
		}
		status := pg.(framework.DeschedulePlugin).Deschedule(ctx, nodes)
		if status != nil && status.Err != nil {
			klog.V(1).ErrorS(err, "plugin finished with error", "pluginName", removepodsviolatingnodeaffinity.PluginName)
		}
	},
	"RemovePodsViolatingInterPodAntiAffinity": func(ctx context.Context, nodes []*v1.Node, params *api.StrategyParameters, handle *handleImpl) {
		args := &componentconfig.RemovePodsViolatingInterPodAntiAffinityArgs{
			Namespaces:    params.Namespaces,
			LabelSelector: params.LabelSelector,
		}
		if err := validation.ValidateRemovePodsViolatingInterPodAntiAffinityArgs(args); err != nil {
			klog.V(1).ErrorS(err, "unable to validate plugin arguments", "pluginName", removepodsviolatinginterpodantiaffinity.PluginName)
			return
		}
		pg, err := removepodsviolatinginterpodantiaffinity.New(args, handle)
		if err != nil {
			klog.V(1).ErrorS(err, "unable to initialize a plugin", "pluginName", removepodsviolatinginterpodantiaffinity.PluginName)
			return
		}
		status := pg.(framework.DeschedulePlugin).Deschedule(ctx, nodes)
		if status != nil && status.Err != nil {
			klog.V(1).ErrorS(err, "plugin finished with error", "pluginName", removepodsviolatinginterpodantiaffinity.PluginName)
		}
	},
	"RemovePodsHavingTooManyRestarts": func(ctx context.Context, nodes []*v1.Node, params *api.StrategyParameters, handle *handleImpl) {
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
			klog.V(1).ErrorS(err, "unable to validate plugin arguments", "pluginName", removepodshavingtoomanyrestarts.PluginName)
			return
		}
		pg, err := removepodshavingtoomanyrestarts.New(args, handle)
		if err != nil {
			klog.V(1).ErrorS(err, "unable to initialize a plugin", "pluginName", removepodshavingtoomanyrestarts.PluginName)
			return
		}
		status := pg.(framework.DeschedulePlugin).Deschedule(ctx, nodes)
		if status != nil && status.Err != nil {
			klog.V(1).ErrorS(err, "plugin finished with error", "pluginName", removepodshavingtoomanyrestarts.PluginName)
		}
	},
	"PodLifeTime": func(ctx context.Context, nodes []*v1.Node, params *api.StrategyParameters, handle *handleImpl) {
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
			klog.V(1).ErrorS(err, "unable to validate plugin arguments", "pluginName", podlifetime.PluginName)
			return
		}
		pg, err := podlifetime.New(args, handle)
		if err != nil {
			klog.V(1).ErrorS(err, "unable to initialize a plugin", "pluginName", podlifetime.PluginName)
			return
		}
		status := pg.(framework.DeschedulePlugin).Deschedule(ctx, nodes)
		if status != nil && status.Err != nil {
			klog.V(1).ErrorS(err, "plugin finished with error", "pluginName", podlifetime.PluginName)
		}
	},
	"RemoveDuplicates": func(ctx context.Context, nodes []*v1.Node, params *api.StrategyParameters, handle *handleImpl) {
		args := &removeduplicates.RemoveDuplicatesArgs{
			Namespaces: params.Namespaces,
		}
		if params.RemoveDuplicates != nil {
			args.ExcludeOwnerKinds = params.RemoveDuplicates.ExcludeOwnerKinds
		}
		if err := removeduplicates.ValidateRemoveDuplicatesArgs(args); err != nil {
			klog.V(1).ErrorS(err, "unable to validate plugin arguments", "pluginName", removeduplicates.PluginName)
			return
		}
		pg, err := removeduplicates.New(args, handle)
		if err != nil {
			klog.V(1).ErrorS(err, "unable to initialize a plugin", "pluginName", removeduplicates.PluginName)
			return
		}
		status := pg.(framework.BalancePlugin).Balance(ctx, nodes)
		if status != nil && status.Err != nil {
			klog.V(1).ErrorS(err, "plugin finished with error", "pluginName", removeduplicates.PluginName)
		}
	},
	"RemovePodsViolatingTopologySpreadConstraint": func(ctx context.Context, nodes []*v1.Node, params *api.StrategyParameters, handle *handleImpl) {
		args := &componentconfig.RemovePodsViolatingTopologySpreadConstraintArgs{
			Namespaces:             params.Namespaces,
			LabelSelector:          params.LabelSelector,
			IncludeSoftConstraints: params.IncludePreferNoSchedule,
		}
		if err := validation.ValidateRemovePodsViolatingTopologySpreadConstraintArgs(args); err != nil {
			klog.V(1).ErrorS(err, "unable to validate plugin arguments", "pluginName", removepodsviolatingtopologyspreadconstraint.PluginName)
			return
		}
		pg, err := removepodsviolatingtopologyspreadconstraint.New(args, handle)
		if err != nil {
			klog.V(1).ErrorS(err, "unable to initialize a plugin", "pluginName", removepodsviolatingtopologyspreadconstraint.PluginName)
			return
		}
		status := pg.(framework.BalancePlugin).Balance(ctx, nodes)
		if status != nil && status.Err != nil {
			klog.V(1).ErrorS(err, "plugin finished with error", "pluginName", removepodsviolatingtopologyspreadconstraint.PluginName)
		}
	},
	"HighNodeUtilization": func(ctx context.Context, nodes []*v1.Node, params *api.StrategyParameters, handle *handleImpl) {
		args := &nodeutilization.HighNodeUtilizationArgs{
			Thresholds:    params.NodeResourceUtilizationThresholds.Thresholds,
			NumberOfNodes: params.NodeResourceUtilizationThresholds.NumberOfNodes,
		}

		if err := nodeutilization.ValidateHighNodeUtilizationArgs(args); err != nil {
			klog.V(1).ErrorS(err, "unable to validate plugin arguments", "pluginName", nodeutilization.HighNodeUtilizationPluginName)
			return
		}
		pg, err := nodeutilization.NewHighNodeUtilization(args, handle)
		if err != nil {
			klog.V(1).ErrorS(err, "unable to initialize a plugin", "pluginName", nodeutilization.HighNodeUtilizationPluginName)
			return
		}
		status := pg.(framework.BalancePlugin).Balance(ctx, nodes)
		if status != nil && status.Err != nil {
			klog.V(1).ErrorS(err, "plugin finished with error", "pluginName", nodeutilization.HighNodeUtilizationPluginName)
		}
	},
	"LowNodeUtilization": func(ctx context.Context, nodes []*v1.Node, params *api.StrategyParameters, handle *handleImpl) {
		args := &nodeutilization.LowNodeUtilizationArgs{
			Thresholds:             params.NodeResourceUtilizationThresholds.Thresholds,
			TargetThresholds:       params.NodeResourceUtilizationThresholds.TargetThresholds,
			UseDeviationThresholds: params.NodeResourceUtilizationThresholds.UseDeviationThresholds,
			NumberOfNodes:          params.NodeResourceUtilizationThresholds.NumberOfNodes,
		}

		if err := nodeutilization.ValidateLowNodeUtilizationArgs(args); err != nil {
			klog.V(1).ErrorS(err, "unable to validate plugin arguments", "pluginName", nodeutilization.LowNodeUtilizationPluginName)
			return
		}
		pg, err := nodeutilization.NewLowNodeUtilization(args, handle)
		if err != nil {
			klog.V(1).ErrorS(err, "unable to initialize a plugin", "pluginName", nodeutilization.LowNodeUtilizationPluginName)
			return
		}
		status := pg.(framework.BalancePlugin).Balance(ctx, nodes)
		if status != nil && status.Err != nil {
			klog.V(1).ErrorS(err, "plugin finished with error", "pluginName", nodeutilization.LowNodeUtilizationPluginName)
		}
	},
}
