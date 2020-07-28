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
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/utils"
)

// NodePodsMap is a set of (node, pods) pairs
type NodePodsMap map[*v1.Node][]*v1.Pod

// NodeUsageMap stores a node's info, pods on it and its resource usage
type NodeUsageMap struct {
	node    *v1.Node
	usage   api.ResourceThresholds
	allPods []*v1.Pod
}

const (
	// MinResourcePercentage is the minimum value of a resource's percentage
	MinResourcePercentage = 0
	// MaxResourcePercentage is the maximum value of a resource's percentage
	MaxResourcePercentage = 100
)

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

// setMaxValuesForMissingThreshold check if Pods/CPU/Mem are set, if not, set them to 100
func setMaxValuesForMissingThresholds(thresholds, targetThresholds api.ResourceThresholds) {
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

// isNodeBelowThresholdUtilization checks if a node is underutilized based on threshold
func isNodeBelowThresholdUtilization(nodeThresholds api.ResourceThresholds, thresholds api.ResourceThresholds) bool {
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

// isNodeAboveThresholdUtilization checks if a node is overutilized based on threshold
func isNodeAboveThresholdUtilization(nodeThresholds api.ResourceThresholds, thresholds api.ResourceThresholds) bool {
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

func classifyPods(pods []*v1.Pod, evictor *evictions.PodEvictor) ([]*v1.Pod, []*v1.Pod) {
	var nonRemovablePods, removablePods []*v1.Pod

	for _, pod := range pods {
		if !evictor.IsEvictable(pod) {
			nonRemovablePods = append(nonRemovablePods, pod)
		} else {
			removablePods = append(removablePods, pod)
		}
	}

	return nonRemovablePods, removablePods
}

// sortNodesByUsage sorts nodes based on usage in descending order
func sortNodesByUsage(nodes []NodeUsageMap, descending bool) {
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
		if descending {
			return ti > tj
		}
		return ti < tj
	})
}

func computeDesiredNodeResourcesAndTaints(desiredNodes []NodeUsageMap,
	thresholds api.ResourceThresholds) (float64, float64, float64, map[string][]v1.Taint) {
	var totalPods, totalCPU, totalMem float64
	var taintsOfDesiredNodes = make(map[string][]v1.Taint, len(desiredNodes))
	for _, node := range desiredNodes {
		taintsOfDesiredNodes[node.node.Name] = node.node.Spec.Taints
		nodeCapacity := node.node.Status.Capacity
		if len(node.node.Status.Allocatable) > 0 {
			nodeCapacity = node.node.Status.Allocatable
		}
		// totalPods to be moved
		podsPercentage := thresholds[v1.ResourcePods] - node.usage[v1.ResourcePods]
		totalPods += ((float64(podsPercentage) * float64(nodeCapacity.Pods().Value())) / 100)

		// totalCPU capacity to be moved
		cpuPercentage := thresholds[v1.ResourceCPU] - node.usage[v1.ResourceCPU]
		totalCPU += ((float64(cpuPercentage) * float64(nodeCapacity.Cpu().MilliValue())) / 100)

		// totalMem capacity to be moved
		memPercentage := thresholds[v1.ResourceMemory] - node.usage[v1.ResourceMemory]
		totalMem += ((float64(memPercentage) * float64(nodeCapacity.Memory().Value())) / 100)
	}
	return totalPods, totalCPU, totalMem, taintsOfDesiredNodes
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
