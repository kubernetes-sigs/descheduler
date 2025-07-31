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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/descheduler/pkg/api"
)

// EvictionMode describe a mode of eviction. See the list below for the
// available modes.
type EvictionMode string

const (
	// EvictionModeOnlyThresholdingResources makes the descheduler evict
	// only pods that have a resource request defined for any of the user
	// provided thresholds. If the pod does not request the resource, it
	// will not be evicted.
	EvictionModeOnlyThresholdingResources EvictionMode = "OnlyThresholdingResources"
)

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type LowNodeUtilizationArgs struct {
	metav1.TypeMeta `json:",inline"`

	UseDeviationThresholds bool                   `json:"useDeviationThresholds,omitempty"`
	Thresholds             api.ResourceThresholds `json:"thresholds"`
	TargetThresholds       api.ResourceThresholds `json:"targetThresholds"`
	NumberOfNodes          int                    `json:"numberOfNodes,omitempty"`
	MetricsUtilization     *MetricsUtilization    `json:"metricsUtilization,omitempty"`

	// EvictionModes is a set of modes to be taken into account when the
	// descheduler evicts pods. For example the mode
	// `OnlyThresholdingResources` can be used to make sure the descheduler
	// only evicts pods who have resource requests for the defined
	// thresholds.
	EvictionModes []EvictionMode `json:"evictionModes,omitempty"`

	// Naming this one differently since namespaces are still
	// considered while considering resources used by pods
	// but then filtered out before eviction
	EvictableNamespaces *api.Namespaces `json:"evictableNamespaces,omitempty"`

	// evictionLimits limits the number of evictions per domain. E.g. node, namespace, total.
	EvictionLimits *api.EvictionLimits `json:"evictionLimits,omitempty"`
}

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type HighNodeUtilizationArgs struct {
	metav1.TypeMeta `json:",inline"`

	Thresholds    api.ResourceThresholds `json:"thresholds"`
	NumberOfNodes int                    `json:"numberOfNodes,omitempty"`

	// EvictionModes is a set of modes to be taken into account when the
	// descheduler evicts pods. For example the mode
	// `OnlyThresholdingResources` can be used to make sure the descheduler
	// only evicts pods who have resource requests for the defined
	// thresholds.
	EvictionModes []EvictionMode `json:"evictionModes,omitempty"`

	// Naming this one differently since namespaces are still
	// considered while considering resources used by pods
	// but then filtered out before eviction
	EvictableNamespaces *api.Namespaces `json:"evictableNamespaces,omitempty"`
}

// MetricsUtilization allow to consume actual resource utilization from metrics
// +k8s:deepcopy-gen=true
type MetricsUtilization struct {
	// metricsServer enables metrics from a kubernetes metrics server.
	// Please see https://kubernetes-sigs.github.io/metrics-server/ for more.
	// Deprecated. Use Source instead.
	MetricsServer bool `json:"metricsServer,omitempty"`

	// source enables the plugin to consume metrics from a metrics source.
	// Currently only KubernetesMetrics available.
	Source api.MetricsSource `json:"source,omitempty"`

	// prometheus enables metrics collection through a prometheus query.
	Prometheus *Prometheus `json:"prometheus,omitempty"`
}

type Prometheus struct {
	// query returning a vector of samples, each sample labeled with `instance`
	// corresponding to a node name with each sample value as a real number
	// in <0; 1> interval.
	Query string `json:"query,omitempty"`
}
