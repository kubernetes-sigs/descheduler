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
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/prometheus/common/model"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/informers"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	"k8s.io/metrics/pkg/apis/metrics/v1beta1"
	fakemetricsclient "k8s.io/metrics/pkg/client/clientset/versioned/fake"

	"sigs.k8s.io/descheduler/pkg/descheduler/metricscollector"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/test"
)

var (
	nodesgvr = schema.GroupVersionResource{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "nodes"}
	podsgvr  = schema.GroupVersionResource{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "pods"}
)

func updateMetricsAndCheckNodeUtilization(
	t *testing.T,
	ctx context.Context,
	newValue, expectedValue int64,
	metricsClientset *fakemetricsclient.Clientset,
	collector *metricscollector.MetricsCollector,
	usageClient usageClient,
	nodes []*v1.Node,
	nodeName string,
	nodemetrics *v1beta1.NodeMetrics,
) {
	t.Logf("Set current node cpu usage to %v", newValue)
	nodemetrics.Usage[v1.ResourceCPU] = *resource.NewMilliQuantity(newValue, resource.DecimalSI)
	metricsClientset.Tracker().Update(nodesgvr, nodemetrics, "")
	err := collector.Collect(ctx)
	if err != nil {
		t.Fatalf("failed to capture metrics: %v", err)
	}
	err = usageClient.sync(nodes)
	if err != nil {
		t.Fatalf("failed to sync a snapshot: %v", err)
	}
	nodeUtilization := usageClient.nodeUtilization(nodeName)
	t.Logf("current node cpu usage: %v\n", nodeUtilization[v1.ResourceCPU].MilliValue())
	if nodeUtilization[v1.ResourceCPU].MilliValue() != expectedValue {
		t.Fatalf("cpu node usage expected to be %v, got %v instead", expectedValue, nodeUtilization[v1.ResourceCPU].MilliValue())
	}
	pods := usageClient.pods(nodeName)
	fmt.Printf("pods: %#v\n", pods)
	if len(pods) != 2 {
		t.Fatalf("expected 2 pods for node %v, got %v instead", nodeName, len(pods))
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
	metricsClientset := fakemetricsclient.NewSimpleClientset()
	metricsClientset.Tracker().Create(nodesgvr, n1metrics, "")
	metricsClientset.Tracker().Create(nodesgvr, n2metrics, "")
	metricsClientset.Tracker().Create(nodesgvr, n3metrics, "")

	ctx := context.TODO()

	resourceNames := []v1.ResourceName{
		v1.ResourceCPU,
		v1.ResourceMemory,
	}

	sharedInformerFactory := informers.NewSharedInformerFactory(clientset, 0)
	podInformer := sharedInformerFactory.Core().V1().Pods().Informer()
	nodeLister := sharedInformerFactory.Core().V1().Nodes().Lister()
	podsAssignedToNode, err := podutil.BuildGetPodsAssignedToNodeFunc(podInformer)
	if err != nil {
		t.Fatalf("Build get pods assigned to node function error: %v", err)
	}

	sharedInformerFactory.Start(ctx.Done())
	sharedInformerFactory.WaitForCacheSync(ctx.Done())

	collector := metricscollector.NewMetricsCollector(nodeLister, metricsClientset, labels.Everything())

	usageClient := newActualUsageClient(
		resourceNames,
		podsAssignedToNode,
		collector,
	)

	updateMetricsAndCheckNodeUtilization(t, ctx,
		1400, 1400,
		metricsClientset, collector, usageClient, nodes, n2.Name, n2metrics,
	)

	updateMetricsAndCheckNodeUtilization(t, ctx,
		500, 1310,
		metricsClientset, collector, usageClient, nodes, n2.Name, n2metrics,
	)

	updateMetricsAndCheckNodeUtilization(t, ctx,
		900, 1269,
		metricsClientset, collector, usageClient, nodes, n2.Name, n2metrics,
	)
}

type fakePromClient struct {
	result   interface{}
	dataType model.ValueType
}

type fakePayload struct {
	Status string      `json:"status"`
	Data   queryResult `json:"data"`
}

type queryResult struct {
	Type   model.ValueType `json:"resultType"`
	Result interface{}     `json:"result"`
}

func (client *fakePromClient) URL(ep string, args map[string]string) *url.URL {
	return &url.URL{}
}

func (client *fakePromClient) Do(ctx context.Context, request *http.Request) (*http.Response, []byte, error) {
	jsonData, err := json.Marshal(fakePayload{
		Status: "success",
		Data: queryResult{
			Type:   client.dataType,
			Result: client.result,
		},
	})

	return &http.Response{StatusCode: 200}, jsonData, err
}

func sample(metricName, nodeName string, value float64) *model.Sample {
	return &model.Sample{
		Metric: model.Metric{
			"__name__": model.LabelValue(metricName),
			"instance": model.LabelValue(nodeName),
		},
		Value:     model.SampleValue(value),
		Timestamp: 1728991761711,
	}
}

func TestPrometheusUsageClient(t *testing.T) {
	n1 := test.BuildTestNode("ip-10-0-17-165.ec2.internal", 2000, 3000, 10, nil)
	n2 := test.BuildTestNode("ip-10-0-51-101.ec2.internal", 2000, 3000, 10, nil)
	n3 := test.BuildTestNode("ip-10-0-94-25.ec2.internal", 2000, 3000, 10, nil)

	nodes := []*v1.Node{n1, n2, n3}

	p1 := test.BuildTestPod("p1", 400, 0, n1.Name, nil)
	p21 := test.BuildTestPod("p21", 400, 0, n2.Name, nil)
	p22 := test.BuildTestPod("p22", 400, 0, n2.Name, nil)
	p3 := test.BuildTestPod("p3", 400, 0, n3.Name, nil)

	tests := []struct {
		name      string
		result    interface{}
		dataType  model.ValueType
		nodeUsage map[string]int64
		err       error
	}{
		{
			name:     "valid data",
			dataType: model.ValVector,
			result: model.Vector{
				sample("instance:node_cpu:rate:sum", "ip-10-0-51-101.ec2.internal", 0.20381818181818104),
				sample("instance:node_cpu:rate:sum", "ip-10-0-17-165.ec2.internal", 0.4245454545454522),
				sample("instance:node_cpu:rate:sum", "ip-10-0-94-25.ec2.internal", 0.5695757575757561),
			},
			nodeUsage: map[string]int64{
				"ip-10-0-51-101.ec2.internal": 20,
				"ip-10-0-17-165.ec2.internal": 42,
				"ip-10-0-94-25.ec2.internal":  56,
			},
		},
		{
			name:     "invalid data missing instance label",
			dataType: model.ValVector,
			result: model.Vector{
				&model.Sample{
					Metric: model.Metric{
						"__name__": model.LabelValue("instance:node_cpu:rate:sum"),
					},
					Value:     model.SampleValue(0.20381818181818104),
					Timestamp: 1728991761711,
				},
			},
			err: fmt.Errorf("The collected metrics sample is missing 'instance' key"),
		},
		{
			name:     "invalid data value out of range",
			dataType: model.ValVector,
			result: model.Vector{
				sample("instance:node_cpu:rate:sum", "ip-10-0-51-101.ec2.internal", 1.20381818181818104),
			},
			err: fmt.Errorf("The collected metrics sample for \"ip-10-0-51-101.ec2.internal\" has value 1.203818181818181 outside of <0; 1> interval"),
		},
		{
			name:     "invalid data not a vector",
			dataType: model.ValScalar,
			result: model.Scalar{
				Value:     model.SampleValue(0.20381818181818104),
				Timestamp: 1728991761711,
			},
			err: fmt.Errorf("expected query results to be of type \"vector\", got \"scalar\" instead"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pClient := &fakePromClient{
				result:   tc.result,
				dataType: tc.dataType,
			}

			clientset := fakeclientset.NewSimpleClientset(n1, n2, n3, p1, p21, p22, p3)

			ctx := context.TODO()
			sharedInformerFactory := informers.NewSharedInformerFactory(clientset, 0)
			podInformer := sharedInformerFactory.Core().V1().Pods().Informer()
			podsAssignedToNode, err := podutil.BuildGetPodsAssignedToNodeFunc(podInformer)
			if err != nil {
				t.Fatalf("Build get pods assigned to node function error: %v", err)
			}

			sharedInformerFactory.Start(ctx.Done())
			sharedInformerFactory.WaitForCacheSync(ctx.Done())

			prometheusUsageClient := newPrometheusUsageClient(podsAssignedToNode, pClient, "instance:node_cpu:rate:sum")
			err = prometheusUsageClient.sync(nodes)
			if tc.err == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Fatalf("unexpected %q error, got nil instead", tc.err)
				} else if err.Error() != tc.err.Error() {
					t.Fatalf("expected %q error, got %q instead", tc.err, err)
				}
				return
			}

			for _, node := range nodes {
				nodeUtil := prometheusUsageClient.nodeUtilization(node.Name)
				if nodeUtil[ResourceMetrics].Value() != tc.nodeUsage[node.Name] {
					t.Fatalf("expected %q node utilization to be %v, got %v instead", node.Name, tc.nodeUsage[node.Name], nodeUtil[ResourceMetrics])
				} else {
					t.Logf("%v node utilization: %v", node.Name, nodeUtil[ResourceMetrics])
				}
			}
		})
	}
}
