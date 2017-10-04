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

	"fmt"
	"github.com/kubernetes-incubator/descheduler/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	core "k8s.io/client-go/testing"
	"k8s.io/kubernetes/pkg/api/v1"
	"k8s.io/kubernetes/pkg/client/clientset_generated/clientset/fake"
)

func TestPodAntiAffinity(t *testing.T) {
	node := test.BuildTestNode("n1", 2000, 3000, 10)
	p1 := test.BuildTestPod("p1", 100, 0, node.Name)
	p2 := test.BuildTestPod("p2", 100, 0, node.Name)
	p3 := test.BuildTestPod("p3", 100, 0, node.Name)
	p3.Labels = map[string]string{"foo": "bar"}
	p1.Spec.Affinity = &v1.Affinity{
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
	p3.Spec.Affinity = &v1.Affinity{
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
	fakeClient := &fake.Clientset{}
	fakeClient.Fake.AddReactor("list", "pods", func(action core.Action) (bool, runtime.Object, error) {
		return true, &v1.PodList{Items: []v1.Pod{*p1, *p2, *p3}}, nil
	})
	fakeClient.Fake.AddReactor("get", "nodes", func(action core.Action) (bool, runtime.Object, error) {
		return true, node, nil
	})
	expectedEvictedPodCount := 1
	podsEvicted := removePodsWithAffinityRules(fakeClient, "v1", []*v1.Node{node}, false)
	if podsEvicted != expectedEvictedPodCount {
		fmt.Println(podsEvicted)
		t.Errorf("Unexpected no of pods evicted")
	}
}
