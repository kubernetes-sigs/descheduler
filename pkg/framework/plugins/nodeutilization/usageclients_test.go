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

package nodeutilization

import (
	"context"
	"fmt"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"k8s.io/client-go/informers"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	"k8s.io/metrics/pkg/apis/metrics/v1beta1"
	fakemetricsclient "k8s.io/metrics/pkg/client/clientset/versioned/fake"
	"sigs.k8s.io/descheduler/pkg/descheduler/metricscollector"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/test"
)

var gvr = schema.GroupVersionResource{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "nodemetricses"}

func updateMetricsAndCheckNodeUtilization(
	t *testing.T,
	ctx context.Context,
	newValue, expectedValue int64,
	metricsClientset *fakemetricsclient.Clientset,
	collector *metricscollector.MetricsCollector,
	usageSnapshot usageClient,
	nodes []*v1.Node,
	nodeName string,
	nodemetrics *v1beta1.NodeMetrics,
) {
	t.Logf("Set current node cpu usage to %v", newValue)
	nodemetrics.Usage[v1.ResourceCPU] = *resource.NewMilliQuantity(newValue, resource.DecimalSI)
	metricsClientset.Tracker().Update(gvr, nodemetrics, "")
	err := collector.Collect(ctx)
	if err != nil {
		t.Fatalf("failed to capture metrics: %v", err)
	}
	err = usageSnapshot.capture(nodes)
	if err != nil {
		t.Fatalf("failed to capture a snapshot: %v", err)
	}
	nodeUtilization := usageSnapshot.nodeUtilization(nodeName)
	t.Logf("current node cpu usage: %v\n", nodeUtilization[v1.ResourceCPU].MilliValue())
	if nodeUtilization[v1.ResourceCPU].MilliValue() != expectedValue {
		t.Fatalf("cpu node usage expected to be %v, got %v instead", expectedValue, nodeUtilization[v1.ResourceCPU].MilliValue())
	}
	pods := usageSnapshot.pods(nodeName)
	fmt.Printf("pods: %#v\n", pods)
	if len(pods) != 2 {
		t.Fatalf("expected 2 pods for node %v, got %v instead", nodeName, len(pods))
	}
	capturedNodes := usageSnapshot.nodes()
	if len(capturedNodes) != 3 {
		t.Fatalf("expected 3 captured node, got %v instead", len(capturedNodes))
	}
}

func TestActualUsageClient(t *testing.T) {
	n1 := test.BuildTestNode("n1", 2000, 3000, 10, nil)
	n2 := test.BuildTestNode("n2", 2000, 3000, 10, nil)
	n3 := test.BuildTestNode("n3", 2000, 3000, 10, nil)

	p1 := test.BuildTestPod("p1", 400, 0, n1.Name, nil)
	p21 := test.BuildTestPod("p21", 400, 0, n2.Name, nil)
	p22 := test.BuildTestPod("p22", 400, 0, n2.Name, nil)
	p3 := test.BuildTestPod("p3", 400, 0, n3.Name, nil)

	nodes := []*v1.Node{n1, n2, n3}

	n1metrics := test.BuildNodeMetrics("n1", 400, 1714978816)
	n2metrics := test.BuildNodeMetrics("n2", 1400, 1714978816)
	n3metrics := test.BuildNodeMetrics("n3", 300, 1714978816)

	clientset := fakeclientset.NewSimpleClientset(n1, n2, n3, p1, p21, p22, p3)
	metricsClientset := fakemetricsclient.NewSimpleClientset(n1metrics, n2metrics, n3metrics)

	ctx := context.TODO()

	resourceNames := []v1.ResourceName{
		v1.ResourceCPU,
		v1.ResourceMemory,
	}

	sharedInformerFactory := informers.NewSharedInformerFactory(clientset, 0)
	podInformer := sharedInformerFactory.Core().V1().Pods().Informer()
	podsAssignedToNode, err := podutil.BuildGetPodsAssignedToNodeFunc(podInformer)
	if err != nil {
		t.Fatalf("Build get pods assigned to node function error: %v", err)
	}

	sharedInformerFactory.Start(ctx.Done())
	sharedInformerFactory.WaitForCacheSync(ctx.Done())

	collector := metricscollector.NewMetricsCollector(clientset, metricsClientset)

	usageSnapshot := newActualUsageSnapshot(
		resourceNames,
		podsAssignedToNode,
		collector,
	)

	updateMetricsAndCheckNodeUtilization(t, ctx,
		1400, 1400,
		metricsClientset, collector, usageSnapshot, nodes, n2.Name, n2metrics,
	)

	updateMetricsAndCheckNodeUtilization(t, ctx,
		500, 1310,
		metricsClientset, collector, usageSnapshot, nodes, n2.Name, n2metrics,
	)

	updateMetricsAndCheckNodeUtilization(t, ctx,
		900, 1269,
		metricsClientset, collector, usageSnapshot, nodes, n2.Name, n2metrics,
	)
}
