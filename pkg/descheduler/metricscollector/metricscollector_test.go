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
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime/schema"

	fakeclientset "k8s.io/client-go/kubernetes/fake"
	fakemetricsclient "k8s.io/metrics/pkg/client/clientset/versioned/fake"
	"sigs.k8s.io/descheduler/test"
)

func checkCpuNodeUsage(t *testing.T, usage map[v1.ResourceName]*resource.Quantity, millicpu int64) {
	t.Logf("current node cpu usage: %v\n", usage[v1.ResourceCPU].MilliValue())
	if usage[v1.ResourceCPU].MilliValue() != millicpu {
		t.Fatalf("cpu node usage expected to be %v, got %v instead", millicpu, usage[v1.ResourceCPU].MilliValue())
	}
}

func TestMetricsCollector(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "nodes"}

	n1 := test.BuildTestNode("n1", 2000, 3000, 10, nil)
	n2 := test.BuildTestNode("n2", 2000, 3000, 10, nil)
	n3 := test.BuildTestNode("n3", 2000, 3000, 10, nil)

	n1metrics := test.BuildNodeMetrics("n1", 400, 1714978816)
	n2metrics := test.BuildNodeMetrics("n2", 1400, 1714978816)
	n3metrics := test.BuildNodeMetrics("n3", 300, 1714978816)

	clientset := fakeclientset.NewSimpleClientset(n1, n2, n3)
	metricsClientset := fakemetricsclient.NewSimpleClientset()
	metricsClientset.Tracker().Create(gvr, n1metrics, "")
	metricsClientset.Tracker().Create(gvr, n2metrics, "")
	metricsClientset.Tracker().Create(gvr, n3metrics, "")

	t.Logf("Set initial node cpu usage to 1400")
	collector := NewMetricsCollector(clientset, metricsClientset)
	collector.Collect(context.TODO())
	nodesUsage, _ := collector.NodeUsage(n2)
	checkCpuNodeUsage(t, nodesUsage, 1400)

	t.Logf("Set current node cpu usage to 500")
	n2metrics.Usage[v1.ResourceCPU] = *resource.NewMilliQuantity(500, resource.DecimalSI)
	metricsClientset.Tracker().Update(gvr, n2metrics, "")
	collector.Collect(context.TODO())
	nodesUsage, _ = collector.NodeUsage(n2)
	checkCpuNodeUsage(t, nodesUsage, 1310)

	t.Logf("Set current node cpu usage to 500")
	n2metrics.Usage[v1.ResourceCPU] = *resource.NewMilliQuantity(900, resource.DecimalSI)
	metricsClientset.Tracker().Update(gvr, n2metrics, "")
	collector.Collect(context.TODO())
	nodesUsage, _ = collector.NodeUsage(n2)
	checkCpuNodeUsage(t, nodesUsage, 1269)
}
