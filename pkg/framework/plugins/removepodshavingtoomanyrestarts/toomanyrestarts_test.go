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

package removepodshavingtoomanyrestarts

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
	"sigs.k8s.io/descheduler/test"
)

const (
	nodeName1 = "node1"
	nodeName2 = "node2"
	nodeName3 = "node3"
	nodeName4 = "node4"
	nodeName5 = "node5"
)

func buildTestNode(nodeName string, apply func(*v1.Node)) *v1.Node {
	return test.BuildTestNode(nodeName, 2000, 3000, 10, apply)
}

func setPodContainerStatusRestartCount(pod *v1.Pod, base int32) {
	pod.Status = v1.PodStatus{
		InitContainerStatuses: []v1.ContainerStatus{
			{
				RestartCount: 5 * base,
			},
		},
		ContainerStatuses: []v1.ContainerStatus{
			{
				RestartCount: 10 * base,
			},
			{
				RestartCount: 10 * base,
			},
		},
	}
}

func initPodContainersWithStatusRestartCount(name string, base int32, apply func(pod *v1.Pod)) *v1.Pod {
	return test.BuildTestPod(name, 100, 0, nodeName1, func(pod *v1.Pod) {
		test.SetNormalOwnerRef(pod)
		// pod at index i will have 25 * i restarts, 5 for init container, 20 for other two containers
		setPodContainerStatusRestartCount(pod, base)
		if apply != nil {
			apply(pod)
		}
	})
}

func initPods(apply func(pod *v1.Pod)) []*v1.Pod {
	pods := make([]*v1.Pod, 0)

	for i := int32(0); i <= 9; i++ {
		switch i {
		default:
			pods = append(pods, initPodContainersWithStatusRestartCount(fmt.Sprintf("pod-%d", i), i, apply))
		// The following 3 pods won't get evicted.
		// A daemonset.
		case 6:
			pods = append(pods, initPodContainersWithStatusRestartCount(fmt.Sprintf("pod-%d", i), i, func(pod *v1.Pod) {
				test.SetDSOwnerRef(pod)
				if apply != nil {
					apply(pod)
				}
			}))
		// A pod with local storage.
		case 7:
			pods = append(pods, initPodContainersWithStatusRestartCount(fmt.Sprintf("pod-%d", i), i, func(pod *v1.Pod) {
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
				if apply != nil {
					apply(pod)
				}
			}))
		// A Mirror Pod.
		case 8:
			pods = append(pods, initPodContainersWithStatusRestartCount(fmt.Sprintf("pod-%d", i), i, func(pod *v1.Pod) {
				pod.Annotations = test.GetMirrorPodAnnotation()
				if apply != nil {
					apply(pod)
				}
			}))
		}
	}

	pods = append(
		pods,
		test.BuildTestPod("CPU-consumer-1", 150, 100, nodeName4, test.SetNormalOwnerRef),
		test.BuildTestPod("CPU-consumer-2", 150, 100, nodeName5, test.SetNormalOwnerRef),
	)

	return pods
}

func TestRemovePodsHavingTooManyRestarts(t *testing.T) {
	createRemovePodsHavingTooManyRestartsAgrs := func(
		podRestartThresholds int32,
		includingInitContainers bool,
	) RemovePodsHavingTooManyRestartsArgs {
		return RemovePodsHavingTooManyRestartsArgs{
			PodRestartThreshold:     podRestartThresholds,
			IncludingInitContainers: includingInitContainers,
		}
	}

	var uint3 uint = 3

	tests := []struct {
		description                    string
		pods                           []*v1.Pod
		nodes                          []*v1.Node
		args                           RemovePodsHavingTooManyRestartsArgs
		expectedEvictedPodCount        uint
		maxPodsToEvictPerNode          *uint
		maxNoOfPodsToEvictPerNamespace *uint
		nodeFit                        bool
	}{
		{
			description: "All pods have total restarts under threshold, no pod evictions",
			args:        createRemovePodsHavingTooManyRestartsAgrs(10000, true),
			pods:        initPods(nil),
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
			},
			expectedEvictedPodCount: 0,
		},
		{
			description: "Some pods have total restarts bigger than threshold",
			args:        createRemovePodsHavingTooManyRestartsAgrs(1, true),
			pods:        initPods(nil),
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
			},
			expectedEvictedPodCount: 6,
		},
		{
			description: "Nine pods have total restarts equals threshold(includingInitContainers=true), 6 pod evictions",
			args:        createRemovePodsHavingTooManyRestartsAgrs(1*25, true),
			pods:        initPods(nil),
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
			},
			expectedEvictedPodCount: 6,
		},
		{
			description: "Nine pods have total restarts equals threshold(includingInitContainers=false), 5 pod evictions",
			args:        createRemovePodsHavingTooManyRestartsAgrs(1*25, false),
			pods:        initPods(nil),
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
			},
			expectedEvictedPodCount: 5,
		},
		{
			description: "All pods have total restarts equals threshold(includingInitContainers=true), 6 pod evictions",
			args:        createRemovePodsHavingTooManyRestartsAgrs(1*20, true),
			pods:        initPods(nil),
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
			},
			expectedEvictedPodCount: 6,
		},
		{
			description: "Nine pods have total restarts equals threshold(includingInitContainers=false), 6 pod evictions",
			args:        createRemovePodsHavingTooManyRestartsAgrs(1*20, false),
			pods:        initPods(nil),
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
			},
			expectedEvictedPodCount: 6,
		},
		{
			description: "Five pods have total restarts bigger than threshold(includingInitContainers=true), but only 1 pod eviction",
			args:        createRemovePodsHavingTooManyRestartsAgrs(5*25+1, true),
			pods:        initPods(nil),
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
			},
			expectedEvictedPodCount: 1,
		},
		{
			description: "Five pods have total restarts bigger than threshold(includingInitContainers=false), but only 1 pod eviction",
			args:        createRemovePodsHavingTooManyRestartsAgrs(5*20+1, false),
			pods:        initPods(nil),
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
			},
			expectedEvictedPodCount: 1,
			nodeFit:                 false,
		},
		{
			description: "All pods have total restarts equals threshold(maxPodsToEvictPerNode=3), 3 pod evictions",
			args:        createRemovePodsHavingTooManyRestartsAgrs(1, true),
			pods:        initPods(nil),
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
			},
			expectedEvictedPodCount: 3,
			maxPodsToEvictPerNode:   &uint3,
		},
		{
			description: "All pods have total restarts equals threshold(maxNoOfPodsToEvictPerNamespace=3), 3 pod evictions",
			args:        createRemovePodsHavingTooManyRestartsAgrs(1, true),
			pods:        initPods(nil),
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
			},
			expectedEvictedPodCount:        3,
			maxNoOfPodsToEvictPerNamespace: &uint3,
		},
		{
			description: "All pods have total restarts equals threshold(maxPodsToEvictPerNode=3) but the only other node is tainted, 0 pod evictions",
			args:        createRemovePodsHavingTooManyRestartsAgrs(1, true),
			pods:        initPods(nil),
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
				buildTestNode(nodeName2, func(node *v1.Node) {
					node.Spec.Taints = []v1.Taint{
						{
							Key:    "hardware",
							Value:  "gpu",
							Effect: v1.TaintEffectNoSchedule,
						},
					}
				}),
			},
			expectedEvictedPodCount: 0,
			maxPodsToEvictPerNode:   &uint3,
			nodeFit:                 true,
		},
		{
			description: "All pods have total restarts equals threshold(maxPodsToEvictPerNode=3) but the only other node is not schedulable, 0 pod evictions",
			args:        createRemovePodsHavingTooManyRestartsAgrs(1, true),
			pods:        initPods(nil),
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
				buildTestNode(nodeName3, func(node *v1.Node) {
					node.Spec = v1.NodeSpec{
						Unschedulable: true,
					}
				}),
			},
			expectedEvictedPodCount: 0,
			maxPodsToEvictPerNode:   &uint3,
			nodeFit:                 true,
		},
		{
			description: "All pods have total restarts equals threshold(maxPodsToEvictPerNode=3) but the only other node does not have enough CPU, 0 pod evictions",
			args:        createRemovePodsHavingTooManyRestartsAgrs(1, true),
			pods:        initPods(nil),
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
				test.BuildTestNode(nodeName4, 200, 3000, 10, nil),
			},
			expectedEvictedPodCount: 0,
			maxPodsToEvictPerNode:   &uint3,
			nodeFit:                 true,
		},
		{
			description: "All pods have total restarts equals threshold(maxPodsToEvictPerNode=3) but the only other node has enough CPU, 3 pod evictions",
			args:        createRemovePodsHavingTooManyRestartsAgrs(1, true),
			pods:        initPods(nil),
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
				buildTestNode(nodeName5, nil),
			},
			expectedEvictedPodCount: 3,
			maxPodsToEvictPerNode:   &uint3,
			nodeFit:                 true,
		},
		{
			description: "pods are in CrashLoopBackOff with states=CrashLoopBackOff, 3 pod evictions",
			args:        RemovePodsHavingTooManyRestartsArgs{PodRestartThreshold: 1, States: []string{"CrashLoopBackOff"}},
			pods: initPods(func(pod *v1.Pod) {
				if len(pod.Status.ContainerStatuses) > 0 {
					pod.Status.ContainerStatuses[0].State = v1.ContainerState{
						Waiting: &v1.ContainerStateWaiting{Reason: "CrashLoopBackOff"},
					}
				}
			}),
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
				buildTestNode(nodeName5, nil),
			},
			expectedEvictedPodCount: 3,
			maxPodsToEvictPerNode:   &uint3,
			nodeFit:                 true,
		},
		{
			description: "pods without CrashLoopBackOff with states=CrashLoopBackOff, 0 pod evictions",
			args:        RemovePodsHavingTooManyRestartsArgs{PodRestartThreshold: 1, States: []string{"CrashLoopBackOff"}},
			pods:        initPods(nil),
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
				buildTestNode(nodeName5, nil),
			},
			expectedEvictedPodCount: 0,
			maxPodsToEvictPerNode:   &uint3,
			nodeFit:                 true,
		},
		{
			description: "pods running with state=Running, 3 pod evictions",
			args:        RemovePodsHavingTooManyRestartsArgs{PodRestartThreshold: 1, States: []string{string(v1.PodRunning)}},
			pods: initPods(func(pod *v1.Pod) {
				pod.Status.Phase = v1.PodRunning
			}),
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
			},
			expectedEvictedPodCount: 3,
			maxPodsToEvictPerNode:   &uint3,
		},
		{
			description: "pods pending with state=Running, 0 pod evictions",
			args:        RemovePodsHavingTooManyRestartsArgs{PodRestartThreshold: 1, States: []string{string(v1.PodRunning)}},
			pods: initPods(func(pod *v1.Pod) {
				pod.Status.Phase = v1.PodPending
			}),
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
			},
			expectedEvictedPodCount: 0,
			maxPodsToEvictPerNode:   &uint3,
		},
		{
			description: "pods pending with initContainer with states=CrashLoopBackOff threshold(includingInitContainers=true), 3 pod evictions",
			args:        RemovePodsHavingTooManyRestartsArgs{PodRestartThreshold: 1, States: []string{"CrashLoopBackOff"}, IncludingInitContainers: true},
			pods: initPods(func(pod *v1.Pod) {
				pod.Status.InitContainerStatuses = []v1.ContainerStatus{
					{
						State: v1.ContainerState{
							Waiting: &v1.ContainerStateWaiting{Reason: "CrashLoopBackOff"},
						},
					},
				}
			}),
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
			},
			expectedEvictedPodCount: 3,
			maxPodsToEvictPerNode:   &uint3,
		},
		{
			description: "pods pending with initContainer with states=CrashLoopBackOff threshold(includingInitContainers=false), 0 pod evictions",
			args:        RemovePodsHavingTooManyRestartsArgs{PodRestartThreshold: 1, States: []string{"CrashLoopBackOff"}, IncludingInitContainers: false},
			pods: initPods(func(pod *v1.Pod) {
				pod.Status.InitContainerStatuses = []v1.ContainerStatus{
					{
						State: v1.ContainerState{
							Waiting: &v1.ContainerStateWaiting{Reason: "CrashLoopBackOff"},
						},
					},
				}
			}),
			nodes: []*v1.Node{
				buildTestNode(nodeName1, nil),
			},
			expectedEvictedPodCount: 0,
			maxPodsToEvictPerNode:   &uint3,
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
					WithMaxPodsToEvictPerNamespace(tc.maxNoOfPodsToEvictPerNamespace),
				defaultevictor.DefaultEvictorArgs{NodeFit: tc.nodeFit},
				nil,
			)
			if err != nil {
				t.Fatalf("Unable to initialize a framework handle: %v", err)
			}

			plugin, err := New(
				ctx,
				&tc.args,
				handle)
			if err != nil {
				t.Fatalf("Unable to initialize the plugin: %v", err)
			}

			plugin.(frameworktypes.DeschedulePlugin).Deschedule(ctx, tc.nodes)
			actualEvictedPodCount := podEvictor.TotalEvicted()
			if actualEvictedPodCount != tc.expectedEvictedPodCount {
				t.Errorf("Test %#v failed, expected %v pod evictions, but got %v pod evictions\n", tc.description, tc.expectedEvictedPodCount, actualEvictedPodCount)
			}
		})
	}
}
