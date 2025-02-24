/*
Copyright 2025 The Kubernetes Authors.
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

package example

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/descheduler/pkg/api"
)

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ExampleArgs holds a list of arguments used to configure the plugin. For this
// simple example we only care about a regex, a maximum age and possibly a list
// of namespaces to which we want to apply the descheduler. This plugin evicts
// pods that match a given regular expression and are older than the maximum
// allowed age. Most of the fields here were defined as strings so we can
// validate them somewhere else (show you a better implementation example).
type ExampleArgs struct {
	metav1.TypeMeta `json:",inline"`

	// Regex is a regular expression we use to match against pod names. If
	// the pod name matches the regex it will be evicted. This is expected
	// to be a valid regular expression (according to go's regexp package).
	Regex string `json:"regex"`

	// MaxAge is the maximum age a pod can have before it is considered for
	// eviction. This is expected to be a valid time.Duration.
	MaxAge string `json:"maxAge"`

	// Namespaces allows us to filter on which namespaces we want to apply
	// the descheduler.
	Namespaces *api.Namespaces `json:"namespaces,omitempty"`
}
