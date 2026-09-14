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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	logger         klog.Logger
	handle         frameworktypes.Handle
	args           *RemovePodsViolatingNodeTaintsArgs
	taintFilterFnc func(taint *v1.Taint) bool
	podFilter      podutil.FilterFunc
}

var _ frameworktypes.DeschedulePlugin = &RemovePodsViolatingNodeTaints{}

// New builds plugin from its arguments while passing a handle
func New(ctx context.Context, args runtime.Object, handle frameworktypes.Handle) (frameworktypes.Plugin, error) {
	nodeTaintsArgs, ok := args.(*RemovePodsViolatingNodeTaintsArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type RemovePodsViolatingNodeTaintsArgs, got %T", args)
	}
	logger := klog.FromContext(ctx).WithValues("plugin", PluginName)

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

	includedTaints := sets.New(nodeTaintsArgs.IncludedTaints...)
	includeTaint := func(taint *v1.Taint) bool {
		// Include only taints by key *or* key=value
		// Always returns true if no includedTaints argument is provided
		return (nodeTaintsArgs.IncludedTaints == nil) || includedTaints.Has(taint.Key) || (taint.Value != "" && includedTaints.Has(fmt.Sprintf("%s=%s", taint.Key, taint.Value)))
	}

	excludedTaints := sets.New(nodeTaintsArgs.ExcludedTaints...)
	excludeTaint := func(taint *v1.Taint) bool {
		// Exclude taints by key *or* key=value
		return excludedTaints.Has(taint.Key) || (taint.Value != "" && excludedTaints.Has(fmt.Sprintf("%s=%s", taint.Key, taint.Value)))
	}

	taintFilterFnc := func(taint *v1.Taint) bool {
		return (taint.Effect == v1.TaintEffectNoSchedule) && !excludeTaint(taint) && includeTaint(taint)
	}
	if nodeTaintsArgs.IncludePreferNoSchedule {
		taintFilterFnc = func(taint *v1.Taint) bool {
			return (taint.Effect == v1.TaintEffectNoSchedule || taint.Effect == v1.TaintEffectPreferNoSchedule) && !excludeTaint(taint) && includeTaint(taint)
		}
	}

	return &RemovePodsViolatingNodeTaints{
		logger:         logger,
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
	ctx = klog.NewContext(ctx, d.logger)
	logger := klog.FromContext(ctx).WithValues("ExtensionPoint", frameworktypes.DescheduleExtensionPoint)
	for _, node := range nodes {
		logger.V(1).Info("Processing node", "node", klog.KObj(node))

		pods, err := podutil.ListPodsOnANode(node.Name, d.handle.GetPodsAssignedToNodeFunc(), d.podFilter)
		if err != nil {
			// no pods evicted as error encountered retrieving evictable Pods
			return &frameworktypes.Status{
				Err: fmt.Errorf("error listing pods on a node: %v", err),
			}
		}
		totalPods := len(pods)

		// Cordon the node right before the first eviction, matching the kubectl
		// drain behavior. The node is only cordoned when it carries a taint this
		// plugin is configured to react to and at least one pod on it is going to
		// be evicted, so nodes without a matching taint or without violating pods
		// are left untouched.
		shouldCordon := d.args.Cordon && nodeHasMatchingTaint(node, d.taintFilterFnc)
		cordoned := false
	loop:
		for i := 0; i < totalPods; i++ {
			if !utils.TolerationsTolerateTaintsWithFilter(ctx, pods[i].Spec.Tolerations, node.Spec.Taints, d.taintFilterFnc) {
				logger.V(2).Info("Not all taints with NoSchedule effect are tolerated after update for pod on node", "pod", klog.KObj(pods[i]), "node", klog.KObj(node))
				if shouldCordon && !cordoned {
					if err := d.cordonNode(ctx, node); err != nil {
						logger.Error(err, "Error cordoning node", "node", klog.KObj(node))
						return &frameworktypes.Status{
							Err: fmt.Errorf("error cordoning node %v: %v", node.Name, err),
						}
					}
					cordoned = true
				}
				err := d.handle.Evictor().Evict(ctx, pods[i], evictions.EvictOptions{StrategyName: PluginName})
				if err == nil {
					continue
				}
				switch err.(type) {
				case *evictions.EvictionNodeLimitError:
					break loop
				case *evictions.EvictionTotalLimitError:
					return nil
				default:
					logger.Error(err, "eviction failed")
				}
			}
		}
	}

	return nil
}

// nodeHasMatchingTaint returns true when the node carries at least one taint
// that matches the plugin's taint filter.
func nodeHasMatchingTaint(node *v1.Node, taintFilterFnc func(taint *v1.Taint) bool) bool {
	for i := range node.Spec.Taints {
		if taintFilterFnc(&node.Spec.Taints[i]) {
			return true
		}
	}
	return false
}

// cordonNode marks the given node as unschedulable so that the scheduler no
// longer places new pods on it. It is a no-op when the node is already cordoned.
func (d *RemovePodsViolatingNodeTaints) cordonNode(ctx context.Context, node *v1.Node) error {
	if node.Spec.Unschedulable {
		return nil
	}
	nodeCopy := node.DeepCopy()
	nodeCopy.Spec.Unschedulable = true
	_, err := d.handle.ClientSet().CoreV1().Nodes().Update(ctx, nodeCopy, metav1.UpdateOptions{})
	return err
}
