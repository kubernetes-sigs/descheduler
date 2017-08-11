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

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/kubernetes/pkg/api/v1"
	helper "k8s.io/kubernetes/pkg/api/v1/resource"
	"k8s.io/kubernetes/pkg/client/clientset_generated/clientset"

	"github.com/aveshagarwal/rescheduler/pkg/api"
	podutil "github.com/aveshagarwal/rescheduler/pkg/rescheduler/pod"
)

type NodeUsageMap map[*v1.Node]api.ResourceThresholds
type NodePodsMap map[*v1.Node][]*v1.Pod

func LowNodeUtilization(client clientset.Interface, strategy api.ReschedulerStrategy, evictionPolicyGroupVersion string, nodes []*v1.Node) {
	if !strategy.Enabled {
		return
	}

	thresholds := strategy.Params.NodeResourceUtilizationThresholds.Thresholds
	if thresholds != nil {
		return
	}

	npm := CreateNodePodsMap(client, nodes)
	lowNodes, otherNodes := []*v1.Node{}, []*v1.Node{}
	nodeUsageMap := NodeUsageMap{}
	for node, pods := range npm {
		nodeUsageMap[node] = NodeUtilization(node, pods)
		fmt.Printf("Node %#v usage: %#v\n", node.Name, nodeUsageMap[node])
		if IsNodeWithLowUtilization(nodeUsageMap[node], thresholds) {
			lowNodes = append(lowNodes, node)
		} else {
			otherNodes = append(otherNodes, node)
		}
	}

	if len(lowNodes) == 0 || len(lowNodes) < strategy.Params.NodeResourceUtilizationThresholds.NumberOfNodes {
		return
	}

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

func IsNodeWithLowUtilization(nodeThresholds api.ResourceThresholds, thresholds api.ResourceThresholds) bool {
	found := true
	for name, nodeValue := range nodeThresholds {
		if name == v1.ResourceCPU || name == v1.ResourceMemory || name == v1.ResourcePods {
			if value, ok := thresholds[name]; !ok {
				continue
			} else if nodeValue > value {
				found = false
			}
		}
	}
	return found
}

func NodeUtilization(node *v1.Node, pods []*v1.Pod) api.ResourceThresholds {

	totalReqs := map[v1.ResourceName]resource.Quantity{}
	for _, pod := range pods {
		if podutil.IsBestEffortPod(pod) {
			continue
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

	allocatable := node.Status.Capacity
	if len(node.Status.Allocatable) > 0 {
		allocatable = node.Status.Allocatable
	}

	rt := api.ResourceThresholds{}
	totalCPUReq := totalReqs[v1.ResourceCPU]
	totalMemReq := totalReqs[v1.ResourceMemory]
	totalPods := len(pods)
	rt[v1.ResourceCPU] = api.Percentage((float64(totalCPUReq.MilliValue()) * 100) / float64(allocatable.Cpu().MilliValue()))
	rt[v1.ResourceMemory] = api.Percentage(float64(totalMemReq.Value()) / float64(allocatable.Memory().Value()) * 100)
	rt[v1.ResourcePods] = api.Percentage((float64(totalPods) * 100) / float64(allocatable.Pods().Value()))
	return rt
}
