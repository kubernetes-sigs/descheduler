/*
Copyright 2018 The Kubernetes Authors.

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

package priorities

import (
	"fmt"
	"math"

	"k8s.io/klog"
	framework "k8s.io/kubernetes/pkg/scheduler/framework/v1alpha1"
)

// FunctionShape represents shape of scoring function.
// For safety use NewFunctionShape which performs precondition checks for struct creation.
type FunctionShape []FunctionShapePoint

// FunctionShapePoint represents single point in scoring function shape.
type FunctionShapePoint struct {
	// Utilization is function argument.
	Utilization int64
	// Score is function value.
	Score int64
}

var (
	// give priority to least utilized nodes by default
	defaultFunctionShape, _ = NewFunctionShape([]FunctionShapePoint{
		{
			Utilization: 0,
			Score:       framework.MaxNodeScore,
		},
		{
			Utilization: 100,
			Score:       framework.MinNodeScore,
		},
	})
)

const (
	minUtilization = 0
	maxUtilization = 100
	minScore       = 0
	maxScore       = framework.MaxNodeScore
)

// NewFunctionShape creates instance of FunctionShape in a safe way performing all
// necessary sanity checks.
func NewFunctionShape(points []FunctionShapePoint) (FunctionShape, error) {

	n := len(points)

	if n == 0 {
		return nil, fmt.Errorf("at least one point must be specified")
	}

	for i := 1; i < n; i++ {
		if points[i-1].Utilization >= points[i].Utilization {
			return nil, fmt.Errorf("utilization values must be sorted. Utilization[%d]==%d >= Utilization[%d]==%d", i-1, points[i-1].Utilization, i, points[i].Utilization)
		}
	}

	for i, point := range points {
		if point.Utilization < minUtilization {
			return nil, fmt.Errorf("utilization values must not be less than %d. Utilization[%d]==%d", minUtilization, i, point.Utilization)
		}
		if point.Utilization > maxUtilization {
			return nil, fmt.Errorf("utilization values must not be greater than %d. Utilization[%d]==%d", maxUtilization, i, point.Utilization)
		}
		if point.Score < minScore {
			return nil, fmt.Errorf("score values must not be less than %d. Score[%d]==%d", minScore, i, point.Score)
		}
		if point.Score > maxScore {
			return nil, fmt.Errorf("score valuses not be greater than %d. Score[%d]==%d", maxScore, i, point.Score)
		}
	}

	// We make defensive copy so we make no assumption if array passed as argument is not changed afterwards
	pointsCopy := make(FunctionShape, n)
	copy(pointsCopy, points)
	return pointsCopy, nil
}

func validateResourceWeightMap(resourceToWeightMap ResourceToWeightMap) error {
	if len(resourceToWeightMap) == 0 {
		return fmt.Errorf("resourceToWeightMap cannot be nil")
	}

	for resource, weight := range resourceToWeightMap {
		if weight < 1 {
			return fmt.Errorf("resource %s weight %d must not be less than 1", string(resource), weight)
		}
	}
	return nil
}

// RequestedToCapacityRatioResourceAllocationPriorityDefault creates a requestedToCapacity based
// ResourceAllocationPriority using default resource scoring function shape.
// The default function assigns 1.0 to resource when all capacity is available
// and 0.0 when requested amount is equal to capacity.
func RequestedToCapacityRatioResourceAllocationPriorityDefault() *ResourceAllocationPriority {
	return RequestedToCapacityRatioResourceAllocationPriority(defaultFunctionShape, DefaultRequestedRatioResources)
}

// RequestedToCapacityRatioResourceAllocationPriority creates a requestedToCapacity based
// ResourceAllocationPriority using provided resource scoring function shape.
func RequestedToCapacityRatioResourceAllocationPriority(scoringFunctionShape FunctionShape, resourceToWeightMap ResourceToWeightMap) *ResourceAllocationPriority {
	return &ResourceAllocationPriority{"RequestedToCapacityRatioResourceAllocationPriority", buildRequestedToCapacityRatioScorerFunction(scoringFunctionShape, resourceToWeightMap), resourceToWeightMap}
}

func buildRequestedToCapacityRatioScorerFunction(scoringFunctionShape FunctionShape, resourceToWeightMap ResourceToWeightMap) func(ResourceToValueMap, ResourceToValueMap, bool, int, int) int64 {
	rawScoringFunction := buildBrokenLinearFunction(scoringFunctionShape)
	err := validateResourceWeightMap(resourceToWeightMap)
	if err != nil {
		klog.Error(err)
	}
	resourceScoringFunction := func(requested, capacity int64) int64 {
		if capacity == 0 || requested > capacity {
			return rawScoringFunction(maxUtilization)
		}

		return rawScoringFunction(maxUtilization - (capacity-requested)*maxUtilization/capacity)
	}
	return func(requested, allocable ResourceToValueMap, includeVolumes bool, requestedVolumes int, allocatableVolumes int) int64 {
		var nodeScore, weightSum int64
		for resource, weight := range resourceToWeightMap {
			resourceScore := resourceScoringFunction(requested[resource], allocable[resource])
			if resourceScore > 0 {
				nodeScore += resourceScore * weight
				weightSum += weight
			}
		}
		if weightSum == 0 {
			return 0
		}
		return int64(math.Round(float64(nodeScore) / float64(weightSum)))
	}
}

// Creates a function which is built using linear segments. Segments are defined via shape array.
// Shape[i].Utilization slice represents points on "utilization" axis where different segments meet.
// Shape[i].Score represents function values at meeting points.
//
// function f(p) is defined as:
//   shape[0].Score for p < f[0].Utilization
//   shape[i].Score for p == shape[i].Utilization
//   shape[n-1].Score for p > shape[n-1].Utilization
// and linear between points (p < shape[i].Utilization)
func buildBrokenLinearFunction(shape FunctionShape) func(int64) int64 {
	n := len(shape)
	return func(p int64) int64 {
		for i := 0; i < n; i++ {
			if p <= shape[i].Utilization {
				if i == 0 {
					return shape[0].Score
				}
				return shape[i-1].Score + (shape[i].Score-shape[i-1].Score)*(p-shape[i-1].Utilization)/(shape[i].Utilization-shape[i-1].Utilization)
			}
		}
		return shape[n-1].Score
	}
}
