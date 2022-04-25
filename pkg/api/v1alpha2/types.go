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

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type DeschedulerConfiguration struct {
	metav1.TypeMeta `json:",inline"`

	// Profiles
	Profiles []Profile `json:"profiles,omitempty"`

	// NodeSelector for a set of nodes to operate over
	NodeSelector *string `json:"nodeSelector,omitempty"`

	// MaxNoOfPodsToEvictPerNode restricts maximum of pods to be evicted per node.
	MaxNoOfPodsToEvictPerNode *int `json:"maxNoOfPodsToEvictPerNode,omitempty"`

	// MaxNoOfPodsToEvictPerNamespace restricts maximum of pods to be evicted per namespace.
	MaxNoOfPodsToEvictPerNamespace *int `json:"maxNoOfPodsToEvictPerNamespace,omitempty"`
}

type Profile struct {
	Name         string         `json:"name"`
	PluginConfig []PluginConfig `json:"pluginConfig"`
	Plugins      Plugins        `json:"plugins"`
}

type Plugins struct {
	PreSort    Plugin `json:"presort"`
	Sort       Plugin `json:"sort"`
	Deschedule Plugin `json:"deschedule"`
	Balance    Plugin `json:"balance"`
	Evict      Plugin `json:"evict"`
}

type PluginConfig struct {
	Name string         `json:"name"`
	Args runtime.Object `json:"args"`
}

type Plugin struct {
	Enabled  []string `json:"enabled"`
	Disabled []string `json:"disabled"`
}

// Namespaces carries a list of included/excluded namespaces
// for which a given strategy is applicable.
type Namespaces struct {
	Include []string `json:"include"`
	Exclude []string `json:"exclude"`
}
