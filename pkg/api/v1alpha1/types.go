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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type DeschedulerPolicy struct {
	metav1.TypeMeta `json:",inline"`

	// Strategies
	Strategies StrategyList `json:"strategies,omitempty"`

	// NodeSelector for a set of nodes to operate over
	NodeSelector *string `json:"nodeSelector,omitempty"`

	// EvictLocalStoragePods allows pods using local storage to be evicted.
	EvictLocalStoragePods *bool `json:"evictLocalStoragePods,omitempty"`

	// MaxNoOfPodsToEvictPerNode restricts maximum of pods to be evicted per node.
	MaxNoOfPodsToEvictPerNode *int `json:"maxNoOfPodsToEvictPerNode,omitempty"`
}

type StrategyName string
type StrategyList map[StrategyName]DeschedulerStrategy

type DeschedulerStrategy struct {
	// Enabled or disabled
	Enabled bool `json:"enabled,omitempty"`

	// Weight
	Weight int `json:"weight,omitempty"`

	// Strategy parameters
	Params *StrategyParameters `json:"params,omitempty"`
}

// Namespaces carries a list of included/excluded namespaces
// for which a given strategy is applicable.
type Namespaces struct {
	Include []string `json:"include"`
	Exclude []string `json:"exclude"`
}

// Besides Namespaces ThresholdPriority and ThresholdPriorityClassName only one of its members may be specified
type StrategyParameters struct {
	NodeResourceUtilizationThresholds *NodeResourceUtilizationThresholds `json:"nodeResourceUtilizationThresholds,omitempty"`
	NodeAffinityType                  []string                           `json:"nodeAffinityType,omitempty"`
	PodsHavingTooManyRestarts         *PodsHavingTooManyRestarts         `json:"podsHavingTooManyRestarts,omitempty"`
	MaxPodLifeTimeSeconds             *uint                              `json:"maxPodLifeTimeSeconds,omitempty"`
	RemoveDuplicates                  *RemoveDuplicates                  `json:"removeDuplicates,omitempty"`
	Namespaces                        *Namespaces                        `json:"namespaces"`
	ThresholdPriority                 *int32                             `json:"thresholdPriority"`
	ThresholdPriorityClassName        string                             `json:"thresholdPriorityClassName"`
}

type Percentage float64
type ResourceThresholds map[v1.ResourceName]Percentage

type NodeResourceUtilizationThresholds struct {
	Thresholds       ResourceThresholds `json:"thresholds,omitempty"`
	TargetThresholds ResourceThresholds `json:"targetThresholds,omitempty"`
	NumberOfNodes    int                `json:"numberOfNodes,omitempty"`
}

type PodsHavingTooManyRestarts struct {
	PodRestartThreshold     int32 `json:"podRestartThreshold,omitempty"`
	IncludingInitContainers bool  `json:"includingInitContainers,omitempty"`
}

type RemoveDuplicates struct {
	ExcludeOwnerKinds []string `json:"excludeOwnerKinds,omitempty"`
}
