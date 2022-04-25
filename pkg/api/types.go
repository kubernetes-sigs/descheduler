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
	"k8s.io/apimachinery/pkg/runtime"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type DeschedulerPolicy struct {
	metav1.TypeMeta

	// Strategies
	Strategies StrategyList

	// NodeSelector for a set of nodes to operate over
	NodeSelector *string

	// EvictFailedBarePods allows pods without ownerReferences and in failed phase to be evicted.
	EvictFailedBarePods *bool

	// EvictLocalStoragePods allows pods using local storage to be evicted.
	EvictLocalStoragePods *bool

	// EvictSystemCriticalPods allows eviction of pods of any priority (including Kubernetes system pods)
	EvictSystemCriticalPods *bool

	// IgnorePVCPods prevents pods with PVCs from being evicted.
	IgnorePVCPods *bool

	// MaxNoOfPodsToEvictPerNode restricts maximum of pods to be evicted per node.
	MaxNoOfPodsToEvictPerNode *uint

	// MaxNoOfPodsToEvictPerNamespace restricts maximum of pods to be evicted per namespace.
	MaxNoOfPodsToEvictPerNamespace *uint
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

type PriorityThreshold struct {
	Value *int32
	Name  string
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
	FailedPods                        *FailedPods
	IncludeSoftConstraints            bool
	Namespaces                        *Namespaces
	ThresholdPriority                 *int32
	ThresholdPriorityClassName        string
	LabelSelector                     *metav1.LabelSelector
	NodeFit                           bool
	IncludePreferNoSchedule           bool
	ExcludedTaints                    []string
}

type Percentage float64
type ResourceThresholds map[v1.ResourceName]Percentage

type NodeResourceUtilizationThresholds struct {
	UseDeviationThresholds bool
	Thresholds             ResourceThresholds
	TargetThresholds       ResourceThresholds
	NumberOfNodes          int
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

type FailedPods struct {
	ExcludeOwnerKinds       []string
	MinPodLifetimeSeconds   *uint
	Reasons                 []string
	IncludingInitContainers bool
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type DeschedulerConfiguration struct {
	metav1.TypeMeta

	// Profiles
	Profiles []Profile

	// NodeSelector for a set of nodes to operate over
	NodeSelector *string

	// MaxNoOfPodsToEvictPerNode restricts maximum of pods to be evicted per node.
	MaxNoOfPodsToEvictPerNode *int

	// MaxNoOfPodsToEvictPerNamespace restricts maximum of pods to be evicted per namespace.
	MaxNoOfPodsToEvictPerNamespace *int
}

type Profile struct {
	Name         string
	PluginConfig []PluginConfig
	Plugins      Plugins
}

type Plugins struct {
	PreSort    Plugin
	Deschedule Plugin
	Balance    Plugin
	Sort       Plugin
	Evict      Plugin
}

type PluginConfig struct {
	Name string
	Args runtime.Object
}

type Plugin struct {
	Enabled  []string
	Disabled []string
}

// RemoveDuplicatePodsArgs holds arguments used to configure the RemoveDuplicatePods plugin.
type RemoveDuplicatePodsArgs struct {
	metav1.TypeMeta

	Namespaces        *Namespaces
	ExcludeOwnerKinds []string
}

// RemoveFailedPodsArgs holds arguments used to configure the RemoveFailedPods plugin.
type RemoveFailedPodsArgs struct {
	metav1.TypeMeta

	Namespaces              *Namespaces
	LabelSelector           *metav1.LabelSelector
	MinPodLifetimeSeconds   *uint
	Reasons                 []string
	IncludingInitContainers bool
	ExcludeOwnerKinds       []string
}

// RemovePodsViolatingNodeAffinityArgs holds arguments used to configure the RemovePodsViolatingNodeAffinity plugin.
type RemovePodsViolatingNodeAffinityArgs struct {
	metav1.TypeMeta

	Namespaces       *Namespaces
	LabelSelector    *metav1.LabelSelector
	NodeAffinityType []string
}

// RemovePodsViolatingNodeTaintsArgs holds arguments used to configure the RemovePodsViolatingNodeTaints plugin.
type RemovePodsViolatingNodeTaintsArgs struct {
	metav1.TypeMeta

	Namespaces              *Namespaces
	LabelSelector           *metav1.LabelSelector
	IncludePreferNoSchedule bool
	ExcludedTaints          []string
}

// RemovePodsViolatingInterPodAntiAffinityArgs holds arguments used to configure the RemovePodsViolatingInterPodAntiAffinity plugin.
type RemovePodsViolatingInterPodAntiAffinityArgs struct {
	metav1.TypeMeta

	Namespaces    *Namespaces
	LabelSelector *metav1.LabelSelector
}

// PodLifeTimeArgs holds arguments used to configure the PodLifeTime plugin.
type PodLifeTimeArgs struct {
	metav1.TypeMeta

	Namespaces            *Namespaces
	LabelSelector         *metav1.LabelSelector
	MaxPodLifeTimeSeconds *uint
	PodStatusPhases       []string
}

// RemovePodsHavingTooManyRestartsArgs holds arguments used to configure the RemovePodsHavingTooManyRestarts plugin.
type RemovePodsHavingTooManyRestartsArgs struct {
	metav1.TypeMeta

	Namespaces              *Namespaces
	LabelSelector           *metav1.LabelSelector
	PodRestartThreshold     int32
	IncludingInitContainers bool
}

// RemovePodsViolatingTopologySpreadConstraintArgs holds arguments used to configure the RemovePodsViolatingTopologySpreadConstraint plugin.
type RemovePodsViolatingTopologySpreadConstraintArgs struct {
	metav1.TypeMeta

	Namespaces             *Namespaces
	LabelSelector          *metav1.LabelSelector
	IncludeSoftConstraints bool
}

// LowNodeUtilizationArgs holds arguments used to configure the LowNodeUtilization plugin.
type LowNodeUtilizationArgs struct {
	metav1.TypeMeta

	UseDeviationThresholds bool
	Thresholds             ResourceThresholds
	TargetThresholds       ResourceThresholds
	NumberOfNodes          int
}

// HighNodeUtilizationArgs holds arguments used to configure the HighNodeUtilization plugin.
type HighNodeUtilizationArgs struct {
	metav1.TypeMeta

	Thresholds       ResourceThresholds
	TargetThresholds ResourceThresholds
	NumberOfNodes    int
}

// DefaultEvictorArgs holds arguments used to configure the DefaultEvictor plugin.
type DefaultEvictorArgs struct {
	metav1.TypeMeta

	EvictFailedBarePods     bool
	EvictLocalStoragePods   bool
	EvictSystemCriticalPods bool
	IgnorePvcPods           bool
	PriorityThreshold       *PriorityThreshold
	NodeFit                 bool
	LabelSelector           *metav1.LabelSelector
	// TODO(jchaloup): turn it into *metav1.LabelSelector
	NodeSelector string
}
