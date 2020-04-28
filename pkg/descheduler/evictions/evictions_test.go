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

package evictions

import (
	"testing"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"sigs.k8s.io/descheduler/test"
)

func TestEvictPod(t *testing.T) {
	node1 := test.BuildTestNode("node1", 1000, 2000, 9, nil)
	pod1 := test.BuildTestPod("p1", 400, 0, "node1", nil)
	tests := []struct {
		description string
		node        *v1.Node
		pod         *v1.Pod
		pods        []v1.Pod
		want        bool
	}{
		{
			description: "test pod eviction - pod present",
			node:        node1,
			pod:         pod1,
			pods:        []v1.Pod{*pod1},
			want:        true,
		},
		{
			description: "test pod eviction - pod absent",
			node:        node1,
			pod:         pod1,
			pods:        []v1.Pod{*test.BuildTestPod("p2", 400, 0, "node1", nil), *test.BuildTestPod("p3", 450, 0, "node1", nil)},
			want:        true,
		},
	}

	for _, test := range tests {
		fakeClient := &fake.Clientset{}
		fakeClient.Fake.AddReactor("list", "pods", func(action core.Action) (bool, runtime.Object, error) {
			return true, &v1.PodList{Items: test.pods}, nil
		})
		got, _ := EvictPod(fakeClient, test.pod, "v1", false)
		if got != test.want {
			t.Errorf("Test error for Desc: %s. Expected %v pod eviction to be %v, got %v", test.description, test.pod.Name, test.want, got)
		}
	}
}
