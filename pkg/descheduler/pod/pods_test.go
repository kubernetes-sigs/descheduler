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
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"sigs.k8s.io/descheduler/test"
)

var (
	lowPriority  = int32(0)
	highPriority = int32(10000)
)

func TestListPodsOnANode(t *testing.T) {
	testCases := []struct {
		name             string
		pods             []*v1.Pod
		node             *v1.Node
		labelSelector    *metav1.LabelSelector
		expectedPodCount int
	}{
		{
			name: "test listing pods on a node",
			pods: []*v1.Pod{
				test.BuildTestPod("pod1", 100, 0, "n1", nil),
				test.BuildTestPod("pod2", 100, 0, "n1", nil),
				test.BuildTestPod("pod3", 100, 0, "n2", nil),
			},
			node:             test.BuildTestNode("n1", 2000, 3000, 10, nil),
			labelSelector:    nil,
			expectedPodCount: 2,
		},
		{
			name: "test listing pods with label selector",
			pods: []*v1.Pod{
				test.BuildTestPod("pod1", 100, 0, "n1", nil),
				test.BuildTestPod("pod2", 100, 0, "n1", func(pod *v1.Pod) {
					pod.Labels = map[string]string{"foo": "bar"}
				}),
				test.BuildTestPod("pod3", 100, 0, "n1", func(pod *v1.Pod) {
					pod.Labels = map[string]string{"foo": "bar1"}
				}),
				test.BuildTestPod("pod4", 100, 0, "n2", nil),
			},
			node: test.BuildTestNode("n1", 2000, 3000, 10, nil),
			labelSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "foo",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"bar", "bar1"},
					},
				},
			},
			expectedPodCount: 2,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			var objs []runtime.Object
			objs = append(objs, testCase.node)
			for _, pod := range testCase.pods {
				objs = append(objs, pod)
			}
			fakeClient := fake.NewSimpleClientset(objs...)

			sharedInformerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
			podInformer := sharedInformerFactory.Core().V1().Pods().Informer()

			getPodsAssignedToNode, err := BuildGetPodsAssignedToNodeFunc(podInformer)
			if err != nil {
				t.Errorf("Build get pods assigned to node function error: %v", err)
			}

			sharedInformerFactory.Start(ctx.Done())
			sharedInformerFactory.WaitForCacheSync(ctx.Done())

			filter, err := NewOptions().WithLabelSelector(testCase.labelSelector).BuildFilterFunc()
			if err != nil {
				t.Errorf("Build filter function error: %v", err)
			}

			pods, _ := ListPodsOnANode(testCase.node.Name, getPodsAssignedToNode, filter)
			if len(pods) != testCase.expectedPodCount {
				t.Errorf("Expected %v pods on node %v, got %+v", testCase.expectedPodCount, testCase.node.Name, len(pods))
			}
		})
	}
}

func getPodListNames(pods []*v1.Pod) []string {
	names := []string{}
	for _, pod := range pods {
		names = append(names, pod.Name)
	}
	return names
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

	p7 := test.BuildTestPod("p7", 400, 0, n1.Name, func(pod *v1.Pod) {
		test.SetPodPriority(pod, lowPriority)
		pod.Annotations = map[string]string{
			"descheduler.alpha.kubernetes.io/prefer-no-eviction": "",
		}
	})

	// BestEffort
	p8 := test.BuildTestPod("p8", 400, 0, n1.Name, func(pod *v1.Pod) {
		test.SetPodPriority(pod, highPriority)
		test.MakeBestEffortPod(pod)
		pod.Annotations = map[string]string{
			"descheduler.alpha.kubernetes.io/prefer-no-eviction": "",
		}
	})

	// Burstable
	p9 := test.BuildTestPod("p9", 400, 0, n1.Name, func(pod *v1.Pod) {
		test.SetPodPriority(pod, highPriority)
		test.MakeBurstablePod(pod)
		pod.Annotations = map[string]string{
			"descheduler.alpha.kubernetes.io/prefer-no-eviction": "",
		}
	})

	// Guaranteed
	p10 := test.BuildTestPod("p10", 400, 100, n1.Name, func(pod *v1.Pod) {
		test.SetPodPriority(pod, highPriority)
		test.MakeGuaranteedPod(pod)
		pod.Annotations = map[string]string{
			"descheduler.alpha.kubernetes.io/prefer-no-eviction": "",
		}
	})

	// Burstable
	p11 := test.BuildTestPod("p11", 400, 0, n1.Name, func(pod *v1.Pod) {
		test.MakeBurstablePod(pod)
	})

	// Burstable
	p12 := test.BuildTestPod("p12", 400, 0, n1.Name, func(pod *v1.Pod) {
		test.MakeBurstablePod(pod)
		pod.Annotations = map[string]string{
			"descheduler.alpha.kubernetes.io/prefer-no-eviction": "",
		}
	})

	podList := []*v1.Pod{p1, p8, p9, p10, p2, p3, p4, p5, p6, p7, p11, p12}
	// p5: no priority, best effort
	// p11: no priority, burstable
	// p6: no priority, guaranteed
	// p1: low priority
	// p7: low priority, prefer-no-eviction
	// p2: high priority, best effort
	// p8: high priority, best effort, prefer-no-eviction
	// p3: high priority, burstable
	// p9: high priority, burstable, prefer-no-eviction
	// p4: high priority, guaranteed
	// p10: high priority, guaranteed, prefer-no-eviction
	expectedPodList := []*v1.Pod{p5, p11, p12, p6, p1, p7, p2, p8, p3, p9, p4, p10}

	SortPodsBasedOnPriorityLowToHigh(podList)
	if !reflect.DeepEqual(getPodListNames(podList), getPodListNames(expectedPodList)) {
		t.Errorf("Pods were sorted in an unexpected order: %v, expected %v", getPodListNames(podList), getPodListNames(expectedPodList))
	}
}

func TestSortPodsBasedOnAge(t *testing.T) {
	podList := make([]*v1.Pod, 9)
	n1 := test.BuildTestNode("n1", 4000, 3000, int64(len(podList)), nil)

	for i := 0; i < len(podList); i++ {
		podList[i] = test.BuildTestPod(fmt.Sprintf("p%d", i), 1, 32, n1.Name, func(pod *v1.Pod) {
			creationTimestamp := metav1.Now().Add(time.Minute * time.Duration(-i))
			pod.ObjectMeta.SetCreationTimestamp(metav1.NewTime(creationTimestamp))
		})
	}

	SortPodsBasedOnAge(podList)

	for i := 0; i < len(podList)-1; i++ {
		if podList[i+1].CreationTimestamp.Before(&podList[i].CreationTimestamp) {
			t.Errorf("Expected pods to be sorted by age but pod at index %d was older than %d", i+1, i)
		}
	}
}

func TestGroupByNodeName(t *testing.T) {
	tests := []struct {
		name   string
		pods   []*v1.Pod
		expMap map[string][]*v1.Pod
	}{
		{
			name:   "list of pods is empty",
			pods:   []*v1.Pod{},
			expMap: map[string][]*v1.Pod{},
		},
		{
			name: "pods are on same node",
			pods: []*v1.Pod{
				{Spec: v1.PodSpec{
					NodeName: "node1",
				}},
				{Spec: v1.PodSpec{
					NodeName: "node1",
				}},
			},
			expMap: map[string][]*v1.Pod{"node1": {
				{Spec: v1.PodSpec{
					NodeName: "node1",
				}},
				{Spec: v1.PodSpec{
					NodeName: "node1",
				}},
			}},
		},
		{
			name: "pods are on different nodes",
			pods: []*v1.Pod{
				{Spec: v1.PodSpec{
					NodeName: "node1",
				}},
				{Spec: v1.PodSpec{
					NodeName: "node2",
				}},
			},
			expMap: map[string][]*v1.Pod{
				"node1": {
					{Spec: v1.PodSpec{
						NodeName: "node1",
					}},
				},
				"node2": {
					{Spec: v1.PodSpec{
						NodeName: "node2",
					}},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resultMap := GroupByNodeName(test.pods)
			if !reflect.DeepEqual(resultMap, test.expMap) {
				t.Errorf("Expected %v node map, got %v", test.expMap, resultMap)
			}
		})
	}
}
