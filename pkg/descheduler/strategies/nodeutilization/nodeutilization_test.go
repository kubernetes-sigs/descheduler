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
	"fmt"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"math"
	"sigs.k8s.io/descheduler/pkg/api"
	"testing"
)

var (
	lowPriority      = int32(0)
	highPriority     = int32(10000)
	extendedResource = v1.ResourceName("example.com/foo")
)

func TestValidateThresholds(t *testing.T) {
	tests := []struct {
		name    string
		input   api.ResourceThresholds
		errInfo error
	}{
		{
			name:    "passing nil map for threshold",
			input:   nil,
			errInfo: fmt.Errorf("no resource threshold is configured"),
		},
		{
			name:    "passing no threshold",
			input:   api.ResourceThresholds{},
			errInfo: fmt.Errorf("no resource threshold is configured"),
		},
		{
			name: "passing extended resource name other than cpu/memory/pods",
			input: api.ResourceThresholds{
				v1.ResourceCPU:   40,
				extendedResource: 50,
			},
			errInfo: nil,
		},
		{
			name: "passing invalid resource value",
			input: api.ResourceThresholds{
				v1.ResourceCPU:    110,
				v1.ResourceMemory: 80,
			},
			errInfo: fmt.Errorf("%v threshold not in [%v, %v] range", v1.ResourceCPU, MinResourcePercentage, MaxResourcePercentage),
		},
		{
			name: "passing a valid threshold with max and min resource value",
			input: api.ResourceThresholds{
				v1.ResourceCPU:    100,
				v1.ResourceMemory: 0,
			},
			errInfo: nil,
		},
		{
			name: "passing a valid threshold with only cpu",
			input: api.ResourceThresholds{
				v1.ResourceCPU: 80,
			},
			errInfo: nil,
		},
		{
			name: "passing a valid threshold with cpu, memory and pods",
			input: api.ResourceThresholds{
				v1.ResourceCPU:    20,
				v1.ResourceMemory: 30,
				v1.ResourcePods:   40,
			},
			errInfo: nil,
		},
		{
			name: "passing a valid threshold with only extended resource",
			input: api.ResourceThresholds{
				extendedResource: 80,
			},
			errInfo: nil,
		},
		{
			name: "passing a valid threshold with cpu, memory, pods and extended resource",
			input: api.ResourceThresholds{
				v1.ResourceCPU:    20,
				v1.ResourceMemory: 30,
				v1.ResourcePods:   40,
				extendedResource:  50,
			},
			errInfo: nil,
		},
	}

	for _, test := range tests {
		validateErr := validateThresholds(test.input)

		if validateErr == nil || test.errInfo == nil {
			if validateErr != test.errInfo {
				t.Errorf("expected validity of threshold: %#v to be %v but got %v instead", test.input, test.errInfo, validateErr)
			}
		} else if validateErr.Error() != test.errInfo.Error() {
			t.Errorf("expected validity of threshold: %#v to be %v but got %v instead", test.input, test.errInfo, validateErr)
		}
	}
}

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
		usage: map[v1.ResourceName]*resource.Quantity{
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
