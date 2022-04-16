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

const LowNodeUtilizationPluginName = "LowNodeUtilization"

// LowNodeUtilization evicts pods from overutilized nodes to underutilized nodes. Note that CPU/Memory requests are used
// to calculate nodes' utilization and not the actual resource usage.
type LowNodeUtilization struct {
	handle               framework.Handle
	args                 *framework.LowNodeUtilizationArgs
	resourceNames        []v1.ResourceName
	continueEvictionCond func(nodeInfo NodeInfo, totalAvailableUsage map[v1.ResourceName]*resource.Quantity) bool
}

var _ framework.Plugin = &LowNodeUtilization{}
var _ framework.BalancePlugin = &LowNodeUtilization{}

func NewLowNodeUtilization(args runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	utilizationArgs, ok := args.(*framework.LowNodeUtilizationArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type LowNodeUtilizationArgs, got %T", args)
	}

	if utilizationArgs.PriorityThreshold != nil && utilizationArgs.PriorityThreshold.Value != nil && utilizationArgs.PriorityThreshold.Name != "" {
		return nil, fmt.Errorf("only one of priorityThreshold fields can be set")
	}

	if err := validateLowUtilizationStrategyConfig(utilizationArgs.Thresholds, utilizationArgs.TargetThresholds, utilizationArgs.UseDeviationThresholds); err != nil {
		return nil, fmt.Errorf("lowNodeUtilization config is not valid: %v", err)
	}

	// check if Pods/CPU/Mem are set, if not, set them to 100
	if _, ok := utilizationArgs.Thresholds[v1.ResourcePods]; !ok {
		if utilizationArgs.UseDeviationThresholds {
			utilizationArgs.Thresholds[v1.ResourcePods] = MinResourcePercentage
			utilizationArgs.TargetThresholds[v1.ResourcePods] = MinResourcePercentage
		} else {
			utilizationArgs.Thresholds[v1.ResourcePods] = MaxResourcePercentage
			utilizationArgs.TargetThresholds[v1.ResourcePods] = MaxResourcePercentage
		}
	}
	if _, ok := utilizationArgs.Thresholds[v1.ResourceCPU]; !ok {
		if utilizationArgs.UseDeviationThresholds {
			utilizationArgs.Thresholds[v1.ResourceCPU] = MinResourcePercentage
			utilizationArgs.TargetThresholds[v1.ResourceCPU] = MinResourcePercentage
		} else {
			utilizationArgs.Thresholds[v1.ResourceCPU] = MaxResourcePercentage
			utilizationArgs.TargetThresholds[v1.ResourceCPU] = MaxResourcePercentage
		}
	}
	if _, ok := utilizationArgs.Thresholds[v1.ResourceMemory]; !ok {
		if utilizationArgs.UseDeviationThresholds {
			utilizationArgs.Thresholds[v1.ResourceMemory] = MinResourcePercentage
			utilizationArgs.TargetThresholds[v1.ResourceMemory] = MinResourcePercentage
		} else {
			utilizationArgs.Thresholds[v1.ResourceMemory] = MaxResourcePercentage
			utilizationArgs.TargetThresholds[v1.ResourceMemory] = MaxResourcePercentage
		}
	}

	return &LowNodeUtilization{
		handle:        handle,
		args:          utilizationArgs,
		resourceNames: getResourceNames(utilizationArgs.Thresholds),
		// stop if node utilization drops below target threshold or any of required capacity (cpu, memory, pods) is moved
		continueEvictionCond: func(nodeInfo NodeInfo, totalAvailableUsage map[v1.ResourceName]*resource.Quantity) bool {
			if !isNodeAboveTargetUtilization(nodeInfo.NodeUsage, nodeInfo.thresholds.highResourceThreshold) {
				return false
			}
			for name := range totalAvailableUsage {
				if totalAvailableUsage[name].CmpInt64(0) < 1 {
					return false
				}
			}

			return true
		},
	}, nil
}

func (d *LowNodeUtilization) Name() string {
	return LowNodeUtilizationPluginName
}

func (d *LowNodeUtilization) Balance(ctx context.Context, nodes []*v1.Node) *framework.Status {
	lowNodes, sourceNodes := classifyNodes(
		getNodeUsage(nodes, d.resourceNames, d.handle.GetPodsAssignedToNodeFunc()),
		getNodeThresholds(nodes, d.args.Thresholds, d.args.TargetThresholds, d.resourceNames, d.handle.GetPodsAssignedToNodeFunc(), d.args.UseDeviationThresholds),
		// The node has to be schedulable (to be able to move workload there)
		func(node *v1.Node, usage NodeUsage, threshold NodeThresholds) bool {
			if nodeutil.IsNodeUnschedulable(node) {
				klog.V(2).InfoS("Node is unschedulable, thus not considered as underutilized", "node", klog.KObj(node))
				return false
			}
			return isNodeWithLowUtilization(usage, threshold.lowResourceThreshold)
		},
		func(node *v1.Node, usage NodeUsage, threshold NodeThresholds) bool {
			return isNodeAboveTargetUtilization(usage, threshold.highResourceThreshold)
		},
	)

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
	klog.V(1).InfoS("Criteria for a node under utilization", keysAndValues...)
	klog.V(1).InfoS("Number of underutilized nodes", "totalNumber", len(lowNodes))

	// log message in one line
	keysAndValues = []interface{}{
		"CPU", d.args.TargetThresholds[v1.ResourceCPU],
		"Mem", d.args.TargetThresholds[v1.ResourceMemory],
		"Pods", d.args.TargetThresholds[v1.ResourcePods],
	}
	for name := range d.args.TargetThresholds {
		if !isBasicResource(name) {
			keysAndValues = append(keysAndValues, string(name), int64(d.args.TargetThresholds[name]))
		}
	}
	klog.V(1).InfoS("Criteria for a node above target utilization", keysAndValues...)
	klog.V(1).InfoS("Number of overutilized nodes", "totalNumber", len(sourceNodes))

	if len(lowNodes) == 0 {
		klog.V(1).InfoS("no node is underutilized, nothing to do here, you might tune your thresholds further")
		return nil
	}

	if len(lowNodes) <= d.args.NumberOfNodes {
		klog.V(1).InfoS("Number of nodes underutilized is less or equal than NumberOfNodes, nothing to do here", "underutilizedNodes", len(lowNodes), "numberOfNodes", d.args.NumberOfNodes)
		return nil
	}

	if len(lowNodes) == len(nodes) {
		klog.V(1).InfoS("All nodes are underutilized, nothing to do here")
		return nil
	}

	if len(sourceNodes) == 0 {
		klog.V(1).InfoS("All nodes are under target utilization, nothing to do here")
		return nil
	}

	// Sort the nodes by the usage in descending order
	sortNodesByUsage(sourceNodes, false)

	evictPodsFromSourceNodes(
		ctx,
		sourceNodes,
		lowNodes,
		d.handle.Evictor(),
		d.handle.Evictor().Filter,
		d.resourceNames,
		"LowNodeUtilization",
		d.continueEvictionCond)

	return nil
}

// validateLowUtilizationStrategyConfig checks if the strategy's config is valid
func validateLowUtilizationStrategyConfig(thresholds, targetThresholds api.ResourceThresholds, useDeviationThresholds bool) error {
	// validate thresholds and targetThresholds config
	if err := validateThresholds(thresholds); err != nil {
		return fmt.Errorf("thresholds config is not valid: %v", err)
	}
	if err := validateThresholds(targetThresholds); err != nil {
		return fmt.Errorf("targetThresholds config is not valid: %v", err)
	}

	// validate if thresholds and targetThresholds have same resources configured
	if len(thresholds) != len(targetThresholds) {
		return fmt.Errorf("thresholds and targetThresholds configured different resources")
	}
	for resourceName, value := range thresholds {
		if targetValue, ok := targetThresholds[resourceName]; !ok {
			return fmt.Errorf("thresholds and targetThresholds configured different resources")
		} else if value > targetValue && !useDeviationThresholds {
			return fmt.Errorf("thresholds' %v percentage is greater than targetThresholds'", resourceName)
		}
	}
	return nil
}
