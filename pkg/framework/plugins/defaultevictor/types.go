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
	// Deprecated: Use DisabledDefaultPodProtection with "withLocalStorage" instead.
	EvictLocalStoragePods bool `json:"evictLocalStoragePods,omitempty"`
	// Deprecated: Use DisabledDefaultPodProtection with "daemonSetPods" instead.
	EvictDaemonSetPods bool `json:"evictDaemonSetPods,omitempty"`
	// Deprecated: Use DisabledDefaultPodProtection with "systemCriticalPods" instead.
	EvictSystemCriticalPods bool `json:"evictSystemCriticalPods,omitempty"`
	// Deprecated: Use ExtraPodProtection with "withPVC" instead.
	IgnorePvcPods bool `json:"ignorePvcPods,omitempty"`
	// Deprecated: Use DisabledDefaultPodProtection with "failedBarePods" instead.
	EvictFailedBarePods bool                   `json:"evictFailedBarePods,omitempty"`
	LabelSelector       *metav1.LabelSelector  `json:"labelSelector,omitempty"`
	PriorityThreshold   *api.PriorityThreshold `json:"priorityThreshold,omitempty"`
	NodeFit             bool                   `json:"nodeFit,omitempty"`
	MinReplicas         uint                   `json:"minReplicas,omitempty"`
	MinPodAge           *metav1.Duration       `json:"minPodAge,omitempty"`
	// Deprecated: Use ExtraPodProtection with "withoutPDB" instead.
	IgnorePodsWithoutPDB bool `json:"ignorePodsWithoutPDB,omitempty"`

	// PodProtectionPolicies holds the list of enabled and disabled protection policies.
	// Users can selectively disable certain default protection rules or enable extra ones.
	PodProtectionPolicies PodProtections `json:"protectionPolicies,omitempty"`

	// defaultPodProtectionPolicies is a private field that contains the list of PodProtectionPolicy values
	// that are enabled by default.
	// These policies represent the built-in, default protections applied to certain types of Pods,
	// such as DaemonSet pods or SystemCritical pods. Users can selectively disable any of these
	// using the "disabled" list in PodProtectionPolicies.
	defaultPodProtectionPolicies []PodProtectionPolicy
}

var BuiltInPodProtectionPolicies = []PodProtectionPolicy{
	PodsWithLocalStorage,
	DaemonSetPods,
	SystemCriticalPods,
	FailedBarePods,
}

// PodProtectionPolicy defines the protection policy for a pod.
type PodProtectionPolicy string

const (
	PodsWithLocalStorage PodProtectionPolicy = "podsWithLocalStorage"
	DaemonSetPods        PodProtectionPolicy = "daemonSetPods"
	SystemCriticalPods   PodProtectionPolicy = "systemCriticalPods"
	FailedBarePods       PodProtectionPolicy = "failedBarePods"
	PodsWithPVC          PodProtectionPolicy = "podsWithPVC"
	PodsWithoutPDB       PodProtectionPolicy = "podsWithoutPDB"
)

// PodProtections holds the list of enabled and disabled protection policies.
// NOTE: The list of default enabled pod protection policies is subject to change in future versions.
// +k8s:deepcopy-gen=true
type PodProtections struct {
	// ExtraEnabled specifies additional protection policies that should be enabled.
	// Supports: podsWithPVC, podsWithoutPDB
	ExtraEnabled []PodProtectionPolicy `json:"extraEnabled,omitempty"`

	// Disabled specifies which default protection policies should be disabled.
	// Supports: podsWithLocalStorage, daemonSetPods, systemCriticalPods, failedBarePods
	Disabled []PodProtectionPolicy `json:"disabled,omitempty"`
}
