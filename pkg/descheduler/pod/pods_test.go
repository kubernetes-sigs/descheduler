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
	"reflect"
	"strings"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"sigs.k8s.io/descheduler/test"
)

var (
	lowPriority  = int32(0)
	highPriority = int32(10000)
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

func TestGetPodOwnerReplicaSetReplicaCount(t *testing.T) {
	pod := test.BuildTestPod("pod1", 100, 0, "n1", nil)
	replicasetName := "replicaset-1"
	expectedReplicaCount := 3

	rs := test.BuildTestReplicaSet(replicasetName, 3)
	ownerRef1 := test.GetReplicaSetOwnerRefList()
	pod.ObjectMeta.OwnerReferences = ownerRef1

	fakeClient := &fake.Clientset{}
	fakeClient.Fake.AddReactor("get", "replicasets", func(action core.Action) (bool, runtime.Object, error) {
		name := action.(core.GetAction)
		fieldString := name.GetName()
		if strings.Contains(fieldString, replicasetName) {
			return true, rs.DeepCopy(), nil
		}
		return true, nil, fmt.Errorf("Failed to get replicaset: %v", replicasetName)
	})
	actualReplicas, _ := GetPodOwnerReplicationCount(context.TODO(), fakeClient, pod.OwnerReferences[0])
	if actualReplicas != expectedReplicaCount {
		t.Errorf("expected %v replicas for pod owner %v, got %+v", expectedReplicaCount, replicasetName, actualReplicas)
	}
}

func TestGetPodOwnerReplicationControllerReplicaCount(t *testing.T) {
	pod := test.BuildTestPod("pod1", 100, 0, "n1", nil)
	rcOwnerName := "replicationcontroller-1"
	expectedReplicaCount := 3

	rc := test.BuildTestReplicaController(rcOwnerName, 3)
	ownerRef1 := test.GetReplicationControllerOwnerRefList()
	pod.ObjectMeta.OwnerReferences = ownerRef1

	fakeClient := &fake.Clientset{}
	fakeClient.Fake.AddReactor("get", "replicationcontrollers", func(action core.Action) (bool, runtime.Object, error) {
		name := action.(core.GetAction)
		fieldString := name.GetName()
		if strings.Contains(fieldString, rcOwnerName) {
			return true, rc.DeepCopy(), nil
		}
		return true, nil, fmt.Errorf("Failed to get replication controller: %v", rcOwnerName)
	})
	actualReplicas, _ := GetPodOwnerReplicationCount(context.TODO(), fakeClient, pod.OwnerReferences[0])
	if actualReplicas != expectedReplicaCount {
		t.Errorf("expected %v replicas for pod owner %v, got %+v", expectedReplicaCount, rcOwnerName, actualReplicas)
	}
}

func TestSortPodsBasedOnPriorityLowToHigh(t *testing.T) {
	n1 := test.BuildTestNode("n1", 4000, 3000, 9, nil)

	p1 := test.BuildTestPod("p1", 400, 0, n1.Name, func(pod *v1.Pod) {
		test.SetPodPriority(pod, lowPriority)
	})

	// BestEffort
	p2 := test.BuildTestPod("p2", 400, 0, n1.Name, func(pod *v1.Pod) {
		test.SetPodPriority(pod, highPriority)
		test.MakeBestEffortPod(pod)
	})

	// Burstable
	p3 := test.BuildTestPod("p3", 400, 0, n1.Name, func(pod *v1.Pod) {
		test.SetPodPriority(pod, highPriority)
		test.MakeBurstablePod(pod)
	})

	// Guaranteed
	p4 := test.BuildTestPod("p4", 400, 100, n1.Name, func(pod *v1.Pod) {
		test.SetPodPriority(pod, highPriority)
		test.MakeGuaranteedPod(pod)
	})

	// Best effort with nil priorities.
	p5 := test.BuildTestPod("p5", 400, 100, n1.Name, test.MakeBestEffortPod)
	p5.Spec.Priority = nil

	p6 := test.BuildTestPod("p6", 400, 100, n1.Name, test.MakeGuaranteedPod)
	p6.Spec.Priority = nil

	podList := []*v1.Pod{p4, p3, p2, p1, p6, p5}

	SortPodsBasedOnPriorityLowToHigh(podList)
	if !reflect.DeepEqual(podList[len(podList)-1], p4) {
		t.Errorf("Expected last pod in sorted list to be %v which of highest priority and guaranteed but got %v", p4, podList[len(podList)-1])
	}
}
