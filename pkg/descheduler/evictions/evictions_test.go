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
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"sigs.k8s.io/descheduler/test"
	"testing"
)

func TestEvictPod(t *testing.T) {
	n1 := test.BuildTestNode("node1", 1000, 2000, 9)
	p1 := test.BuildTestPod("p1", 400, 0, n1.Name)
	fakeClient := &fake.Clientset{}
	fakeClient.Fake.AddReactor("list", "pods", func(action core.Action) (bool, runtime.Object, error) {
		return true, &v1.PodList{Items: []v1.Pod{*p1}}, nil
	})
	evicted, _ := EvictPod(fakeClient, p1, "v1", false)
	if !evicted {
		t.Errorf("Expected %v pod to be evicted", p1.Name)
	}

}
