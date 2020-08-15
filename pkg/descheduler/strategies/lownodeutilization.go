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

package strategies

import (
	"context"
	"fmt"
	"sort"

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

// NodeUsageMap stores a node's info, pods on it and its resource usage
type NodeUsageMap struct {
	node    *v1.Node
	usage   api.ResourceThresholds
	allPods []*v1.Pod
}

// NodePodsMap is a set of (node, pods) pairs
type NodePodsMap map[*v1.Node][]*v1.Pod

const (
	// MinResourcePercentage is the minimum value of a resource's percentage
	MinResourcePercentage = 0
	// MaxResourcePercentage is the maximum value of a resource's percentage
	MaxResourcePercentage = 100
)

func validateLowNodeUtilizationParams(params *api.StrategyParameters) error {
	if params == nil || params.NodeResourceUtilizationThresholds == nil {
		return fmt.Errorf("NodeResourceUtilizationThresholds not set")
	}
	if params.ThresholdPriority != nil && params.ThresholdPriorityClassName != "" {
		return fmt.Errorf("only one of thresholdPriority and thresholdPriorityClassName can be set")
	}

	return nil
}

// LowNodeUtilization evicts pods from overutilized nodes to underutilized nodes. Note that CPU/Memory requests are used
// to calculate nodes' utilization and not the actual resource usage.
func LowNodeUtilization(ctx context.Context, client clientset.Interface, strategy api.DeschedulerStrategy, nodes []*v1.Node, podEvictor *evictions.PodEvictor) {
	// TODO: May be create a struct for the strategy as well, so that we don't have to pass along the all the params?
	if err := validateLowNodeUtilizationParams(strategy.Params); err != nil {
		klog.V(1).Info(err)
		return
	}
	thresholdPriority, err := utils.GetPriorityFromStrategyParams(ctx, client, strategy.Params)
	if err != nil {
		klog.V(1).InfoS("Failed to get threshold priority from strategy's params", "err", err)
		return
	}

	thresholds := strategy.Params.NodeResourceUtilizationThresholds.Thresholds
	targetThresholds := strategy.Params.NodeResourceUtilizationThresholds.TargetThresholds
	if err := validateStrategyConfig(thresholds, targetThresholds); err != nil {
		klog.Errorf("LowNodeUtilization config is not valid: %v", err)
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

	npm := createNodePodsMap(ctx, client, nodes)
	lowNodes, targetNodes := classifyNodes(npm, thresholds, targetThresholds)

	klog.V(1).Infof("Criteria for a node under utilization: CPU: %v, Mem: %v, Pods: %v",
		thresholds[v1.ResourceCPU], thresholds[v1.ResourceMemory], thresholds[v1.ResourcePods])

	if len(lowNodes) == 0 {
		klog.V(1).Infof("No node is underutilized, nothing to do here, you might tune your thresholds further")
		return
	}
	klog.V(1).Infof("Total number of underutilized nodes: %v", len(lowNodes))

	if len(lowNodes) < strategy.Params.NodeResourceUtilizationThresholds.NumberOfNodes {
		klog.V(1).Infof("number of nodes underutilized (%v) is less than NumberOfNodes (%v), nothing to do here", len(lowNodes), strategy.Params.NodeResourceUtilizationThresholds.NumberOfNodes)
		return
	}

	if len(lowNodes) == len(nodes) {
		klog.V(1).Infof("All nodes are underutilized, nothing to do here")
		return
	}

	if len(targetNodes) == 0 {
		klog.V(1).Infof("All nodes are under target utilization, nothing to do here")
		return
	}

	klog.V(1).Infof("Criteria for a node above target utilization: CPU: %v, Mem: %v, Pods: %v",
		targetThresholds[v1.ResourceCPU], targetThresholds[v1.ResourceMemory], targetThresholds[v1.ResourcePods])
	klog.V(1).Infof("Total number of nodes above target utilization: %v", len(targetNodes))

	evictable := podEvictor.Evictable(evictions.WithPriorityThreshold(thresholdPriority))

	evictPodsFromTargetNodes(
		ctx,
		targetNodes,
		lowNodes,
		targetThresholds,
		podEvictor,
		evictable.IsEvictable)

	klog.V(1).InfoS("Total number of pods evicted", "evictedPods", podEvictor.TotalEvicted())
}

// validateStrategyConfig checks if the strategy's config is valid
func validateStrategyConfig(thresholds, targetThresholds api.ResourceThresholds) error {
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

// validateThresholds checks if thresholds have valid resource name and resource percentage configured
func validateThresholds(thresholds api.ResourceThresholds) error {
	if thresholds == nil || len(thresholds) == 0 {
		return fmt.Errorf("no resource threshold is configured")
	}
	for name, percent := range thresholds {
		switch name {
		case v1.ResourceCPU, v1.ResourceMemory, v1.ResourcePods:
			if percent < MinResourcePercentage || percent > MaxResourcePercentage {
				return fmt.Errorf("%v threshold not in [%v, %v] range", name, MinResourcePercentage, MaxResourcePercentage)
			}
		default:
			return fmt.Errorf("only cpu, memory, or pods thresholds can be specified")
		}
	}
	return nil
}

// classifyNodes classifies the nodes into low-utilization or high-utilization nodes. If a node lies between
// low and high thresholds, it is simply ignored.
func classifyNodes(npm NodePodsMap, thresholds api.ResourceThresholds, targetThresholds api.ResourceThresholds) ([]NodeUsageMap, []NodeUsageMap) {
	lowNodes, targetNodes := []NodeUsageMap{}, []NodeUsageMap{}
	for node, pods := range npm {
		usage := nodeUtilization(node, pods)
		nuMap := NodeUsageMap{
			node:    node,
			usage:   usage,
			allPods: pods,
		}
		// Check if node is underutilized and if we can schedule pods on it.
		if !nodeutil.IsNodeUnschedulable(node) && isNodeWithLowUtilization(usage, thresholds) {
			klog.V(2).InfoS("Node is underutilized", "node", klog.KObj(node), "usage", usage)
			lowNodes = append(lowNodes, nuMap)
		} else if isNodeAboveTargetUtilization(usage, targetThresholds) {
			klog.V(2).InfoS("Node is overutilized", "node", klog.KObj(node), "usage", usage)
			targetNodes = append(targetNodes, nuMap)
		} else {
			klog.V(2).InfoS("Node is appropriately utilized", "node", klog.KObj(node), "usage", usage)

		}
	}
	return lowNodes, targetNodes
}

// evictPodsFromTargetNodes evicts pods based on priority, if all the pods on the node have priority, if not
// evicts them based on QoS as fallback option.
// TODO: @ravig Break this function into smaller functions.
func evictPodsFromTargetNodes(
	ctx context.Context,
	targetNodes, lowNodes []NodeUsageMap,
	targetThresholds api.ResourceThresholds,
	podEvictor *evictions.PodEvictor,
	podFilter func(pod *v1.Pod) bool,
) {

	sortNodesByUsage(targetNodes)

	// upper bound on total number of pods/cpu/memory to be moved
	var totalPods, totalCPU, totalMem float64
	var taintsOfLowNodes = make(map[string][]v1.Taint, len(lowNodes))
	for _, node := range lowNodes {
		taintsOfLowNodes[node.node.Name] = node.node.Spec.Taints
		nodeCapacity := node.node.Status.Capacity
		if len(node.node.Status.Allocatable) > 0 {
			nodeCapacity = node.node.Status.Allocatable
		}
		// totalPods to be moved
		podsPercentage := targetThresholds[v1.ResourcePods] - node.usage[v1.ResourcePods]
		totalPods += ((float64(podsPercentage) * float64(nodeCapacity.Pods().Value())) / 100)

		// totalCPU capacity to be moved
		cpuPercentage := targetThresholds[v1.ResourceCPU] - node.usage[v1.ResourceCPU]
		totalCPU += ((float64(cpuPercentage) * float64(nodeCapacity.Cpu().MilliValue())) / 100)

		// totalMem capacity to be moved
		memPercentage := targetThresholds[v1.ResourceMemory] - node.usage[v1.ResourceMemory]
		totalMem += ((float64(memPercentage) * float64(nodeCapacity.Memory().Value())) / 100)
	}

	klog.V(1).InfoS("Total capacity to be moved", "CPU", totalCPU, "Mem", totalMem, "Pods", totalPods)

	for _, node := range targetNodes {
		nodeCapacity := node.node.Status.Capacity
		if len(node.node.Status.Allocatable) > 0 {
			nodeCapacity = node.node.Status.Allocatable
		}
		klog.V(3).InfoS("Evicting pods from node", "node", klog.KObj(node.node), "usage", node.usage)

		nonRemovablePods, removablePods := classifyPods(node.allPods, podFilter)
		klog.V(2).Infof("AllPods:%v, nonRemovablePods:%v, removablePods:%v", len(node.allPods), len(nonRemovablePods), len(removablePods))

		if len(removablePods) == 0 {
			klog.V(1).InfoS("No removable pods on node, try next node", "node", klog.KObj(node.node))
			continue
		}

		klog.V(1).Infof("evicting pods based on priority, if they have same priority, they'll be evicted based on QoS tiers")
		// sort the evictable Pods based on priority. This also sorts them based on QoS. If there are multiple pods with same priority, they are sorted based on QoS tiers.
		podutil.SortPodsBasedOnPriorityLowToHigh(removablePods)
		evictPods(ctx, removablePods, targetThresholds, nodeCapacity, node.usage, &totalPods, &totalCPU, &totalMem, taintsOfLowNodes, podEvictor, node.node)
		klog.V(1).InfoS("Evicted pods from node", "node", klog.KObj(node.node), "evictedPods", podEvictor.NodeEvicted(node.node), "usage", node.usage)
	}
}

func evictPods(
	ctx context.Context,
	inputPods []*v1.Pod,
	targetThresholds api.ResourceThresholds,
	nodeCapacity v1.ResourceList,
	nodeUsage api.ResourceThresholds,
	totalPods *float64,
	totalCPU *float64,
	totalMem *float64,
	taintsOfLowNodes map[string][]v1.Taint,
	podEvictor *evictions.PodEvictor,
	node *v1.Node) {
	// stop if node utilization drops below target threshold or any of required capacity (cpu, memory, pods) is moved
	if isNodeAboveTargetUtilization(nodeUsage, targetThresholds) && *totalPods > 0 && *totalCPU > 0 && *totalMem > 0 {
		onePodPercentage := api.Percentage((float64(1) * 100) / float64(nodeCapacity.Pods().Value()))
		for _, pod := range inputPods {
			if !utils.PodToleratesTaints(pod, taintsOfLowNodes) {
				klog.V(3).InfoS("Skipping eviction for pod, doesn't tolerate node taint", "pod", klog.KObj(pod))

				continue
			}

			cUsage := utils.GetResourceRequest(pod, v1.ResourceCPU)
			mUsage := utils.GetResourceRequest(pod, v1.ResourceMemory)

			success, err := podEvictor.EvictPod(ctx, pod, node, "LowNodeUtilization")
			if err != nil {
				klog.ErrorS(err, "Error evicting pod", "pod", klog.KObj(pod))
				break
			}

			if success {
				klog.V(3).InfoS("Evicted pods", "pod", klog.KObj(pod), "err", err)

				// update remaining pods
				nodeUsage[v1.ResourcePods] -= onePodPercentage
				*totalPods--

				// update remaining cpu
				*totalCPU -= float64(cUsage)
				nodeUsage[v1.ResourceCPU] -= api.Percentage((float64(cUsage) * 100) / float64(nodeCapacity.Cpu().MilliValue()))

				// update remaining memory
				*totalMem -= float64(mUsage)
				nodeUsage[v1.ResourceMemory] -= api.Percentage(float64(mUsage) / float64(nodeCapacity.Memory().Value()) * 100)

				klog.V(3).InfoS("Updated node usage", "updatedUsage", nodeUsage)
				// check if node utilization drops below target threshold or any required capacity (cpu, memory, pods) is moved
				if !isNodeAboveTargetUtilization(nodeUsage, targetThresholds) || *totalPods <= 0 || *totalCPU <= 0 || *totalMem <= 0 {
					break
				}
			}
		}
	}
}

// sortNodesByUsage sorts nodes based on usage in descending order
func sortNodesByUsage(nodes []NodeUsageMap) {
	sort.Slice(nodes, func(i, j int) bool {
		var ti, tj api.Percentage
		for name, value := range nodes[i].usage {
			if name == v1.ResourceCPU || name == v1.ResourceMemory || name == v1.ResourcePods {
				ti += value
			}
		}
		for name, value := range nodes[j].usage {
			if name == v1.ResourceCPU || name == v1.ResourceMemory || name == v1.ResourcePods {
				tj += value
			}
		}
		// To return sorted in descending order
		return ti > tj
	})
}

// createNodePodsMap returns nodepodsmap with evictable pods on node.
func createNodePodsMap(ctx context.Context, client clientset.Interface, nodes []*v1.Node) NodePodsMap {
	npm := NodePodsMap{}
	for _, node := range nodes {
		pods, err := podutil.ListPodsOnANode(ctx, client, node)
		if err != nil {
			klog.Warningf("node %s will not be processed, error in accessing its pods (%#v)", node.Name, err)
		} else {
			npm[node] = pods
		}
	}
	return npm
}

// isNodeAboveTargetUtilization checks if a node is overutilized
func isNodeAboveTargetUtilization(nodeThresholds api.ResourceThresholds, thresholds api.ResourceThresholds) bool {
	for name, nodeValue := range nodeThresholds {
		if name == v1.ResourceCPU || name == v1.ResourceMemory || name == v1.ResourcePods {
			if value, ok := thresholds[name]; !ok {
				continue
			} else if nodeValue > value {
				return true
			}
		}
	}
	return false
}

// isNodeWithLowUtilization checks if a node is underutilized
func isNodeWithLowUtilization(nodeThresholds api.ResourceThresholds, thresholds api.ResourceThresholds) bool {
	for name, nodeValue := range nodeThresholds {
		if name == v1.ResourceCPU || name == v1.ResourceMemory || name == v1.ResourcePods {
			if value, ok := thresholds[name]; !ok {
				continue
			} else if nodeValue > value {
				return false
			}
		}
	}
	return true
}

func nodeUtilization(node *v1.Node, pods []*v1.Pod) api.ResourceThresholds {
	totalReqs := map[v1.ResourceName]*resource.Quantity{
		v1.ResourceCPU:    {},
		v1.ResourceMemory: {},
	}
	for _, pod := range pods {
		req, _ := utils.PodRequestsAndLimits(pod)
		for name, quantity := range req {
			if name == v1.ResourceCPU || name == v1.ResourceMemory {
				// As Quantity.Add says: Add adds the provided y quantity to the current value. If the current value is zero,
				// the format of the quantity will be updated to the format of y.
				totalReqs[name].Add(quantity)
			}
		}
	}

	nodeCapacity := node.Status.Capacity
	if len(node.Status.Allocatable) > 0 {
		nodeCapacity = node.Status.Allocatable
	}

	totalPods := len(pods)
	return api.ResourceThresholds{
		v1.ResourceCPU:    api.Percentage((float64(totalReqs[v1.ResourceCPU].MilliValue()) * 100) / float64(nodeCapacity.Cpu().MilliValue())),
		v1.ResourceMemory: api.Percentage(float64(totalReqs[v1.ResourceMemory].Value()) / float64(nodeCapacity.Memory().Value()) * 100),
		v1.ResourcePods:   api.Percentage((float64(totalPods) * 100) / float64(nodeCapacity.Pods().Value())),
	}
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
