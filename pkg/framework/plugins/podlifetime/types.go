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

package podlifetime

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/descheduler/pkg/api"
)

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PodLifeTimeArgs holds arguments used to configure PodLifeTime plugin.
type PodLifeTimeArgs struct {
	metav1.TypeMeta `json:",inline"`

	Namespaces    *api.Namespaces       `json:"namespaces,omitempty"`
	LabelSelector *metav1.LabelSelector `json:"labelSelector,omitempty"`
	OwnerKinds    *OwnerKinds           `json:"ownerKinds,omitempty"`

	MaxPodLifeTimeSeconds *uint `json:"maxPodLifeTimeSeconds,omitempty"`

	// States filters pods by phase, pod status reason, container waiting reason,
	// or container terminated reason. A pod matches if any of its states appear
	// in this list.
	States []string `json:"states,omitempty"`

	// Conditions filters pods by status.conditions entries. A pod matches if
	// any of its conditions satisfy at least one filter. Each filter can
	// optionally require a minimum time since the condition last transitioned.
	Conditions []PodConditionFilter `json:"conditions,omitempty"`

	// ExitCodes filters by container terminated exit codes.
	ExitCodes []int32 `json:"exitCodes,omitempty"`

	IncludingInitContainers      bool `json:"includingInitContainers,omitempty"`
	IncludingEphemeralContainers bool `json:"includingEphemeralContainers,omitempty"`
}

// +k8s:deepcopy-gen=true

// OwnerKinds allows filtering pods by owner reference kinds with include/exclude support.
// At most one of Include/Exclude may be set.
type OwnerKinds struct {
	Include []string `json:"include,omitempty"`
	Exclude []string `json:"exclude,omitempty"`
}

// +k8s:deepcopy-gen=true

// PodConditionFilter matches a pod condition by type, status, and/or reason.
// All specified fields must match (AND). Unset fields are not checked.
// When MinTimeSinceLastTransitionSeconds is set, the condition must also have
// a lastTransitionTime older than this many seconds.
type PodConditionFilter struct {
	Type   string `json:"type,omitempty"`
	Status string `json:"status,omitempty"`
	Reason string `json:"reason,omitempty"`

	// MinTimeSinceLastTransitionSeconds requires the matching condition's
	// lastTransitionTime to be at least this many seconds in the past.
	MinTimeSinceLastTransitionSeconds *uint `json:"minTimeSinceLastTransitionSeconds,omitempty"`
}
