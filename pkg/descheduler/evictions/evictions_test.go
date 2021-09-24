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
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/utils"
	"sigs.k8s.io/descheduler/test"
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
		got := evictPod(ctx, fakeClient, test.pod, "v1", false)
		if got != test.want {
			t.Errorf("Test error for Desc: %s. Expected %v pod eviction to be %v, got %v", test.description, test.pod.Name, test.want, got)
		}
	}
}

func TestIsEvictable(t *testing.T) {
	ctx := context.Background()
	n1 := test.BuildTestNode("node1", 1000, 2000, 13, nil)
	lowPriority := int32(800)
	highPriority := int32(900)

	nodeTaintKey := "hardware"
	nodeTaintValue := "gpu"

	nodeLabelKey := "datacenter"
	nodeLabelValue := "east"
	type testCase struct {
		pod                     *v1.Pod
		nodes                   []*v1.Node
		podsOnNodes             []v1.Pod
		runBefore               func(*v1.Pod, []*v1.Node)
		evictLocalStoragePods   bool
		evictSystemCriticalPods bool
		priorityThreshold       *int32
		nodeFit                 bool
		result                  bool
	}

	testCases := []testCase{
		{ // Normal pod eviction with normal ownerRefs
			pod: test.BuildTestPod("p1", 400, 0, n1.Name, nil),
			runBefore: func(pod *v1.Pod, nodes []*v1.Node) {
				pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  true,
		}, { // Normal pod eviction with normal ownerRefs and descheduler.alpha.kubernetes.io/evict annotation
			pod: test.BuildTestPod("p2", 400, 0, n1.Name, nil),
			runBefore: func(pod *v1.Pod, nodes []*v1.Node) {
				pod.Annotations = map[string]string{"descheduler.alpha.kubernetes.io/evict": "true"}
				pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  true,
		}, { // Normal pod eviction with replicaSet ownerRefs
			pod: test.BuildTestPod("p3", 400, 0, n1.Name, nil),
			runBefore: func(pod *v1.Pod, nodes []*v1.Node) {
				pod.ObjectMeta.OwnerReferences = test.GetReplicaSetOwnerRefList()
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  true,
		}, { // Normal pod eviction with replicaSet ownerRefs and descheduler.alpha.kubernetes.io/evict annotation
			pod: test.BuildTestPod("p4", 400, 0, n1.Name, nil),
			runBefore: func(pod *v1.Pod, nodes []*v1.Node) {
				pod.Annotations = map[string]string{"descheduler.alpha.kubernetes.io/evict": "true"}
				pod.ObjectMeta.OwnerReferences = test.GetReplicaSetOwnerRefList()
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  true,
		}, { // Normal pod eviction with statefulSet ownerRefs
			pod: test.BuildTestPod("p18", 400, 0, n1.Name, nil),
			runBefore: func(pod *v1.Pod, nodes []*v1.Node) {
				pod.ObjectMeta.OwnerReferences = test.GetStatefulSetOwnerRefList()
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  true,
		}, { // Normal pod eviction with statefulSet ownerRefs and descheduler.alpha.kubernetes.io/evict annotation
			pod: test.BuildTestPod("p19", 400, 0, n1.Name, nil),
			runBefore: func(pod *v1.Pod, nodes []*v1.Node) {
				pod.Annotations = map[string]string{"descheduler.alpha.kubernetes.io/evict": "true"}
				pod.ObjectMeta.OwnerReferences = test.GetStatefulSetOwnerRefList()
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  true,
		}, { // Pod not evicted because it is bound to a PV and evictLocalStoragePods = false
			pod: test.BuildTestPod("p5", 400, 0, n1.Name, nil),
			runBefore: func(pod *v1.Pod, nodes []*v1.Node) {
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
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  false,
		}, { // Pod is evicted because it is bound to a PV and evictLocalStoragePods = true
			pod: test.BuildTestPod("p6", 400, 0, n1.Name, nil),
			runBefore: func(pod *v1.Pod, nodes []*v1.Node) {
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
			},
			evictLocalStoragePods:   true,
			evictSystemCriticalPods: false,
			result:                  true,
		}, { // Pod is evicted because it is bound to a PV and evictLocalStoragePods = false, but it has scheduler.alpha.kubernetes.io/evict annotation
			pod: test.BuildTestPod("p7", 400, 0, n1.Name, nil),
			runBefore: func(pod *v1.Pod, nodes []*v1.Node) {
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
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  true,
		}, { // Pod not evicted becasuse it is part of a daemonSet
			pod: test.BuildTestPod("p8", 400, 0, n1.Name, nil),
			runBefore: func(pod *v1.Pod, nodes []*v1.Node) {
				pod.ObjectMeta.OwnerReferences = test.GetDaemonSetOwnerRefList()
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  false,
		}, { // Pod is evicted becasuse it is part of a daemonSet, but it has scheduler.alpha.kubernetes.io/evict annotation
			pod: test.BuildTestPod("p9", 400, 0, n1.Name, nil),
			runBefore: func(pod *v1.Pod, nodes []*v1.Node) {
				pod.Annotations = map[string]string{"descheduler.alpha.kubernetes.io/evict": "true"}
				pod.ObjectMeta.OwnerReferences = test.GetDaemonSetOwnerRefList()
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  true,
		}, { // Pod not evicted becasuse it is a mirror pod
			pod: test.BuildTestPod("p10", 400, 0, n1.Name, nil),
			runBefore: func(pod *v1.Pod, nodes []*v1.Node) {
				pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
				pod.Annotations = test.GetMirrorPodAnnotation()
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  false,
		}, { // Pod is evicted becasuse it is a mirror pod, but it has scheduler.alpha.kubernetes.io/evict annotation
			pod: test.BuildTestPod("p11", 400, 0, n1.Name, nil),
			runBefore: func(pod *v1.Pod, nodes []*v1.Node) {
				pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
				pod.Annotations = test.GetMirrorPodAnnotation()
				pod.Annotations["descheduler.alpha.kubernetes.io/evict"] = "true"
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  true,
		}, { // Pod not evicted becasuse it has system critical priority
			pod: test.BuildTestPod("p12", 400, 0, n1.Name, nil),
			runBefore: func(pod *v1.Pod, nodes []*v1.Node) {
				pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
				priority := utils.SystemCriticalPriority
				pod.Spec.Priority = &priority
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  false,
		}, { // Pod is evicted becasuse it has system critical priority, but it has scheduler.alpha.kubernetes.io/evict annotation
			pod: test.BuildTestPod("p13", 400, 0, n1.Name, nil),
			runBefore: func(pod *v1.Pod, nodes []*v1.Node) {
				pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
				priority := utils.SystemCriticalPriority
				pod.Spec.Priority = &priority
				pod.Annotations = map[string]string{
					"descheduler.alpha.kubernetes.io/evict": "true",
				}
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  true,
		}, { // Pod not evicted becasuse it has a priority higher than the configured priority threshold
			pod: test.BuildTestPod("p14", 400, 0, n1.Name, nil),
			runBefore: func(pod *v1.Pod, nodes []*v1.Node) {
				pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
				pod.Spec.Priority = &highPriority
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			priorityThreshold:       &lowPriority,
			result:                  false,
		}, { // Pod is evicted becasuse it has a priority higher than the configured priority threshold, but it has scheduler.alpha.kubernetes.io/evict annotation
			pod: test.BuildTestPod("p15", 400, 0, n1.Name, nil),
			runBefore: func(pod *v1.Pod, nodes []*v1.Node) {
				pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
				pod.Annotations = map[string]string{"descheduler.alpha.kubernetes.io/evict": "true"}
				pod.Spec.Priority = &highPriority
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			priorityThreshold:       &lowPriority,
			result:                  true,
		}, { // Pod is evicted becasuse it has system critical priority, but evictSystemCriticalPods = true
			pod: test.BuildTestPod("p16", 400, 0, n1.Name, nil),
			runBefore: func(pod *v1.Pod, nodes []*v1.Node) {
				pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
				priority := utils.SystemCriticalPriority
				pod.Spec.Priority = &priority
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: true,
			result:                  true,
		}, { // Pod is evicted becasuse it has system critical priority, but evictSystemCriticalPods = true and it has scheduler.alpha.kubernetes.io/evict annotation
			pod: test.BuildTestPod("p16", 400, 0, n1.Name, nil),
			runBefore: func(pod *v1.Pod, nodes []*v1.Node) {
				pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
				pod.Annotations = map[string]string{"descheduler.alpha.kubernetes.io/evict": "true"}
				priority := utils.SystemCriticalPriority
				pod.Spec.Priority = &priority
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: true,
			result:                  true,
		}, { // Pod is evicted becasuse it has a priority higher than the configured priority threshold, but evictSystemCriticalPods = true
			pod: test.BuildTestPod("p17", 400, 0, n1.Name, nil),
			runBefore: func(pod *v1.Pod, nodes []*v1.Node) {
				pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
				pod.Spec.Priority = &highPriority
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: true,
			priorityThreshold:       &lowPriority,
			result:                  true,
		}, { // Pod is evicted becasuse it has a priority higher than the configured priority threshold, but evictSystemCriticalPods = true and it has scheduler.alpha.kubernetes.io/evict annotation
			pod: test.BuildTestPod("p17", 400, 0, n1.Name, nil),
			runBefore: func(pod *v1.Pod, nodes []*v1.Node) {
				pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
				pod.Annotations = map[string]string{"descheduler.alpha.kubernetes.io/evict": "true"}
				pod.Spec.Priority = &highPriority
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: true,
			priorityThreshold:       &lowPriority,
			result:                  true,
		}, { // Pod with no tolerations running on normal node, all other nodes tainted
			pod:   test.BuildTestPod("p1", 400, 0, n1.Name, nil),
			nodes: []*v1.Node{test.BuildTestNode("node2", 1000, 2000, 13, nil), test.BuildTestNode("node3", 1000, 2000, 13, nil)},
			runBefore: func(pod *v1.Pod, nodes []*v1.Node) {
				pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()

				for _, node := range nodes {
					node.Spec.Taints = []v1.Taint{
						{
							Key:    nodeTaintKey,
							Value:  nodeTaintValue,
							Effect: v1.TaintEffectNoSchedule,
						},
					}
				}
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			nodeFit:                 true,
			result:                  false,
		}, { // Pod with correct tolerations running on normal node, all other nodes tainted
			pod: test.BuildTestPod("p1", 400, 0, n1.Name, func(pod *v1.Pod) {
				pod.Spec.Tolerations = []v1.Toleration{
					{
						Key:    nodeTaintKey,
						Value:  nodeTaintValue,
						Effect: v1.TaintEffectNoSchedule,
					},
				}
			}),
			nodes: []*v1.Node{test.BuildTestNode("node2", 1000, 2000, 13, nil), test.BuildTestNode("node3", 1000, 2000, 13, nil)},
			runBefore: func(pod *v1.Pod, nodes []*v1.Node) {
				pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()

				for _, node := range nodes {
					node.Spec.Taints = []v1.Taint{
						{
							Key:    nodeTaintKey,
							Value:  nodeTaintValue,
							Effect: v1.TaintEffectNoSchedule,
						},
					}
				}
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			nodeFit:                 true,
			result:                  true,
		}, { // Pod with incorrect node selector
			pod: test.BuildTestPod("p1", 400, 0, n1.Name, func(pod *v1.Pod) {
				pod.Spec.NodeSelector = map[string]string{
					nodeLabelKey: "fail",
				}
			}),
			nodes: []*v1.Node{test.BuildTestNode("node2", 1000, 2000, 13, nil), test.BuildTestNode("node3", 1000, 2000, 13, nil)},
			runBefore: func(pod *v1.Pod, nodes []*v1.Node) {
				pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()

				for _, node := range nodes {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			nodeFit:                 true,
			result:                  false,
		}, { // Pod with correct node selector
			pod: test.BuildTestPod("p1", 400, 0, n1.Name, func(pod *v1.Pod) {
				pod.Spec.NodeSelector = map[string]string{
					nodeLabelKey: nodeLabelValue,
				}
			}),
			nodes: []*v1.Node{test.BuildTestNode("node2", 1000, 2000, 13, nil), test.BuildTestNode("node3", 1000, 2000, 13, nil)},
			runBefore: func(pod *v1.Pod, nodes []*v1.Node) {
				pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()

				for _, node := range nodes {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			nodeFit:                 true,
			result:                  true,
		}, { // Pod with correct node selector, but only available node doesn't have enough CPU
			pod: test.BuildTestPod("p1", 12, 8, n1.Name, func(pod *v1.Pod) {
				pod.Spec.NodeSelector = map[string]string{
					nodeLabelKey: nodeLabelValue,
				}
			}),
			nodes: []*v1.Node{test.BuildTestNode("node2", 10, 16, 10, nil), test.BuildTestNode("node3", 10, 16, 10, nil)},
			runBefore: func(pod *v1.Pod, nodes []*v1.Node) {
				pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()

				for _, node := range nodes {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			nodeFit:                 true,
			result:                  false,
		}, { // Pod with correct node selector, and one node has enough memory
			pod: test.BuildTestPod("p1", 12, 8, n1.Name, func(pod *v1.Pod) {
				pod.Spec.NodeSelector = map[string]string{
					nodeLabelKey: nodeLabelValue,
				}
			}),
			nodes: []*v1.Node{
				test.BuildTestNode("node2", 100, 16, 10, nil),
				test.BuildTestNode("node3", 100, 20, 10, nil),
			},
			podsOnNodes: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "node2-pod-10GB-mem",
						Namespace: "test",
						Labels: map[string]string{
							"test": "true",
						},
					},
					Spec: v1.PodSpec{
						NodeName: "node2",
						Containers: []v1.Container{
							{
								Name: "backend",
								Resources: v1.ResourceRequirements{
									Requests: v1.ResourceList{
										v1.ResourceCPU:    *resource.NewMilliQuantity(20, resource.DecimalSI),
										v1.ResourceMemory: *resource.NewQuantity(10, resource.DecimalSI),
									},
								},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "node3-pod-10GB-mem",
						Namespace: "test",
						Labels: map[string]string{
							"test": "true",
						},
					},
					Spec: v1.PodSpec{
						NodeName: "node3",
						Containers: []v1.Container{
							{
								Name: "backend",
								Resources: v1.ResourceRequirements{
									Requests: v1.ResourceList{
										v1.ResourceCPU:    *resource.NewMilliQuantity(20, resource.DecimalSI),
										v1.ResourceMemory: *resource.NewQuantity(10, resource.DecimalSI),
									},
								},
							},
						},
					},
				},
			},
			runBefore: func(pod *v1.Pod, nodes []*v1.Node) {
				pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()

				for _, node := range nodes {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			nodeFit:                 true,
			result:                  true,
		}, { // Pod with correct node selector, but both nodes don't have enough memory
			pod: test.BuildTestPod("p1", 12, 8, n1.Name, func(pod *v1.Pod) {
				pod.Spec.NodeSelector = map[string]string{
					nodeLabelKey: nodeLabelValue,
				}
			}),
			nodes: []*v1.Node{
				test.BuildTestNode("node2", 100, 16, 10, nil),
				test.BuildTestNode("node3", 100, 16, 10, nil),
			},
			podsOnNodes: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "node2-pod-10GB-mem",
						Namespace: "test",
						Labels: map[string]string{
							"test": "true",
						},
					},
					Spec: v1.PodSpec{
						NodeName: "node2",
						Containers: []v1.Container{
							{
								Name: "backend",
								Resources: v1.ResourceRequirements{
									Requests: v1.ResourceList{
										v1.ResourceCPU:    *resource.NewMilliQuantity(5000, resource.DecimalSI),
										v1.ResourceMemory: *resource.NewQuantity(10, resource.DecimalSI),
									},
								},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "node3-pod-10GB-mem",
						Namespace: "test",
						Labels: map[string]string{
							"test": "true",
						},
					},
					Spec: v1.PodSpec{
						NodeName: "node3",
						Containers: []v1.Container{
							{
								Name: "backend",
								Resources: v1.ResourceRequirements{
									Requests: v1.ResourceList{
										v1.ResourceCPU:    *resource.NewMilliQuantity(5000, resource.DecimalSI),
										v1.ResourceMemory: *resource.NewQuantity(10, resource.DecimalSI),
									},
								},
							},
						},
					},
				},
			},
			runBefore: func(pod *v1.Pod, nodes []*v1.Node) {
				pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()

				for _, node := range nodes {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			nodeFit:                 true,
			result:                  false,
		},
	}

	for _, test := range testCases {
		test.runBefore(test.pod, test.nodes)
		nodes := append(test.nodes, n1)

		fakeClient := &fake.Clientset{}
		fakeClient.Fake.AddReactor("list", "pods", func(action core.Action) (bool, runtime.Object, error) {
			return true, &v1.PodList{Items: test.podsOnNodes}, nil
		})

		podEvictor := &PodEvictor{
			client:                  fakeClient,
			evictLocalStoragePods:   test.evictLocalStoragePods,
			evictSystemCriticalPods: test.evictSystemCriticalPods,
			nodes:                   nodes,
		}

		var opts []func(opts *Options)
		if test.priorityThreshold != nil {
			opts = append(opts, WithPriorityThreshold(*test.priorityThreshold))
		}
		if test.nodeFit {
			opts = append(opts, WithNodeFit(true))
		}
		evictable := podEvictor.Evictable(opts...)

		result := evictable.IsEvictable(ctx, test.pod)
		if result != test.result {
			t.Errorf("IsEvictable should return for pod %s %t, but it returns %t", test.pod.Name, test.result, result)
		}

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
