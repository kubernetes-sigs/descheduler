/*
Copyright 2021 The Kubernetes Authors.

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
	policy "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	frameworktesting "sigs.k8s.io/descheduler/pkg/framework/testing"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
	"sigs.k8s.io/descheduler/pkg/utils"
	"sigs.k8s.io/descheduler/test"
)

func TestHighNodeUtilization(t *testing.T) {
	n1NodeName := "n1"
	n2NodeName := "n2"
	n3NodeName := "n3"

	nodeSelectorKey := "datacenter"
	nodeSelectorValue := "west"

	testCases := []struct {
		name                string
		thresholds          api.ResourceThresholds
		nodes               []*v1.Node
		pods                []*v1.Pod
		expectedPodsEvicted uint
		evictedPods         []string
	}{
		{
			name: "no node below threshold usage",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:  20,
				v1.ResourcePods: 20,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 3000, 10, nil),
				test.BuildTestNode(n2NodeName, 4000, 3000, 10, nil),
				test.BuildTestNode(n3NodeName, 4000, 3000, 10, nil),
			},
			pods: []*v1.Pod{
				// These won't be evicted.
				test.BuildTestPod("p1", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p2", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p3", 400, 0, n1NodeName, test.SetRSOwnerRef),
				// These won't be evicted.
				test.BuildTestPod("p4", 400, 0, n2NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p5", 400, 0, n2NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p6", 400, 0, n2NodeName, test.SetRSOwnerRef),
				// These won't be evicted.
				test.BuildTestPod("p7", 400, 0, n3NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p8", 400, 0, n3NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p9", 400, 0, n3NodeName, test.SetRSOwnerRef),
			},
			expectedPodsEvicted: 0,
		},
		{
			name: "no evictable pods",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:  40,
				v1.ResourcePods: 40,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 3000, 9, nil),
				test.BuildTestNode(n2NodeName, 4000, 3000, 10, nil),
				test.BuildTestNode(n3NodeName, 4000, 3000, 10, nil),
			},
			pods: []*v1.Pod{
				// These won't be evicted.
				test.BuildTestPod("p1", 400, 0, n1NodeName, func(pod *v1.Pod) {
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
				test.BuildTestPod("p2", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A Critical Pod.
					test.SetNormalOwnerRef(pod)
					pod.Namespace = "kube-system"
					priority := utils.SystemCriticalPriority
					pod.Spec.Priority = &priority
				}),
				// These won't be evicted.
				test.BuildTestPod("p3", 400, 0, n2NodeName, test.SetDSOwnerRef),
				test.BuildTestPod("p4", 400, 0, n2NodeName, test.SetDSOwnerRef),
				test.BuildTestPod("p5", 400, 0, n3NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p6", 400, 0, n3NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p7", 400, 0, n3NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p8", 400, 0, n3NodeName, test.SetRSOwnerRef),
			},
			expectedPodsEvicted: 0,
		},
		{
			name: "no node to schedule evicted pods",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:  20,
				v1.ResourcePods: 20,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 3000, 10, nil),
				test.BuildTestNode(n2NodeName, 4000, 3000, 10, nil),
				test.BuildTestNode(n3NodeName, 4000, 3000, 10, test.SetNodeUnschedulable),
			},
			pods: []*v1.Pod{
				// These can't be evicted.
				test.BuildTestPod("p1", 400, 0, n1NodeName, test.SetRSOwnerRef),
				// These can't be evicted.
				test.BuildTestPod("p2", 400, 0, n2NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p3", 400, 0, n3NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p4", 400, 0, n3NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p5", 400, 0, n3NodeName, test.SetRSOwnerRef),
			},
			expectedPodsEvicted: 0,
		},
		{
			name: "without priorities",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:  30,
				v1.ResourcePods: 30,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 3000, 10, nil),
				test.BuildTestNode(n2NodeName, 4000, 3000, 10, nil),
				test.BuildTestNode(n3NodeName, 4000, 3000, 10, test.SetNodeUnschedulable),
			},
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1NodeName, test.SetRSOwnerRef),
				// These won't be evicted.
				test.BuildTestPod("p2", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A Critical Pod.
					test.SetNormalOwnerRef(pod)
					pod.Namespace = "kube-system"
					priority := utils.SystemCriticalPriority
					pod.Spec.Priority = &priority
				}),
				// These won't be evicted.
				test.BuildTestPod("p3", 400, 0, n2NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p4", 400, 0, n2NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p5", 400, 0, n2NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p6", 400, 0, n2NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p7", 400, 0, n3NodeName, test.SetRSOwnerRef),
			},
			expectedPodsEvicted: 2,
			evictedPods:         []string{"p1", "p7"},
		},
		{
			name: "without priorities stop when resource capacity is depleted",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:  30,
				v1.ResourcePods: 30,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 2000, 3000, 10, nil),
				test.BuildTestNode(n2NodeName, 2000, 3000, 10, nil),
				test.BuildTestNode(n3NodeName, 2000, 3000, 10, test.SetNodeUnschedulable),
			},
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1NodeName, test.SetRSOwnerRef),
				// These won't be evicted.
				test.BuildTestPod("p2", 400, 0, n2NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p3", 400, 0, n2NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p4", 400, 0, n2NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p5", 400, 0, n2NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p6", 400, 0, n3NodeName, test.SetRSOwnerRef),
			},
			expectedPodsEvicted: 1,
		},
		{
			name: "with priorities",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU: 30,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 3000, 10, nil),
				test.BuildTestNode(n2NodeName, 2000, 3000, 10, nil),
				test.BuildTestNode(n3NodeName, 2000, 3000, 10, test.SetNodeUnschedulable),
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
				test.BuildTestPod("p5", 400, 0, n2NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p6", 400, 0, n2NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p7", 400, 0, n2NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p8", 400, 0, n2NodeName, test.SetRSOwnerRef),
				// These won't be evicted.
				test.BuildTestPod("p9", 400, 0, n3NodeName, test.SetDSOwnerRef),
			},
			expectedPodsEvicted: 1,
			evictedPods:         []string{"p1"},
		},
		{
			name: "without priorities evicting best-effort pods only",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU: 30,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 3000, 3000, 10, nil),
				test.BuildTestNode(n2NodeName, 3000, 3000, 5, nil),
				test.BuildTestNode(n3NodeName, 3000, 3000, 10, test.SetNodeUnschedulable),
			},
			// All pods are assumed to be burstable (test.BuildTestNode always sets both cpu/memory resource requests to some value)
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.MakeBestEffortPod(pod)
				}),
				test.BuildTestPod("p2", 400, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
				}),
				// These won't be evicted.
				test.BuildTestPod("p3", 400, 0, n2NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p4", 400, 0, n2NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p5", 400, 0, n2NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p6", 400, 0, n2NodeName, test.SetRSOwnerRef),
			},
			expectedPodsEvicted: 1,
			evictedPods:         []string{"p1"},
		},
		{
			name: "with extended resource",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:   20,
				extendedResource: 40,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 3000, 10, func(node *v1.Node) {
					test.SetNodeExtendedResource(node, extendedResource, 8)
				}),
				test.BuildTestNode(n2NodeName, 4000, 3000, 10, func(node *v1.Node) {
					test.SetNodeExtendedResource(node, extendedResource, 8)
				}),
				test.BuildTestNode(n3NodeName, 4000, 3000, 10, test.SetNodeUnschedulable),
			},
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 100, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.SetPodExtendedResourceRequest(pod, extendedResource, 1)
				}),
				test.BuildTestPod("p2", 100, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.SetPodExtendedResourceRequest(pod, extendedResource, 1)
				}),
				// These won't be evicted
				test.BuildTestPod("p3", 500, 0, n2NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.SetPodExtendedResourceRequest(pod, extendedResource, 1)
				}),
				test.BuildTestPod("p4", 500, 0, n2NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.SetPodExtendedResourceRequest(pod, extendedResource, 1)
				}),
				test.BuildTestPod("p5", 500, 0, n2NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.SetPodExtendedResourceRequest(pod, extendedResource, 1)
				}),
				test.BuildTestPod("p6", 500, 0, n2NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.SetPodExtendedResourceRequest(pod, extendedResource, 1)
				}),
			},
			expectedPodsEvicted: 2,
			evictedPods:         []string{"p1", "p2"},
		},
		{
			name: "with extended resource in some of nodes",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:   40,
				extendedResource: 40,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 3000, 10, func(node *v1.Node) {
					test.SetNodeExtendedResource(node, extendedResource, 8)
				}),
				test.BuildTestNode(n2NodeName, 4000, 3000, 10, nil),
				test.BuildTestNode(n3NodeName, 4000, 3000, 10, test.SetNodeUnschedulable),
			},
			pods: []*v1.Pod{
				// These won't be evicted
				test.BuildTestPod("p1", 100, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.SetPodExtendedResourceRequest(pod, extendedResource, 1)
				}),
				test.BuildTestPod("p2", 100, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.SetPodExtendedResourceRequest(pod, extendedResource, 1)
				}),
				test.BuildTestPod("p3", 500, 0, n2NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p4", 500, 0, n2NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p5", 500, 0, n2NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p6", 500, 0, n2NodeName, test.SetRSOwnerRef),
			},
			expectedPodsEvicted: 0,
		},
		{
			name: "Other node match pod node selector",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:  30,
				v1.ResourcePods: 30,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 3000, 9, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeSelectorKey: nodeSelectorValue,
					}
				}),
				test.BuildTestNode(n2NodeName, 4000, 3000, 10, nil),
			},
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p2", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p3", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p4", 400, 0, n1NodeName, test.SetDSOwnerRef),
				test.BuildTestPod("p5", 400, 0, n2NodeName, func(pod *v1.Pod) {
					// A pod selecting nodes in the "west" datacenter
					test.SetRSOwnerRef(pod)
					pod.Spec.NodeSelector = map[string]string{
						nodeSelectorKey: nodeSelectorValue,
					}
				}),
			},
			expectedPodsEvicted: 1,
		},
		{
			name: "Other node does not match pod node selector",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:  30,
				v1.ResourcePods: 30,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 3000, 9, nil),
				test.BuildTestNode(n2NodeName, 4000, 3000, 10, nil),
			},
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p2", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p3", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p4", 400, 0, n1NodeName, test.SetDSOwnerRef),
				test.BuildTestPod("p5", 400, 0, n2NodeName, func(pod *v1.Pod) {
					// A pod selecting nodes in the "west" datacenter
					test.SetRSOwnerRef(pod)
					pod.Spec.NodeSelector = map[string]string{
						nodeSelectorKey: nodeSelectorValue,
					}
				}),
			},
			expectedPodsEvicted: 0,
		},
		{
			name: "Other node does not have enough Memory",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:  30,
				v1.ResourcePods: 30,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 200, 9, nil),
				test.BuildTestNode(n2NodeName, 4000, 3000, 10, nil),
			},
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 50, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p2", 400, 50, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p3", 400, 50, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p4", 400, 50, n1NodeName, test.SetDSOwnerRef),
				test.BuildTestPod("p5", 400, 100, n2NodeName, func(pod *v1.Pod) {
					// A pod requesting more memory than is available on node1
					test.SetRSOwnerRef(pod)
				}),
			},
			expectedPodsEvicted: 0,
		},
		{
			name: "Other node does not have enough Memory",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:  30,
				v1.ResourcePods: 30,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 200, 9, nil),
				test.BuildTestNode(n2NodeName, 4000, 3000, 10, nil),
			},
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 50, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p2", 400, 50, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p3", 400, 50, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p4", 400, 50, n1NodeName, test.SetDSOwnerRef),
				test.BuildTestPod("p5", 400, 100, n2NodeName, func(pod *v1.Pod) {
					// A pod requesting more memory than is available on node1
					test.SetRSOwnerRef(pod)
				}),
			},
			expectedPodsEvicted: 0,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			var objs []runtime.Object
			for _, node := range testCase.nodes {
				objs = append(objs, node)
			}
			for _, pod := range testCase.pods {
				objs = append(objs, pod)
			}
			podsForEviction := make(map[string]struct{})
			for _, pod := range testCase.evictedPods {
				podsForEviction[pod] = struct{}{}
			}
			fakeClient := fake.NewSimpleClientset(objs...)

			handle, podEvictor, err := frameworktesting.InitFrameworkHandle(ctx, fakeClient, nil, defaultevictor.DefaultEvictorArgs{NodeFit: true}, nil)
			if err != nil {
				t.Fatalf("Unable to initialize a framework handle: %v", err)
			}

			evictionFailed := false
			if len(testCase.evictedPods) > 0 {
				fakeClient.Fake.AddReactor("create", "pods", func(action core.Action) (bool, runtime.Object, error) {
					getAction := action.(core.CreateAction)
					obj := getAction.GetObject()
					if eviction, ok := obj.(*policy.Eviction); ok {
						if _, exists := podsForEviction[eviction.Name]; exists {
							return true, obj, nil
						}
						evictionFailed = true
						return true, nil, fmt.Errorf("pod %q was unexpectedly evicted", eviction.Name)
					}
					return true, obj, nil
				})
			}

			plugin, err := NewHighNodeUtilization(ctx, &HighNodeUtilizationArgs{
				Thresholds: testCase.thresholds,
			},
				handle)
			if err != nil {
				t.Fatalf("Unable to initialize the plugin: %v", err)
			}
			plugin.(frameworktypes.BalancePlugin).Balance(ctx, testCase.nodes)

			podsEvicted := podEvictor.TotalEvicted()
			if testCase.expectedPodsEvicted != podsEvicted {
				t.Errorf("Expected %v pods to be evicted but %v got evicted", testCase.expectedPodsEvicted, podsEvicted)
			}
			if evictionFailed {
				t.Errorf("Pod evictions failed unexpectedly")
			}
		})
	}
}

func TestHighNodeUtilizationWithTaints(t *testing.T) {
	n1 := test.BuildTestNode("n1", 1000, 3000, 10, nil)
	n2 := test.BuildTestNode("n2", 1000, 3000, 10, nil)
	n3 := test.BuildTestNode("n3", 1000, 3000, 10, nil)
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
		name              string
		nodes             []*v1.Node
		pods              []*v1.Pod
		evictionsExpected uint
	}{
		{
			name:  "No taints",
			nodes: []*v1.Node{n1, n2, n3},
			pods: []*v1.Pod{
				// Node 1 pods
				test.BuildTestPod(fmt.Sprintf("pod_1_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_2_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_3_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				// Node 2 pods
				test.BuildTestPod(fmt.Sprintf("pod_4_%s", n2.Name), 200, 0, n2.Name, test.SetRSOwnerRef),
			},
			evictionsExpected: 1,
		},
		{
			name:  "No pod tolerates node taint",
			nodes: []*v1.Node{n1, n3withTaints},
			pods: []*v1.Pod{
				// Node 1 pods
				test.BuildTestPod(fmt.Sprintf("pod_1_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				// Node 3 pods
				test.BuildTestPod(fmt.Sprintf("pod_2_%s", n3withTaints.Name), 200, 0, n3withTaints.Name, test.SetRSOwnerRef),
			},
			evictionsExpected: 0,
		},
		{
			name:  "Pod which tolerates node taint",
			nodes: []*v1.Node{n1, n3withTaints},
			pods: []*v1.Pod{
				// Node 1 pods
				test.BuildTestPod(fmt.Sprintf("pod_1_%s", n1.Name), 100, 0, n1.Name, test.SetRSOwnerRef),
				podThatToleratesTaint,
				// Node 3 pods
				test.BuildTestPod(fmt.Sprintf("pod_9_%s", n3withTaints.Name), 500, 0, n3withTaints.Name, test.SetRSOwnerRef),
			},
			evictionsExpected: 1,
		},
	}

	for _, item := range tests {
		t.Run(item.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			var objs []runtime.Object
			for _, node := range item.nodes {
				objs = append(objs, node)
			}

			for _, pod := range item.pods {
				objs = append(objs, pod)
			}

			fakeClient := fake.NewSimpleClientset(objs...)

			handle, podEvictor, err := frameworktesting.InitFrameworkHandle(
				ctx,
				fakeClient,
				evictions.NewOptions().WithMaxPodsToEvictPerNode(&item.evictionsExpected),
				defaultevictor.DefaultEvictorArgs{},
				nil,
			)
			if err != nil {
				t.Fatalf("Unable to initialize a framework handle: %v", err)
			}

			plugin, err := NewHighNodeUtilization(ctx, &HighNodeUtilizationArgs{
				Thresholds: api.ResourceThresholds{
					v1.ResourceCPU: 40,
				},
			},
				handle)
			if err != nil {
				t.Fatalf("Unable to initialize the plugin: %v", err)
			}
			plugin.(frameworktypes.BalancePlugin).Balance(ctx, item.nodes)

			if item.evictionsExpected != podEvictor.TotalEvicted() {
				t.Errorf("Expected %v evictions, got %v", item.evictionsExpected, podEvictor.TotalEvicted())
			}
		})
	}
}
