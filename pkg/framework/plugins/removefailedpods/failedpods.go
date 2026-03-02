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

package removefailedpods

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"sigs.k8s.io/descheduler/pkg/framework/plugins/podlifetime"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
)

const PluginName = "RemoveFailedPods"

// RemoveFailedPods evicts pods in failed status phase that match the given args criteria.
// It delegates to PodLifeTime under the hood.
type RemoveFailedPods struct {
	delegate frameworktypes.DeschedulePlugin
}

var _ frameworktypes.DeschedulePlugin = &RemoveFailedPods{}

// New builds plugin from its arguments while passing a handle.
func New(ctx context.Context, args runtime.Object, handle frameworktypes.Handle) (frameworktypes.Plugin, error) {
	failedPodsArgs, ok := args.(*RemoveFailedPodsArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type RemoveFailedPodsArgs, got %T", args)
	}
	podLifeTimeArgs := toLifeTimeArgs(failedPodsArgs)
	plugin, err := podlifetime.NewAs(ctx, podLifeTimeArgs, handle, PluginName, func(pod *v1.Pod) bool {
		return pod.Status.Phase == v1.PodFailed
	})
	if err != nil {
		return nil, err
	}

	delegate, ok := plugin.(frameworktypes.DeschedulePlugin)
	if !ok {
		return nil, fmt.Errorf("expected DeschedulePlugin from PodLifeTime, got %T", plugin)
	}

	return &RemoveFailedPods{
		delegate: delegate,
	}, nil
}

// Name retrieves the plugin name.
func (d *RemoveFailedPods) Name() string {
	return PluginName
}

// Deschedule extension point implementation for the plugin.
func (d *RemoveFailedPods) Deschedule(ctx context.Context, nodes []*v1.Node) *frameworktypes.Status {
	return d.delegate.Deschedule(ctx, nodes)
}

func toLifeTimeArgs(args *RemoveFailedPodsArgs) *podlifetime.PodLifeTimeArgs {
	ta := &podlifetime.PodLifeTimeArgs{
		Namespaces:              args.Namespaces,
		LabelSelector:           args.LabelSelector,
		MaxPodLifeTimeSeconds:   args.MinPodLifetimeSeconds,
		States:                  args.Reasons,
		ExitCodes:               args.ExitCodes,
		IncludingInitContainers: args.IncludingInitContainers,
	}

	if len(args.ExcludeOwnerKinds) > 0 {
		ta.OwnerKinds = &podlifetime.OwnerKinds{
			Exclude: args.ExcludeOwnerKinds,
		}
	}

	return ta
}
