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

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/paypal/load-watcher/pkg/watcher"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilpointer "k8s.io/utils/pointer"

	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	frameworkfake "sigs.k8s.io/descheduler/pkg/framework/fake"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/trimaran"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/trimaran/targetloadpacking"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
)

func makeWatcherMetrics(nodeUtilization map[string]float64) watcher.WatcherMetrics {
	nodeMetricsMap := map[string]watcher.NodeMetrics{}
	for node, value := range nodeUtilization {
		nodeMetricsMap[node] = watcher.NodeMetrics{
			Metrics: []watcher.Metric{
				{
					Type:     watcher.CPU,
					Value:    value,
					Operator: watcher.Latest,
				},
			},
		}
	}
	return watcher.WatcherMetrics{
		Window: watcher.Window{},
		Data:   watcher.Data{NodeMetricsMap: nodeMetricsMap},
	}
}

func TestTargetLoadPacking(t *testing.T) {
	// 1. Prepare nodes and pods.
	ctx := context.Background()
	clientSet, sharedInformerFactory, _, getPodsAssignedToNode, stopCh := initializeClient(t)
	defer close(stopCh)
	nodeList, err := clientSet.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Errorf("Error listing node with %v", err)
	}
	nodes, workerNodes := splitNodesAndWorkerNodes(nodeList.Items)

	t.Log("Creating testing namespace")
	testNamespace := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "e2e-" + strings.ToLower(t.Name())}}
	if _, err := clientSet.CoreV1().Namespaces().Create(ctx, testNamespace, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Unable to create ns %v", testNamespace.Name)
	}
	defer clientSet.CoreV1().Namespaces().Delete(ctx, testNamespace.Name, metav1.DeleteOptions{})

	t.Log("Creating pods each consuming 10% of node's allocatable")
	nodeCpu := workerNodes[0].Status.Allocatable[v1.ResourceCPU]
	tenthOfCpu := int64(float64((&nodeCpu).MilliValue()) * 0.1)

	t.Log("Creating pods all bound to a single node")
	for i := 0; i < 4; i++ {
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("tlp-pod-%v", i),
				Namespace: testNamespace.Name,
				Labels:    map[string]string{"test": "node-targetloadpacking", "name": "test-rc-node-targetloadpacking"},
			},
			Spec: v1.PodSpec{
				SecurityContext: &v1.PodSecurityContext{
					RunAsNonRoot:   utilpointer.Bool(true),
					RunAsUser:      utilpointer.Int64(1000),
					RunAsGroup:     utilpointer.Int64(1000),
					SeccompProfile: &v1.SeccompProfile{Type: v1.SeccompProfileTypeRuntimeDefault},
				},
				Containers: []v1.Container{{
					Name:            "pause",
					ImagePullPolicy: "Never",
					Image:           "kubernetes/pause",
					Ports:           []v1.ContainerPort{{ContainerPort: 80}},
					Resources: v1.ResourceRequirements{
						Limits:   v1.ResourceList{v1.ResourceCPU: *resource.NewMilliQuantity(tenthOfCpu, resource.DecimalSI)},
						Requests: v1.ResourceList{v1.ResourceCPU: *resource.NewMilliQuantity(tenthOfCpu, resource.DecimalSI)},
					},
				}},
				Affinity: &v1.Affinity{
					NodeAffinity: &v1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
							NodeSelectorTerms: []v1.NodeSelectorTerm{
								{
									MatchFields: []v1.NodeSelectorRequirement{
										{Key: "metadata.name", Operator: v1.NodeSelectorOpIn, Values: []string{workerNodes[0].Name}},
									},
								},
							},
						},
					},
				},
			},
		}

		t.Logf("Creating pod %v in %v namespace for node %v", pod.Name, pod.Namespace, workerNodes[0].Name)
		_, err := clientSet.CoreV1().Pods(pod.Namespace).Create(ctx, pod, metav1.CreateOptions{})
		if err != nil {
			t.Logf("Error creating TLP pods: %v", err)
			if err = clientSet.CoreV1().Pods(pod.Namespace).DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{
				LabelSelector: labels.SelectorFromSet(labels.Set(map[string]string{"test": "node-targetloadpacking", "name": "test-rc-node-targetloadpacking"})).String(),
			}); err != nil {
				t.Fatalf("Unable to delete TLP pods: %v", err)
			}
			return
		}
	}

	t.Log("Creating RC with 4 replicas owning the created pods")
	rc := RcByNameContainer("test-rc-node-targetloadpacking", testNamespace.Name, int32(4), map[string]string{"test": "node-targetloadpacking"}, nil, "")
	if _, err := clientSet.CoreV1().ReplicationControllers(rc.Namespace).Create(ctx, rc, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Error creating RC %v", err)
	}
	defer deleteRC(ctx, t, clientSet, rc)
	waitForRCPodsRunning(ctx, t, clientSet, rc)

	// 2. Run TargetLoadPacking plugin.
	evictorFilter, _ := defaultevictor.New(
		&defaultevictor.DefaultEvictorArgs{
			EvictLocalStoragePods:   true,
			EvictSystemCriticalPods: false,
			IgnorePvcPods:           false,
			EvictFailedBarePods:     false,
		},
		&frameworkfake.HandleImpl{
			ClientsetImpl:                 clientSet,
			GetPodsAssignedToNodeFuncImpl: getPodsAssignedToNode,
		},
	)

	getPodsOnNodes := func() []*v1.Pod {
		podFilter, err := podutil.NewOptions().WithFilter(evictorFilter.(frameworktypes.EvictorPlugin).Filter).BuildFilterFunc()
		if err != nil {
			t.Errorf("Error initializing pod filter function, %v", err)
		}
		podsOnNodes, err := podutil.ListPodsOnANode(workerNodes[0].Name, getPodsAssignedToNode, podFilter)
		if err != nil {
			t.Errorf("Error listing pods on a node %v", err)
		}
		return podsOnNodes
	}

	podsBefore := len(getPodsOnNodes())

	server := httptest.NewServer(http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		nodeUtilization := map[string]float64{}
		for i := 0; i < len(workerNodes); i++ {
			utilization := float64(0)
			if i == 0 {
				utilization = 100
			}
			nodeUtilization[workerNodes[i].Name] = utilization
		}
		bytes, err := json.Marshal(makeWatcherMetrics(nodeUtilization))
		if err != nil {
			t.Errorf("Error json marshal: %v", err)
		}
		resp.Write(bytes)
	}))
	defer server.Close()
	targetUtilization := int64(50)

	t.Log("Running TargetLoadPacking plugin")
	plugin, err := targetloadpacking.New(
		&trimaran.TargetLoadPackingArgs{
			TrimaranSpec:              trimaran.TrimaranSpec{WatcherAddress: &server.URL},
			TargetUtilization:         &targetUtilization,
			DefaultRequestsMultiplier: &trimaran.DefaultRequestsMultiplier,
		},
		&frameworkfake.HandleImpl{
			ClientsetImpl:                 clientSet,
			GetPodsAssignedToNodeFuncImpl: getPodsAssignedToNode,
			PodEvictorImpl:                initPodEvictorOrFail(t, clientSet, getPodsAssignedToNode, nodes),
			EvictorFilterImpl:             evictorFilter.(frameworktypes.EvictorPlugin),
			SharedInformerFactoryImpl:     sharedInformerFactory,
		},
	)
	if err != nil {
		t.Fatalf("Unable to initialize the plugin: %v", err)
	}
	t.Logf("Pods on the node %v should be evicted because it is over-utilized according to the metrics", workerNodes[0].Name)
	plugin.(frameworktypes.BalancePlugin).Balance(ctx, workerNodes)

	waitForTerminatingPodsToDisappear(ctx, t, clientSet, rc.Namespace)

	podsAfter := len(getPodsOnNodes())

	if podsAfter >= podsBefore {
		t.Fatalf("No pod has been evicted from %v node", workerNodes[0].Name)
	}
	t.Logf("Number of pods on node %v changed from %v to %v", workerNodes[0].Name, podsBefore, podsAfter)
}
