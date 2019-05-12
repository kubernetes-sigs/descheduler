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
	"github.com/kubernetes-incubator/descheduler/test"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"testing"
)

func TestEvictPod(t *testing.T) {
	tests := []struct {
		description    string
		node           *v1.Node
		pod            *v1.Pod
		fakeClientPods []v1.Pod
		success        bool
	}{
		{
			description:    "test pod eviction when pod is present",
			node:           test.BuildTestNode("node1", 1000, 2000, 9),
			pod:            test.BuildTestPod("p1", 400, 0, "node1"),
			fakeClientPods: []v1.Pod{*pod},
			success:        true,
		},
		{
			description:    "test pod eviction when pod is not present",
			node:           test.BuildTestNode("node1", 1000, 2000, 9),
			pod:            test.BuildTestPod("p1", 400, 0, "node1"),
			fakeClientPods: []v1.Pod{},
			success:        true,
		},
	}

	for _, test := range tests {
		fakeClient := &fake.Clientset{}
		fakeClient.Fake.AddReactor("list", "pods", func(action core.Action) (bool, runtime.Object, error) {
			return true, &v1.PodList{Items: test.fakeClientPods}, nil
		})
		evicted, _ := EvictPod(fakeClient, test.pod, "v1", false)
		if evicted != test.success {
			t.Errorf("Expected %v pod eviction to be %v ", test.pod.Name, test.success)
		}
	}
}
