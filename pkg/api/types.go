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
}

// Namespaces carries a list of included/excluded namespaces
// for which a given strategy is applicable
type Namespaces struct {
	Include []string
	Exclude []string
}

type (
	Percentage         float64
	ResourceThresholds map[v1.ResourceName]Percentage
)

type PriorityThreshold struct {
	Value *int32
	Name  string
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
