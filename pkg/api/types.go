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

package api

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type DeschedulerPolicy struct {
	metav1.TypeMeta

	// Profiles
	Profiles []DeschedulerProfile

	// NodeSelector for a set of nodes to operate over
	NodeSelector *string

	// MaxNoOfPodsToEvictPerNode restricts maximum of pods to be evicted per node.
	MaxNoOfPodsToEvictPerNode *uint

	// MaxNoOfPodsToEvictPerNamespace restricts maximum of pods to be evicted per namespace.
	MaxNoOfPodsToEvictPerNamespace *uint

	// MaxNoOfPodsToTotal restricts maximum of pods to be evicted total.
	MaxNoOfPodsToEvictTotal *uint

	// MetricsCollector configures collection of metrics about actual resource utilization
	MetricsCollector MetricsCollector

	// Prometheus enables metrics collection through Prometheus
	Prometheus Prometheus
}

// Namespaces carries a list of included/excluded namespaces
// for which a given strategy is applicable
type Namespaces struct {
	Include []string `json:"include,omitempty"`
	Exclude []string `json:"exclude,omitempty"`
}

type (
	Percentage         float64
	ResourceThresholds map[v1.ResourceName]Percentage
)

type PriorityThreshold struct {
	Value *int32 `json:"value"`
	Name  string `json:"name"`
}

type DeschedulerProfile struct {
	Name          string
	PluginConfigs []PluginConfig
	Plugins       Plugins
}

type PluginConfig struct {
	Name string
	Args runtime.Object
}

type Plugins struct {
	PreSort           PluginSet
	Sort              PluginSet
	Deschedule        PluginSet
	Balance           PluginSet
	Filter            PluginSet
	PreEvictionFilter PluginSet
}

type PluginSet struct {
	Enabled  []string
	Disabled []string
}

// MetricsCollector configures collection of metrics about actual resource utilization
type MetricsCollector struct {
	// Enabled metrics collection from kubernetes metrics.
	// Later, the collection can be extended to other providers.
	Enabled bool
}

type Prometheus struct {
	URL                string
	AuthToken          AuthToken
	InsecureSkipVerify bool
}

type AuthToken struct {
	// raw for a raw authentication token
	Raw string
	// secretReference references an authentication token.
	// secrets are expected to be created under the descheduler's namespace.
	SecretReference SecretReference
}

// SecretReference holds a reference to a Secret
type SecretReference struct {
	// namespace is the namespace of the secret.
	Namespace string
	// name is the name of the secret.
	Name string
}
