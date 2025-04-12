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
