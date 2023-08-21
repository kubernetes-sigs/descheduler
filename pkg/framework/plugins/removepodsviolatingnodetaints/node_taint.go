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

package removepodsviolatingnodetaints

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
	"sigs.k8s.io/descheduler/pkg/utils"
)

const PluginName = "RemovePodsViolatingNodeTaints"

// RemovePodsViolatingNodeTaints evicts pods on the node which violate NoSchedule Taints on nodes
type RemovePodsViolatingNodeTaints struct {
	handle         frameworktypes.Handle
	args           *RemovePodsViolatingNodeTaintsArgs
	taintFilterFnc func(taint *v1.Taint) bool
	podFilter      podutil.FilterFunc
}

var _ frameworktypes.DeschedulePlugin = &RemovePodsViolatingNodeTaints{}

// New builds plugin from its arguments while passing a handle
func New(args runtime.Object, handle frameworktypes.Handle) (frameworktypes.Plugin, error) {
	nodeTaintsArgs, ok := args.(*RemovePodsViolatingNodeTaintsArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type RemovePodsViolatingNodeTaintsArgs, got %T", args)
	}

	var includedNamespaces, excludedNamespaces sets.Set[string]
	if nodeTaintsArgs.Namespaces != nil {
		includedNamespaces = sets.New(nodeTaintsArgs.Namespaces.Include...)
		excludedNamespaces = sets.New(nodeTaintsArgs.Namespaces.Exclude...)
	}

	// We can combine Filter and PreEvictionFilter since for this strategy it does not matter where we run PreEvictionFilter
	podFilter, err := podutil.NewOptions().
		WithFilter(podutil.WrapFilterFuncs(handle.Evictor().Filter, handle.Evictor().PreEvictionFilter)).
		WithNamespaces(includedNamespaces).
		WithoutNamespaces(excludedNamespaces).
		WithLabelSelector(nodeTaintsArgs.LabelSelector).
		BuildFilterFunc()
	if err != nil {
		return nil, fmt.Errorf("error initializing pod filter function: %v", err)
	}

	excludedTaints := sets.New(nodeTaintsArgs.ExcludedTaints...)
	excludeTaint := func(taint *v1.Taint) bool {
		// Exclude taints by key *or* key=value
		return excludedTaints.Has(taint.Key) || (taint.Value != "" && excludedTaints.Has(fmt.Sprintf("%s=%s", taint.Key, taint.Value)))
	}

	taintFilterFnc := func(taint *v1.Taint) bool { return (taint.Effect == v1.TaintEffectNoSchedule) && !excludeTaint(taint) }
	if nodeTaintsArgs.IncludePreferNoSchedule {
		taintFilterFnc = func(taint *v1.Taint) bool {
			return (taint.Effect == v1.TaintEffectNoSchedule || taint.Effect == v1.TaintEffectPreferNoSchedule) && !excludeTaint(taint)
		}
	}

	return &RemovePodsViolatingNodeTaints{
		handle:         handle,
		podFilter:      podFilter,
		args:           nodeTaintsArgs,
		taintFilterFnc: taintFilterFnc,
	}, nil
}

// Name retrieves the plugin name
func (d *RemovePodsViolatingNodeTaints) Name() string {
	return PluginName
}

// Deschedule extension point implementation for the plugin
func (d *RemovePodsViolatingNodeTaints) Deschedule(ctx context.Context, nodes []*v1.Node) *frameworktypes.Status {
	for _, node := range nodes {
		klog.V(1).InfoS("Processing node", "node", klog.KObj(node))
		pods, err := podutil.ListAllPodsOnANode(node.Name, d.handle.GetPodsAssignedToNodeFunc(), d.podFilter)
		if err != nil {
			// no pods evicted as error encountered retrieving evictable Pods
			return &frameworktypes.Status{
				Err: fmt.Errorf("error listing pods on a node: %v", err),
			}
		}
		totalPods := len(pods)
		for i := 0; i < totalPods; i++ {
			if !utils.TolerationsTolerateTaintsWithFilter(
				pods[i].Spec.Tolerations,
				node.Spec.Taints,
				d.taintFilterFnc,
			) {
				klog.V(2).InfoS("Not all taints with NoSchedule effect are tolerated after update for pod on node", "pod", klog.KObj(pods[i]), "node", klog.KObj(node))
				d.handle.Evictor().Evict(ctx, pods[i], evictions.EvictOptions{})
				if d.handle.Evictor().NodeLimitExceeded(node) {
					break
				}
			}
		}
	}

	return nil
}
