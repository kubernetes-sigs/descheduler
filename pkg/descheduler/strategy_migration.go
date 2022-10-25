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
	"k8s.io/apimachinery/pkg/runtime"

	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
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

var pluginsMap = map[string]func(ctx context.Context, nodes []*v1.Node, pluginArgs runtime.Object, handle *handleImpl){
	"RemovePodsViolatingNodeTaints": func(ctx context.Context, nodes []*v1.Node, pluginArgs runtime.Object, handle *handleImpl) {
		args := pluginArgs.(*removepodsviolatingnodetaints.RemovePodsViolatingNodeTaintsArgs)
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
	"RemoveFailedPods": func(ctx context.Context, nodes []*v1.Node, pluginArgs runtime.Object, handle *handleImpl) {
		args := pluginArgs.(*removefailedpods.RemoveFailedPodsArgs)
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
	"RemovePodsViolatingNodeAffinity": func(ctx context.Context, nodes []*v1.Node, pluginArgs runtime.Object, handle *handleImpl) {
		args := pluginArgs.(*removepodsviolatingnodeaffinity.RemovePodsViolatingNodeAffinityArgs)
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
	"RemovePodsViolatingInterPodAntiAffinity": func(ctx context.Context, nodes []*v1.Node, pluginArgs runtime.Object, handle *handleImpl) {
		args := pluginArgs.(*removepodsviolatinginterpodantiaffinity.RemovePodsViolatingInterPodAntiAffinityArgs)
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
	"RemovePodsHavingTooManyRestarts": func(ctx context.Context, nodes []*v1.Node, pluginArgs runtime.Object, handle *handleImpl) {
		args := pluginArgs.(*removepodshavingtoomanyrestarts.RemovePodsHavingTooManyRestartsArgs)
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
	"PodLifeTime": func(ctx context.Context, nodes []*v1.Node, pluginArgs runtime.Object, handle *handleImpl) {
		args := pluginArgs.(*podlifetime.PodLifeTimeArgs)
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
	"RemoveDuplicates": func(ctx context.Context, nodes []*v1.Node, pluginArgs runtime.Object, handle *handleImpl) {
		args := pluginArgs.(*removeduplicates.RemoveDuplicatesArgs)
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
	"RemovePodsViolatingTopologySpreadConstraint": func(ctx context.Context, nodes []*v1.Node, pluginArgs runtime.Object, handle *handleImpl) {
		args := pluginArgs.(*removepodsviolatingtopologyspreadconstraint.RemovePodsViolatingTopologySpreadConstraintArgs)
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
	"HighNodeUtilization": func(ctx context.Context, nodes []*v1.Node, pluginArgs runtime.Object, handle *handleImpl) {
		args := pluginArgs.(*nodeutilization.HighNodeUtilizationArgs)
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
	"LowNodeUtilization": func(ctx context.Context, nodes []*v1.Node, pluginArgs runtime.Object, handle *handleImpl) {
		args := pluginArgs.(*nodeutilization.LowNodeUtilizationArgs)
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
