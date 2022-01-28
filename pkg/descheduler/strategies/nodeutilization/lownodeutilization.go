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
	"errors"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	nodeutil "sigs.k8s.io/descheduler/pkg/descheduler/node"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/utils"
)

type LowNodeUtilizationStrategy struct {
	strategy api.DeschedulerStrategy
	client   clientset.Interface
}

func NewLowNodeUtilizationStrategy(client clientset.Interface, strategyList api.StrategyList) (*LowNodeUtilizationStrategy, error) {
	s := &LowNodeUtilizationStrategy{}
	strategy, ok := strategyList[s.Name()]
	if !ok {
		return nil, errors.New("")
	}
	s.strategy = strategy

	return s, nil
}

func (s *LowNodeUtilizationStrategy) Name() api.StrategyName {
	return LowNodeUtilization
}

func (s *LowNodeUtilizationStrategy) Enabled() bool {
	return s.strategy.Enabled
}

func (s *LowNodeUtilizationStrategy) Validate() error {
	err := validateNodeUtilizationParams(s.strategy.Params)
	if err != nil {
		klog.ErrorS(err, "Invalid LowNodeUtilizationStrategy parameters")
		return err
	}
	return nil
}

// Run evicts pods from overutilized nodes to underutilized nodes. Note that CPU/Memory requests are used
// to calculate nodes' utilization and not the actual resource usage.
func (s *LowNodeUtilizationStrategy) Run(ctx context.Context, client clientset.Interface, strategy api.DeschedulerStrategy, nodes []*v1.Node, podEvictor *evictions.PodEvictor, getPodsAssignedToNode podutil.GetPodsAssignedToNodeFunc) {
	// TODO: May be create a struct for the strategy as well, so that we don't have to pass along the all the params?
	if err := validateNodeUtilizationParams(strategy.Params); err != nil {
		klog.ErrorS(err, "Invalid LowNodeUtilization parameters")
		return
	}
	thresholdPriority, err := utils.GetPriorityFromStrategyParams(ctx, client, strategy.Params)
	if err != nil {
		klog.ErrorS(err, "Failed to get threshold priority from strategy's params")
		return
	}

	nodeFit := false
	if strategy.Params != nil {
		nodeFit = strategy.Params.NodeFit
	}

	thresholds := strategy.Params.NodeResourceUtilizationThresholds.Thresholds
	targetThresholds := strategy.Params.NodeResourceUtilizationThresholds.TargetThresholds
	if err := validateLowUtilizationStrategyConfig(thresholds, targetThresholds); err != nil {
		klog.ErrorS(err, "LowNodeUtilization config is not valid")
		return
	}
	// check if Pods/CPU/Mem are set, if not, set them to 100
	if _, ok := thresholds[v1.ResourcePods]; !ok {
		thresholds[v1.ResourcePods] = MaxResourcePercentage
		targetThresholds[v1.ResourcePods] = MaxResourcePercentage
	}
	if _, ok := thresholds[v1.ResourceCPU]; !ok {
		thresholds[v1.ResourceCPU] = MaxResourcePercentage
		targetThresholds[v1.ResourceCPU] = MaxResourcePercentage
	}
	if _, ok := thresholds[v1.ResourceMemory]; !ok {
		thresholds[v1.ResourceMemory] = MaxResourcePercentage
		targetThresholds[v1.ResourceMemory] = MaxResourcePercentage
	}
	resourceNames := getResourceNames(thresholds)

	lowNodes, sourceNodes := classifyNodes(
		getNodeUsage(nodes, thresholds, targetThresholds, resourceNames, getPodsAssignedToNode),
		// The node has to be schedulable (to be able to move workload there)
		func(node *v1.Node, usage NodeUsage) bool {
			if nodeutil.IsNodeUnschedulable(node) {
				klog.V(2).InfoS("Node is unschedulable, thus not considered as underutilized", "node", klog.KObj(node))
				return false
			}
			return isNodeWithLowUtilization(usage)
		},
		func(node *v1.Node, usage NodeUsage) bool {
			return isNodeAboveTargetUtilization(usage)
		},
	)

	// log message in one line
	keysAndValues := []interface{}{
		"CPU", thresholds[v1.ResourceCPU],
		"Mem", thresholds[v1.ResourceMemory],
		"Pods", thresholds[v1.ResourcePods],
	}
	for name := range thresholds {
		if !isBasicResource(name) {
			keysAndValues = append(keysAndValues, string(name), int64(thresholds[name]))
		}
	}
	klog.V(1).InfoS("Criteria for a node under utilization", keysAndValues...)
	klog.V(1).InfoS("Number of underutilized nodes", "totalNumber", len(lowNodes))

	// log message in one line
	keysAndValues = []interface{}{
		"CPU", targetThresholds[v1.ResourceCPU],
		"Mem", targetThresholds[v1.ResourceMemory],
		"Pods", targetThresholds[v1.ResourcePods],
	}
	for name := range targetThresholds {
		if !isBasicResource(name) {
			keysAndValues = append(keysAndValues, string(name), int64(targetThresholds[name]))
		}
	}
	klog.V(1).InfoS("Criteria for a node above target utilization", keysAndValues...)
	klog.V(1).InfoS("Number of overutilized nodes", "totalNumber", len(sourceNodes))

	if len(lowNodes) == 0 {
		klog.V(1).InfoS("No node is underutilized, nothing to do here, you might tune your thresholds further")
		return
	}

	if len(lowNodes) <= strategy.Params.NodeResourceUtilizationThresholds.NumberOfNodes {
		klog.V(1).InfoS("Number of nodes underutilized is less or equal than NumberOfNodes, nothing to do here", "underutilizedNodes", len(lowNodes), "numberOfNodes", strategy.Params.NodeResourceUtilizationThresholds.NumberOfNodes)
		return
	}

	if len(lowNodes) == len(nodes) {
		klog.V(1).InfoS("All nodes are underutilized, nothing to do here")
		return
	}

	if len(sourceNodes) == 0 {
		klog.V(1).InfoS("All nodes are under target utilization, nothing to do here")
		return
	}

	evictable := podEvictor.Evictable(evictions.WithPriorityThreshold(thresholdPriority), evictions.WithNodeFit(nodeFit))

	// stop if node utilization drops below target threshold or any of required capacity (cpu, memory, pods) is moved
	continueEvictionCond := func(nodeUsage NodeUsage, totalAvailableUsage map[v1.ResourceName]*resource.Quantity) bool {
		if !isNodeAboveTargetUtilization(nodeUsage) {
			return false
		}
		for name := range totalAvailableUsage {
			if totalAvailableUsage[name].CmpInt64(0) < 1 {
				return false
			}
		}

		return true
	}

	evictPodsFromSourceNodes(
		ctx,
		sourceNodes,
		lowNodes,
		podEvictor,
		evictable.IsEvictable,
		resourceNames,
		"LowNodeUtilization",
		continueEvictionCond)

	klog.V(1).InfoS("Total number of pods evicted", "evictedPods", podEvictor.TotalEvicted())
}

// validateLowUtilizationStrategyConfig checks if the strategy's config is valid
func validateLowUtilizationStrategyConfig(thresholds, targetThresholds api.ResourceThresholds) error {
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
		} else if value > targetValue {
			return fmt.Errorf("thresholds' %v percentage is greater than targetThresholds'", resourceName)
		}
	}
	return nil
}
