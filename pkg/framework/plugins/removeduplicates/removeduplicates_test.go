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

const (
	nodeName1 = "n1"
	nodeName2 = "n2"
	nodeName3 = "n3"
	nodeName4 = "n4"
	nodeName5 = "n5"
	nodeName6 = "n6"
)

func buildTestNode(nodeName string, apply func(*v1.Node)) *v1.Node {
	return test.BuildTestNode(nodeName, 2000, 3000, 10, apply)
}

func buildTestPodForNode(name, nodeName string, apply func(*v1.Pod)) *v1.Pod {
	return test.BuildTestPod(name, 100, 0, nodeName, apply)
}

func buildTestPodWithImage(podName, image string) *v1.Pod {
	pod := buildTestPodForNode(podName, nodeName1, test.SetRSOwnerRef)
	pod.Spec.Containers = append(pod.Spec.Containers, v1.Container{
		Name:  image,
		Image: image,
	})
	return pod
}

func buildTestPodWithRSOwnerRefForNode1(name string, apply func(*v1.Pod)) *v1.Pod {
	return buildTestPodForNode(name, nodeName1, func(pod *v1.Pod) {
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
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
				buildTestNode(nodeName2, nil),
			},
			expectedEvictedPodCount: 1,
		},
		{
			description: "Three pods in the `dev` Namespace, bound to same ReplicaSet, but ReplicaSet kind is excluded. 0 should be evicted.",
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p1", "dev", nil),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p2", "dev", nil),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p3", "dev", nil),
			},
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
				buildTestNode(nodeName2, nil),
			},
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
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
				buildTestNode(nodeName2, nil),
			},
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
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
				buildTestNode(nodeName2, nil),
			},
			expectedEvictedPodCount: 2,
		},
		{
			description: "Pods are: part of DaemonSet, with local storage, mirror pod annotation, critical pod annotation - none should be evicted.",
			pods: []*v1.Pod{
				buildTestPodForNode("p4", nodeName1, func(pod *v1.Pod) {
					test.SetDSOwnerRef(pod)
				}),
				buildTestPodForNode("p5", nodeName1, func(pod *v1.Pod) {
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
				buildTestPodForNode("p6", nodeName1, func(pod *v1.Pod) {
					pod.Annotations = test.GetMirrorPodAnnotation()
				}),
				buildTestPodForNode("p7", nodeName1, func(pod *v1.Pod) {
					pod.Namespace = "kube-system"
					test.SetPodPriority(pod, utils.SystemCriticalPriority)
				}),
			},
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
				buildTestNode(nodeName2, nil),
			},
			expectedEvictedPodCount: 0,
		},
		{
			description: "Test all Pods: 4 should be evicted.",
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p1", "dev", nil),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p2", "dev", nil),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p3", "dev", nil),
				buildTestPodForNode("p4", nodeName1, func(pod *v1.Pod) {
					test.SetDSOwnerRef(pod)
				}),
				buildTestPodForNode("p5", nodeName1, func(pod *v1.Pod) {
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
				buildTestPodForNode("p6", nodeName1, func(pod *v1.Pod) {
					pod.Annotations = test.GetMirrorPodAnnotation()
				}),
				buildTestPodForNode("p7", nodeName1, func(pod *v1.Pod) {
					pod.Namespace = "kube-system"
					test.SetPodPriority(pod, utils.SystemCriticalPriority)
				}),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p8", "test", nil),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p9", "test", nil),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p10", "test", nil),
			},
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
				buildTestNode(nodeName2, nil),
			},
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
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
				buildTestNode(nodeName2, nil),
			},
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
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
				buildTestNode(nodeName2, nil),
			},
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
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
				buildTestNode(nodeName2, nil),
			},
			expectedEvictedPodCount: 0,
		},
		{
			description: "Three pods in the `dev` Namespace, bound to same ReplicaSet. Only node available has a taint, and nodeFit set to true. 0 should be evicted.",
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p1", "dev", nil),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p2", "dev", nil),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p3", "dev", nil),
			},
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
				buildTestNode(nodeName3, func(node *v1.Node) {
					node.Spec.Taints = []v1.Taint{
						{
							Key:    "hardware",
							Value:  "gpu",
							Effect: v1.TaintEffectNoSchedule,
						},
					}
				}),
			},
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
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("NOT1", "node-fit", func(pod *v1.Pod) {
					pod.Spec.NodeSelector = map[string]string{
						"datacenter": "west",
					}
				}),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("NOT2", "node-fit", func(pod *v1.Pod) {
					pod.Spec.NodeSelector = map[string]string{
						"datacenter": "west",
					}
				}),
			},
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
				buildTestNode(nodeName4, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						"datacenter": "east",
					}
				}),
			},
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
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
				buildTestNode(nodeName5, func(node *v1.Node) {
					node.Spec = v1.NodeSpec{
						Unschedulable: true,
					}
				}),
			},
			expectedEvictedPodCount: 0,
			nodefit:                 true,
		},
		{
			description: "Three pods in the `node-fit` Namespace, bound to same ReplicaSet. Only node available does not have enough CPU, and nodeFit set to true. 0 should be evicted.",
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p1", "dev", nil),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p2", "dev", nil),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p3", "dev", nil),
				test.BuildTestPod("CPU-eater", 150, 150, nodeName6, func(pod *v1.Pod) {
					pod.Namespace = "test"
				}),
			},
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
				test.BuildTestNode(nodeName6, 200, 200, 10, nil),
			},
			expectedEvictedPodCount: 0,
			nodefit:                 true,
		},
		{
			description: "Three pods in the `node-fit` Namespace, bound to same ReplicaSet. Only node available has enough CPU, and nodeFit set to true. 1 should be evicted.",
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p1", "dev", nil),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p2", "dev", nil),
				buildTestPodWithRSOwnerRefWithNamespaceForNode1("p3", "dev", nil),
				test.BuildTestPod("CPU-saver", 100, 150, nodeName6, func(pod *v1.Pod) {
					pod.Namespace = "test"
				}),
			},
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
				test.BuildTestNode(nodeName6, 200, 200, 10, nil),
			},
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

	setNoScheduleTolerations := func(key, value string) func(*v1.Pod) {
		return func(pod *v1.Pod) {
			test.SetRSOwnerRef(pod)
			pod.Spec.Tolerations = []v1.Toleration{
				{
					Key:      key,
					Value:    value,
					Operator: v1.TolerationOpEqual,
					Effect:   v1.TaintEffectNoSchedule,
				},
			}
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

	setNotMasterNodeSelector := func(key string) func(*v1.Pod) {
		return func(pod *v1.Pod) {
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
										Key:      key,
										Operator: v1.NodeSelectorOpDoesNotExist,
									},
								},
							},
						},
					},
				},
			}
		}
	}

	setWorkerLabelSelector := func(value string) func(*v1.Pod) {
		return func(pod *v1.Pod) {
			test.SetRSOwnerRef(pod)
			if pod.Spec.NodeSelector == nil {
				pod.Spec.NodeSelector = map[string]string{}
			}
			pod.Spec.NodeSelector["node-role.kubernetes.io/worker"] = value
		}
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
				buildTestPodForNode("p1", "n1", test.SetRSOwnerRef),
				buildTestPodForNode("p2", "n1", test.SetRSOwnerRef),
				buildTestPodForNode("p3", "n1", test.SetRSOwnerRef),
				buildTestPodForNode("p4", "n1", test.SetRSOwnerRef),
				buildTestPodForNode("p5", "n1", test.SetRSOwnerRef),
				buildTestPodForNode("p6", "n2", test.SetRSOwnerRef),
				buildTestPodForNode("p7", "n2", test.SetRSOwnerRef),
				buildTestPodForNode("p8", "n2", test.SetRSOwnerRef),
				buildTestPodForNode("p9", "n3", test.SetRSOwnerRef),
			},
			expectedEvictedPodCount: 2,
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
				buildTestNode(nodeName2, nil),
				buildTestNode(nodeName3, nil),
			},
		},
		{
			description: "Evict pods uniformly with one node left out",
			pods: []*v1.Pod{
				// (5,3,1) -> (4,4,1) -> 1 eviction
				buildTestPodForNode("p1", "n1", test.SetRSOwnerRef),
				buildTestPodForNode("p2", "n1", test.SetRSOwnerRef),
				buildTestPodForNode("p3", "n1", test.SetRSOwnerRef),
				buildTestPodForNode("p4", "n1", test.SetRSOwnerRef),
				buildTestPodForNode("p5", "n1", test.SetRSOwnerRef),
				buildTestPodForNode("p6", "n2", test.SetRSOwnerRef),
				buildTestPodForNode("p7", "n2", test.SetRSOwnerRef),
				buildTestPodForNode("p8", "n2", test.SetRSOwnerRef),
				buildTestPodForNode("p9", "n3", test.SetRSOwnerRef),
			},
			expectedEvictedPodCount: 1,
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
				buildTestNode(nodeName2, nil),
			},
		},
		{
			description: "Evict pods uniformly with two replica sets",
			pods: []*v1.Pod{
				// (5,3,1) -> (3,3,3) -> 2 evictions
				buildTestPodForNode("p11", "n1", setTwoRSOwnerRef),
				buildTestPodForNode("p12", "n1", setTwoRSOwnerRef),
				buildTestPodForNode("p13", "n1", setTwoRSOwnerRef),
				buildTestPodForNode("p14", "n1", setTwoRSOwnerRef),
				buildTestPodForNode("p15", "n1", setTwoRSOwnerRef),
				buildTestPodForNode("p16", "n2", setTwoRSOwnerRef),
				buildTestPodForNode("p17", "n2", setTwoRSOwnerRef),
				buildTestPodForNode("p18", "n2", setTwoRSOwnerRef),
				buildTestPodForNode("p19", "n3", setTwoRSOwnerRef),
			},
			expectedEvictedPodCount: 4,
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
				buildTestNode(nodeName2, nil),
				buildTestNode(nodeName3, nil),
			},
		},
		{
			description: "Evict pods uniformly with two owner references",
			pods: []*v1.Pod{
				// (5,3,1) -> (3,3,3) -> 2 evictions
				buildTestPodForNode("p11", "n1", test.SetRSOwnerRef),
				buildTestPodForNode("p12", "n1", test.SetRSOwnerRef),
				buildTestPodForNode("p13", "n1", test.SetRSOwnerRef),
				buildTestPodForNode("p14", "n1", test.SetRSOwnerRef),
				buildTestPodForNode("p15", "n1", test.SetRSOwnerRef),
				buildTestPodForNode("p16", "n2", test.SetRSOwnerRef),
				buildTestPodForNode("p17", "n2", test.SetRSOwnerRef),
				buildTestPodForNode("p18", "n2", test.SetRSOwnerRef),
				buildTestPodForNode("p19", "n3", test.SetRSOwnerRef),
				// (1,3,5) -> (3,3,3) -> 2 evictions
				buildTestPodForNode("p21", "n1", setRSOwnerRef2),
				buildTestPodForNode("p22", "n2", setRSOwnerRef2),
				buildTestPodForNode("p23", "n2", setRSOwnerRef2),
				buildTestPodForNode("p24", "n2", setRSOwnerRef2),
				buildTestPodForNode("p25", "n3", setRSOwnerRef2),
				buildTestPodForNode("p26", "n3", setRSOwnerRef2),
				buildTestPodForNode("p27", "n3", setRSOwnerRef2),
				buildTestPodForNode("p28", "n3", setRSOwnerRef2),
				buildTestPodForNode("p29", "n3", setRSOwnerRef2),
			},
			expectedEvictedPodCount: 4,
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
				buildTestNode(nodeName2, nil),
				buildTestNode(nodeName3, nil),
			},
		},
		{
			description: "Evict pods with number of pods less than nodes",
			pods: []*v1.Pod{
				// (2,0,0) -> (1,1,0) -> 1 eviction
				buildTestPodForNode("p1", "n1", test.SetRSOwnerRef),
				buildTestPodForNode("p2", "n1", test.SetRSOwnerRef),
			},
			expectedEvictedPodCount: 1,
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
				buildTestNode(nodeName2, nil),
				buildTestNode(nodeName3, nil),
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
				buildTestNode(nodeName1, nil),
				buildTestNode(nodeName2, nil),
				buildTestNode(nodeName3, nil),
			},
		},
		{
			description: "Evict pods with a single pod with three nodes",
			pods: []*v1.Pod{
				// (2,0,0) -> (1,1,0) -> 1 eviction
				buildTestPodForNode("p1", "n1", test.SetRSOwnerRef),
			},
			expectedEvictedPodCount: 0,
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
				buildTestNode(nodeName2, nil),
				buildTestNode(nodeName3, nil),
			},
		},
		{
			description: "Evict pods uniformly respecting taints",
			pods: []*v1.Pod{
				// (5,3,1,0,0,0) -> (3,3,3,0,0,0) -> 2 evictions
				buildTestPodForNode("p1", "worker1", setNoScheduleTolerations("k1", "v1")),
				buildTestPodForNode("p2", "worker1", setNoScheduleTolerations("k2", "v2")),
				buildTestPodForNode("p3", "worker1", setNoScheduleTolerations("k1", "v1")),
				buildTestPodForNode("p4", "worker1", setNoScheduleTolerations("k2", "v2")),
				buildTestPodForNode("p5", "worker1", setNoScheduleTolerations("k1", "v1")),
				buildTestPodForNode("p6", "worker2", setNoScheduleTolerations("k2", "v2")),
				buildTestPodForNode("p7", "worker2", setNoScheduleTolerations("k1", "v1")),
				buildTestPodForNode("p8", "worker2", setNoScheduleTolerations("k2", "v2")),
				buildTestPodForNode("p9", "worker3", setNoScheduleTolerations("k1", "v1")),
			},
			expectedEvictedPodCount: 2,
			nodes: []*v1.Node{
				buildTestNode("worker1", nil),
				buildTestNode("worker2", nil),
				buildTestNode("worker3", nil),
				buildTestNode("master1", setMasterNoScheduleTaint),
				buildTestNode("master2", setMasterNoScheduleTaint),
				buildTestNode("master3", setMasterNoScheduleTaint),
			},
		},
		{
			description: "Evict pods uniformly respecting RequiredDuringSchedulingIgnoredDuringExecution node affinity",
			pods: []*v1.Pod{
				// (5,3,1,0,0,0) -> (3,3,3,0,0,0) -> 2 evictions
				buildTestPodForNode("p1", "worker1", setNotMasterNodeSelector("k1")),
				buildTestPodForNode("p2", "worker1", setNotMasterNodeSelector("k2")),
				buildTestPodForNode("p3", "worker1", setNotMasterNodeSelector("k1")),
				buildTestPodForNode("p4", "worker1", setNotMasterNodeSelector("k2")),
				buildTestPodForNode("p5", "worker1", setNotMasterNodeSelector("k1")),
				buildTestPodForNode("p6", "worker2", setNotMasterNodeSelector("k2")),
				buildTestPodForNode("p7", "worker2", setNotMasterNodeSelector("k1")),
				buildTestPodForNode("p8", "worker2", setNotMasterNodeSelector("k2")),
				buildTestPodForNode("p9", "worker3", setNotMasterNodeSelector("k1")),
			},
			expectedEvictedPodCount: 2,
			nodes: []*v1.Node{
				buildTestNode("worker1", nil),
				buildTestNode("worker2", nil),
				buildTestNode("worker3", nil),
				buildTestNode("master1", setMasterNoScheduleLabel),
				buildTestNode("master2", setMasterNoScheduleLabel),
				buildTestNode("master3", setMasterNoScheduleLabel),
			},
		},
		{
			description: "Evict pods uniformly respecting node selector",
			pods: []*v1.Pod{
				// (5,3,1,0,0,0) -> (3,3,3,0,0,0) -> 2 evictions
				buildTestPodForNode("p1", "worker1", setWorkerLabelSelector("k1")),
				buildTestPodForNode("p2", "worker1", setWorkerLabelSelector("k2")),
				buildTestPodForNode("p3", "worker1", setWorkerLabelSelector("k1")),
				buildTestPodForNode("p4", "worker1", setWorkerLabelSelector("k2")),
				buildTestPodForNode("p5", "worker1", setWorkerLabelSelector("k1")),
				buildTestPodForNode("p6", "worker2", setWorkerLabelSelector("k2")),
				buildTestPodForNode("p7", "worker2", setWorkerLabelSelector("k1")),
				buildTestPodForNode("p8", "worker2", setWorkerLabelSelector("k2")),
				buildTestPodForNode("p9", "worker3", setWorkerLabelSelector("k1")),
			},
			expectedEvictedPodCount: 2,
			nodes: []*v1.Node{
				buildTestNode("worker1", setWorkerLabel),
				buildTestNode("worker2", setWorkerLabel),
				buildTestNode("worker3", setWorkerLabel),
				buildTestNode("master1", nil),
				buildTestNode("master2", nil),
				buildTestNode("master3", nil),
			},
		},
		{
			description: "Evict pods uniformly respecting node selector with zero target nodes",
			pods: []*v1.Pod{
				// (5,3,1,0,0,0) -> (3,3,3,0,0,0) -> 2 evictions
				buildTestPodForNode("p1", "worker1", setWorkerLabelSelector("k1")),
				buildTestPodForNode("p2", "worker1", setWorkerLabelSelector("k2")),
				buildTestPodForNode("p3", "worker1", setWorkerLabelSelector("k1")),
				buildTestPodForNode("p4", "worker1", setWorkerLabelSelector("k2")),
				buildTestPodForNode("p5", "worker1", setWorkerLabelSelector("k1")),
				buildTestPodForNode("p6", "worker2", setWorkerLabelSelector("k2")),
				buildTestPodForNode("p7", "worker2", setWorkerLabelSelector("k1")),
				buildTestPodForNode("p8", "worker2", setWorkerLabelSelector("k2")),
				buildTestPodForNode("p9", "worker3", setWorkerLabelSelector("k1")),
			},
			expectedEvictedPodCount: 0,
			nodes: []*v1.Node{
				buildTestNode("worker1", nil),
				buildTestNode("worker2", nil),
				buildTestNode("worker3", nil),
				buildTestNode("master1", nil),
				buildTestNode("master2", nil),
				buildTestNode("master3", nil),
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
