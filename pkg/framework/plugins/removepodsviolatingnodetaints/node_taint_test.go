/*
Copyright 2022 The Kubernetes Authors.

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

package removepodsviolatingnodetaints

import (
	"context"
	"fmt"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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
	nodeName6 = "n6"
	nodeName7 = "n7"
)

func buildTestNode(name string, apply func(*v1.Node)) *v1.Node {
	return test.BuildTestNode(name, 2000, 3000, 10, apply)
}

func buildTestPod(name, nodeName string, apply func(*v1.Pod)) *v1.Pod {
	return test.BuildTestPod(name, 100, 0, nodeName, apply)
}

func createNoScheduleTaint(key, value string, index int) v1.Taint {
	return v1.Taint{
		Key:    "testTaint" + fmt.Sprintf("%v", index),
		Value:  "test" + fmt.Sprintf("%v", index),
		Effect: v1.TaintEffectNoSchedule,
	}
}

func createPreferNoScheduleTaint(key, value string, index int) v1.Taint {
	return v1.Taint{
		Key:    "testTaint" + fmt.Sprintf("%v", index),
		Value:  "test" + fmt.Sprintf("%v", index),
		Effect: v1.TaintEffectPreferNoSchedule,
	}
}

func withTestTaint1(node *v1.Node) {
	node.Spec.Taints = []v1.Taint{
		createNoScheduleTaint("testTaint", "test", 1),
	}
}

func withTestingTaint1(node *v1.Node) {
	node.Spec.Taints = []v1.Taint{
		createNoScheduleTaint("testingTaint", "testing", 1),
	}
}

func withBothTaints1(node *v1.Node) {
	node.Spec.Taints = []v1.Taint{
		createNoScheduleTaint("testTaint", "test", 1),
		createNoScheduleTaint("testingTaint", "testing", 1),
	}
}

func withDatacenterEastLabel(node *v1.Node) {
	node.ObjectMeta.Labels = map[string]string{
		"datacenter": "east",
	}
}

func withUnschedulable(node *v1.Node) {
	node.Spec.Unschedulable = true
}

func withPreferNoScheduleTestTaint1(node *v1.Node) {
	node.Spec.Taints = []v1.Taint{
		createPreferNoScheduleTaint("testTaint", "test", 1),
	}
}

func addTolerationToPod(pod *v1.Pod, key, value string, index int, effect v1.TaintEffect) *v1.Pod {
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}

	pod.Spec.Tolerations = []v1.Toleration{{Key: key + fmt.Sprintf("%v", index), Value: value + fmt.Sprintf("%v", index), Effect: effect}}

	return pod
}

func TestDeletePodsViolatingNodeTaints(t *testing.T) {
	p1 := buildTestPod("p1", nodeName1, func(pod *v1.Pod) {
		test.SetNormalOwnerRef(pod)
		addTolerationToPod(pod, "testTaint", "test", 1, v1.TaintEffectNoSchedule)
	})
	p2 := buildTestPod("p2", nodeName1, func(pod *v1.Pod) {
		test.SetNormalOwnerRef(pod)
	})
	p3 := buildTestPod("p3", nodeName1, func(pod *v1.Pod) {
		test.SetNormalOwnerRef(pod)
		addTolerationToPod(pod, "testTaint", "test", 1, v1.TaintEffectNoSchedule)
	})
	p4 := buildTestPod("p4", nodeName1, func(pod *v1.Pod) {
		test.SetNormalOwnerRef(pod)
		addTolerationToPod(pod, "testTaintX", "testX", 1, v1.TaintEffectNoSchedule)
	})
	p5 := buildTestPod("p5", nodeName1, func(pod *v1.Pod) {
		test.SetNormalOwnerRef(pod)
	})
	p6 := buildTestPod("p6", nodeName1, func(pod *v1.Pod) {
		test.SetNormalOwnerRef(pod)
	})
	p7 := buildTestPod("p7", nodeName2, func(pod *v1.Pod) {
		test.SetNormalOwnerRef(pod)
		pod.Namespace = "kube-system"
		priority := utils.SystemCriticalPriority
		pod.Spec.Priority = &priority
	})
	p8 := buildTestPod("p8", nodeName2, func(pod *v1.Pod) {
		pod.ObjectMeta.OwnerReferences = test.GetDaemonSetOwnerRefList()
	})
	p9 := buildTestPod("p9", nodeName2, func(pod *v1.Pod) {
		test.SetNormalOwnerRef(pod)
		pod.Spec.Volumes = []v1.Volume{
			{
				Name: "sample",
				VolumeSource: v1.VolumeSource{
					HostPath: &v1.HostPathVolumeSource{Path: "somePath"},
					EmptyDir: &v1.EmptyDirVolumeSource{
						SizeLimit: resource.NewQuantity(int64(10), resource.BinarySI),
					},
				},
			},
		}
	})
	p10 := buildTestPod("p10", nodeName2, func(pod *v1.Pod) {
		test.SetNormalOwnerRef(pod)
		pod.Annotations = test.GetMirrorPodAnnotation()
	})
	p11 := buildTestPod("p11", nodeName2, func(pod *v1.Pod) {
		test.SetNormalOwnerRef(pod)
	})
	p12 := buildTestPod("p11", nodeName2, func(pod *v1.Pod) {
		test.SetNormalOwnerRef(pod)
		pod.Spec.NodeSelector = map[string]string{
			"datacenter": "west",
		}
	})

	// The following 4 pods won't get evicted.
	// A Critical Pod.
	// A daemonset.
	// A pod with local storage.
	// A Mirror Pod.

	// node5 has PreferNoSchedule:testTaint1=test1, so the p13 has to have
	// PreferNoSchedule:testTaint0=test0 so the pod is not tolarated
	p13 := buildTestPod("p13", nodeName5, func(pod *v1.Pod) {
		test.SetNormalOwnerRef(pod)
		addTolerationToPod(pod, "testTaint", "test", 0, v1.TaintEffectPreferNoSchedule)
	})

	p14 := buildTestPod("p14", nodeName7, func(pod *v1.Pod) {
		test.SetNormalOwnerRef(pod)
		addTolerationToPod(pod, "testTaint", "test", 1, v1.TaintEffectNoSchedule)
	})

	p15 := buildTestPod("p15", nodeName7, func(pod *v1.Pod) {
		test.SetNormalOwnerRef(pod)
		addTolerationToPod(pod, "testTaint", "test", 1, v1.TaintEffectNoSchedule)
		addTolerationToPod(pod, "testingTaint", "testing", 1, v1.TaintEffectNoSchedule)
	})

	var uint1, uint2 uint = 1, 2

	tests := []struct {
		description                    string
		nodes                          []*v1.Node
		pods                           []*v1.Pod
		evictLocalStoragePods          bool
		evictSystemCriticalPods        bool
		maxPodsToEvictPerNode          *uint
		maxNoOfPodsToEvictPerNamespace *uint
		maxNoOfPodsToEvictTotal        *uint
		expectedEvictedPodCount        uint
		nodeFit                        bool
		includePreferNoSchedule        bool
		excludedTaints                 []string
		includedTaints                 []string
	}{
		{
			description: "Pods not tolerating node taint should be evicted",
			pods:        []*v1.Pod{p1, p2, p3},
			nodes: []*v1.Node{
				buildTestNode(nodeName1, withTestTaint1),
			},
			expectedEvictedPodCount: 1, // p2 gets evicted
		},
		{
			description: "Pods with tolerations but not tolerating node taint should be evicted",
			pods:        []*v1.Pod{p1, p3, p4},
			nodes: []*v1.Node{
				buildTestNode(nodeName1, withTestTaint1),
			},
			expectedEvictedPodCount: 1, // p4 gets evicted
		},
		{
			description: "Only <maxNoOfPodsToEvictTotal> number of Pods not tolerating node taint should be evicted",
			pods:        []*v1.Pod{p1, p5, p6},
			nodes: []*v1.Node{
				buildTestNode(nodeName1, withTestTaint1),
			},
			maxPodsToEvictPerNode:   &uint2,
			maxNoOfPodsToEvictTotal: &uint1,
			expectedEvictedPodCount: 1, // p5 or p6 gets evicted
		},
		{
			description: "Only <maxPodsToEvictPerNode> number of Pods not tolerating node taint should be evicted",
			pods:        []*v1.Pod{p1, p5, p6},
			nodes: []*v1.Node{
				buildTestNode(nodeName1, withTestTaint1),
			},
			maxPodsToEvictPerNode:   &uint1,
			expectedEvictedPodCount: 1, // p5 or p6 gets evicted
		},
		{
			description: "Only <maxNoOfPodsToEvictPerNamespace> number of Pods not tolerating node taint should be evicted",
			pods:        []*v1.Pod{p1, p5, p6},
			nodes: []*v1.Node{
				buildTestNode(nodeName1, withTestTaint1),
			},
			maxNoOfPodsToEvictPerNamespace: &uint1,
			expectedEvictedPodCount:        1, // p5 or p6 gets evicted
		},
		{
			description: "Only <maxNoOfPodsToEvictPerNamespace> number of Pods not tolerating node taint should be evicted",
			pods:        []*v1.Pod{p1, p5, p6},
			nodes: []*v1.Node{
				buildTestNode(nodeName1, withTestTaint1),
			},
			maxNoOfPodsToEvictPerNamespace: &uint1,
			expectedEvictedPodCount:        1, // p5 or p6 gets evicted
		},
		{
			description: "Critical pods not tolerating node taint should not be evicted",
			pods:        []*v1.Pod{p7, p8, p9, p10},
			nodes: []*v1.Node{
				buildTestNode(nodeName2, withTestingTaint1),
			},
			expectedEvictedPodCount: 0, // nothing is evicted
		},
		{
			description: "Critical pods except storage pods not tolerating node taint should not be evicted",
			pods:        []*v1.Pod{p7, p8, p9, p10},
			nodes: []*v1.Node{
				buildTestNode(nodeName2, withTestingTaint1),
			},
			evictLocalStoragePods:   true,
			expectedEvictedPodCount: 1, // p9 gets evicted
		},
		{
			description: "Critical and non critical pods, only non critical pods not tolerating node taint should be evicted",
			pods:        []*v1.Pod{p7, p8, p10, p11},
			nodes: []*v1.Node{
				buildTestNode(nodeName2, withTestingTaint1),
			},
			expectedEvictedPodCount: 1, // p11 gets evicted
		},
		{
			description: "Critical and non critical pods, pods not tolerating node taint should be evicted even if they are critical",
			pods:        []*v1.Pod{p2, p7, p9, p10},
			nodes: []*v1.Node{
				buildTestNode(nodeName1, withTestTaint1),
				buildTestNode(nodeName2, withTestingTaint1),
			},
			evictSystemCriticalPods: true,
			expectedEvictedPodCount: 2, // p2 and p7 are evicted
		},
		{
			description: "Pod p2 doesn't tolerate taint on it's node, but also doesn't tolerate taints on other nodes",
			pods:        []*v1.Pod{p1, p2, p3},
			nodes: []*v1.Node{
				buildTestNode(nodeName1, withTestTaint1),
				buildTestNode(nodeName2, withTestingTaint1),
			},
			expectedEvictedPodCount: 0, // p2 gets evicted
			nodeFit:                 true,
		},
		{
			description: "Pod p12 doesn't tolerate taint on it's node, but other nodes don't match it's selector",
			pods:        []*v1.Pod{p1, p3, p12},
			nodes: []*v1.Node{
				buildTestNode(nodeName1, withTestTaint1),
				buildTestNode(nodeName3, withDatacenterEastLabel),
			},
			expectedEvictedPodCount: 0, // p2 gets evicted
			nodeFit:                 true,
		},
		{
			description: "Pod p2 doesn't tolerate taint on it's node, but other nodes are unschedulable",
			pods:        []*v1.Pod{p1, p2, p3},
			nodes: []*v1.Node{
				buildTestNode(nodeName1, withTestTaint1),
				buildTestNode(nodeName4, withUnschedulable),
			},
			expectedEvictedPodCount: 0, // p2 gets evicted
			nodeFit:                 true,
		},
		{
			description: "Pods not tolerating PreferNoSchedule node taint should not be evicted when not enabled",
			pods:        []*v1.Pod{p13},
			nodes: []*v1.Node{
				buildTestNode(nodeName5, withPreferNoScheduleTestTaint1),
			},
			expectedEvictedPodCount: 0,
		},
		{
			description: "Pods not tolerating PreferNoSchedule node taint should be evicted when enabled",
			pods:        []*v1.Pod{p13},
			nodes: []*v1.Node{
				buildTestNode(nodeName5, withPreferNoScheduleTestTaint1),
			},
			includePreferNoSchedule: true,
			expectedEvictedPodCount: 1, // p13 gets evicted
		},
		{
			description: "Pods not tolerating excluded node taints (by key) should not be evicted",
			pods:        []*v1.Pod{p2},
			nodes: []*v1.Node{
				buildTestNode(nodeName1, withTestTaint1),
			},
			excludedTaints:          []string{"excludedTaint1", "testTaint1"},
			expectedEvictedPodCount: 0, // nothing gets evicted, as one of the specified excludedTaints matches the key of node1's taint
		},
		{
			description: "Pods not tolerating excluded node taints (by key and value) should not be evicted",
			pods:        []*v1.Pod{p2},
			nodes: []*v1.Node{
				buildTestNode(nodeName1, withTestTaint1),
			},
			excludedTaints:          []string{"testTaint1=test1"},
			expectedEvictedPodCount: 0, // nothing gets evicted, as both the key and value of the excluded taint match node1's taint
		},
		{
			description: "The excluded taint matches the key of node1's taint, but does not match the value",
			pods:        []*v1.Pod{p2},
			nodes: []*v1.Node{
				buildTestNode(nodeName1, withTestTaint1),
			},
			excludedTaints:          []string{"testTaint1=test2"},
			expectedEvictedPodCount: 1, // pod gets evicted, as excluded taint value does not match node1's taint value
		},
		{
			description: "Critical and non critical pods, pods not tolerating node taint can't be evicted because the only available node does not have enough resources.",
			pods:        []*v1.Pod{p2, p7, p9, p10},
			nodes: []*v1.Node{
				buildTestNode(nodeName1, withTestTaint1),
				test.BuildTestNode(nodeName6, 1, 1, 1, withPreferNoScheduleTestTaint1),
			},
			evictSystemCriticalPods: true,
			expectedEvictedPodCount: 0, // p2 and p7 can't be evicted
			nodeFit:                 true,
		},
		{
			description: "Pods tolerating included taints should not get evicted even with other taints present",
			pods:        []*v1.Pod{p1},
			nodes: []*v1.Node{
				buildTestNode(nodeName7, withBothTaints1),
			},
			includedTaints:          []string{"testTaint1=test1"},
			expectedEvictedPodCount: 0, // nothing gets evicted, as p1 tolerates the included taint, and taint "testingTaint1=testing1" is not included
		},
		{
			description: "Pods not tolerating not included taints should not get evicted",
			pods:        []*v1.Pod{p1, p2, p4},
			nodes: []*v1.Node{
				buildTestNode(nodeName1, withTestTaint1),
			},
			includedTaints:          []string{"testTaint2=test2"},
			expectedEvictedPodCount: 0, // nothing gets evicted, as taint is not included, even though the pods' p2 and p4 tolerations do not match node1's taint
		},
		{
			description: "Pods tolerating includedTaint should not get evicted. Pods not tolerating includedTaints should get evicted",
			pods:        []*v1.Pod{p1, p2, p3},
			nodes: []*v1.Node{
				buildTestNode(nodeName1, withTestTaint1),
			},
			includedTaints:          []string{"testTaint1=test1"},
			expectedEvictedPodCount: 1, // node1 taint is included. p1 and p3 tolerate the included taint, p2 gets evicted
		},
		{
			description: "Pods not tolerating all taints are evicted when includedTaints is empty",
			pods:        []*v1.Pod{p14, p15},
			nodes: []*v1.Node{
				buildTestNode(nodeName7, withBothTaints1),
			},
			expectedEvictedPodCount: 1, // includedTaints is empty so all taints are included. p15 tolerates both node taints and does not get evicted. p14 tolerate only one and gets evicted
		},
	}

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			var objs []runtime.Object
			for _, node := range tc.nodes {
				objs = append(objs, node)
			}
			for _, pod := range tc.pods {
				objs = append(objs, pod)
			}
			fakeClient := fake.NewSimpleClientset(objs...)

			handle, podEvictor, err := frameworktesting.InitFrameworkHandle(
				ctx,
				fakeClient,
				evictions.NewOptions().
					WithMaxPodsToEvictPerNode(tc.maxPodsToEvictPerNode).
					WithMaxPodsToEvictPerNamespace(tc.maxNoOfPodsToEvictPerNamespace).
					WithMaxPodsToEvictTotal(tc.maxNoOfPodsToEvictTotal),
				defaultevictor.DefaultEvictorArgs{
					EvictLocalStoragePods:   tc.evictLocalStoragePods,
					EvictSystemCriticalPods: tc.evictSystemCriticalPods,
					NodeFit:                 tc.nodeFit,
				},
				nil,
			)
			if err != nil {
				t.Fatalf("Unable to initialize a framework handle: %v", err)
			}

			plugin, err := New(ctx, &RemovePodsViolatingNodeTaintsArgs{
				IncludePreferNoSchedule: tc.includePreferNoSchedule,
				ExcludedTaints:          tc.excludedTaints,
				IncludedTaints:          tc.includedTaints,
			},
				handle,
			)
			if err != nil {
				t.Fatalf("Unable to initialize the plugin: %v", err)
			}

			plugin.(frameworktypes.DeschedulePlugin).Deschedule(ctx, tc.nodes)
			actualEvictedPodCount := podEvictor.TotalEvicted()
			if actualEvictedPodCount != tc.expectedEvictedPodCount {
				t.Errorf("Test %#v failed, Unexpected no of pods evicted: pods evicted: %d, expected: %d", tc.description, actualEvictedPodCount, tc.expectedEvictedPodCount)
			}
		})
	}
}

func TestToleratesTaint(t *testing.T) {
	testCases := []struct {
		description     string
		toleration      v1.Toleration
		taint           v1.Taint
		expectTolerated bool
	}{
		{
			description: "toleration and taint have the same key and effect, and operator is Exists, and taint has no value, expect tolerated",
			toleration: v1.Toleration{
				Key:      "foo",
				Operator: v1.TolerationOpExists,
				Effect:   v1.TaintEffectNoSchedule,
			},
			taint: v1.Taint{
				Key:    "foo",
				Effect: v1.TaintEffectNoSchedule,
			},
			expectTolerated: true,
		},
		{
			description: "toleration and taint have the same key and effect, and operator is Exists, and taint has some value, expect tolerated",
			toleration: v1.Toleration{
				Key:      "foo",
				Operator: v1.TolerationOpExists,
				Effect:   v1.TaintEffectNoSchedule,
			},
			taint: v1.Taint{
				Key:    "foo",
				Value:  "bar",
				Effect: v1.TaintEffectNoSchedule,
			},
			expectTolerated: true,
		},
		{
			description: "toleration and taint have the same effect, toleration has empty key and operator is Exists, means match all taints, expect tolerated",
			toleration: v1.Toleration{
				Key:      "",
				Operator: v1.TolerationOpExists,
				Effect:   v1.TaintEffectNoSchedule,
			},
			taint: v1.Taint{
				Key:    "foo",
				Value:  "bar",
				Effect: v1.TaintEffectNoSchedule,
			},
			expectTolerated: true,
		},
		{
			description: "toleration and taint have the same key, effect and value, and operator is Equal, expect tolerated",
			toleration: v1.Toleration{
				Key:      "foo",
				Operator: v1.TolerationOpEqual,
				Value:    "bar",
				Effect:   v1.TaintEffectNoSchedule,
			},
			taint: v1.Taint{
				Key:    "foo",
				Value:  "bar",
				Effect: v1.TaintEffectNoSchedule,
			},
			expectTolerated: true,
		},
		{
			description: "toleration and taint have the same key and effect, but different values, and operator is Equal, expect not tolerated",
			toleration: v1.Toleration{
				Key:      "foo",
				Operator: v1.TolerationOpEqual,
				Value:    "value1",
				Effect:   v1.TaintEffectNoSchedule,
			},
			taint: v1.Taint{
				Key:    "foo",
				Value:  "value2",
				Effect: v1.TaintEffectNoSchedule,
			},
			expectTolerated: false,
		},
		{
			description: "toleration and taint have the same key and value, but different effects, and operator is Equal, expect not tolerated",
			toleration: v1.Toleration{
				Key:      "foo",
				Operator: v1.TolerationOpEqual,
				Value:    "bar",
				Effect:   v1.TaintEffectNoSchedule,
			},
			taint: v1.Taint{
				Key:    "foo",
				Value:  "bar",
				Effect: v1.TaintEffectNoExecute,
			},
			expectTolerated: false,
		},
	}
	for _, tc := range testCases {
		if tolerated := tc.toleration.ToleratesTaint(&tc.taint); tc.expectTolerated != tolerated {
			t.Errorf("[%s] expect %v, got %v: toleration %+v, taint %s", tc.description, tc.expectTolerated, tolerated, tc.toleration, tc.taint.ToString())
		}
	}
}
