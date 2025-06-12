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
	"errors"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/conversion"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	utilptr "k8s.io/utils/ptr"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/framework/pluginregistry"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removefailedpods"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodshavingtoomanyrestarts"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodsviolatingtopologyspreadconstraint"
)

// scope contains information about an ongoing conversion.
type scope struct {
	converter *conversion.Converter
	meta      *conversion.Meta
}

// Convert continues a conversion.
func (s scope) Convert(src, dest interface{}) error {
	return s.converter.Convert(src, dest, s.meta)
}

// Meta returns the meta object that was originally passed to Convert.
func (s scope) Meta() *conversion.Meta {
	return s.meta
}

func TestDecodeVersionedPolicy(t *testing.T) {
	client := fakeclientset.NewSimpleClientset()
	SetupPlugins()

	type testCase struct {
		description string
		policy      []byte
		err         error
		result      *api.DeschedulerPolicy
	}
	testCases := []testCase{
		{
			description: "v1alpha2 to internal",
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
        evictDaemonSetPods: true
        nodeFit: true
    - name: "RemovePodsHavingTooManyRestarts"
      args:
        podRestartThreshold: 100
        includingInitContainers: true
    plugins:
      deschedule:
        enabled:
          - "RemovePodsHavingTooManyRestarts"
`),
			result: &api.DeschedulerPolicy{
				Profiles: []api.DeschedulerProfile{
					{
						Name: "ProfileName",
						PluginConfigs: []api.PluginConfig{
							{
								Name: defaultevictor.PluginName,
								Args: &defaultevictor.DefaultEvictorArgs{
									EvictSystemCriticalPods: true,
									EvictFailedBarePods:     true,
									EvictLocalStoragePods:   true,
									EvictDaemonSetPods:      true,
									PriorityThreshold:       &api.PriorityThreshold{Value: utilptr.To[int32](2000000000)},
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
		{
			description: "v1alpha2 to internal, validate error handling (priorityThreshold exceeding maximum)",
			policy: []byte(`apiVersion: "descheduler/v1alpha2"
kind: "DeschedulerPolicy"
profiles:
  - name: ProfileName
    pluginConfig:
    - name: "DefaultEvictor"
      args:
        priorityThreshold:
          value: 2000000001
    plugins:
      deschedule:
        enabled:
          - "RemovePodsHavingTooManyRestarts"
`),
			result: nil,
			err:    errors.New("priority threshold can't be greater than 2000000000"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			result, err := decode("filename", tc.policy, client, pluginregistry.PluginRegistry)
			if err != nil {
				if tc.err == nil {
					t.Errorf("unexpected error: %s.", err.Error())
				} else if err.Error() != tc.err.Error() {
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

func TestValidateDeschedulerConfiguration(t *testing.T) {
	SetupPlugins()
	type testCase struct {
		description       string
		deschedulerPolicy api.DeschedulerPolicy
		result            error
	}
	testCases := []testCase{
		{
			description: "multiple errors",
			deschedulerPolicy: api.DeschedulerPolicy{
				Profiles: []api.DeschedulerProfile{
					{
						Name: removefailedpods.PluginName,
						Plugins: api.Plugins{
							Deschedule: api.PluginSet{Enabled: []string{removefailedpods.PluginName}},
						},
						PluginConfigs: []api.PluginConfig{
							{
								Name: removefailedpods.PluginName,
								Args: &removefailedpods.RemoveFailedPodsArgs{
									Namespaces: &api.Namespaces{
										Include: []string{"test1"},
										Exclude: []string{"test1"},
									},
								},
							},
						},
					},
					{
						Name: removepodsviolatingtopologyspreadconstraint.PluginName,
						Plugins: api.Plugins{
							Deschedule: api.PluginSet{Enabled: []string{removepodsviolatingtopologyspreadconstraint.PluginName}},
						},
						PluginConfigs: []api.PluginConfig{
							{
								Name: removepodsviolatingtopologyspreadconstraint.PluginName,
								Args: &removepodsviolatingtopologyspreadconstraint.RemovePodsViolatingTopologySpreadConstraintArgs{
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
			result: fmt.Errorf("[in profile RemoveFailedPods: only one of Include/Exclude namespaces can be set, in profile RemovePodsViolatingTopologySpreadConstraint: only one of Include/Exclude namespaces can be set]"),
		},
		{
			description: "Duplicit metrics providers error",
			deschedulerPolicy: api.DeschedulerPolicy{
				MetricsProviders: []api.MetricsProvider{
					{Source: api.KubernetesMetrics},
					{Source: api.KubernetesMetrics},
				},
			},
			result: fmt.Errorf("metric provider \"KubernetesMetrics\" is already configured, each source can be configured only once"),
		},
		{
			description: "Too many metrics providers error",
			deschedulerPolicy: api.DeschedulerPolicy{
				MetricsCollector: &api.MetricsCollector{
					Enabled: true,
				},
				MetricsProviders: []api.MetricsProvider{
					{Source: api.KubernetesMetrics},
				},
			},
			result: fmt.Errorf("it is not allowed to combine metrics provider when metrics collector is enabled"),
		},
		{
			description: "missing prometheus url error",
			deschedulerPolicy: api.DeschedulerPolicy{
				MetricsProviders: []api.MetricsProvider{
					{
						Source:     api.PrometheusMetrics,
						Prometheus: &api.Prometheus{},
					},
				},
			},
			result: fmt.Errorf("prometheus URL is required when prometheus is enabled"),
		},
		{
			description: "prometheus url is not valid error",
			deschedulerPolicy: api.DeschedulerPolicy{
				MetricsProviders: []api.MetricsProvider{
					{
						Source: api.PrometheusMetrics,
						Prometheus: &api.Prometheus{
							URL: "http://example.com:-80",
						},
					},
				},
			},
			result: fmt.Errorf("error parsing prometheus URL: parse \"http://example.com:-80\": invalid port \":-80\" after host"),
		},
		{
			description: "prometheus authtoken with no secret reference error",
			deschedulerPolicy: api.DeschedulerPolicy{
				MetricsProviders: []api.MetricsProvider{
					{
						Source: api.PrometheusMetrics,
						Prometheus: &api.Prometheus{
							URL:       "https://example.com:80",
							AuthToken: &api.AuthToken{},
						},
					},
				},
			},
			result: fmt.Errorf("prometheus authToken secret is expected to be set when authToken field is"),
		},
		{
			description: "prometheus authtoken with empty secret reference error",
			deschedulerPolicy: api.DeschedulerPolicy{
				MetricsProviders: []api.MetricsProvider{
					{
						Source: api.PrometheusMetrics,
						Prometheus: &api.Prometheus{
							URL: "https://example.com:80",
							AuthToken: &api.AuthToken{
								SecretReference: &api.SecretReference{},
							},
						},
					},
				},
			},
			result: fmt.Errorf("prometheus authToken secret reference does not set both namespace and name"),
		},
		{
			description: "prometheus authtoken missing secret reference namespace error",
			deschedulerPolicy: api.DeschedulerPolicy{
				MetricsProviders: []api.MetricsProvider{
					{
						Source: api.PrometheusMetrics,
						Prometheus: &api.Prometheus{
							URL: "https://example.com:80",
							AuthToken: &api.AuthToken{
								SecretReference: &api.SecretReference{
									Name: "secretname",
								},
							},
						},
					},
				},
			},
			result: fmt.Errorf("prometheus authToken secret reference does not set both namespace and name"),
		},
		{
			description: "prometheus authtoken missing secret reference name error",
			deschedulerPolicy: api.DeschedulerPolicy{
				MetricsProviders: []api.MetricsProvider{
					{
						Source: api.PrometheusMetrics,
						Prometheus: &api.Prometheus{
							URL: "https://example.com:80",
							AuthToken: &api.AuthToken{
								SecretReference: &api.SecretReference{
									Namespace: "secretnamespace",
								},
							},
						},
					},
				},
			},
			result: fmt.Errorf("prometheus authToken secret reference does not set both namespace and name"),
		},
		{
			description: "valid prometheus authtoken secret reference",
			deschedulerPolicy: api.DeschedulerPolicy{
				MetricsProviders: []api.MetricsProvider{
					{
						Source: api.PrometheusMetrics,
						Prometheus: &api.Prometheus{
							URL: "https://example.com:80",
							AuthToken: &api.AuthToken{
								SecretReference: &api.SecretReference{
									Name:      "secretname",
									Namespace: "secretnamespace",
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			result := validateDeschedulerConfiguration(tc.deschedulerPolicy, pluginregistry.PluginRegistry)
			if result == nil && tc.result != nil || result != nil && tc.result == nil {
				t.Errorf("test '%s' failed. expected \n'%s', got \n'%s'", tc.description, tc.result, result)
			} else if result == nil && tc.result == nil {
				return
			} else if result.Error() != tc.result.Error() {
				t.Errorf("test '%s' failed. expected \n'%s', got \n'%s'", tc.description, tc.result, result)
			}
		})
	}
}

func TestDecodeDefaults(t *testing.T) {
	client := fakeclientset.NewSimpleClientset()
	SetupPlugins()
	type testCase struct {
		description string
		policy      []byte
		err         error
		result      *api.DeschedulerPolicy
	}
	testCases := []testCase{
		{
			description: "use empty RemoveFailedPods, check MinPodLifetimeSeconds default",
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
        evictDaemonSetPods: true
        nodeFit: true
    - name: "RemoveFailedPods"
    plugins:
      filter:
        enabled:
          - "DefaultEvictor"
      preEvictionFilter:
        enabled:
          - "DefaultEvictor"
      deschedule:
        enabled:
          - "RemovePodsHavingTooManyRestarts"
`),
			result: &api.DeschedulerPolicy{
				Profiles: []api.DeschedulerProfile{
					{
						Name: "ProfileName",
						PluginConfigs: []api.PluginConfig{
							{
								Name: defaultevictor.PluginName,
								Args: &defaultevictor.DefaultEvictorArgs{
									EvictSystemCriticalPods: true,
									EvictFailedBarePods:     true,
									EvictLocalStoragePods:   true,
									EvictDaemonSetPods:      true,
									PriorityThreshold:       &api.PriorityThreshold{Value: utilptr.To[int32](2000000000)},
									NodeFit:                 true,
								},
							},
							{
								Name: removefailedpods.PluginName,
								Args: &removefailedpods.RemoveFailedPodsArgs{
									MinPodLifetimeSeconds: utilptr.To[uint](3600),
								},
							},
						},
						Plugins: api.Plugins{
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
		{
			description: "omit default evictor extension point with their enablement",
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
        evictDaemonSetPods: true
        nodeFit: true
    - name: "RemoveFailedPods"
    plugins:
      deschedule:
        enabled:
          - "RemovePodsHavingTooManyRestarts"
`),
			result: &api.DeschedulerPolicy{
				Profiles: []api.DeschedulerProfile{
					{
						Name: "ProfileName",
						PluginConfigs: []api.PluginConfig{
							{
								Name: defaultevictor.PluginName,
								Args: &defaultevictor.DefaultEvictorArgs{
									EvictSystemCriticalPods: true,
									EvictFailedBarePods:     true,
									EvictLocalStoragePods:   true,
									EvictDaemonSetPods:      true,
									PriorityThreshold:       &api.PriorityThreshold{Value: utilptr.To[int32](2000000000)},
									NodeFit:                 true,
								},
							},
							{
								Name: removefailedpods.PluginName,
								Args: &removefailedpods.RemoveFailedPodsArgs{
									MinPodLifetimeSeconds: utilptr.To[uint](3600),
								},
							},
						},
						Plugins: api.Plugins{
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
			result, err := decode("filename", tc.policy, client, pluginregistry.PluginRegistry)
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
