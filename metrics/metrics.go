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
	"strings"
	"sync"
	"unicode"

	"k8s.io/component-base/metrics"
	"k8s.io/component-base/metrics/legacyregistry"
	"sigs.k8s.io/descheduler/pkg/version"
)

const (
	// DeschedulerSubsystem - subsystem name used by descheduler
	DeschedulerSubsystem = "descheduler"
)

// MetricsHandler provides a smart wrapper for metrics that handles export logic
type MetricsHandler interface {
	// WithLabelValues is equivalent to metrics.WithLabelValues but respects ShouldExportMetrics
	WithLabelValues(lvs ...string) MetricsHandler
	// Set sets the gauge value (for GaugeVec metrics)
	Set(val float64)
	// Inc increments the counter (for CounterVec metrics)
	Inc()
	// Add adds a value to the counter (for CounterVec metrics)
	Add(val float64)
	// Observe observes a value for histogram (for HistogramVec metrics)
	Observe(val float64)
}

// normalizePluginName converts a plugin name to a normalized form for use in metrics subsystems
// Examples: "LowNodeUtilization" -> "low_node_utilization", "RemovePodsViolatingNodeTaints" -> "remove_pods_violating_node_taints"
func normalizePluginName(pluginName string) string {
	var result strings.Builder
	for i, r := range pluginName {
		if unicode.IsUpper(r) && i > 0 {
			result.WriteRune('_')
		}
		result.WriteRune(unicode.ToLower(r))
	}
	return result.String()
}

// getPluginSubsystem returns the subsystem name for a plugin, optionally including profile name
func getPluginSubsystem(profileName, pluginName string) string {
	normalizedPluginName := normalizePluginName(pluginName)
	if profileName == "" {
		return DeschedulerSubsystem + "_" + normalizedPluginName
	}
	normalizedProfileName := normalizePluginName(profileName)
	return DeschedulerSubsystem + "_" + normalizedProfileName + "_" + normalizedPluginName
}

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

type PluginMetricsRegistry struct {
	pluginMetricsMap map[string]map[string]interface{} // plugin -> metric name -> metric object
	shouldExport     bool
	mutex            sync.RWMutex
}

func NewPluginMetricsRegistry() *PluginMetricsRegistry {
	return &PluginMetricsRegistry{
		pluginMetricsMap: make(map[string]map[string]interface{}),
		shouldExport:     true, // Default to true, can be updated later
	}
}

func (r *PluginMetricsRegistry) SetShouldExport(shouldExport bool) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.shouldExport = shouldExport
}

func (r *PluginMetricsRegistry) ShouldExport() bool {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.shouldExport
}

// RegisterMetricsWithNames registers metrics for a specific plugin with name mapping
func (r *PluginMetricsRegistry) RegisterNamedPluginMetrics(pluginName string, namedMetrics map[string]metrics.Registerable) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Initialize the plugin metrics map if needed
	if r.pluginMetricsMap[pluginName] == nil {
		r.pluginMetricsMap[pluginName] = make(map[string]interface{})
	}

	for name, metric := range namedMetrics {
		// Store in the metrics map
		r.pluginMetricsMap[pluginName][name] = metric

		// Register the metric
		legacyregistry.MustRegister(metric)
	}
}

func (r *PluginMetricsRegistry) GetPluginSubsystem(profileName, pluginName string) string {
	return getPluginSubsystem(profileName, pluginName)
}

func (r *PluginMetricsRegistry) GetPluginMetric(pluginName, metricName string) interface{} {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	if pluginMap, exists := r.pluginMetricsMap[pluginName]; exists {
		if metric, exists := pluginMap[metricName]; exists {
			return metric
		}
	}
	return nil
}

func (r *PluginMetricsRegistry) HandlePluginMetric(pluginName, metricName string) MetricsHandler {
	metric := r.GetPluginMetric(pluginName, metricName)
	return newMetricsHandler(metric, r.ShouldExport())
}

type metricsHandler struct {
	metric        interface{} // Could be *metrics.GaugeVec, *metrics.CounterVec, etc.
	shouldExport  bool
	currentLabels []string
}

func newMetricsHandler(metric interface{}, shouldExport bool) *metricsHandler {
	return &metricsHandler{
		metric:       metric,
		shouldExport: shouldExport,
	}
}

func (h *metricsHandler) WithLabelValues(lvs ...string) MetricsHandler {
	if !h.shouldExport || h.metric == nil {
		return h // Return no-op handler
	}

	newHandler := &metricsHandler{
		metric:        h.metric,
		shouldExport:  h.shouldExport,
		currentLabels: lvs,
	}
	return newHandler
}

func (h *metricsHandler) Set(val float64) {
	if !h.shouldExport || h.metric == nil {
		return
	}

	if gaugeVec, ok := h.metric.(*metrics.GaugeVec); ok {
		gaugeVec.WithLabelValues(h.currentLabels...).Set(val)
	}
}

func (h *metricsHandler) Inc() {
	if !h.shouldExport || h.metric == nil {
		return
	}

	if counterVec, ok := h.metric.(*metrics.CounterVec); ok {
		counterVec.WithLabelValues(h.currentLabels...).Inc()
	}
}

func (h *metricsHandler) Add(val float64) {
	if !h.shouldExport || h.metric == nil {
		return
	}

	if counterVec, ok := h.metric.(*metrics.CounterVec); ok {
		counterVec.WithLabelValues(h.currentLabels...).Add(val)
	}
}

func (h *metricsHandler) Observe(val float64) {
	if !h.shouldExport || h.metric == nil {
		return
	}

	if histogramVec, ok := h.metric.(*metrics.HistogramVec); ok {
		histogramVec.WithLabelValues(h.currentLabels...).Observe(val)
	}
}

var PluginRegistry = NewPluginMetricsRegistry()

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
