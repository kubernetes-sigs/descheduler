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
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removefailedpods"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodsviolatingnodetaints"
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
		args := &componentconfig.RemoveFailedPodsArgs{
			Namespaces:              params.Namespaces,
			LabelSelector:           params.LabelSelector,
			IncludingInitContainers: failedPodsParams.IncludingInitContainers,
			MinPodLifetimeSeconds:   failedPodsParams.MinPodLifetimeSeconds,
			ExcludeOwnerKinds:       failedPodsParams.ExcludeOwnerKinds,
			Reasons:                 failedPodsParams.Reasons,
		}
		if err := validation.ValidateRemoveFailedPodsArgs(args); err != nil {
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
}
