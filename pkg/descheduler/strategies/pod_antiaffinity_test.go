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

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	"sigs.k8s.io/descheduler/test"
)

func TestPodAntiAffinity(t *testing.T) {
	ctx := context.Background()
	node := test.BuildTestNode("n1", 2000, 3000, 10, nil)
	p1 := test.BuildTestPod("p1", 100, 0, node.Name, nil)
	p2 := test.BuildTestPod("p2", 100, 0, node.Name, nil)
	p3 := test.BuildTestPod("p3", 100, 0, node.Name, nil)
	p4 := test.BuildTestPod("p4", 100, 0, node.Name, nil)
	p2.Labels = map[string]string{"foo": "bar"}
	p1.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
	p2.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
	p3.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
	p4.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()

	// set pod anti affinity
	setPodAntiAffinity(p1)
	setPodAntiAffinity(p3)
	setPodAntiAffinity(p4)

	tests := []struct {
		description             string
		maxPodsToEvict          int
		pods                    []v1.Pod
		expectedEvictedPodCount int
	}{
		{
			description:             "Maximum pods to evict - 0",
			maxPodsToEvict:          0,
			pods:                    []v1.Pod{*p1, *p2, *p3, *p4},
			expectedEvictedPodCount: 3,
		},
		{
			description:             "Maximum pods to evict - 3",
			maxPodsToEvict:          3,
			pods:                    []v1.Pod{*p1, *p2, *p3, *p4},
			expectedEvictedPodCount: 3,
		},
	}

	for _, test := range tests {
		// create fake client
		fakeClient := &fake.Clientset{}
		fakeClient.Fake.AddReactor("list", "pods", func(action core.Action) (bool, runtime.Object, error) {
			return true, &v1.PodList{Items: []v1.Pod{*p1, *p2, *p3, *p4}}, nil
		})
		fakeClient.Fake.AddReactor("get", "nodes", func(action core.Action) (bool, runtime.Object, error) {
			return true, node, nil
		})

		podEvictor := evictions.NewPodEvictor(
			fakeClient,
			"v1",
			false,
			false,
			test.maxPodsToEvict,
			[]*v1.Node{node},
		)

		RemovePodsViolatingInterPodAntiAffinity(ctx, fakeClient, api.DeschedulerStrategy{}, []*v1.Node{node}, false, podEvictor)
		podsEvicted := podEvictor.TotalEvicted()
		if podsEvicted != test.expectedEvictedPodCount {
			t.Errorf("Unexpected no of pods evicted: pods evicted: %d, expected: %d", podsEvicted, test.expectedEvictedPodCount)
		}
	}
}

func setPodAntiAffinity(inputPod *v1.Pod) {
	inputPod.Spec.Affinity = &v1.Affinity{
		PodAntiAffinity: &v1.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
				{
					LabelSelector: &metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "foo",
								Operator: metav1.LabelSelectorOpIn,
								Values:   []string{"bar"},
							},
						},
					},
					TopologyKey: "region",
				},
			},
		},
	}
}
