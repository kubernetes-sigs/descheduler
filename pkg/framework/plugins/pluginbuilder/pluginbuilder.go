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

package pluginbuilder

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"sigs.k8s.io/descheduler/pkg/framework"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
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

var PluginRegistry Registry

type PluginBuilderAndArgsInstance struct {
	PluginBuilder PluginBuilder
	// Just an example instance of this PluginArg so we can avoid having
	// to deal with reflect Types
	PluginArgInstance runtime.Object
	Balance           bool
}

type PluginBuilder = func(args runtime.Object, handle framework.Handle) (framework.Plugin, error)

type Registry = map[string]PluginBuilderAndArgsInstance

func NewRegistry() Registry {
	return Registry{}
}

func Register(name string, builderFunc PluginBuilder, exampleArg runtime.Object, balance bool, registry Registry) {
	if _, ok := registry[name]; ok {
		klog.V(10).InfoS("Plugin already registered", "plugin", name)
	} else {
		registry[name] = PluginBuilderAndArgsInstance{
			PluginBuilder:     builderFunc,
			PluginArgInstance: exampleArg,
			Balance:           balance,
		}
	}
}

func RegisterDefault(registry Registry) {
	Register(defaultevictor.PluginName, defaultevictor.New, &defaultevictor.DefaultEvictorArgs{}, false, registry)
	Register(nodeutilization.LowNodeUtilizationPluginName, nodeutilization.NewLowNodeUtilization, &nodeutilization.LowNodeUtilizationArgs{}, true, registry)
	Register(nodeutilization.HighNodeUtilizationPluginName, nodeutilization.NewHighNodeUtilization, &nodeutilization.HighNodeUtilizationArgs{}, true, registry)
	Register(podlifetime.PluginName, podlifetime.New, &podlifetime.PodLifeTimeArgs{}, false, registry)
	Register(removeduplicates.PluginName, removeduplicates.New, &removeduplicates.RemoveDuplicatesArgs{}, true, registry)
	Register(removefailedpods.PluginName, removefailedpods.New, &removefailedpods.RemoveFailedPodsArgs{}, false, registry)
	Register(removepodshavingtoomanyrestarts.PluginName, removepodshavingtoomanyrestarts.New, &removepodshavingtoomanyrestarts.RemovePodsHavingTooManyRestartsArgs{}, false, registry)
	Register(removepodsviolatinginterpodantiaffinity.PluginName, removepodsviolatinginterpodantiaffinity.New, &removepodsviolatinginterpodantiaffinity.RemovePodsViolatingInterPodAntiAffinityArgs{}, false, registry)
	Register(removepodsviolatingnodeaffinity.PluginName, removepodsviolatingnodeaffinity.New, &removepodsviolatingnodeaffinity.RemovePodsViolatingNodeAffinityArgs{}, false, registry)
	Register(removepodsviolatingnodetaints.PluginName, removepodsviolatingnodetaints.New, &removepodsviolatingnodetaints.RemovePodsViolatingNodeTaintsArgs{}, false, registry)
	Register(removepodsviolatingtopologyspreadconstraint.PluginName, removepodsviolatingtopologyspreadconstraint.New, &removepodsviolatingtopologyspreadconstraint.RemovePodsViolatingTopologySpreadConstraintArgs{}, true, registry)
}
