package registry

import (
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/descheduler/pkg/framework"

	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/nodeutilization"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/podlifetime"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removeduplicatepods"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removefailedpods"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodshavingtoomanyrestarts"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodsviolatinginterpodantiaffinity"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodsviolatingnodeaffinity"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodsviolatingnodetaints"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodsviolatingtopologyspreadconstraint"
)

type PluginBuilder = func(args runtime.Object, handle framework.Handle) (framework.Plugin, error)

type Registry = map[string]PluginBuilder

func NewRegistry() Registry {
	return Registry{
		nodeutilization.HighNodeUtilizationPluginName:          nodeutilization.NewHighNodeUtilization,
		nodeutilization.LowNodeUtilizationPluginName:           nodeutilization.NewLowNodeUtilization,
		podlifetime.PluginName:                                 podlifetime.New,
		removeduplicatepods.PluginName:                         removeduplicatepods.New,
		removefailedpods.PluginName:                            removefailedpods.New,
		removepodshavingtoomanyrestarts.PluginName:             removepodshavingtoomanyrestarts.New,
		removepodsviolatinginterpodantiaffinity.PluginName:     removepodsviolatinginterpodantiaffinity.New,
		removepodsviolatingnodeaffinity.PluginName:             removepodsviolatingnodeaffinity.New,
		removepodsviolatingnodetaints.PluginName:               removepodsviolatingnodetaints.New,
		removepodsviolatingtopologyspreadconstraint.PluginName: removepodsviolatingtopologyspreadconstraint.New,
		defaultevictor.PluginName:                              defaultevictor.New,
	}
}
