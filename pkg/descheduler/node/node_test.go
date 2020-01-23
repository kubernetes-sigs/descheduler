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
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/descheduler/test"
)

func TestReadyNodes(t *testing.T) {
	node1 := test.BuildTestNode("node2", 1000, 2000, 9)
	node2 := test.BuildTestNode("node3", 1000, 2000, 9)
	node2.Status.Conditions = []v1.NodeCondition{{Type: v1.NodeMemoryPressure, Status: v1.ConditionTrue}}
	node3 := test.BuildTestNode("node4", 1000, 2000, 9)
	node3.Status.Conditions = []v1.NodeCondition{{Type: v1.NodeNetworkUnavailable, Status: v1.ConditionTrue}}
	node4 := test.BuildTestNode("node5", 1000, 2000, 9)
	node4.Spec.Unschedulable = true
	node5 := test.BuildTestNode("node6", 1000, 2000, 9)
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
	node1 := test.BuildTestNode("node1", 1000, 2000, 9)
	node1.Labels = map[string]string{"type": "compute"}
	node2 := test.BuildTestNode("node2", 1000, 2000, 9)
	node2.Labels = map[string]string{"type": "infra"}

	fakeClient := fake.NewSimpleClientset(node1, node2)
	nodeSelector := "type=compute"
	nodes, _ := ReadyNodes(fakeClient, nodeSelector, nil)

	if nodes[0].Name != "node1" {
		t.Errorf("Expected node1, got %s", nodes[0].Name)
	}
}

func TestIsNodeUschedulable(t *testing.T) {
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
		actualUnSchedulable := IsNodeUschedulable(test.node)
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
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						nodeLabelKey: nodeLabelValue,
					},
				},
			},
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
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						nodeLabelKey: "no",
					},
				},
			},
			success: false,
		},
	}

	for _, tc := range tests {
		actual := PodFitsCurrentNode(tc.pod, tc.node)
		if actual != tc.success {
			t.Errorf("Test %#v failed", tc.description)
		}
	}
}

func TestPodFitsAnyNode(t *testing.T) {

	nodeLabelKey := "kubernetes.io/desiredNode"
	nodeLabelValue := "yes"

	tests := []struct {
		description string
		pod         *v1.Pod
		nodes       []*v1.Node
		success     bool
	}{
		{
			description: "Pod expected to fit one of the nodes",
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
			nodes: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							nodeLabelKey: nodeLabelValue,
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							nodeLabelKey: "no",
						},
					},
				},
			},
			success: true,
		},
		{
			description: "Pod expected to fit none of the nodes",
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
			nodes: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							nodeLabelKey: "unfit1",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							nodeLabelKey: "unfit2",
						},
					},
				},
			},
			success: false,
		},
		{
			description: "Nodes are unschedulable but labels match, should fail",
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
			nodes: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							nodeLabelKey: nodeLabelValue,
						},
					},
					Spec: v1.NodeSpec{
						Unschedulable: true,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							nodeLabelKey: "no",
						},
					},
				},
			},
			success: false,
		},
	}

	for _, tc := range tests {
		actual := PodFitsAnyNode(tc.pod, tc.nodes)
		if actual != tc.success {
			t.Errorf("Test %#v failed", tc.description)
		}
	}
}
