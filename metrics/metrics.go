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
			Subsystem:      DeschedulerSubsystem,
			Name:           "pods_evicted",
			Help:           "Number of evicted pods, by the result, by the strategy, by the namespace, by the node name. 'error' result means a pod could not be evicted",
			StabilityLevel: metrics.ALPHA,
		}, []string{"result", "strategy", "namespace", "node"})

	PodsEvictedSuccess = metrics.NewCounterVec(
		&metrics.CounterOpts{
			Subsystem:      DeschedulerSubsystem,
			Name:           "pods_evicted_success",
			Help:           "Number of pods successfully evicted per strategy",
			StabilityLevel: metrics.ALPHA,
		}, []string{"strategy", "namespace"})

	PodsEvictedFailed = metrics.NewCounterVec(
		&metrics.CounterOpts{
			Subsystem:      DeschedulerSubsystem,
			Name:           "pods_evicted_failed",
			Help:           "Number of pods failed to evict per strategy",
			StabilityLevel: metrics.ALPHA,
		}, []string{"strategy", "namespace"})

	TotalPodsEvicted = metrics.NewCounterVec(
		&metrics.CounterOpts{
			Subsystem:      DeschedulerSubsystem,
			Name:           "total_pods_evicted",
			Help:           "Total pods evicted so far",
			StabilityLevel: metrics.ALPHA,
		}, []string{"result", "namespace"})

	TotalPodsSkipped = metrics.NewCounterVec(
		&metrics.CounterOpts{
			Subsystem:      DeschedulerSubsystem,
			Name:           "total_pods_skipped",
			Help:           "Total pods skipped for a single run",
			StabilityLevel: metrics.ALPHA,
		}, []string{"result", "namespace"})

	buildInfo = metrics.NewGauge(
		&metrics.GaugeOpts{
			Subsystem:      DeschedulerSubsystem,
			Name:           "build_info",
			Help:           "Build info about descheduler, including Go version, Descheduler version, Git SHA, Git branch",
			ConstLabels:    map[string]string{"GoVersion": version.Get().GoVersion, "DeschedulerVersion": version.Get().GitVersion, "GitBranch": version.Get().GitBranch, "GitSha1": version.Get().GitSha1},
			StabilityLevel: metrics.ALPHA,
		},
	)

	metricsList = []metrics.Registerable{
		PodsEvicted,
		buildInfo,
		PodsEvictedSuccess,
		PodsEvictedFailed,
		TotalPodsEvicted,
		TotalPodsSkipped,
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
