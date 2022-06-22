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

package evictions

import (
	"context"
	"fmt"
	"github.com/prometheus/client_golang/prometheus/testutil"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"sigs.k8s.io/descheduler/metrics"
	"sigs.k8s.io/descheduler/pkg/api"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/utils"
	"sigs.k8s.io/descheduler/test"
	"sort"
	"strings"
	"testing"
)

func TestEvictPod(t *testing.T) {
	ctx := context.Background()
	node1 := test.BuildTestNode("node1", 1000, 2000, 9, nil)
	pod1 := test.BuildTestPod("p1", 400, 0, "node1", nil)
	tests := []struct {
		description string
		node        *v1.Node
		pod         *v1.Pod
		pods        []v1.Pod
		want        error
	}{
		{
			description: "test pod eviction - pod present",
			node:        node1,
			pod:         pod1,
			pods:        []v1.Pod{*pod1},
			want:        nil,
		},
		{
			description: "test pod eviction - pod absent",
			node:        node1,
			pod:         pod1,
			pods:        []v1.Pod{*test.BuildTestPod("p2", 400, 0, "node1", nil), *test.BuildTestPod("p3", 450, 0, "node1", nil)},
			want:        nil,
		},
	}

	for _, test := range tests {
		fakeClient := &fake.Clientset{}
		fakeClient.Fake.AddReactor("list", "pods", func(action core.Action) (bool, runtime.Object, error) {
			return true, &v1.PodList{Items: test.pods}, nil
		})
		got := evictPod(ctx, fakeClient, test.pod, "v1")
		if got != test.want {
			t.Errorf("Test error for Desc: %s. Expected %v pod eviction to be %v, got %v", test.description, test.pod.Name, test.want, got)
		}
	}
}

func TestIsEvictable(t *testing.T) {
	n1 := test.BuildTestNode("node1", 1000, 2000, 13, nil)
	lowPriority := int32(800)
	highPriority := int32(900)

	nodeTaintKey := "hardware"
	nodeTaintValue := "gpu"

	nodeLabelKey := "datacenter"
	nodeLabelValue := "east"
	type testCase struct {
		description             string
		pods                    []*v1.Pod
		nodes                   []*v1.Node
		evictFailedBarePods     bool
		evictLocalStoragePods   bool
		evictSystemCriticalPods bool
		priorityThreshold       *int32
		nodeFit                 bool
		result                  bool
	}

	testCases := []testCase{
		{
			description: "Failed pod eviction with no ownerRefs",
			pods: []*v1.Pod{
				test.BuildTestPod("bare_pod_failed", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.Status.Phase = v1.PodFailed
				}),
			},
			evictFailedBarePods: false,
			result:              false,
		}, {
			description:         "Normal pod eviction with no ownerRefs and evictFailedBarePods enabled",
			pods:                []*v1.Pod{test.BuildTestPod("bare_pod", 400, 0, n1.Name, nil)},
			evictFailedBarePods: true,
			result:              false,
		}, {
			description: "Failed pod eviction with no ownerRefs",
			pods: []*v1.Pod{
				test.BuildTestPod("bare_pod_failed_but_can_be_evicted", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.Status.Phase = v1.PodFailed
				}),
			},
			evictFailedBarePods: true,
			result:              true,
		}, {
			description: "Normal pod eviction with normal ownerRefs",
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  true,
		}, {
			description: "Normal pod eviction with normal ownerRefs and descheduler.alpha.kubernetes.io/evict annotation",
			pods: []*v1.Pod{
				test.BuildTestPod("p2", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.Annotations = map[string]string{"descheduler.alpha.kubernetes.io/evict": "true"}
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  true,
		}, {
			description: "Normal pod eviction with replicaSet ownerRefs",
			pods: []*v1.Pod{
				test.BuildTestPod("p3", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  true,
		}, {
			description: "Normal pod eviction with replicaSet ownerRefs and descheduler.alpha.kubernetes.io/evict annotation",
			pods: []*v1.Pod{
				test.BuildTestPod("p4", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.Annotations = map[string]string{"descheduler.alpha.kubernetes.io/evict": "true"}
					pod.ObjectMeta.OwnerReferences = test.GetReplicaSetOwnerRefList()
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  true,
		}, {
			description: "Normal pod eviction with statefulSet ownerRefs",
			pods: []*v1.Pod{
				test.BuildTestPod("p18", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  true,
		}, {
			description: "Normal pod eviction with statefulSet ownerRefs and descheduler.alpha.kubernetes.io/evict annotation",
			pods: []*v1.Pod{
				test.BuildTestPod("p19", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.Annotations = map[string]string{"descheduler.alpha.kubernetes.io/evict": "true"}
					pod.ObjectMeta.OwnerReferences = test.GetStatefulSetOwnerRefList()
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  true,
		}, {
			description: "Pod not evicted because it is bound to a PV and evictLocalStoragePods = false",
			pods: []*v1.Pod{
				test.BuildTestPod("p5", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.Spec.Volumes = []v1.Volume{
						{
							Name: "sample",
							VolumeSource: v1.VolumeSource{
								HostPath: &v1.HostPathVolumeSource{Path: "somePath"},
								EmptyDir: &v1.EmptyDirVolumeSource{
									SizeLimit: resource.NewQuantity(int64(10), resource.BinarySI)},
							},
						},
					}
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  false,
		}, {
			description: "Pod is evicted because it is bound to a PV and evictLocalStoragePods = true",
			pods: []*v1.Pod{
				test.BuildTestPod("p6", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.Spec.Volumes = []v1.Volume{
						{
							Name: "sample",
							VolumeSource: v1.VolumeSource{
								HostPath: &v1.HostPathVolumeSource{Path: "somePath"},
								EmptyDir: &v1.EmptyDirVolumeSource{
									SizeLimit: resource.NewQuantity(int64(10), resource.BinarySI)},
							},
						},
					}
				}),
			},
			evictLocalStoragePods:   true,
			evictSystemCriticalPods: false,
			result:                  true,
		}, {
			description: "Pod is evicted because it is bound to a PV and evictLocalStoragePods = false, but it has scheduler.alpha.kubernetes.io/evict annotation",
			pods: []*v1.Pod{
				test.BuildTestPod("p7", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.Annotations = map[string]string{"descheduler.alpha.kubernetes.io/evict": "true"}
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.Spec.Volumes = []v1.Volume{
						{
							Name: "sample",
							VolumeSource: v1.VolumeSource{
								HostPath: &v1.HostPathVolumeSource{Path: "somePath"},
								EmptyDir: &v1.EmptyDirVolumeSource{
									SizeLimit: resource.NewQuantity(int64(10), resource.BinarySI)},
							},
						},
					}
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  true,
		}, {
			description: "Pod not evicted becasuse it is part of a daemonSet",
			pods: []*v1.Pod{
				test.BuildTestPod("p8", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.ObjectMeta.OwnerReferences = test.GetDaemonSetOwnerRefList()
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  false,
		}, {
			description: "Pod is evicted becasuse it is part of a daemonSet, but it has scheduler.alpha.kubernetes.io/evict annotation",
			pods: []*v1.Pod{
				test.BuildTestPod("p9", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.Annotations = map[string]string{"descheduler.alpha.kubernetes.io/evict": "true"}
					pod.ObjectMeta.OwnerReferences = test.GetDaemonSetOwnerRefList()
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  true,
		}, {
			description: "Pod not evicted becasuse it is a mirror poddsa",
			pods: []*v1.Pod{
				test.BuildTestPod("p10", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.Annotations = test.GetMirrorPodAnnotation()
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  false,
		}, {
			description: "Pod is evicted becasuse it is a mirror pod, but it has scheduler.alpha.kubernetes.io/evict annotation",
			pods: []*v1.Pod{
				test.BuildTestPod("p11", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.Annotations = test.GetMirrorPodAnnotation()
					pod.Annotations["descheduler.alpha.kubernetes.io/evict"] = "true"
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  true,
		}, {
			description: "Pod not evicted becasuse it has system critical priority",
			pods: []*v1.Pod{
				test.BuildTestPod("p12", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					priority := utils.SystemCriticalPriority
					pod.Spec.Priority = &priority
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  false,
		}, {
			description: "Pod is evicted becasuse it has system critical priority, but it has scheduler.alpha.kubernetes.io/evict annotation",
			pods: []*v1.Pod{
				test.BuildTestPod("p13", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					priority := utils.SystemCriticalPriority
					pod.Spec.Priority = &priority
					pod.Annotations = map[string]string{
						"descheduler.alpha.kubernetes.io/evict": "true",
					}
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  true,
		}, {
			description: "Pod not evicted becasuse it has a priority higher than the configured priority threshold",
			pods: []*v1.Pod{
				test.BuildTestPod("p14", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.Spec.Priority = &highPriority
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			priorityThreshold:       &lowPriority,
			result:                  false,
		}, {
			description: "Pod is evicted becasuse it has a priority higher than the configured priority threshold, but it has scheduler.alpha.kubernetes.io/evict annotation",
			pods: []*v1.Pod{
				test.BuildTestPod("p15", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.Annotations = map[string]string{"descheduler.alpha.kubernetes.io/evict": "true"}
					pod.Spec.Priority = &highPriority
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			priorityThreshold:       &lowPriority,
			result:                  true,
		}, {
			description: "Pod is evicted becasuse it has system critical priority, but evictSystemCriticalPods = true",
			pods: []*v1.Pod{
				test.BuildTestPod("p16", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					priority := utils.SystemCriticalPriority
					pod.Spec.Priority = &priority
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: true,
			result:                  true,
		}, {
			description: "Pod is evicted becasuse it has system critical priority, but evictSystemCriticalPods = true and it has scheduler.alpha.kubernetes.io/evict annotation",
			pods: []*v1.Pod{
				test.BuildTestPod("p16", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.Annotations = map[string]string{"descheduler.alpha.kubernetes.io/evict": "true"}
					priority := utils.SystemCriticalPriority
					pod.Spec.Priority = &priority
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: true,
			result:                  true,
		}, {
			description: "Pod is evicted becasuse it has a priority higher than the configured priority threshold, but evictSystemCriticalPods = true",
			pods: []*v1.Pod{
				test.BuildTestPod("p17", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.Spec.Priority = &highPriority
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: true,
			priorityThreshold:       &lowPriority,
			result:                  true,
		}, {
			description: "Pod is evicted becasuse it has a priority higher than the configured priority threshold, but evictSystemCriticalPods = true and it has scheduler.alpha.kubernetes.io/evict annotation",
			pods: []*v1.Pod{
				test.BuildTestPod("p17", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.Annotations = map[string]string{"descheduler.alpha.kubernetes.io/evict": "true"}
					pod.Spec.Priority = &highPriority
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: true,
			priorityThreshold:       &lowPriority,
			result:                  true,
		}, {
			description: "Pod with no tolerations running on normal node, all other nodes tainted",
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
				}),
			},
			nodes: []*v1.Node{
				test.BuildTestNode("node2", 1000, 2000, 13, func(node *v1.Node) {
					node.Spec.Taints = []v1.Taint{
						{
							Key:    nodeTaintKey,
							Value:  nodeTaintValue,
							Effect: v1.TaintEffectNoSchedule,
						},
					}
				}),
				test.BuildTestNode("node3", 1000, 2000, 13, func(node *v1.Node) {
					node.Spec.Taints = []v1.Taint{
						{
							Key:    nodeTaintKey,
							Value:  nodeTaintValue,
							Effect: v1.TaintEffectNoSchedule,
						},
					}
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			nodeFit:                 true,
			result:                  false,
		}, {
			description: "Pod with correct tolerations running on normal node, all other nodes tainted",
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.Spec.Tolerations = []v1.Toleration{
						{
							Key:    nodeTaintKey,
							Value:  nodeTaintValue,
							Effect: v1.TaintEffectNoSchedule,
						},
					}
				}),
			},
			nodes: []*v1.Node{
				test.BuildTestNode("node2", 1000, 2000, 13, func(node *v1.Node) {
					node.Spec.Taints = []v1.Taint{
						{
							Key:    nodeTaintKey,
							Value:  nodeTaintValue,
							Effect: v1.TaintEffectNoSchedule,
						},
					}
				}),
				test.BuildTestNode("node3", 1000, 2000, 13, func(node *v1.Node) {
					node.Spec.Taints = []v1.Taint{
						{
							Key:    nodeTaintKey,
							Value:  nodeTaintValue,
							Effect: v1.TaintEffectNoSchedule,
						},
					}
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			nodeFit:                 true,
			result:                  true,
		}, {
			description: "Pod with incorrect node selector",
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.Spec.NodeSelector = map[string]string{
						nodeLabelKey: "fail",
					}
				}),
			},
			nodes: []*v1.Node{
				test.BuildTestNode("node2", 1000, 2000, 13, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}),
				test.BuildTestNode("node3", 1000, 2000, 13, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			nodeFit:                 true,
			result:                  false,
		}, {
			description: "Pod with correct node selector",
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.Spec.NodeSelector = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}),
			},
			nodes: []*v1.Node{
				test.BuildTestNode("node2", 1000, 2000, 13, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}),
				test.BuildTestNode("node3", 1000, 2000, 13, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			nodeFit:                 true,
			result:                  true,
		}, {
			description: "Pod with correct node selector, but only available node doesn't have enough CPU",
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 12, 8, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.Spec.NodeSelector = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}),
			},
			nodes: []*v1.Node{
				test.BuildTestNode("node2-TEST", 10, 16, 10, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}),
				test.BuildTestNode("node3-TEST", 10, 16, 10, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			nodeFit:                 true,
			result:                  false,
		}, {
			description: "Pod with correct node selector, and one node has enough memory",
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 12, 8, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.Spec.NodeSelector = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}),
				test.BuildTestPod("node2-pod-10GB-mem", 20, 10, "node2", func(pod *v1.Pod) {
					pod.ObjectMeta.Labels = map[string]string{
						"test": "true",
					}
				}),
				test.BuildTestPod("node3-pod-10GB-mem", 20, 10, "node3", func(pod *v1.Pod) {
					pod.ObjectMeta.Labels = map[string]string{
						"test": "true",
					}
				}),
			},
			nodes: []*v1.Node{
				test.BuildTestNode("node2", 100, 16, 10, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}),
				test.BuildTestNode("node3", 100, 20, 10, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			nodeFit:                 true,
			result:                  true,
		}, {
			description: "Pod with correct node selector, but both nodes don't have enough memory",
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 12, 8, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.Spec.NodeSelector = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}),
				test.BuildTestPod("node2-pod-10GB-mem", 10, 10, "node2", func(pod *v1.Pod) {
					pod.ObjectMeta.Labels = map[string]string{
						"test": "true",
					}
				}),
				test.BuildTestPod("node3-pod-10GB-mem", 10, 10, "node3", func(pod *v1.Pod) {
					pod.ObjectMeta.Labels = map[string]string{
						"test": "true",
					}
				}),
			},
			nodes: []*v1.Node{
				test.BuildTestNode("node2", 100, 16, 10, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}),
				test.BuildTestNode("node3", 100, 16, 10, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			nodeFit:                 true,
			result:                  false,
		},
	}

	for _, test := range testCases {

		t.Run(test.description, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			nodes := append(test.nodes, n1)

			var objs []runtime.Object
			for _, node := range test.nodes {
				objs = append(objs, node)
			}
			for _, pod := range test.pods {
				objs = append(objs, pod)
			}

			fakeClient := fake.NewSimpleClientset(objs...)

			sharedInformerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
			podInformer := sharedInformerFactory.Core().V1().Pods()

			getPodsAssignedToNode, err := podutil.BuildGetPodsAssignedToNodeFunc(podInformer)
			if err != nil {
				t.Errorf("Build get pods assigned to node function error: %v", err)
			}

			sharedInformerFactory.Start(ctx.Done())
			sharedInformerFactory.WaitForCacheSync(ctx.Done())

			var opts []func(opts *Options)
			if test.priorityThreshold != nil {
				opts = append(opts, WithPriorityThreshold(*test.priorityThreshold))
			}
			if test.nodeFit {
				opts = append(opts, WithNodeFit(true))
			}

			evictorFilter := NewEvictorFilter(
				nodes,
				getPodsAssignedToNode,
				test.evictLocalStoragePods,
				test.evictSystemCriticalPods,
				false,
				test.evictFailedBarePods,
				opts...,
			)

			result := evictorFilter.Filter(test.pods[0])
			if result != test.result {
				t.Errorf("IsEvictable should return for pod %s %t, but it returns %t", test.pods[0].Name, test.result, result)
			}
		})
	}
}

func TestPodTypes(t *testing.T) {
	n1 := test.BuildTestNode("node1", 1000, 2000, 9, nil)
	p1 := test.BuildTestPod("p1", 400, 0, n1.Name, nil)

	// These won't be evicted.
	p2 := test.BuildTestPod("p2", 400, 0, n1.Name, nil)
	p3 := test.BuildTestPod("p3", 400, 0, n1.Name, nil)
	p4 := test.BuildTestPod("p4", 400, 0, n1.Name, nil)

	p1.ObjectMeta.OwnerReferences = test.GetReplicaSetOwnerRefList()
	// The following 4 pods won't get evicted.
	// A daemonset.
	//p2.Annotations = test.GetDaemonSetAnnotation()
	p2.ObjectMeta.OwnerReferences = test.GetDaemonSetOwnerRefList()
	// A pod with local storage.
	p3.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
	p3.Spec.Volumes = []v1.Volume{
		{
			Name: "sample",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{Path: "somePath"},
				EmptyDir: &v1.EmptyDirVolumeSource{
					SizeLimit: resource.NewQuantity(int64(10), resource.BinarySI)},
			},
		},
	}
	// A Mirror Pod.
	p4.Annotations = test.GetMirrorPodAnnotation()
	if !utils.IsMirrorPod(p4) {
		t.Errorf("Expected p4 to be a mirror pod.")
	}
	if !utils.IsPodWithLocalStorage(p3) {
		t.Errorf("Expected p3 to be a pod with local storage.")
	}
	ownerRefList := podutil.OwnerRef(p2)
	if !utils.IsDaemonsetPod(ownerRefList) {
		t.Errorf("Expected p2 to be a daemonset pod.")
	}
	ownerRefList = podutil.OwnerRef(p1)
	if utils.IsDaemonsetPod(ownerRefList) || utils.IsPodWithLocalStorage(p1) || utils.IsCriticalPriorityPod(p1) || utils.IsMirrorPod(p1) || utils.IsStaticPod(p1) {
		t.Errorf("Expected p1 to be a normal pod.")
	}
}

func TestPodEvictedMetrics(t *testing.T) {
	ctx := context.Background()

	// metrics register uses sync.Do (once) so we can only initialize it once
	metricsConfig := &api.MetricsConfig{
		NodeLabels: []string{"topology.kubernetes.io/zone"},
		PodLabels:  []string{"app.kubernetes.io/name"},
	}
	metrics.Register(append(metricsConfig.NodeLabels, metricsConfig.PodLabels...))

	for _, tc := range []struct {
		description        string
		node               *v1.Node
		pod                *v1.Pod
		customMetricLabels []string
	}{
		{
			description: "test custom labels",
			node: test.BuildTestNode("node1", 1000, 2000, 1, func(node *v1.Node) {
				node.Labels["topology.kubernetes.io/zone"] = "zoneA"
			}),
			pod: test.BuildTestPod("p1", 400, 0, "node1", func(pod *v1.Pod) {
				pod.Labels = map[string]string{"app.kubernetes.io/name": "p1"}
			}),
			customMetricLabels: []string{"app_kubernetes_io_name=\"p1\"", "topology_kubernetes_io_zone=\"zoneA\""},
		},
		{
			description:        "test handles missing pod and node label",
			node:               test.BuildTestNode("node1", 1000, 2000, 1, nil),
			pod:                test.BuildTestPod("p1", 400, 0, "node1", nil),
			customMetricLabels: []string{"app_kubernetes_io_name=\"\"", "topology_kubernetes_io_zone=\"\""},
		},
		{
			description: "test handles missing pod label",
			node: test.BuildTestNode("node1", 1000, 2000, 1, func(node *v1.Node) {
				node.Labels["topology.kubernetes.io/zone"] = "zoneA"
			}),
			pod:                test.BuildTestPod("p1", 400, 0, "node1", nil),
			customMetricLabels: []string{"app_kubernetes_io_name=\"\"", "topology_kubernetes_io_zone=\"zoneA\""},
		},
		{
			description: "test handles missing node label",
			node:        test.BuildTestNode("node1", 1000, 2000, 1, nil),
			pod: test.BuildTestPod("p1", 400, 0, "node1", func(pod *v1.Pod) {
				pod.Labels = map[string]string{"app.kubernetes.io/name": "p1"}
			}),
			customMetricLabels: []string{"app_kubernetes_io_name=\"p1\"", "topology_kubernetes_io_zone=\"\""},
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			metrics.PodsEvicted.Reset() // delete all existing metrics first

			const strategy = "RemoveFailedPods"
			fakeClient := fake.NewSimpleClientset(tc.node, tc.pod)

			podEvictor := NewPodEvictor(
				fakeClient,
				"v1",
				false,
				nil,
				nil,
				[]*v1.Node{tc.node},
				true,
				metricsConfig,
			)

			podEvictor.EvictPod(ctx, tc.pod, tc.node, strategy, "metrics labels test")

			metadata := fmt.Sprintf(`
			# HELP %[1]s %s
			# TYPE %[1]s counter
			`, metrics.PodsEvicted.FQName(), metrics.PodsEvicted.Help,
			)

			metricsLabels := []string{
				fmt.Sprintf("namespace=\"%s\"", tc.pod.GetNamespace()),
				fmt.Sprintf("node=\"%s\"", tc.node.GetName()),
				fmt.Sprintf("result=\"%s\"", "success"),
				fmt.Sprintf("strategy=\"%s\"", strategy),
			}
			if tc.customMetricLabels != nil {
				metricsLabels = append(metricsLabels, tc.customMetricLabels...)
			}
			sort.Strings(metricsLabels)

			expected := fmt.Sprintf(`
			%s{%s} 1
			`, metrics.PodsEvicted.FQName(), strings.Join(metricsLabels, ","),
			)
			err := testutil.CollectAndCompare(metrics.PodsEvicted, strings.NewReader(metadata+expected), metrics.PodsEvicted.FQName())
			if err != nil {
				t.Errorf("Error occurred when collecting metrics %s. %v", metrics.PodsEvicted.Name, err)
			}
		})
	}
}
