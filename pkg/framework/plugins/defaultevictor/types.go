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

	// DisabledDefaultPodProtections is a list that specifies which default Pod protection mechanisms are disabled.
	// Users can selectively disable certain default protection rules via this field.
	DisabledDefaultPodProtections []DisabledDefaultPodProtection `json:"disabledDefaultPodProtections,omitempty"`

	// ExtraPodProtections is a list that specifies additional Pod protection mechanisms that users can enable.
	// These protection rules are optional and allow users to extend the protection scope based on their needs.
	// Users can selectively enable certain extra protection rules via this field.
	ExtraPodProtections []ExtraPodProtection `json:"extraPodProtections,omitempty"`
}

// DisabledDefaultPodProtection defines the type for default Pod protection mechanisms.
// Each value represents a specific protection rule that can be disabled.
type DisabledDefaultPodProtection string

const (
	// WithLocalStorage indicates disabling protection for Pods using local storage.
	// Once disabled, Pods using local storage may be deleted or interfered with.
	WithLocalStorage DisabledDefaultPodProtection = "withLocalStorage"

	// DaemonSetPods indicates disabling protection for Pods managed by DaemonSet.
	// Once disabled, Pods created by DaemonSet may be deleted or interfered with.
	DaemonSetPods DisabledDefaultPodProtection = "daemonSetPods"

	// SystemCriticalPods indicates disabling protection for system-critical Pods.
	// Once disabled, system-critical Pods may be deleted or interfered with.
	SystemCriticalPods DisabledDefaultPodProtection = "systemCriticalPods"

	// FailedBarePods indicates disabling protection for failed bare Pods (Pods not bound to ReplicaSet or Deployment).
	// Once disabled, failed bare Pods may be deleted or interfered with.
	FailedBarePods DisabledDefaultPodProtection = "failedBarePods"
)

// ExtraPodProtection defines the type for additional Pod protection mechanisms that users can enable.
// Each value represents an extra protection rule that can be enabled.
type ExtraPodProtection string

const (
	// WithPVC indicates enabling protection for Pods using Persistent Volume Claims (PVC).
	// Once enabled, Pods using PVCs will be protected from deletion or interference.
	WithPVC ExtraPodProtection = "withPVC"

	// WithoutPDB indicates enabling protection for Pods without a Pod Disruption Budget (PDB).
	// Once enabled, Pods without a configured PDB will be protected from deletion or interference.
	WithoutPDB ExtraPodProtection = "withoutPDB"
)
