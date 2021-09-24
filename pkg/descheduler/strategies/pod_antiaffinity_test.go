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
	"context"
	"testing"

	v1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	"sigs.k8s.io/descheduler/pkg/utils"
	"sigs.k8s.io/descheduler/test"
)

func TestPodAntiAffinity(t *testing.T) {
	ctx := context.Background()
	node1 := test.BuildTestNode("n1", 2000, 3000, 10, nil)
	node2 := test.BuildTestNode("n2", 2000, 3000, 10, func(node *v1.Node) {
		node.ObjectMeta.Labels = map[string]string{
			"datacenter": "east",
		}
	})

	node3 := test.BuildTestNode("n3", 2000, 3000, 10, func(node *v1.Node) {
		node.Spec = v1.NodeSpec{
			Unschedulable: true,
		}
	})
	node4 := test.BuildTestNode("n1", 2, 2, 1, nil)

	p1 := test.BuildTestPod("p1", 100, 0, node1.Name, nil)
	p2 := test.BuildTestPod("p2", 100, 0, node1.Name, nil)
	p3 := test.BuildTestPod("p3", 100, 0, node1.Name, nil)
	p4 := test.BuildTestPod("p4", 100, 0, node1.Name, nil)
	p5 := test.BuildTestPod("p5", 100, 0, node1.Name, nil)
	p6 := test.BuildTestPod("p6", 100, 0, node1.Name, nil)
	p7 := test.BuildTestPod("p7", 100, 0, node1.Name, nil)
	p8 := test.BuildTestPod("p8", 100, 0, node1.Name, nil)

	criticalPriority := utils.SystemCriticalPriority
	nonEvictablePod := test.BuildTestPod("non-evict", 100, 0, node1.Name, func(pod *v1.Pod) {
		pod.Spec.Priority = &criticalPriority
	})
	p2.Labels = map[string]string{"foo": "bar"}
	p5.Labels = map[string]string{"foo": "bar"}
	p6.Labels = map[string]string{"foo": "bar"}
	p7.Labels = map[string]string{"foo1": "bar1"}
	nonEvictablePod.Labels = map[string]string{"foo": "bar"}
	test.SetNormalOwnerRef(p1)
	test.SetNormalOwnerRef(p2)
	test.SetNormalOwnerRef(p3)
	test.SetNormalOwnerRef(p4)
	test.SetNormalOwnerRef(p5)
	test.SetNormalOwnerRef(p6)
	test.SetNormalOwnerRef(p7)

	// set pod anti affinity
	setPodAntiAffinity(p1, "foo", "bar")
	setPodAntiAffinity(p3, "foo", "bar")
	setPodAntiAffinity(p4, "foo", "bar")
	setPodAntiAffinity(p5, "foo1", "bar1")
	setPodAntiAffinity(p6, "foo1", "bar1")
	setPodAntiAffinity(p7, "foo", "bar")

	// set pod priority
	test.SetPodPriority(p5, 100)
	test.SetPodPriority(p6, 50)
	test.SetPodPriority(p7, 0)

	// Set pod node selectors
	p8.Spec.NodeSelector = map[string]string{
		"datacenter": "west",
	}

	tests := []struct {
		description             string
		maxPodsToEvictPerNode   int
		pods                    []v1.Pod
		expectedEvictedPodCount int
		nodeFit                 bool
		nodes                   []*v1.Node
	}{
		{
			description:             "Maximum pods to evict - 0",
			maxPodsToEvictPerNode:   0,
			pods:                    []v1.Pod{*p1, *p2, *p3, *p4},
			nodes:                   []*v1.Node{node1},
			expectedEvictedPodCount: 3,
		},
		{
			description:             "Maximum pods to evict - 3",
			maxPodsToEvictPerNode:   3,
			pods:                    []v1.Pod{*p1, *p2, *p3, *p4},
			nodes:                   []*v1.Node{node1},
			expectedEvictedPodCount: 3,
		},
		{
			description:             "Evict only 1 pod after sorting",
			maxPodsToEvictPerNode:   0,
			pods:                    []v1.Pod{*p5, *p6, *p7},
			nodes:                   []*v1.Node{node1},
			expectedEvictedPodCount: 1,
		},
		{
			description:             "Evicts pod that conflicts with critical pod (but does not evict critical pod)",
			maxPodsToEvictPerNode:   1,
			pods:                    []v1.Pod{*p1, *nonEvictablePod},
			nodes:                   []*v1.Node{node1},
			expectedEvictedPodCount: 1,
		},
		{
			description:             "Evicts pod that conflicts with critical pod (but does not evict critical pod)",
			maxPodsToEvictPerNode:   1,
			pods:                    []v1.Pod{*p1, *nonEvictablePod},
			nodes:                   []*v1.Node{node1},
			expectedEvictedPodCount: 1,
		},
		{
			description:             "Won't evict pods because node selectors don't match available nodes",
			maxPodsToEvictPerNode:   1,
			pods:                    []v1.Pod{*p8, *nonEvictablePod},
			nodes:                   []*v1.Node{node1, node2},
			expectedEvictedPodCount: 0,
			nodeFit:                 true,
		},
		{
			description:             "Won't evict pods because only other node is not schedulable",
			maxPodsToEvictPerNode:   1,
			pods:                    []v1.Pod{*p8, *nonEvictablePod},
			nodes:                   []*v1.Node{node1, node3},
			expectedEvictedPodCount: 0,
			nodeFit:                 true,
		},
		{
			description:             "Won't evict pods because only other node doesn't have enough resources",
			maxPodsToEvictPerNode:   3,
			pods:                    []v1.Pod{*p1, *p2, *p3, *p4},
			nodes:                   []*v1.Node{node1, node4},
			expectedEvictedPodCount: 0,
			nodeFit:                 true,
		},
	}

	for _, test := range tests {
		// create fake client
		fakeClient := &fake.Clientset{}
		fakeClient.Fake.AddReactor("list", "pods", func(action core.Action) (bool, runtime.Object, error) {
			return true, &v1.PodList{Items: test.pods}, nil
		})
		fakeClient.Fake.AddReactor("get", "nodes", func(action core.Action) (bool, runtime.Object, error) {
			return true, node1, nil
		})

		podEvictor := evictions.NewPodEvictor(
			fakeClient,
			policyv1.SchemeGroupVersion.String(),
			false,
			test.maxPodsToEvictPerNode,
			test.nodes,
			false,
			false,
			false,
		)
		strategy := api.DeschedulerStrategy{
			Params: &api.StrategyParameters{
				NodeFit: test.nodeFit,
			},
		}

		RemovePodsViolatingInterPodAntiAffinity(ctx, fakeClient, strategy, test.nodes, podEvictor)
		podsEvicted := podEvictor.TotalEvicted()
		if podsEvicted != test.expectedEvictedPodCount {
			t.Errorf("Unexpected no of pods evicted: pods evicted: %d, expected: %d", podsEvicted, test.expectedEvictedPodCount)
		}
	}
}

func setPodAntiAffinity(inputPod *v1.Pod, labelKey, labelValue string) {
	inputPod.Spec.Affinity = &v1.Affinity{
		PodAntiAffinity: &v1.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
				{
					LabelSelector: &metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      labelKey,
								Operator: metav1.LabelSelectorOpIn,
								Values:   []string{labelValue},
							},
						},
					},
					TopologyKey: "region",
				},
			},
		},
	}
}
