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

package pluginregistry

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
)

var PluginRegistry Registry

type PluginUtilities struct {
	PluginBuilder PluginBuilder

	PluginType interface{}
	// Just an example instance of this PluginArg so we can avoid having
	// to deal with reflect Types
	PluginArgInstance  runtime.Object
	PluginArgValidator PluginArgValidator
	PluginArgDefaulter PluginArgDefaulter
}

type PluginBuilder = func(ctx context.Context, args runtime.Object, handle frameworktypes.Handle) (frameworktypes.Plugin, error)

type (
	PluginArgValidator = func(logger klog.Logger, args runtime.Object) error
	PluginArgDefaulter = func(args runtime.Object)
)

type Registry map[string]PluginUtilities

func NewRegistry() Registry {
	return Registry{}
}

func Register(
	ctx context.Context,
	name string,
	builderFunc PluginBuilder,
	pluginType interface{},
	exampleArg runtime.Object,
	pluginArgValidator PluginArgValidator,
	pluginArgDefaulter PluginArgDefaulter,
	registry Registry,
) {
	logger := klog.FromContext(ctx)
	if _, ok := registry[name]; ok {
		logger.V(10).Info("Plugin already registered", "plugin", name)
	} else {
		registry[name] = PluginUtilities{
			PluginBuilder:      builderFunc,
			PluginType:         pluginType,
			PluginArgInstance:  exampleArg,
			PluginArgValidator: pluginArgValidator,
			PluginArgDefaulter: pluginArgDefaulter,
		}
	}
}
