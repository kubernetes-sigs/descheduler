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

	NodeSelector      string                 `json:"nodeSelector,omitempty"`
	LabelSelector     *metav1.LabelSelector  `json:"labelSelector,omitempty"`
	PriorityThreshold *api.PriorityThreshold `json:"priorityThreshold,omitempty"`
	NodeFit           bool                   `json:"nodeFit,omitempty"`
	MinReplicas       uint                   `json:"minReplicas,omitempty"`
	MinPodAge         *metav1.Duration       `json:"minPodAge,omitempty"`
	NoEvictionPolicy  NoEvictionPolicy       `json:"noEvictionPolicy,omitempty"`

	// PodProtections holds the list of enabled and disabled protection policies.
	// Users can selectively disable certain default protection rules or enable extra ones.
	PodProtections PodProtections `json:"podProtections,omitempty"`

	// Deprecated: Use DisabledDefaultPodProtection with "PodsWithLocalStorage" instead.
	EvictLocalStoragePods bool `json:"evictLocalStoragePods,omitempty"`
	// Deprecated: Use DisabledDefaultPodProtection with "DaemonSetPods" instead.
	EvictDaemonSetPods bool `json:"evictDaemonSetPods,omitempty"`
	// Deprecated: Use DisabledDefaultPodProtection with "SystemCriticalPods" instead.
	EvictSystemCriticalPods bool `json:"evictSystemCriticalPods,omitempty"`
	// Deprecated: Use ExtraPodProtection with "PodsWithPVC" instead.
	IgnorePvcPods bool `json:"ignorePvcPods,omitempty"`
	// Deprecated: Use ExtraPodProtection with "PodsWithoutPDB" instead.
	IgnorePodsWithoutPDB bool `json:"ignorePodsWithoutPDB,omitempty"`
	// Deprecated: Use DisabledDefaultPodProtection with "FailedBarePods" instead.
	EvictFailedBarePods bool `json:"evictFailedBarePods,omitempty"`
}

// PodProtection defines the protection policy for a pod.
type PodProtection string

const (
	PodsWithLocalStorage   PodProtection = "PodsWithLocalStorage"
	DaemonSetPods          PodProtection = "DaemonSetPods"
	SystemCriticalPods     PodProtection = "SystemCriticalPods"
	FailedBarePods         PodProtection = "FailedBarePods"
	PodsWithPVC            PodProtection = "PodsWithPVC"
	PodsWithoutPDB         PodProtection = "PodsWithoutPDB"
	PodsWithResourceClaims PodProtection = "PodsWithResourceClaims"
)

// PodProtections holds the list of enabled and disabled protection policies.
// NOTE: The list of default enabled pod protection policies is subject to change in future versions.
// +k8s:deepcopy-gen=true
type PodProtections struct {
	// ExtraEnabled specifies additional protection policies that should be enabled.
	// Supports: PodsWithPVC, PodsWithoutPDB
	ExtraEnabled []PodProtection `json:"extraEnabled,omitempty"`

	// DefaultDisabled specifies which default protection policies should be disabled.
	// Supports: PodsWithLocalStorage, DaemonSetPods, SystemCriticalPods, FailedBarePods
	DefaultDisabled []PodProtection `json:"defaultDisabled,omitempty"`
}

// defaultPodProtections holds the list of protection policies that are enabled by default.
// User can use the 'disabledDefaultPodProtections' evictor arguments (via PodProtections.DefaultDisabled)
// to disable any of these default protections.
//
// The following four policies are included by default:
//   - PodsWithLocalStorage: Protects pods with local storage.
//   - DaemonSetPods: Protects DaemonSet managed pods.
//   - SystemCriticalPods: Protects system-critical pods.
//   - FailedBarePods: Protects failed bare pods (not part of any controller).
var defaultPodProtections = []PodProtection{
	PodsWithLocalStorage,
	SystemCriticalPods,
	FailedBarePods,
	DaemonSetPods,
}

// extraPodProtections holds a list of protection policies that the user can optionally enable
// through the configuration (via PodProtections.ExtraEnabled). These policies are not enabled by default.
//
// Currently supported extra policies:
//   - PodsWithPVC: Protects pods using PersistentVolumeClaims.
//   - PodsWithoutPDB: Protects pods lacking a PodDisruptionBudget.
//   - PodsWithResourceClaims: Protects pods using ResourceClaims.
var extraPodProtections = []PodProtection{
	PodsWithPVC,
	PodsWithoutPDB,
	PodsWithResourceClaims,
}

// NoEvictionPolicy dictates whether a no-eviction policy is preferred or mandatory.
// Needs to be used with caution as this will give users ability to protect their pods
// from eviction. Which might work against enfored policies. E.g. plugins evicting pods
// violating security policies.
type NoEvictionPolicy string

const (
	// PreferredNoEvictionPolicy interprets the no-eviction policy as a preference.
	// Meaning the annotation will get ignored by the DefaultEvictor plugin.
	// Yet, plugins may optionally sort their pods based on the annotation
	// and focus on evicting pods that do not set the annotation.
	PreferredNoEvictionPolicy NoEvictionPolicy = "Preferred"

	// MandatoryNoEvictionPolicy interprets the no-eviction policy as mandatory.
	// Every pod carying the annotation will get excluded from eviction.
	MandatoryNoEvictionPolicy NoEvictionPolicy = "Mandatory"
)
