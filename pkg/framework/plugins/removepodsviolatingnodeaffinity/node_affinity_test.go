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

package removepodsviolatingnodeaffinity

import (
	"context"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"

	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	frameworktesting "sigs.k8s.io/descheduler/pkg/framework/testing"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
	"sigs.k8s.io/descheduler/test"
)

const (
	nodeWithLabelsName              = "nodeWithLabels"
	nodeWithoutLabelsName           = "nodeWithoutLabels"
	unschedulableNodeWithLabelsName = "unschedulableNodeWithLabels"
	nodeLabelKey                    = "kubernetes.io/desiredNode"
	nodeLabelValue                  = "yes"
)

func buildTestNode(name string, apply func(*v1.Node)) *v1.Node {
	return test.BuildTestNode(name, 2000, 3000, 10, apply)
}

func setNodeDesiredNodeLabel(node *v1.Node) {
	node.Labels[nodeLabelKey] = nodeLabelValue
}

func buildTestPod(name, nodeName string, apply func(*v1.Pod)) *v1.Pod {
	return test.BuildTestPod(name, 100, 0, nodeName, apply)
}

func buildUnschedulableNodeWithLabels() *v1.Node {
	return buildTestNode(unschedulableNodeWithLabelsName, func(node *v1.Node) {
		setNodeDesiredNodeLabel(node)
		node.Spec.Unschedulable = true
	})
}

func TestRemovePodsViolatingNodeAffinity(t *testing.T) {
	addPodsToNode := func(nodeName string, deletionTimestamp *metav1.Time, affinityType string) []*v1.Pod {
		podWithNodeAffinity := buildTestPod("podWithNodeAffinity", nodeName, func(pod *v1.Pod) {
			pod.Spec.Affinity = &v1.Affinity{
				NodeAffinity: &v1.NodeAffinity{},
			}

			switch affinityType {
			case "requiredDuringSchedulingIgnoredDuringExecution":
				pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &v1.NodeSelector{
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
				}
			case "preferredDuringSchedulingIgnoredDuringExecution":
				pod.Spec.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution = []v1.PreferredSchedulingTerm{
					{
						Weight: 10,
						Preference: v1.NodeSelectorTerm{
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
				}
			case "requiredDuringSchedulingRequiredDuringExecution":
			default:
				t.Fatalf("Invalid affinity type %s", affinityType)
			}

			pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
			pod.DeletionTimestamp = deletionTimestamp
		})

		pod1 := buildTestPod("pod1", nodeName, func(pod *v1.Pod) {
			pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
			pod.DeletionTimestamp = deletionTimestamp
		})
		pod2 := buildTestPod("pod2", nodeName, func(pod *v1.Pod) {
			pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
			pod.DeletionTimestamp = deletionTimestamp
		})

		return []*v1.Pod{
			podWithNodeAffinity,
			pod1,
			pod2,
		}
	}

	var uint0 uint = 0
	var uint1 uint = 1
	tests := []struct {
		description                    string
		nodes                          []*v1.Node
		pods                           []*v1.Pod
		expectedEvictedPodCount        uint
		maxPodsToEvictPerNode          *uint
		maxNoOfPodsToEvictPerNamespace *uint
		maxNoOfPodsToEvictTotal        *uint
		args                           RemovePodsViolatingNodeAffinityArgs
		nodefit                        bool
	}{
		{
			description: "Invalid Affinity type, should not evict any pods",
			args: RemovePodsViolatingNodeAffinityArgs{
				NodeAffinityType: []string{"requiredDuringSchedulingRequiredDuringExecution"},
			},
			expectedEvictedPodCount: 0,
			pods:                    addPodsToNode(nodeWithoutLabelsName, nil, "requiredDuringSchedulingRequiredDuringExecution"),
			nodes: []*v1.Node{
				buildTestNode(nodeWithoutLabelsName, nil),
				buildTestNode(nodeWithLabelsName, setNodeDesiredNodeLabel),
			},
		},
		{
			description: "Pod is correctly scheduled on node, no eviction expected [required affinity]",
			args: RemovePodsViolatingNodeAffinityArgs{
				NodeAffinityType: []string{"requiredDuringSchedulingIgnoredDuringExecution"},
			},
			expectedEvictedPodCount: 0,
			pods:                    addPodsToNode(nodeWithLabelsName, nil, "requiredDuringSchedulingIgnoredDuringExecution"),
			nodes: []*v1.Node{
				buildTestNode(nodeWithLabelsName, setNodeDesiredNodeLabel),
			},
		},
		{
			description: "Pod is correctly scheduled on node, no eviction expected [preferred affinity]",
			args: RemovePodsViolatingNodeAffinityArgs{
				NodeAffinityType: []string{"preferredDuringSchedulingIgnoredDuringExecution"},
			},
			expectedEvictedPodCount: 0,
			pods:                    addPodsToNode(nodeWithLabelsName, nil, "preferredDuringSchedulingIgnoredDuringExecution"),
			nodes: []*v1.Node{
				buildTestNode(nodeWithLabelsName, setNodeDesiredNodeLabel),
			},
		},
		{
			description:             "Pod is scheduled on node without matching labels, another schedulable node available, should be evicted",
			expectedEvictedPodCount: 1,
			args: RemovePodsViolatingNodeAffinityArgs{
				NodeAffinityType: []string{"requiredDuringSchedulingIgnoredDuringExecution"},
			},
			pods: addPodsToNode(nodeWithoutLabelsName, nil, "requiredDuringSchedulingIgnoredDuringExecution"),
			nodes: []*v1.Node{
				buildTestNode(nodeWithoutLabelsName, nil),
				buildTestNode(nodeWithLabelsName, setNodeDesiredNodeLabel),
			},
		},
		{
			description:             "Pod is scheduled on node without matching labels, another schedulable node available with better fit, should be evicted",
			expectedEvictedPodCount: 1,
			args: RemovePodsViolatingNodeAffinityArgs{
				NodeAffinityType: []string{"preferredDuringSchedulingIgnoredDuringExecution"},
			},
			pods: addPodsToNode(nodeWithoutLabelsName, nil, "preferredDuringSchedulingIgnoredDuringExecution"),
			nodes: []*v1.Node{
				buildTestNode(nodeWithoutLabelsName, nil),
				buildTestNode(nodeWithLabelsName, setNodeDesiredNodeLabel),
			},
		},
		{
			description:             "Pod is scheduled on node without matching labels, another schedulable node available, maxPodsToEvictPerNode set to 1, should be evicted [required affinity]",
			expectedEvictedPodCount: 1,
			args: RemovePodsViolatingNodeAffinityArgs{
				NodeAffinityType: []string{"requiredDuringSchedulingIgnoredDuringExecution"},
			},
			pods: addPodsToNode(nodeWithoutLabelsName, nil, "requiredDuringSchedulingIgnoredDuringExecution"),
			nodes: []*v1.Node{
				buildTestNode(nodeWithoutLabelsName, nil),
				buildTestNode(nodeWithLabelsName, setNodeDesiredNodeLabel),
			},
			maxPodsToEvictPerNode: &uint1,
		},
		{
			description:             "Pod is scheduled on node without matching labels, another schedulable node available, maxPodsToEvictPerNode set to 1, should be evicted [preferred affinity]",
			expectedEvictedPodCount: 1,
			args: RemovePodsViolatingNodeAffinityArgs{
				NodeAffinityType: []string{"preferredDuringSchedulingIgnoredDuringExecution"},
			},
			pods: addPodsToNode(nodeWithoutLabelsName, nil, "preferredDuringSchedulingIgnoredDuringExecution"),
			nodes: []*v1.Node{
				buildTestNode(nodeWithoutLabelsName, nil),
				buildTestNode(nodeWithLabelsName, setNodeDesiredNodeLabel),
			},
			maxPodsToEvictPerNode: &uint1,
		},
		{
			description:             "Pod is scheduled on node without matching labels, another schedulable node available, maxPodsToEvictPerNode set to 0, should be not evicted [required affinity]",
			expectedEvictedPodCount: 0,
			args: RemovePodsViolatingNodeAffinityArgs{
				NodeAffinityType: []string{"requiredDuringSchedulingIgnoredDuringExecution"},
			},
			pods: addPodsToNode(nodeWithoutLabelsName, nil, "requiredDuringSchedulingIgnoredDuringExecution"),
			nodes: []*v1.Node{
				buildTestNode(nodeWithoutLabelsName, nil),
				buildTestNode(nodeWithLabelsName, setNodeDesiredNodeLabel),
			},
			maxPodsToEvictPerNode: &uint0,
		},
		{
			description:             "Pod is scheduled on node without matching labels, another schedulable node available, maxPodsToEvictPerNode set to 0, should be not evicted [preferred affinity]",
			expectedEvictedPodCount: 0,
			args: RemovePodsViolatingNodeAffinityArgs{
				NodeAffinityType: []string{"preferredDuringSchedulingIgnoredDuringExecution"},
			},
			pods: addPodsToNode(nodeWithoutLabelsName, nil, "preferredDuringSchedulingIgnoredDuringExecution"),
			nodes: []*v1.Node{
				buildTestNode(nodeWithoutLabelsName, nil),
				buildTestNode(nodeWithLabelsName, setNodeDesiredNodeLabel),
			},
			maxPodsToEvictPerNode: &uint0,
		},
		{
			description:             "Pod is scheduled on node without matching labels, another schedulable node available, maxPodsToEvictPerNode set to 1, no pod evicted since pod terminating [required affinity]",
			expectedEvictedPodCount: 0,
			args: RemovePodsViolatingNodeAffinityArgs{
				NodeAffinityType: []string{"requiredDuringSchedulingIgnoredDuringExecution"},
			},
			pods: addPodsToNode(nodeWithoutLabelsName, &metav1.Time{}, "requiredDuringSchedulingIgnoredDuringExecution"),
			nodes: []*v1.Node{
				buildTestNode(nodeWithoutLabelsName, nil),
				buildTestNode(nodeWithLabelsName, setNodeDesiredNodeLabel),
			},
			maxPodsToEvictPerNode: &uint1,
		},
		{
			description:             "Pod is scheduled on node without matching labels, another schedulable node available, maxPodsToEvictPerNode set to 1, no pod evicted since pod terminating [preferred affinity]",
			expectedEvictedPodCount: 0,
			args: RemovePodsViolatingNodeAffinityArgs{
				NodeAffinityType: []string{"preferredDuringSchedulingIgnoredDuringExecution"},
			},
			pods: addPodsToNode(nodeWithoutLabelsName, &metav1.Time{}, "preferredDuringSchedulingIgnoredDuringExecution"),
			nodes: []*v1.Node{
				buildTestNode(nodeWithoutLabelsName, nil),
				buildTestNode(nodeWithLabelsName, setNodeDesiredNodeLabel),
			},
			maxPodsToEvictPerNode: &uint1,
		},
		{
			description:             "Pod is scheduled on node without matching labels, another schedulable node available, maxNoOfPodsToEvictPerNamespace set to 1, should be evicted [required affinity]",
			expectedEvictedPodCount: 1,
			args: RemovePodsViolatingNodeAffinityArgs{
				NodeAffinityType: []string{"requiredDuringSchedulingIgnoredDuringExecution"},
			},
			pods: addPodsToNode(nodeWithoutLabelsName, nil, "requiredDuringSchedulingIgnoredDuringExecution"),
			nodes: []*v1.Node{
				buildTestNode(nodeWithoutLabelsName, nil),
				buildTestNode(nodeWithLabelsName, setNodeDesiredNodeLabel),
			},
			maxNoOfPodsToEvictPerNamespace: &uint1,
		},
		{
			description:             "Pod is scheduled on node without matching labels, another schedulable node available, maxNoOfPodsToEvictTotal set to 0, should not be evicted [required affinity]",
			expectedEvictedPodCount: 0,
			args: RemovePodsViolatingNodeAffinityArgs{
				NodeAffinityType: []string{"requiredDuringSchedulingIgnoredDuringExecution"},
			},
			pods: addPodsToNode(nodeWithoutLabelsName, nil, "requiredDuringSchedulingIgnoredDuringExecution"),
			nodes: []*v1.Node{
				buildTestNode(nodeWithoutLabelsName, nil),
				buildTestNode(nodeWithLabelsName, setNodeDesiredNodeLabel),
			},
			maxNoOfPodsToEvictPerNamespace: &uint1,
			maxNoOfPodsToEvictTotal:        &uint0,
		},
		{
			description:             "Pod is scheduled on node without matching labels, another schedulable node available, maxNoOfPodsToEvictPerNamespace set to 1, should be evicted [preferred affinity]",
			expectedEvictedPodCount: 1,
			args: RemovePodsViolatingNodeAffinityArgs{
				NodeAffinityType: []string{"preferredDuringSchedulingIgnoredDuringExecution"},
			},
			pods: addPodsToNode(nodeWithoutLabelsName, nil, "preferredDuringSchedulingIgnoredDuringExecution"),
			nodes: []*v1.Node{
				buildTestNode(nodeWithoutLabelsName, nil),
				buildTestNode(nodeWithLabelsName, setNodeDesiredNodeLabel),
			},
			maxNoOfPodsToEvictPerNamespace: &uint1,
		},
		{
			description:             "Pod is scheduled on node without matching labels, another schedulable node available, maxNoOfPodsToEvictPerNamespace set to 0, should not be evicted [required affinity]",
			expectedEvictedPodCount: 0,
			args: RemovePodsViolatingNodeAffinityArgs{
				NodeAffinityType: []string{"requiredDuringSchedulingIgnoredDuringExecution"},
			},
			pods: addPodsToNode(nodeWithoutLabelsName, nil, "requiredDuringSchedulingIgnoredDuringExecution"),
			nodes: []*v1.Node{
				buildTestNode(nodeWithoutLabelsName, nil),
				buildTestNode(nodeWithLabelsName, setNodeDesiredNodeLabel),
			},
			maxNoOfPodsToEvictPerNamespace: &uint0,
		},
		{
			description:             "Pod is scheduled on node without matching labels, another schedulable node available, maxNoOfPodsToEvictPerNamespace set to 0, should not be evicted [preferred affinity]",
			expectedEvictedPodCount: 0,
			args: RemovePodsViolatingNodeAffinityArgs{
				NodeAffinityType: []string{"preferredDuringSchedulingIgnoredDuringExecution"},
			},
			pods: addPodsToNode(nodeWithoutLabelsName, nil, "preferredDuringSchedulingIgnoredDuringExecution"),
			nodes: []*v1.Node{
				buildTestNode(nodeWithoutLabelsName, nil),
				buildTestNode(nodeWithLabelsName, setNodeDesiredNodeLabel),
			},
			maxNoOfPodsToEvictPerNamespace: &uint0,
		},
		{
			description:             "Pod is scheduled on node without matching labels, another schedulable node available, maxNoOfPodsToEvictPerNamespace set to 1, no pod evicted since pod terminting [required affinity]",
			expectedEvictedPodCount: 0,
			args: RemovePodsViolatingNodeAffinityArgs{
				NodeAffinityType: []string{"requiredDuringSchedulingIgnoredDuringExecution"},
			},
			pods: addPodsToNode(nodeWithoutLabelsName, &metav1.Time{}, "requiredDuringSchedulingIgnoredDuringExecution"),
			nodes: []*v1.Node{
				buildTestNode(nodeWithoutLabelsName, nil),
				buildTestNode(nodeWithLabelsName, setNodeDesiredNodeLabel),
			},
			maxNoOfPodsToEvictPerNamespace: &uint1,
		},
		{
			description:             "Pod is scheduled on node without matching labels, another schedulable node available, maxNoOfPodsToEvictPerNamespace set to 1, no pod evicted since pod terminting [preferred affinity]",
			expectedEvictedPodCount: 0,
			args: RemovePodsViolatingNodeAffinityArgs{
				NodeAffinityType: []string{"preferredDuringSchedulingIgnoredDuringExecution"},
			},
			pods: addPodsToNode(nodeWithoutLabelsName, &metav1.Time{}, "preferredDuringSchedulingIgnoredDuringExecution"),
			nodes: []*v1.Node{
				buildTestNode(nodeWithoutLabelsName, nil),
				buildTestNode(nodeWithLabelsName, setNodeDesiredNodeLabel),
			},
			maxNoOfPodsToEvictPerNamespace: &uint1,
		},
		{
			description:             "Pod is scheduled on node without matching labels, but no node where pod fits is available, should not evict [required affinity]",
			expectedEvictedPodCount: 0,
			args: RemovePodsViolatingNodeAffinityArgs{
				NodeAffinityType: []string{"requiredDuringSchedulingIgnoredDuringExecution"},
			},
			pods: addPodsToNode(nodeWithoutLabelsName, nil, "requiredDuringSchedulingIgnoredDuringExecution"),
			nodes: []*v1.Node{
				buildTestNode(nodeWithoutLabelsName, nil),
				buildUnschedulableNodeWithLabels(),
			},
			nodefit: true,
		},
		{
			description:             "Pod is scheduled on node without matching labels, but no node where pod fits is available, should not evict [preferred affinity]",
			expectedEvictedPodCount: 0,
			args: RemovePodsViolatingNodeAffinityArgs{
				NodeAffinityType: []string{"preferredDuringSchedulingIgnoredDuringExecution"},
			},
			pods: addPodsToNode(nodeWithoutLabelsName, nil, "preferredDuringSchedulingIgnoredDuringExecution"),
			nodes: []*v1.Node{
				buildTestNode(nodeWithoutLabelsName, nil),
				buildUnschedulableNodeWithLabels(),
			},
			nodefit: true,
		},
		{
			description:             "Pod is scheduled on node without matching labels, and unschedulable node where pod could fit is available, should not evict [required affinity]",
			expectedEvictedPodCount: 0,
			args: RemovePodsViolatingNodeAffinityArgs{
				NodeAffinityType: []string{"requiredDuringSchedulingIgnoredDuringExecution"},
			},
			pods: addPodsToNode(nodeWithoutLabelsName, nil, "requiredDuringSchedulingIgnoredDuringExecution"),
			nodes: []*v1.Node{
				buildTestNode(nodeWithLabelsName, setNodeDesiredNodeLabel),
				buildUnschedulableNodeWithLabels(),
			},
			maxPodsToEvictPerNode: &uint1,
			nodefit:               true,
		},
		{
			description:             "Pod is scheduled on node without matching labels, and unschedulable node where pod could fit is available, should not evict [preferred affinity]",
			expectedEvictedPodCount: 0,
			args: RemovePodsViolatingNodeAffinityArgs{
				NodeAffinityType: []string{"preferredDuringSchedulingIgnoredDuringExecution"},
			},
			pods: addPodsToNode(nodeWithoutLabelsName, nil, "preferredDuringSchedulingIgnoredDuringExecution"),
			nodes: []*v1.Node{
				buildTestNode(nodeWithLabelsName, setNodeDesiredNodeLabel),
				buildUnschedulableNodeWithLabels(),
			},
			maxPodsToEvictPerNode: &uint1,
			nodefit:               true,
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
			for _, pod := range tc.pods {
				objs = append(objs, pod)
			}
			fakeClient := fake.NewSimpleClientset(objs...)

			handle, podEvictor, err := frameworktesting.InitFrameworkHandle(
				ctx,
				fakeClient,
				evictions.NewOptions().
					WithMaxPodsToEvictPerNode(tc.maxPodsToEvictPerNode).
					WithMaxPodsToEvictPerNamespace(tc.maxNoOfPodsToEvictPerNamespace).
					WithMaxPodsToEvictTotal(tc.maxNoOfPodsToEvictTotal),
				defaultevictor.DefaultEvictorArgs{NodeFit: tc.nodefit},
				nil,
			)
			if err != nil {
				t.Fatalf("Unable to initialize a framework handle: %v", err)
			}

			plugin, err := New(
				ctx,
				&RemovePodsViolatingNodeAffinityArgs{
					NodeAffinityType: tc.args.NodeAffinityType,
				},
				handle,
			)
			if err != nil {
				t.Fatalf("Unable to initialize the plugin: %v", err)
			}

			plugin.(frameworktypes.DeschedulePlugin).Deschedule(ctx, tc.nodes)
			actualEvictedPodCount := podEvictor.TotalEvicted()
			if actualEvictedPodCount != tc.expectedEvictedPodCount {
				t.Errorf("Test %#v failed, expected %v pod evictions, but got %v pod evictions\n", tc.description, tc.expectedEvictedPodCount, actualEvictedPodCount)
			}
		})
	}
}
