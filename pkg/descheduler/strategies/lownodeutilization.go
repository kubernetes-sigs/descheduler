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
	"fmt"
	"sort"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/kubernetes/pkg/api/v1"
	helper "k8s.io/kubernetes/pkg/api/v1/resource"
	"k8s.io/kubernetes/pkg/client/clientset_generated/clientset"

	"github.com/kubernetes-incubator/descheduler/cmd/descheduler/app/options"
	"github.com/kubernetes-incubator/descheduler/pkg/api"
	"github.com/kubernetes-incubator/descheduler/pkg/descheduler/evictions"
	podutil "github.com/kubernetes-incubator/descheduler/pkg/descheduler/pod"
)

type NodeUsageMap struct {
	node             *v1.Node
	usage            api.ResourceThresholds
	nonRemovablePods []*v1.Pod
	bePods           []*v1.Pod
	bPods            []*v1.Pod
	gPods            []*v1.Pod
}
type NodePodsMap map[*v1.Node][]*v1.Pod

func LowNodeUtilization(ds *options.DeschedulerServer, strategy api.DeschedulerStrategy, evictionPolicyGroupVersion string, nodes []*v1.Node) {
	if !strategy.Enabled {
		return
	}
	// todo: move to config validation?
	// TODO: May be create a struct for the strategy as well, so that we don't have to pass along the all the params?

	thresholds := strategy.Params.NodeResourceUtilizationThresholds.Thresholds
	if !validateThresholds(thresholds) {
		return
	}
	targetThresholds := strategy.Params.NodeResourceUtilizationThresholds.TargetThresholds
	if !validateTargetThresholds(targetThresholds) {
		return
	}

	npm := CreateNodePodsMap(ds.Client, nodes)
	lowNodes, targetNodes, _ := classifyNodes(npm, thresholds, targetThresholds)

	if len(lowNodes) == 0 {
		fmt.Printf("No node is underutilized\n")
		return
	} else if len(lowNodes) < strategy.Params.NodeResourceUtilizationThresholds.NumberOfNodes {
		fmt.Printf("number of nodes underutilized is less than NumberOfNodes\n")
		return
	} else if len(lowNodes) == len(nodes) {
		fmt.Printf("all nodes are underutilized\n")
		return
	} else if len(targetNodes) == 0 {
		fmt.Printf("no node is above target utilization\n")
		return
	}
	evictPodsFromTargetNodes(ds.Client, evictionPolicyGroupVersion, targetNodes, lowNodes, targetThresholds, ds.DryRun)
}

func validateThresholds(thresholds api.ResourceThresholds) bool {
	if thresholds == nil {
		fmt.Printf("no resource threshold is configured\n")
		return false
	}
	found := false
	for name, _ := range thresholds {
		if name == v1.ResourceCPU || name == v1.ResourceMemory || name == v1.ResourcePods {
			found = true
			break
		}
	}
	if !found {
		fmt.Printf("one of cpu, memory, or pods resource threshold must be configured\n")
		return false
	}
	return found
}

//This function could be merged into above once we are clear.
func validateTargetThresholds(targetThresholds api.ResourceThresholds) bool {
	if targetThresholds == nil {
		fmt.Printf("no target resource threshold is configured\n")
		return false
	} else if _, ok := targetThresholds[v1.ResourcePods]; !ok {
		fmt.Printf("no target resource threshold for pods is configured\n")
		return false
	}
	return true
}

func classifyNodes(npm NodePodsMap, thresholds api.ResourceThresholds, targetThresholds api.ResourceThresholds) ([]NodeUsageMap, []NodeUsageMap, []NodeUsageMap) {
	lowNodes, targetNodes, otherNodes := []NodeUsageMap{}, []NodeUsageMap{}, []NodeUsageMap{}
	for node, pods := range npm {
		usage, nonRemovablePods, bePods, bPods, gPods := NodeUtilization(node, pods)
		nuMap := NodeUsageMap{node, usage, nonRemovablePods, bePods, bPods, gPods}
		fmt.Printf("Node %#v usage: %#v\n", node.Name, usage)
		if IsNodeWithLowUtilization(usage, thresholds) {
			lowNodes = append(lowNodes, nuMap)
		} else if IsNodeAboveTargetUtilization(usage, targetThresholds) {
			targetNodes = append(targetNodes, nuMap)
		} else {
			// Seems we don't need to collect them?
			otherNodes = append(otherNodes, nuMap)
		}
	}
	return lowNodes, targetNodes, otherNodes
}

func evictPodsFromTargetNodes(client clientset.Interface, evictionPolicyGroupVersion string, targetNodes, lowNodes []NodeUsageMap, targetThresholds api.ResourceThresholds, dryRun bool) int {
	podsEvicted := 0

	SortNodesByUsage(targetNodes)

	// upper bound on total number of pods/cpu/memory to be moved
	var totalPods, totalCpu, totalMem float64
	for _, node := range lowNodes {
		nodeCapacity := node.node.Status.Capacity
		if len(node.node.Status.Allocatable) > 0 {
			nodeCapacity = node.node.Status.Allocatable
		}
		// totalPods to be moved
		podsPercentage := targetThresholds[v1.ResourcePods] - node.usage[v1.ResourcePods]
		totalPods += ((float64(podsPercentage) * float64(nodeCapacity.Pods().Value())) / 100)

		// totalCPU capacity to be moved
		if _, ok := targetThresholds[v1.ResourceCPU]; ok {
			cpuPercentage := targetThresholds[v1.ResourceCPU] - node.usage[v1.ResourceCPU]
			totalCpu += ((float64(cpuPercentage) * float64(nodeCapacity.Cpu().MilliValue())) / 100)
		}

		// totalMem capacity to be moved
		if _, ok := targetThresholds[v1.ResourceMemory]; ok {
			memPercentage := targetThresholds[v1.ResourceMemory] - node.usage[v1.ResourceMemory]
			totalMem += ((float64(memPercentage) * float64(nodeCapacity.Memory().Value())) / 100)
		}
	}

	for _, node := range targetNodes {
		nodeCapacity := node.node.Status.Capacity
		if len(node.node.Status.Allocatable) > 0 {
			nodeCapacity = node.node.Status.Allocatable
		}
		fmt.Printf("evicting pods from node %#v with usage: %#v\n", node.node.Name, node.usage)
		// evict best effort pods
		evictPods(node.bePods, client, evictionPolicyGroupVersion, targetThresholds, nodeCapacity, node.usage, &totalPods, &totalCpu, &totalMem, &podsEvicted, dryRun)
		// evict burstable pods
		evictPods(node.bPods, client, evictionPolicyGroupVersion, targetThresholds, nodeCapacity, node.usage, &totalPods, &totalCpu, &totalMem, &podsEvicted, dryRun)
		// evict guaranteed pods
		evictPods(node.gPods, client, evictionPolicyGroupVersion, targetThresholds, nodeCapacity, node.usage, &totalPods, &totalCpu, &totalMem, &podsEvicted, dryRun)
	}
	return podsEvicted
}

func evictPods(inputPods []*v1.Pod,
	client clientset.Interface,
	evictionPolicyGroupVersion string,
	targetThresholds api.ResourceThresholds,
	nodeCapacity v1.ResourceList,
	nodeUsage api.ResourceThresholds,
	totalPods *float64,
	totalCpu *float64,
	totalMem *float64,
	podsEvicted *int,
	dryRun bool) {
	if IsNodeAboveTargetUtilization(nodeUsage, targetThresholds) && (*totalPods > 0 || *totalCpu > 0 || *totalMem > 0) {
		onePodPercentage := api.Percentage((float64(1) * 100) / float64(nodeCapacity.Pods().Value()))
		for _, pod := range inputPods {
			cUsage := helper.GetResourceRequest(pod, v1.ResourceCPU)
			mUsage := helper.GetResourceRequest(pod, v1.ResourceMemory)
			success, err := evictions.EvictPod(client, pod, evictionPolicyGroupVersion, dryRun)
			if !success {
				fmt.Printf("Error when evicting pod: %#v (%#v)\n", pod.Name, err)
			} else {
				fmt.Printf("Evicted pod: %#v (%#v)\n", pod.Name, err)
				// update remaining pods
				*podsEvicted++
				nodeUsage[v1.ResourcePods] -= onePodPercentage
				*totalPods--

				// update remaining cpu
				*totalCpu -= float64(cUsage)
				nodeUsage[v1.ResourceCPU] -= api.Percentage((float64(cUsage) * 100) / float64(nodeCapacity.Cpu().MilliValue()))

				// update remaining memory
				*totalMem -= float64(mUsage)
				nodeUsage[v1.ResourceMemory] -= api.Percentage(float64(mUsage) / float64(nodeCapacity.Memory().Value()) * 100)

				fmt.Printf("updated node usage: %#v\n", nodeUsage)
				// check if node utilization drops below target threshold or required capacity (cpu, memory, pods) is moved
				if !IsNodeAboveTargetUtilization(nodeUsage, targetThresholds) || (*totalPods <= 0 && *totalCpu <= 0 && *totalMem <= 0) {
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

func CreateNodePodsMap(client clientset.Interface, nodes []*v1.Node) NodePodsMap {
	npm := NodePodsMap{}
	for _, node := range nodes {
		pods, err := podutil.ListPodsOnANode(client, node)
		if err != nil {
			fmt.Printf("node %s will not be processed, error in accessing its pods (%#v)\n", node.Name, err)
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

func NodeUtilization(node *v1.Node, pods []*v1.Pod) (api.ResourceThresholds, []*v1.Pod, []*v1.Pod, []*v1.Pod, []*v1.Pod) {
	bePods := []*v1.Pod{}
	nonRemovablePods := []*v1.Pod{}
	bPods := []*v1.Pod{}
	gPods := []*v1.Pod{}
	totalReqs := map[v1.ResourceName]resource.Quantity{}
	for _, pod := range pods {
		sr, err := podutil.CreatorRef(pod)
		if err != nil {
			sr = nil
		}

		if podutil.IsMirrorPod(pod) || podutil.IsPodWithLocalStorage(pod) || sr == nil || podutil.IsDaemonsetPod(sr) || podutil.IsCriticalPod(pod) {
			nonRemovablePods = append(nonRemovablePods, pod)
			if podutil.IsBestEffortPod(pod) {
				continue
			}
		} else if podutil.IsBestEffortPod(pod) {
			bePods = append(bePods, pod)
			continue
		} else if podutil.IsBurstablePod(pod) {
			bPods = append(bPods, pod)
		} else {
			gPods = append(gPods, pod)
		}

		req, _, err := helper.PodRequestsAndLimits(pod)
		if err != nil {
			fmt.Printf("Error computing resource usage of pod, ignoring: %#v\n", pod.Name)
			continue
		}
		for name, quantity := range req {
			if name == v1.ResourceCPU || name == v1.ResourceMemory {
				if value, ok := totalReqs[name]; !ok {
					totalReqs[name] = *quantity.Copy()
				} else {
					value.Add(quantity)
					totalReqs[name] = value
				}
			}
		}
	}

	nodeCapacity := node.Status.Capacity
	if len(node.Status.Allocatable) > 0 {
		nodeCapacity = node.Status.Allocatable
	}

	usage := api.ResourceThresholds{}
	totalCPUReq := totalReqs[v1.ResourceCPU]
	totalMemReq := totalReqs[v1.ResourceMemory]
	totalPods := len(pods)
	usage[v1.ResourceCPU] = api.Percentage((float64(totalCPUReq.MilliValue()) * 100) / float64(nodeCapacity.Cpu().MilliValue()))
	usage[v1.ResourceMemory] = api.Percentage(float64(totalMemReq.Value()) / float64(nodeCapacity.Memory().Value()) * 100)
	usage[v1.ResourcePods] = api.Percentage((float64(totalPods) * 100) / float64(nodeCapacity.Pods().Value()))
	return usage, nonRemovablePods, bePods, bPods, gPods
}
