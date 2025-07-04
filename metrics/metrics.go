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
	PodsEvicted = metrics.NewCounterVec(
		&metrics.CounterOpts{
			Subsystem:         DeschedulerSubsystem,
			Name:              "pods_evicted",
			Help:              "Number of total evicted pods, by the result, by the strategy, by the namespace, by the node name. 'error' result means a pod could not be evicted",
			StabilityLevel:    metrics.ALPHA,
			DeprecatedVersion: "0.34.0",
		}, []string{"result", "strategy", "profile", "namespace", "node"})
	PodsEvictedTotal = metrics.NewCounterVec(
		&metrics.CounterOpts{
			Subsystem:      DeschedulerSubsystem,
			Name:           "pods_evicted_total",
			Help:           "Number of total evicted pods, by the result, by the strategy, by the namespace, by the node name. 'error' result means a pod could not be evicted",
			StabilityLevel: metrics.ALPHA,
		}, []string{"result", "strategy", "profile", "namespace", "node"})

	buildInfo = metrics.NewGauge(
		&metrics.GaugeOpts{
			Subsystem:      DeschedulerSubsystem,
			Name:           "build_info",
			Help:           "Build info about descheduler, including Go version, Descheduler version, Git SHA, Git branch",
			ConstLabels:    map[string]string{"GoVersion": version.Get().GoVersion, "AppVersion": version.Get().Major + "." + version.Get().Minor, "DeschedulerVersion": version.Get().GitVersion, "GitBranch": version.Get().GitBranch, "GitSha1": version.Get().GitSha1},
			StabilityLevel: metrics.ALPHA,
		},
	)

	DeschedulerLoopDuration = metrics.NewHistogramVec(
		&metrics.HistogramOpts{
			Subsystem:         DeschedulerSubsystem,
			Name:              "descheduler_loop_duration_seconds",
			Help:              "Time taken to complete a full descheduling cycle",
			StabilityLevel:    metrics.ALPHA,
			DeprecatedVersion: "0.34.0",
			Buckets:           []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 25, 50, 100, 250, 500},
		}, []string{})
	LoopDuration = metrics.NewHistogramVec(
		&metrics.HistogramOpts{
			Subsystem:      DeschedulerSubsystem,
			Name:           "loop_duration_seconds",
			Help:           "Time taken to complete a full descheduling cycle",
			StabilityLevel: metrics.ALPHA,
			Buckets:        []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 25, 50, 100, 250, 500},
		}, []string{})

	DeschedulerStrategyDuration = metrics.NewHistogramVec(
		&metrics.HistogramOpts{
			Subsystem:         DeschedulerSubsystem,
			Name:              "descheduler_strategy_duration_seconds",
			Help:              "Time taken to complete Each strategy of the descheduling operation",
			StabilityLevel:    metrics.ALPHA,
			DeprecatedVersion: "0.34.0",
			Buckets:           []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 25, 50, 100},
		}, []string{"strategy", "profile"})
	StrategyDuration = metrics.NewHistogramVec(
		&metrics.HistogramOpts{
			Subsystem:      DeschedulerSubsystem,
			Name:           "strategy_duration_seconds",
			Help:           "Time taken to complete Each strategy of the descheduling operation",
			StabilityLevel: metrics.ALPHA,
			Buckets:        []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 25, 50, 100},
		}, []string{"strategy", "profile"})

	metricsList = []metrics.Registerable{
		PodsEvicted,
		PodsEvictedTotal,
		buildInfo,
		DeschedulerLoopDuration,
		DeschedulerStrategyDuration,
		LoopDuration,
		StrategyDuration,
	}
)

var registerMetrics sync.Once

// Register all metrics.
func Register() {
	// Register the metrics.
	registerMetrics.Do(func() {
		RegisterMetrics(metricsList...)
	})
}

// RegisterMetrics registers a list of metrics.
func RegisterMetrics(extraMetrics ...metrics.Registerable) {
	for _, metric := range extraMetrics {
		legacyregistry.MustRegister(metric)
	}
}
