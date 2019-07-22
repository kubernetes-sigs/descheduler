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
	"sort"

	"github.com/golang/glog"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	clientset "k8s.io/client-go/kubernetes"
	helper "k8s.io/kubernetes/pkg/api/v1/resource"

	"github.com/kubernetes-incubator/descheduler/cmd/descheduler/app/options"
	"github.com/kubernetes-incubator/descheduler/pkg/api"
	"github.com/kubernetes-incubator/descheduler/pkg/descheduler/evictions"
	nodeutil "github.com/kubernetes-incubator/descheduler/pkg/descheduler/node"
	podutil "github.com/kubernetes-incubator/descheduler/pkg/descheduler/pod"
)

type NodeUsageMap struct {
	node             *v1.Node
	usage            api.ResourceThresholds
	allPods          []*v1.Pod
	nonRemovablePods []*v1.Pod
	bePods           []*v1.Pod
	bPods            []*v1.Pod
	gPods            []*v1.Pod
}

type NodePodsMap map[*v1.Node][]*v1.Pod

func LowNodeUtilization(ds *options.DeschedulerServer, strategy api.DeschedulerStrategy, evictionPolicyGroupVersion string, nodes []*v1.Node, nodepodCount NodePodEvictedCount) {
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

	npm := createNodePodsMap(ds.Client, nodes)
	lowNodes, targetNodes := classifyNodes(npm, thresholds, targetThresholds, ds.EvictLocalStoragePods)

	glog.V(1).Infof("Criteria for a node under utilization: CPU: %v, Mem: %v, Pods: %v",
		thresholds[v1.ResourceCPU], thresholds[v1.ResourceMemory], thresholds[v1.ResourcePods])

	if len(lowNodes) == 0 {
		glog.V(1).Infof("No node is underutilized, nothing to do here, you might tune your thersholds further")
		return
	}
	glog.V(1).Infof("Total number of underutilized nodes: %v", len(lowNodes))

	if len(lowNodes) < strategy.Params.NodeResourceUtilizationThresholds.NumberOfNodes {
		glog.V(1).Infof("number of nodes underutilized (%v) is less than NumberOfNodes (%v), nothing to do here", len(lowNodes), strategy.Params.NodeResourceUtilizationThresholds.NumberOfNodes)
		return
	}

	if len(lowNodes) == len(nodes) {
		glog.V(1).Infof("all nodes are underutilized, nothing to do here")
		return
	}

	if len(targetNodes) == 0 {
		glog.V(1).Infof("all nodes are under target utilization, nothing to do here")
		return
	}

	glog.V(1).Infof("Criteria for a node above target utilization: CPU: %v, Mem: %v, Pods: %v",
		targetThresholds[v1.ResourceCPU], targetThresholds[v1.ResourceMemory], targetThresholds[v1.ResourcePods])
	glog.V(1).Infof("Total number of nodes above target utilization: %v", len(targetNodes))

	totalPodsEvicted := evictPodsFromTargetNodes(ds.Client, evictionPolicyGroupVersion, targetNodes, lowNodes, targetThresholds, ds.DryRun, ds.MaxNoOfPodsToEvictPerNode, nodepodCount)
	glog.V(1).Infof("Total number of pods evicted: %v", totalPodsEvicted)

}

func validateThresholds(thresholds api.ResourceThresholds) bool {
	if thresholds == nil || len(thresholds) == 0 {
		glog.V(1).Infof("no resource threshold is configured")
		return false
	}
	for name := range thresholds {
		switch name {
		case v1.ResourceCPU:
			continue
		case v1.ResourceMemory:
			continue
		case v1.ResourcePods:
			continue
		default:
			glog.Errorf("only cpu, memory, or pods thresholds can be specified")
			return false
		}
	}
	return true
}

//This function could be merged into above once we are clear.
func validateTargetThresholds(targetThresholds api.ResourceThresholds) bool {
	if targetThresholds == nil {
		glog.V(1).Infof("no target resource threshold is configured")
		return false
	} else if _, ok := targetThresholds[v1.ResourcePods]; !ok {
		glog.V(1).Infof("no target resource threshold for pods is configured")
		return false
	}
	return true
}

// classifyNodes classifies the nodes into low-utilization or high-utilization nodes. If a node lies between
// low and high thresholds, it is simply ignored.
func classifyNodes(npm NodePodsMap, thresholds api.ResourceThresholds, targetThresholds api.ResourceThresholds, evictLocalStoragePods bool) ([]NodeUsageMap, []NodeUsageMap) {
	lowNodes, targetNodes := []NodeUsageMap{}, []NodeUsageMap{}
	for node, pods := range npm {
		usage, allPods, nonRemovablePods, bePods, bPods, gPods := NodeUtilization(node, pods, evictLocalStoragePods)
		nuMap := NodeUsageMap{node, usage, allPods, nonRemovablePods, bePods, bPods, gPods}

		// Check if node is underutilized and if we can schedule pods on it.
		if !nodeutil.IsNodeUschedulable(node) && IsNodeWithLowUtilization(usage, thresholds) {
			glog.V(2).Infof("Node %#v is under utilized with usage: %#v", node.Name, usage)
			lowNodes = append(lowNodes, nuMap)
		} else if IsNodeAboveTargetUtilization(usage, targetThresholds) {
			glog.V(2).Infof("Node %#v is over utilized with usage: %#v", node.Name, usage)
			targetNodes = append(targetNodes, nuMap)
		} else {
			glog.V(2).Infof("Node %#v is appropriately utilized with usage: %#v", node.Name, usage)
		}
		glog.V(2).Infof("allPods:%v, nonRemovablePods:%v, bePods:%v, bPods:%v, gPods:%v", len(allPods), len(nonRemovablePods), len(bePods), len(bPods), len(gPods))
	}
	return lowNodes, targetNodes
}

// evictPodsFromTargetNodes evicts pods based on priority, if all the pods on the node have priority, if not
// evicts them based on QoS as fallback option.
// TODO: @ravig Break this function into smaller functions.
func evictPodsFromTargetNodes(client clientset.Interface, evictionPolicyGroupVersion string, targetNodes, lowNodes []NodeUsageMap, targetThresholds api.ResourceThresholds, dryRun bool, maxPodsToEvict int, nodepodCount NodePodEvictedCount) int {
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

	glog.V(1).Infof("Total capacity to be moved: CPU:%v, Mem:%v, Pods:%v", totalCpu, totalMem, totalPods)
	glog.V(1).Infof("********Number of pods evicted from each node:***********")

	for _, node := range targetNodes {
		nodeCapacity := node.node.Status.Capacity
		if len(node.node.Status.Allocatable) > 0 {
			nodeCapacity = node.node.Status.Allocatable
		}
		glog.V(3).Infof("evicting pods from node %#v with usage: %#v", node.node.Name, node.usage)
		currentPodsEvicted := nodepodCount[node.node]

		// Check if one pod has priority, if yes, assume that all pods have priority and evict pods based on priority.
		if node.allPods[0].Spec.Priority != nil {
			glog.V(1).Infof("All pods have priority associated with them. Evicting pods based on priority")
			evictablePods := make([]*v1.Pod, 0)
			evictablePods = append(append(node.bPods, node.bePods...), node.gPods...)

			// sort the evictable Pods based on priority. This also sorts them based on QoS. If there are multiple pods with same priority, they are sorted based on QoS tiers.
			sortPodsBasedOnPriority(evictablePods)
			evictPods(evictablePods, client, evictionPolicyGroupVersion, targetThresholds, nodeCapacity, node.usage, &totalPods, &totalCpu, &totalMem, &currentPodsEvicted, dryRun, maxPodsToEvict)
		} else {
			// TODO: Remove this when we support only priority.
			//  Falling back to evicting pods based on priority.
			glog.V(1).Infof("Evicting pods based on QoS")
			glog.V(1).Infof("There are %v non-evictable pods on the node", len(node.nonRemovablePods))
			// evict best effort pods
			evictPods(node.bePods, client, evictionPolicyGroupVersion, targetThresholds, nodeCapacity, node.usage, &totalPods, &totalCpu, &totalMem, &currentPodsEvicted, dryRun, maxPodsToEvict)
			// evict burstable pods
			evictPods(node.bPods, client, evictionPolicyGroupVersion, targetThresholds, nodeCapacity, node.usage, &totalPods, &totalCpu, &totalMem, &currentPodsEvicted, dryRun, maxPodsToEvict)
			// evict guaranteed pods
			evictPods(node.gPods, client, evictionPolicyGroupVersion, targetThresholds, nodeCapacity, node.usage, &totalPods, &totalCpu, &totalMem, &currentPodsEvicted, dryRun, maxPodsToEvict)
		}
		nodepodCount[node.node] = currentPodsEvicted
		podsEvicted = podsEvicted + nodepodCount[node.node]
		glog.V(1).Infof("%v pods evicted from node %#v with usage %v", nodepodCount[node.node], node.node.Name, node.usage)
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
	dryRun bool, maxPodsToEvict int) {
	if IsNodeAboveTargetUtilization(nodeUsage, targetThresholds) && (*totalPods > 0 || *totalCpu > 0 || *totalMem > 0) {
		onePodPercentage := api.Percentage((float64(1) * 100) / float64(nodeCapacity.Pods().Value()))
		for _, pod := range inputPods {
			if maxPodsToEvict > 0 && *podsEvicted+1 > maxPodsToEvict {
				break
			}
			cUsage := helper.GetResourceRequest(pod, v1.ResourceCPU)
			mUsage := helper.GetResourceRequest(pod, v1.ResourceMemory)
			success, err := evictions.EvictPod(client, pod, evictionPolicyGroupVersion, dryRun)
			if !success {
				glog.Warningf("Error when evicting pod: %#v (%#v)", pod.Name, err)
			} else {
				glog.V(3).Infof("Evicted pod: %#v (%#v)", pod.Name, err)
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

				glog.V(3).Infof("updated node usage: %#v", nodeUsage)
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
func createNodePodsMap(client clientset.Interface, nodes []*v1.Node) NodePodsMap {
	npm := NodePodsMap{}
	for _, node := range nodes {
		pods, err := podutil.ListPodsOnANode(client, node)
		if err != nil {
			glog.Warningf("node %s will not be processed, error in accessing its pods (%#v)", node.Name, err)
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

// Nodeutilization returns the current usage of node.
func NodeUtilization(node *v1.Node, pods []*v1.Pod, evictLocalStoragePods bool) (api.ResourceThresholds, []*v1.Pod, []*v1.Pod, []*v1.Pod, []*v1.Pod, []*v1.Pod) {
	bePods := []*v1.Pod{}
	nonRemovablePods := []*v1.Pod{}
	bPods := []*v1.Pod{}
	gPods := []*v1.Pod{}
	totalReqs := map[v1.ResourceName]resource.Quantity{}
	for _, pod := range pods {
		// We need to compute the usage of nonRemovablePods unless it is a best effort pod. So, cannot use podutil.ListEvictablePodsOnNode
		if !podutil.IsEvictable(pod, evictLocalStoragePods) {
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

		req, _ := helper.PodRequestsAndLimits(pod)
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
	return usage, pods, nonRemovablePods, bePods, bPods, gPods
}
