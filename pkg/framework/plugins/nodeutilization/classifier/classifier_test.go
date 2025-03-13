/*
Copyright 2025 The Kubernetes Authors.
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

package classifier

import (
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/descheduler/pkg/api"
)

func TestClassifySimple(t *testing.T) {
	for _, tt := range []struct {
		name        string
		usage       map[string]int
		limits      map[string][]int
		classifiers []Classifier[string, int]
		expected    []map[string]int
	}{
		{
			name:     "empty",
			usage:    map[string]int{},
			limits:   map[string][]int{},
			expected: []map[string]int{},
		},
		{
			name: "one under one over",
			usage: map[string]int{
				"node1": 2,
				"node2": 8,
			},
			limits: map[string][]int{
				"node1": {4, 6},
				"node2": {4, 6},
			},
			expected: []map[string]int{
				{"node1": 2},
				{"node2": 8},
			},
			classifiers: []Classifier[string, int]{
				func(_ string, usage, limit int) bool {
					return usage < limit
				},
				func(_ string, usage, limit int) bool {
					return usage > limit
				},
			},
		},
		{
			name: "randomly positioned over utilized",
			usage: map[string]int{
				"node1": 2,
				"node2": 8,
				"node3": 2,
				"node4": 8,
				"node5": 8,
				"node6": 2,
				"node7": 2,
				"node8": 8,
				"node9": 8,
			},
			limits: map[string][]int{
				"node1": {4, 6},
				"node2": {4, 6},
				"node3": {4, 6},
				"node4": {4, 6},
				"node5": {4, 6},
				"node6": {4, 6},
				"node7": {4, 6},
				"node8": {4, 6},
				"node9": {4, 6},
			},
			expected: []map[string]int{
				{
					"node1": 2,
					"node3": 2,
					"node6": 2,
					"node7": 2,
				},
				{
					"node2": 8,
					"node4": 8,
					"node5": 8,
					"node8": 8,
					"node9": 8,
				},
			},
			classifiers: []Classifier[string, int]{
				func(_ string, usage, limit int) bool {
					return usage < limit
				},
				func(_ string, usage, limit int) bool {
					return usage > limit
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result := Classify(tt.usage, tt.limits, tt.classifiers...)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Fatalf("unexpected result: %v", result)
			}
		})
	}
}

func TestClassify_pointers(t *testing.T) {
	for _, tt := range []struct {
		name        string
		usage       map[string]map[v1.ResourceName]*resource.Quantity
		limits      map[string][]map[v1.ResourceName]*resource.Quantity
		classifiers []Classifier[string, map[v1.ResourceName]*resource.Quantity]
		expected    []map[string]map[v1.ResourceName]*resource.Quantity
	}{
		{
			name:     "empty",
			usage:    map[string]map[v1.ResourceName]*resource.Quantity{},
			limits:   map[string][]map[v1.ResourceName]*resource.Quantity{},
			expected: []map[string]map[v1.ResourceName]*resource.Quantity{},
		},
		{
			name: "single underutilized",
			usage: map[string]map[v1.ResourceName]*resource.Quantity{
				"node1": {
					v1.ResourceCPU:    ptr.To(resource.MustParse("2")),
					v1.ResourceMemory: ptr.To(resource.MustParse("2Gi")),
				},
			},
			limits: map[string][]map[v1.ResourceName]*resource.Quantity{
				"node1": {
					{
						v1.ResourceCPU:    ptr.To(resource.MustParse("4")),
						v1.ResourceMemory: ptr.To(resource.MustParse("4Gi")),
					},
				},
			},
			expected: []map[string]map[v1.ResourceName]*resource.Quantity{
				{
					"node1": {
						v1.ResourceCPU:    ptr.To(resource.MustParse("2")),
						v1.ResourceMemory: ptr.To(resource.MustParse("2Gi")),
					},
				},
			},
			classifiers: []Classifier[string, map[v1.ResourceName]*resource.Quantity]{
				ForMap[string, v1.ResourceName, *resource.Quantity, map[v1.ResourceName]*resource.Quantity](
					func(usage, limit *resource.Quantity) int {
						return usage.Cmp(*limit)
					},
				),
			},
		},
		{
			name: "single underutilized and properly utilized",
			usage: map[string]map[v1.ResourceName]*resource.Quantity{
				"node1": {
					v1.ResourceCPU:    ptr.To(resource.MustParse("2")),
					v1.ResourceMemory: ptr.To(resource.MustParse("2Gi")),
				},
				"node2": {
					v1.ResourceCPU:    ptr.To(resource.MustParse("5")),
					v1.ResourceMemory: ptr.To(resource.MustParse("5Gi")),
				},
				"node3": {
					v1.ResourceCPU:    ptr.To(resource.MustParse("8")),
					v1.ResourceMemory: ptr.To(resource.MustParse("8Gi")),
				},
			},
			limits: map[string][]map[v1.ResourceName]*resource.Quantity{
				"node1": {
					{
						v1.ResourceCPU:    ptr.To(resource.MustParse("4")),
						v1.ResourceMemory: ptr.To(resource.MustParse("4Gi")),
					},
					{
						v1.ResourceCPU:    ptr.To(resource.MustParse("16")),
						v1.ResourceMemory: ptr.To(resource.MustParse("16Gi")),
					},
				},
				"node2": {
					{
						v1.ResourceCPU:    ptr.To(resource.MustParse("4")),
						v1.ResourceMemory: ptr.To(resource.MustParse("4Gi")),
					},
					{
						v1.ResourceCPU:    ptr.To(resource.MustParse("16")),
						v1.ResourceMemory: ptr.To(resource.MustParse("16Gi")),
					},
				},
				"node3": {
					{
						v1.ResourceCPU:    ptr.To(resource.MustParse("4")),
						v1.ResourceMemory: ptr.To(resource.MustParse("4Gi")),
					},
					{
						v1.ResourceCPU:    ptr.To(resource.MustParse("16")),
						v1.ResourceMemory: ptr.To(resource.MustParse("16Gi")),
					},
				},
			},
			expected: []map[string]map[v1.ResourceName]*resource.Quantity{
				{
					"node1": {
						v1.ResourceCPU:    ptr.To(resource.MustParse("2")),
						v1.ResourceMemory: ptr.To(resource.MustParse("2Gi")),
					},
				},
				{},
			},
			classifiers: []Classifier[string, map[v1.ResourceName]*resource.Quantity]{
				ForMap[string, v1.ResourceName, *resource.Quantity, map[v1.ResourceName]*resource.Quantity](
					func(usage, limit *resource.Quantity) int {
						return usage.Cmp(*limit)
					},
				),
				ForMap[string, v1.ResourceName, *resource.Quantity, map[v1.ResourceName]*resource.Quantity](
					func(usage, limit *resource.Quantity) int {
						return limit.Cmp(*usage)
					},
				),
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result := Classify(tt.usage, tt.limits, tt.classifiers...)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Fatalf("unexpected result: %v", result)
			}
		})
	}
}

func TestClassify(t *testing.T) {
	for _, tt := range []struct {
		name        string
		usage       map[string]v1.ResourceList
		limits      map[string][]v1.ResourceList
		classifiers []Classifier[string, v1.ResourceList]
		expected    []map[string]v1.ResourceList
	}{
		{
			name:     "empty",
			usage:    map[string]v1.ResourceList{},
			limits:   map[string][]v1.ResourceList{},
			expected: []map[string]v1.ResourceList{},
		},
		{
			name: "single underutilized",
			usage: map[string]v1.ResourceList{
				"node1": {
					v1.ResourceCPU:    resource.MustParse("2"),
					v1.ResourceMemory: resource.MustParse("2Gi"),
				},
			},
			limits: map[string][]v1.ResourceList{
				"node1": {
					{
						v1.ResourceCPU:    resource.MustParse("4"),
						v1.ResourceMemory: resource.MustParse("4Gi"),
					},
				},
			},
			expected: []map[string]v1.ResourceList{
				{
					"node1": {
						v1.ResourceCPU:    resource.MustParse("2"),
						v1.ResourceMemory: resource.MustParse("2Gi"),
					},
				},
			},
			classifiers: []Classifier[string, v1.ResourceList]{
				ForMap[string, v1.ResourceName, resource.Quantity, v1.ResourceList](
					func(usage, limit resource.Quantity) int {
						return usage.Cmp(limit)
					},
				),
			},
		},
		{
			name: "less classifiers than limits",
			usage: map[string]v1.ResourceList{
				"node1": {
					v1.ResourceCPU:    resource.MustParse("2"),
					v1.ResourceMemory: resource.MustParse("2Gi"),
				},
				"node2": {
					v1.ResourceCPU:    resource.MustParse("5"),
					v1.ResourceMemory: resource.MustParse("5Gi"),
				},
				"node3": {
					v1.ResourceCPU:    resource.MustParse("8"),
					v1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
			limits: map[string][]v1.ResourceList{
				"node1": {
					{
						v1.ResourceCPU:    resource.MustParse("4"),
						v1.ResourceMemory: resource.MustParse("4Gi"),
					},
					{
						v1.ResourceCPU:    resource.MustParse("16"),
						v1.ResourceMemory: resource.MustParse("16Gi"),
					},
				},
				"node2": {
					{
						v1.ResourceCPU:    resource.MustParse("4"),
						v1.ResourceMemory: resource.MustParse("4Gi"),
					},
					{
						v1.ResourceCPU:    resource.MustParse("16"),
						v1.ResourceMemory: resource.MustParse("16Gi"),
					},
				},
				"node3": {
					{
						v1.ResourceCPU:    resource.MustParse("4"),
						v1.ResourceMemory: resource.MustParse("4Gi"),
					},
					{
						v1.ResourceCPU:    resource.MustParse("16"),
						v1.ResourceMemory: resource.MustParse("16Gi"),
					},
				},
			},
			expected: []map[string]v1.ResourceList{
				{
					"node1": {
						v1.ResourceCPU:    resource.MustParse("2"),
						v1.ResourceMemory: resource.MustParse("2Gi"),
					},
				},
			},
			classifiers: []Classifier[string, v1.ResourceList]{
				ForMap[string, v1.ResourceName, resource.Quantity, v1.ResourceList](
					func(usage, limit resource.Quantity) int {
						return usage.Cmp(limit)
					},
				),
			},
		},
		{
			name: "more classifiers than limits",
			usage: map[string]v1.ResourceList{
				"node1": {
					v1.ResourceCPU:    resource.MustParse("20"),
					v1.ResourceMemory: resource.MustParse("20"),
				},
				"node2": {
					v1.ResourceCPU:    resource.MustParse("50"),
					v1.ResourceMemory: resource.MustParse("50"),
				},
				"node3": {
					v1.ResourceCPU:    resource.MustParse("80"),
					v1.ResourceMemory: resource.MustParse("80"),
				},
			},
			limits: map[string][]v1.ResourceList{
				"node1": {
					{
						v1.ResourceCPU:    resource.MustParse("30"),
						v1.ResourceMemory: resource.MustParse("30"),
					},
				},
				"node2": {
					{
						v1.ResourceCPU:    resource.MustParse("30"),
						v1.ResourceMemory: resource.MustParse("30"),
					},
				},
				"node3": {
					{
						v1.ResourceCPU:    resource.MustParse("30"),
						v1.ResourceMemory: resource.MustParse("30"),
					},
				},
			},
			expected: []map[string]v1.ResourceList{
				{
					"node1": {
						v1.ResourceCPU:    resource.MustParse("20"),
						v1.ResourceMemory: resource.MustParse("20"),
					},
				},
				{},
			},
			classifiers: []Classifier[string, v1.ResourceList]{
				ForMap[string, v1.ResourceName, resource.Quantity, v1.ResourceList](
					func(usage, limit resource.Quantity) int {
						return usage.Cmp(limit)
					},
				),
				ForMap[string, v1.ResourceName, resource.Quantity, v1.ResourceList](
					func(usage, limit resource.Quantity) int {
						return limit.Cmp(usage)
					},
				),
			},
		},
		{
			name: "single underutilized and properly utilized",
			usage: map[string]v1.ResourceList{
				"node1": {
					v1.ResourceCPU:    resource.MustParse("2"),
					v1.ResourceMemory: resource.MustParse("2Gi"),
				},
				"node2": {
					v1.ResourceCPU:    resource.MustParse("5"),
					v1.ResourceMemory: resource.MustParse("5Gi"),
				},
				"node3": {
					v1.ResourceCPU:    resource.MustParse("8"),
					v1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
			limits: map[string][]v1.ResourceList{
				"node1": {
					{
						v1.ResourceCPU:    resource.MustParse("4"),
						v1.ResourceMemory: resource.MustParse("4Gi"),
					},
					{
						v1.ResourceCPU:    resource.MustParse("16"),
						v1.ResourceMemory: resource.MustParse("16Gi"),
					},
				},
				"node2": {
					{
						v1.ResourceCPU:    resource.MustParse("4"),
						v1.ResourceMemory: resource.MustParse("4Gi"),
					},
					{
						v1.ResourceCPU:    resource.MustParse("16"),
						v1.ResourceMemory: resource.MustParse("16Gi"),
					},
				},
				"node3": {
					{
						v1.ResourceCPU:    resource.MustParse("4"),
						v1.ResourceMemory: resource.MustParse("4Gi"),
					},
					{
						v1.ResourceCPU:    resource.MustParse("16"),
						v1.ResourceMemory: resource.MustParse("16Gi"),
					},
				},
			},
			expected: []map[string]v1.ResourceList{
				{
					"node1": {
						v1.ResourceCPU:    resource.MustParse("2"),
						v1.ResourceMemory: resource.MustParse("2Gi"),
					},
				},
				{},
			},
			classifiers: []Classifier[string, v1.ResourceList]{
				ForMap[string, v1.ResourceName, resource.Quantity, v1.ResourceList](
					func(usage, limit resource.Quantity) int {
						return usage.Cmp(limit)
					},
				),
				ForMap[string, v1.ResourceName, resource.Quantity, v1.ResourceList](
					func(usage, limit resource.Quantity) int {
						return limit.Cmp(usage)
					},
				),
			},
		},
		{
			name: "single underutilized and multiple over utilized nodes",
			usage: map[string]v1.ResourceList{
				"node1": {
					v1.ResourceCPU:    resource.MustParse("2"),
					v1.ResourceMemory: resource.MustParse("2Gi"),
				},
				"node2": {
					v1.ResourceCPU:    resource.MustParse("8"),
					v1.ResourceMemory: resource.MustParse("8Gi"),
				},
				"node3": {
					v1.ResourceCPU:    resource.MustParse("8"),
					v1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
			limits: map[string][]v1.ResourceList{
				"node1": {
					{
						v1.ResourceCPU:    resource.MustParse("4"),
						v1.ResourceMemory: resource.MustParse("4Gi"),
					},
					{
						v1.ResourceCPU:    resource.MustParse("6"),
						v1.ResourceMemory: resource.MustParse("6Gi"),
					},
				},
				"node2": {
					{
						v1.ResourceCPU:    resource.MustParse("4"),
						v1.ResourceMemory: resource.MustParse("4Gi"),
					},
					{
						v1.ResourceCPU:    resource.MustParse("6"),
						v1.ResourceMemory: resource.MustParse("6Gi"),
					},
				},
				"node3": {
					{
						v1.ResourceCPU:    resource.MustParse("4"),
						v1.ResourceMemory: resource.MustParse("4Gi"),
					},
					{
						v1.ResourceCPU:    resource.MustParse("6"),
						v1.ResourceMemory: resource.MustParse("6Gi"),
					},
				},
			},
			expected: []map[string]v1.ResourceList{
				{
					"node1": {
						v1.ResourceCPU:    resource.MustParse("2"),
						v1.ResourceMemory: resource.MustParse("2Gi"),
					},
				},
				{
					"node2": {
						v1.ResourceCPU:    resource.MustParse("8"),
						v1.ResourceMemory: resource.MustParse("8Gi"),
					},
					"node3": {
						v1.ResourceCPU:    resource.MustParse("8"),
						v1.ResourceMemory: resource.MustParse("8Gi"),
					},
				},
			},
			classifiers: []Classifier[string, v1.ResourceList]{
				ForMap[string, v1.ResourceName, resource.Quantity, v1.ResourceList](
					func(usage, limit resource.Quantity) int {
						return usage.Cmp(limit)
					},
				),
				ForMap[string, v1.ResourceName, resource.Quantity, v1.ResourceList](
					func(usage, limit resource.Quantity) int {
						return limit.Cmp(usage)
					},
				),
			},
		},
		{
			name: "over and under at the same time",
			usage: map[string]v1.ResourceList{
				"node1": {
					v1.ResourceCPU:    resource.MustParse("1"),
					v1.ResourceMemory: resource.MustParse("8Gi"),
				},
				"node2": {
					v1.ResourceCPU:    resource.MustParse("1"),
					v1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
			limits: map[string][]v1.ResourceList{
				"node1": {
					{
						v1.ResourceCPU:    resource.MustParse("4"),
						v1.ResourceMemory: resource.MustParse("4Gi"),
					},
					{
						v1.ResourceCPU:    resource.MustParse("6"),
						v1.ResourceMemory: resource.MustParse("6Gi"),
					},
				},
				"node2": {
					{
						v1.ResourceCPU:    resource.MustParse("4"),
						v1.ResourceMemory: resource.MustParse("4Gi"),
					},
					{
						v1.ResourceCPU:    resource.MustParse("6"),
						v1.ResourceMemory: resource.MustParse("6Gi"),
					},
				},
			},
			expected: []map[string]v1.ResourceList{
				{},
				{},
			},
			classifiers: []Classifier[string, v1.ResourceList]{
				ForMap[string, v1.ResourceName, resource.Quantity, v1.ResourceList](
					func(usage, limit resource.Quantity) int {
						return usage.Cmp(limit)
					},
				),
				ForMap[string, v1.ResourceName, resource.Quantity, v1.ResourceList](
					func(usage, limit resource.Quantity) int {
						return limit.Cmp(usage)
					},
				),
			},
		},
		{
			name: "only memory over utilized",
			usage: map[string]v1.ResourceList{
				"node1": {
					v1.ResourceCPU:    resource.MustParse("5"),
					v1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
			limits: map[string][]v1.ResourceList{
				"node1": {
					{
						v1.ResourceCPU:    resource.MustParse("4"),
						v1.ResourceMemory: resource.MustParse("4Gi"),
					},
					{
						v1.ResourceCPU:    resource.MustParse("6"),
						v1.ResourceMemory: resource.MustParse("6Gi"),
					},
				},
			},
			expected: []map[string]v1.ResourceList{
				{},
				{},
			},
			classifiers: []Classifier[string, v1.ResourceList]{
				ForMap[string, v1.ResourceName, resource.Quantity, v1.ResourceList](
					func(usage, limit resource.Quantity) int {
						return usage.Cmp(limit)
					},
				),
				ForMap[string, v1.ResourceName, resource.Quantity, v1.ResourceList](
					func(usage, limit resource.Quantity) int {
						return limit.Cmp(usage)
					},
				),
			},
		},
		{
			name: "randomly positioned over utilized",
			usage: map[string]v1.ResourceList{
				"node1": {v1.ResourceCPU: resource.MustParse("8")},
				"node2": {v1.ResourceCPU: resource.MustParse("2")},
				"node3": {v1.ResourceCPU: resource.MustParse("8")},
				"node4": {v1.ResourceCPU: resource.MustParse("2")},
				"node5": {v1.ResourceCPU: resource.MustParse("8")},
				"node6": {v1.ResourceCPU: resource.MustParse("8")},
				"node7": {v1.ResourceCPU: resource.MustParse("8")},
				"node8": {v1.ResourceCPU: resource.MustParse("2")},
				"node9": {v1.ResourceCPU: resource.MustParse("5")},
			},
			limits: map[string][]v1.ResourceList{
				"node1": {
					{v1.ResourceCPU: resource.MustParse("4")},
					{v1.ResourceCPU: resource.MustParse("6")},
				},
				"node2": {
					{v1.ResourceCPU: resource.MustParse("4")},
					{v1.ResourceCPU: resource.MustParse("6")},
				},
				"node3": {
					{v1.ResourceCPU: resource.MustParse("4")},
					{v1.ResourceCPU: resource.MustParse("6")},
				},
				"node4": {
					{v1.ResourceCPU: resource.MustParse("4")},
					{v1.ResourceCPU: resource.MustParse("6")},
				},
				"node5": {
					{v1.ResourceCPU: resource.MustParse("4")},
					{v1.ResourceCPU: resource.MustParse("6")},
				},
				"node6": {
					{v1.ResourceCPU: resource.MustParse("4")},
					{v1.ResourceCPU: resource.MustParse("6")},
				},
				"node7": {
					{v1.ResourceCPU: resource.MustParse("4")},
					{v1.ResourceCPU: resource.MustParse("6")},
				},
				"node8": {
					{v1.ResourceCPU: resource.MustParse("4")},
					{v1.ResourceCPU: resource.MustParse("6")},
				},
				"node9": {
					{v1.ResourceCPU: resource.MustParse("4")},
					{v1.ResourceCPU: resource.MustParse("6")},
				},
			},
			expected: []map[string]v1.ResourceList{
				{
					"node2": {v1.ResourceCPU: resource.MustParse("2")},
					"node4": {v1.ResourceCPU: resource.MustParse("2")},
					"node8": {v1.ResourceCPU: resource.MustParse("2")},
				},
				{
					"node1": {v1.ResourceCPU: resource.MustParse("8")},
					"node3": {v1.ResourceCPU: resource.MustParse("8")},
					"node5": {v1.ResourceCPU: resource.MustParse("8")},
					"node6": {v1.ResourceCPU: resource.MustParse("8")},
					"node7": {v1.ResourceCPU: resource.MustParse("8")},
				},
			},
			classifiers: []Classifier[string, v1.ResourceList]{
				ForMap[string, v1.ResourceName, resource.Quantity, v1.ResourceList](
					func(usage, limit resource.Quantity) int {
						return usage.Cmp(limit)
					},
				),
				ForMap[string, v1.ResourceName, resource.Quantity, v1.ResourceList](
					func(usage, limit resource.Quantity) int {
						return limit.Cmp(usage)
					},
				),
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result := Classify(tt.usage, tt.limits, tt.classifiers...)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Fatalf("unexpected result: %v", result)
			}
		})
	}
}

func TestNormalizeAndClassify(t *testing.T) {
	for _, tt := range []struct {
		name        string
		usage       map[string]v1.ResourceList
		totals      map[string]v1.ResourceList
		thresholds  map[string][]api.ResourceThresholds
		expected    []map[string]api.ResourceThresholds
		classifiers []Classifier[string, api.ResourceThresholds]
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
			totals: Replicate(
				[]string{"node1", "node2", "node3", "node4"},
				v1.ResourceList{
					v1.ResourceCPU:    resource.MustParse("100"),
					v1.ResourceMemory: resource.MustParse("100"),
				},
			),
			thresholds: Replicate(
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
			classifiers: []Classifier[string, api.ResourceThresholds]{
				ForMap[string, v1.ResourceName, api.Percentage, api.ResourceThresholds](
					func(usage, limit api.Percentage) int {
						return int(usage - limit)
					},
				),
				ForMap[string, v1.ResourceName, api.Percentage, api.ResourceThresholds](
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
			totals: Replicate(
				[]string{"node1", "node2", "node3", "node4", "node5"},
				v1.ResourceList{
					v1.ResourceCPU:    resource.MustParse("100"),
					v1.ResourceMemory: resource.MustParse("100"),
				},
			),
			thresholds: Replicate(
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
			classifiers: []Classifier[string, api.ResourceThresholds]{
				ForMap[string, v1.ResourceName, api.Percentage, api.ResourceThresholds](
					func(usage, limit api.Percentage) int {
						return int(usage - limit)
					},
				),
				ForMap[string, v1.ResourceName, api.Percentage, api.ResourceThresholds](
					func(usage, limit api.Percentage) int {
						return int(usage - limit)
					},
				),
				ForMap[string, v1.ResourceName, api.Percentage, api.ResourceThresholds](
					func(usage, limit api.Percentage) int {
						return int(limit - usage)
					},
				),
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pct := Normalize(tt.usage, tt.totals, ResourceUsageNormalizer)
			res := Classify(pct, tt.thresholds, tt.classifiers...)
			if !reflect.DeepEqual(res, tt.expected) {
				t.Fatalf("unexpected result: %v, expecting: %v", res, tt.expected)
			}
		})
	}
}

// This is a test for thresholds being defined as deviations from the average
// usage. This is expected to be a little longer test case. We are going to
// comment the steps to make it easier to follow.
func TestUsingDeviationThresholds(t *testing.T) {
	// These are the two thresholds defined by the user. We are using
	// negative values here to compute the deviation from the average.
	// Users provide these as positive values. This needs to be taken
	// into account. These thresholds mean that our low limit will be
	// 5 pct points below the average and the high limit will be 5 pct
	// points above the average.
	userDefinedThresholds := map[string]api.ResourceThresholds{
		"low":  {v1.ResourceCPU: -5, v1.ResourceMemory: -5},
		"high": {v1.ResourceCPU: 5, v1.ResourceMemory: 5},
	}

	// Create a fake total amount of resources for all nodes. We define
	// the total amount to 1000 for both memory and cpu. This is so we
	// can easily calculate (manually) the percentage of usages here.
	nodesTotal := Replicate(
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
	usage := Normalize(nodesUsage, nodesTotal, ResourceUsageNormalizer)
	average := Average(usage)

	// Create the thresholds by first applying the deviations and then
	// replicating once for each node. Thresholds are supposed to be per
	// node even though the user provides them only once. This is by
	// design as it opens the possibility for further implementations of
	// thresholds per node.
	thresholds := Replicate(
		[]string{"node1", "node2", "node3", "node4", "node5"},
		Deviate(
			average,
			[]api.ResourceThresholds{
				userDefinedThresholds["low"],
				userDefinedThresholds["high"],
			},
		),
	)

	// Classify the nodes according to the thresholds. Nodes below the low
	// threshold (45%) are underutilized, nodes above the high threshold
	// (55%) are overutilized and nodes in between are properly utilized.
	result := Classify(
		usage, thresholds,
		ForMap[string, v1.ResourceName, api.Percentage, api.ResourceThresholds](
			func(usage, limit api.Percentage) int {
				return int(usage - limit)
			},
		),
		ForMap[string, v1.ResourceName, api.Percentage, api.ResourceThresholds](
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
		"low":  {v1.ResourceCPU: -5, v1.ResourceMemory: -5},
		"high": {v1.ResourceCPU: 5, v1.ResourceMemory: 5},
	}

	nodesTotal := Replicate(
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
		return ResourceUsageNormalizer(newUsages, newTotals)
	}

	usage := Normalize(nodesUsage, nodesTotal, ptrNormalizer)
	average := Average(usage)

	thresholds := Replicate(
		[]string{"node1", "node2", "node3", "node4", "node5"},
		Deviate(
			average,
			[]api.ResourceThresholds{
				userDefinedThresholds["low"],
				userDefinedThresholds["high"],
			},
		),
	)

	result := Classify(
		usage, thresholds,
		ForMap[string, v1.ResourceName, api.Percentage, api.ResourceThresholds](
			func(usage, limit api.Percentage) int {
				return int(usage - limit)
			},
		),
		ForMap[string, v1.ResourceName, api.Percentage, api.ResourceThresholds](
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
