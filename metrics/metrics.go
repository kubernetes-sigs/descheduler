/*
Copyright 2021 The Kubernetes Authors.

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

package metrics

import (
	"regexp"
	"sync"

	"k8s.io/component-base/metrics"
	"k8s.io/component-base/metrics/legacyregistry"
	"sigs.k8s.io/descheduler/pkg/version"
)

const (
	// DeschedulerSubsystem - subsystem name used by descheduler
	DeschedulerSubsystem = "descheduler"
)

var (
	buildInfo = metrics.NewGauge(
		&metrics.GaugeOpts{
			Subsystem:      DeschedulerSubsystem,
			Name:           "build_info",
			Help:           "Build info about descheduler, including Go version, Descheduler version, Git SHA, Git branch",
			ConstLabels:    map[string]string{"GoVersion": version.Get().GoVersion, "DeschedulerVersion": version.Get().GitVersion, "GitBranch": version.Get().GitBranch, "GitSha1": version.Get().GitSha1},
			StabilityLevel: metrics.ALPHA,
		},
	)

	metricsNameRegex, _ = regexp.Compile("[^a-zA-Z0-9_]+")

	PodsEvicted *metrics.CounterVec
)

var registerMetrics sync.Once

// Register all metrics.
func Register(customLabels []string) {
	// Register the metrics.
	registerMetrics.Do(func() {
		metrics := initialize(customLabels)
		for _, metric := range metrics {
			legacyregistry.MustRegister(metric)
		}
	})
}

// ensure that labels conform to https://prometheus.io/docs/concepts/data_model/#metric-names-and-labels
func ConvertToMetricLabel(label string) string {
	return metricsNameRegex.ReplaceAllString(label, "_")
}

func initialize(customLabels []string) []metrics.Registerable {
	labels := []string{"result", "strategy", "namespace", "node"}
	for _, customLabel := range customLabels {
		labels = append(labels, ConvertToMetricLabel(customLabel))
	}

	PodsEvicted = metrics.NewCounterVec(
		&metrics.CounterOpts{
			Subsystem:      DeschedulerSubsystem,
			Name:           "pods_evicted",
			Help:           "Number of evicted pods, by the result, by the strategy, by the namespace, by the node name. 'error' result means a pod could not be evicted",
			StabilityLevel: metrics.ALPHA,
		},
		labels,
	)

	return []metrics.Registerable{
		PodsEvicted,
	}
}
