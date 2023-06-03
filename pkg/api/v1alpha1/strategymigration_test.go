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

package v1alpha1

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	utilpointer "k8s.io/utils/pointer"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/nodeutilization"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/podlifetime"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removeduplicates"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removefailedpods"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodshavingtoomanyrestarts"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodsviolatinginterpodantiaffinity"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodsviolatingnodeaffinity"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodsviolatingnodetaints"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodsviolatingtopologyspreadconstraint"
)

func TestStrategyParamsToPluginArgsRemovePodsViolatingNodeTaints(t *testing.T) {
	strategyName := "RemovePodsViolatingNodeTaints"
	type testCase struct {
		description string
		params      *StrategyParameters
		err         error
		result      *api.PluginConfig
	}
	testCases := []testCase{
		{
			description: "wire in all valid parameters",
			params: &StrategyParameters{
				ExcludedTaints: []string{
					"dedicated=special-user",
					"reserved",
				},
				ThresholdPriority: utilpointer.Int32(100),
				Namespaces: &Namespaces{
					Exclude: []string{"test1"},
				},
			},
			err: nil,
			result: &api.PluginConfig{
				Name: removepodsviolatingnodetaints.PluginName,
				Args: &removepodsviolatingnodetaints.RemovePodsViolatingNodeTaintsArgs{
					Namespaces: &api.Namespaces{
						Exclude: []string{"test1"},
					},
					ExcludedTaints: []string{"dedicated=special-user", "reserved"},
				},
			},
		},
		{
			description: "invalid params namespaces",
			params: &StrategyParameters{
				Namespaces: &Namespaces{
					Exclude: []string{"test1"},
					Include: []string{"test2"},
				},
			},
			err:    fmt.Errorf("strategy \"%s\" param validation failed: only one of Include/Exclude namespaces can be set", strategyName),
			result: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			var result *api.PluginConfig
			var err error
			if pcFnc, exists := StrategyParamsToPluginArgs[strategyName]; exists {
				result, err = pcFnc(tc.params)
			}
			if err != nil {
				if err.Error() != tc.err.Error() {
					t.Errorf("unexpected error: %s", err.Error())
				}
			}
			if err == nil {
				// sort to easily compare deepequality
				diff := cmp.Diff(tc.result, result)
				if diff != "" {
					t.Errorf("test '%s' failed. Results are not deep equal. mismatch (-want +got):\n%s", tc.description, diff)
				}
			}
		})
	}
}

func TestStrategyParamsToPluginArgsRemoveFailedPods(t *testing.T) {
	strategyName := "RemoveFailedPods"
	type testCase struct {
		description string
		params      *StrategyParameters
		err         error
		result      *api.PluginConfig
	}
	testCases := []testCase{
		{
			description: "wire in all valid parameters",
			params: &StrategyParameters{
				FailedPods: &FailedPods{
					MinPodLifetimeSeconds:   utilpointer.Uint(3600),
					ExcludeOwnerKinds:       []string{"Job"},
					Reasons:                 []string{"NodeAffinity"},
					IncludingInitContainers: true,
				},
				ThresholdPriority: utilpointer.Int32(100),
				Namespaces: &Namespaces{
					Exclude: []string{"test1"},
				},
			},
			err: nil,
			result: &api.PluginConfig{
				Name: removefailedpods.PluginName,
				Args: &removefailedpods.RemoveFailedPodsArgs{
					ExcludeOwnerKinds:       []string{"Job"},
					MinPodLifetimeSeconds:   utilpointer.Uint(3600),
					Reasons:                 []string{"NodeAffinity"},
					IncludingInitContainers: true,
					Namespaces: &api.Namespaces{
						Exclude: []string{"test1"},
					},
				},
			},
		},
		{
			description: "invalid params namespaces",
			params: &StrategyParameters{
				Namespaces: &Namespaces{
					Exclude: []string{"test1"},
					Include: []string{"test2"},
				},
			},
			err:    fmt.Errorf("strategy \"%s\" param validation failed: only one of Include/Exclude namespaces can be set", strategyName),
			result: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			var result *api.PluginConfig
			var err error
			if pcFnc, exists := StrategyParamsToPluginArgs[strategyName]; exists {
				result, err = pcFnc(tc.params)
			}
			if err != nil {
				if err.Error() != tc.err.Error() {
					t.Errorf("unexpected error: %s", err.Error())
				}
			}
			if err == nil {
				// sort to easily compare deepequality
				diff := cmp.Diff(tc.result, result)
				if diff != "" {
					t.Errorf("test '%s' failed. Results are not deep equal. mismatch (-want +got):\n%s", tc.description, diff)
				}
			}
		})
	}
}

func TestStrategyParamsToPluginArgsRemovePodsViolatingNodeAffinity(t *testing.T) {
	strategyName := "RemovePodsViolatingNodeAffinity"
	type testCase struct {
		description string
		params      *StrategyParameters
		err         error
		result      *api.PluginConfig
	}
	testCases := []testCase{
		{
			description: "wire in all valid parameters",
			params: &StrategyParameters{
				NodeAffinityType:  []string{"requiredDuringSchedulingIgnoredDuringExecution"},
				ThresholdPriority: utilpointer.Int32(100),
				Namespaces: &Namespaces{
					Exclude: []string{"test1"},
				},
			},
			err: nil,
			result: &api.PluginConfig{
				Name: removepodsviolatingnodeaffinity.PluginName,
				Args: &removepodsviolatingnodeaffinity.RemovePodsViolatingNodeAffinityArgs{
					NodeAffinityType: []string{"requiredDuringSchedulingIgnoredDuringExecution"},
					Namespaces: &api.Namespaces{
						Exclude: []string{"test1"},
					},
				},
			},
		},
		{
			description: "invalid params, not setting nodeaffinity type",
			params:      &StrategyParameters{},
			err:         fmt.Errorf("strategy \"%s\" param validation failed: nodeAffinityType needs to be set", strategyName),
			result:      nil,
		},
		{
			description: "invalid params namespaces",
			params: &StrategyParameters{
				NodeAffinityType: []string{"requiredDuringSchedulingIgnoredDuringExecution"},
				Namespaces: &Namespaces{
					Exclude: []string{"test1"},
					Include: []string{"test2"},
				},
			},
			err:    fmt.Errorf("strategy \"%s\" param validation failed: only one of Include/Exclude namespaces can be set", strategyName),
			result: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			var result *api.PluginConfig
			var err error
			if pcFnc, exists := StrategyParamsToPluginArgs[strategyName]; exists {
				result, err = pcFnc(tc.params)
			}
			if err != nil {
				if err.Error() != tc.err.Error() {
					t.Errorf("unexpected error: %s", err.Error())
				}
			}
			if err == nil {
				// sort to easily compare deepequality
				diff := cmp.Diff(tc.result, result)
				if diff != "" {
					t.Errorf("test '%s' failed. Results are not deep equal. mismatch (-want +got):\n%s", tc.description, diff)
				}
			}
		})
	}
}

func TestStrategyParamsToPluginArgsRemovePodsViolatingInterPodAntiAffinity(t *testing.T) {
	strategyName := "RemovePodsViolatingInterPodAntiAffinity"
	type testCase struct {
		description string
		params      *StrategyParameters
		err         error
		result      *api.PluginConfig
	}
	testCases := []testCase{
		{
			description: "wire in all valid parameters",
			params: &StrategyParameters{
				ThresholdPriority: utilpointer.Int32(100),
				Namespaces: &Namespaces{
					Exclude: []string{"test1"},
				},
			},
			err: nil,
			result: &api.PluginConfig{
				Name: removepodsviolatinginterpodantiaffinity.PluginName,
				Args: &removepodsviolatinginterpodantiaffinity.RemovePodsViolatingInterPodAntiAffinityArgs{
					Namespaces: &api.Namespaces{
						Exclude: []string{"test1"},
					},
				},
			},
		},
		{
			description: "invalid params namespaces",
			params: &StrategyParameters{
				Namespaces: &Namespaces{
					Exclude: []string{"test1"},
					Include: []string{"test2"},
				},
			},
			err:    fmt.Errorf("strategy \"%s\" param validation failed: only one of Include/Exclude namespaces can be set", strategyName),
			result: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			var result *api.PluginConfig
			var err error
			if pcFnc, exists := StrategyParamsToPluginArgs[strategyName]; exists {
				result, err = pcFnc(tc.params)
			}
			if err != nil {
				if err.Error() != tc.err.Error() {
					t.Errorf("unexpected error: %s", err.Error())
				}
			}
			if err == nil {
				// sort to easily compare deepequality
				diff := cmp.Diff(tc.result, result)
				if diff != "" {
					t.Errorf("test '%s' failed. Results are not deep equal. mismatch (-want +got):\n%s", tc.description, diff)
				}
			}
		})
	}
}

func TestStrategyParamsToPluginArgsRemovePodsHavingTooManyRestarts(t *testing.T) {
	strategyName := "RemovePodsHavingTooManyRestarts"
	type testCase struct {
		description string
		params      *StrategyParameters
		err         error
		result      *api.PluginConfig
	}
	testCases := []testCase{
		{
			description: "wire in all valid parameters",
			params: &StrategyParameters{
				PodsHavingTooManyRestarts: &PodsHavingTooManyRestarts{
					PodRestartThreshold:     100,
					IncludingInitContainers: true,
				},
				ThresholdPriority: utilpointer.Int32(100),
				Namespaces: &Namespaces{
					Exclude: []string{"test1"},
				},
			},
			err: nil,
			result: &api.PluginConfig{
				Name: removepodshavingtoomanyrestarts.PluginName,
				Args: &removepodshavingtoomanyrestarts.RemovePodsHavingTooManyRestartsArgs{
					PodRestartThreshold:     100,
					IncludingInitContainers: true,
					Namespaces: &api.Namespaces{
						Exclude: []string{"test1"},
					},
				},
			},
		},
		{
			description: "invalid params namespaces",
			params: &StrategyParameters{
				Namespaces: &Namespaces{
					Exclude: []string{"test1"},
					Include: []string{"test2"},
				},
			},
			err:    fmt.Errorf("strategy \"%s\" param validation failed: only one of Include/Exclude namespaces can be set", strategyName),
			result: nil,
		},
		{
			description: "invalid params restart threshold",
			params: &StrategyParameters{
				PodsHavingTooManyRestarts: &PodsHavingTooManyRestarts{
					PodRestartThreshold: 0,
				},
			},
			err:    fmt.Errorf("strategy \"%s\" param validation failed: invalid PodsHavingTooManyRestarts threshold", strategyName),
			result: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			var result *api.PluginConfig
			var err error
			if pcFnc, exists := StrategyParamsToPluginArgs[strategyName]; exists {
				result, err = pcFnc(tc.params)
			}
			if err != nil {
				if err.Error() != tc.err.Error() {
					t.Errorf("unexpected error: %s", err.Error())
				}
			}
			if err == nil {
				// sort to easily compare deepequality
				diff := cmp.Diff(tc.result, result)
				if diff != "" {
					t.Errorf("test '%s' failed. Results are not deep equal. mismatch (-want +got):\n%s", tc.description, diff)
				}
			}
		})
	}
}

func TestStrategyParamsToPluginArgsPodLifeTime(t *testing.T) {
	strategyName := "PodLifeTime"
	type testCase struct {
		description string
		params      *StrategyParameters
		err         error
		result      *api.PluginConfig
	}
	testCases := []testCase{
		{
			description: "wire in all valid parameters",
			params: &StrategyParameters{
				PodLifeTime: &PodLifeTime{
					MaxPodLifeTimeSeconds: utilpointer.Uint(86400),
					States: []string{
						"Pending",
						"PodInitializing",
					},
				},
				ThresholdPriority: utilpointer.Int32(100),
				Namespaces: &Namespaces{
					Exclude: []string{"test1"},
				},
			},
			err: nil,
			result: &api.PluginConfig{
				Name: podlifetime.PluginName,
				Args: &podlifetime.PodLifeTimeArgs{
					MaxPodLifeTimeSeconds: utilpointer.Uint(86400),
					States: []string{
						"Pending",
						"PodInitializing",
					},
					Namespaces: &api.Namespaces{
						Exclude: []string{"test1"},
					},
				},
			},
		},
		{
			description: "invalid params namespaces",
			params: &StrategyParameters{
				PodLifeTime: &PodLifeTime{
					MaxPodLifeTimeSeconds: utilpointer.Uint(86400),
				},
				Namespaces: &Namespaces{
					Exclude: []string{"test1"},
					Include: []string{"test2"},
				},
			},
			err:    fmt.Errorf("strategy \"%s\" param validation failed: only one of Include/Exclude namespaces can be set", strategyName),
			result: nil,
		},
		{
			description: "invalid params MaxPodLifeTimeSeconds not set",
			params: &StrategyParameters{
				PodLifeTime: &PodLifeTime{},
			},
			err:    fmt.Errorf("strategy \"%s\" param validation failed: MaxPodLifeTimeSeconds not set", strategyName),
			result: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			var result *api.PluginConfig
			var err error
			if pcFnc, exists := StrategyParamsToPluginArgs[strategyName]; exists {
				result, err = pcFnc(tc.params)
			}
			if err != nil {
				if err.Error() != tc.err.Error() {
					t.Errorf("unexpected error: %s", err.Error())
				}
			}
			if err == nil {
				// sort to easily compare deepequality
				diff := cmp.Diff(tc.result, result)
				if diff != "" {
					t.Errorf("test '%s' failed. Results are not deep equal. mismatch (-want +got):\n%s", tc.description, diff)
				}
			}
		})
	}
}

func TestStrategyParamsToPluginArgsRemoveDuplicates(t *testing.T) {
	strategyName := "RemoveDuplicates"
	type testCase struct {
		description string
		params      *StrategyParameters
		err         error
		result      *api.PluginConfig
	}
	testCases := []testCase{
		{
			description: "wire in all valid parameters",
			params: &StrategyParameters{
				RemoveDuplicates: &RemoveDuplicates{
					ExcludeOwnerKinds: []string{"ReplicaSet"},
				},
				ThresholdPriority: utilpointer.Int32(100),
				Namespaces: &Namespaces{
					Exclude: []string{"test1"},
				},
			},
			err: nil,
			result: &api.PluginConfig{
				Name: removeduplicates.PluginName,
				Args: &removeduplicates.RemoveDuplicatesArgs{
					ExcludeOwnerKinds: []string{"ReplicaSet"},
					Namespaces: &api.Namespaces{
						Exclude: []string{"test1"},
					},
				},
			},
		},
		{
			description: "invalid params namespaces",
			params: &StrategyParameters{
				PodLifeTime: &PodLifeTime{
					MaxPodLifeTimeSeconds: utilpointer.Uint(86400),
				},
				Namespaces: &Namespaces{
					Exclude: []string{"test1"},
					Include: []string{"test2"},
				},
			},
			err:    fmt.Errorf("strategy \"%s\" param validation failed: only one of Include/Exclude namespaces can be set", strategyName),
			result: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			var result *api.PluginConfig
			var err error
			if pcFnc, exists := StrategyParamsToPluginArgs[strategyName]; exists {
				result, err = pcFnc(tc.params)
			}
			if err != nil {
				if err.Error() != tc.err.Error() {
					t.Errorf("unexpected error: %s", err.Error())
				}
			}
			if err == nil {
				// sort to easily compare deepequality
				diff := cmp.Diff(tc.result, result)
				if diff != "" {
					t.Errorf("test '%s' failed. Results are not deep equal. mismatch (-want +got):\n%s", tc.description, diff)
				}
			}
		})
	}
}

func TestStrategyParamsToPluginArgsRemovePodsViolatingTopologySpreadConstraint(t *testing.T) {
	strategyName := "RemovePodsViolatingTopologySpreadConstraint"
	type testCase struct {
		description string
		params      *StrategyParameters
		err         error
		result      *api.PluginConfig
	}
	testCases := []testCase{
		{
			description: "wire in all valid parameters",
			params: &StrategyParameters{
				IncludeSoftConstraints: true,
				ThresholdPriority:      utilpointer.Int32(100),
				Namespaces: &Namespaces{
					Exclude: []string{"test1"},
				},
			},
			err: nil,
			result: &api.PluginConfig{
				Name: removepodsviolatingtopologyspreadconstraint.PluginName,
				Args: &removepodsviolatingtopologyspreadconstraint.RemovePodsViolatingTopologySpreadConstraintArgs{
					IncludeSoftConstraints: true,
					TopologyBalanceNodeFit: utilpointer.Bool(true),
					Namespaces: &api.Namespaces{
						Exclude: []string{"test1"},
					},
				},
			},
		},
		{
			description: "invalid params namespaces",
			params: &StrategyParameters{
				Namespaces: &Namespaces{
					Exclude: []string{"test1"},
					Include: []string{"test2"},
				},
			},
			err:    fmt.Errorf("strategy \"%s\" param validation failed: only one of Include/Exclude namespaces can be set", strategyName),
			result: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			var result *api.PluginConfig
			var err error
			if pcFnc, exists := StrategyParamsToPluginArgs[strategyName]; exists {
				result, err = pcFnc(tc.params)
			}
			if err != nil {
				if err.Error() != tc.err.Error() {
					t.Errorf("unexpected error: %s", err.Error())
				}
			}
			if err == nil {
				// sort to easily compare deepequality
				diff := cmp.Diff(tc.result, result)
				if diff != "" {
					t.Errorf("test '%s' failed. Results are not deep equal. mismatch (-want +got):\n%s", tc.description, diff)
				}
			}
		})
	}
}

func TestStrategyParamsToPluginArgsHighNodeUtilization(t *testing.T) {
	strategyName := "HighNodeUtilization"
	type testCase struct {
		description string
		params      *StrategyParameters
		err         error
		result      *api.PluginConfig
	}
	testCases := []testCase{
		{
			description: "wire in all valid parameters",
			params: &StrategyParameters{
				NodeResourceUtilizationThresholds: &NodeResourceUtilizationThresholds{
					NumberOfNodes: 3,
					Thresholds: ResourceThresholds{
						"cpu":    Percentage(20),
						"memory": Percentage(20),
						"pods":   Percentage(20),
					},
				},
				ThresholdPriority: utilpointer.Int32(100),
				Namespaces: &Namespaces{
					Exclude: []string{"test1"},
				},
			},
			err: nil,
			result: &api.PluginConfig{
				Name: nodeutilization.HighNodeUtilizationPluginName,
				Args: &nodeutilization.HighNodeUtilizationArgs{
					Thresholds: api.ResourceThresholds{
						"cpu":    api.Percentage(20),
						"memory": api.Percentage(20),
						"pods":   api.Percentage(20),
					},
					NumberOfNodes: 3,
					EvictableNamespaces: &api.Namespaces{
						Exclude: []string{"test1"},
					},
				},
			},
		},
		{
			description: "invalid params namespaces",
			params: &StrategyParameters{
				NodeResourceUtilizationThresholds: &NodeResourceUtilizationThresholds{
					NumberOfNodes: 3,
					Thresholds: ResourceThresholds{
						"cpu":    Percentage(20),
						"memory": Percentage(20),
						"pods":   Percentage(20),
					},
				},
				Namespaces: &Namespaces{
					Include: []string{"test2"},
				},
			},
			err:    fmt.Errorf("strategy \"%s\" param validation failed: only Exclude namespaces can be set, inclusion is not supported", strategyName),
			result: nil,
		},
		{
			description: "invalid params nil ResourceThresholds",
			params: &StrategyParameters{
				NodeResourceUtilizationThresholds: &NodeResourceUtilizationThresholds{
					NumberOfNodes: 3,
				},
			},
			err:    fmt.Errorf("strategy \"%s\" param validation failed: no resource threshold is configured", strategyName),
			result: nil,
		},
		{
			description: "invalid params out of bounds threshold",
			params: &StrategyParameters{
				NodeResourceUtilizationThresholds: &NodeResourceUtilizationThresholds{
					NumberOfNodes: 3,
					Thresholds: ResourceThresholds{
						"cpu": Percentage(150),
					},
				},
			},
			err:    fmt.Errorf("strategy \"%s\" param validation failed: cpu threshold not in [0, 100] range", strategyName),
			result: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			var result *api.PluginConfig
			var err error
			if pcFnc, exists := StrategyParamsToPluginArgs[strategyName]; exists {
				result, err = pcFnc(tc.params)
			}
			if err != nil {
				if err.Error() != tc.err.Error() {
					t.Errorf("unexpected error: %s", err.Error())
				}
			}
			if err == nil {
				// sort to easily compare deepequality
				diff := cmp.Diff(tc.result, result)
				if diff != "" {
					t.Errorf("test '%s' failed. Results are not deep equal. mismatch (-want +got):\n%s", tc.description, diff)
				}
			}
		})
	}
}

func TestStrategyParamsToPluginArgsLowNodeUtilization(t *testing.T) {
	strategyName := "LowNodeUtilization"
	type testCase struct {
		description string
		params      *StrategyParameters
		err         error
		result      *api.PluginConfig
	}
	testCases := []testCase{
		{
			description: "wire in all valid parameters",
			params: &StrategyParameters{
				NodeResourceUtilizationThresholds: &NodeResourceUtilizationThresholds{
					NumberOfNodes: 3,
					Thresholds: ResourceThresholds{
						"cpu":    Percentage(20),
						"memory": Percentage(20),
						"pods":   Percentage(20),
					},
					TargetThresholds: ResourceThresholds{
						"cpu":    Percentage(50),
						"memory": Percentage(50),
						"pods":   Percentage(50),
					},
					UseDeviationThresholds: true,
				},
				ThresholdPriority: utilpointer.Int32(100),
				Namespaces: &Namespaces{
					Exclude: []string{"test1"},
				},
			},
			err: nil,
			result: &api.PluginConfig{
				Name: nodeutilization.LowNodeUtilizationPluginName,
				Args: &nodeutilization.LowNodeUtilizationArgs{
					Thresholds: api.ResourceThresholds{
						"cpu":    api.Percentage(20),
						"memory": api.Percentage(20),
						"pods":   api.Percentage(20),
					},
					TargetThresholds: api.ResourceThresholds{
						"cpu":    api.Percentage(50),
						"memory": api.Percentage(50),
						"pods":   api.Percentage(50),
					},
					UseDeviationThresholds: true,
					NumberOfNodes:          3,
					EvictableNamespaces: &api.Namespaces{
						Exclude: []string{"test1"},
					},
				},
			},
		},
		{
			description: "invalid params namespaces",
			params: &StrategyParameters{
				NodeResourceUtilizationThresholds: &NodeResourceUtilizationThresholds{
					NumberOfNodes: 3,
					Thresholds: ResourceThresholds{
						"cpu":    Percentage(20),
						"memory": Percentage(20),
						"pods":   Percentage(20),
					},
					TargetThresholds: ResourceThresholds{
						"cpu":    Percentage(50),
						"memory": Percentage(50),
						"pods":   Percentage(50),
					},
					UseDeviationThresholds: true,
				},
				Namespaces: &Namespaces{
					Include: []string{"test2"},
				},
			},
			err:    fmt.Errorf("strategy \"%s\" param validation failed: only Exclude namespaces can be set, inclusion is not supported", strategyName),
			result: nil,
		},
		{
			description: "invalid params nil ResourceThresholds",
			params: &StrategyParameters{
				NodeResourceUtilizationThresholds: &NodeResourceUtilizationThresholds{
					NumberOfNodes: 3,
				},
			},
			err:    fmt.Errorf("strategy \"%s\" param validation failed: thresholds config is not valid: no resource threshold is configured", strategyName),
			result: nil,
		},
		{
			description: "invalid params out of bounds threshold",
			params: &StrategyParameters{
				NodeResourceUtilizationThresholds: &NodeResourceUtilizationThresholds{
					NumberOfNodes: 3,
					Thresholds: ResourceThresholds{
						"cpu": Percentage(150),
					},
				},
			},
			err:    fmt.Errorf("strategy \"%s\" param validation failed: thresholds config is not valid: cpu threshold not in [0, 100] range", strategyName),
			result: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			var result *api.PluginConfig
			var err error
			if pcFnc, exists := StrategyParamsToPluginArgs[strategyName]; exists {
				result, err = pcFnc(tc.params)
			}
			if err != nil {
				if err.Error() != tc.err.Error() {
					t.Errorf("unexpected error: %s", err.Error())
				}
			}
			if err == nil {
				// sort to easily compare deepequality
				diff := cmp.Diff(tc.result, result)
				if diff != "" {
					t.Errorf("test '%s' failed. Results are not deep equal. mismatch (-want +got):\n%s", tc.description, diff)
				}
			}
		})
	}
}
