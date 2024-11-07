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

	"k8s.io/klog/v2"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	utilptr "k8s.io/utils/ptr"
)

const (
	beta float64 = 0.9
)

type MetricsCollector struct {
	clientset        kubernetes.Interface
	metricsClientset metricsclient.Interface

	nodes map[string]map[v1.ResourceName]*resource.Quantity

	mu sync.Mutex
	// hasSynced signals at least one sync succeeded
	hasSynced bool
}

func NewMetricsCollector(clientset kubernetes.Interface, metricsClientset metricsclient.Interface) *MetricsCollector {
	return &MetricsCollector{
		clientset:        clientset,
		metricsClientset: metricsClientset,
		nodes:            make(map[string]map[v1.ResourceName]*resource.Quantity),
	}
}

func (mc *MetricsCollector) Run(ctx context.Context) {
	wait.NonSlidingUntil(func() {
		mc.Collect(ctx)
	}, 5*time.Second, ctx.Done())
}

func weightedAverage(prevValue, value int64) int64 {
	return int64(math.Floor(beta*float64(prevValue) + (1-beta)*float64(value)))
}

func (mc *MetricsCollector) NodeUsage(node *v1.Node) (map[v1.ResourceName]*resource.Quantity, error) {
	if mc == nil {
		return nil, fmt.Errorf("metrics collector not initialized")
	}
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if _, exists := mc.nodes[node.Name]; !exists {
		klog.V(4).Infof("unable to find node %q in the collected metrics", node.Name)
		return nil, fmt.Errorf("unable to find node %q in the collected metrics", node.Name)
	}
	return map[v1.ResourceName]*resource.Quantity{
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
	if mc == nil {
		return fmt.Errorf("metrics collector not initialized")
	}
	mc.mu.Lock()
	defer mc.mu.Unlock()
	nodes, err := mc.clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("unable to list nodes: %v", err)
	}

	for _, node := range nodes.Items {
		metrics, err := mc.metricsClientset.MetricsV1beta1().NodeMetricses().Get(ctx, node.Name, metav1.GetOptions{})
		if err != nil {
			klog.Errorf("Error fetching metrics for node %s: %v\n", node.Name, err)
			// No entry -> duplicate the previous value -> do nothing as beta*PV + (1-beta)*PV = PV
			continue
		}

		if _, exists := mc.nodes[node.Name]; !exists {
			mc.nodes[node.Name] = map[v1.ResourceName]*resource.Quantity{
				v1.ResourceCPU:    utilptr.To[resource.Quantity](metrics.Usage.Cpu().DeepCopy()),
				v1.ResourceMemory: utilptr.To[resource.Quantity](metrics.Usage.Memory().DeepCopy()),
			}
		} else {
			// get MilliValue to reduce loss of precision
			mc.nodes[node.Name][v1.ResourceCPU].SetMilli(
				weightedAverage(mc.nodes[node.Name][v1.ResourceCPU].MilliValue(), metrics.Usage.Cpu().MilliValue()),
			)
			mc.nodes[node.Name][v1.ResourceMemory].SetMilli(
				weightedAverage(mc.nodes[node.Name][v1.ResourceMemory].MilliValue(), metrics.Usage.Memory().MilliValue()),
			)
		}
	}

	mc.hasSynced = true
	return nil
}
