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

package normalizer

import (
	"fmt"
	"math"
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/descheduler/pkg/api"
)

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

func TestNormalizeSimple(t *testing.T) {
	for _, tt := range []struct {
		name       string
		usages     map[string]float64
		totals     map[string]float64
		expected   map[string]float64
		normalizer Normalizer[float64, float64]
	}{
		{
			name:     "single normalization",
			usages:   map[string]float64{"cpu": 1},
			totals:   map[string]float64{"cpu": 2},
			expected: map[string]float64{"cpu": 0.5},
			normalizer: func(usage, total float64) float64 {
				return usage / total
			},
		},
		{
			name: "multiple normalizations",
			usages: map[string]float64{
				"cpu": 1,
				"mem": 6,
			},
			totals: map[string]float64{
				"cpu": 2,
				"mem": 10,
			},
			expected: map[string]float64{
				"cpu": 0.5,
				"mem": 0.6,
			},
			normalizer: func(usage, total float64) float64 {
				return usage / total
			},
		},
		{
			name: "missing totals for a key",
			usages: map[string]float64{
				"cpu": 1,
				"mem": 6,
			},
			totals: map[string]float64{
				"cpu": 2,
			},
			expected: map[string]float64{
				"cpu": 0.5,
			},
			normalizer: func(usage, total float64) float64 {
				return usage / total
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result := Normalize(tt.usages, tt.totals, tt.normalizer)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Fatalf("unexpected result: %v", result)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	for _, tt := range []struct {
		name       string
		usages     map[string]v1.ResourceList
		totals     map[string]v1.ResourceList
		expected   map[string]api.ResourceThresholds
		normalizer Normalizer[v1.ResourceList, api.ResourceThresholds]
	}{
		{
			name: "single normalization",
			usages: map[string]v1.ResourceList{
				"node1": {v1.ResourceCPU: resource.MustParse("1")},
			},
			totals: map[string]v1.ResourceList{
				"node1": {v1.ResourceCPU: resource.MustParse("2")},
			},
			expected: map[string]api.ResourceThresholds{
				"node1": {v1.ResourceCPU: 50},
			},
			normalizer: ResourceListUsageNormalizer,
		},
		{
			name: "multiple normalization",
			usages: map[string]v1.ResourceList{
				"node1": {
					v1.ResourceCPU:    resource.MustParse("1"),
					v1.ResourceMemory: resource.MustParse("6"),
					v1.ResourcePods:   resource.MustParse("2"),
				},
				"node2": {
					v1.ResourceCPU:    resource.MustParse("10"),
					v1.ResourceMemory: resource.MustParse("20"),
					v1.ResourcePods:   resource.MustParse("30"),
				},
			},
			totals: map[string]v1.ResourceList{
				"node1": {
					v1.ResourceCPU:    resource.MustParse("2"),
					v1.ResourceMemory: resource.MustParse("6"),
					v1.ResourcePods:   resource.MustParse("100"),
				},
				"node2": {
					v1.ResourceCPU:    resource.MustParse("100"),
					v1.ResourceMemory: resource.MustParse("100"),
					v1.ResourcePods:   resource.MustParse("100"),
				},
			},
			expected: map[string]api.ResourceThresholds{
				"node1": {
					v1.ResourceCPU:    50,
					v1.ResourceMemory: 100,
					v1.ResourcePods:   2,
				},
				"node2": {
					v1.ResourceCPU:    10,
					v1.ResourceMemory: 20,
					v1.ResourcePods:   30,
				},
			},
			normalizer: ResourceListUsageNormalizer,
		},
		{
			name: "multiple normalization with over 100% usage",
			usages: map[string]v1.ResourceList{
				"node1": {
					v1.ResourceCPU:    resource.MustParse("120"),
					v1.ResourceMemory: resource.MustParse("130"),
					v1.ResourcePods:   resource.MustParse("140"),
				},
				"node2": {
					v1.ResourceCPU:    resource.MustParse("150"),
					v1.ResourceMemory: resource.MustParse("160"),
					v1.ResourcePods:   resource.MustParse("170"),
				},
			},
			totals: Replicate(
				[]string{"node1", "node2"},
				v1.ResourceList{
					v1.ResourceCPU:    resource.MustParse("100"),
					v1.ResourceMemory: resource.MustParse("100"),
					v1.ResourcePods:   resource.MustParse("100"),
				},
			),
			expected: Replicate(
				[]string{"node1", "node2"},
				api.ResourceThresholds{
					v1.ResourceCPU:    100,
					v1.ResourceMemory: 100,
					v1.ResourcePods:   100,
				},
			),
			normalizer: ResourceListUsageNormalizer,
		},
		{
			name: "multiple normalization with over 100% usage and different totals",
			usages: map[string]v1.ResourceList{
				"node1": {
					v1.ResourceCPU:    resource.MustParse("2"),
					v1.ResourceMemory: resource.MustParse("2Gi"),
				},
				"node2": {
					v1.ResourceCPU:    resource.MustParse("99"),
					v1.ResourceMemory: resource.MustParse("99Gi"),
				},
				"node3": {
					v1.ResourceCPU:    resource.MustParse("8"),
					v1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
			totals: map[string]v1.ResourceList{
				"node1": {
					v1.ResourceCPU:    resource.MustParse("4"),
					v1.ResourceMemory: resource.MustParse("4Gi"),
				},
				"node2": {
					v1.ResourceCPU:    resource.MustParse("100"),
					v1.ResourceMemory: resource.MustParse("100Gi"),
				},
				"node3": {
					v1.ResourceCPU:    resource.MustParse("4"),
					v1.ResourceMemory: resource.MustParse("4Gi"),
				},
			},
			expected: map[string]api.ResourceThresholds{
				"node1": {
					v1.ResourceCPU:    50,
					v1.ResourceMemory: 50,
				},
				"node2": {
					v1.ResourceCPU:    99,
					v1.ResourceMemory: 99,
				},
				"node3": {
					v1.ResourceCPU:    100,
					v1.ResourceMemory: 100,
				},
			},
			normalizer: ResourceListUsageNormalizer,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result := Normalize(tt.usages, tt.totals, tt.normalizer)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Fatalf("unexpected result: %v", result)
			}
		})
	}
}

func TestAverage(t *testing.T) {
	for _, tt := range []struct {
		name     string
		usage    map[string]v1.ResourceList
		limits   map[string]v1.ResourceList
		expected api.ResourceThresholds
	}{
		{
			name:     "empty usage",
			usage:    map[string]v1.ResourceList{},
			limits:   map[string]v1.ResourceList{},
			expected: api.ResourceThresholds{},
		},
		{
			name: "fifty percent usage",
			usage: map[string]v1.ResourceList{
				"node1": {
					v1.ResourceCPU:    resource.MustParse("1"),
					v1.ResourceMemory: resource.MustParse("6"),
				},
				"node2": {
					v1.ResourceCPU:    resource.MustParse("2"),
					v1.ResourceMemory: resource.MustParse("6"),
				},
			},
			limits: map[string]v1.ResourceList{
				"node1": {
					v1.ResourceCPU:    resource.MustParse("2"),
					v1.ResourceMemory: resource.MustParse("12"),
				},
				"node2": {
					v1.ResourceCPU:    resource.MustParse("4"),
					v1.ResourceMemory: resource.MustParse("12"),
				},
			},
			expected: api.ResourceThresholds{
				v1.ResourceCPU:    50,
				v1.ResourceMemory: 50,
			},
		},
		{
			name: "mixed percent usage",
			usage: map[string]v1.ResourceList{
				"node1": {
					v1.ResourceCPU:    resource.MustParse("10"),
					v1.ResourceMemory: resource.MustParse("80"),
					v1.ResourcePods:   resource.MustParse("20"),
				},
				"node2": {
					v1.ResourceCPU:    resource.MustParse("20"),
					v1.ResourceMemory: resource.MustParse("60"),
					v1.ResourcePods:   resource.MustParse("20"),
				},
			},
			limits: Replicate(
				[]string{"node1", "node2"},
				v1.ResourceList{
					v1.ResourceCPU:    resource.MustParse("100"),
					v1.ResourceMemory: resource.MustParse("100"),
					v1.ResourcePods:   resource.MustParse("10000"),
				},
			),
			expected: api.ResourceThresholds{
				v1.ResourceCPU:    15,
				v1.ResourceMemory: 70,
				v1.ResourcePods:   0.2,
			},
		},
		{
			name: "mixed limits",
			usage: map[string]v1.ResourceList{
				"node1": {
					v1.ResourceCPU:    resource.MustParse("10"),
					v1.ResourceMemory: resource.MustParse("30"),
					v1.ResourcePods:   resource.MustParse("200"),
				},
				"node2": {
					v1.ResourceCPU:    resource.MustParse("10"),
					v1.ResourceMemory: resource.MustParse("72"),
					v1.ResourcePods:   resource.MustParse("200"),
				},
			},
			limits: map[string]v1.ResourceList{
				"node1": {
					v1.ResourceCPU:    resource.MustParse("10"),
					v1.ResourceMemory: resource.MustParse("100"),
					v1.ResourcePods:   resource.MustParse("1000"),
				},
				"node2": {
					v1.ResourceCPU:    resource.MustParse("1000"),
					v1.ResourceMemory: resource.MustParse("180"),
					v1.ResourcePods:   resource.MustParse("10"),
				},
			},
			expected: api.ResourceThresholds{
				v1.ResourceCPU:    50.5,
				v1.ResourceMemory: 35,
				v1.ResourcePods:   60,
			},
		},
		{
			name: "some nodes missing some resources",
			usage: map[string]v1.ResourceList{
				"node1": {
					"limit-exists-in-all":  resource.MustParse("10"),
					"limit-exists-in-two":  resource.MustParse("11"),
					"limit-does-not-exist": resource.MustParse("12"),
					"usage-exists-in-all":  resource.MustParse("13"),
					"usage-exists-in-two":  resource.MustParse("20"),
				},
				"node2": {
					"limit-exists-in-all":  resource.MustParse("10"),
					"limit-exists-in-two":  resource.MustParse("11"),
					"limit-does-not-exist": resource.MustParse("12"),
					"usage-exists-in-all":  resource.MustParse("13"),
					"usage-exists-in-two":  resource.MustParse("20"),
				},
				"node3": {
					"limit-exists-in-all":  resource.MustParse("10"),
					"limit-exists-in-two":  resource.MustParse("11"),
					"limit-does-not-exist": resource.MustParse("12"),
					"usage-exists-in-all":  resource.MustParse("13"),
				},
				"node4": {
					"limit-exists-in-all":  resource.MustParse("10"),
					"limit-exists-in-two":  resource.MustParse("11"),
					"limit-does-not-exist": resource.MustParse("12"),
					"usage-exists-in-all":  resource.MustParse("13"),
				},
				"node5": {
					"random-usage-without-limit": resource.MustParse("10"),
				},
			},
			limits: map[string]v1.ResourceList{
				"node1": {
					"limit-exists-in-all":  resource.MustParse("100"),
					"limit-exists-in-two":  resource.MustParse("100"),
					"usage-exists-in-all":  resource.MustParse("100"),
					"usage-exists-in-two":  resource.MustParse("100"),
					"usage-does-not-exist": resource.MustParse("100"),
				},
				"node2": {
					"limit-exists-in-all":  resource.MustParse("100"),
					"limit-exists-in-two":  resource.MustParse("100"),
					"usage-exists-in-all":  resource.MustParse("100"),
					"usage-exists-in-two":  resource.MustParse("100"),
					"usage-does-not-exist": resource.MustParse("100"),
				},
				"node3": {
					"limit-exists-in-all":  resource.MustParse("100"),
					"usage-exists-in-all":  resource.MustParse("100"),
					"usage-exists-in-two":  resource.MustParse("100"),
					"usage-does-not-exist": resource.MustParse("100"),
				},
				"node4": {
					"limit-exists-in-all":  resource.MustParse("100"),
					"usage-exists-in-all":  resource.MustParse("100"),
					"usage-exists-in-two":  resource.MustParse("100"),
					"usage-does-not-exist": resource.MustParse("100"),
				},
				"node5": {
					"random-limit-without-usage": resource.MustParse("100"),
				},
			},
			expected: api.ResourceThresholds{
				"limit-exists-in-all": 10,
				"limit-exists-in-two": 11,
				"usage-exists-in-all": 13,
				"usage-exists-in-two": 20,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			average := Average(
				Normalize(
					tt.usage, tt.limits, ResourceListUsageNormalizer,
				),
			)
			if !reflect.DeepEqual(average, tt.expected) {
				t.Fatalf("unexpected result: %v, expected: %v", average, tt.expected)
			}
		})
	}
}

func TestSum(t *testing.T) {
	for _, tt := range []struct {
		name       string
		data       api.ResourceThresholds
		deviations []api.ResourceThresholds
		expected   []api.ResourceThresholds
	}{
		{
			name: "single deviation",
			data: api.ResourceThresholds{
				v1.ResourceCPU:    50,
				v1.ResourceMemory: 50,
				v1.ResourcePods:   50,
			},
			deviations: []api.ResourceThresholds{
				{
					v1.ResourceCPU:    1,
					v1.ResourceMemory: 1,
					v1.ResourcePods:   1,
				},
				{
					v1.ResourceCPU:    2,
					v1.ResourceMemory: 2,
					v1.ResourcePods:   2,
				},
				{
					v1.ResourceCPU:    3,
					v1.ResourceMemory: 3,
					v1.ResourcePods:   3,
				},
			},
			expected: []api.ResourceThresholds{
				{
					v1.ResourceCPU:    51,
					v1.ResourceMemory: 51,
					v1.ResourcePods:   51,
				},
				{
					v1.ResourceCPU:    52,
					v1.ResourceMemory: 52,
					v1.ResourcePods:   52,
				},
				{
					v1.ResourceCPU:    53,
					v1.ResourceMemory: 53,
					v1.ResourcePods:   53,
				},
			},
		},
		{
			name: "deviate with negative values",
			data: api.ResourceThresholds{
				v1.ResourceCPU:    50,
				v1.ResourceMemory: 50,
				v1.ResourcePods:   50,
			},
			deviations: []api.ResourceThresholds{
				{
					v1.ResourceCPU:    -2,
					v1.ResourceMemory: -2,
					v1.ResourcePods:   -2,
				},
				{
					v1.ResourceCPU:    -1,
					v1.ResourceMemory: -1,
					v1.ResourcePods:   -1,
				},
				{
					v1.ResourceCPU:    0,
					v1.ResourceMemory: 0,
					v1.ResourcePods:   0,
				},
				{
					v1.ResourceCPU:    1,
					v1.ResourceMemory: 1,
					v1.ResourcePods:   1,
				},
				{
					v1.ResourceCPU:    2,
					v1.ResourceMemory: 2,
					v1.ResourcePods:   2,
				},
			},
			expected: []api.ResourceThresholds{
				{
					v1.ResourceCPU:    48,
					v1.ResourceMemory: 48,
					v1.ResourcePods:   48,
				},
				{
					v1.ResourceCPU:    49,
					v1.ResourceMemory: 49,
					v1.ResourcePods:   49,
				},
				{
					v1.ResourceCPU:    50,
					v1.ResourceMemory: 50,
					v1.ResourcePods:   50,
				},
				{
					v1.ResourceCPU:    51,
					v1.ResourceMemory: 51,
					v1.ResourcePods:   51,
				},
				{
					v1.ResourceCPU:    52,
					v1.ResourceMemory: 52,
					v1.ResourcePods:   52,
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result := []api.ResourceThresholds{}
			for _, deviation := range tt.deviations {
				partial := Sum(tt.data, deviation)
				result = append(result, partial)
			}

			if len(result) != len(tt.deviations) {
				t.Fatalf("unexpected result: %v", result)
			}
			if !reflect.DeepEqual(result, tt.expected) {
				fmt.Printf("%T, %T\n", result, tt.expected)
				t.Fatalf("unexpected result: %v", result)
			}
		})
	}
}

func TestClamp(t *testing.T) {
	for _, tt := range []struct {
		name     string
		data     []api.ResourceThresholds
		minimum  api.Percentage
		maximum  api.Percentage
		expected []api.ResourceThresholds
	}{
		{
			name: "all over the limit",
			data: []api.ResourceThresholds{
				{
					v1.ResourceCPU:    50,
					v1.ResourceMemory: 50,
					v1.ResourcePods:   50,
				},
			},
			minimum: 10,
			maximum: 20,
			expected: []api.ResourceThresholds{
				{
					v1.ResourceCPU:    20,
					v1.ResourceMemory: 20,
					v1.ResourcePods:   20,
				},
			},
		},
		{
			name: "some over some below the limits",
			data: []api.ResourceThresholds{
				{
					v1.ResourceCPU:    7,
					v1.ResourceMemory: 8,
					v1.ResourcePods:   88,
				},
			},
			minimum: 10,
			maximum: 20,
			expected: []api.ResourceThresholds{
				{
					v1.ResourceCPU:    10,
					v1.ResourceMemory: 10,
					v1.ResourcePods:   20,
				},
			},
		},
		{
			name: "all within the limits",
			data: []api.ResourceThresholds{
				{
					v1.ResourceCPU:    15,
					v1.ResourceMemory: 15,
					v1.ResourcePods:   15,
				},
			},
			minimum: 10,
			maximum: 20,
			expected: []api.ResourceThresholds{
				{
					v1.ResourceCPU:    15,
					v1.ResourceMemory: 15,
					v1.ResourcePods:   15,
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fn := func(thresholds api.ResourceThresholds) api.ResourceThresholds {
				return Clamp(thresholds, tt.minimum, tt.maximum)
			}
			result := Map(tt.data, fn)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Fatalf("unexpected result: %v", result)
			}
		})
	}
}
