/*
Copyright 2022 The Kubernetes Authors.

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

package removeduplicates

import (
	"context"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"

	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	frameworktesting "sigs.k8s.io/descheduler/pkg/framework/testing"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
	"sigs.k8s.io/descheduler/pkg/utils"
	"sigs.k8s.io/descheduler/test"
)

const nodeName1 = "n1"

func buildTestPodForNode1(name string, apply func(*v1.Pod)) *v1.Pod {
	return test.BuildTestPod(name, 100, 0, nodeName1, apply)
}

func buildTestPodWithImage(podName, image string) *v1.Pod {
	pod := buildTestPodForNode1(podName, test.SetRSOwnerRef)
	pod.Spec.Containers = append(pod.Spec.Containers, v1.Container{
		Name:  image,
		Image: image,
	})
	return pod
}

func buildTestPodWithRSOwnerRefForNode1(name string, apply func(*v1.Pod)) *v1.Pod {
	return buildTestPodForNode1(name, func(pod *v1.Pod) {
		test.SetRSOwnerRef(pod)
		if apply != nil {
			apply(pod)
		}
	})
}

func buildTestPodWithRSOwnerRefWithNamespaceForNode1(name, namespace string, apply func(*v1.Pod)) *v1.Pod {
	return buildTestPodWithRSOwnerRefForNode1(name, func(pod *v1.Pod) {
		pod.Namespace = namespace
		if apply != nil {
			apply(pod)
		}
	})
}

func TestFindDuplicatePods(t *testing.T) {
	// first setup pods
	node1 := test.BuildTestNode("n1", 2000, 3000, 10, nil)
	node2 := test.BuildTestNode("n2", 2000, 3000, 10, nil)
	node3 := test.BuildTestNode("n3", 2000, 3000, 10, func(node *v1.Node) {
		node.Spec.Taints = []v1.Taint{
			{
				Key:    "hardware",
				Value:  "gpu",
				Effect: v1.TaintEffectNoSchedule,
			},
		}
	})
	node4 := test.BuildTestNode("n4", 2000, 3000, 10, func(node *v1.Node) {
		node.ObjectMeta.Labels = map[string]string{
			"datacenter": "east",
		}
	})
	node5 := test.BuildTestNode("n5", 2000, 3000, 10, func(node *v1.Node) {
		node.Spec = v1.NodeSpec{
			Unschedulable: true,
		}
	})
	node6 := test.BuildTestNode("n6", 200, 200, 10, nil)

	// Three Pods in the "dev" Namespace, bound to same ReplicaSet. 2 should be evicted.
	// A DaemonSet.
	// A Pod with local storage.
	// A Mirror Pod.
	// A Critical Pod.
	// Three Pods in the "test" Namespace, bound to same ReplicaSet. 2 should be evicted.
	// Same owners, but different images
	// Multiple containers
	// ### Pods Evictable Based On Node Fit ###
	p16 := buildTestPodWithRSOwnerRefWithNamespaceForNode1("NOT1", "node-fit", func(pod *v1.Pod) {
		pod.Spec.NodeSelector = map[string]string{
			"datacenter": "west",
		}
	})
	p17 := buildTestPodWithRSOwnerRefWithNamespaceForNode1("NOT2", "node-fit", func(pod *v1.Pod) {
		pod.Spec.NodeSelector = map[string]string{
			"datacenter": "west",
		}
	})

	// This pod sits on node6 and is used to take up CPU requests on the node
	p19 := test.BuildTestPod("CPU-eater", 150, 150, node6.Name, func(pod *v1.Pod) {
		pod.Namespace = "test"
	})

	// Dummy pod for node6 used to do the opposite of p19
	p20 := test.BuildTestPod("CPU-saver", 100, 150, node6.Name, func(pod *v1.Pod) {
		pod.Namespace = "test"
	})

	// ### Evictable Pods ###

	// ### Non-evictable Pods ###

	testCases := []struct {
		description             string
		pods                    []*v1.Pod
		nodes                   []*v1.Node
		expectedEvictedPodCount uint
		excludeOwnerKinds       []string
		nodefit                 bool
	}{
		{
			description: "Three pods in the `dev` Namespace, bound to same ReplicaSet. 1 should be evicted.",
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p1", "dev", nil),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p2", "dev", nil),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p3", "dev", nil),
			},
			nodes:                   []*v1.Node{node1, node2},
			expectedEvictedPodCount: 1,
		},
		{
			description: "Three pods in the `dev` Namespace, bound to same ReplicaSet, but ReplicaSet kind is excluded. 0 should be evicted.",
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p1", "dev", nil),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p2", "dev", nil),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p3", "dev", nil),
			},
			nodes:                   []*v1.Node{node1, node2},
			expectedEvictedPodCount: 0,
			excludeOwnerKinds:       []string{"ReplicaSet"},
		},
		{
			description: "Three Pods in the `test` Namespace, bound to same ReplicaSet. 1 should be evicted.",
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p8", "test", nil),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p9", "test", nil),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p10", "test", nil),
			},
			nodes:                   []*v1.Node{node1, node2},
			expectedEvictedPodCount: 1,
		},
		{
			description: "Three Pods in the `dev` Namespace, three Pods in the `test` Namespace. Bound to ReplicaSet with same name. 4 should be evicted.",
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p1", "dev", nil),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p2", "dev", nil),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p3", "dev", nil),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p8", "test", nil),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p9", "test", nil),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p10", "test", nil),
			},
			nodes:                   []*v1.Node{node1, node2},
			expectedEvictedPodCount: 2,
		},
		{
			description: "Pods are: part of DaemonSet, with local storage, mirror pod annotation, critical pod annotation - none should be evicted.",
			pods: []*v1.Pod{
				buildTestPodForNode1("p4", func(pod *v1.Pod) {
					test.SetDSOwnerRef(pod)
				}),
				buildTestPodForNode1("p5", func(pod *v1.Pod) {
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
				}),
				buildTestPodForNode1("p6", func(pod *v1.Pod) {
					pod.Annotations = test.GetMirrorPodAnnotation()
				}),
				buildTestPodForNode1("p7", func(pod *v1.Pod) {
					pod.Namespace = "kube-system"
					priority := utils.SystemCriticalPriority
					pod.Spec.Priority = &priority
				}),
			},
			nodes:                   []*v1.Node{node1, node2},
			expectedEvictedPodCount: 0,
		},
		{
			description: "Test all Pods: 4 should be evicted.",
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p1", "dev", nil),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p2", "dev", nil),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p3", "dev", nil),
				buildTestPodForNode1("p4", func(pod *v1.Pod) {
					test.SetDSOwnerRef(pod)
				}),
				buildTestPodForNode1("p5", func(pod *v1.Pod) {
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
				}),
				buildTestPodForNode1("p6", func(pod *v1.Pod) {
					pod.Annotations = test.GetMirrorPodAnnotation()
				}),
				buildTestPodForNode1("p7", func(pod *v1.Pod) {
					pod.Namespace = "kube-system"
					priority := utils.SystemCriticalPriority
					pod.Spec.Priority = &priority
				}),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p8", "test", nil),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p9", "test", nil),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p10", "test", nil),
			},
			nodes:                   []*v1.Node{node1, node2},
			expectedEvictedPodCount: 2,
		},
		{
			description: "Pods with the same owner but different images should not be evicted",
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p11", "different-images", func(pod *v1.Pod) {
					pod.Spec.Containers[0].Image = "foo"
				}),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p12", "different-images", func(pod *v1.Pod) {
					pod.Spec.Containers[0].Image = "bar"
				}),
			},
			nodes:                   []*v1.Node{node1, node2},
			expectedEvictedPodCount: 0,
		},
		{
			description: "Pods with multiple containers should not match themselves",
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p13", "different-images", func(pod *v1.Pod) {
					pod.Spec.Containers = append(pod.Spec.Containers, v1.Container{
						Name:  "foo",
						Image: "foo",
					})
				}),
			},
			nodes:                   []*v1.Node{node1, node2},
			expectedEvictedPodCount: 0,
		},
		{
			description: "Pods with matching ownerrefs and at not all matching image should not trigger an eviction",
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p11", "different-images", func(pod *v1.Pod) {
					pod.Spec.Containers[0].Image = "foo"
				}),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p13", "different-images", func(pod *v1.Pod) {
					pod.Spec.Containers = append(pod.Spec.Containers, v1.Container{
						Name:  "foo",
						Image: "foo",
					})
				}),
			},
			nodes:                   []*v1.Node{node1, node2},
			expectedEvictedPodCount: 0,
		},
		{
			description: "Three pods in the `dev` Namespace, bound to same ReplicaSet. Only node available has a taint, and nodeFit set to true. 0 should be evicted.",
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p1", "dev", nil),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p2", "dev", nil),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p3", "dev", nil),
			},
			nodes:                   []*v1.Node{node1, node3},
			expectedEvictedPodCount: 0,
			nodefit:                 true,
		},
		{
			description: "Three pods in the `node-fit` Namespace, bound to same ReplicaSet, all with a nodeSelector. Only node available has an incorrect node label, and nodeFit set to true. 0 should be evicted.",
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p15", "node-fit", func(pod *v1.Pod) {
					pod.Spec.NodeSelector = map[string]string{
						"datacenter": "west",
					}
				}),
				p16,
				p17,
			},
			nodes:                   []*v1.Node{node1, node4},
			expectedEvictedPodCount: 0,
			nodefit:                 true,
		},
		{
			description: "Three pods in the `node-fit` Namespace, bound to same ReplicaSet. Only node available is not schedulable, and nodeFit set to true. 0 should be evicted.",
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p1", "dev", nil),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p2", "dev", nil),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p3", "dev", nil),
			},
			nodes:                   []*v1.Node{node1, node5},
			expectedEvictedPodCount: 0,
			nodefit:                 true,
		},
		{
			description: "Three pods in the `node-fit` Namespace, bound to same ReplicaSet. Only node available does not have enough CPU, and nodeFit set to true. 0 should be evicted.",
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p1", "dev", nil),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p2", "dev", nil),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p3", "dev", nil),
				p19,
			},
			nodes:                   []*v1.Node{node1, node6},
			expectedEvictedPodCount: 0,
			nodefit:                 true,
		},
		{
			description: "Three pods in the `node-fit` Namespace, bound to same ReplicaSet. Only node available has enough CPU, and nodeFit set to true. 1 should be evicted.",
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p1", "dev", nil),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p2", "dev", nil),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p3", "dev", nil),
				p20,
			},
			nodes:                   []*v1.Node{node1, node6},
			expectedEvictedPodCount: 1,
			nodefit:                 true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.description, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			var objs []runtime.Object
			for _, node := range testCase.nodes {
				objs = append(objs, node)
			}
			for _, pod := range testCase.pods {
				objs = append(objs, pod)
			}
			fakeClient := fake.NewSimpleClientset(objs...)

			handle, podEvictor, err := frameworktesting.InitFrameworkHandle(ctx, fakeClient, nil, defaultevictor.DefaultEvictorArgs{NodeFit: testCase.nodefit}, nil)
			if err != nil {
				t.Fatalf("Unable to initialize a framework handle: %v", err)
			}

			plugin, err := New(ctx, &RemoveDuplicatesArgs{
				ExcludeOwnerKinds: testCase.excludeOwnerKinds,
			},
				handle,
			)
			if err != nil {
				t.Fatalf("Unable to initialize the plugin: %v", err)
			}

			plugin.(frameworktypes.BalancePlugin).Balance(ctx, testCase.nodes)
			actualEvictedPodCount := podEvictor.TotalEvicted()
			if actualEvictedPodCount != testCase.expectedEvictedPodCount {
				t.Errorf("Test %#v failed, Unexpected no of pods evicted: pods evicted: %d, expected: %d", testCase.description, actualEvictedPodCount, testCase.expectedEvictedPodCount)
			}
		})
	}
}

func TestRemoveDuplicatesUniformly(t *testing.T) {
	setRSOwnerRef2 := func(pod *v1.Pod) {
		pod.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
			{Kind: "ReplicaSet", APIVersion: "v1", Name: "replicaset-2"},
		}
	}
	setTwoRSOwnerRef := func(pod *v1.Pod) {
		pod.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
			{Kind: "ReplicaSet", APIVersion: "v1", Name: "replicaset-1"},
			{Kind: "ReplicaSet", APIVersion: "v1", Name: "replicaset-2"},
		}
	}

	setTolerationsK1 := func(pod *v1.Pod) {
		test.SetRSOwnerRef(pod)
		pod.Spec.Tolerations = []v1.Toleration{
			{
				Key:      "k1",
				Value:    "v1",
				Operator: v1.TolerationOpEqual,
				Effect:   v1.TaintEffectNoSchedule,
			},
		}
	}
	setTolerationsK2 := func(pod *v1.Pod) {
		test.SetRSOwnerRef(pod)
		pod.Spec.Tolerations = []v1.Toleration{
			{
				Key:      "k2",
				Value:    "v2",
				Operator: v1.TolerationOpEqual,
				Effect:   v1.TaintEffectNoSchedule,
			},
		}
	}

	setMasterNoScheduleTaint := func(node *v1.Node) {
		node.Spec.Taints = []v1.Taint{
			{
				Effect: v1.TaintEffectNoSchedule,
				Key:    "node-role.kubernetes.io/control-plane",
			},
		}
	}

	setMasterNoScheduleLabel := func(node *v1.Node) {
		if node.ObjectMeta.Labels == nil {
			node.ObjectMeta.Labels = map[string]string{}
		}
		node.ObjectMeta.Labels["node-role.kubernetes.io/control-plane"] = ""
	}

	setWorkerLabel := func(node *v1.Node) {
		if node.ObjectMeta.Labels == nil {
			node.ObjectMeta.Labels = map[string]string{}
		}
		node.ObjectMeta.Labels["node-role.kubernetes.io/worker"] = "k1"
		node.ObjectMeta.Labels["node-role.kubernetes.io/worker"] = "k2"
	}

	setNotMasterNodeSelectorK1 := func(pod *v1.Pod) {
		test.SetRSOwnerRef(pod)
		pod.Spec.Affinity = &v1.Affinity{
			NodeAffinity: &v1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
					NodeSelectorTerms: []v1.NodeSelectorTerm{
						{
							MatchExpressions: []v1.NodeSelectorRequirement{
								{
									Key:      "node-role.kubernetes.io/control-plane",
									Operator: v1.NodeSelectorOpDoesNotExist,
								},
								{
									Key:      "k1",
									Operator: v1.NodeSelectorOpDoesNotExist,
								},
							},
						},
					},
				},
			},
		}
	}

	setNotMasterNodeSelectorK2 := func(pod *v1.Pod) {
		test.SetRSOwnerRef(pod)
		pod.Spec.Affinity = &v1.Affinity{
			NodeAffinity: &v1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
					NodeSelectorTerms: []v1.NodeSelectorTerm{
						{
							MatchExpressions: []v1.NodeSelectorRequirement{
								{
									Key:      "node-role.kubernetes.io/control-plane",
									Operator: v1.NodeSelectorOpDoesNotExist,
								},
								{
									Key:      "k2",
									Operator: v1.NodeSelectorOpDoesNotExist,
								},
							},
						},
					},
				},
			},
		}
	}

	setWorkerLabelSelectorK1 := func(pod *v1.Pod) {
		test.SetRSOwnerRef(pod)
		if pod.Spec.NodeSelector == nil {
			pod.Spec.NodeSelector = map[string]string{}
		}
		pod.Spec.NodeSelector["node-role.kubernetes.io/worker"] = "k1"
	}

	setWorkerLabelSelectorK2 := func(pod *v1.Pod) {
		test.SetRSOwnerRef(pod)
		if pod.Spec.NodeSelector == nil {
			pod.Spec.NodeSelector = map[string]string{}
		}
		pod.Spec.NodeSelector["node-role.kubernetes.io/worker"] = "k2"
	}

	testCases := []struct {
		description             string
		pods                    []*v1.Pod
		nodes                   []*v1.Node
		expectedEvictedPodCount uint
	}{
		{
			description: "Evict pods uniformly",
			pods: []*v1.Pod{
				// (5,3,1) -> (3,3,3) -> 2 evictions
				test.BuildTestPod("p1", 100, 0, "n1", test.SetRSOwnerRef),
				test.BuildTestPod("p2", 100, 0, "n1", test.SetRSOwnerRef),
				test.BuildTestPod("p3", 100, 0, "n1", test.SetRSOwnerRef),
				test.BuildTestPod("p4", 100, 0, "n1", test.SetRSOwnerRef),
				test.BuildTestPod("p5", 100, 0, "n1", test.SetRSOwnerRef),
				test.BuildTestPod("p6", 100, 0, "n2", test.SetRSOwnerRef),
				test.BuildTestPod("p7", 100, 0, "n2", test.SetRSOwnerRef),
				test.BuildTestPod("p8", 100, 0, "n2", test.SetRSOwnerRef),
				test.BuildTestPod("p9", 100, 0, "n3", test.SetRSOwnerRef),
			},
			expectedEvictedPodCount: 2,
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 2000, 3000, 10, nil),
				test.BuildTestNode("n2", 2000, 3000, 10, nil),
				test.BuildTestNode("n3", 2000, 3000, 10, nil),
			},
		},
		{
			description: "Evict pods uniformly with one node left out",
			pods: []*v1.Pod{
				// (5,3,1) -> (4,4,1) -> 1 eviction
				test.BuildTestPod("p1", 100, 0, "n1", test.SetRSOwnerRef),
				test.BuildTestPod("p2", 100, 0, "n1", test.SetRSOwnerRef),
				test.BuildTestPod("p3", 100, 0, "n1", test.SetRSOwnerRef),
				test.BuildTestPod("p4", 100, 0, "n1", test.SetRSOwnerRef),
				test.BuildTestPod("p5", 100, 0, "n1", test.SetRSOwnerRef),
				test.BuildTestPod("p6", 100, 0, "n2", test.SetRSOwnerRef),
				test.BuildTestPod("p7", 100, 0, "n2", test.SetRSOwnerRef),
				test.BuildTestPod("p8", 100, 0, "n2", test.SetRSOwnerRef),
				test.BuildTestPod("p9", 100, 0, "n3", test.SetRSOwnerRef),
			},
			expectedEvictedPodCount: 1,
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 2000, 3000, 10, nil),
				test.BuildTestNode("n2", 2000, 3000, 10, nil),
			},
		},
		{
			description: "Evict pods uniformly with two replica sets",
			pods: []*v1.Pod{
				// (5,3,1) -> (3,3,3) -> 2 evictions
				test.BuildTestPod("p11", 100, 0, "n1", setTwoRSOwnerRef),
				test.BuildTestPod("p12", 100, 0, "n1", setTwoRSOwnerRef),
				test.BuildTestPod("p13", 100, 0, "n1", setTwoRSOwnerRef),
				test.BuildTestPod("p14", 100, 0, "n1", setTwoRSOwnerRef),
				test.BuildTestPod("p15", 100, 0, "n1", setTwoRSOwnerRef),
				test.BuildTestPod("p16", 100, 0, "n2", setTwoRSOwnerRef),
				test.BuildTestPod("p17", 100, 0, "n2", setTwoRSOwnerRef),
				test.BuildTestPod("p18", 100, 0, "n2", setTwoRSOwnerRef),
				test.BuildTestPod("p19", 100, 0, "n3", setTwoRSOwnerRef),
			},
			expectedEvictedPodCount: 4,
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 2000, 3000, 10, nil),
				test.BuildTestNode("n2", 2000, 3000, 10, nil),
				test.BuildTestNode("n3", 2000, 3000, 10, nil),
			},
		},
		{
			description: "Evict pods uniformly with two owner references",
			pods: []*v1.Pod{
				// (5,3,1) -> (3,3,3) -> 2 evictions
				test.BuildTestPod("p11", 100, 0, "n1", test.SetRSOwnerRef),
				test.BuildTestPod("p12", 100, 0, "n1", test.SetRSOwnerRef),
				test.BuildTestPod("p13", 100, 0, "n1", test.SetRSOwnerRef),
				test.BuildTestPod("p14", 100, 0, "n1", test.SetRSOwnerRef),
				test.BuildTestPod("p15", 100, 0, "n1", test.SetRSOwnerRef),
				test.BuildTestPod("p16", 100, 0, "n2", test.SetRSOwnerRef),
				test.BuildTestPod("p17", 100, 0, "n2", test.SetRSOwnerRef),
				test.BuildTestPod("p18", 100, 0, "n2", test.SetRSOwnerRef),
				test.BuildTestPod("p19", 100, 0, "n3", test.SetRSOwnerRef),
				// (1,3,5) -> (3,3,3) -> 2 evictions
				test.BuildTestPod("p21", 100, 0, "n1", setRSOwnerRef2),
				test.BuildTestPod("p22", 100, 0, "n2", setRSOwnerRef2),
				test.BuildTestPod("p23", 100, 0, "n2", setRSOwnerRef2),
				test.BuildTestPod("p24", 100, 0, "n2", setRSOwnerRef2),
				test.BuildTestPod("p25", 100, 0, "n3", setRSOwnerRef2),
				test.BuildTestPod("p26", 100, 0, "n3", setRSOwnerRef2),
				test.BuildTestPod("p27", 100, 0, "n3", setRSOwnerRef2),
				test.BuildTestPod("p28", 100, 0, "n3", setRSOwnerRef2),
				test.BuildTestPod("p29", 100, 0, "n3", setRSOwnerRef2),
			},
			expectedEvictedPodCount: 4,
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 2000, 3000, 10, nil),
				test.BuildTestNode("n2", 2000, 3000, 10, nil),
				test.BuildTestNode("n3", 2000, 3000, 10, nil),
			},
		},
		{
			description: "Evict pods with number of pods less than nodes",
			pods: []*v1.Pod{
				// (2,0,0) -> (1,1,0) -> 1 eviction
				test.BuildTestPod("p1", 100, 0, "n1", test.SetRSOwnerRef),
				test.BuildTestPod("p2", 100, 0, "n1", test.SetRSOwnerRef),
			},
			expectedEvictedPodCount: 1,
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 2000, 3000, 10, nil),
				test.BuildTestNode("n2", 2000, 3000, 10, nil),
				test.BuildTestNode("n3", 2000, 3000, 10, nil),
			},
		},
		{
			description: "Evict pods with number of pods less than nodes, but ignore different pods with the same ownerref",
			pods: []*v1.Pod{
				// (1, 0, 0) for "bar","baz" images -> no eviction, even with a matching ownerKey
				// (2, 0, 0) for "foo" image -> (1,1,0) - 1 eviction
				// In this case the only "real" duplicates are p1 and p4, so one of those should be evicted
				buildTestPodWithImage("p1", "foo"),
				buildTestPodWithImage("p2", "bar"),
				buildTestPodWithImage("p3", "baz"),
				buildTestPodWithImage("p4", "foo"),
			},
			expectedEvictedPodCount: 1,
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 2000, 3000, 10, nil),
				test.BuildTestNode("n2", 2000, 3000, 10, nil),
				test.BuildTestNode("n3", 2000, 3000, 10, nil),
			},
		},
		{
			description: "Evict pods with a single pod with three nodes",
			pods: []*v1.Pod{
				// (2,0,0) -> (1,1,0) -> 1 eviction
				test.BuildTestPod("p1", 100, 0, "n1", test.SetRSOwnerRef),
			},
			expectedEvictedPodCount: 0,
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 2000, 3000, 10, nil),
				test.BuildTestNode("n2", 2000, 3000, 10, nil),
				test.BuildTestNode("n3", 2000, 3000, 10, nil),
			},
		},
		{
			description: "Evict pods uniformly respecting taints",
			pods: []*v1.Pod{
				// (5,3,1,0,0,0) -> (3,3,3,0,0,0) -> 2 evictions
				test.BuildTestPod("p1", 100, 0, "worker1", setTolerationsK1),
				test.BuildTestPod("p2", 100, 0, "worker1", setTolerationsK2),
				test.BuildTestPod("p3", 100, 0, "worker1", setTolerationsK1),
				test.BuildTestPod("p4", 100, 0, "worker1", setTolerationsK2),
				test.BuildTestPod("p5", 100, 0, "worker1", setTolerationsK1),
				test.BuildTestPod("p6", 100, 0, "worker2", setTolerationsK2),
				test.BuildTestPod("p7", 100, 0, "worker2", setTolerationsK1),
				test.BuildTestPod("p8", 100, 0, "worker2", setTolerationsK2),
				test.BuildTestPod("p9", 100, 0, "worker3", setTolerationsK1),
			},
			expectedEvictedPodCount: 2,
			nodes: []*v1.Node{
				test.BuildTestNode("worker1", 2000, 3000, 10, nil),
				test.BuildTestNode("worker2", 2000, 3000, 10, nil),
				test.BuildTestNode("worker3", 2000, 3000, 10, nil),
				test.BuildTestNode("master1", 2000, 3000, 10, setMasterNoScheduleTaint),
				test.BuildTestNode("master2", 2000, 3000, 10, setMasterNoScheduleTaint),
				test.BuildTestNode("master3", 2000, 3000, 10, setMasterNoScheduleTaint),
			},
		},
		{
			description: "Evict pods uniformly respecting RequiredDuringSchedulingIgnoredDuringExecution node affinity",
			pods: []*v1.Pod{
				// (5,3,1,0,0,0) -> (3,3,3,0,0,0) -> 2 evictions
				test.BuildTestPod("p1", 100, 0, "worker1", setNotMasterNodeSelectorK1),
				test.BuildTestPod("p2", 100, 0, "worker1", setNotMasterNodeSelectorK2),
				test.BuildTestPod("p3", 100, 0, "worker1", setNotMasterNodeSelectorK1),
				test.BuildTestPod("p4", 100, 0, "worker1", setNotMasterNodeSelectorK2),
				test.BuildTestPod("p5", 100, 0, "worker1", setNotMasterNodeSelectorK1),
				test.BuildTestPod("p6", 100, 0, "worker2", setNotMasterNodeSelectorK2),
				test.BuildTestPod("p7", 100, 0, "worker2", setNotMasterNodeSelectorK1),
				test.BuildTestPod("p8", 100, 0, "worker2", setNotMasterNodeSelectorK2),
				test.BuildTestPod("p9", 100, 0, "worker3", setNotMasterNodeSelectorK1),
			},
			expectedEvictedPodCount: 2,
			nodes: []*v1.Node{
				test.BuildTestNode("worker1", 2000, 3000, 10, nil),
				test.BuildTestNode("worker2", 2000, 3000, 10, nil),
				test.BuildTestNode("worker3", 2000, 3000, 10, nil),
				test.BuildTestNode("master1", 2000, 3000, 10, setMasterNoScheduleLabel),
				test.BuildTestNode("master2", 2000, 3000, 10, setMasterNoScheduleLabel),
				test.BuildTestNode("master3", 2000, 3000, 10, setMasterNoScheduleLabel),
			},
		},
		{
			description: "Evict pods uniformly respecting node selector",
			pods: []*v1.Pod{
				// (5,3,1,0,0,0) -> (3,3,3,0,0,0) -> 2 evictions
				test.BuildTestPod("p1", 100, 0, "worker1", setWorkerLabelSelectorK1),
				test.BuildTestPod("p2", 100, 0, "worker1", setWorkerLabelSelectorK2),
				test.BuildTestPod("p3", 100, 0, "worker1", setWorkerLabelSelectorK1),
				test.BuildTestPod("p4", 100, 0, "worker1", setWorkerLabelSelectorK2),
				test.BuildTestPod("p5", 100, 0, "worker1", setWorkerLabelSelectorK1),
				test.BuildTestPod("p6", 100, 0, "worker2", setWorkerLabelSelectorK2),
				test.BuildTestPod("p7", 100, 0, "worker2", setWorkerLabelSelectorK1),
				test.BuildTestPod("p8", 100, 0, "worker2", setWorkerLabelSelectorK2),
				test.BuildTestPod("p9", 100, 0, "worker3", setWorkerLabelSelectorK1),
			},
			expectedEvictedPodCount: 2,
			nodes: []*v1.Node{
				test.BuildTestNode("worker1", 2000, 3000, 10, setWorkerLabel),
				test.BuildTestNode("worker2", 2000, 3000, 10, setWorkerLabel),
				test.BuildTestNode("worker3", 2000, 3000, 10, setWorkerLabel),
				test.BuildTestNode("master1", 2000, 3000, 10, nil),
				test.BuildTestNode("master2", 2000, 3000, 10, nil),
				test.BuildTestNode("master3", 2000, 3000, 10, nil),
			},
		},
		{
			description: "Evict pods uniformly respecting node selector with zero target nodes",
			pods: []*v1.Pod{
				// (5,3,1,0,0,0) -> (3,3,3,0,0,0) -> 2 evictions
				test.BuildTestPod("p1", 100, 0, "worker1", setWorkerLabelSelectorK1),
				test.BuildTestPod("p2", 100, 0, "worker1", setWorkerLabelSelectorK2),
				test.BuildTestPod("p3", 100, 0, "worker1", setWorkerLabelSelectorK1),
				test.BuildTestPod("p4", 100, 0, "worker1", setWorkerLabelSelectorK2),
				test.BuildTestPod("p5", 100, 0, "worker1", setWorkerLabelSelectorK1),
				test.BuildTestPod("p6", 100, 0, "worker2", setWorkerLabelSelectorK2),
				test.BuildTestPod("p7", 100, 0, "worker2", setWorkerLabelSelectorK1),
				test.BuildTestPod("p8", 100, 0, "worker2", setWorkerLabelSelectorK2),
				test.BuildTestPod("p9", 100, 0, "worker3", setWorkerLabelSelectorK1),
			},
			expectedEvictedPodCount: 0,
			nodes: []*v1.Node{
				test.BuildTestNode("worker1", 2000, 3000, 10, nil),
				test.BuildTestNode("worker2", 2000, 3000, 10, nil),
				test.BuildTestNode("worker3", 2000, 3000, 10, nil),
				test.BuildTestNode("master1", 2000, 3000, 10, nil),
				test.BuildTestNode("master2", 2000, 3000, 10, nil),
				test.BuildTestNode("master3", 2000, 3000, 10, nil),
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.description, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			var objs []runtime.Object
			for _, node := range testCase.nodes {
				objs = append(objs, node)
			}
			for _, pod := range testCase.pods {
				objs = append(objs, pod)
			}
			fakeClient := fake.NewSimpleClientset(objs...)

			handle, podEvictor, err := frameworktesting.InitFrameworkHandle(ctx, fakeClient, nil, defaultevictor.DefaultEvictorArgs{}, nil)
			if err != nil {
				t.Fatalf("Unable to initialize a framework handle: %v", err)
			}

			plugin, err := New(ctx, &RemoveDuplicatesArgs{},
				handle,
			)
			if err != nil {
				t.Fatalf("Unable to initialize the plugin: %v", err)
			}

			plugin.(frameworktypes.BalancePlugin).Balance(ctx, testCase.nodes)
			actualEvictedPodCount := podEvictor.TotalEvicted()
			if actualEvictedPodCount != testCase.expectedEvictedPodCount {
				t.Errorf("Test %#v failed, Unexpected no of pods evicted: pods evicted: %d, expected: %d", testCase.description, actualEvictedPodCount, testCase.expectedEvictedPodCount)
			}
		})
	}
}
