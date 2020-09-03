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

package api

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type DeschedulerPolicy struct {
	metav1.TypeMeta

	// Strategies
	Strategies StrategyList

	// NodeSelector for a set of nodes to operate over
	NodeSelector *string

	// EvictLocalStoragePods allows pods using local storage to be evicted.
	EvictLocalStoragePods *bool

	// MaxNoOfPodsToEvictPerNode restricts maximum of pods to be evicted per node.
	MaxNoOfPodsToEvictPerNode *int
}

type StrategyName string
type StrategyList map[StrategyName]DeschedulerStrategy

type DeschedulerStrategy struct {
	// Enabled or disabled
	Enabled bool

	// Weight
	Weight int

	// Strategy parameters
	Params *StrategyParameters
}

// Namespaces carries a list of included/excluded namespaces
// for which a given strategy is applicable
type Namespaces struct {
	Include []string
	Exclude []string
}

// Besides Namespaces only one of its members may be specified
// TODO(jchaloup): move Namespaces ThresholdPriority and ThresholdPriorityClassName to individual strategies
//  once the policy version is bumped to v1alpha2
type StrategyParameters struct {
	NodeResourceUtilizationThresholds *NodeResourceUtilizationThresholds
	NodeAffinityType                  []string
	PodsHavingTooManyRestarts         *PodsHavingTooManyRestarts
	PodLifeTime                       *PodLifeTime
	RemoveDuplicates                  *RemoveDuplicates
	Namespaces                        *Namespaces
	ThresholdPriority                 *int32
	ThresholdPriorityClassName        string
}

type Percentage float64
type ResourceThresholds map[v1.ResourceName]Percentage

type NodeResourceUtilizationThresholds struct {
	Thresholds       ResourceThresholds
	TargetThresholds ResourceThresholds
	NumberOfNodes    int
}

type PodsHavingTooManyRestarts struct {
	PodRestartThreshold     int32
	IncludingInitContainers bool
}

type RemoveDuplicates struct {
	ExcludeOwnerKinds []string
}

type PodLifeTime struct {
	MaxPodLifeTimeSeconds *uint
	PodStatusPhases       []string
}
