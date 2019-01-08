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

package strategies

import (
	"testing"

	"github.com/kubernetes-incubator/descheduler/cmd/descheduler/app/options"
	"github.com/kubernetes-incubator/descheduler/pkg/api"
	"github.com/kubernetes-incubator/descheduler/test"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
)

func TestRemovePodsViolatingNodeAffinity(t *testing.T) {

	requiredDuringSchedulingIgnoredDuringExecutionStrategy := api.DeschedulerStrategy{
		Enabled: true,
		Params: api.StrategyParameters{
			NodeAffinityType: []string{
				"requiredDuringSchedulingIgnoredDuringExecution",
			},
		},
	}
	preferredDuringSchedulingIgnoredDuringExecutionStrategy := api.DeschedulerStrategy{
		Enabled: true,
		Params: api.StrategyParameters{
			NodeAffinityType: []string{
				"preferredDuringSchedulingIgnoredDuringExecution",
			},
		},
	}
	mixedStrategy := api.DeschedulerStrategy{
		Enabled: true,
		Params: api.StrategyParameters{
			NodeAffinityType: []string{
				"requiredDuringSchedulingIgnoredDuringExecution",
				"preferredDuringSchedulingIgnoredDuringExecution",
			},
		},
	}

	nodeLabelKey := "kubernetes.io/desiredNode"
	nodeLabelKey2 := "kubernetes.io/desiredNode2"
	nodeLabelValue := "yes"
	nodeWithLabels := test.BuildTestNode("nodeWithLabels", 2000, 3000, 10)
	nodeWithLabels.Labels[nodeLabelKey] = nodeLabelValue
	nodeWithLabels.Labels[nodeLabelKey2] = nodeLabelValue

	nodeWithoutLabels := test.BuildTestNode("nodeWithoutLabels", 2000, 3000, 10)

	unschedulableNodeWithLabels := test.BuildTestNode("unschedulableNodeWithLabels", 2000, 3000, 10)
	unschedulableNodeWithLabels.Labels[nodeLabelKey] = nodeLabelValue
	unschedulableNodeWithLabels.Spec.Unschedulable = true

	addPodsToNode := func(node *v1.Node) []v1.Pod {
		podWithNodeAffinity := test.BuildTestPod("podWithNodeAffinity", 100, 0, node.Name)
		podWithNodeAffinity.Spec.Affinity = &v1.Affinity{
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
				PreferredDuringSchedulingIgnoredDuringExecution: []v1.PreferredSchedulingTerm{
					{
						Weight: 10,
						Preference: v1.NodeSelectorTerm{
							MatchExpressions: []v1.NodeSelectorRequirement{
								{
									Key:      nodeLabelKey2,
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
		}

		podWithNodeAffinity2 := test.BuildTestPod("podWithNodeAffinity2", 100, 0, node.Name)
		podWithNodeAffinity2.Spec.Affinity = &v1.Affinity{
			NodeAffinity: &v1.NodeAffinity{
				PreferredDuringSchedulingIgnoredDuringExecution: []v1.PreferredSchedulingTerm{
					{
						Weight: 10,
						Preference: v1.NodeSelectorTerm{
							MatchExpressions: []v1.NodeSelectorRequirement{
								{
									Key:      nodeLabelKey2,
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
		}

		pod1 := test.BuildTestPod("pod1", 100, 0, node.Name)
		pod2 := test.BuildTestPod("pod2", 100, 0, node.Name)

		podWithNodeAffinity.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
		podWithNodeAffinity2.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
		pod1.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
		pod2.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()

		return []v1.Pod{
			*podWithNodeAffinity,
			*podWithNodeAffinity2,
			*pod1,
			*pod2,
		}
	}

	tests := []struct {
		description             string
		nodes                   []*v1.Node
		pods                    []v1.Pod
		strategy                api.DeschedulerStrategy
		expectedEvictedPodCount int
		npe                     nodePodEvictedCount
		maxPodsToEvict          int
	}{
		{
			description: "Strategy disabled, should not evict any pods",
			strategy: api.DeschedulerStrategy{
				Enabled: false,
				Params: api.StrategyParameters{
					NodeAffinityType: []string{
						"requiredDuringSchedulingIgnoredDuringExecution",
					},
				},
			},
			expectedEvictedPodCount: 0,
			pods:           addPodsToNode(nodeWithoutLabels),
			nodes:          []*v1.Node{nodeWithoutLabels, nodeWithLabels},
			npe:            nodePodEvictedCount{nodeWithoutLabels: 0, nodeWithLabels: 0},
			maxPodsToEvict: 0,
		},
		{
			description: "Invalid strategy type, should not evict any pods",
			strategy: api.DeschedulerStrategy{
				Enabled: true,
				Params: api.StrategyParameters{
					NodeAffinityType: []string{
						"requiredDuringSchedulingRequiredDuringExecution",
					},
				},
			},
			expectedEvictedPodCount: 0,
			pods:           addPodsToNode(nodeWithoutLabels),
			nodes:          []*v1.Node{nodeWithoutLabels, nodeWithLabels},
			npe:            nodePodEvictedCount{nodeWithoutLabels: 0, nodeWithLabels: 0},
			maxPodsToEvict: 0,
		},
		{
			description:             "Pod is correctly scheduled on node, no eviction expected",
			strategy:                requiredDuringSchedulingIgnoredDuringExecutionStrategy,
			expectedEvictedPodCount: 0,
			pods:           addPodsToNode(nodeWithLabels),
			nodes:          []*v1.Node{nodeWithLabels},
			npe:            nodePodEvictedCount{nodeWithLabels: 0},
			maxPodsToEvict: 0,
		},
		{
			description:             "Pod is scheduled on node without matching labels, another schedulable node available, should be evicted",
			expectedEvictedPodCount: 1,
			strategy:                requiredDuringSchedulingIgnoredDuringExecutionStrategy,
			pods:                    addPodsToNode(nodeWithoutLabels),
			nodes:                   []*v1.Node{nodeWithoutLabels, nodeWithLabels},
			npe:                     nodePodEvictedCount{nodeWithoutLabels: 0, nodeWithLabels: 0},
			maxPodsToEvict:          0,
		},
		{
			description:             "Pod is scheduled on node without matching labels, another schedulable node available, maxPodsToEvict set to 1, should not be evicted",
			expectedEvictedPodCount: 1,
			strategy:                requiredDuringSchedulingIgnoredDuringExecutionStrategy,
			pods:                    addPodsToNode(nodeWithoutLabels),
			nodes:                   []*v1.Node{nodeWithoutLabels, nodeWithLabels},
			npe:                     nodePodEvictedCount{nodeWithoutLabels: 0, nodeWithLabels: 0},
			maxPodsToEvict:          1,
		},
		{
			description:             "Pod is scheduled on node without matching labels, but no node where pod fits is available, should not evict",
			expectedEvictedPodCount: 0,
			strategy:                requiredDuringSchedulingIgnoredDuringExecutionStrategy,
			pods:                    addPodsToNode(nodeWithoutLabels),
			nodes:                   []*v1.Node{nodeWithoutLabels, unschedulableNodeWithLabels},
			npe:                     nodePodEvictedCount{nodeWithoutLabels: 0, unschedulableNodeWithLabels: 0},
			maxPodsToEvict:          0,
		},
		{
			description:             "2 pods scheduled on 0 score node, another better node available, should be evicted",
			expectedEvictedPodCount: 2,
			strategy:                preferredDuringSchedulingIgnoredDuringExecutionStrategy,
			pods:                    addPodsToNode(nodeWithoutLabels),
			nodes:                   []*v1.Node{nodeWithoutLabels, nodeWithLabels},
			npe:                     nodePodEvictedCount{nodeWithoutLabels: 0, nodeWithLabels: 0},
			maxPodsToEvict:          0,
		},
		{
			description:             "[required/preferred mixed strategy]: There are 2 violated pods, both should be evicted",
			expectedEvictedPodCount: 2,
			strategy:                mixedStrategy,
			pods:                    addPodsToNode(nodeWithoutLabels),
			nodes:                   []*v1.Node{nodeWithoutLabels, nodeWithLabels},
			npe:                     nodePodEvictedCount{nodeWithoutLabels: 0, nodeWithLabels: 0},
			maxPodsToEvict:          0,
		},
	}

	for _, tc := range tests {

		fakeClient := &fake.Clientset{}
		fakeClient.Fake.AddReactor("list", "pods", func(action core.Action) (bool, runtime.Object, error) {
			return true, &v1.PodList{Items: tc.pods}, nil
		})

		ds := options.DeschedulerServer{
			Client: fakeClient,
		}

		actualEvictedPodCount := removePodsViolatingNodeAffinityCount(&ds, tc.strategy, "v1", tc.nodes, tc.npe, tc.maxPodsToEvict)
		if actualEvictedPodCount != tc.expectedEvictedPodCount {
			t.Errorf("Test %#v failed, expected %v pod evictions, but got %v pod evictions\n", tc.description, tc.expectedEvictedPodCount, actualEvictedPodCount)
		}
	}
}
