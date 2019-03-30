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

package v1alpha1

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type DeschedulerConfiguration struct {
	metav1.TypeMeta `json:",inline"`

	// Time interval for descheduler to run
	DeschedulingInterval time.Duration `json:"deschedulingInterval,omitempty"`

	// KubeconfigFile is path to kubeconfig file with authorization and master
	// location information.
	KubeconfigFile string `json:"kubeconfigFile"`

	// PolicyConfigFile is the filepath to the descheduler policy configuration.
	PolicyConfigFile string `json:"policyConfigFile,omitempty"`

	// Dry run
	DryRun bool `json:"dryRun,omitempty"`

	// Node selectors
	NodeSelector string `json:"nodeSelector,omitempty"`

	// MaxNoOfPodsToEvictPerNode restricts maximum of pods to be evicted per node.
	MaxNoOfPodsToEvictPerNode int `json:"maxNoOfPodsToEvictPerNode,omitempty"`

	// Prefix of the annotations specific to the descheduler.
	AnnotationsPrefix string `json:"annotationsPrefix,omitempty"`
}
