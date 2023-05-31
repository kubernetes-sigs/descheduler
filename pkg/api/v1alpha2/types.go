/*
Copyright 2023 The Kubernetes Authors.

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

type DeschedulerPolicy struct {
	metav1.TypeMeta `json:",inline"`

	// Profiles
	Profiles []DeschedulerProfile `json:"profiles,omitempty"`

	// NodeSelector for a set of nodes to operate over
	NodeSelector *string `json:"nodeSelector,omitempty"`

	// MaxNoOfPodsToEvictPerNode restricts maximum of pods to be evicted per node.
	MaxNoOfPodsToEvictPerNode *uint `json:"maxNoOfPodsToEvictPerNode,omitempty"`

	// MaxNoOfPodsToEvictPerNamespace restricts maximum of pods to be evicted per namespace.
	MaxNoOfPodsToEvictPerNamespace *uint `json:"maxNoOfPodsToEvictPerNamespace,omitempty"`
}

type DeschedulerProfile struct {
	Name          string         `json:"name"`
	PluginConfigs []PluginConfig `json:"pluginConfig"`
	Plugins       Plugins        `json:"plugins"`
}

type Plugins struct {
	PreSort           PluginSet `json:"presort"`
	Sort              PluginSet `json:"sort"`
	Deschedule        PluginSet `json:"deschedule"`
	Balance           PluginSet `json:"balance"`
	Filter            PluginSet `json:"filter"`
	PreEvictionFilter PluginSet `json:"preevictionfilter"`
}

type PluginConfig struct {
	Name string               `json:"name"`
	Args runtime.RawExtension `json:"args"`
}

type PluginSet struct {
	Enabled  []string `json:"enabled"`
	Disabled []string `json:"disabled"`
}
