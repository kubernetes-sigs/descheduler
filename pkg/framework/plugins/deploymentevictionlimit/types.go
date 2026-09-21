/*
Copyright 2026 The Kubernetes Authors.

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

package deploymentevictionlimit

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DeploymentEvictionLimitArgs holds arguments used to configure the
// DeploymentEvictionLimit plugin.
type DeploymentEvictionLimitArgs struct {
	metav1.TypeMeta `json:",inline"`

	// Deployments lists the Deployments whose pods are subject to a per-cycle
	// eviction budget. Pods owned by Deployments not listed here are not
	// limited by this plugin.
	Deployments []DeploymentLimit `json:"deployments"`
}

// +k8s:deepcopy-gen=true

// DeploymentLimit is the per-cycle eviction budget for a single Deployment.
type DeploymentLimit struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	// MaxPodsToEvict is the maximum number of pods owned by this Deployment
	// (through its ReplicaSets) that may be evicted in one descheduling cycle.
	// 0 blocks eviction of the Deployment's pods entirely.
	MaxPodsToEvict uint `json:"maxPodsToEvict"`
}
