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

package pod

import (
	"context"
	"fmt"
	"strings"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"sigs.k8s.io/descheduler/test"
)

func TestListPodsOnANode(t *testing.T) {
	testCases := []struct {
		name             string
		pods             map[string][]v1.Pod
		node             *v1.Node
		expectedPodCount int
	}{
		{
			name: "test listing pods on a node",
			pods: map[string][]v1.Pod{
				"n1": {
					*test.BuildTestPod("pod1", 100, 0, "n1", nil),
					*test.BuildTestPod("pod2", 100, 0, "n1", nil),
				},
				"n2": {*test.BuildTestPod("pod3", 100, 0, "n2", nil)},
			},
			node:             test.BuildTestNode("n1", 2000, 3000, 10, nil),
			expectedPodCount: 2,
		},
	}
	for _, testCase := range testCases {
		fakeClient := &fake.Clientset{}
		fakeClient.Fake.AddReactor("list", "pods", func(action core.Action) (bool, runtime.Object, error) {
			list := action.(core.ListAction)
			fieldString := list.GetListRestrictions().Fields.String()
			if strings.Contains(fieldString, "n1") {
				return true, &v1.PodList{Items: testCase.pods["n1"]}, nil
			} else if strings.Contains(fieldString, "n2") {
				return true, &v1.PodList{Items: testCase.pods["n2"]}, nil
			}
			return true, nil, fmt.Errorf("Failed to list: %v", list)
		})
		pods, _ := ListPodsOnANode(context.TODO(), fakeClient, testCase.node, nil)
		if len(pods) != testCase.expectedPodCount {
			t.Errorf("expected %v pods on node %v, got %+v", testCase.expectedPodCount, testCase.node.Name, len(pods))
		}
	}
}
