/*
Copyright 2023 The Kubernetes Authors.

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

package trimaran

import (
	"sync"
	"time"

	"github.com/paypal/load-watcher/pkg/watcher"
	loadwatcherapi "github.com/paypal/load-watcher/pkg/watcher/api"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

const (
	metricsUpdateIntervalSeconds = 30
)

type Collector interface {
	GetNodeMetrics(nodeName string) ([]watcher.Metric, *watcher.Window)
}

// Collector : get data from load watcher, encapsulating the load watcher and its operations
//
// Trimaran plugins have different, potentially conflicting, objectives. Thus, it is recommended not
// to enable them concurrently. As such, they are currently designed to each have its own Collector.
// If a need arises in the future to enable multiple Trimaran plugins, a restructuring to have a single
// Collector, serving the multiple plugins, may be beneficial for performance reasons.
type collector struct {
	// load watcher client
	client loadwatcherapi.Client
	// data collected by load watcher
	metrics watcher.WatcherMetrics
	// for safe access to metrics
	sync.RWMutex
}

// NewCollector : create an instance of a data collector
func NewCollector(trimaranSpec *TrimaranSpec) Collector {
	klog.V(4).InfoS("Using TrimaranSpec", "type", trimaranSpec.MetricProvider.Type,
		"address", trimaranSpec.MetricProvider.Address, "watcher", trimaranSpec.WatcherAddress)

	c := &collector{}
	if *trimaranSpec.WatcherAddress != "" {
		c.client, _ = loadwatcherapi.NewServiceClient(*trimaranSpec.WatcherAddress)
	} else {
		c.client, _ = loadwatcherapi.NewLibraryClient(
			watcher.MetricsProviderOpts{
				Name:               string(trimaranSpec.MetricProvider.Type),
				Address:            *trimaranSpec.MetricProvider.Address,
				AuthToken:          *trimaranSpec.MetricProvider.Token,
				InsecureSkipVerify: *trimaranSpec.MetricProvider.InsecureSkipVerify,
			})
	}

	// populate metrics before returning
	if err := c.updateMetrics(); err != nil {
		klog.ErrorS(err, "Unable to populate metrics initially")
	}
	// start periodic updates
	go wait.Until(func() {
		if err := c.updateMetrics(); err != nil {
			klog.ErrorS(err, "Unable to update metrics")
		}
	}, time.Second*metricsUpdateIntervalSeconds, wait.NeverStop)
	return c
}

// GetNodeMetrics : get metrics for a node from watcher
func (c *collector) GetNodeMetrics(nodeName string) ([]watcher.Metric, *watcher.Window) {
	c.RLock()
	allMetrics := &c.metrics
	c.RUnlock()
	// This happens if metrics were never populated since scheduler started
	if allMetrics.Data.NodeMetricsMap == nil {
		klog.ErrorS(nil, "Metrics not available from watcher")
		return nil, nil
	}
	// Check if node is new (no metrics yet) or metrics are unavailable due to 404 or 500
	nodeMetrics, ok := allMetrics.Data.NodeMetricsMap[nodeName]
	if !ok {
		klog.ErrorS(nil, "Unable to find metrics for node", "nodeName", nodeName)
		return nil, nil
	}
	return nodeMetrics.Metrics, &allMetrics.Window
}

// updateMetrics : request to load watcher to update all metrics
func (c *collector) updateMetrics() error {
	metrics, err := c.client.GetLatestWatcherMetrics()
	if err != nil {
		klog.ErrorS(err, "Load watcher client failed")
		return err
	}
	c.Lock()
	c.metrics = *metrics
	c.Unlock()
	return nil
}
