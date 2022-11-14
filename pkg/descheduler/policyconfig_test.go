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

package descheduler

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	utilpointer "k8s.io/utils/pointer"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/api/v1alpha1"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/nodeutilization"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removeduplicates"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removefailedpods"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodshavingtoomanyrestarts"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodsviolatinginterpodantiaffinity"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodsviolatingnodeaffinity"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodsviolatingnodetaints"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodsviolatingtopologyspreadconstraint"
	"sigs.k8s.io/descheduler/pkg/utils"
)

func TestV1alpha1ToV1alpha2(t *testing.T) {
	defaultEvictorPluginConfig := api.PluginConfig{
		Name: defaultevictor.PluginName,
		Args: &defaultevictor.DefaultEvictorArgs{
			PriorityThreshold: &api.PriorityThreshold{
				Value: utilpointer.Int32(utils.SystemCriticalPriority),
			},
		},
	}
	defaultEvictorPluginSet := api.PluginSet{
		Enabled: []string{defaultevictor.PluginName},
	}
	type testCase struct {
		description string
		policy      *v1alpha1.DeschedulerPolicy
		err         error
		result      *api.DeschedulerPolicy
	}
	testCases := []testCase{
		{
			description: "RemoveFailedPods enabled, LowNodeUtilization disabled strategies to profile",
			policy: &v1alpha1.DeschedulerPolicy{
				Strategies: v1alpha1.StrategyList{
					removeduplicates.PluginName: v1alpha1.DeschedulerStrategy{
						Enabled: true,
						Params: &v1alpha1.StrategyParameters{
							Namespaces: &v1alpha1.Namespaces{
								Exclude: []string{
									"test2",
								},
							},
						},
					},
					nodeutilization.LowNodeUtilizationPluginName: v1alpha1.DeschedulerStrategy{
						Enabled: false,
						Params: &v1alpha1.StrategyParameters{
							NodeResourceUtilizationThresholds: &v1alpha1.NodeResourceUtilizationThresholds{
								Thresholds: v1alpha1.ResourceThresholds{
									"cpu":    v1alpha1.Percentage(20),
									"memory": v1alpha1.Percentage(20),
									"pods":   v1alpha1.Percentage(20),
								},
								TargetThresholds: v1alpha1.ResourceThresholds{
									"cpu":    v1alpha1.Percentage(50),
									"memory": v1alpha1.Percentage(50),
									"pods":   v1alpha1.Percentage(50),
								},
							},
						},
					},
				},
			},
			result: &api.DeschedulerPolicy{
				Profiles: []api.Profile{
					{
						Name: fmt.Sprintf("strategy-%s-profile", removeduplicates.PluginName),
						PluginConfigs: []api.PluginConfig{
							defaultEvictorPluginConfig,
							{
								Name: removeduplicates.PluginName,
								Args: &removeduplicates.RemoveDuplicatesArgs{
									Namespaces: &api.Namespaces{
										Exclude: []string{
											"test2",
										},
									},
								},
							},
						},
						Plugins: api.Plugins{
							Balance: api.PluginSet{
								Enabled: []string{removeduplicates.PluginName},
							},
							Evict: defaultEvictorPluginSet,
						},
					},
					// Disabled strategy is not generating internal plugin since it is not being used internally currently
					// {
					// 	Name: nodeutilization.LowNodeUtilizationPluginName,
					// 	PluginConfigs: []api.PluginConfig{
					// 		{
					// 			Name: nodeutilization.LowNodeUtilizationPluginName,
					// 			Args: &nodeutilization.LowNodeUtilizationArgs{
					// 				Thresholds: api.ResourceThresholds{
					// 					"cpu":    api.Percentage(20),
					// [...]
					// [...]
					// },
				},
			},
		},
		{
			description: "convert all strategies",
			policy: &v1alpha1.DeschedulerPolicy{
				Strategies: v1alpha1.StrategyList{
					removeduplicates.PluginName: v1alpha1.DeschedulerStrategy{
						Enabled: true,
						Params:  &v1alpha1.StrategyParameters{},
					},
					nodeutilization.LowNodeUtilizationPluginName: v1alpha1.DeschedulerStrategy{
						Enabled: true,
						Params: &v1alpha1.StrategyParameters{
							NodeResourceUtilizationThresholds: &v1alpha1.NodeResourceUtilizationThresholds{
								Thresholds: v1alpha1.ResourceThresholds{
									"cpu":    v1alpha1.Percentage(20),
									"memory": v1alpha1.Percentage(20),
									"pods":   v1alpha1.Percentage(20),
								},
								TargetThresholds: v1alpha1.ResourceThresholds{
									"cpu":    v1alpha1.Percentage(50),
									"memory": v1alpha1.Percentage(50),
									"pods":   v1alpha1.Percentage(50),
								},
							},
						},
					},
					nodeutilization.HighNodeUtilizationPluginName: v1alpha1.DeschedulerStrategy{
						Enabled: true,
						Params: &v1alpha1.StrategyParameters{
							NodeResourceUtilizationThresholds: &v1alpha1.NodeResourceUtilizationThresholds{
								Thresholds: v1alpha1.ResourceThresholds{
									"cpu":    v1alpha1.Percentage(20),
									"memory": v1alpha1.Percentage(20),
									"pods":   v1alpha1.Percentage(20),
								},
							},
						},
					},
					removefailedpods.PluginName: v1alpha1.DeschedulerStrategy{
						Enabled: true,
						Params:  &v1alpha1.StrategyParameters{},
					},
					removepodshavingtoomanyrestarts.PluginName: v1alpha1.DeschedulerStrategy{
						Enabled: true,
						Params: &v1alpha1.StrategyParameters{
							PodsHavingTooManyRestarts: &v1alpha1.PodsHavingTooManyRestarts{
								PodRestartThreshold: 100,
							},
						},
					},
					removepodsviolatinginterpodantiaffinity.PluginName: v1alpha1.DeschedulerStrategy{
						Enabled: true,
						Params:  &v1alpha1.StrategyParameters{},
					},
					removepodsviolatingnodeaffinity.PluginName: v1alpha1.DeschedulerStrategy{
						Enabled: true,
						Params: &v1alpha1.StrategyParameters{
							NodeAffinityType: []string{"requiredDuringSchedulingIgnoredDuringExecution"},
						},
					},
					removepodsviolatingnodetaints.PluginName: v1alpha1.DeschedulerStrategy{
						Enabled: true,
						Params:  &v1alpha1.StrategyParameters{},
					},
					removepodsviolatingtopologyspreadconstraint.PluginName: v1alpha1.DeschedulerStrategy{
						Enabled: true,
						Params:  &v1alpha1.StrategyParameters{},
					},
				},
			},
			result: &api.DeschedulerPolicy{
				Profiles: []api.Profile{
					{
						Name: fmt.Sprintf("strategy-%s-profile", nodeutilization.HighNodeUtilizationPluginName),
						PluginConfigs: []api.PluginConfig{
							defaultEvictorPluginConfig,
							{
								Name: nodeutilization.HighNodeUtilizationPluginName,
								Args: &nodeutilization.HighNodeUtilizationArgs{
									Thresholds: api.ResourceThresholds{
										"cpu":    api.Percentage(20),
										"memory": api.Percentage(20),
										"pods":   api.Percentage(20),
									},
								},
							},
						},
						Plugins: api.Plugins{
							Balance: api.PluginSet{
								Enabled: []string{nodeutilization.HighNodeUtilizationPluginName},
							},
							Evict: defaultEvictorPluginSet,
						},
					},
					{
						Name: fmt.Sprintf("strategy-%s-profile", nodeutilization.LowNodeUtilizationPluginName),
						PluginConfigs: []api.PluginConfig{
							defaultEvictorPluginConfig,
							{
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
								},
							},
						},
						Plugins: api.Plugins{
							Balance: api.PluginSet{
								Enabled: []string{nodeutilization.LowNodeUtilizationPluginName},
							},
							Evict: defaultEvictorPluginSet,
						},
					},
					{
						Name: fmt.Sprintf("strategy-%s-profile", removeduplicates.PluginName),
						PluginConfigs: []api.PluginConfig{
							defaultEvictorPluginConfig,
							{
								Name: removeduplicates.PluginName,
								Args: &removeduplicates.RemoveDuplicatesArgs{},
							},
						},
						Plugins: api.Plugins{
							Balance: api.PluginSet{
								Enabled: []string{removeduplicates.PluginName},
							},
							Evict: defaultEvictorPluginSet,
						},
					},
					{
						Name: fmt.Sprintf("strategy-%s-profile", removefailedpods.PluginName),
						PluginConfigs: []api.PluginConfig{
							defaultEvictorPluginConfig,
							{
								Name: removefailedpods.PluginName,
								Args: &removefailedpods.RemoveFailedPodsArgs{},
							},
						},
						Plugins: api.Plugins{
							Deschedule: api.PluginSet{
								Enabled: []string{removefailedpods.PluginName},
							},
							Evict: defaultEvictorPluginSet,
						},
					},
					{
						Name: fmt.Sprintf("strategy-%s-profile", removepodshavingtoomanyrestarts.PluginName),
						PluginConfigs: []api.PluginConfig{
							defaultEvictorPluginConfig,
							{
								Name: removepodshavingtoomanyrestarts.PluginName,
								Args: &removepodshavingtoomanyrestarts.RemovePodsHavingTooManyRestartsArgs{
									PodRestartThreshold: 100,
								},
							},
						},
						Plugins: api.Plugins{
							Deschedule: api.PluginSet{
								Enabled: []string{removepodshavingtoomanyrestarts.PluginName},
							},
							Evict: defaultEvictorPluginSet,
						},
					},
					{
						Name: fmt.Sprintf("strategy-%s-profile", removepodsviolatinginterpodantiaffinity.PluginName),
						PluginConfigs: []api.PluginConfig{
							defaultEvictorPluginConfig,
							{
								Name: removepodsviolatinginterpodantiaffinity.PluginName,
								Args: &removepodsviolatinginterpodantiaffinity.RemovePodsViolatingInterPodAntiAffinityArgs{},
							},
						},
						Plugins: api.Plugins{
							Deschedule: api.PluginSet{
								Enabled: []string{removepodsviolatinginterpodantiaffinity.PluginName},
							},
							Evict: defaultEvictorPluginSet,
						},
					},
					{
						Name: fmt.Sprintf("strategy-%s-profile", removepodsviolatingnodeaffinity.PluginName),
						PluginConfigs: []api.PluginConfig{
							defaultEvictorPluginConfig,
							{
								Name: removepodsviolatingnodeaffinity.PluginName,
								Args: &removepodsviolatingnodeaffinity.RemovePodsViolatingNodeAffinityArgs{
									NodeAffinityType: []string{"requiredDuringSchedulingIgnoredDuringExecution"},
								},
							},
						},
						Plugins: api.Plugins{
							Deschedule: api.PluginSet{
								Enabled: []string{removepodsviolatingnodeaffinity.PluginName},
							},
							Evict: defaultEvictorPluginSet,
						},
					},
					{
						Name: fmt.Sprintf("strategy-%s-profile", removepodsviolatingnodetaints.PluginName),
						PluginConfigs: []api.PluginConfig{
							defaultEvictorPluginConfig,
							{
								Name: removepodsviolatingnodetaints.PluginName,
								Args: &removepodsviolatingnodetaints.RemovePodsViolatingNodeTaintsArgs{},
							},
						},
						Plugins: api.Plugins{
							Deschedule: api.PluginSet{
								Enabled: []string{removepodsviolatingnodetaints.PluginName},
							},
							Evict: defaultEvictorPluginSet,
						},
					},
					{
						Name: fmt.Sprintf("strategy-%s-profile", removepodsviolatingtopologyspreadconstraint.PluginName),
						PluginConfigs: []api.PluginConfig{
							defaultEvictorPluginConfig,
							{
								Name: removepodsviolatingtopologyspreadconstraint.PluginName,
								Args: &removepodsviolatingtopologyspreadconstraint.RemovePodsViolatingTopologySpreadConstraintArgs{},
							},
						},
						Plugins: api.Plugins{
							Balance: api.PluginSet{
								Enabled: []string{removepodsviolatingtopologyspreadconstraint.PluginName},
							},
							Evict: defaultEvictorPluginSet,
						},
					},
				},
			},
		},
		{
			description: "pass in all params to check args",
			policy: &v1alpha1.DeschedulerPolicy{
				Strategies: v1alpha1.StrategyList{
					removeduplicates.PluginName: v1alpha1.DeschedulerStrategy{
						Enabled: true,
						Params: &v1alpha1.StrategyParameters{
							RemoveDuplicates: &v1alpha1.RemoveDuplicates{
								ExcludeOwnerKinds: []string{"ReplicaSet"},
							},
						},
					},
					nodeutilization.LowNodeUtilizationPluginName: v1alpha1.DeschedulerStrategy{
						Enabled: true,
						Params: &v1alpha1.StrategyParameters{
							NodeResourceUtilizationThresholds: &v1alpha1.NodeResourceUtilizationThresholds{
								Thresholds: v1alpha1.ResourceThresholds{
									"cpu":    v1alpha1.Percentage(20),
									"memory": v1alpha1.Percentage(20),
									"pods":   v1alpha1.Percentage(20),
								},
								TargetThresholds: v1alpha1.ResourceThresholds{
									"cpu":    v1alpha1.Percentage(50),
									"memory": v1alpha1.Percentage(50),
									"pods":   v1alpha1.Percentage(50),
								},
								UseDeviationThresholds: true,
								NumberOfNodes:          3,
							},
						},
					},
					nodeutilization.HighNodeUtilizationPluginName: v1alpha1.DeschedulerStrategy{
						Enabled: true,
						Params: &v1alpha1.StrategyParameters{
							NodeResourceUtilizationThresholds: &v1alpha1.NodeResourceUtilizationThresholds{
								Thresholds: v1alpha1.ResourceThresholds{
									"cpu":    v1alpha1.Percentage(20),
									"memory": v1alpha1.Percentage(20),
									"pods":   v1alpha1.Percentage(20),
								},
							},
						},
					},
					removefailedpods.PluginName: v1alpha1.DeschedulerStrategy{
						Enabled: true,
						Params: &v1alpha1.StrategyParameters{
							FailedPods: &v1alpha1.FailedPods{
								MinPodLifetimeSeconds:   utilpointer.Uint(3600),
								ExcludeOwnerKinds:       []string{"Job"},
								Reasons:                 []string{"NodeAffinity"},
								IncludingInitContainers: true,
							},
						},
					},
					removepodshavingtoomanyrestarts.PluginName: v1alpha1.DeschedulerStrategy{
						Enabled: true,
						Params: &v1alpha1.StrategyParameters{
							PodsHavingTooManyRestarts: &v1alpha1.PodsHavingTooManyRestarts{
								PodRestartThreshold:     100,
								IncludingInitContainers: true,
							},
						},
					},
					removepodsviolatinginterpodantiaffinity.PluginName: v1alpha1.DeschedulerStrategy{
						Enabled: true,
						Params:  &v1alpha1.StrategyParameters{},
					},
					removepodsviolatingnodeaffinity.PluginName: v1alpha1.DeschedulerStrategy{
						Enabled: true,
						Params: &v1alpha1.StrategyParameters{
							NodeAffinityType: []string{"requiredDuringSchedulingIgnoredDuringExecution"},
						},
					},
					removepodsviolatingnodetaints.PluginName: v1alpha1.DeschedulerStrategy{
						Enabled: true,
						Params: &v1alpha1.StrategyParameters{
							ExcludedTaints: []string{"dedicated=special-user", "reserved"},
						},
					},
					removepodsviolatingtopologyspreadconstraint.PluginName: v1alpha1.DeschedulerStrategy{
						Enabled: true,
						Params: &v1alpha1.StrategyParameters{
							IncludeSoftConstraints: true,
						},
					},
				},
			},
			result: &api.DeschedulerPolicy{
				Profiles: []api.Profile{
					{
						Name: fmt.Sprintf("strategy-%s-profile", nodeutilization.HighNodeUtilizationPluginName),
						PluginConfigs: []api.PluginConfig{
							defaultEvictorPluginConfig,
							{
								Name: nodeutilization.HighNodeUtilizationPluginName,
								Args: &nodeutilization.HighNodeUtilizationArgs{
									Thresholds: api.ResourceThresholds{
										"cpu":    api.Percentage(20),
										"memory": api.Percentage(20),
										"pods":   api.Percentage(20),
									},
								},
							},
						},
						Plugins: api.Plugins{
							Balance: api.PluginSet{
								Enabled: []string{nodeutilization.HighNodeUtilizationPluginName},
							},
							Evict: defaultEvictorPluginSet,
						},
					},
					{
						Name: fmt.Sprintf("strategy-%s-profile", nodeutilization.LowNodeUtilizationPluginName),
						PluginConfigs: []api.PluginConfig{
							defaultEvictorPluginConfig,
							{
								Name: nodeutilization.LowNodeUtilizationPluginName,
								Args: &nodeutilization.LowNodeUtilizationArgs{
									UseDeviationThresholds: true,
									NumberOfNodes:          3,
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
								},
							},
						},
						Plugins: api.Plugins{
							Balance: api.PluginSet{
								Enabled: []string{nodeutilization.LowNodeUtilizationPluginName},
							},
							Evict: defaultEvictorPluginSet,
						},
					},
					{
						Name: fmt.Sprintf("strategy-%s-profile", removeduplicates.PluginName),
						PluginConfigs: []api.PluginConfig{
							defaultEvictorPluginConfig,
							{
								Name: removeduplicates.PluginName,
								Args: &removeduplicates.RemoveDuplicatesArgs{
									ExcludeOwnerKinds: []string{"ReplicaSet"},
								},
							},
						},
						Plugins: api.Plugins{
							Balance: api.PluginSet{
								Enabled: []string{removeduplicates.PluginName},
							},
							Evict: defaultEvictorPluginSet,
						},
					},
					{
						Name: fmt.Sprintf("strategy-%s-profile", removefailedpods.PluginName),
						PluginConfigs: []api.PluginConfig{
							defaultEvictorPluginConfig,
							{
								Name: removefailedpods.PluginName,
								Args: &removefailedpods.RemoveFailedPodsArgs{
									ExcludeOwnerKinds:       []string{"Job"},
									MinPodLifetimeSeconds:   utilpointer.Uint(3600),
									Reasons:                 []string{"NodeAffinity"},
									IncludingInitContainers: true,
								},
							},
						},
						Plugins: api.Plugins{
							Deschedule: api.PluginSet{
								Enabled: []string{removefailedpods.PluginName},
							},
							Evict: defaultEvictorPluginSet,
						},
					},
					{
						Name: fmt.Sprintf("strategy-%s-profile", removepodshavingtoomanyrestarts.PluginName),
						PluginConfigs: []api.PluginConfig{
							defaultEvictorPluginConfig,
							{
								Name: removepodshavingtoomanyrestarts.PluginName,
								Args: &removepodshavingtoomanyrestarts.RemovePodsHavingTooManyRestartsArgs{
									PodRestartThreshold:     100,
									IncludingInitContainers: true,
								},
							},
						},
						Plugins: api.Plugins{
							Deschedule: api.PluginSet{
								Enabled: []string{removepodshavingtoomanyrestarts.PluginName},
							},
							Evict: defaultEvictorPluginSet,
						},
					},
					{
						Name: fmt.Sprintf("strategy-%s-profile", removepodsviolatinginterpodantiaffinity.PluginName),
						PluginConfigs: []api.PluginConfig{
							defaultEvictorPluginConfig,
							{
								Name: removepodsviolatinginterpodantiaffinity.PluginName,
								Args: &removepodsviolatinginterpodantiaffinity.RemovePodsViolatingInterPodAntiAffinityArgs{},
							},
						},
						Plugins: api.Plugins{
							Deschedule: api.PluginSet{
								Enabled: []string{removepodsviolatinginterpodantiaffinity.PluginName},
							},
							Evict: defaultEvictorPluginSet,
						},
					},
					{
						Name: fmt.Sprintf("strategy-%s-profile", removepodsviolatingnodeaffinity.PluginName),
						PluginConfigs: []api.PluginConfig{
							defaultEvictorPluginConfig,
							{
								Name: removepodsviolatingnodeaffinity.PluginName,
								Args: &removepodsviolatingnodeaffinity.RemovePodsViolatingNodeAffinityArgs{
									NodeAffinityType: []string{"requiredDuringSchedulingIgnoredDuringExecution"},
								},
							},
						},
						Plugins: api.Plugins{
							Deschedule: api.PluginSet{
								Enabled: []string{removepodsviolatingnodeaffinity.PluginName},
							},
							Evict: defaultEvictorPluginSet,
						},
					},
					{
						Name: fmt.Sprintf("strategy-%s-profile", removepodsviolatingnodetaints.PluginName),
						PluginConfigs: []api.PluginConfig{
							defaultEvictorPluginConfig,
							{
								Name: removepodsviolatingnodetaints.PluginName,
								Args: &removepodsviolatingnodetaints.RemovePodsViolatingNodeTaintsArgs{
									ExcludedTaints: []string{"dedicated=special-user", "reserved"},
								},
							},
						},
						Plugins: api.Plugins{
							Deschedule: api.PluginSet{
								Enabled: []string{removepodsviolatingnodetaints.PluginName},
							},
							Evict: defaultEvictorPluginSet,
						},
					},
					{
						Name: fmt.Sprintf("strategy-%s-profile", removepodsviolatingtopologyspreadconstraint.PluginName),
						PluginConfigs: []api.PluginConfig{
							defaultEvictorPluginConfig,
							{
								Name: removepodsviolatingtopologyspreadconstraint.PluginName,
								Args: &removepodsviolatingtopologyspreadconstraint.RemovePodsViolatingTopologySpreadConstraintArgs{
									IncludeSoftConstraints: true,
								},
							},
						},
						Plugins: api.Plugins{
							Balance: api.PluginSet{
								Enabled: []string{removepodsviolatingtopologyspreadconstraint.PluginName},
							},
							Evict: defaultEvictorPluginSet,
						},
					},
				},
			},
		},
		{
			description: "invalid strategy name",
			policy: &v1alpha1.DeschedulerPolicy{Strategies: v1alpha1.StrategyList{
				"InvalidName": v1alpha1.DeschedulerStrategy{
					Enabled: true,
					Params:  &v1alpha1.StrategyParameters{},
				},
			}},
			result: nil,
			err:    fmt.Errorf("unknown strategy name: InvalidName"),
		},
		{
			description: "invalid threshold priority",
			policy: &v1alpha1.DeschedulerPolicy{Strategies: v1alpha1.StrategyList{
				nodeutilization.LowNodeUtilizationPluginName: v1alpha1.DeschedulerStrategy{
					Enabled: true,
					Params: &v1alpha1.StrategyParameters{
						ThresholdPriority:          utilpointer.Int32(100),
						ThresholdPriorityClassName: "name",
						NodeResourceUtilizationThresholds: &v1alpha1.NodeResourceUtilizationThresholds{
							Thresholds: v1alpha1.ResourceThresholds{
								"cpu":    v1alpha1.Percentage(20),
								"memory": v1alpha1.Percentage(20),
								"pods":   v1alpha1.Percentage(20),
							},
							TargetThresholds: v1alpha1.ResourceThresholds{
								"cpu":    v1alpha1.Percentage(50),
								"memory": v1alpha1.Percentage(50),
								"pods":   v1alpha1.Percentage(50),
							},
						},
					},
				},
			}},
			result: nil,
			err:    fmt.Errorf("priority threshold misconfigured for plugin LowNodeUtilization"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			client := fakeclientset.NewSimpleClientset()
			result, err := V1alpha1ToInternal(client, tc.policy)
			if err != nil {
				if err.Error() != tc.err.Error() {
					t.Errorf("unexpected error: %s", err.Error())
				}
			}
			if err == nil {
				// sort to easily compare deepequality
				result.Profiles = api.SortProfilesByName(result.Profiles)
				diff := cmp.Diff(tc.result, result)
				if diff != "" {
					t.Errorf("test '%s' failed. Results are not deep equal. mismatch (-want +got):\n%s", tc.description, diff)
				}
			}
		})
	}
}
