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
	"k8s.io/component-base/metrics"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
)

var (
	// ThresholdsMetric tracks thresholds used by the LowNodeUtilization plugin
	ThresholdsMetric *metrics.GaugeVec

	// ClassificationMetric tracks number of nodes by class
	ClassificationMetric *metrics.GaugeVec
)

// RegisterMetrics registers the plugin's metrics using the provided registry
func RegisterMetrics(registry frameworktypes.MetricsRegistry, profileName string) {
	pluginSubsystem := registry.GetPluginSubsystem(profileName, LowNodeUtilizationPluginName)

	// Initialize metrics with the plugin-specific subsystem
	ThresholdsMetric = metrics.NewGaugeVec(
		&metrics.GaugeOpts{
			Subsystem:      pluginSubsystem,
			Name:           "thresholds",
			Help:           "Thresholds used by the LowNodeUtilization to classify nodes by node, by class, by resource",
			StabilityLevel: metrics.ALPHA,
		},
		[]string{"node", "class", "resource"},
	)

	ClassificationMetric = metrics.NewGaugeVec(
		&metrics.GaugeOpts{
			Subsystem:      pluginSubsystem,
			Name:           "classification",
			Help:           "Number of nodes by class",
			StabilityLevel: metrics.ALPHA,
		},
		[]string{"class"},
	)

	// Register the metrics with names in the centralized registry
	namedMetrics := map[string]metrics.Registerable{
		"thresholds":     ThresholdsMetric,
		"classification": ClassificationMetric,
	}

	registry.RegisterNamedPluginMetrics(LowNodeUtilizationPluginName, namedMetrics)
}
