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

package targetloadpacking

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/paypal/load-watcher/pkg/watcher"

	v1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/events"

	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	frameworkfake "sigs.k8s.io/descheduler/pkg/framework/fake"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/trimaran"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
	"sigs.k8s.io/descheduler/pkg/utils"
	"sigs.k8s.io/descheduler/test"
)

var (
	lowPriority  = int32(0)
	highPriority = int32(10000)

	defaultEvictorFilterArgs = &defaultevictor.DefaultEvictorArgs{
		EvictLocalStoragePods:   false,
		EvictSystemCriticalPods: false,
		IgnorePvcPods:           false,
		EvictFailedBarePods:     false,
		NodeFit:                 true,
	}
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
	n1NodeName := "n1"
	n2NodeName := "n2"
	n3NodeName := "n3"

	nodeSelectorKey := "datacenter"
	nodeSelectorValue := "west"
	notMatchingNodeSelectorValue := "east"

	testCases := []struct {
		name                string
		targetUtilization   int64
		nodeUtilization     map[string]float64
		nodes               []*v1.Node
		pods                []*v1.Pod
		expectedPodsEvicted uint
		evictedPods         sets.Set[string]
	}{
		{
			name:              "n1 is overutilized according to metrics, but there is no evictable pod",
			targetUtilization: 50,
			nodeUtilization: map[string]float64{
				n1NodeName: 70,
				n2NodeName: 10,
				n3NodeName: 0,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 0, 10, nil),
				test.BuildTestNode(n2NodeName, 4000, 0, 10, nil),
				test.BuildTestNode(n3NodeName, 4000, 0, 10, test.SetNodeUnschedulable),
			},
			pods: []*v1.Pod{
				// These won't be evicted.
				test.BuildTestPod("p1", 400, 0, n1NodeName, test.SetDSOwnerRef),
				test.BuildTestPod("p2", 400, 0, n1NodeName, test.SetDSOwnerRef),
				test.BuildTestPod("p3", 400, 0, n1NodeName, test.SetDSOwnerRef),
				test.BuildTestPod("p4", 400, 0, n1NodeName, test.SetDSOwnerRef),
				test.BuildTestPod("p5", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A pod with local storage.
					test.SetNormalOwnerRef(pod)
					pod.Spec.Volumes = []v1.Volume{
						{
							Name: "sample",
							VolumeSource: v1.VolumeSource{
								HostPath: &v1.HostPathVolumeSource{Path: "somePath"},
								EmptyDir: &v1.EmptyDirVolumeSource{
									SizeLimit: resource.NewQuantity(int64(10), resource.BinarySI),
								},
							},
						},
					}
					// A Mirror Pod.
					pod.Annotations = test.GetMirrorPodAnnotation()
				}),
				test.BuildTestPod("p6", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A Critical Pod.
					pod.Namespace = "kube-system"
					priority := utils.SystemCriticalPriority
					pod.Spec.Priority = &priority
				}),
				test.BuildTestPod("p7", 400, 0, n2NodeName, test.SetRSOwnerRef),
			},
			expectedPodsEvicted: 0,
		},
		{
			name:              "n1 is overutilized according to metrics, and there are evictable pods",
			targetUtilization: 50,
			nodeUtilization: map[string]float64{
				n1NodeName: 70,
				n2NodeName: 10,
				n3NodeName: 0,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 0, 10, nil),
				test.BuildTestNode(n2NodeName, 4000, 0, 10, nil),
				test.BuildTestNode(n3NodeName, 4000, 0, 10, test.SetNodeUnschedulable),
			},
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p2", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p3", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p4", 400, 0, n1NodeName, test.SetRSOwnerRef),
				// These won't be evicted.
				test.BuildTestPod("p5", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A pod with local storage.
					test.SetNormalOwnerRef(pod)
					pod.Spec.Volumes = []v1.Volume{
						{
							Name: "sample",
							VolumeSource: v1.VolumeSource{
								HostPath: &v1.HostPathVolumeSource{Path: "somePath"},
								EmptyDir: &v1.EmptyDirVolumeSource{
									SizeLimit: resource.NewQuantity(int64(10), resource.BinarySI),
								},
							},
						},
					}
					// A Mirror Pod.
					pod.Annotations = test.GetMirrorPodAnnotation()
				}),
				test.BuildTestPod("p6", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A Critical Pod.
					pod.Namespace = "kube-system"
					priority := utils.SystemCriticalPriority
					pod.Spec.Priority = &priority
				}),
				test.BuildTestPod("p7", 400, 0, n2NodeName, test.SetRSOwnerRef),
			},
			// trimaran.PodAssignEventHandler will have such a small effect on the calculation of the score,
			// so remove 3 pods to meet the goal.
			expectedPodsEvicted: 3,
		},
		{
			name:              "n1 is overutilized according to metrics, even if it's pod requests is below the threshold percentage",
			targetUtilization: 50,
			nodeUtilization: map[string]float64{
				n1NodeName: 100,
				n2NodeName: 10,
				n3NodeName: 0,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 0, 10, nil),
				test.BuildTestNode(n2NodeName, 4000, 0, 10, nil),
				test.BuildTestNode(n3NodeName, 4000, 0, 10, test.SetNodeUnschedulable),
			},
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p2", 400, 0, n2NodeName, test.SetRSOwnerRef),
			},
			expectedPodsEvicted: 1,
		},
		{
			name:              "n1 is underutilized according to metrics, even if it's pod requests is above the threshold percentage",
			targetUtilization: 50,
			nodeUtilization: map[string]float64{
				n1NodeName: 30,
				n2NodeName: 10,
				n3NodeName: 0,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 0, 10, nil),
				test.BuildTestNode(n2NodeName, 4000, 0, 10, nil),
				test.BuildTestNode(n3NodeName, 4000, 0, 10, test.SetNodeUnschedulable),
			},
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p2", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p3", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p4", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p5", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p6", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p7", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p8", 400, 0, n2NodeName, test.SetRSOwnerRef),
			},
			expectedPodsEvicted: 0,
		},
		{
			name:              "n1 is overutilized according to metrics, evicts pods first according to priority (evict p1)",
			targetUtilization: 50,
			nodeUtilization: map[string]float64{
				n1NodeName: 55,
				n2NodeName: 10,
				n3NodeName: 0,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 0, 10, nil),
				test.BuildTestNode(n2NodeName, 4000, 0, 10, nil),
				test.BuildTestNode(n3NodeName, 4000, 0, 10, test.SetNodeUnschedulable),
			},
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.SetPodPriority(pod, lowPriority)
				}),
				test.BuildTestPod("p2", 400, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.SetPodPriority(pod, highPriority)
				}),
				// These won't be evicted.
				test.BuildTestPod("p3", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A Critical Pod.
					pod.Namespace = "kube-system"
					priority := utils.SystemCriticalPriority
					pod.Spec.Priority = &priority
				}),
				test.BuildTestPod("p4", 400, 0, n2NodeName, test.SetRSOwnerRef),
			},
			// trimaran.PodAssignEventHandler will have such a small effect on the calculation of the score,
			// so remove 4 pods to meet the goal.
			expectedPodsEvicted: 1,
			evictedPods:         sets.New("p1"),
		},
		{
			name:              "n1 is overutilized according to metrics, evicts pods first according to priority (evict both p1 and p2)",
			targetUtilization: 50,
			nodeUtilization: map[string]float64{
				n1NodeName: 100,
				n2NodeName: 10,
				n3NodeName: 0,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 0, 10, nil),
				test.BuildTestNode(n2NodeName, 4000, 0, 10, nil),
				test.BuildTestNode(n3NodeName, 4000, 0, 10, test.SetNodeUnschedulable),
			},
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.SetPodPriority(pod, highPriority)
				}),
				test.BuildTestPod("p2", 400, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.SetPodPriority(pod, lowPriority)
				}),
				// These won't be evicted.
				test.BuildTestPod("p3", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A Critical Pod.
					pod.Namespace = "kube-system"
					priority := utils.SystemCriticalPriority
					pod.Spec.Priority = &priority
				}),
				test.BuildTestPod("p4", 400, 0, n2NodeName, test.SetRSOwnerRef),
			},
			// trimaran.PodAssignEventHandler will have such a small effect on the calculation of the score,
			// so remove 4 pods to meet the goal.
			expectedPodsEvicted: 2,
			evictedPods:         sets.New("p1", "p2"),
		},
		{
			name:              "n1 is overutilized according to metrics, evicts pods first according to priority and second according to QoS (evict p1)",
			targetUtilization: 50,
			nodeUtilization: map[string]float64{
				n1NodeName: 55,
				n2NodeName: 10,
				n3NodeName: 0,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 0, 10, nil),
				test.BuildTestNode(n2NodeName, 4000, 0, 10, nil),
				test.BuildTestNode(n3NodeName, 4000, 0, 10, test.SetNodeUnschedulable),
			},
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.SetPodPriority(pod, lowPriority)
				}),
				test.BuildTestPod("p2", 400, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.SetPodPriority(pod, highPriority)
					test.MakeBestEffortPod(pod)
				}),
				test.BuildTestPod("p3", 400, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.SetPodPriority(pod, highPriority)
					test.MakeBurstablePod(pod)
				}),
				test.BuildTestPod("p4", 400, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.SetPodPriority(pod, highPriority)
					test.MakeGuaranteedPod(pod)
				}),
				test.BuildTestPod("p5", 400, 0, n2NodeName, test.SetRSOwnerRef),
			},
			expectedPodsEvicted: 1,
			evictedPods:         sets.New("p1"),
		},
		{
			name:              "n1 is overutilized according to metrics, evicts pods first according to priority and second according to QoS (evict p1, p2 and p3)",
			targetUtilization: 50,
			nodeUtilization: map[string]float64{
				n1NodeName: 65,
				n2NodeName: 10,
				n3NodeName: 0,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 0, 10, nil),
				test.BuildTestNode(n2NodeName, 4000, 0, 10, nil),
				test.BuildTestNode(n3NodeName, 4000, 0, 10, test.SetNodeUnschedulable),
			},
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.SetPodPriority(pod, lowPriority)
				}),
				test.BuildTestPod("p2", 400, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.SetPodPriority(pod, highPriority)
					test.MakeBestEffortPod(pod)
				}),
				test.BuildTestPod("p3", 400, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.SetPodPriority(pod, highPriority)
					test.MakeBurstablePod(pod)
				}),
				test.BuildTestPod("p4", 400, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.SetPodPriority(pod, highPriority)
					test.MakeGuaranteedPod(pod)
				}),
				test.BuildTestPod("p5", 400, 0, n2NodeName, test.SetRSOwnerRef),
			},
			expectedPodsEvicted: 3,
			evictedPods:         sets.New("p1", "p2", "p3"),
		},
		{
			name:              "n1 is overutilized according to metrics, evicts pods first according to priority and second according to QoS (evict p1, p2 p3, and p4)",
			targetUtilization: 50,
			nodeUtilization: map[string]float64{
				n1NodeName: 75,
				n2NodeName: 10,
				n3NodeName: 0,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 0, 10, nil),
				test.BuildTestNode(n2NodeName, 4000, 0, 10, nil),
				test.BuildTestNode(n3NodeName, 4000, 0, 10, test.SetNodeUnschedulable),
			},
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.SetPodPriority(pod, lowPriority)
				}),
				test.BuildTestPod("p2", 400, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.SetPodPriority(pod, highPriority)
					test.MakeBestEffortPod(pod)
				}),
				test.BuildTestPod("p3", 400, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.SetPodPriority(pod, highPriority)
					test.MakeBurstablePod(pod)
				}),
				test.BuildTestPod("p4", 400, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.SetPodPriority(pod, highPriority)
					test.MakeGuaranteedPod(pod)
				}),
				test.BuildTestPod("p5", 400, 0, n2NodeName, test.SetRSOwnerRef),
			},
			expectedPodsEvicted: 4,
			evictedPods:         sets.New("p1", "p2", "p3", "p4"),
		},
		{
			name:              "n1 is overutilized according to metrics, will stop when cpu capacity is depleted",
			targetUtilization: 50,
			nodeUtilization: map[string]float64{
				n1NodeName: 80,
				n2NodeName: 20,
				n3NodeName: 0,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 0, 10, nil),
				test.BuildTestNode(n2NodeName, 4000, 0, 10, nil),
				test.BuildTestNode(n3NodeName, 4000, 0, 10, test.SetNodeUnschedulable),
			},
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p2", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p3", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p4", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p5", 400, 0, n1NodeName, test.SetRSOwnerRef),
				// These won't be evicted.
				test.BuildTestPod("p6", 400, 0, n1NodeName, test.SetDSOwnerRef),
				test.BuildTestPod("p7", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A pod with local storage.
					test.SetNormalOwnerRef(pod)
					pod.Spec.Volumes = []v1.Volume{
						{
							Name: "sample",
							VolumeSource: v1.VolumeSource{
								HostPath: &v1.HostPathVolumeSource{Path: "somePath"},
								EmptyDir: &v1.EmptyDirVolumeSource{
									SizeLimit: resource.NewQuantity(int64(10), resource.BinarySI),
								},
							},
						},
					}
					// A Mirror Pod.
					pod.Annotations = test.GetMirrorPodAnnotation()
				}),
				test.BuildTestPod("p8", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A Critical Pod.
					pod.Namespace = "kube-system"
					priority := utils.SystemCriticalPriority
					pod.Spec.Priority = &priority
				}),
				test.BuildTestPod("p9", 800, 0, n2NodeName, test.SetRSOwnerRef),
			},
			// Only 3 pods can be evicted before cpu is depleted
			expectedPodsEvicted: 3,
		},
		{
			name:              "n1 overutilized according to metrics, but only other node is unschedulable",
			targetUtilization: 50,
			nodeUtilization: map[string]float64{
				n1NodeName: 100,
				n2NodeName: 0,
				n3NodeName: 0,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 0, 10, nil),
				test.BuildTestNode(n2NodeName, 4000, 0, 10, test.SetNodeUnschedulable),
			},
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p2", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p3", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p4", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p5", 400, 0, n1NodeName, test.SetRSOwnerRef),
				// These won't be evicted.
				test.BuildTestPod("p6", 400, 0, n1NodeName, test.SetDSOwnerRef),
				test.BuildTestPod("p7", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A pod with local storage.
					test.SetNormalOwnerRef(pod)
					pod.Spec.Volumes = []v1.Volume{
						{
							Name: "sample",
							VolumeSource: v1.VolumeSource{
								HostPath: &v1.HostPathVolumeSource{Path: "somePath"},
								EmptyDir: &v1.EmptyDirVolumeSource{
									SizeLimit: resource.NewQuantity(int64(10), resource.BinarySI),
								},
							},
						},
					}
					// A Mirror Pod.
					pod.Annotations = test.GetMirrorPodAnnotation()
				}),
				test.BuildTestPod("p8", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A Critical Pod.
					pod.Namespace = "kube-system"
					priority := utils.SystemCriticalPriority
					pod.Spec.Priority = &priority
				}),
			},
			expectedPodsEvicted: 0,
		},
		{
			name:              "n1 overutilized according to metrics, but only other node doesn't match pod node selector for p4 and p5",
			targetUtilization: 50,
			nodeUtilization: map[string]float64{
				n1NodeName: 100,
				n2NodeName: 0,
				n3NodeName: 0,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 0, 10, nil),
				test.BuildTestNode(n2NodeName, 4000, 0, 10, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{nodeSelectorKey: notMatchingNodeSelectorValue}
				}),
			},
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p2", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p3", 400, 0, n1NodeName, test.SetRSOwnerRef),
				// These won't be evicted.
				test.BuildTestPod("p4", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A pod selecting nodes in the "west" datacenter
					test.SetNormalOwnerRef(pod)
					pod.Spec.NodeSelector = map[string]string{nodeSelectorKey: nodeSelectorValue}
				}),
				test.BuildTestPod("p5", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A pod selecting nodes in the "west" datacenter
					test.SetNormalOwnerRef(pod)
					pod.Spec.NodeSelector = map[string]string{nodeSelectorKey: nodeSelectorValue}
				}),
				test.BuildTestPod("p6", 400, 0, n1NodeName, test.SetDSOwnerRef),
				test.BuildTestPod("p7", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A pod with local storage.
					test.SetNormalOwnerRef(pod)
					pod.Spec.Volumes = []v1.Volume{
						{
							Name: "sample",
							VolumeSource: v1.VolumeSource{
								HostPath: &v1.HostPathVolumeSource{Path: "somePath"},
								EmptyDir: &v1.EmptyDirVolumeSource{
									SizeLimit: resource.NewQuantity(int64(10), resource.BinarySI),
								},
							},
						},
					}
					// A Mirror Pod.
					pod.Annotations = test.GetMirrorPodAnnotation()
				}),
				test.BuildTestPod("p8", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A Critical Pod.
					pod.Namespace = "kube-system"
					priority := utils.SystemCriticalPriority
					pod.Spec.Priority = &priority
				}),
			},
			expectedPodsEvicted: 3,
			evictedPods:         sets.New("p1", "p2", "p3"),
		},
		{
			name:              "without priorities, but only other node doesn't match pod node affinity for p4 and p5",
			targetUtilization: 50,
			nodeUtilization: map[string]float64{
				n1NodeName: 100,
				n2NodeName: 0,
				n3NodeName: 0,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 0, 10, nil),
				test.BuildTestNode(n2NodeName, 4000, 0, 10, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{nodeSelectorKey: notMatchingNodeSelectorValue}
				}),
			},
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p2", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p3", 400, 0, n1NodeName, test.SetRSOwnerRef),
				// These won't be evicted.
				test.BuildTestPod("p4", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A pod with affinity to run in the "west" datacenter upon scheduling
					test.SetNormalOwnerRef(pod)
					pod.Spec.Affinity = &v1.Affinity{
						NodeAffinity: &v1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
								NodeSelectorTerms: []v1.NodeSelectorTerm{
									{
										MatchExpressions: []v1.NodeSelectorRequirement{
											{
												Key:      nodeSelectorKey,
												Operator: "In",
												Values:   []string{nodeSelectorValue},
											},
										},
									},
								},
							},
						},
					}
				}),
				test.BuildTestPod("p5", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A pod with affinity to run in the "west" datacenter upon scheduling
					test.SetNormalOwnerRef(pod)
					pod.Spec.Affinity = &v1.Affinity{
						NodeAffinity: &v1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
								NodeSelectorTerms: []v1.NodeSelectorTerm{
									{
										MatchExpressions: []v1.NodeSelectorRequirement{
											{
												Key:      nodeSelectorKey,
												Operator: "In",
												Values:   []string{nodeSelectorValue},
											},
										},
									},
								},
							},
						},
					}
				}),
				test.BuildTestPod("p6", 400, 0, n1NodeName, test.SetDSOwnerRef),
				test.BuildTestPod("p7", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A pod with local storage.
					test.SetNormalOwnerRef(pod)
					pod.Spec.Volumes = []v1.Volume{
						{
							Name: "sample",
							VolumeSource: v1.VolumeSource{
								HostPath: &v1.HostPathVolumeSource{Path: "somePath"},
								EmptyDir: &v1.EmptyDirVolumeSource{
									SizeLimit: resource.NewQuantity(int64(10), resource.BinarySI),
								},
							},
						},
					}
					// A Mirror Pod.
					pod.Annotations = test.GetMirrorPodAnnotation()
				}),
				test.BuildTestPod("p8", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A Critical Pod.
					pod.Namespace = "kube-system"
					priority := utils.SystemCriticalPriority
					pod.Spec.Priority = &priority
				}),
				test.BuildTestPod("p9", 0, 0, n2NodeName, test.SetRSOwnerRef),
			},
			expectedPodsEvicted: 3,
			evictedPods:         sets.New("p1", "p2", "p3"),
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			server := httptest.NewServer(http.HandlerFunc(func(resp http.ResponseWriter, _ *http.Request) {
				bytes, _ := json.Marshal(makeWatcherMetrics(test.nodeUtilization))
				resp.Write(bytes)
			}))
			defer server.Close()

			var objs []runtime.Object
			for _, node := range test.nodes {
				objs = append(objs, node)
			}
			for _, pod := range test.pods {
				objs = append(objs, pod)
			}
			fakeClient := fake.NewSimpleClientset(objs...)

			sharedInformerFactory := informers.NewSharedInformerFactory(fakeClient, 0)

			getPodsAssignedToNode, err := podutil.BuildGetPodsAssignedToNodeFunc(sharedInformerFactory.Core().V1().Pods().Informer())
			if err != nil {
				t.Errorf("Build get pods assigned to node function error: %v", err)
			}

			gotEvictedPods := sets.New[string]()
			if len(test.evictedPods) > 0 {
				fakeClient.Fake.PrependReactor("create", "pods", func(action core.Action) (bool, runtime.Object, error) {
					getAction := action.(core.CreateAction)
					obj := getAction.GetObject()
					if eviction, ok := obj.(*policy.Eviction); ok {
						gotEvictedPods.Insert(eviction.Name)
					}
					return true, obj, nil
				})
			}

			sharedInformerFactory.Start(ctx.Done())
			sharedInformerFactory.WaitForCacheSync(ctx.Done())

			podEvictor := evictions.NewPodEvictor(
				fakeClient,
				policy.SchemeGroupVersion.String(),
				false,
				nil,
				nil,
				test.nodes,
				false,
				&events.FakeRecorder{},
			)

			evictorFilter, err := defaultevictor.New(
				defaultEvictorFilterArgs,
				&frameworkfake.HandleImpl{
					ClientsetImpl:                 fakeClient,
					GetPodsAssignedToNodeFuncImpl: getPodsAssignedToNode,
					SharedInformerFactoryImpl:     sharedInformerFactory,
				},
			)
			if err != nil {
				t.Fatalf("Unable to initialize the plugin: %v", err)
			}

			handle := &frameworkfake.HandleImpl{
				ClientsetImpl:                 fakeClient,
				GetPodsAssignedToNodeFuncImpl: getPodsAssignedToNode,
				PodEvictorImpl:                podEvictor,
				EvictorFilterImpl:             evictorFilter.(frameworktypes.EvictorPlugin),
				SharedInformerFactoryImpl:     sharedInformerFactory,
			}

			plugin, err := New(
				&trimaran.TargetLoadPackingArgs{
					TrimaranSpec:              trimaran.TrimaranSpec{WatcherAddress: &server.URL},
					TargetUtilization:         &test.targetUtilization,
					DefaultRequestsMultiplier: &trimaran.DefaultRequestsMultiplier,
				},
				handle)
			if err != nil {
				t.Fatalf("Unable to initialize the plugin: %v", err)
			}
			plugin.(frameworktypes.BalancePlugin).Balance(ctx, test.nodes)

			podsEvicted := podEvictor.TotalEvicted()
			if test.expectedPodsEvicted != podsEvicted {
				t.Errorf("Expected %v pods to be evicted but %v got evicted", test.expectedPodsEvicted, podsEvicted)
			}
			if test.evictedPods != nil && !gotEvictedPods.Equal(test.evictedPods) {
				t.Errorf("Pod evictions failed unexpectedly, expected %v but got %v", test.evictedPods, gotEvictedPods)
			}

			t.Logf("\n\n\n")
		})
	}
}

func TestTargetLoadPackingWithTaints(t *testing.T) {
	n1 := test.BuildTestNode("n1", 2000, 0, 10, nil)
	n2 := test.BuildTestNode("n2", 1000, 0, 10, nil)
	n3 := test.BuildTestNode("n3", 1000, 0, 10, nil)
	n3withTaints := n3.DeepCopy()
	n3withTaints.Spec.Taints = []v1.Taint{
		{
			Key:    "key",
			Value:  "value",
			Effect: v1.TaintEffectNoSchedule,
		},
	}

	podThatToleratesTaint := test.BuildTestPod("tolerate_pod", 200, 0, n1.Name, test.SetRSOwnerRef)
	podThatToleratesTaint.Spec.Tolerations = []v1.Toleration{
		{
			Key:   "key",
			Value: "value",
		},
	}

	tests := []struct {
		name                string
		targetUtilization   int64
		nodeUtilization     map[string]float64
		nodes               []*v1.Node
		pods                []*v1.Pod
		expectedPodsEvicted uint
		evictedPods         sets.Set[string]
	}{
		{
			name:              "No taints",
			targetUtilization: 50,
			nodeUtilization: map[string]float64{
				n1.Name: 55,
				n2.Name: 10,
				n3.Name: 0,
			},
			nodes: []*v1.Node{n1, n2, n3},
			pods: []*v1.Pod{
				// Node 1 pods
				test.BuildTestPod(fmt.Sprintf("pod_1_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_2_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_3_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_4_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_5_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_6_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				// Node 2 pods
				test.BuildTestPod(fmt.Sprintf("pod_7_%s", n2.Name), 200, 0, n2.Name, test.SetRSOwnerRef),
			},
			expectedPodsEvicted: 1,
		},
		{
			name:              "No pod tolerates node taint",
			targetUtilization: 50,
			nodeUtilization: map[string]float64{
				n1.Name: 55,
				n2.Name: 0,
				n3.Name: 10,
			},
			nodes: []*v1.Node{n1, n3withTaints},
			pods: []*v1.Pod{
				// Node 1 pods
				test.BuildTestPod(fmt.Sprintf("pod_1_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_2_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_3_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_4_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_5_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_6_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				// Node 3 pods
				test.BuildTestPod(fmt.Sprintf("pod_7_%s", n3withTaints.Name), 200, 0, n3withTaints.Name, test.SetRSOwnerRef),
			},
			expectedPodsEvicted: 0,
		},
		{
			name:              "Pod which tolerates node taint",
			targetUtilization: 50,
			nodeUtilization: map[string]float64{
				n1.Name: 55,
				n2.Name: 0,
				n3.Name: 10,
			},
			nodes: []*v1.Node{n1, n3withTaints},
			pods: []*v1.Pod{
				// Node 1 pods
				test.BuildTestPod(fmt.Sprintf("pod_1_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_2_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_3_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_4_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_5_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				podThatToleratesTaint,
				// Node 3 pods
				test.BuildTestPod(fmt.Sprintf("pod_7_%s", n3withTaints.Name), 200, 0, n3withTaints.Name, test.SetRSOwnerRef),
			},
			expectedPodsEvicted: 1,
			evictedPods:         sets.New("tolerate_pod"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			server := httptest.NewServer(http.HandlerFunc(func(resp http.ResponseWriter, _ *http.Request) {
				bytes, _ := json.Marshal(makeWatcherMetrics(test.nodeUtilization))
				resp.Write(bytes)
			}))
			defer server.Close()

			var objs []runtime.Object
			for _, node := range test.nodes {
				objs = append(objs, node)
			}
			for _, pod := range test.pods {
				objs = append(objs, pod)
			}
			fakeClient := fake.NewSimpleClientset(objs...)

			sharedInformerFactory := informers.NewSharedInformerFactory(fakeClient, 0)

			getPodsAssignedToNode, err := podutil.BuildGetPodsAssignedToNodeFunc(sharedInformerFactory.Core().V1().Pods().Informer())
			if err != nil {
				t.Errorf("Build get pods assigned to node function error: %v", err)
			}

			gotEvictedPods := sets.New[string]()
			if len(test.evictedPods) > 0 {
				fakeClient.Fake.PrependReactor("create", "pods", func(action core.Action) (bool, runtime.Object, error) {
					getAction := action.(core.CreateAction)
					obj := getAction.GetObject()
					if eviction, ok := obj.(*policy.Eviction); ok {
						gotEvictedPods.Insert(eviction.Name)
					}
					return true, obj, nil
				})
			}

			sharedInformerFactory.Start(ctx.Done())
			sharedInformerFactory.WaitForCacheSync(ctx.Done())

			podEvictor := evictions.NewPodEvictor(
				fakeClient,
				policy.SchemeGroupVersion.String(),
				false,
				nil,
				nil,
				test.nodes,
				false,
				&events.FakeRecorder{},
			)

			evictorFilter, err := defaultevictor.New(
				defaultEvictorFilterArgs,
				&frameworkfake.HandleImpl{
					ClientsetImpl:                 fakeClient,
					GetPodsAssignedToNodeFuncImpl: getPodsAssignedToNode,
					SharedInformerFactoryImpl:     sharedInformerFactory,
				},
			)
			if err != nil {
				t.Fatalf("Unable to initialize the plugin: %v", err)
			}

			handle := &frameworkfake.HandleImpl{
				ClientsetImpl:                 fakeClient,
				GetPodsAssignedToNodeFuncImpl: getPodsAssignedToNode,
				PodEvictorImpl:                podEvictor,
				EvictorFilterImpl:             evictorFilter.(frameworktypes.EvictorPlugin),
				SharedInformerFactoryImpl:     sharedInformerFactory,
			}

			plugin, err := New(
				&trimaran.TargetLoadPackingArgs{
					TrimaranSpec:              trimaran.TrimaranSpec{WatcherAddress: &server.URL},
					TargetUtilization:         &test.targetUtilization,
					DefaultRequestsMultiplier: &trimaran.DefaultRequestsMultiplier,
				},
				handle)
			if err != nil {
				t.Fatalf("Unable to initialize the plugin: %v", err)
			}
			plugin.(frameworktypes.BalancePlugin).Balance(ctx, test.nodes)

			podsEvicted := podEvictor.TotalEvicted()
			if test.expectedPodsEvicted != podsEvicted {
				t.Errorf("Expected %v pods to be evicted but %v got evicted", test.expectedPodsEvicted, podsEvicted)
			}
			if test.evictedPods != nil && !gotEvictedPods.Equal(test.evictedPods) {
				t.Errorf("Pod evictions failed unexpectedly, expected %v but got %v", test.evictedPods, gotEvictedPods)
			}
		})
	}
}
