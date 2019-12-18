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

package winkernel

import (
	"sync"
	"time"

	"k8s.io/component-base/metrics"
	"k8s.io/component-base/metrics/legacyregistry"
)

const kubeProxySubsystem = "kubeproxy"

var (
	SyncProxyRulesLatency = metrics.NewHistogram(
		&metrics.HistogramOpts{
			Subsystem:      kubeProxySubsystem,
			Name:           "sync_proxy_rules_duration_seconds",
			Help:           "SyncProxyRules latency in seconds",
			Buckets:        metrics.ExponentialBuckets(0.001, 2, 15),
			StabilityLevel: metrics.ALPHA,
		},
	)

	DeprecatedSyncProxyRulesLatency = metrics.NewHistogram(
		&metrics.HistogramOpts{
			Subsystem:         kubeProxySubsystem,
			Name:              "sync_proxy_rules_latency_microseconds",
			Help:              "SyncProxyRules latency in microseconds",
			Buckets:           metrics.ExponentialBuckets(1000, 2, 15),
			StabilityLevel:    metrics.ALPHA,
			DeprecatedVersion: "1.14.0",
		},
	)

	// SyncProxyRulesLastTimestamp is the timestamp proxy rules were last
	// successfully synced.
	SyncProxyRulesLastTimestamp = metrics.NewGauge(
		&metrics.GaugeOpts{
			Subsystem:      kubeProxySubsystem,
			Name:           "sync_proxy_rules_last_timestamp_seconds",
			Help:           "The last time proxy rules were successfully synced",
			StabilityLevel: metrics.ALPHA,
		},
	)
)

var registerMetricsOnce sync.Once

func RegisterMetrics() {
	registerMetricsOnce.Do(func() {
		legacyregistry.MustRegister(SyncProxyRulesLatency)
		legacyregistry.MustRegister(DeprecatedSyncProxyRulesLatency)
		legacyregistry.MustRegister(SyncProxyRulesLastTimestamp)
	})
}

// Gets the time since the specified start in microseconds.
func sinceInMicroseconds(start time.Time) float64 {
	return float64(time.Since(start).Nanoseconds() / time.Microsecond.Nanoseconds())
}

// Gets the time since the specified start in seconds.
func sinceInSeconds(start time.Time) float64 {
	return time.Since(start).Seconds()
}
