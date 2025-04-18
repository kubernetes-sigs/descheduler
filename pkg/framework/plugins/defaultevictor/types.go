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

package defaultevictor

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/descheduler/pkg/api"
)

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DefaultEvictorArgs holds arguments used to configure DefaultEvictor plugin.
type DefaultEvictorArgs struct {
	metav1.TypeMeta `json:",inline"`

	NodeSelector string `json:"nodeSelector,omitempty"`
	// Deprecated: Use EvictionActions with "withLocalStorage" instead.
	EvictLocalStoragePods bool `json:"evictLocalStoragePods,omitempty"`
	// Deprecated: Use EvictionActions with "withDaemonSet" instead.
	EvictDaemonSetPods bool `json:"evictDaemonSetPods,omitempty"`
	// Deprecated: Use EvictionActions with "withSystemCritical" instead.
	EvictSystemCriticalPods bool `json:"evictSystemCriticalPods,omitempty"`
	// Deprecated: Use EvictionProtections with "withPVC" instead.
	IgnorePvcPods bool `json:"ignorePvcPods,omitempty"`
	// Deprecated: Use EvictionActions with "withFailedBarePods" instead.
	EvictFailedBarePods bool                   `json:"evictFailedBarePods,omitempty"`
	LabelSelector       *metav1.LabelSelector  `json:"labelSelector,omitempty"`
	PriorityThreshold   *api.PriorityThreshold `json:"priorityThreshold,omitempty"`
	NodeFit             bool                   `json:"nodeFit,omitempty"`
	MinReplicas         uint                   `json:"minReplicas,omitempty"`
	MinPodAge           *metav1.Duration       `json:"minPodAge,omitempty"`
	// Deprecated: Use EvictionProtection with "withoutPDB" instead.
	IgnorePodsWithoutPDB bool `json:"ignorePodsWithoutPDB,omitempty"`

	// EvictionActions specifies actions to take during eviction.
	// These represent types of Pods that should not be evicted by default,
	// but users can explicitly specify them to be evicted.
	// Supported values:
	// - "withLocalStorage": Evict pods with local storage.
	// - "daemonSetPods": Evict pods managed by DaemonSets.
	// - "systemCriticalPods": Evict system-critical pods.
	// - "failedBarePods": Evict failed bare pods (uncontrolled pods).
	EvictionActions []EvictionActionType `json:"evictionAction,omitempty"`
	// EvictionProtections specifies protections to apply during eviction.
	// These represent types of Pods that should be evicted by default,
	// but users can explicitly specify them to be protected from eviction.
	// Supported values:
	// - "withPVC": Protect pods with PersistentVolumeClaims (PVCs).
	// - "withoutPDB": Protect pods not covered by a PodDisruptionBudget (PDB).
	EvictionProtections []EvictionProtectionType `json:"evictionProtection,omitempty"`
}

type EvictionActionType string

const (
	// WithLocalStorage indicates that pods with local storage should be evicted.
	WithLocalStorage EvictionActionType = "withLocalStorage"
	// DaemonSetPods indicates that pods managed by DaemonSets should be evicted.
	DaemonSetPods EvictionActionType = "daemonSetPods"
	// SystemCriticalPods indicates that system-critical pods should be evicted.
	SystemCriticalPods EvictionActionType = "systemCriticalPods"
	// FailedBarePods indicates that failed bare pods (uncontrolled pods) should be evicted.
	FailedBarePods EvictionActionType = "failedBarePods"
)

type EvictionProtectionType string

const (
	// WithPVC indicates that pods with PersistentVolumeClaims (PVCs) should be protected from eviction.
	WithPVC EvictionProtectionType = "withPVC"
	// WithoutPDB indicates that pods not covered by a PodDisruptionBudget (PDB) should be protected from eviction.
	WithoutPDB EvictionProtectionType = "withoutPDB"
)
