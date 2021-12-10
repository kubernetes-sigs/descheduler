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
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
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
		opts             []func(opts *Options)
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
			opts: []func(opts *Options){
				WithLabelSelector(&metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "foo",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"bar", "bar1"},
						},
					},
				}),
			},
			expectedPodCount: 2,
		},
		{
			name: "test listing pods with includeNamespace",
			pods: []*v1.Pod{
				test.BuildTestPod("pod1", 100, 0, "n1", func(pod *v1.Pod) {
					pod.Namespace = "n1"
				}),
				test.BuildTestPod("pod2", 100, 0, "n1", func(pod *v1.Pod) {
					pod.Namespace = "n2"
				}),
				test.BuildTestPod("pod4", 100, 0, "n2", func(pod *v1.Pod) {
					pod.Namespace = "n1"
				}),
			},
			node: test.BuildTestNode("n1", 2000, 3000, 10, nil),
			opts: []func(opts *Options){
				WithNamespaces([]string{"n1"}),
			},
			expectedPodCount: 1,
		},
		{
			name: "test listing pods with excludeNamespace",
			pods: []*v1.Pod{
				test.BuildTestPod("pod1", 100, 0, "n1", func(pod *v1.Pod) {
					pod.Namespace = "n1"
				}),
				test.BuildTestPod("pod2", 100, 0, "n1", func(pod *v1.Pod) {
					pod.Namespace = "n2"
				}),
				test.BuildTestPod("pod2", 100, 0, "n1", func(pod *v1.Pod) {
					pod.Namespace = "n3"
				}),
				test.BuildTestPod("pod4", 100, 0, "n2", func(pod *v1.Pod) {
					pod.Namespace = "n1"
				}),
			},
			node: test.BuildTestNode("n1", 2000, 3000, 10, nil),
			opts: []func(opts *Options){
				WithoutNamespaces([]string{"n3"}),
				WithRunningPodsFilter(),
			},
			expectedPodCount: 2,
		},
		{
			name: "test listing pods not failed not succeeded",
			pods: []*v1.Pod{
				test.BuildTestPod("pod1", 100, 0, "n1", func(pod *v1.Pod) {
					pod.Status = v1.PodStatus{Phase: v1.PodRunning}
				}),
				test.BuildTestPod("pod2", 100, 0, "n1", func(pod *v1.Pod) {
					pod.Status = v1.PodStatus{Phase: v1.PodFailed}
				}),
				test.BuildTestPod("pod2", 100, 0, "n1", func(pod *v1.Pod) {
					pod.Status = v1.PodStatus{Phase: v1.PodPending}
				}),
				test.BuildTestPod("pod4", 100, 0, "n2", func(pod *v1.Pod) {
					pod.Status = v1.PodStatus{Phase: v1.PodSucceeded}
				}),
			},
			node: test.BuildTestNode("n1", 2000, 3000, 10, nil),
			opts: []func(opts *Options){
				WithRunningPodsFilter(),
			},
			expectedPodCount: 2,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{
				cache.NamespaceIndex: cache.MetaNamespaceIndexFunc,
			})
			for _, pod := range testCase.pods {
				if err := indexer.Add(pod); err != nil {
					t.Fatal(err.Error())
				}
			}

			pods, _ := ListPodsOnANode(
				context.TODO(),
				listersv1.NewPodLister(indexer),
				testCase.node,
				testCase.opts...,
			)
			if len(pods) != testCase.expectedPodCount {
				t.Errorf("expected %v pods on node %v, got %+v", testCase.expectedPodCount, testCase.node.Name, len(pods))
			}
		})
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
