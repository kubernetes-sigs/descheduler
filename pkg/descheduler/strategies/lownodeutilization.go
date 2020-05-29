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
	"k8s.io/klog"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	nodeutil "sigs.k8s.io/descheduler/pkg/descheduler/node"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/utils"
)

type NodeUsageMap struct {
	node    *v1.Node
	usage   api.ResourceThresholds
	allPods []*v1.Pod
}

type NodePodsMap map[*v1.Node][]*v1.Pod

const (
	MinResourcePercentage = 0
	MaxResourcePercentage = 100
)

func LowNodeUtilization(ctx context.Context, client clientset.Interface, strategy api.DeschedulerStrategy, nodes []*v1.Node, evictLocalStoragePods bool, podEvictor *evictions.PodEvictor) {
	// todo: move to config validation?
	// TODO: May be create a struct for the strategy as well, so that we don't have to pass along the all the params?
	if strategy.Params == nil || strategy.Params.NodeResourceUtilizationThresholds == nil {
		klog.V(1).Infof("NodeResourceUtilizationThresholds not set")
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
	lowNodes, targetNodes := classifyNodes(npm, thresholds, targetThresholds, evictLocalStoragePods)

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
		klog.V(1).Infof("all nodes are underutilized, nothing to do here")
		return
	}

	if len(targetNodes) == 0 {
		klog.V(1).Infof("all nodes are under target utilization, nothing to do here")
		return
	}

	klog.V(1).Infof("Criteria for a node above target utilization: CPU: %v, Mem: %v, Pods: %v",
		targetThresholds[v1.ResourceCPU], targetThresholds[v1.ResourceMemory], targetThresholds[v1.ResourcePods])
	klog.V(1).Infof("Total number of nodes above target utilization: %v", len(targetNodes))

	evictPodsFromTargetNodes(
		ctx,
		targetNodes,
		lowNodes,
		targetThresholds,
		evictLocalStoragePods,
		podEvictor)

	klog.V(1).Infof("Total number of pods evicted: %v", podEvictor.TotalEvicted())
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
func classifyNodes(npm NodePodsMap, thresholds api.ResourceThresholds, targetThresholds api.ResourceThresholds, evictLocalStoragePods bool) ([]NodeUsageMap, []NodeUsageMap) {
	lowNodes, targetNodes := []NodeUsageMap{}, []NodeUsageMap{}
	for node, pods := range npm {
		usage := nodeUtilization(node, pods, evictLocalStoragePods)
		nuMap := NodeUsageMap{
			node:    node,
			usage:   usage,
			allPods: pods,
		}
		// Check if node is underutilized and if we can schedule pods on it.
		if !nodeutil.IsNodeUnschedulable(node) && IsNodeWithLowUtilization(usage, thresholds) {
			klog.V(2).Infof("Node %#v is under utilized with usage: %#v", node.Name, usage)
			lowNodes = append(lowNodes, nuMap)
		} else if IsNodeAboveTargetUtilization(usage, targetThresholds) {
			klog.V(2).Infof("Node %#v is over utilized with usage: %#v", node.Name, usage)
			targetNodes = append(targetNodes, nuMap)
		} else {
			klog.V(2).Infof("Node %#v is appropriately utilized with usage: %#v", node.Name, usage)
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
	evictLocalStoragePods bool,
	podEvictor *evictions.PodEvictor,
) {

	SortNodesByUsage(targetNodes)

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

	klog.V(1).Infof("Total capacity to be moved: CPU:%v, Mem:%v, Pods:%v", totalCPU, totalMem, totalPods)
	klog.V(1).Infof("********Number of pods evicted from each node:***********")

	for _, node := range targetNodes {
		nodeCapacity := node.node.Status.Capacity
		if len(node.node.Status.Allocatable) > 0 {
			nodeCapacity = node.node.Status.Allocatable
		}
		klog.V(3).Infof("evicting pods from node %#v with usage: %#v", node.node.Name, node.usage)

		nonRemovablePods, bestEffortPods, burstablePods, guaranteedPods := classifyPods(node.allPods, evictLocalStoragePods)
		klog.V(2).Infof("allPods:%v, nonRemovablePods:%v, bestEffortPods:%v, burstablePods:%v, guaranteedPods:%v", len(node.allPods), len(nonRemovablePods), len(bestEffortPods), len(burstablePods), len(guaranteedPods))

		// Check if one pod has priority, if yes, assume that all pods have priority and evict pods based on priority.
		if node.allPods[0].Spec.Priority != nil {
			klog.V(1).Infof("All pods have priority associated with them. Evicting pods based on priority")
			evictablePods := make([]*v1.Pod, 0)
			evictablePods = append(append(burstablePods, bestEffortPods...), guaranteedPods...)

			// sort the evictable Pods based on priority. This also sorts them based on QoS. If there are multiple pods with same priority, they are sorted based on QoS tiers.
			sortPodsBasedOnPriority(evictablePods)
			evictPods(ctx, evictablePods, targetThresholds, nodeCapacity, node.usage, &totalPods, &totalCPU, &totalMem, taintsOfLowNodes, podEvictor, node.node)
		} else {
			// TODO: Remove this when we support only priority.
			//  Falling back to evicting pods based on priority.
			klog.V(1).Infof("Evicting pods based on QoS")
			klog.V(1).Infof("There are %v non-evictable pods on the node", len(nonRemovablePods))
			// evict best effort pods
			evictPods(ctx, bestEffortPods, targetThresholds, nodeCapacity, node.usage, &totalPods, &totalCPU, &totalMem, taintsOfLowNodes, podEvictor, node.node)
			// evict burstable pods
			evictPods(ctx, burstablePods, targetThresholds, nodeCapacity, node.usage, &totalPods, &totalCPU, &totalMem, taintsOfLowNodes, podEvictor, node.node)
			// evict guaranteed pods
			evictPods(ctx, guaranteedPods, targetThresholds, nodeCapacity, node.usage, &totalPods, &totalCPU, &totalMem, taintsOfLowNodes, podEvictor, node.node)
		}
		klog.V(1).Infof("%v pods evicted from node %#v with usage %v", podEvictor.NodeEvicted(node.node), node.node.Name, node.usage)
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
	if IsNodeAboveTargetUtilization(nodeUsage, targetThresholds) && *totalPods > 0 && *totalCPU > 0 && *totalMem > 0 {
		onePodPercentage := api.Percentage((float64(1) * 100) / float64(nodeCapacity.Pods().Value()))
		for _, pod := range inputPods {
			if !utils.PodToleratesTaints(pod, taintsOfLowNodes) {
				klog.V(3).Infof("Skipping eviction for Pod: %#v, doesn't tolerate node taint", pod.Name)
				continue
			}

			cUsage := utils.GetResourceRequest(pod, v1.ResourceCPU)
			mUsage := utils.GetResourceRequest(pod, v1.ResourceMemory)

			success, err := podEvictor.EvictPod(ctx, pod, node)
			if err != nil {
				klog.Errorf("Error evicting pod: (%#v)", err)
				break
			}

			if success {
				klog.V(3).Infof("Evicted pod: %#v", pod.Name)
				// update remaining pods
				nodeUsage[v1.ResourcePods] -= onePodPercentage
				*totalPods--

				// update remaining cpu
				*totalCPU -= float64(cUsage)
				nodeUsage[v1.ResourceCPU] -= api.Percentage((float64(cUsage) * 100) / float64(nodeCapacity.Cpu().MilliValue()))

				// update remaining memory
				*totalMem -= float64(mUsage)
				nodeUsage[v1.ResourceMemory] -= api.Percentage(float64(mUsage) / float64(nodeCapacity.Memory().Value()) * 100)

				klog.V(3).Infof("updated node usage: %#v", nodeUsage)
				// check if node utilization drops below target threshold or any required capacity (cpu, memory, pods) is moved
				if !IsNodeAboveTargetUtilization(nodeUsage, targetThresholds) || *totalPods <= 0 || *totalCPU <= 0 || *totalMem <= 0 {
					break
				}
			}
		}
	}
}

func SortNodesByUsage(nodes []NodeUsageMap) {
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

// sortPodsBasedOnPriority sorts pods based on priority and if their priorities are equal, they are sorted based on QoS tiers.
func sortPodsBasedOnPriority(evictablePods []*v1.Pod) {
	sort.Slice(evictablePods, func(i, j int) bool {
		if evictablePods[i].Spec.Priority == nil && evictablePods[j].Spec.Priority != nil {
			return true
		}
		if evictablePods[j].Spec.Priority == nil && evictablePods[i].Spec.Priority != nil {
			return false
		}
		if (evictablePods[j].Spec.Priority == nil && evictablePods[i].Spec.Priority == nil) || (*evictablePods[i].Spec.Priority == *evictablePods[j].Spec.Priority) {
			if podutil.IsBestEffortPod(evictablePods[i]) {
				return true
			}
			if podutil.IsBurstablePod(evictablePods[i]) && podutil.IsGuaranteedPod(evictablePods[j]) {
				return true
			}
			return false
		}
		return *evictablePods[i].Spec.Priority < *evictablePods[j].Spec.Priority
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

func IsNodeAboveTargetUtilization(nodeThresholds api.ResourceThresholds, thresholds api.ResourceThresholds) bool {
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

func IsNodeWithLowUtilization(nodeThresholds api.ResourceThresholds, thresholds api.ResourceThresholds) bool {
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

func nodeUtilization(node *v1.Node, pods []*v1.Pod, evictLocalStoragePods bool) api.ResourceThresholds {
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

func classifyPods(pods []*v1.Pod, evictLocalStoragePods bool) ([]*v1.Pod, []*v1.Pod, []*v1.Pod, []*v1.Pod) {
	var nonRemovablePods, bestEffortPods, burstablePods, guaranteedPods []*v1.Pod

	// From https://kubernetes.io/docs/tasks/configure-pod-container/quality-service-pod/
	//
	// For a Pod to be given a QoS class of Guaranteed:
	// - every Container in the Pod must have a memory limit and a memory request, and they must be the same.
	// - every Container in the Pod must have a CPU limit and a CPU request, and they must be the same.
	// A Pod is given a QoS class of Burstable if:
	// - the Pod does not meet the criteria for QoS class Guaranteed.
	// - at least one Container in the Pod has a memory or CPU request.
	// For a Pod to be given a QoS class of BestEffort, the Containers in the Pod must not have any memory or CPU limits or requests.

	for _, pod := range pods {
		if !podutil.IsEvictable(pod, evictLocalStoragePods) {
			nonRemovablePods = append(nonRemovablePods, pod)
			continue
		}

		switch utils.GetPodQOS(pod) {
		case v1.PodQOSGuaranteed:
			guaranteedPods = append(guaranteedPods, pod)
		case v1.PodQOSBurstable:
			burstablePods = append(burstablePods, pod)
		default: // alias v1.PodQOSBestEffort
			bestEffortPods = append(bestEffortPods, pod)
		}
	}

	return nonRemovablePods, bestEffortPods, burstablePods, guaranteedPods
}
