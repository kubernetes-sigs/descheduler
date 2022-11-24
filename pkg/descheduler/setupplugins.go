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
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/nodeutilization"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/pluginbuilder"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/podlifetime"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removeduplicates"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removefailedpods"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodshavingtoomanyrestarts"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodsviolatinginterpodantiaffinity"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodsviolatingnodeaffinity"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodsviolatingnodetaints"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodsviolatingtopologyspreadconstraint"
)

func SetupPlugins() {
	pluginbuilder.PluginRegistry = pluginbuilder.NewRegistry()
	RegisterDefaultPlugins(pluginbuilder.PluginRegistry)
}

func RegisterDefaultPlugins(registry pluginbuilder.Registry) {
	pluginbuilder.Register(defaultevictor.PluginName, defaultevictor.New, &defaultevictor.DefaultEvictorArgs{}, registry)
	pluginbuilder.Register(nodeutilization.LowNodeUtilizationPluginName, nodeutilization.NewLowNodeUtilization, &nodeutilization.LowNodeUtilizationArgs{}, registry)
	pluginbuilder.Register(nodeutilization.HighNodeUtilizationPluginName, nodeutilization.NewHighNodeUtilization, &nodeutilization.HighNodeUtilizationArgs{}, registry)
	pluginbuilder.Register(podlifetime.PluginName, podlifetime.New, &podlifetime.PodLifeTimeArgs{}, registry)
	pluginbuilder.Register(removeduplicates.PluginName, removeduplicates.New, &removeduplicates.RemoveDuplicatesArgs{}, registry)
	pluginbuilder.Register(removefailedpods.PluginName, removefailedpods.New, &removefailedpods.RemoveFailedPodsArgs{}, registry)
	pluginbuilder.Register(removepodshavingtoomanyrestarts.PluginName, removepodshavingtoomanyrestarts.New, &removepodshavingtoomanyrestarts.RemovePodsHavingTooManyRestartsArgs{}, registry)
	pluginbuilder.Register(removepodsviolatinginterpodantiaffinity.PluginName, removepodsviolatinginterpodantiaffinity.New, &removepodsviolatinginterpodantiaffinity.RemovePodsViolatingInterPodAntiAffinityArgs{}, registry)
	pluginbuilder.Register(removepodsviolatingnodeaffinity.PluginName, removepodsviolatingnodeaffinity.New, &removepodsviolatingnodeaffinity.RemovePodsViolatingNodeAffinityArgs{}, registry)
	pluginbuilder.Register(removepodsviolatingnodetaints.PluginName, removepodsviolatingnodetaints.New, &removepodsviolatingnodetaints.RemovePodsViolatingNodeTaintsArgs{}, registry)
	pluginbuilder.Register(removepodsviolatingtopologyspreadconstraint.PluginName, removepodsviolatingtopologyspreadconstraint.New, &removepodsviolatingtopologyspreadconstraint.RemovePodsViolatingTopologySpreadConstraintArgs{}, registry)
}
