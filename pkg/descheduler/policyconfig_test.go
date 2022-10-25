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
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilpointer "k8s.io/utils/pointer"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/api/v1alpha1"
	"sigs.k8s.io/descheduler/pkg/api/v1alpha2"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
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

func TestConvertRemoveFailedPodsArgs(t *testing.T) {

	type testCase struct {
		description string
		params      *v1alpha1.StrategyParameters
		result      *removefailedpods.RemoveFailedPodsArgs
	}
	// Namespaces            *api.Namespaces
	// LabelSelector         *metav1.LabelSelector
	// MaxPodLifeTimeSeconds *uint
	// States                []string
	defaultLabel := map[string]string{"test": "test"}
	testCases := []testCase{
		{
			description: "convertRemoveFailedPodsArgs with all fields set",
			params: &v1alpha1.StrategyParameters{
				Namespaces: &v1alpha1.Namespaces{
					Include: []string{"default"},
					Exclude: []string{"test1"},
				},
				LabelSelector: &v1.LabelSelector{
					MatchLabels: defaultLabel,
				},
				FailedPods: &v1alpha1.FailedPods{
					ExcludeOwnerKinds: []string{
						"StatefulSet",
					},
					MinPodLifetimeSeconds: utilpointer.Uint(3600),
					Reasons: []string{
						"NodeAffinity",
					},
					IncludingInitContainers: true,
				},
			},
			result: &removefailedpods.RemoveFailedPodsArgs{
				Namespaces: &api.Namespaces{
					Include: []string{"default"},
					Exclude: []string{"test1"},
				},
				LabelSelector: &v1.LabelSelector{
					MatchLabels: defaultLabel,
				},
				ExcludeOwnerKinds: []string{
					"StatefulSet",
				},
				MinPodLifetimeSeconds: utilpointer.Uint(3600),
				Reasons: []string{
					"NodeAffinity",
				},
				IncludingInitContainers: true,
			},
		},
		{
			description: "convertRemoveFailedPodsArgs nil namespace",
			params: &v1alpha1.StrategyParameters{
				LabelSelector: &v1.LabelSelector{
					MatchLabels: defaultLabel,
				},
				FailedPods: &v1alpha1.FailedPods{
					ExcludeOwnerKinds: []string{
						"StatefulSet",
					},
					MinPodLifetimeSeconds: utilpointer.Uint(3600),
					Reasons: []string{
						"NodeAffinity",
					},
					IncludingInitContainers: true,
				},
			},
			result: &removefailedpods.RemoveFailedPodsArgs{
				LabelSelector: &v1.LabelSelector{
					MatchLabels: defaultLabel,
				},
				ExcludeOwnerKinds: []string{
					"StatefulSet",
				},
				MinPodLifetimeSeconds: utilpointer.Uint(3600),
				Reasons: []string{
					"NodeAffinity",
				},
				IncludingInitContainers: true,
			},
		},
		{
			description: "convertRemoveFailedPodsArgs nil labelSelector",
			params: &v1alpha1.StrategyParameters{
				Namespaces: &v1alpha1.Namespaces{
					Include: []string{"default"},
					Exclude: []string{"test1"},
				},
				FailedPods: &v1alpha1.FailedPods{
					ExcludeOwnerKinds: []string{
						"StatefulSet",
					},
					MinPodLifetimeSeconds: utilpointer.Uint(3600),
					Reasons: []string{
						"NodeAffinity",
					},
					IncludingInitContainers: true,
				},
			},
			result: &removefailedpods.RemoveFailedPodsArgs{
				Namespaces: &api.Namespaces{
					Include: []string{"default"},
					Exclude: []string{"test1"},
				},
				ExcludeOwnerKinds: []string{
					"StatefulSet",
				},
				MinPodLifetimeSeconds: utilpointer.Uint(3600),
				Reasons: []string{
					"NodeAffinity",
				},
				IncludingInitContainers: true,
			},
		},
		{
			description: "convertRemoveFailedPodsArgs nil failedPods field",
			params: &v1alpha1.StrategyParameters{
				Namespaces: &v1alpha1.Namespaces{
					Include: []string{"default"},
					Exclude: []string{"test1"},
				},
			},
			result: &removefailedpods.RemoveFailedPodsArgs{
				Namespaces: &api.Namespaces{
					Include: []string{"default"},
					Exclude: []string{"test1"},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			result := convertRemoveFailedPodsArgs(tc.params)
			if diff := cmp.Diff(tc.result, result); diff != "" {
				t.Errorf("test '%s' failed. Results are not deep equal. mismatch (-want +got):\n%s", tc.description, diff)
			}
		})
	}
}

func TestStrategyToProfileWithDeschedulePlugin(t *testing.T) {

	type testCase struct {
		description string
		args        runtime.Object
		name        v1alpha1.StrategyName
		strategy    v1alpha1.DeschedulerStrategy
		result      v1alpha2.Profile
	}
	testCases := []testCase{
		{
			description: "RemoveFailedPods strategy to profile with deschedule plugin enabled",
			args: &removefailedpods.RemoveFailedPodsArgs{
				MinPodLifetimeSeconds: utilpointer.Uint(3600),
			},
			name: removefailedpods.PluginName,
			strategy: v1alpha1.DeschedulerStrategy{
				Enabled: true,
			},
			result: v1alpha2.Profile{
				PluginConfig: []v1alpha2.PluginConfig{
					{
						Name: removefailedpods.PluginName,
						Args: runtime.RawExtension{
							Object: &removefailedpods.RemoveFailedPodsArgs{
								MinPodLifetimeSeconds: utilpointer.Uint(3600),
							},
						},
					},
				},
				Plugins: v1alpha2.Plugins{
					Deschedule: v1alpha2.PluginSet{
						Enabled: []string{
							removefailedpods.PluginName,
						},
					},
				},
			},
		},
		{
			description: "RemoveFailedPods strategy to profile with deschedule plugin disabled",
			args: &removefailedpods.RemoveFailedPodsArgs{
				MinPodLifetimeSeconds: utilpointer.Uint(3600),
			},
			name: removefailedpods.PluginName,
			strategy: v1alpha1.DeschedulerStrategy{
				Enabled: false,
			},
			result: v1alpha2.Profile{
				PluginConfig: []v1alpha2.PluginConfig{
					{
						Name: removefailedpods.PluginName,
						Args: runtime.RawExtension{
							Object: &removefailedpods.RemoveFailedPodsArgs{
								MinPodLifetimeSeconds: utilpointer.Uint(3600),
							},
						},
					},
				},
				Plugins: v1alpha2.Plugins{
					Deschedule: v1alpha2.PluginSet{
						Disabled: []string{
							removefailedpods.PluginName,
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			result := strategyToProfileWithDeschedulePlugin(tc.args, tc.name, tc.strategy)
			if diff := cmp.Diff(tc.result, result); diff != "" {
				t.Errorf("test '%s' failed. Results are not deep equal. mismatch (-want +got):\n%s", tc.description, diff)
			}
		})
	}
}

func TestStrategiesToProfiles(t *testing.T) {

	type testCase struct {
		description string
		strategies  v1alpha1.StrategyList
		err         error
		result      *[]v1alpha2.Profile
	}
	testCases := []testCase{
		{
			description: "RemoveFailedPods enabled, LowNodeUtilization disabled strategies to profile",
			strategies: v1alpha1.StrategyList{
				removeduplicates.PluginName: v1alpha1.DeschedulerStrategy{
					Enabled: true,
					Params: &v1alpha1.StrategyParameters{
						Namespaces: &v1alpha1.Namespaces{
							Include: []string{
								"test1",
							},
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
			},
			// alphabetical to make it easier to test
			result: &[]v1alpha2.Profile{
				{
					Name: nodeutilization.LowNodeUtilizationPluginName,
					PluginConfig: []v1alpha2.PluginConfig{
						{
							Name: nodeutilization.LowNodeUtilizationPluginName,
							Args: runtime.RawExtension{
								Object: &nodeutilization.LowNodeUtilizationArgs{
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
					},
					Plugins: v1alpha2.Plugins{
						Balance: v1alpha2.PluginSet{
							Disabled: []string{nodeutilization.LowNodeUtilizationPluginName},
						},
					},
				},
				{
					Name: removeduplicates.PluginName,
					PluginConfig: []v1alpha2.PluginConfig{
						{
							Name: removeduplicates.PluginName,
							Args: runtime.RawExtension{
								Object: &removeduplicates.RemoveDuplicatesArgs{
									Namespaces: &api.Namespaces{
										Include: []string{
											"test1",
										},
										Exclude: []string{
											"test2",
										},
									},
								},
							},
						},
					},
					Plugins: v1alpha2.Plugins{
						Balance: v1alpha2.PluginSet{
							Enabled: []string{removeduplicates.PluginName},
						},
					},
				},
			},
		},
		{
			description: "convert all strategies",
			strategies: v1alpha1.StrategyList{
				removeduplicates.PluginName: v1alpha1.DeschedulerStrategy{
					Enabled: true,
					Params:  &v1alpha1.StrategyParameters{},
				},
				nodeutilization.LowNodeUtilizationPluginName: v1alpha1.DeschedulerStrategy{
					Enabled: true,
					Params:  &v1alpha1.StrategyParameters{},
				},
				nodeutilization.HighNodeUtilizationPluginName: v1alpha1.DeschedulerStrategy{
					Enabled: true,
					Params:  &v1alpha1.StrategyParameters{},
				},
				removefailedpods.PluginName: v1alpha1.DeschedulerStrategy{
					Enabled: true,
					Params:  &v1alpha1.StrategyParameters{},
				},
				removepodshavingtoomanyrestarts.PluginName: v1alpha1.DeschedulerStrategy{
					Enabled: true,
					Params:  &v1alpha1.StrategyParameters{},
				},
				removepodsviolatinginterpodantiaffinity.PluginName: v1alpha1.DeschedulerStrategy{
					Enabled: true,
					Params:  &v1alpha1.StrategyParameters{},
				},
				removepodsviolatingnodeaffinity.PluginName: v1alpha1.DeschedulerStrategy{
					Enabled: true,
					Params:  &v1alpha1.StrategyParameters{},
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
			// alphabetical to make it easier to test
			result: &[]v1alpha2.Profile{
				{
					Name: nodeutilization.HighNodeUtilizationPluginName,
					PluginConfig: []v1alpha2.PluginConfig{
						{
							Name: nodeutilization.HighNodeUtilizationPluginName,
							Args: runtime.RawExtension{
								Object: &nodeutilization.HighNodeUtilizationArgs{},
							},
						},
					},
					Plugins: v1alpha2.Plugins{
						Balance: v1alpha2.PluginSet{
							Enabled: []string{nodeutilization.HighNodeUtilizationPluginName},
						},
					},
				},
				{
					Name: nodeutilization.LowNodeUtilizationPluginName,
					PluginConfig: []v1alpha2.PluginConfig{
						{
							Name: nodeutilization.LowNodeUtilizationPluginName,
							Args: runtime.RawExtension{
								Object: &nodeutilization.LowNodeUtilizationArgs{},
							},
						},
					},
					Plugins: v1alpha2.Plugins{
						Balance: v1alpha2.PluginSet{
							Enabled: []string{nodeutilization.LowNodeUtilizationPluginName},
						},
					},
				},
				{
					Name: removeduplicates.PluginName,
					PluginConfig: []v1alpha2.PluginConfig{
						{
							Name: removeduplicates.PluginName,
							Args: runtime.RawExtension{
								Object: &removeduplicates.RemoveDuplicatesArgs{},
							},
						},
					},
					Plugins: v1alpha2.Plugins{
						Balance: v1alpha2.PluginSet{
							Enabled: []string{removeduplicates.PluginName},
						},
					},
				},
				{
					Name: removefailedpods.PluginName,
					PluginConfig: []v1alpha2.PluginConfig{
						{
							Name: removefailedpods.PluginName,
							Args: runtime.RawExtension{
								Object: &removefailedpods.RemoveFailedPodsArgs{},
							},
						},
					},
					Plugins: v1alpha2.Plugins{
						Deschedule: v1alpha2.PluginSet{
							Enabled: []string{removefailedpods.PluginName},
						},
					},
				},
				{
					Name: removepodshavingtoomanyrestarts.PluginName,
					PluginConfig: []v1alpha2.PluginConfig{
						{
							Name: removepodshavingtoomanyrestarts.PluginName,
							Args: runtime.RawExtension{
								Object: &removepodshavingtoomanyrestarts.RemovePodsHavingTooManyRestartsArgs{},
							},
						},
					},
					Plugins: v1alpha2.Plugins{
						Deschedule: v1alpha2.PluginSet{
							Enabled: []string{removepodshavingtoomanyrestarts.PluginName},
						},
					},
				},
				{
					Name: removepodsviolatinginterpodantiaffinity.PluginName,
					PluginConfig: []v1alpha2.PluginConfig{
						{
							Name: removepodsviolatinginterpodantiaffinity.PluginName,
							Args: runtime.RawExtension{
								Object: &removepodsviolatinginterpodantiaffinity.RemovePodsViolatingInterPodAntiAffinityArgs{},
							},
						},
					},
					Plugins: v1alpha2.Plugins{
						Deschedule: v1alpha2.PluginSet{
							Enabled: []string{removepodsviolatinginterpodantiaffinity.PluginName},
						},
					},
				},
				{
					Name: removepodsviolatingnodeaffinity.PluginName,
					PluginConfig: []v1alpha2.PluginConfig{
						{
							Name: removepodsviolatingnodeaffinity.PluginName,
							Args: runtime.RawExtension{
								Object: &removepodsviolatingnodeaffinity.RemovePodsViolatingNodeAffinityArgs{},
							},
						},
					},
					Plugins: v1alpha2.Plugins{
						Deschedule: v1alpha2.PluginSet{
							Enabled: []string{removepodsviolatingnodeaffinity.PluginName},
						},
					},
				},
				{
					Name: removepodsviolatingnodetaints.PluginName,
					PluginConfig: []v1alpha2.PluginConfig{
						{
							Name: removepodsviolatingnodetaints.PluginName,
							Args: runtime.RawExtension{
								Object: &removepodsviolatingnodetaints.RemovePodsViolatingNodeTaintsArgs{},
							},
						},
					},
					Plugins: v1alpha2.Plugins{
						Deschedule: v1alpha2.PluginSet{
							Enabled: []string{removepodsviolatingnodetaints.PluginName},
						},
					},
				},
				{
					Name: removepodsviolatingtopologyspreadconstraint.PluginName,
					PluginConfig: []v1alpha2.PluginConfig{
						{
							Name: removepodsviolatingtopologyspreadconstraint.PluginName,
							Args: runtime.RawExtension{
								Object: &removepodsviolatingtopologyspreadconstraint.RemovePodsViolatingTopologySpreadConstraintArgs{},
							},
						},
					},
					Plugins: v1alpha2.Plugins{
						Balance: v1alpha2.PluginSet{
							Enabled: []string{removepodsviolatingtopologyspreadconstraint.PluginName},
						},
					},
				},
			},
		},
		{
			description: "converting invalid strategy should result in expected error",
			strategies: v1alpha1.StrategyList{
				"InvalidName": v1alpha1.DeschedulerStrategy{},
			},
			result: &[]v1alpha2.Profile{},
			err:    fmt.Errorf("could not process strategy: InvalidName"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			result, err := strategiesToProfiles(tc.strategies)
			if err != nil {
				if err.Error() != tc.err.Error() {
					t.Errorf("unexpected error: %s", err.Error())
				}
			}
			diff := cmp.Diff(tc.result, result)
			if diff != "" && err == nil {
				t.Errorf("test '%s' failed. Results are not deep equal. mismatch (-want +got):\n%s", tc.description, diff)
			}
		})
	}
}

func TestValidateDeschedulerConfiguration(t *testing.T) {

	type testCase struct {
		description       string
		deschedulerPolicy v1alpha2.DeschedulerPolicy
		result            error
	}
	testCases := []testCase{
		{
			description: "multiple errors",
			deschedulerPolicy: v1alpha2.DeschedulerPolicy{
				Profiles: []v1alpha2.Profile{
					{
						Name: removefailedpods.PluginName,
						Plugins: v1alpha2.Plugins{
							Deschedule: v1alpha2.PluginSet{Enabled: []string{removefailedpods.PluginName}},
						},
						PluginConfig: []v1alpha2.PluginConfig{
							{
								Name: removefailedpods.PluginName,
								Args: runtime.RawExtension{
									Object: &removefailedpods.RemoveFailedPodsArgs{
										Namespaces: &api.Namespaces{
											Include: []string{"test1"},
											Exclude: []string{"test1"},
										},
									},
								},
							},
						},
					},
					{
						Name: removepodsviolatingtopologyspreadconstraint.PluginName,
						Plugins: v1alpha2.Plugins{
							Deschedule: v1alpha2.PluginSet{Enabled: []string{removepodsviolatingtopologyspreadconstraint.PluginName}},
						},
						PluginConfig: []v1alpha2.PluginConfig{
							{
								Name: removepodsviolatingtopologyspreadconstraint.PluginName,
								Args: runtime.RawExtension{
									Object: &removepodsviolatingtopologyspreadconstraint.RemovePodsViolatingTopologySpreadConstraintArgs{
										Namespaces: &api.Namespaces{
											Include: []string{"test1"},
											Exclude: []string{"test1"},
										},
									},
								},
							},
						},
					},
				},
			},
			result: fmt.Errorf("in profile RemoveFailedPods: profile with invalid number of evictor plugins enabled found. Please enable a single evictor plugin.: in profile RemoveFailedPods: only one of Include/Exclude namespaces can be set: in profile RemovePodsViolatingTopologySpreadConstraint: profile with invalid number of evictor plugins enabled found. Please enable a single evictor plugin.: in profile RemovePodsViolatingTopologySpreadConstraint: only one of Include/Exclude namespaces can be set"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			result := validateDeschedulerConfiguration(tc.deschedulerPolicy)
			if result.Error() != tc.result.Error() {
				t.Errorf("test '%s' failed. expected \n'%s', got \n'%s'", tc.description, tc.result, result)
			}
		})
	}
}

func TestDecodeVersionedPolicy(t *testing.T) {

	type testCase struct {
		description string
		policy      []byte
		err         error
		result      *api.DeschedulerPolicy
	}
	testCases := []testCase{
		{
			description: "v1alpha1 to internal",
			policy: []byte(`apiVersion: "descheduler/v1alpha1"
kind: "DeschedulerPolicy"
strategies:
  "PodLifeTime":
    enabled: true
    params:
      podLifeTime:
        maxPodLifeTimeSeconds: 5
      namespaces:
        include:
          - "testleaderelection-a"

`),
			result: &api.DeschedulerPolicy{
				Profiles: []api.Profile{
					{
						Name: podlifetime.PluginName,
						PluginConfig: []api.PluginConfig{
							{
								Name: podlifetime.PluginName,
								Args: &podlifetime.PodLifeTimeArgs{
									Namespaces: &api.Namespaces{
										Include: []string{"testleaderelection-a"},
									},
									MaxPodLifeTimeSeconds: utilpointer.Uint(5),
								},
							},
							{
								Name: defaultevictor.PluginName,
								Args: &defaultevictor.DefaultEvictorArgs{},
							},
						},
						Plugins: api.Plugins{
							Evict: api.PluginSet{
								Enabled: []string{defaultevictor.PluginName},
							},
							Filter: api.PluginSet{
								Enabled: []string{defaultevictor.PluginName},
							},
							PreEvictionFilter: api.PluginSet{
								Enabled: []string{defaultevictor.PluginName},
							},
							Deschedule: api.PluginSet{
								Enabled: []string{podlifetime.PluginName},
							},
						},
					},
				},
			},
		},
		{
			description: "v1aplha2 to internal",
			policy: []byte(`apiVersion: "descheduler/v1alpha2"
kind: "DeschedulerPolicy"
profiles:
  - name: ProfileName
    pluginConfig:
    - name: "DefaultEvictor"
      args:
        evictSystemCriticalPods: true
        evictFailedBarePods: true
        evictLocalStoragePods: true
        nodeFit: true
    - name: "RemovePodsHavingTooManyRestarts"
      args:
        podRestartThreshold: 100
        includingInitContainers: true
    plugins:
      filter:
        enabled:
          - "DefaultEvictor"
      deschedule:
        enabled:
          - "RemovePodsHavingTooManyRestarts"
`),
			result: &api.DeschedulerPolicy{
				TypeMeta: v1.TypeMeta{
					Kind:       "DeschedulerPolicy",
					APIVersion: "descheduler/v1alpha2",
				},
				Profiles: []api.Profile{
					{
						Name: "ProfileName",
						PluginConfig: []api.PluginConfig{
							{
								Name: defaultevictor.PluginName,
								Args: &defaultevictor.DefaultEvictorArgs{
									EvictSystemCriticalPods: true,
									EvictFailedBarePods:     true,
									EvictLocalStoragePods:   true,
									NodeFit:                 true,
								},
							},
							{
								Name: removepodshavingtoomanyrestarts.PluginName,
								Args: &removepodshavingtoomanyrestarts.RemovePodsHavingTooManyRestartsArgs{
									PodRestartThreshold:     100,
									IncludingInitContainers: true,
								},
							},
						},
						Plugins: api.Plugins{
							Evict: api.PluginSet{
								Enabled: []string{defaultevictor.PluginName},
							},
							Filter: api.PluginSet{
								Enabled: []string{defaultevictor.PluginName},
							},
							PreEvictionFilter: api.PluginSet{
								Enabled: []string{defaultevictor.PluginName},
							},
							Deschedule: api.PluginSet{
								Enabled: []string{removepodshavingtoomanyrestarts.PluginName},
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			result, err := decode(tc.policy, "filename")
			if err != nil {
				if tc.err == nil {
					t.Errorf("unexpected error: %s.", err.Error())
				} else {
					t.Errorf("unexpected error: %s. Was expecting %s", err.Error(), tc.err.Error())
				}
			}
			diff := cmp.Diff(tc.result, result)
			if diff != "" && err == nil {
				t.Errorf("test '%s' failed. Results are not deep equal. mismatch (-want +got):\n%s", tc.description, diff)
			}
		})
	}
}