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
	"math"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/descheduler/pkg/api"
)

func BuildTestNodeInfo(name string, apply func(*NodeInfo)) *NodeInfo {
	nodeInfo := &NodeInfo{
		NodeUsage: NodeUsage{
			node: &v1.Node{
				Status: v1.NodeStatus{
					Capacity: v1.ResourceList{
						v1.ResourceCPU:    *resource.NewMilliQuantity(2000, resource.DecimalSI),
						v1.ResourceMemory: *resource.NewQuantity(3977868*1024, resource.BinarySI),
						v1.ResourcePods:   *resource.NewQuantity(29, resource.BinarySI),
					},
					Allocatable: v1.ResourceList{
						v1.ResourceCPU:    *resource.NewMilliQuantity(1930, resource.DecimalSI),
						v1.ResourceMemory: *resource.NewQuantity(3287692*1024, resource.BinarySI),
						v1.ResourcePods:   *resource.NewQuantity(29, resource.BinarySI),
					},
				},
				ObjectMeta: metav1.ObjectMeta{Name: name},
			},
		},
	}
	apply(nodeInfo)
	return nodeInfo
}

var (
	lowPriority      = int32(0)
	highPriority     = int32(10000)
	extendedResource = v1.ResourceName("example.com/foo")
)

func TestResourceUsagePercentages(t *testing.T) {
	resourceUsagePercentage := resourceUsagePercentages(NodeUsage{
		node: &v1.Node{
			Status: v1.NodeStatus{
				Capacity: v1.ResourceList{
					v1.ResourceCPU:    *resource.NewMilliQuantity(2000, resource.DecimalSI),
					v1.ResourceMemory: *resource.NewQuantity(3977868*1024, resource.BinarySI),
					v1.ResourcePods:   *resource.NewQuantity(29, resource.BinarySI),
				},
				Allocatable: v1.ResourceList{
					v1.ResourceCPU:    *resource.NewMilliQuantity(1930, resource.DecimalSI),
					v1.ResourceMemory: *resource.NewQuantity(3287692*1024, resource.BinarySI),
					v1.ResourcePods:   *resource.NewQuantity(29, resource.BinarySI),
				},
			},
		},
		usage: api.ReferencedResourceList{
			v1.ResourceCPU:    resource.NewMilliQuantity(1220, resource.DecimalSI),
			v1.ResourceMemory: resource.NewQuantity(3038982964, resource.BinarySI),
			v1.ResourcePods:   resource.NewQuantity(11, resource.BinarySI),
		},
	})

	expectedUsageInIntPercentage := map[v1.ResourceName]float64{
		v1.ResourceCPU:    63,
		v1.ResourceMemory: 90,
		v1.ResourcePods:   37,
	}

	for resourceName, percentage := range expectedUsageInIntPercentage {
		if math.Floor(resourceUsagePercentage[resourceName]) != percentage {
			t.Errorf("Incorrect percentange computation, expected %v, got math.Floor(%v) instead", percentage, resourceUsagePercentage[resourceName])
		}
	}

	t.Logf("resourceUsagePercentage: %#v\n", resourceUsagePercentage)
}

func TestSortNodesByUsage(t *testing.T) {
	tests := []struct {
		name                  string
		nodeInfoList          []NodeInfo
		expectedNodeInfoNames []string
	}{
		{
			name: "cpu memory pods",
			nodeInfoList: []NodeInfo{
				*BuildTestNodeInfo("node1", func(nodeInfo *NodeInfo) {
					nodeInfo.usage = api.ReferencedResourceList{
						v1.ResourceCPU:    resource.NewMilliQuantity(1730, resource.DecimalSI),
						v1.ResourceMemory: resource.NewQuantity(3038982964, resource.BinarySI),
						v1.ResourcePods:   resource.NewQuantity(25, resource.BinarySI),
					}
				}),
				*BuildTestNodeInfo("node2", func(nodeInfo *NodeInfo) {
					nodeInfo.usage = api.ReferencedResourceList{
						v1.ResourceCPU:    resource.NewMilliQuantity(1220, resource.DecimalSI),
						v1.ResourceMemory: resource.NewQuantity(3038982964, resource.BinarySI),
						v1.ResourcePods:   resource.NewQuantity(11, resource.BinarySI),
					}
				}),
				*BuildTestNodeInfo("node3", func(nodeInfo *NodeInfo) {
					nodeInfo.usage = api.ReferencedResourceList{
						v1.ResourceCPU:    resource.NewMilliQuantity(1530, resource.DecimalSI),
						v1.ResourceMemory: resource.NewQuantity(5038982964, resource.BinarySI),
						v1.ResourcePods:   resource.NewQuantity(20, resource.BinarySI),
					}
				}),
			},
			expectedNodeInfoNames: []string{"node3", "node1", "node2"},
		},
		{
			name: "memory",
			nodeInfoList: []NodeInfo{
				*BuildTestNodeInfo("node1", func(nodeInfo *NodeInfo) {
					nodeInfo.usage = api.ReferencedResourceList{
						v1.ResourceMemory: resource.NewQuantity(3038982964, resource.BinarySI),
					}
				}),
				*BuildTestNodeInfo("node2", func(nodeInfo *NodeInfo) {
					nodeInfo.usage = api.ReferencedResourceList{
						v1.ResourceMemory: resource.NewQuantity(2038982964, resource.BinarySI),
					}
				}),
				*BuildTestNodeInfo("node3", func(nodeInfo *NodeInfo) {
					nodeInfo.usage = api.ReferencedResourceList{
						v1.ResourceMemory: resource.NewQuantity(5038982964, resource.BinarySI),
					}
				}),
			},
			expectedNodeInfoNames: []string{"node3", "node1", "node2"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name+" descending", func(t *testing.T) {
			sortNodesByUsage(tc.nodeInfoList, false) // ascending=false, sort nodes in descending order

			for i := 0; i < len(tc.nodeInfoList); i++ {
				if tc.nodeInfoList[i].NodeUsage.node.Name != tc.expectedNodeInfoNames[i] {
					t.Errorf("Expected %v, got %v", tc.expectedNodeInfoNames[i], tc.nodeInfoList[i].NodeUsage.node.Name)
				}
			}
		})
		t.Run(tc.name+" ascending", func(t *testing.T) {
			sortNodesByUsage(tc.nodeInfoList, true) // ascending=true, sort nodes in ascending order

			size := len(tc.nodeInfoList)
			for i := 0; i < size; i++ {
				if tc.nodeInfoList[i].NodeUsage.node.Name != tc.expectedNodeInfoNames[size-i-1] {
					t.Errorf("Expected %v, got %v", tc.expectedNodeInfoNames[size-i-1], tc.nodeInfoList[i].NodeUsage.node.Name)
				}
			}
		})
	}
}
