/*
Copyright 2024 The Kubernetes Authors.

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

package metricscollector

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	listercorev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
	utilptr "k8s.io/utils/ptr"
	"sigs.k8s.io/descheduler/pkg/api"
)

const (
	beta float64 = 0.9
)

type MetricsCollector struct {
	nodeLister       listercorev1.NodeLister
	metricsClientset metricsclient.Interface
	nodeSelector     labels.Selector

	nodes map[string]api.ReferencedResourceList

	mu sync.RWMutex
	// hasSynced signals at least one sync succeeded
	hasSynced bool
}

func NewMetricsCollector(nodeLister listercorev1.NodeLister, metricsClientset metricsclient.Interface, nodeSelector labels.Selector) *MetricsCollector {
	return &MetricsCollector{
		nodeLister:       nodeLister,
		metricsClientset: metricsClientset,
		nodeSelector:     nodeSelector,
		nodes:            make(map[string]api.ReferencedResourceList),
	}
}

func (mc *MetricsCollector) Run(ctx context.Context) {
	wait.NonSlidingUntil(func() {
		mc.Collect(ctx)
	}, 5*time.Second, ctx.Done())
}

// During experiments rounding to int error causes weightedAverage to never
// reach value even when weightedAverage is repeated many times in a row.
// The difference between the limit and computed average stops within 5 units.
// Nevertheless, the value is expected to change in time. So the weighted
// average nevers gets a chance to converge. Which makes the computed
// error negligible.
// The speed of convergence depends on how often the metrics collector
// syncs with the current value. Currently, the interval is set to 5s.
func weightedAverage(prevValue, value int64) int64 {
	return int64(math.Round(beta*float64(prevValue) + (1-beta)*float64(value)))
}

func (mc *MetricsCollector) AllNodesUsage() (map[string]api.ReferencedResourceList, error) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	allNodesUsage := make(map[string]api.ReferencedResourceList)
	for nodeName := range mc.nodes {
		allNodesUsage[nodeName] = api.ReferencedResourceList{
			v1.ResourceCPU:    utilptr.To[resource.Quantity](mc.nodes[nodeName][v1.ResourceCPU].DeepCopy()),
			v1.ResourceMemory: utilptr.To[resource.Quantity](mc.nodes[nodeName][v1.ResourceMemory].DeepCopy()),
		}
	}

	return allNodesUsage, nil
}

func (mc *MetricsCollector) NodeUsage(node *v1.Node) (api.ReferencedResourceList, error) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	if _, exists := mc.nodes[node.Name]; !exists {
		klog.V(4).InfoS("unable to find node in the collected metrics", "node", klog.KObj(node))
		return nil, fmt.Errorf("unable to find node %q in the collected metrics", node.Name)
	}
	return api.ReferencedResourceList{
		v1.ResourceCPU:    utilptr.To[resource.Quantity](mc.nodes[node.Name][v1.ResourceCPU].DeepCopy()),
		v1.ResourceMemory: utilptr.To[resource.Quantity](mc.nodes[node.Name][v1.ResourceMemory].DeepCopy()),
	}, nil
}

func (mc *MetricsCollector) HasSynced() bool {
	return mc.hasSynced
}

func (mc *MetricsCollector) MetricsClient() metricsclient.Interface {
	return mc.metricsClientset
}

func (mc *MetricsCollector) Collect(ctx context.Context) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	nodes, err := mc.nodeLister.List(mc.nodeSelector)
	if err != nil {
		return fmt.Errorf("unable to list nodes: %v", err)
	}

	for _, node := range nodes {
		metrics, err := mc.metricsClientset.MetricsV1beta1().NodeMetricses().Get(ctx, node.Name, metav1.GetOptions{})
		if err != nil {
			klog.ErrorS(err, "Error fetching metrics", "node", node.Name)
			// No entry -> duplicate the previous value -> do nothing as beta*PV + (1-beta)*PV = PV
			continue
		}

		if _, exists := mc.nodes[node.Name]; !exists {
			mc.nodes[node.Name] = api.ReferencedResourceList{
				v1.ResourceCPU:    utilptr.To[resource.Quantity](metrics.Usage.Cpu().DeepCopy()),
				v1.ResourceMemory: utilptr.To[resource.Quantity](metrics.Usage.Memory().DeepCopy()),
			}
		} else {
			// get MilliValue to reduce loss of precision
			mc.nodes[node.Name][v1.ResourceCPU].SetMilli(
				weightedAverage(mc.nodes[node.Name][v1.ResourceCPU].MilliValue(), metrics.Usage.Cpu().MilliValue()),
			)
			mc.nodes[node.Name][v1.ResourceMemory].Set(
				weightedAverage(mc.nodes[node.Name][v1.ResourceMemory].Value(), metrics.Usage.Memory().Value()),
			)
		}
	}

	mc.hasSynced = true
	return nil
}
