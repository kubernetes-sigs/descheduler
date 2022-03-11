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
	"sort"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/utils"
)

// NodeUsage stores a node's info, pods on it, thresholds and its resource usage
type NodeUsage struct {
	node    *v1.Node
	usage   map[v1.ResourceName]*resource.Quantity
	allPods []*v1.Pod

	lowResourceThreshold  map[v1.ResourceName]*resource.Quantity
	highResourceThreshold map[v1.ResourceName]*resource.Quantity
}

type continueEvictionCond func(nodeUsage NodeUsage, totalAvailableUsage map[v1.ResourceName]*resource.Quantity) bool

// NodePodsMap is a set of (node, pods) pairs
type NodePodsMap map[*v1.Node][]*v1.Pod

const (
	// MinResourcePercentage is the minimum value of a resource's percentage
	MinResourcePercentage = 0
	// MaxResourcePercentage is the maximum value of a resource's percentage
	MaxResourcePercentage = 100
)

func validateNodeUtilizationParams(params *api.StrategyParameters) error {
	if params == nil || params.NodeResourceUtilizationThresholds == nil {
		return fmt.Errorf("NodeResourceUtilizationThresholds not set")
	}
	if params.ThresholdPriority != nil && params.ThresholdPriorityClassName != "" {
		return fmt.Errorf("only one of thresholdPriority and thresholdPriorityClassName can be set")
	}

	return nil
}

// validateThresholds checks if thresholds have valid resource name and resource percentage configured
func validateThresholds(thresholds api.ResourceThresholds) error {
	if thresholds == nil || len(thresholds) == 0 {
		return fmt.Errorf("no resource threshold is configured")
	}
	for name, percent := range thresholds {
		if percent < MinResourcePercentage || percent > MaxResourcePercentage {
			return fmt.Errorf("%v threshold not in [%v, %v] range", name, MinResourcePercentage, MaxResourcePercentage)
		}
	}
	return nil
}

func getNodeUsage(
	nodes []*v1.Node,
	lowThreshold, highThreshold api.ResourceThresholds,
	resourceNames []v1.ResourceName,
	getPodsAssignedToNode podutil.GetPodsAssignedToNodeFunc,
) []NodeUsage {
	var nodeUsageList []NodeUsage

	for _, node := range nodes {
		pods, err := podutil.ListPodsOnANode(node.Name, getPodsAssignedToNode, nil)
		if err != nil {
			klog.V(2).InfoS("Node will not be processed, error accessing its pods", "node", klog.KObj(node), "err", err)
			continue
		}

		nodeCapacity := node.Status.Capacity
		if len(node.Status.Allocatable) > 0 {
			nodeCapacity = node.Status.Allocatable
		}
		lowResourceThreshold := map[v1.ResourceName]*resource.Quantity{
			v1.ResourceCPU:    resourceThreshold(nodeCapacity, v1.ResourceCPU, lowThreshold[v1.ResourceCPU]),
			v1.ResourceMemory: resourceThreshold(nodeCapacity, v1.ResourceMemory, lowThreshold[v1.ResourceMemory]),
			v1.ResourcePods:   resourceThreshold(nodeCapacity, v1.ResourcePods, lowThreshold[v1.ResourcePods]),
		}
		for _, name := range resourceNames {
			if !isBasicResource(name) {
				lowResourceThreshold[name] = resourceThreshold(nodeCapacity, name, lowThreshold[name])
			}
		}
		highResourceThreshold := map[v1.ResourceName]*resource.Quantity{
			v1.ResourceCPU:    resourceThreshold(nodeCapacity, v1.ResourceCPU, highThreshold[v1.ResourceCPU]),
			v1.ResourceMemory: resourceThreshold(nodeCapacity, v1.ResourceMemory, highThreshold[v1.ResourceMemory]),
			v1.ResourcePods:   resourceThreshold(nodeCapacity, v1.ResourcePods, highThreshold[v1.ResourcePods]),
		}
		for _, name := range resourceNames {
			if !isBasicResource(name) {
				highResourceThreshold[name] = resourceThreshold(nodeCapacity, name, highThreshold[name])
			}
		}

		nodeUsageList = append(nodeUsageList, NodeUsage{
			node:                  node,
			usage:                 nodeUtilization(node, pods, resourceNames),
			allPods:               pods,
			lowResourceThreshold:  lowResourceThreshold,
			highResourceThreshold: highResourceThreshold,
		})
	}

	return nodeUsageList
}

func resourceThreshold(nodeCapacity v1.ResourceList, resourceName v1.ResourceName, threshold api.Percentage) *resource.Quantity {
	defaultFormat := resource.DecimalSI
	if resourceName == v1.ResourceMemory {
		defaultFormat = resource.BinarySI
	}

	resourceCapacityFraction := func(resourceNodeCapacity int64) int64 {
		// A threshold is in percentages but in <0;100> interval.
		// Performing `threshold * 0.01` will convert <0;100> interval into <0;1>.
		// Multiplying it with capacity will give fraction of the capacity corresponding to the given resource threshold in Quantity units.
		return int64(float64(threshold) * 0.01 * float64(resourceNodeCapacity))
	}

	resourceCapacityQuantity := nodeCapacity.Name(resourceName, defaultFormat)

	if resourceName == v1.ResourceCPU {
		return resource.NewMilliQuantity(resourceCapacityFraction(resourceCapacityQuantity.MilliValue()), defaultFormat)
	}
	return resource.NewQuantity(resourceCapacityFraction(resourceCapacityQuantity.Value()), defaultFormat)
}

func resourceUsagePercentages(nodeUsage NodeUsage) map[v1.ResourceName]float64 {
	nodeCapacity := nodeUsage.node.Status.Capacity
	if len(nodeUsage.node.Status.Allocatable) > 0 {
		nodeCapacity = nodeUsage.node.Status.Allocatable
	}

	resourceUsagePercentage := map[v1.ResourceName]float64{}
	for resourceName, resourceUsage := range nodeUsage.usage {
		cap := nodeCapacity[resourceName]
		if !cap.IsZero() {
			resourceUsagePercentage[resourceName] = 100 * float64(resourceUsage.MilliValue()) / float64(cap.MilliValue())
		}
	}

	return resourceUsagePercentage
}

// classifyNodes classifies the nodes into low-utilization or high-utilization nodes. If a node lies between
// low and high thresholds, it is simply ignored.
func classifyNodes(
	nodeUsages []NodeUsage,
	lowThresholdFilter, highThresholdFilter func(node *v1.Node, usage NodeUsage) bool,
) ([]NodeUsage, []NodeUsage) {
	lowNodes, highNodes := []NodeUsage{}, []NodeUsage{}

	for _, nodeUsage := range nodeUsages {
		if lowThresholdFilter(nodeUsage.node, nodeUsage) {
			klog.V(2).InfoS("Node is underutilized", "node", klog.KObj(nodeUsage.node), "usage", nodeUsage.usage, "usagePercentage", resourceUsagePercentages(nodeUsage))
			lowNodes = append(lowNodes, nodeUsage)
		} else if highThresholdFilter(nodeUsage.node, nodeUsage) {
			klog.V(2).InfoS("Node is overutilized", "node", klog.KObj(nodeUsage.node), "usage", nodeUsage.usage, "usagePercentage", resourceUsagePercentages(nodeUsage))
			highNodes = append(highNodes, nodeUsage)
		} else {
			klog.V(2).InfoS("Node is appropriately utilized", "node", klog.KObj(nodeUsage.node), "usage", nodeUsage.usage, "usagePercentage", resourceUsagePercentages(nodeUsage))
		}
	}

	return lowNodes, highNodes
}

// evictPodsFromSourceNodes evicts pods based on priority, if all the pods on the node have priority, if not
// evicts them based on QoS as fallback option.
// TODO: @ravig Break this function into smaller functions.
func evictPodsFromSourceNodes(
	ctx context.Context,
	sourceNodes, destinationNodes []NodeUsage,
	podEvictor *evictions.PodEvictor,
	podFilter func(pod *v1.Pod) bool,
	resourceNames []v1.ResourceName,
	strategy string,
	continueEviction continueEvictionCond,
) {

	sortNodesByUsage(sourceNodes)

	// upper bound on total number of pods/cpu/memory and optional extended resources to be moved
	totalAvailableUsage := map[v1.ResourceName]*resource.Quantity{
		v1.ResourcePods:   {},
		v1.ResourceCPU:    {},
		v1.ResourceMemory: {},
	}

	var taintsOfDestinationNodes = make(map[string][]v1.Taint, len(destinationNodes))
	for _, node := range destinationNodes {
		taintsOfDestinationNodes[node.node.Name] = node.node.Spec.Taints

		for _, name := range resourceNames {
			if _, ok := totalAvailableUsage[name]; !ok {
				totalAvailableUsage[name] = resource.NewQuantity(0, resource.DecimalSI)
			}
			totalAvailableUsage[name].Add(*node.highResourceThreshold[name])
			totalAvailableUsage[name].Sub(*node.usage[name])
		}
	}

	// log message in one line
	keysAndValues := []interface{}{
		"CPU", totalAvailableUsage[v1.ResourceCPU].MilliValue(),
		"Mem", totalAvailableUsage[v1.ResourceMemory].Value(),
		"Pods", totalAvailableUsage[v1.ResourcePods].Value(),
	}
	for name := range totalAvailableUsage {
		if !isBasicResource(name) {
			keysAndValues = append(keysAndValues, string(name), totalAvailableUsage[name].Value())
		}
	}
	klog.V(1).InfoS("Total capacity to be moved", keysAndValues...)

	for _, node := range sourceNodes {
		klog.V(3).InfoS("Evicting pods from node", "node", klog.KObj(node.node), "usage", node.usage)

		nonRemovablePods, removablePods := classifyPods(node.allPods, podFilter)
		klog.V(2).InfoS("Pods on node", "node", klog.KObj(node.node), "allPods", len(node.allPods), "nonRemovablePods", len(nonRemovablePods), "removablePods", len(removablePods))

		if len(removablePods) == 0 {
			klog.V(1).InfoS("No removable pods on node, try next node", "node", klog.KObj(node.node))
			continue
		}

		klog.V(1).InfoS("Evicting pods based on priority, if they have same priority, they'll be evicted based on QoS tiers")
		// sort the evictable Pods based on priority. This also sorts them based on QoS. If there are multiple pods with same priority, they are sorted based on QoS tiers.
		podutil.SortPodsBasedOnPriorityLowToHigh(removablePods)
		evictPods(ctx, removablePods, node, totalAvailableUsage, taintsOfDestinationNodes, podEvictor, strategy, continueEviction)
		klog.V(1).InfoS("Evicted pods from node", "node", klog.KObj(node.node), "evictedPods", podEvictor.NodeEvicted(node.node), "usage", node.usage)
	}
}

func evictPods(
	ctx context.Context,
	inputPods []*v1.Pod,
	nodeUsage NodeUsage,
	totalAvailableUsage map[v1.ResourceName]*resource.Quantity,
	taintsOfLowNodes map[string][]v1.Taint,
	podEvictor *evictions.PodEvictor,
	strategy string,
	continueEviction continueEvictionCond,
) {

	if continueEviction(nodeUsage, totalAvailableUsage) {
		for _, pod := range inputPods {
			if !utils.PodToleratesTaints(pod, taintsOfLowNodes) {
				klog.V(3).InfoS("Skipping eviction for pod, doesn't tolerate node taint", "pod", klog.KObj(pod))
				continue
			}

			success, err := podEvictor.EvictPod(ctx, pod, nodeUsage.node, strategy)
			if err != nil {
				klog.ErrorS(err, "Error evicting pod", "pod", klog.KObj(pod))
				break
			}

			if success {
				klog.V(3).InfoS("Evicted pods", "pod", klog.KObj(pod), "err", err)

				for name := range totalAvailableUsage {
					if name == v1.ResourcePods {
						nodeUsage.usage[name].Sub(*resource.NewQuantity(1, resource.DecimalSI))
						totalAvailableUsage[name].Sub(*resource.NewQuantity(1, resource.DecimalSI))
					} else {
						quantity := utils.GetResourceRequestQuantity(pod, name)
						nodeUsage.usage[name].Sub(quantity)
						totalAvailableUsage[name].Sub(quantity)
					}
				}

				keysAndValues := []interface{}{
					"node", nodeUsage.node.Name,
					"CPU", nodeUsage.usage[v1.ResourceCPU].MilliValue(),
					"Mem", nodeUsage.usage[v1.ResourceMemory].Value(),
					"Pods", nodeUsage.usage[v1.ResourcePods].Value(),
				}
				for name := range totalAvailableUsage {
					if !isBasicResource(name) {
						keysAndValues = append(keysAndValues, string(name), totalAvailableUsage[name].Value())
					}
				}

				klog.V(3).InfoS("Updated node usage", keysAndValues...)
				// check if pods can be still evicted
				if !continueEviction(nodeUsage, totalAvailableUsage) {
					break
				}
			}
		}
	}
}

// sortNodesByUsage sorts nodes based on usage in descending order
func sortNodesByUsage(nodes []NodeUsage) {
	sort.Slice(nodes, func(i, j int) bool {
		ti := nodes[i].usage[v1.ResourceMemory].Value() + nodes[i].usage[v1.ResourceCPU].MilliValue() + nodes[i].usage[v1.ResourcePods].Value()
		tj := nodes[j].usage[v1.ResourceMemory].Value() + nodes[j].usage[v1.ResourceCPU].MilliValue() + nodes[j].usage[v1.ResourcePods].Value()

		// extended resources
		for name := range nodes[i].usage {
			if !isBasicResource(name) {
				ti = ti + nodes[i].usage[name].Value()
				tj = tj + nodes[j].usage[name].Value()
			}
		}

		// To return sorted in descending order
		return ti > tj
	})
}

// isNodeAboveTargetUtilization checks if a node is overutilized
// At least one resource has to be above the high threshold
func isNodeAboveTargetUtilization(usage NodeUsage) bool {
	for name, nodeValue := range usage.usage {
		// usage.highResourceThreshold[name] < nodeValue
		if usage.highResourceThreshold[name].Cmp(*nodeValue) == -1 {
			return true
		}
	}
	return false
}

// isNodeWithLowUtilization checks if a node is underutilized
// All resources have to be below the low threshold
func isNodeWithLowUtilization(usage NodeUsage) bool {
	for name, nodeValue := range usage.usage {
		// usage.lowResourceThreshold[name] < nodeValue
		if usage.lowResourceThreshold[name].Cmp(*nodeValue) == -1 {
			return false
		}
	}

	return true
}

// getResourceNames returns list of resource names in resource thresholds
func getResourceNames(thresholds api.ResourceThresholds) []v1.ResourceName {
	resourceNames := make([]v1.ResourceName, 0, len(thresholds))
	for name := range thresholds {
		resourceNames = append(resourceNames, name)
	}
	return resourceNames
}

// isBasicResource checks if resource is basic native.
func isBasicResource(name v1.ResourceName) bool {
	switch name {
	case v1.ResourceCPU, v1.ResourceMemory, v1.ResourcePods:
		return true
	default:
		return false
	}
}

func nodeUtilization(node *v1.Node, pods []*v1.Pod, resourceNames []v1.ResourceName) map[v1.ResourceName]*resource.Quantity {
	totalReqs := map[v1.ResourceName]*resource.Quantity{
		v1.ResourceCPU:    resource.NewMilliQuantity(0, resource.DecimalSI),
		v1.ResourceMemory: resource.NewQuantity(0, resource.BinarySI),
		v1.ResourcePods:   resource.NewQuantity(int64(len(pods)), resource.DecimalSI),
	}
	for _, name := range resourceNames {
		if !isBasicResource(name) {
			totalReqs[name] = resource.NewQuantity(0, resource.DecimalSI)
		}
	}

	for _, pod := range pods {
		req, _ := utils.PodRequestsAndLimits(pod)
		for _, name := range resourceNames {
			quantity, ok := req[name]
			if ok && name != v1.ResourcePods {
				// As Quantity.Add says: Add adds the provided y quantity to the current value. If the current value is zero,
				// the format of the quantity will be updated to the format of y.
				totalReqs[name].Add(quantity)
			}
		}
	}

	return totalReqs
}

func classifyPods(pods []*v1.Pod, filter func(pod *v1.Pod) bool) ([]*v1.Pod, []*v1.Pod) {
	var nonRemovablePods, removablePods []*v1.Pod

	for _, pod := range pods {
		if !filter(pod) {
			nonRemovablePods = append(nonRemovablePods, pod)
		} else {
			removablePods = append(removablePods, pod)
		}
	}

	return nonRemovablePods, removablePods
}
