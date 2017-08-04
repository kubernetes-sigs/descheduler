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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/api"
)

type ReschedulerPolicy struct {
	metav1.TypeMeta

	// Time interval for rescheduler to run
	ReschedulingInterval time.Duration

	// Strategies
	Strategies StrategyList
}

type StrategyName string
type StrategyList map[StrategyName]ReschedulerStrategy

type ReschedulerStrategy struct {
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
}

type Percentage int
type ResourceThresholds map[api.ResourceName]Percentage

type NodeResourceUtilizationThresholds struct {
	Thresholds       ResourceThresholds
	TargetThresholds ResourceThresholds
}
