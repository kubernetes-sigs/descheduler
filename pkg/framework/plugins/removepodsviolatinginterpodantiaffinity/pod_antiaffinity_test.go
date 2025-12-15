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

package removepodsviolatinginterpodantiaffinity

import (
	"context"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"

	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	frameworktesting "sigs.k8s.io/descheduler/pkg/framework/testing"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
	"sigs.k8s.io/descheduler/pkg/utils"
	"sigs.k8s.io/descheduler/test"
)

const (
	nodeName1 = "n1"
	nodeName2 = "n2"
	nodeName3 = "n3"
	nodeName4 = "n4"
	nodeName5 = "n5"
)

func buildTestNode(name string, apply func(*v1.Node)) *v1.Node {
	return test.BuildTestNode(name, 2000, 3000, 10, apply)
}

func setNodeMainRegionLabel(node *v1.Node) {
	node.ObjectMeta.Labels = map[string]string{
		"region": "main-region",
	}
}

func buildTestNode1() *v1.Node {
	return buildTestNode(nodeName1, setNodeMainRegionLabel)
}

func buildTestPod(name, nodeName string, apply func(*v1.Pod)) *v1.Pod {
	return test.BuildTestPod(name, 100, 0, nodeName, apply)
}

func buildTestPodForNode1(name string, apply func(*v1.Pod)) *v1.Pod {
	return buildTestPod(name, nodeName1, apply)
}

func TestPodAntiAffinity(t *testing.T) {

	p1 := buildTestPodForNode1("p1", func(pod *v1.Pod) {
		test.SetNormalOwnerRef(pod)
		test.SetPodAntiAffinity(pod, "foo", "bar")
	})
	p2 := buildTestPodForNode1("p2", func(pod *v1.Pod) {
		test.SetNormalOwnerRef(pod)
		pod.Labels = map[string]string{"foo": "bar"}
	})
	p3 := buildTestPodForNode1("p3", func(pod *v1.Pod) {
		test.SetNormalOwnerRef(pod)
		test.SetPodAntiAffinity(pod, "foo", "bar")
	})
	p4 := buildTestPodForNode1("p4", func(pod *v1.Pod) {
		test.SetNormalOwnerRef(pod)
		test.SetPodAntiAffinity(pod, "foo", "bar")
	})
	p5 := buildTestPodForNode1("p5", func(pod *v1.Pod) {
		test.SetNormalOwnerRef(pod)
		pod.Labels = map[string]string{"foo": "bar"}
		test.SetPodAntiAffinity(pod, "foo1", "bar1")
		test.SetPodPriority(pod, 100)
	})
	p6 := buildTestPodForNode1("p6", func(pod *v1.Pod) {
		test.SetNormalOwnerRef(pod)
		pod.Labels = map[string]string{"foo": "bar"}
		test.SetPodAntiAffinity(pod, "foo1", "bar1")
		test.SetPodPriority(pod, 50)
	})
	p7 := buildTestPodForNode1("p7", func(pod *v1.Pod) {
		test.SetNormalOwnerRef(pod)
		pod.Labels = map[string]string{"foo1": "bar1"}
		test.SetPodAntiAffinity(pod, "foo", "bar")
		test.SetPodPriority(pod, 0)
	})
	p8 := buildTestPodForNode1("p8", func(pod *v1.Pod) {
		pod.Spec.NodeSelector = map[string]string{
			"datacenter": "west",
		}
	})
	p9 := buildTestPodForNode1("p9", nil)
	p10 := buildTestPodForNode1("p10", nil)
	p11 := buildTestPod("p11", nodeName5, nil)
	p9.DeletionTimestamp = &metav1.Time{}
	p10.DeletionTimestamp = &metav1.Time{}

	criticalPriority := utils.SystemCriticalPriority
	nonEvictablePod := buildTestPodForNode1("non-evict", func(pod *v1.Pod) {
		pod.Spec.Priority = &criticalPriority
	})
	p11.Labels = map[string]string{"foo": "bar"}
	nonEvictablePod.Labels = map[string]string{"foo": "bar"}
	test.SetNormalOwnerRef(p9)
	test.SetNormalOwnerRef(p10)
	test.SetNormalOwnerRef(p11)

	// set pod anti affinity
	test.SetPodAntiAffinity(p9, "foo", "bar")
	test.SetPodAntiAffinity(p10, "foo", "bar")

	var uint1 uint = 1
	var uint3 uint = 3

	tests := []struct {
		description                    string
		maxPodsToEvictPerNode          *uint
		maxNoOfPodsToEvictPerNamespace *uint
		maxNoOfPodsToEvictTotal        *uint
		pods                           []*v1.Pod
		expectedEvictedPodCount        uint
		nodeFit                        bool
		nodes                          []*v1.Node
	}{
		{
			description: "Maximum pods to evict - 0",
			pods:        []*v1.Pod{p1, p2, p3, p4},
			nodes: []*v1.Node{
				buildTestNode1(),
			},
			expectedEvictedPodCount: 3,
		},
		{
			description:           "Maximum pods to evict - 3",
			maxPodsToEvictPerNode: &uint3,
			pods:                  []*v1.Pod{p1, p2, p3, p4},
			nodes: []*v1.Node{
				buildTestNode1(),
			},
			expectedEvictedPodCount: 3,
		},
		{
			description:                    "Maximum pods to evict (maxPodsToEvictPerNamespace=3) - 3",
			maxNoOfPodsToEvictPerNamespace: &uint3,
			pods:                           []*v1.Pod{p1, p2, p3, p4},
			nodes: []*v1.Node{
				buildTestNode1(),
			},
			expectedEvictedPodCount: 3,
		},
		{
			description:                    "Maximum pods to evict (maxNoOfPodsToEvictTotal)",
			maxNoOfPodsToEvictPerNamespace: &uint3,
			maxNoOfPodsToEvictTotal:        &uint1,
			pods:                           []*v1.Pod{p1, p2, p3, p4},
			nodes: []*v1.Node{
				buildTestNode1(),
			},
			expectedEvictedPodCount: 1,
		},
		{
			description: "Evict only 1 pod after sorting",
			pods:        []*v1.Pod{p5, p6, p7},
			nodes: []*v1.Node{
				buildTestNode1(),
			},
			expectedEvictedPodCount: 1,
		},
		{
			description:           "Evicts pod that conflicts with critical pod (but does not evict critical pod)",
			maxPodsToEvictPerNode: &uint1,
			pods:                  []*v1.Pod{p1, nonEvictablePod},
			nodes: []*v1.Node{
				buildTestNode1(),
			},
			expectedEvictedPodCount: 1,
		},
		{
			description:           "Evicts pod that conflicts with critical pod (but does not evict critical pod)",
			maxPodsToEvictPerNode: &uint1,
			pods:                  []*v1.Pod{p1, nonEvictablePod},
			nodes: []*v1.Node{
				buildTestNode1(),
			},
			expectedEvictedPodCount: 1,
		},
		{
			description:           "Won't evict pods because node selectors don't match available nodes",
			maxPodsToEvictPerNode: &uint1,
			pods:                  []*v1.Pod{p8, nonEvictablePod},
			nodes: []*v1.Node{
				buildTestNode1(),
				buildTestNode(nodeName2, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						"datacenter": "east",
					}
				}),
			},
			expectedEvictedPodCount: 0,
			nodeFit:                 true,
		},
		{
			description:           "Won't evict pods because only other node is not schedulable",
			maxPodsToEvictPerNode: &uint1,
			pods:                  []*v1.Pod{p8, nonEvictablePod},
			nodes: []*v1.Node{
				buildTestNode1(),
				buildTestNode(nodeName3, func(node *v1.Node) {
					node.Spec = v1.NodeSpec{
						Unschedulable: true,
					}
				}),
			},
			expectedEvictedPodCount: 0,
			nodeFit:                 true,
		},
		{
			description: "No pod to evicted since all pod terminating",
			pods:        []*v1.Pod{p9, p10},
			nodes: []*v1.Node{
				buildTestNode1(),
			},
			expectedEvictedPodCount: 0,
		},
		{
			description:           "Won't evict pods because only other node doesn't have enough resources",
			maxPodsToEvictPerNode: &uint3,
			pods:                  []*v1.Pod{p1, p2, p3, p4},
			nodes: []*v1.Node{
				buildTestNode1(),
				test.BuildTestNode(nodeName4, 2, 2, 1, nil),
			},
			expectedEvictedPodCount: 0,
			nodeFit:                 true,
		},
		{
			description: "Evict pod violating anti-affinity among different node (all pods have anti-affinity)",
			pods:        []*v1.Pod{p1, p11},
			nodes: []*v1.Node{
				buildTestNode1(),
				test.BuildTestNode(nodeName5, 200, 3000, 10, setNodeMainRegionLabel),
			},
			expectedEvictedPodCount: 1,
			nodeFit:                 false,
		},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			var objs []runtime.Object
			for _, node := range test.nodes {
				objs = append(objs, node)
			}
			for _, pod := range test.pods {
				objs = append(objs, pod)
			}
			fakeClient := fake.NewSimpleClientset(objs...)

			handle, podEvictor, err := frameworktesting.InitFrameworkHandle(
				ctx,
				fakeClient,
				evictions.NewOptions().
					WithMaxPodsToEvictPerNode(test.maxPodsToEvictPerNode).
					WithMaxPodsToEvictPerNamespace(test.maxNoOfPodsToEvictPerNamespace).
					WithMaxPodsToEvictTotal(test.maxNoOfPodsToEvictTotal),
				defaultevictor.DefaultEvictorArgs{NodeFit: test.nodeFit},
				nil,
			)
			if err != nil {
				t.Fatalf("Unable to initialize a framework handle: %v", err)
			}

			plugin, err := New(
				ctx,
				&RemovePodsViolatingInterPodAntiAffinityArgs{},
				handle,
			)

			plugin.(frameworktypes.DeschedulePlugin).Deschedule(ctx, test.nodes)
			podsEvicted := podEvictor.TotalEvicted()
			if podsEvicted != test.expectedEvictedPodCount {
				t.Errorf("Unexpected no of pods evicted: pods evicted: %d, expected: %d", podsEvicted, test.expectedEvictedPodCount)
			}
		})
	}
}
