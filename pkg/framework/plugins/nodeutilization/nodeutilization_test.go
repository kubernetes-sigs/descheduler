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
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/nodeutilization/classifier"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/nodeutilization/normalizer"
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

func TestResourceUsageToResourceThreshold(t *testing.T) {
	for _, tt := range []struct {
		name     string
		usage    api.ReferencedResourceList
		capacity api.ReferencedResourceList
		expected api.ResourceThresholds
	}{
		{
			name: "10 percent",
			usage: api.ReferencedResourceList{
				v1.ResourceCPU: resource.NewMilliQuantity(100, resource.DecimalSI),
			},
			capacity: api.ReferencedResourceList{
				v1.ResourceCPU: resource.NewMilliQuantity(1000, resource.DecimalSI),
			},
			expected: api.ResourceThresholds{v1.ResourceCPU: 10},
		},
		{
			name: "zeroed out capacity",
			usage: api.ReferencedResourceList{
				v1.ResourceCPU: resource.NewMilliQuantity(100, resource.DecimalSI),
			},
			capacity: api.ReferencedResourceList{
				v1.ResourceCPU: resource.NewMilliQuantity(0, resource.DecimalSI),
			},
			expected: api.ResourceThresholds{v1.ResourceCPU: 0},
		},
		{
			name: "non existing usage",
			usage: api.ReferencedResourceList{
				"does-not-exist": resource.NewMilliQuantity(100, resource.DecimalSI),
			},
			capacity: api.ReferencedResourceList{
				v1.ResourceCPU:    resource.NewMilliQuantity(100, resource.DecimalSI),
				v1.ResourceMemory: resource.NewMilliQuantity(100, resource.DecimalSI),
			},
			expected: api.ResourceThresholds{},
		},
		{
			name: "existing and non existing usage",
			usage: api.ReferencedResourceList{
				"does-not-exist": resource.NewMilliQuantity(100, resource.DecimalSI),
				v1.ResourceCPU:   resource.NewMilliQuantity(200, resource.DecimalSI),
			},
			capacity: api.ReferencedResourceList{
				v1.ResourceCPU:    resource.NewMilliQuantity(1000, resource.DecimalSI),
				v1.ResourceMemory: resource.NewMilliQuantity(1000, resource.DecimalSI),
			},
			expected: api.ResourceThresholds{v1.ResourceCPU: 20},
		},
		{
			name: "nil usage",
			usage: api.ReferencedResourceList{
				v1.ResourceCPU: nil,
			},
			capacity: api.ReferencedResourceList{
				v1.ResourceCPU: resource.NewMilliQuantity(1000, resource.DecimalSI),
			},
			expected: api.ResourceThresholds{},
		},
		{
			name: "nil capacity",
			usage: api.ReferencedResourceList{
				v1.ResourceCPU: resource.NewMilliQuantity(100, resource.DecimalSI),
			},
			capacity: api.ReferencedResourceList{
				v1.ResourceCPU: nil,
			},
			expected: api.ResourceThresholds{},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result := ResourceUsageToResourceThreshold(tt.usage, tt.capacity)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func ResourceListUsageNormalizer(usages, totals v1.ResourceList) api.ResourceThresholds {
	result := api.ResourceThresholds{}
	for rname, value := range usages {
		total, ok := totals[rname]
		if !ok {
			continue
		}

		used, avail := value.Value(), total.Value()
		if rname == v1.ResourceCPU {
			used, avail = value.MilliValue(), total.MilliValue()
		}

		pct := math.Max(math.Min(float64(used)/float64(avail)*100, 100), 0)
		result[rname] = api.Percentage(pct)
	}
	return result
}

// This is a test for thresholds being defined as deviations from the average
// usage. This is expected to be a little longer test case. We are going to
// comment the steps to make it easier to follow.
func TestClassificationUsingDeviationThresholds(t *testing.T) {
	// These are the two thresholds defined by the user. These thresholds
	// mean that our low limit will be 5 pct points below the average and
	// the high limit will be 5 pct points above the average.
	userDefinedThresholds := map[string]api.ResourceThresholds{
		"low":  {v1.ResourceCPU: 5, v1.ResourceMemory: 5},
		"high": {v1.ResourceCPU: 5, v1.ResourceMemory: 5},
	}

	// Create a fake total amount of resources for all nodes. We define
	// the total amount to 1000 for both memory and cpu. This is so we
	// can easily calculate (manually) the percentage of usages here.
	nodesTotal := normalizer.Replicate(
		[]string{"node1", "node2", "node3", "node4", "node5"},
		v1.ResourceList{
			v1.ResourceCPU:    resource.MustParse("1000"),
			v1.ResourceMemory: resource.MustParse("1000"),
		},
	)

	// Create a fake usage per server per resource. We are aiming to
	// have the average of these resources in 50%. When applying the
	// thresholds we should obtain the low threhold at 45% and the high
	// threshold at 55%.
	nodesUsage := map[string]v1.ResourceList{
		"node1": {
			v1.ResourceCPU:    resource.MustParse("100"),
			v1.ResourceMemory: resource.MustParse("100"),
		},
		"node2": {
			v1.ResourceCPU:    resource.MustParse("480"),
			v1.ResourceMemory: resource.MustParse("480"),
		},
		"node3": {
			v1.ResourceCPU:    resource.MustParse("520"),
			v1.ResourceMemory: resource.MustParse("520"),
		},
		"node4": {
			v1.ResourceCPU:    resource.MustParse("500"),
			v1.ResourceMemory: resource.MustParse("500"),
		},
		"node5": {
			v1.ResourceCPU:    resource.MustParse("900"),
			v1.ResourceMemory: resource.MustParse("900"),
		},
	}

	// Normalize the usage to percentages and then calculate the average
	// among all nodes.
	usage := normalizer.Normalize(nodesUsage, nodesTotal, ResourceListUsageNormalizer)
	average := normalizer.Average(usage)

	// Create the thresholds by first applying the deviations and then
	// replicating once for each node. Thresholds are supposed to be per
	// node even though the user provides them only once. This is by
	// design as it opens the possibility for further implementations of
	// thresholds per node.
	thresholds := normalizer.Replicate(
		[]string{"node1", "node2", "node3", "node4", "node5"},
		[]api.ResourceThresholds{
			normalizer.Sum(average, normalizer.Negate(userDefinedThresholds["low"])),
			normalizer.Sum(average, userDefinedThresholds["high"]),
		},
	)

	// Classify the nodes according to the thresholds. Nodes below the low
	// threshold (45%) are underutilized, nodes above the high threshold
	// (55%) are overutilized and nodes in between are properly utilized.
	result := classifier.Classify(
		usage, thresholds,
		classifier.ForMap[string, v1.ResourceName, api.Percentage, api.ResourceThresholds](
			func(usage, limit api.Percentage) int {
				return int(usage - limit)
			},
		),
		classifier.ForMap[string, v1.ResourceName, api.Percentage, api.ResourceThresholds](
			func(usage, limit api.Percentage) int {
				return int(limit - usage)
			},
		),
	)

	// we expect the node1 to be undertilized (10%), node2, node3 and node4
	// to be properly utilized (48%, 52% and 50% respectively) and node5 to
	// be overutilized (90%).
	expected := []map[string]api.ResourceThresholds{
		{"node1": {v1.ResourceCPU: 10, v1.ResourceMemory: 10}},
		{"node5": {v1.ResourceCPU: 90, v1.ResourceMemory: 90}},
	}

	if !reflect.DeepEqual(result, expected) {
		t.Fatalf("unexpected result: %v, expecting: %v", result, expected)
	}
}

// This is almost a copy of TestUsingDeviationThresholds but we are using
// pointers here. This is for making sure our generic types are in check. To
// understand this code better read comments on TestUsingDeviationThresholds.
func TestUsingDeviationThresholdsWithPointers(t *testing.T) {
	userDefinedThresholds := map[string]api.ResourceThresholds{
		"low":  {v1.ResourceCPU: 5, v1.ResourceMemory: 5},
		"high": {v1.ResourceCPU: 5, v1.ResourceMemory: 5},
	}

	nodesTotal := normalizer.Replicate(
		[]string{"node1", "node2", "node3", "node4", "node5"},
		map[v1.ResourceName]*resource.Quantity{
			v1.ResourceCPU:    ptr.To(resource.MustParse("1000")),
			v1.ResourceMemory: ptr.To(resource.MustParse("1000")),
		},
	)

	nodesUsage := map[string]map[v1.ResourceName]*resource.Quantity{
		"node1": {
			v1.ResourceCPU:    ptr.To(resource.MustParse("100")),
			v1.ResourceMemory: ptr.To(resource.MustParse("100")),
		},
		"node2": {
			v1.ResourceCPU:    ptr.To(resource.MustParse("480")),
			v1.ResourceMemory: ptr.To(resource.MustParse("480")),
		},
		"node3": {
			v1.ResourceCPU:    ptr.To(resource.MustParse("520")),
			v1.ResourceMemory: ptr.To(resource.MustParse("520")),
		},
		"node4": {
			v1.ResourceCPU:    ptr.To(resource.MustParse("500")),
			v1.ResourceMemory: ptr.To(resource.MustParse("500")),
		},
		"node5": {
			v1.ResourceCPU:    ptr.To(resource.MustParse("900")),
			v1.ResourceMemory: ptr.To(resource.MustParse("900")),
		},
	}

	ptrNormalizer := func(
		usages, totals map[v1.ResourceName]*resource.Quantity,
	) api.ResourceThresholds {
		newUsages := v1.ResourceList{}
		for name, usage := range usages {
			newUsages[name] = *usage
		}
		newTotals := v1.ResourceList{}
		for name, total := range totals {
			newTotals[name] = *total
		}
		return ResourceListUsageNormalizer(newUsages, newTotals)
	}

	usage := normalizer.Normalize(nodesUsage, nodesTotal, ptrNormalizer)
	average := normalizer.Average(usage)

	thresholds := normalizer.Replicate(
		[]string{"node1", "node2", "node3", "node4", "node5"},
		[]api.ResourceThresholds{
			normalizer.Sum(average, normalizer.Negate(userDefinedThresholds["low"])),
			normalizer.Sum(average, userDefinedThresholds["high"]),
		},
	)

	result := classifier.Classify(
		usage, thresholds,
		classifier.ForMap[string, v1.ResourceName, api.Percentage, api.ResourceThresholds](
			func(usage, limit api.Percentage) int {
				return int(usage - limit)
			},
		),
		classifier.ForMap[string, v1.ResourceName, api.Percentage, api.ResourceThresholds](
			func(usage, limit api.Percentage) int {
				return int(limit - usage)
			},
		),
	)

	expected := []map[string]api.ResourceThresholds{
		{"node1": {v1.ResourceCPU: 10, v1.ResourceMemory: 10}},
		{"node5": {v1.ResourceCPU: 90, v1.ResourceMemory: 90}},
	}

	if !reflect.DeepEqual(result, expected) {
		t.Fatalf("unexpected result: %v, expecting: %v", result, expected)
	}
}

func TestNormalizeAndClassify(t *testing.T) {
	for _, tt := range []struct {
		name        string
		usage       map[string]v1.ResourceList
		totals      map[string]v1.ResourceList
		thresholds  map[string][]api.ResourceThresholds
		expected    []map[string]api.ResourceThresholds
		classifiers []classifier.Classifier[string, api.ResourceThresholds]
	}{
		{
			name: "happy path test",
			usage: map[string]v1.ResourceList{
				"node1": {
					// underutilized on cpu and memory.
					v1.ResourceCPU:    resource.MustParse("10"),
					v1.ResourceMemory: resource.MustParse("10"),
				},
				"node2": {
					// overutilized on cpu and memory.
					v1.ResourceCPU:    resource.MustParse("90"),
					v1.ResourceMemory: resource.MustParse("90"),
				},
				"node3": {
					// properly utilized on cpu and memory.
					v1.ResourceCPU:    resource.MustParse("50"),
					v1.ResourceMemory: resource.MustParse("50"),
				},
				"node4": {
					// underutilized on cpu and overutilized on memory.
					v1.ResourceCPU:    resource.MustParse("10"),
					v1.ResourceMemory: resource.MustParse("90"),
				},
			},
			totals: normalizer.Replicate(
				[]string{"node1", "node2", "node3", "node4"},
				v1.ResourceList{
					v1.ResourceCPU:    resource.MustParse("100"),
					v1.ResourceMemory: resource.MustParse("100"),
				},
			),
			thresholds: normalizer.Replicate(
				[]string{"node1", "node2", "node3", "node4"},
				[]api.ResourceThresholds{
					{v1.ResourceCPU: 20, v1.ResourceMemory: 20},
					{v1.ResourceCPU: 80, v1.ResourceMemory: 80},
				},
			),
			expected: []map[string]api.ResourceThresholds{
				{
					"node1": {v1.ResourceCPU: 10, v1.ResourceMemory: 10},
				},
				{
					"node2": {v1.ResourceCPU: 90, v1.ResourceMemory: 90},
				},
			},
			classifiers: []classifier.Classifier[string, api.ResourceThresholds]{
				classifier.ForMap[string, v1.ResourceName, api.Percentage, api.ResourceThresholds](
					func(usage, limit api.Percentage) int {
						return int(usage - limit)
					},
				),
				classifier.ForMap[string, v1.ResourceName, api.Percentage, api.ResourceThresholds](
					func(usage, limit api.Percentage) int {
						return int(limit - usage)
					},
				),
			},
		},
		{
			name: "three thresholds",
			usage: map[string]v1.ResourceList{
				"node1": {
					// match for the first classifier.
					v1.ResourceCPU:    resource.MustParse("10"),
					v1.ResourceMemory: resource.MustParse("10"),
				},
				"node2": {
					// match for the third classifier.
					v1.ResourceCPU:    resource.MustParse("90"),
					v1.ResourceMemory: resource.MustParse("90"),
				},
				"node3": {
					// match fo the second classifier.
					v1.ResourceCPU:    resource.MustParse("40"),
					v1.ResourceMemory: resource.MustParse("40"),
				},
				"node4": {
					// matches no classifier.
					v1.ResourceCPU:    resource.MustParse("10"),
					v1.ResourceMemory: resource.MustParse("90"),
				},
				"node5": {
					// match for the first classifier.
					v1.ResourceCPU:    resource.MustParse("11"),
					v1.ResourceMemory: resource.MustParse("18"),
				},
			},
			totals: normalizer.Replicate(
				[]string{"node1", "node2", "node3", "node4", "node5"},
				v1.ResourceList{
					v1.ResourceCPU:    resource.MustParse("100"),
					v1.ResourceMemory: resource.MustParse("100"),
				},
			),
			thresholds: normalizer.Replicate(
				[]string{"node1", "node2", "node3", "node4", "node5"},
				[]api.ResourceThresholds{
					{v1.ResourceCPU: 20, v1.ResourceMemory: 20},
					{v1.ResourceCPU: 50, v1.ResourceMemory: 50},
					{v1.ResourceCPU: 80, v1.ResourceMemory: 80},
				},
			),
			expected: []map[string]api.ResourceThresholds{
				{
					"node1": {v1.ResourceCPU: 10, v1.ResourceMemory: 10},
					"node5": {v1.ResourceCPU: 11, v1.ResourceMemory: 18},
				},
				{
					"node3": {v1.ResourceCPU: 40, v1.ResourceMemory: 40},
				},
				{
					"node2": {v1.ResourceCPU: 90, v1.ResourceMemory: 90},
				},
			},
			classifiers: []classifier.Classifier[string, api.ResourceThresholds]{
				classifier.ForMap[string, v1.ResourceName, api.Percentage, api.ResourceThresholds](
					func(usage, limit api.Percentage) int {
						return int(usage - limit)
					},
				),
				classifier.ForMap[string, v1.ResourceName, api.Percentage, api.ResourceThresholds](
					func(usage, limit api.Percentage) int {
						return int(usage - limit)
					},
				),
				classifier.ForMap[string, v1.ResourceName, api.Percentage, api.ResourceThresholds](
					func(usage, limit api.Percentage) int {
						return int(limit - usage)
					},
				),
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pct := normalizer.Normalize(tt.usage, tt.totals, ResourceListUsageNormalizer)
			res := classifier.Classify(pct, tt.thresholds, tt.classifiers...)
			if !reflect.DeepEqual(res, tt.expected) {
				t.Fatalf("unexpected result: %v, expecting: %v", res, tt.expected)
			}
		})
	}
}
