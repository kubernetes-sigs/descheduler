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
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type DeschedulerPolicy struct {
	metav1.TypeMeta

	// Strategies
	Strategies StrategyList
}

type StrategyName string
type StrategyList map[StrategyName]DeschedulerStrategy

type DeschedulerStrategy struct {
	// Enabled or disabled
	Enabled bool

	// Weight
	Weight int

	// Strategy parameters
	Params StrategyParameters
}

// Only one of its members may be specified
type StrategyParameters struct {
	NodeResourceUtilizationThresholds NodeResourceUtilizationThresholds
	NodeAffinityType                  []string
	// TopologySpreadConstraints describes how a group of pods should be spread across topology
	// domains. Descheduler will use these constraints to decide which pods to evict.
	NamespacedTopologySpreadConstraints []NamespacedTopologySpreadConstraint
}

type Percentage float64
type ResourceThresholds map[v1.ResourceName]Percentage

type NodeResourceUtilizationThresholds struct {
	Thresholds       ResourceThresholds
	TargetThresholds ResourceThresholds
	NumberOfNodes    int
}

type NamespacedTopologySpreadConstraint struct {
	Namespace                 string
	TopologySpreadConstraints []TopologySpreadConstraint
}

type TopologySpreadConstraint struct {
	// MaxSkew describes the degree to which pods may be unevenly distributed.
	// It's the maximum permitted difference between the number of matching pods in
	// any two topology domains of a given topology type.
	// For example, in a 3-zone cluster, currently pods with the same labelSelector
	// are "spread" such that zone1 and zone2 each have one pod, but not zone3.
	// - if MaxSkew is 1, incoming pod can only be scheduled to zone3 to become 1/1/1;
	// scheduling it onto zone1(zone2) would make the ActualSkew(2) violate MaxSkew(1)
	// - if MaxSkew is 2, incoming pod can be scheduled to any zone.
	// It's a required value. Default value is 1 and 0 is not allowed.
	MaxSkew int32
	// TopologyKey is the key of node labels. Nodes that have a label with this key
	// and identical values are considered to be in the same topology.
	// We consider each <key, value> as a "bucket", and try to put balanced number
	// of pods into each bucket.
	// It's a required field.
	TopologyKey string
	// LabelSelector is used to find matching pods.
	// Pods that match this label selector are counted to determine the number of pods
	// in their corresponding topology domain.
	// +optional
	LabelSelector *metav1.LabelSelector
}
