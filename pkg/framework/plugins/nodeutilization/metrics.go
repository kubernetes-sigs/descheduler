/*
Copyright 2025 The Kubernetes Authors.

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

package nodeutilization

import (
	"sync"

	"k8s.io/component-base/metrics"
	"k8s.io/component-base/metrics/legacyregistry"
	deschedulermetrics "sigs.k8s.io/descheduler/metrics"
)

const (
	lowNodeUtilizationSubsystem = deschedulermetrics.DeschedulerSubsystem + "_lownodeutilization"
)

var (
	// LowNodeUtilizationThresholdMetric tracks threshold values for node utilization (0-1 range)
	LowNodeUtilizationThresholdMetric = metrics.NewGaugeVec(
		&metrics.GaugeOpts{
			Subsystem:      lowNodeUtilizationSubsystem,
			Name:           "node_utilization_threshold",
			Help:           "Threshold values for node utilization (0-1 range)",
			StabilityLevel: metrics.ALPHA,
		}, []string{
			"node",           // Node name
			"resource",       // Resource type (cpu, memory, etc.)
			"threshold_type", // "low" or "high"
		})

	// LowNodeUtilizationValueMetric tracks actual utilization values for nodes (0-1 range)
	LowNodeUtilizationValueMetric = metrics.NewGaugeVec(
		&metrics.GaugeOpts{
			Subsystem:      lowNodeUtilizationSubsystem,
			Name:           "node_utilization_value",
			Help:           "Actual utilization values for nodes (0-1 range)",
			StabilityLevel: metrics.ALPHA,
		}, []string{"node", "resource"})

	// LowNodeUtilizationClassificationMetric tracks node classification result
	// 0=underutilized, 1=appropriately utilized, 2=overutilized
	LowNodeUtilizationClassificationMetric = metrics.NewGaugeVec(
		&metrics.GaugeOpts{
			Subsystem:      lowNodeUtilizationSubsystem,
			Name:           "node_classification",
			Help:           "Node classification result: 0=underutilized, 1=appropriately utilized, 2=overutilized",
			StabilityLevel: metrics.ALPHA,
		}, []string{"node"})

	// LowNodeUtilizationThresholdModeMetric tracks threshold mode: 0=static, 1=deviation-based
	LowNodeUtilizationThresholdModeMetric = metrics.NewGauge(
		&metrics.GaugeOpts{
			Subsystem:      lowNodeUtilizationSubsystem,
			Name:           "threshold_mode",
			Help:           "Threshold mode: 0=static, 1=deviation-based",
			StabilityLevel: metrics.ALPHA,
		})

	lowNodeUtilizationMetricsList = []metrics.Registerable{
		LowNodeUtilizationThresholdMetric,
		LowNodeUtilizationValueMetric,
		LowNodeUtilizationClassificationMetric,
		LowNodeUtilizationThresholdModeMetric,
	}
)

var registerLowNodeUtilizationMetrics sync.Once

// RegisterMetrics registers the LowNodeUtilization metrics.
func RegisterMetrics() {
	registerLowNodeUtilizationMetrics.Do(func() {
		for _, metric := range lowNodeUtilizationMetricsList {
			legacyregistry.MustRegister(metric)
		}
	})
}
