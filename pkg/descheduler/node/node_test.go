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

package node

import (
	"context"
	"errors"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/test"
)

func TestReadyNodes(t *testing.T) {
	node1 := test.BuildTestNode("node2", 1000, 2000, 9, nil)
	node2 := test.BuildTestNode("node3", 1000, 2000, 9, nil)
	node2.Status.Conditions = []v1.NodeCondition{{Type: v1.NodeMemoryPressure, Status: v1.ConditionTrue}}
	node3 := test.BuildTestNode("node4", 1000, 2000, 9, nil)
	node3.Status.Conditions = []v1.NodeCondition{{Type: v1.NodeNetworkUnavailable, Status: v1.ConditionTrue}}
	node4 := test.BuildTestNode("node5", 1000, 2000, 9, nil)
	node4.Spec.Unschedulable = true
	node5 := test.BuildTestNode("node6", 1000, 2000, 9, nil)
	node5.Status.Conditions = []v1.NodeCondition{{Type: v1.NodeReady, Status: v1.ConditionFalse}}

	if !IsReady(node1) {
		t.Errorf("Expected %v to be ready", node2.Name)
	}
	if !IsReady(node2) {
		t.Errorf("Expected %v to be ready", node3.Name)
	}
	if !IsReady(node3) {
		t.Errorf("Expected %v to be ready", node4.Name)
	}
	if !IsReady(node4) {
		t.Errorf("Expected %v to be ready", node5.Name)
	}
	if IsReady(node5) {
		t.Errorf("Expected %v to be not ready", node5.Name)
	}
}

func TestReadyNodesWithNodeSelector(t *testing.T) {
	ctx := context.Background()
	node1 := test.BuildTestNode("node1", 1000, 2000, 9, nil)
	node1.Labels = map[string]string{"type": "compute"}
	node2 := test.BuildTestNode("node2", 1000, 2000, 9, nil)
	node2.Labels = map[string]string{"type": "infra"}

	fakeClient := fake.NewSimpleClientset(node1, node2)
	nodeSelector := "type=compute"

	sharedInformerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
	nodeLister := sharedInformerFactory.Core().V1().Nodes().Lister()

	stopChannel := make(chan struct{})
	sharedInformerFactory.Start(stopChannel)
	sharedInformerFactory.WaitForCacheSync(stopChannel)
	defer close(stopChannel)

	nodes, _ := ReadyNodes(ctx, fakeClient, nodeLister, nodeSelector)

	if nodes[0].Name != "node1" {
		t.Errorf("Expected node1, got %s", nodes[0].Name)
	}
}

func TestIsNodeUnschedulable(t *testing.T) {
	tests := []struct {
		description     string
		node            *v1.Node
		IsUnSchedulable bool
	}{
		{
			description: "Node is expected to be schedulable",
			node: &v1.Node{
				Spec: v1.NodeSpec{Unschedulable: false},
			},
			IsUnSchedulable: false,
		},
		{
			description: "Node is not expected to be schedulable because of unschedulable field",
			node: &v1.Node{
				Spec: v1.NodeSpec{Unschedulable: true},
			},
			IsUnSchedulable: true,
		},
	}
	for _, test := range tests {
		actualUnSchedulable := IsNodeUnschedulable(test.node)
		if actualUnSchedulable != test.IsUnSchedulable {
			t.Errorf("Test %#v failed", test.description)
		}
	}
}

func TestPodFitsCurrentNode(t *testing.T) {
	nodeLabelKey := "kubernetes.io/desiredNode"
	nodeLabelValue := "yes"

	tests := []struct {
		description string
		pod         *v1.Pod
		node        *v1.Node
		success     bool
	}{
		{
			description: "Pod with nodeAffinity set, expected to fit the node",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					Affinity: &v1.Affinity{
						NodeAffinity: &v1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
								NodeSelectorTerms: []v1.NodeSelectorTerm{
									{
										MatchExpressions: []v1.NodeSelectorRequirement{
											{
												Key:      nodeLabelKey,
												Operator: "In",
												Values: []string{
													nodeLabelValue,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			node: test.BuildTestNode("node1", 64000, 128*1000*1000*1000, 200, func(node *v1.Node) {
				node.ObjectMeta.Labels = map[string]string{
					nodeLabelKey: nodeLabelValue,
				}

				node.Status.Allocatable[v1.ResourceEphemeralStorage] = *resource.NewQuantity(1000*1000*1000*1000, resource.DecimalSI)
			}),
			success: true,
		},
		{
			description: "Pod with nodeAffinity set, not expected to fit the node",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					Affinity: &v1.Affinity{
						NodeAffinity: &v1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
								NodeSelectorTerms: []v1.NodeSelectorTerm{
									{
										MatchExpressions: []v1.NodeSelectorRequirement{
											{
												Key:      nodeLabelKey,
												Operator: "In",
												Values: []string{
													nodeLabelValue,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			node: test.BuildTestNode("node1", 64000, 128*1000*1000*1000, 200, func(node *v1.Node) {
				node.ObjectMeta.Labels = map[string]string{
					nodeLabelKey: "no",
				}

				node.Status.Allocatable[v1.ResourceEphemeralStorage] = *resource.NewQuantity(1000*1000*1000*1000, resource.DecimalSI)
			}),
			success: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			var objs []runtime.Object
			objs = append(objs, tc.node)
			objs = append(objs, tc.pod)

			fakeClient := fake.NewSimpleClientset(objs...)

			sharedInformerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
			podInformer := sharedInformerFactory.Core().V1().Pods().Informer()

			getPodsAssignedToNode, err := podutil.BuildGetPodsAssignedToNodeFunc(podInformer)
			if err != nil {
				t.Errorf("Build get pods assigned to node function error: %v", err)
			}

			sharedInformerFactory.Start(ctx.Done())
			sharedInformerFactory.WaitForCacheSync(ctx.Done())

			actual := PodFitsCurrentNode(getPodsAssignedToNode, tc.pod, tc.node, false)
			if actual != tc.success {
				t.Errorf("Test %#v failed", tc.description)
			}
		})
	}
}

func TestPodFitsAnyOtherNode(t *testing.T) {
	nodeLabelKey := "kubernetes.io/desiredNode"
	nodeLabelValue := "yes"
	nodeTaintKey := "hardware"
	nodeTaintValue := "gpu"

	// Staging node has no scheduling restrictions, but the pod always starts here and PodFitsAnyOtherNode() doesn't take into account the node the pod is running on.
	nodeNames := []string{"node1", "node2", "stagingNode"}

	tests := []struct {
		description string
		pod         *v1.Pod
		nodes       []*v1.Node
		success     bool
		podsOnNodes []*v1.Pod
	}{
		{
			description: "Pod fits another node matching node affinity",
			pod: test.BuildTestPod("p1", 0, 0, nodeNames[2], func(pod *v1.Pod) {
				pod.Spec.NodeSelector = map[string]string{
					nodeLabelKey: nodeLabelValue,
				}
			}),
			nodes: []*v1.Node{
				test.BuildTestNode(nodeNames[0], 64000, 128*1000*1000*1000, 200, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}

					node.Status.Allocatable[v1.ResourceEphemeralStorage] = *resource.NewQuantity(1000*1000*1000*1000, resource.DecimalSI)
				}),
				test.BuildTestNode(nodeNames[1], 64000, 128*1000*1000*1000, 200, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: "no",
					}

					node.Status.Allocatable[v1.ResourceEphemeralStorage] = *resource.NewQuantity(1000*1000*1000*1000, resource.DecimalSI)
				}),
				test.BuildTestNode(nodeNames[2], 0, 0, 0, nil),
			},
			podsOnNodes: []*v1.Pod{},
			success:     true,
		},
		{
			description: "Pod expected to fit one of the nodes",
			pod: test.BuildTestPod("p1", 0, 0, nodeNames[2], func(pod *v1.Pod) {
				pod.Spec.NodeSelector = map[string]string{
					nodeLabelKey: nodeLabelValue,
				}
			}),
			nodes: []*v1.Node{
				test.BuildTestNode(nodeNames[0], 64000, 128*1000*1000*1000, 200, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: "no",
					}

					node.Status.Allocatable[v1.ResourceEphemeralStorage] = *resource.NewQuantity(1000*1000*1000*1000, resource.DecimalSI)
				}),
				test.BuildTestNode(nodeNames[1], 64000, 128*1000*1000*1000, 200, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}

					node.Status.Allocatable[v1.ResourceEphemeralStorage] = *resource.NewQuantity(1000*1000*1000*1000, resource.DecimalSI)
				}),
				test.BuildTestNode(nodeNames[2], 0, 0, 0, nil),
			},
			podsOnNodes: []*v1.Pod{},
			success:     true,
		},
		{
			description: "Pod expected to fit none of the nodes",
			pod: test.BuildTestPod("p1", 0, 0, nodeNames[2], func(pod *v1.Pod) {
				pod.Spec.NodeSelector = map[string]string{
					nodeLabelKey: nodeLabelValue,
				}
			}),
			nodes: []*v1.Node{
				test.BuildTestNode(nodeNames[0], 64000, 128*1000*1000*1000, 200, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: "unfit1",
					}

					node.Status.Allocatable[v1.ResourceEphemeralStorage] = *resource.NewQuantity(1000*1000*1000*1000, resource.DecimalSI)
				}),
				test.BuildTestNode(nodeNames[1], 64000, 128*1000*1000*1000, 200, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: "unfit2",
					}

					node.Status.Allocatable[v1.ResourceEphemeralStorage] = *resource.NewQuantity(1000*1000*1000*1000, resource.DecimalSI)
				}),
				test.BuildTestNode(nodeNames[2], 0, 0, 0, nil),
			},
			podsOnNodes: []*v1.Pod{},
			success:     false,
		},
		{
			description: "Nodes are unschedulable but labels match, should fail",
			pod: test.BuildTestPod("p1", 0, 0, nodeNames[2], func(pod *v1.Pod) {
				pod.Spec.NodeSelector = map[string]string{
					nodeLabelKey: nodeLabelValue,
				}
			}),
			nodes: []*v1.Node{
				test.BuildTestNode(nodeNames[0], 64000, 128*1000*1000*1000, 200, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}

					node.Status.Allocatable[v1.ResourceEphemeralStorage] = *resource.NewQuantity(1000*1000*1000*1000, resource.DecimalSI)
					node.Spec.Unschedulable = true
				}),
				test.BuildTestNode(nodeNames[1], 64000, 128*1000*1000*1000, 200, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: "no",
					}

					node.Status.Allocatable[v1.ResourceEphemeralStorage] = *resource.NewQuantity(1000*1000*1000*1000, resource.DecimalSI)
				}),
				test.BuildTestNode(nodeNames[2], 0, 0, 0, nil),
			},
			podsOnNodes: []*v1.Pod{},
			success:     false,
		},
		{
			description: "Both nodes are tained, should fail",
			pod: test.BuildTestPod("p1", 2000, 2*1000*1000*1000, nodeNames[2], func(pod *v1.Pod) {
				pod.Spec.NodeSelector = map[string]string{
					nodeLabelKey: nodeLabelValue,
				}
				pod.Spec.Containers[0].Resources.Requests[v1.ResourceEphemeralStorage] = *resource.NewQuantity(10*1000*1000*1000, resource.DecimalSI)
			}),
			nodes: []*v1.Node{
				test.BuildTestNode(nodeNames[0], 64000, 128*1000*1000*1000, 200, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}

					node.Status.Allocatable[v1.ResourceEphemeralStorage] = *resource.NewQuantity(1000*1000*1000*1000, resource.DecimalSI)
					node.Spec.Taints = []v1.Taint{
						{
							Key:    nodeTaintKey,
							Value:  nodeTaintValue,
							Effect: v1.TaintEffectNoSchedule,
						},
					}
				}),
				test.BuildTestNode(nodeNames[1], 64000, 128*1000*1000*1000, 200, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}

					node.Status.Allocatable[v1.ResourceEphemeralStorage] = *resource.NewQuantity(1000*1000*1000*1000, resource.DecimalSI)
					node.Spec.Taints = []v1.Taint{
						{
							Key:    nodeTaintKey,
							Value:  nodeTaintValue,
							Effect: v1.TaintEffectNoSchedule,
						},
					}
				}),
				test.BuildTestNode(nodeNames[2], 0, 0, 0, nil),
			},
			podsOnNodes: []*v1.Pod{},
			success:     false,
		},
		{
			description: "Two nodes matches node selector, one of them is tained, there is a pod on the available node, and requests are low, should pass",
			pod: test.BuildTestPod("p1", 2000, 2*1000*1000*1000, nodeNames[2], func(pod *v1.Pod) {
				pod.Spec.NodeSelector = map[string]string{
					nodeLabelKey: nodeLabelValue,
				}
				pod.Spec.Containers[0].Resources.Requests[v1.ResourceEphemeralStorage] = *resource.NewQuantity(10*1000*1000*1000, resource.DecimalSI)
			}),
			nodes: []*v1.Node{
				test.BuildTestNode(nodeNames[0], 64000, 128*1000*1000*1000, 200, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}

					node.Status.Allocatable[v1.ResourceEphemeralStorage] = *resource.NewQuantity(1000*1000*1000*1000, resource.DecimalSI)
					node.Spec.Taints = []v1.Taint{
						{
							Key:    nodeTaintKey,
							Value:  nodeTaintValue,
							Effect: v1.TaintEffectNoSchedule,
						},
					}
				}),
				test.BuildTestNode(nodeNames[1], 64000, 128*1000*1000*1000, 200, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}

					node.Status.Allocatable[v1.ResourceEphemeralStorage] = *resource.NewQuantity(1000*1000*1000*1000, resource.DecimalSI)
				}),
				test.BuildTestNode(nodeNames[2], 0, 0, 0, nil),
			},
			podsOnNodes: []*v1.Pod{
				test.BuildTestPod("test-pod", 12*1000, 20*1000*1000*1000, nodeNames[1], func(pod *v1.Pod) {
					pod.ObjectMeta = metav1.ObjectMeta{
						Namespace: "test",
						Labels: map[string]string{
							"test": "true",
						},
					}
					pod.Spec.Containers[0].Resources.Requests[v1.ResourceEphemeralStorage] = *resource.NewQuantity(40*1000*1000*1000, resource.DecimalSI)
				}),
			},
			success: true,
		},
		{
			description: "Two nodes matches node selector, one of them is tained, but CPU requests are too big, should fail",
			pod: test.BuildTestPod("p1", 2000, 2*1000*1000*1000, nodeNames[2], func(pod *v1.Pod) {
				pod.Spec.NodeSelector = map[string]string{
					nodeLabelKey: nodeLabelValue,
				}
				pod.Spec.Containers[0].Resources.Requests[v1.ResourceEphemeralStorage] = *resource.NewQuantity(10*1000*1000*1000, resource.DecimalSI)
			}),
			nodes: []*v1.Node{
				test.BuildTestNode(nodeNames[0], 64000, 128*1000*1000*1000, 200, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}

					node.Status.Allocatable[v1.ResourceEphemeralStorage] = *resource.NewQuantity(1000*1000*1000*1000, resource.DecimalSI)
					node.Spec.Taints = []v1.Taint{
						{
							Key:    nodeTaintKey,
							Value:  nodeTaintValue,
							Effect: v1.TaintEffectNoSchedule,
						},
					}
				}),
				// Notice that this node only has 4 cores, the pod already on the node below requests 3 cores, and the pod above requests 2 cores
				test.BuildTestNode(nodeNames[1], 4000, 8*1000*1000*1000, 12, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}

					node.Status.Allocatable[v1.ResourceEphemeralStorage] = *resource.NewQuantity(200*1000*1000*1000, resource.DecimalSI)
				}),
				test.BuildTestNode(nodeNames[2], 0, 0, 0, nil),
			},
			podsOnNodes: []*v1.Pod{
				test.BuildTestPod("3-core-pod", 3000, 4*1000*1000*1000, nodeNames[1], func(pod *v1.Pod) {
					pod.ObjectMeta = metav1.ObjectMeta{
						Namespace: "test",
						Labels: map[string]string{
							"test": "true",
						},
					}
					pod.Spec.Containers[0].Resources.Requests[v1.ResourceEphemeralStorage] = *resource.NewQuantity(10*1000*1000*1000, resource.DecimalSI)
				}),
			},
			success: false,
		},
		{
			description: "Two nodes matches node selector, one of them is tained, but memory requests are too big, should fail",
			pod: test.BuildTestPod("p1", 2000, 5*1000*1000*1000, nodeNames[2], func(pod *v1.Pod) {
				pod.Spec.NodeSelector = map[string]string{
					nodeLabelKey: nodeLabelValue,
				}
				pod.Spec.Containers[0].Resources.Requests[v1.ResourceEphemeralStorage] = *resource.NewQuantity(10*1000*1000*1000, resource.DecimalSI)
			}),
			nodes: []*v1.Node{
				test.BuildTestNode(nodeNames[0], 64000, 128*1000*1000*1000, 200, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}

					node.Status.Allocatable[v1.ResourceEphemeralStorage] = *resource.NewQuantity(1000*1000*1000*1000, resource.DecimalSI)
					node.Spec.Taints = []v1.Taint{
						{
							Key:    nodeTaintKey,
							Value:  nodeTaintValue,
							Effect: v1.TaintEffectNoSchedule,
						},
					}
				}),
				// Notice that this node only has 8GB of memory, the pod already on the node below requests 4GB, and the pod above requests 5GB
				test.BuildTestNode(nodeNames[1], 10*1000, 8*1000*1000*1000, 12, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}

					node.Status.Allocatable[v1.ResourceEphemeralStorage] = *resource.NewQuantity(200*1000*1000*1000, resource.DecimalSI)
				}),
				test.BuildTestNode(nodeNames[2], 0, 0, 0, nil),
			},
			podsOnNodes: []*v1.Pod{
				test.BuildTestPod("4GB-mem-pod", 2000, 4*1000*1000*1000, nodeNames[1], func(pod *v1.Pod) {
					pod.ObjectMeta = metav1.ObjectMeta{
						Namespace: "test",
						Labels: map[string]string{
							"test": "true",
						},
					}
					pod.Spec.Containers[0].Resources.Requests[v1.ResourceEphemeralStorage] = *resource.NewQuantity(10*1000*1000*1000, resource.DecimalSI)
				}),
			},
			success: false,
		},
		{
			description: "Two nodes matches node selector, one of them is tained, but ephemeral storage requests are too big, should fail",
			pod: test.BuildTestPod("p1", 2000, 4*1000*1000*1000, nodeNames[2], func(pod *v1.Pod) {
				pod.Spec.NodeSelector = map[string]string{
					nodeLabelKey: nodeLabelValue,
				}
				pod.Spec.Containers[0].Resources.Requests[v1.ResourceEphemeralStorage] = *resource.NewQuantity(10*1000*1000*1000, resource.DecimalSI)
			}),
			nodes: []*v1.Node{
				test.BuildTestNode(nodeNames[0], 64000, 128*1000*1000*1000, 200, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}

					node.Status.Allocatable[v1.ResourceEphemeralStorage] = *resource.NewQuantity(1000*1000*1000*1000, resource.DecimalSI)
					node.Spec.Taints = []v1.Taint{
						{
							Key:    nodeTaintKey,
							Value:  nodeTaintValue,
							Effect: v1.TaintEffectNoSchedule,
						},
					}
				}),
				// Notice that this node only has 20GB of storage, the pod already on the node below requests 11GB, and the pod above requests 10GB
				test.BuildTestNode(nodeNames[1], 10*1000, 8*1000*1000*1000, 12, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}

					node.Status.Allocatable[v1.ResourceEphemeralStorage] = *resource.NewQuantity(20*1000*1000*1000, resource.DecimalSI)
				}),
				test.BuildTestNode(nodeNames[2], 0, 0, 0, nil),
			},
			podsOnNodes: []*v1.Pod{
				test.BuildTestPod("11GB-storage-pod", 2000, 4*1000*1000*1000, nodeNames[1], func(pod *v1.Pod) {
					pod.ObjectMeta = metav1.ObjectMeta{
						Namespace: "test",
						Labels: map[string]string{
							"test": "true",
						},
					}
					pod.Spec.Containers[0].Resources.Requests[v1.ResourceEphemeralStorage] = *resource.NewQuantity(11*1000*1000*1000, resource.DecimalSI)
				}),
			},
			success: false,
		},
		{
			description: "Two nodes matches node selector, one of them is tained, but custom resource requests are too big, should fail",
			pod: test.BuildTestPod("p1", 2000, 2*1000*1000*1000, nodeNames[2], func(pod *v1.Pod) {
				pod.Spec.NodeSelector = map[string]string{
					nodeLabelKey: nodeLabelValue,
				}
				pod.Spec.Containers[0].Resources.Requests[v1.ResourceEphemeralStorage] = *resource.NewQuantity(10*1000*1000*1000, resource.DecimalSI)
				pod.Spec.Containers[0].Resources.Requests["example.com/custom-resource"] = *resource.NewQuantity(10, resource.DecimalSI)
			}),
			nodes: []*v1.Node{
				test.BuildTestNode(nodeNames[0], 64000, 128*1000*1000*1000, 200, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}

					node.Status.Allocatable[v1.ResourceEphemeralStorage] = *resource.NewQuantity(1000*1000*1000*1000, resource.DecimalSI)
					node.Spec.Taints = []v1.Taint{
						{
							Key:    nodeTaintKey,
							Value:  nodeTaintValue,
							Effect: v1.TaintEffectNoSchedule,
						},
					}
					node.Status.Allocatable["example.com/custom-resource"] = *resource.NewQuantity(15, resource.DecimalSI)
				}),
				// Notice that this node only has 15 of the custom resource, the pod already on the node below requests 10, and the pod above requests 10
				test.BuildTestNode(nodeNames[1], 10*1000, 8*1000*1000*1000, 12, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}

					node.Status.Allocatable[v1.ResourceEphemeralStorage] = *resource.NewQuantity(200*1000*1000*1000, resource.DecimalSI)
					node.Status.Allocatable["example.com/custom-resource"] = *resource.NewQuantity(15, resource.DecimalSI)
				}),
				test.BuildTestNode(nodeNames[2], 0, 0, 0, nil),
			},
			podsOnNodes: []*v1.Pod{
				test.BuildTestPod("10-custom-resource-pod", 0, 0, nodeNames[1], func(pod *v1.Pod) {
					pod.ObjectMeta = metav1.ObjectMeta{
						Namespace: "test",
						Labels: map[string]string{
							"test": "true",
						},
					}
					pod.Spec.Containers[0].Resources.Requests["example.com/custom-resource"] = *resource.NewQuantity(10, resource.DecimalSI)
				}),
			},
			success: false,
		},
		{
			description: "Two nodes matches node selector, one of them is tained, CPU requests will fit, and pod Overhead is low enough, should pass",
			pod: test.BuildTestPod("p1", 1000, 2*1000*1000*1000, nodeNames[2], func(pod *v1.Pod) {
				pod.Spec.NodeSelector = map[string]string{
					nodeLabelKey: nodeLabelValue,
				}
				pod.Spec.Containers[0].Resources.Requests[v1.ResourceEphemeralStorage] = *resource.NewQuantity(10*1000*1000*1000, resource.DecimalSI)
			}),
			nodes: []*v1.Node{
				test.BuildTestNode(nodeNames[0], 64000, 128*1000*1000*1000, 200, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}

					node.Status.Allocatable[v1.ResourceEphemeralStorage] = *resource.NewQuantity(1000*1000*1000*1000, resource.DecimalSI)
					node.Spec.Taints = []v1.Taint{
						{
							Key:    nodeTaintKey,
							Value:  nodeTaintValue,
							Effect: v1.TaintEffectNoSchedule,
						},
					}
				}),
				// Notice that this node has 5 CPU cores, the pod below requests 2 cores, and has CPU overhead of 1 cores, and the pod above requests 1 core
				test.BuildTestNode(nodeNames[1], 5000, 8*1000*1000*1000, 12, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}

					node.Status.Allocatable[v1.ResourceEphemeralStorage] = *resource.NewQuantity(200*1000*1000*1000, resource.DecimalSI)
				}),
				test.BuildTestNode(nodeNames[2], 0, 0, 0, nil),
			},
			podsOnNodes: []*v1.Pod{
				test.BuildTestPod("3-core-pod", 2000, 4*1000*1000*1000, nodeNames[1], func(pod *v1.Pod) {
					pod.ObjectMeta = metav1.ObjectMeta{
						Namespace: "test",
						Labels: map[string]string{
							"test": "true",
						},
					}
					pod.Spec.Containers[0].Resources.Requests[v1.ResourceEphemeralStorage] = *resource.NewQuantity(10*1000*1000*1000, resource.DecimalSI)
					pod.Spec.Overhead = createResourceList(1000, 1000*1000*1000, 1000*1000*1000)
				}),
			},
			success: true,
		},
		{
			description: "Two nodes matches node selector, one of them is tained, CPU requests will fit, but pod Overhead is too high, should fail",
			pod: test.BuildTestPod("p1", 2000, 2*1000*1000*1000, nodeNames[2], func(pod *v1.Pod) {
				pod.Spec.NodeSelector = map[string]string{
					nodeLabelKey: nodeLabelValue,
				}
				pod.Spec.Containers[0].Resources.Requests[v1.ResourceEphemeralStorage] = *resource.NewQuantity(10*1000*1000*1000, resource.DecimalSI)
			}),
			nodes: []*v1.Node{
				test.BuildTestNode(nodeNames[0], 64000, 128*1000*1000*1000, 200, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}

					node.Status.Allocatable[v1.ResourceEphemeralStorage] = *resource.NewQuantity(1000*1000*1000*1000, resource.DecimalSI)
					node.Spec.Taints = []v1.Taint{
						{
							Key:    nodeTaintKey,
							Value:  nodeTaintValue,
							Effect: v1.TaintEffectNoSchedule,
						},
					}
				}),
				// Notice that this node only has 5 CPU cores, the pod below requests 2 cores, but has CPU overhead of 2 cores, and the pod above requests 2 cores
				test.BuildTestNode(nodeNames[1], 5000, 8*1000*1000*1000, 12, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}

					node.Status.Allocatable[v1.ResourceEphemeralStorage] = *resource.NewQuantity(200*1000*1000*1000, resource.DecimalSI)
				}),
				test.BuildTestNode(nodeNames[2], 0, 0, 0, nil),
			},
			podsOnNodes: []*v1.Pod{
				test.BuildTestPod("3-core-pod", 2000, 4*1000*1000*1000, nodeNames[1], func(pod *v1.Pod) {
					pod.ObjectMeta = metav1.ObjectMeta{
						Namespace: "test",
						Labels: map[string]string{
							"test": "true",
						},
					}
					pod.Spec.Containers[0].Resources.Requests[v1.ResourceEphemeralStorage] = *resource.NewQuantity(10*1000*1000*1000, resource.DecimalSI)
					pod.Spec.Overhead = createResourceList(2000, 1000*1000*1000, 1000*1000*1000)
				}),
			},
			success: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			var objs []runtime.Object
			for _, node := range tc.nodes {
				objs = append(objs, node)
			}
			for _, pod := range tc.podsOnNodes {
				objs = append(objs, pod)
			}
			objs = append(objs, tc.pod)

			fakeClient := fake.NewSimpleClientset(objs...)

			sharedInformerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
			podInformer := sharedInformerFactory.Core().V1().Pods().Informer()

			getPodsAssignedToNode, err := podutil.BuildGetPodsAssignedToNodeFunc(podInformer)
			if err != nil {
				t.Errorf("Build get pods assigned to node function error: %v", err)
			}

			sharedInformerFactory.Start(ctx.Done())
			sharedInformerFactory.WaitForCacheSync(ctx.Done())

			actual := PodFitsAnyOtherNode(getPodsAssignedToNode, tc.pod, tc.nodes, false)
			if actual != tc.success {
				t.Errorf("Test %#v failed", tc.description)
			}
		})
	}
}

func TestNodeFit(t *testing.T) {
	node := test.BuildTestNode("node", 64000, 128*1000*1000*1000, 2, nil)
	tests := []struct {
		description string
		pod         *v1.Pod
		node        *v1.Node
		podsOnNode  []*v1.Pod
		err         error
	}{
		{
			description: "insufficient cpu",
			pod:         test.BuildTestPod("p1", 10000, 2*1000*1000*1000, "", nil),
			node:        node,
			podsOnNode: []*v1.Pod{
				test.BuildTestPod("p2", 60000, 60*1000*1000*1000, "node", nil),
			},
			err: errors.New("insufficient cpu"),
		},
		{
			description: "insufficient pod num",
			pod:         test.BuildTestPod("p1", 1000, 2*1000*1000*1000, "", nil),
			node:        node,
			podsOnNode: []*v1.Pod{
				test.BuildTestPod("p2", 1000, 2*1000*1000*1000, "node", nil),
				test.BuildTestPod("p3", 1000, 2*1000*1000*1000, "node", nil),
			},
			err: errors.New("insufficient pods"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			objs := []runtime.Object{tc.node, tc.pod}
			for _, pod := range tc.podsOnNode {
				objs = append(objs, pod)
			}

			fakeClient := fake.NewSimpleClientset(objs...)

			sharedInformerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
			podInformer := sharedInformerFactory.Core().V1().Pods().Informer()

			getPodsAssignedToNode, err := podutil.BuildGetPodsAssignedToNodeFunc(podInformer)
			if err != nil {
				t.Errorf("Build get pods assigned to node function error: %v", err)
			}

			sharedInformerFactory.Start(ctx.Done())
			sharedInformerFactory.WaitForCacheSync(ctx.Done())
			if errs := NodeFit(getPodsAssignedToNode, tc.pod, tc.node, false); (len(errs) == 0 && tc.err != nil) || errs[0].Error() != tc.err.Error() {
				t.Errorf("Test %#v failed, got %v, expect %v", tc.description, errs, tc.err)
			}
		})
	}
}

// createResourceList builds a small resource list of core resources
func createResourceList(cpu, memory, ephemeralStorage int64) v1.ResourceList {
	resourceList := make(map[v1.ResourceName]resource.Quantity)
	resourceList[v1.ResourceCPU] = *resource.NewMilliQuantity(cpu, resource.DecimalSI)
	resourceList[v1.ResourceMemory] = *resource.NewQuantity(memory, resource.DecimalSI)
	resourceList[v1.ResourceEphemeralStorage] = *resource.NewQuantity(ephemeralStorage, resource.DecimalSI)
	return resourceList
}
