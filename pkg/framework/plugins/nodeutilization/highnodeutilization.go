/*
Copyright 2021 The Kubernetes Authors.

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

package nodeutilization

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/pkg/api"
	nodeutil "sigs.k8s.io/descheduler/pkg/descheduler/node"
	"sigs.k8s.io/descheduler/pkg/framework"
)

const HighNodeUtilizationPluginName = "HighNodeUtilization"

// HighNodeUtilization evicts pods from under utilized nodes so that scheduler can schedule according to its strategy.
// Note that CPU/Memory requests are used to calculate nodes' utilization and not the actual resource usage.
type HighNodeUtilization struct {
	handle               framework.Handle
	args                 *framework.HighNodeUtilizationArgs
	resourceNames        []v1.ResourceName
	isEvictable          func(pod *v1.Pod) bool
	continueEvictionCond func(nodeInfo NodeInfo, totalAvailableUsage map[v1.ResourceName]*resource.Quantity) bool
}

var _ framework.Plugin = &HighNodeUtilization{}
var _ framework.BalancePlugin = &HighNodeUtilization{}

func NewHighNodeUtilization(args runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	utilizationArgs, ok := args.(*framework.HighNodeUtilizationArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type HighNodeUtilizationArgs, got %T", args)
	}

	if utilizationArgs.PriorityThreshold != nil && utilizationArgs.PriorityThreshold.Value != nil && utilizationArgs.PriorityThreshold.Name != "" {
		return nil, fmt.Errorf("only one of priorityThreshold fields can be set")
	}

	if err := validateHighUtilizationStrategyConfig(utilizationArgs.Thresholds, utilizationArgs.TargetThresholds); err != nil {
		return nil, fmt.Errorf("highNodeUtilization config is not valid: %v", err)
	}

	// TODO(jchaloup): set defaults before initializing the plugin?
	utilizationArgs.TargetThresholds = make(api.ResourceThresholds)
	setDefaultForThresholds(utilizationArgs.Thresholds, utilizationArgs.TargetThresholds)

	return &HighNodeUtilization{
		handle:        handle,
		args:          utilizationArgs,
		isEvictable:   handle.Evictor().Filter,
		resourceNames: getResourceNames(utilizationArgs.TargetThresholds),
		// stop if the total available usage has dropped to zero - no more pods can be scheduled
		continueEvictionCond: func(nodeInfo NodeInfo, totalAvailableUsage map[v1.ResourceName]*resource.Quantity) bool {
			for name := range totalAvailableUsage {
				if totalAvailableUsage[name].CmpInt64(0) < 1 {
					return false
				}
			}

			return true
		},
	}, nil
}

func (d *HighNodeUtilization) Name() string {
	return HighNodeUtilizationPluginName
}

func (d *HighNodeUtilization) Balance(ctx context.Context, nodes []*v1.Node) *framework.Status {
	sourceNodes, highNodes := classifyNodes(
		getNodeUsage(nodes, d.resourceNames, d.handle.GetPodsAssignedToNodeFunc()),
		getNodeThresholds(nodes, d.args.Thresholds, d.args.TargetThresholds, d.resourceNames, d.handle.GetPodsAssignedToNodeFunc(), false),
		func(node *v1.Node, usage NodeUsage, threshold NodeThresholds) bool {
			return isNodeWithLowUtilization(usage, threshold.lowResourceThreshold)
		},
		func(node *v1.Node, usage NodeUsage, threshold NodeThresholds) bool {
			if nodeutil.IsNodeUnschedulable(node) {
				klog.V(2).InfoS("Node is unschedulable", "node", klog.KObj(node))
				return false
			}
			return !isNodeWithLowUtilization(usage, threshold.lowResourceThreshold)
		})

	// log message in one line
	keysAndValues := []interface{}{
		"CPU", d.args.Thresholds[v1.ResourceCPU],
		"Mem", d.args.Thresholds[v1.ResourceMemory],
		"Pods", d.args.Thresholds[v1.ResourcePods],
	}
	for name := range d.args.Thresholds {
		if !isBasicResource(name) {
			keysAndValues = append(keysAndValues, string(name), int64(d.args.Thresholds[name]))
		}
	}

	klog.V(1).InfoS("Criteria for a node below target utilization", keysAndValues...)
	klog.V(1).InfoS("Number of underutilized nodes", "totalNumber", len(sourceNodes))

	if len(sourceNodes) == 0 {
		klog.V(1).InfoS("No node is underutilized, nothing to do here, you might tune your thresholds further")
		return nil
	}
	if len(sourceNodes) <= d.args.NumberOfNodes {
		klog.V(1).InfoS("Number of nodes underutilized is less or equal than NumberOfNodes, nothing to do here", "underutilizedNodes", len(sourceNodes), "numberOfNodes", d.args.NumberOfNodes)
		return nil
	}
	if len(sourceNodes) == len(nodes) {
		klog.V(1).InfoS("All nodes are underutilized, nothing to do here")
		return nil
	}
	if len(highNodes) == 0 {
		klog.V(1).InfoS("No node is available to schedule the pods, nothing to do here")
		return nil
	}

	// Sort the nodes by the usage in ascending order
	sortNodesByUsage(sourceNodes, true)

	evictPodsFromSourceNodes(
		ctx,
		sourceNodes,
		highNodes,
		d.handle.Evictor(),
		d.isEvictable,
		d.resourceNames,
		"HighNodeUtilization",
		d.continueEvictionCond)

	return nil
}

func validateHighUtilizationStrategyConfig(thresholds, targetThresholds api.ResourceThresholds) error {
	if targetThresholds != nil {
		return fmt.Errorf("targetThresholds is not applicable for HighNodeUtilization")
	}
	if err := validateThresholds(thresholds); err != nil {
		return fmt.Errorf("thresholds config is not valid: %v", err)
	}
	return nil
}

func setDefaultForThresholds(thresholds, targetThresholds api.ResourceThresholds) {
	// check if Pods/CPU/Mem are set, if not, set them to 100
	if _, ok := thresholds[v1.ResourcePods]; !ok {
		thresholds[v1.ResourcePods] = MaxResourcePercentage
	}
	if _, ok := thresholds[v1.ResourceCPU]; !ok {
		thresholds[v1.ResourceCPU] = MaxResourcePercentage
	}
	if _, ok := thresholds[v1.ResourceMemory]; !ok {
		thresholds[v1.ResourceMemory] = MaxResourcePercentage
	}

	// Default targetThreshold resource values to 100
	targetThresholds[v1.ResourcePods] = MaxResourcePercentage
	targetThresholds[v1.ResourceCPU] = MaxResourcePercentage
	targetThresholds[v1.ResourceMemory] = MaxResourcePercentage

	for name := range thresholds {
		if !isBasicResource(name) {
			targetThresholds[name] = MaxResourcePercentage
		}
	}
}
