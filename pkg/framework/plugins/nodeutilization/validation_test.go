/*
Copyright 2022 The Kubernetes Authors.

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
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"sigs.k8s.io/descheduler/pkg/api"
)

func TestValidateLowNodeUtilizationPluginConfig(t *testing.T) {
	extendedResource := v1.ResourceName("example.com/foo")
	tests := []struct {
		name    string
		args    *LowNodeUtilizationArgs
		errInfo error
	}{
		{
			name: "passing invalid thresholds",
			args: &LowNodeUtilizationArgs{
				Thresholds: api.ResourceThresholds{
					v1.ResourceCPU:    20,
					v1.ResourceMemory: 120,
				},
				TargetThresholds: api.ResourceThresholds{
					v1.ResourceCPU:    80,
					v1.ResourceMemory: 80,
				},
			},
			errInfo: fmt.Errorf("thresholds config is not valid: %v", fmt.Errorf(
				"%v threshold not in [%v, %v] range", v1.ResourceMemory, MinResourcePercentage, MaxResourcePercentage)),
		},
		{
			name: "thresholds and targetThresholds configured different num of resources",
			args: &LowNodeUtilizationArgs{
				Thresholds: api.ResourceThresholds{
					v1.ResourceCPU:    20,
					v1.ResourceMemory: 20,
				},
				TargetThresholds: api.ResourceThresholds{
					v1.ResourceCPU:    80,
					v1.ResourceMemory: 80,
					v1.ResourcePods:   80,
				},
			},
			errInfo: fmt.Errorf("thresholds and targetThresholds configured different resources"),
		},
		{
			name: "thresholds and targetThresholds configured different resources",
			args: &LowNodeUtilizationArgs{
				Thresholds: api.ResourceThresholds{
					v1.ResourceCPU:    20,
					v1.ResourceMemory: 20,
				},
				TargetThresholds: api.ResourceThresholds{
					v1.ResourceCPU:  80,
					v1.ResourcePods: 80,
				},
			},
			errInfo: fmt.Errorf("thresholds and targetThresholds configured different resources"),
		},
		{
			name: "thresholds' CPU config value is greater than targetThresholds'",
			args: &LowNodeUtilizationArgs{
				Thresholds: api.ResourceThresholds{
					v1.ResourceCPU:    90,
					v1.ResourceMemory: 20,
				},
				TargetThresholds: api.ResourceThresholds{
					v1.ResourceCPU:    80,
					v1.ResourceMemory: 80,
				},
			},
			errInfo: fmt.Errorf("thresholds' %v percentage is greater than targetThresholds'", v1.ResourceCPU),
		},
		{
			name: "only thresholds configured extended resource",
			args: &LowNodeUtilizationArgs{
				Thresholds: api.ResourceThresholds{
					v1.ResourceCPU:    20,
					v1.ResourceMemory: 20,
					extendedResource:  20,
				},
				TargetThresholds: api.ResourceThresholds{
					v1.ResourceCPU:    80,
					v1.ResourceMemory: 80,
				},
			},
			errInfo: fmt.Errorf("thresholds and targetThresholds configured different resources"),
		},
		{
			name: "only targetThresholds configured extended resource",
			args: &LowNodeUtilizationArgs{
				Thresholds: api.ResourceThresholds{
					v1.ResourceCPU:    20,
					v1.ResourceMemory: 20,
				},
				TargetThresholds: api.ResourceThresholds{
					v1.ResourceCPU:    80,
					v1.ResourceMemory: 80,
					extendedResource:  80,
				},
			},
			errInfo: fmt.Errorf("thresholds and targetThresholds configured different resources"),
		},
		{
			name: "thresholds and targetThresholds configured different extended resources",
			args: &LowNodeUtilizationArgs{
				Thresholds: api.ResourceThresholds{
					v1.ResourceCPU:    20,
					v1.ResourceMemory: 20,
					extendedResource:  20,
				},
				TargetThresholds: api.ResourceThresholds{
					v1.ResourceCPU:    80,
					v1.ResourceMemory: 80,
					"example.com/bar": 80,
				},
			},
			errInfo: fmt.Errorf("thresholds and targetThresholds configured different resources"),
		},
		{
			name: "thresholds' extended resource config value is greater than targetThresholds'",
			args: &LowNodeUtilizationArgs{
				Thresholds: api.ResourceThresholds{
					v1.ResourceCPU:    20,
					v1.ResourceMemory: 20,
					extendedResource:  90,
				},
				TargetThresholds: api.ResourceThresholds{
					v1.ResourceCPU:    80,
					v1.ResourceMemory: 80,
					extendedResource:  20,
				},
			},
			errInfo: fmt.Errorf("thresholds' %v percentage is greater than targetThresholds'", extendedResource),
		},
		{
			name: "passing valid plugin config",
			args: &LowNodeUtilizationArgs{
				Thresholds: api.ResourceThresholds{
					v1.ResourceCPU:    20,
					v1.ResourceMemory: 20,
				},
				TargetThresholds: api.ResourceThresholds{
					v1.ResourceCPU:    80,
					v1.ResourceMemory: 80,
				},
			},
			errInfo: nil,
		},
		{
			name: "passing valid plugin config with extended resource",
			args: &LowNodeUtilizationArgs{
				Thresholds: api.ResourceThresholds{
					v1.ResourceCPU:    20,
					v1.ResourceMemory: 20,
					extendedResource:  20,
				},
				TargetThresholds: api.ResourceThresholds{
					v1.ResourceCPU:    80,
					v1.ResourceMemory: 80,
					extendedResource:  80,
				},
			},
			errInfo: nil,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			validateErr := ValidateLowNodeUtilizationArgs(runtime.Object(testCase.args))
			if validateErr == nil || testCase.errInfo == nil {
				if validateErr != testCase.errInfo {
					t.Errorf("expected validity of plugin config: %v but got %v instead", testCase.errInfo, validateErr)
				}
			} else if validateErr.Error() != testCase.errInfo.Error() {
				t.Errorf("expected validity of plugin config: %v but got %v instead", testCase.errInfo, validateErr)
			}
		})
	}
}
