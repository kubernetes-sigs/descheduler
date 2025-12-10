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

package podlifetime

import (
	"context"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	utilptr "k8s.io/utils/ptr"

	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	frameworktesting "sigs.k8s.io/descheduler/pkg/framework/testing"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
	"sigs.k8s.io/descheduler/test"
)

func TestPodLifeTime(t *testing.T) {
	const nodeName1 = "n1"
	buildTestNode1 := func() *v1.Node {
		return test.BuildTestNode(nodeName1, 2000, 3000, 10, nil)
	}

	olderPodCreationTime := metav1.NewTime(time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC))
	newerPodCreationTime := metav1.NewTime(time.Now())

	// Setup pods, one should be evicted
	p1 := test.BuildTestPod("p1", 100, 0, nodeName1, func(pod *v1.Pod) {
		pod.Namespace = "dev"
		pod.ObjectMeta.CreationTimestamp = newerPodCreationTime
		test.SetRSOwnerRef(pod)
	})
	p2 := test.BuildTestPod("p2", 100, 0, nodeName1, func(pod *v1.Pod) {
		pod.Namespace = "dev"
		pod.ObjectMeta.CreationTimestamp = olderPodCreationTime
		test.SetRSOwnerRef(pod)
	})

	// Setup pods, zero should be evicted
	p3 := test.BuildTestPod("p3", 100, 0, nodeName1, func(pod *v1.Pod) {
		pod.Namespace = "dev"
		pod.ObjectMeta.CreationTimestamp = newerPodCreationTime
		test.SetRSOwnerRef(pod)
	})
	p4 := test.BuildTestPod("p4", 100, 0, nodeName1, func(pod *v1.Pod) {
		pod.Namespace = "dev"
		pod.ObjectMeta.CreationTimestamp = newerPodCreationTime
		test.SetRSOwnerRef(pod)
	})

	// Setup pods, one should be evicted
	p5 := test.BuildTestPod("p5", 100, 0, nodeName1, func(pod *v1.Pod) {
		pod.Namespace = "dev"
		pod.ObjectMeta.CreationTimestamp = newerPodCreationTime
		test.SetRSOwnerRef(pod)
	})
	p6 := test.BuildTestPod("p6", 100, 0, nodeName1, func(pod *v1.Pod) {
		pod.Namespace = "dev"
		pod.ObjectMeta.CreationTimestamp = metav1.NewTime(time.Now().Add(-time.Second * 605))
		test.SetRSOwnerRef(pod)
	})

	// Setup pods, zero should be evicted
	p7 := test.BuildTestPod("p7", 100, 0, nodeName1, func(pod *v1.Pod) {
		pod.Namespace = "dev"
		pod.ObjectMeta.CreationTimestamp = newerPodCreationTime
	})
	p8 := test.BuildTestPod("p8", 100, 0, nodeName1, func(pod *v1.Pod) {
		pod.Namespace = "dev"
		pod.ObjectMeta.CreationTimestamp = metav1.NewTime(time.Now().Add(-time.Second * 595))
	})

	// Setup two old pods with different status phases
	p9 := test.BuildTestPod("p9", 100, 0, nodeName1, func(pod *v1.Pod) {
		pod.Namespace = "dev"
		pod.ObjectMeta.CreationTimestamp = olderPodCreationTime
		pod.Status.Phase = "Pending"
		test.SetRSOwnerRef(pod)
	})
	p10 := test.BuildTestPod("p10", 100, 0, nodeName1, func(pod *v1.Pod) {
		pod.Namespace = "dev"
		pod.ObjectMeta.CreationTimestamp = olderPodCreationTime
		pod.Status.Phase = "Running"
		test.SetRSOwnerRef(pod)
	})

	p11 := test.BuildTestPod("p11", 100, 0, nodeName1, func(pod *v1.Pod) {
		pod.Spec.Volumes = []v1.Volume{
			{
				Name: "pvc", VolumeSource: v1.VolumeSource{
					PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{ClaimName: "foo"},
				},
			},
		}
		pod.Namespace = "dev"
		pod.ObjectMeta.CreationTimestamp = olderPodCreationTime
		test.SetRSOwnerRef(pod)
	})

	// Setup two old pods with different labels
	p12 := test.BuildTestPod("p12", 100, 0, nodeName1, func(pod *v1.Pod) {
		pod.Namespace = "dev"
		pod.ObjectMeta.CreationTimestamp = olderPodCreationTime
		pod.ObjectMeta.Labels = map[string]string{"foo": "bar"}
		test.SetRSOwnerRef(pod)
	})
	p13 := test.BuildTestPod("p13", 100, 0, nodeName1, func(pod *v1.Pod) {
		pod.Namespace = "dev"
		pod.ObjectMeta.CreationTimestamp = olderPodCreationTime
		pod.ObjectMeta.Labels = map[string]string{"foo": "bar1"}
		test.SetRSOwnerRef(pod)
	})

	p14 := test.BuildTestPod("p14", 100, 0, nodeName1, func(pod *v1.Pod) {
		pod.Namespace = "dev"
		pod.ObjectMeta.CreationTimestamp = olderPodCreationTime
		test.SetRSOwnerRef(pod)
		pod.DeletionTimestamp = &metav1.Time{}
	})
	p15 := test.BuildTestPod("p15", 100, 0, nodeName1, func(pod *v1.Pod) {
		pod.Namespace = "dev"
		pod.ObjectMeta.CreationTimestamp = olderPodCreationTime
		test.SetRSOwnerRef(pod)
		pod.DeletionTimestamp = &metav1.Time{}
	})

	p16 := test.BuildTestPod("p16", 100, 0, nodeName1, func(pod *v1.Pod) {
		pod.Namespace = "dev"
		pod.ObjectMeta.CreationTimestamp = olderPodCreationTime
		pod.Status.Phase = v1.PodUnknown
		test.SetRSOwnerRef(pod)
	})

	var maxLifeTime uint = 600
	testCases := []struct {
		description                string
		args                       *PodLifeTimeArgs
		pods                       []*v1.Pod
		nodes                      []*v1.Node
		expectedEvictedPodCount    uint
		ignorePvcPods              bool
		maxPodsToEvictPerNode      *uint
		maxPodsToEvictPerNamespace *uint
		maxPodsToEvictTotal        *uint
		applyPodsFunc              func(pods []*v1.Pod)
	}{
		{
			description: "Two pods in the `dev` Namespace, 1 is new and 1 very is old. 1 should be evicted.",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
			},
			pods:                    []*v1.Pod{p1, p2},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
		},
		{
			description: "Two pods in the `dev` Namespace, 2 are new and 0 are old. 0 should be evicted.",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
			},
			pods:                    []*v1.Pod{p3, p4},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 0,
		},
		{
			description: "Two pods in the `dev` Namespace, 1 created 605 seconds ago. 1 should be evicted.",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
			},
			pods:                    []*v1.Pod{p5, p6},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
		},
		{
			description: "Two pods in the `dev` Namespace, 1 created 595 seconds ago. 0 should be evicted.",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
			},
			pods:                    []*v1.Pod{p7, p8},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 0,
		},
		{
			description: "Two pods, one with ContainerCreating state. 1 should be evicted.",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				States:                []string{"ContainerCreating"},
			},
			pods: []*v1.Pod{
				p9,
				test.BuildTestPod("container-creating-stuck", 0, 0, nodeName1, func(pod *v1.Pod) {
					pod.Status.ContainerStatuses = []v1.ContainerStatus{
						{
							State: v1.ContainerState{
								Waiting: &v1.ContainerStateWaiting{Reason: "ContainerCreating"},
							},
						},
					}
					test.SetRSOwnerRef(pod)
					pod.ObjectMeta.CreationTimestamp = olderPodCreationTime
				}),
			},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
		},
		{
			description: "Two pods, one with PodInitializing state. 1 should be evicted.",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				States:                []string{"PodInitializing"},
			},
			pods: []*v1.Pod{
				p9,
				test.BuildTestPod("pod-initializing-stuck", 0, 0, nodeName1, func(pod *v1.Pod) {
					pod.Status.ContainerStatuses = []v1.ContainerStatus{
						{
							State: v1.ContainerState{
								Waiting: &v1.ContainerStateWaiting{Reason: "PodInitializing"},
							},
						},
					}
					test.SetRSOwnerRef(pod)
					pod.ObjectMeta.CreationTimestamp = olderPodCreationTime
				}),
			},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
		},
		{
			description: "Two old pods with different states. 1 should be evicted.",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				States:                []string{"Pending"},
			},
			pods:                    []*v1.Pod{p9, p10},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
		},
		{
			description: "does not evict pvc pods with ignorePvcPods set to true",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
			},
			pods:                    []*v1.Pod{p11},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 0,
			ignorePvcPods:           true,
		},
		{
			description: "evicts pvc pods with ignorePvcPods set to false (or unset)",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
			},
			pods:                    []*v1.Pod{p11},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
		},
		{
			description: "No pod to evicted since all pod terminating",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"foo": "bar"},
				},
			},
			pods:                    []*v1.Pod{p12, p13},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
		},
		{
			description: "No pod should be evicted since pod terminating",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"foo": "bar"},
				},
			},
			pods:                    []*v1.Pod{p14, p15},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 0,
		},
		{
			description: "2 Oldest pods should be evicted when maxPodsToEvictPerNode and maxPodsToEvictPerNamespace are not set",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
			},
			pods:                       []*v1.Pod{p1, p2, p9},
			nodes:                      []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount:    2,
			maxPodsToEvictPerNode:      nil,
			maxPodsToEvictPerNamespace: nil,
		},
		{
			description: "1 Oldest pod should be evicted when maxPodsToEvictPerNamespace is set to 1",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
			},
			pods:                       []*v1.Pod{p1, p2, p9},
			nodes:                      []*v1.Node{buildTestNode1()},
			maxPodsToEvictPerNamespace: utilptr.To[uint](1),
			expectedEvictedPodCount:    1,
		},
		{
			description: "1 Oldest pod should be evicted when maxPodsToEvictTotal is set to 1",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
			},
			pods:                       []*v1.Pod{p1, p2, p9},
			nodes:                      []*v1.Node{buildTestNode1()},
			maxPodsToEvictPerNamespace: utilptr.To[uint](2),
			maxPodsToEvictTotal:        utilptr.To[uint](1),
			expectedEvictedPodCount:    1,
		},
		{
			description: "1 Oldest pod should be evicted when maxPodsToEvictPerNode is set to 1",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
			},
			pods:                    []*v1.Pod{p1, p2, p9},
			nodes:                   []*v1.Node{buildTestNode1()},
			maxPodsToEvictPerNode:   utilptr.To[uint](1),
			expectedEvictedPodCount: 1,
		},
		{
			description: "1 pod with container status ImagePullBackOff should be evicted",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				States:                []string{"ImagePullBackOff"},
			},
			pods:                    []*v1.Pod{p9},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
			applyPodsFunc: func(pods []*v1.Pod) {
				pods[0].Status.ContainerStatuses = []v1.ContainerStatus{
					{
						State: v1.ContainerState{
							Waiting: &v1.ContainerStateWaiting{Reason: "ImagePullBackOff"},
						},
					},
				}
			},
		},
		{
			description: "1 pod with container status CrashLoopBackOff should be evicted",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				States:                []string{"CrashLoopBackOff"},
			},
			pods:                    []*v1.Pod{p9},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
			applyPodsFunc: func(pods []*v1.Pod) {
				pods[0].Status.ContainerStatuses = []v1.ContainerStatus{
					{
						State: v1.ContainerState{
							Waiting: &v1.ContainerStateWaiting{Reason: "CrashLoopBackOff"},
						},
					},
				}
			},
		},
		{
			description: "1 pod with container status CreateContainerConfigError should be evicted",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				States:                []string{"CreateContainerConfigError"},
			},
			pods:                    []*v1.Pod{p9},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
			applyPodsFunc: func(pods []*v1.Pod) {
				pods[0].Status.ContainerStatuses = []v1.ContainerStatus{
					{
						State: v1.ContainerState{
							Waiting: &v1.ContainerStateWaiting{Reason: "CreateContainerConfigError"},
						},
					},
				}
			},
		},
		{
			description: "1 pod with container status ErrImagePull should be evicted",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				States:                []string{"ErrImagePull"},
			},
			pods:                    []*v1.Pod{p9},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
			applyPodsFunc: func(pods []*v1.Pod) {
				pods[0].Status.ContainerStatuses = []v1.ContainerStatus{
					{
						State: v1.ContainerState{
							Waiting: &v1.ContainerStateWaiting{Reason: "ErrImagePull"},
						},
					},
				}
			},
		},
		{
			description: "1 pod with init container status CreateContainerError should not be evicted without includingInitContainers",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				States:                []string{"CreateContainerError"},
			},
			pods:                    []*v1.Pod{p9},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 0,
			applyPodsFunc: func(pods []*v1.Pod) {
				pods[0].Status.InitContainerStatuses = []v1.ContainerStatus{
					{
						State: v1.ContainerState{
							Waiting: &v1.ContainerStateWaiting{Reason: "CreateContainerError"},
						},
					},
				}
			},
		},
		{
			description: "1 pod with init container status CreateContainerError should be evicted with includingInitContainers",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds:   &maxLifeTime,
				States:                  []string{"CreateContainerError"},
				IncludingInitContainers: true,
			},
			pods:                    []*v1.Pod{p9},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
			applyPodsFunc: func(pods []*v1.Pod) {
				pods[0].Status.InitContainerStatuses = []v1.ContainerStatus{
					{
						State: v1.ContainerState{
							Waiting: &v1.ContainerStateWaiting{Reason: "CreateContainerError"},
						},
					},
				}
			},
		},
		{
			description: "1 pod with ephemeral container status CreateContainerError should not be evicted without includingEphemeralContainers",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				States:                []string{"CreateContainerError"},
			},
			pods:                    []*v1.Pod{p9},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 0,
			applyPodsFunc: func(pods []*v1.Pod) {
				pods[0].Status.InitContainerStatuses = []v1.ContainerStatus{
					{
						State: v1.ContainerState{
							Waiting: &v1.ContainerStateWaiting{Reason: "CreateContainerError"},
						},
					},
				}
			},
		},
		{
			description: "1 pod with ephemeral container status CreateContainerError should be evicted with includingEphemeralContainers",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds:        &maxLifeTime,
				States:                       []string{"CreateContainerError"},
				IncludingEphemeralContainers: true,
			},
			pods:                    []*v1.Pod{p9},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
			applyPodsFunc: func(pods []*v1.Pod) {
				pods[0].Status.EphemeralContainerStatuses = []v1.ContainerStatus{
					{
						State: v1.ContainerState{
							Waiting: &v1.ContainerStateWaiting{Reason: "CreateContainerError"},
						},
					},
				}
			},
		},
		{
			description: "1 pod with container status CreateContainerError should be evicted",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				States:                []string{"CreateContainerError"},
			},
			pods:                    []*v1.Pod{p9},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
			applyPodsFunc: func(pods []*v1.Pod) {
				pods[0].Status.ContainerStatuses = []v1.ContainerStatus{
					{
						State: v1.ContainerState{
							Waiting: &v1.ContainerStateWaiting{Reason: "CreateContainerError"},
						},
					},
				}
			},
		},
		{
			description: "1 pod with container status InvalidImageName should be evicted",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				States:                []string{"InvalidImageName"},
			},
			pods:                    []*v1.Pod{p9},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
			applyPodsFunc: func(pods []*v1.Pod) {
				pods[0].Status.ContainerStatuses = []v1.ContainerStatus{
					{
						State: v1.ContainerState{
							Waiting: &v1.ContainerStateWaiting{Reason: "InvalidImageName"},
						},
					},
				}
			},
		},
		{
			description: "1 pod with pod status reason NodeLost should be evicted",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				States:                []string{"NodeLost"},
			},
			pods:                    []*v1.Pod{p9},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
			applyPodsFunc: func(pods []*v1.Pod) {
				pods[0].Status.Reason = "NodeLost"
			},
		},
		{
			description: "1 pod with pod status reason NodeAffinity should be evicted",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				States:                []string{"NodeAffinity"},
			},
			pods:                    []*v1.Pod{p9},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
			applyPodsFunc: func(pods []*v1.Pod) {
				pods[0].Status.Reason = "NodeAffinity"
			},
		},
		{
			description: "1 pod with pod status reason Shutdown should be evicted",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				States:                []string{"Shutdown"},
			},
			pods:                    []*v1.Pod{p9},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
			applyPodsFunc: func(pods []*v1.Pod) {
				pods[0].Status.Reason = "Shutdown"
			},
		},
		{
			description: "1 pod with pod status reason UnexpectedAdmissionError should be evicted",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				States:                []string{"UnexpectedAdmissionError"},
			},
			pods:                    []*v1.Pod{p9},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
			applyPodsFunc: func(pods []*v1.Pod) {
				pods[0].Status.Reason = "UnexpectedAdmissionError"
			},
		},
		{
			description: "1 pod with pod status phase v1.PodSucceeded should be evicted",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				States:                []string{string(v1.PodSucceeded)},
			},
			pods:                    []*v1.Pod{p16},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
			applyPodsFunc: func(pods []*v1.Pod) {
				pods[0].Status.Phase = v1.PodSucceeded
			},
		},
		{
			description: "1 pod with pod status phase v1.PodUnknown should be evicted",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				States:                []string{string(v1.PodFailed)},
			},
			pods:                    []*v1.Pod{p16},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
			applyPodsFunc: func(pods []*v1.Pod) {
				pods[0].Status.Phase = v1.PodFailed
			},
		},
		{
			description: "1 pod with pod status phase v1.PodUnknown should be evicted",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				States:                []string{string(v1.PodUnknown)},
			},
			pods:                    []*v1.Pod{p16},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
			applyPodsFunc: func(pods []*v1.Pod) {
				pods[0].Status.Phase = v1.PodUnknown
			},
		},
		{
			description: "1 pod without ImagePullBackOff States should be ignored",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				States:                []string{"ContainerCreating"},
			},
			pods:                    []*v1.Pod{p9},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 0,
			applyPodsFunc: func(pods []*v1.Pod) {
				pods[0].Status.ContainerStatuses = []v1.ContainerStatus{
					{
						State: v1.ContainerState{
							Waiting: &v1.ContainerStateWaiting{Reason: "ImagePullBackOff"},
						},
					},
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if tc.applyPodsFunc != nil {
				tc.applyPodsFunc(tc.pods)
			}

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
					WithMaxPodsToEvictPerNamespace(tc.maxPodsToEvictPerNamespace).
					WithMaxPodsToEvictTotal(tc.maxPodsToEvictTotal),
				defaultevictor.DefaultEvictorArgs{IgnorePvcPods: tc.ignorePvcPods},
				nil,
			)
			if err != nil {
				t.Fatalf("Unable to initialize a framework handle: %v", err)
			}

			plugin, err := New(ctx, tc.args, handle)
			if err != nil {
				t.Fatalf("Unable to initialize the plugin: %v", err)
			}

			plugin.(frameworktypes.DeschedulePlugin).Deschedule(ctx, tc.nodes)
			podsEvicted := podEvictor.TotalEvicted()
			if podsEvicted != tc.expectedEvictedPodCount {
				t.Errorf("Test error for description: %s. Expected evicted pods count %v, got %v", tc.description, tc.expectedEvictedPodCount, podsEvicted)
			}
		})
	}
}
