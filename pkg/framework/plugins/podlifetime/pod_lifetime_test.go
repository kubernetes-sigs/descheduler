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
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes/fake"
	utilptr "k8s.io/utils/ptr"

	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	frameworktesting "sigs.k8s.io/descheduler/pkg/framework/testing"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
	"sigs.k8s.io/descheduler/test"
)

const nodeName1 = "n1"

var (
	olderPodCreationTime = metav1.NewTime(time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC))
	newerPodCreationTime = metav1.NewTime(time.Now())
)

func buildTestNode1() *v1.Node {
	return test.BuildTestNode(nodeName1, 2000, 3000, 10, nil)
}

func buildTestPodForNode1(name string, creationTime metav1.Time, apply func(*v1.Pod)) *v1.Pod {
	return test.BuildTestPod(name, 100, 0, nodeName1, func(pod *v1.Pod) {
		pod.ObjectMeta.CreationTimestamp = creationTime
		if apply != nil {
			apply(pod)
		}
	})
}

func buildTestPodWithRSOwnerRefForNode1(name string, creationTime metav1.Time, apply func(*v1.Pod)) *v1.Pod {
	return buildTestPodForNode1(name, creationTime, func(pod *v1.Pod) {
		test.SetRSOwnerRef(pod)
		if apply != nil {
			apply(pod)
		}
	})
}

func buildTestPodWithRSOwnerRefWithPendingPhaseForNode1(name string, creationTime metav1.Time, apply func(*v1.Pod)) *v1.Pod {
	return buildTestPodWithRSOwnerRefForNode1(name, creationTime, func(pod *v1.Pod) {
		pod.Status.Phase = "Pending"
		if apply != nil {
			apply(pod)
		}
	})
}

type podLifeTimeTestCase struct {
	description                string
	args                       *PodLifeTimeArgs
	pods                       []*v1.Pod
	nodes                      []*v1.Node
	expectedEvictedPods        []string // if specified, will assert specific pods were evicted
	expectedEvictedPodCount    uint
	ignorePvcPods              bool
	maxPodsToEvictPerNode      *uint
	maxPodsToEvictPerNamespace *uint
	maxPodsToEvictTotal        *uint
}

func runPodLifeTimeTest(t *testing.T, tc podLifeTimeTestCase) {
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
			WithMaxPodsToEvictPerNamespace(tc.maxPodsToEvictPerNamespace).
			WithMaxPodsToEvictTotal(tc.maxPodsToEvictTotal),
		defaultevictor.DefaultEvictorArgs{IgnorePvcPods: tc.ignorePvcPods},
		nil,
	)
	if err != nil {
		t.Fatalf("Unable to initialize a framework handle: %v", err)
	}

	var evictedPods []string
	test.RegisterEvictedPodsCollector(fakeClient, &evictedPods)

	plugin, err := New(ctx, tc.args, handle)
	if err != nil {
		t.Fatalf("Unable to initialize the plugin: %v", err)
	}

	plugin.(frameworktypes.DeschedulePlugin).Deschedule(ctx, tc.nodes)
	podsEvicted := podEvictor.TotalEvicted()
	if podsEvicted != tc.expectedEvictedPodCount {
		t.Errorf("Test error for description: %s. Expected evicted pods count %v, got %v", tc.description, tc.expectedEvictedPodCount, podsEvicted)
	}

	if tc.expectedEvictedPods != nil {
		diff := sets.New(tc.expectedEvictedPods...).Difference(sets.New(evictedPods...))
		if diff.Len() > 0 {
			t.Errorf(
				"Expected pods %v to be evicted but %v were not evicted. Actual pods evicted: %v",
				tc.expectedEvictedPods,
				diff.UnsortedList(),
				evictedPods,
			)
		}
	}
}

func TestPodLifeTime(t *testing.T) {
	var maxLifeTime uint = 600
	testCases := []podLifeTimeTestCase{
		{
			description: "Two pods, one with ContainerCreating state. 1 should be evicted.",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				States:                []string{"ContainerCreating"},
			},
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefWithPendingPhaseForNode1("p9", olderPodCreationTime, nil),
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
				buildTestPodWithRSOwnerRefWithPendingPhaseForNode1("p9", olderPodCreationTime, nil),
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
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefWithPendingPhaseForNode1("p9", olderPodCreationTime, nil),
				buildTestPodWithRSOwnerRefForNode1("p10", olderPodCreationTime, func(pod *v1.Pod) {
					pod.Status.Phase = "Running"
				}),
			},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
			expectedEvictedPods:     []string{"p9"},
		},
		{
			description: "Does not evict pvc pods with ignorePvcPods set to true",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
			},
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefForNode1("p11", olderPodCreationTime, func(pod *v1.Pod) {
					pod.Spec.Volumes = []v1.Volume{
						{
							Name: "pvc", VolumeSource: v1.VolumeSource{
								PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{ClaimName: "foo"},
							},
						},
					}
				}),
			},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 0,
			ignorePvcPods:           true,
		},
		{
			description: "Evicts pvc pods with ignorePvcPods set to false (or unset)",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
			},
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefForNode1("p11", olderPodCreationTime, func(pod *v1.Pod) {
					pod.Spec.Volumes = []v1.Volume{
						{
							Name: "pvc", VolumeSource: v1.VolumeSource{
								PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{ClaimName: "foo"},
							},
						},
					}
				}),
			},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
		},
		{
			description: "1 pod matching label selector should be evicted",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"foo": "bar"},
				},
			},
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefForNode1("p12", olderPodCreationTime, func(pod *v1.Pod) {
					pod.ObjectMeta.Labels = map[string]string{"foo": "bar"}
				}),
				buildTestPodWithRSOwnerRefForNode1("p13", olderPodCreationTime, func(pod *v1.Pod) {
					pod.ObjectMeta.Labels = map[string]string{"foo": "bar1"}
				}),
			},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
			expectedEvictedPods:     []string{"p12"},
		},
		{
			description: "No pod should be evicted since pod terminating",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"foo": "bar"},
				},
			},
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefForNode1("p14", olderPodCreationTime, func(pod *v1.Pod) {
					pod.DeletionTimestamp = &metav1.Time{}
				}),
				buildTestPodWithRSOwnerRefForNode1("p15", olderPodCreationTime, func(pod *v1.Pod) {
					pod.DeletionTimestamp = &metav1.Time{}
				}),
			},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 0,
		},
		{
			description: "2 Oldest pods should be evicted when maxPodsToEvictPerNode and maxPodsToEvictPerNamespace are not set",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
			},
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefForNode1("p1", newerPodCreationTime, nil),
				buildTestPodWithRSOwnerRefForNode1("p2", olderPodCreationTime, nil),
				buildTestPodWithRSOwnerRefWithPendingPhaseForNode1("p9", olderPodCreationTime, nil),
			},
			nodes:                      []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount:    2,
			expectedEvictedPods:        []string{"p2", "p9"},
			maxPodsToEvictPerNode:      nil,
			maxPodsToEvictPerNamespace: nil,
		},
		{
			description: "1 Oldest pod should be evicted when maxPodsToEvictPerNamespace is set to 1",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
			},
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefForNode1("p1", newerPodCreationTime, nil),
				buildTestPodWithRSOwnerRefForNode1("p2", olderPodCreationTime, nil),
			},
			nodes:                      []*v1.Node{buildTestNode1()},
			maxPodsToEvictPerNamespace: utilptr.To[uint](1),
			expectedEvictedPodCount:    1,
			expectedEvictedPods:        []string{"p2"},
		},
		{
			description: "1 Oldest pod should be evicted when maxPodsToEvictTotal is set to 1",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
			},
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefForNode1("p1", newerPodCreationTime, nil),
				buildTestPodWithRSOwnerRefForNode1("p2", olderPodCreationTime, nil),
			},
			nodes:                      []*v1.Node{buildTestNode1()},
			maxPodsToEvictPerNamespace: utilptr.To[uint](2),
			maxPodsToEvictTotal:        utilptr.To[uint](1),
			expectedEvictedPodCount:    1,
			expectedEvictedPods:        []string{"p2"},
		},
		{
			description: "1 Oldest pod should be evicted when maxPodsToEvictPerNode is set to 1",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
			},
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefForNode1("p1", newerPodCreationTime, nil),
				buildTestPodWithRSOwnerRefForNode1("p2", olderPodCreationTime, nil),
			},
			nodes:                   []*v1.Node{buildTestNode1()},
			maxPodsToEvictPerNode:   utilptr.To[uint](1),
			expectedEvictedPodCount: 1,
			expectedEvictedPods:     []string{"p2"},
		},
		{
			description: "1 pod with container status ImagePullBackOff should be evicted",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				States:                []string{"ImagePullBackOff"},
			},
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefWithPendingPhaseForNode1("p9", olderPodCreationTime, func(pod *v1.Pod) {
					pod.Status.ContainerStatuses = []v1.ContainerStatus{
						{
							State: v1.ContainerState{
								Waiting: &v1.ContainerStateWaiting{Reason: "ImagePullBackOff"},
							},
						},
					}
				}),
			},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
		},
		{
			description: "1 pod with container status CrashLoopBackOff should be evicted",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				States:                []string{"CrashLoopBackOff"},
			},
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefWithPendingPhaseForNode1("p9", olderPodCreationTime, func(pod *v1.Pod) {
					pod.Status.ContainerStatuses = []v1.ContainerStatus{
						{
							State: v1.ContainerState{
								Waiting: &v1.ContainerStateWaiting{Reason: "CrashLoopBackOff"},
							},
						},
					}
				}),
			},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
		},
		{
			description: "1 pod with container status CreateContainerConfigError should be evicted",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				States:                []string{"CreateContainerConfigError"},
			},
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefWithPendingPhaseForNode1("p9", olderPodCreationTime, func(pod *v1.Pod) {
					pod.Status.ContainerStatuses = []v1.ContainerStatus{
						{
							State: v1.ContainerState{
								Waiting: &v1.ContainerStateWaiting{Reason: "CreateContainerConfigError"},
							},
						},
					}
				}),
			},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
		},
		{
			description: "1 pod with container status ErrImagePull should be evicted",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				States:                []string{"ErrImagePull"},
			},
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefWithPendingPhaseForNode1("p9", olderPodCreationTime, func(pod *v1.Pod) {
					pod.Status.ContainerStatuses = []v1.ContainerStatus{
						{
							State: v1.ContainerState{
								Waiting: &v1.ContainerStateWaiting{Reason: "ErrImagePull"},
							},
						},
					}
				}),
			},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
		},
		{
			description: "1 pod with init container status CreateContainerError should not be evicted without includingInitContainers",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				States:                []string{"CreateContainerError"},
			},
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefWithPendingPhaseForNode1("p9", olderPodCreationTime, func(pod *v1.Pod) {
					pod.Status.InitContainerStatuses = []v1.ContainerStatus{
						{
							State: v1.ContainerState{
								Waiting: &v1.ContainerStateWaiting{Reason: "CreateContainerError"},
							},
						},
					}
				}),
			},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 0,
		},
		{
			description: "1 pod with init container status CreateContainerError should be evicted with includingInitContainers",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds:   &maxLifeTime,
				States:                  []string{"CreateContainerError"},
				IncludingInitContainers: true,
			},
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefWithPendingPhaseForNode1("p9", olderPodCreationTime, func(pod *v1.Pod) {
					pod.Status.InitContainerStatuses = []v1.ContainerStatus{
						{
							State: v1.ContainerState{
								Waiting: &v1.ContainerStateWaiting{Reason: "CreateContainerError"},
							},
						},
					}
				}),
			},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
		},
		{
			description: "1 pod with ephemeral container status CreateContainerError should not be evicted without includingEphemeralContainers",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				States:                []string{"CreateContainerError"},
			},
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefWithPendingPhaseForNode1("p9", olderPodCreationTime, func(pod *v1.Pod) {
					pod.Status.EphemeralContainerStatuses = []v1.ContainerStatus{
						{
							State: v1.ContainerState{
								Waiting: &v1.ContainerStateWaiting{Reason: "CreateContainerError"},
							},
						},
					}
				}),
			},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 0,
		},
		{
			description: "1 pod with ephemeral container status CreateContainerError should be evicted with includingEphemeralContainers",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds:        &maxLifeTime,
				States:                       []string{"CreateContainerError"},
				IncludingEphemeralContainers: true,
			},
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefWithPendingPhaseForNode1("p9", olderPodCreationTime, func(pod *v1.Pod) {
					pod.Status.EphemeralContainerStatuses = []v1.ContainerStatus{
						{
							State: v1.ContainerState{
								Waiting: &v1.ContainerStateWaiting{Reason: "CreateContainerError"},
							},
						},
					}
				}),
			},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
		},
		{
			description: "1 pod with container status CreateContainerError should be evicted",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				States:                []string{"CreateContainerError"},
			},
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefWithPendingPhaseForNode1("p9", olderPodCreationTime, func(pod *v1.Pod) {
					pod.Status.ContainerStatuses = []v1.ContainerStatus{
						{
							State: v1.ContainerState{
								Waiting: &v1.ContainerStateWaiting{Reason: "CreateContainerError"},
							},
						},
					}
				}),
			},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
		},
		{
			description: "1 pod with container status InvalidImageName should be evicted",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				States:                []string{"InvalidImageName"},
			},
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefWithPendingPhaseForNode1("p9", olderPodCreationTime, func(pod *v1.Pod) {
					pod.Status.ContainerStatuses = []v1.ContainerStatus{
						{
							State: v1.ContainerState{
								Waiting: &v1.ContainerStateWaiting{Reason: "InvalidImageName"},
							},
						},
					}
				}),
			},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
		},
		{
			description: "1 pod with pod status reason NodeLost should be evicted",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				States:                []string{"NodeLost"},
			},
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefWithPendingPhaseForNode1("p9", olderPodCreationTime, func(pod *v1.Pod) {
					pod.Status.Reason = "NodeLost"
				}),
			},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
		},
		{
			description: "1 pod with pod status reason NodeAffinity should be evicted",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				States:                []string{"NodeAffinity"},
			},
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefWithPendingPhaseForNode1("p9", olderPodCreationTime, func(pod *v1.Pod) {
					pod.Status.Reason = "NodeAffinity"
				}),
			},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
		},
		{
			description: "1 pod with pod status reason Shutdown should be evicted",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				States:                []string{"Shutdown"},
			},
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefWithPendingPhaseForNode1("p9", olderPodCreationTime, func(pod *v1.Pod) {
					pod.Status.Reason = "Shutdown"
				}),
			},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
		},
		{
			description: "1 pod with pod status reason UnexpectedAdmissionError should be evicted",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				States:                []string{"UnexpectedAdmissionError"},
			},
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefWithPendingPhaseForNode1("p9", olderPodCreationTime, func(pod *v1.Pod) {
					pod.Status.Reason = "UnexpectedAdmissionError"
				}),
			},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
		},
		{
			description: "1 pod with pod status phase v1.PodSucceeded should be evicted",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				States:                []string{string(v1.PodSucceeded)},
			},
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefForNode1("p16", olderPodCreationTime, func(pod *v1.Pod) {
					pod.Status.Phase = v1.PodSucceeded
				}),
			},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
		},
		{
			description: "1 pod with pod status phase v1.PodFailed should be evicted",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				States:                []string{string(v1.PodFailed)},
			},
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefForNode1("p16", olderPodCreationTime, func(pod *v1.Pod) {
					pod.Status.Phase = v1.PodFailed
				}),
			},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
		},
		{
			description: "1 pod with pod status phase v1.PodUnknown should be evicted",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				States:                []string{string(v1.PodUnknown)},
			},
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefForNode1("p16", olderPodCreationTime, func(pod *v1.Pod) {
					pod.Status.Phase = v1.PodUnknown
				}),
			},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
		},
		{
			description: "1 pod with ImagePullBackOff status should be ignored when States filter is ContainerCreating",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
				States:                []string{"ContainerCreating"},
			},
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefWithPendingPhaseForNode1("p9", olderPodCreationTime, func(pod *v1.Pod) {
					pod.Status.ContainerStatuses = []v1.ContainerStatus{
						{
							State: v1.ContainerState{
								Waiting: &v1.ContainerStateWaiting{Reason: "ImagePullBackOff"},
							},
						},
					}
				}),
			},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			runPodLifeTimeTest(t, tc)
		})
	}
}

func TestPodLifeTime_AgeThreshold(t *testing.T) {
	var maxLifeTime uint = 600
	testCases := []podLifeTimeTestCase{
		{
			description: "Two pods in the default namespace, 1 is new and 1 very is old. 1 should be evicted.",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
			},
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefForNode1("p1", newerPodCreationTime, nil),
				buildTestPodWithRSOwnerRefForNode1("p2", olderPodCreationTime, nil),
			},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
			expectedEvictedPods:     []string{"p2"},
		},
		{
			description: "Two pods in the default namespace, 2 are new and 0 are old. 0 should be evicted.",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
			},
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefForNode1("p3", newerPodCreationTime, nil),
				buildTestPodWithRSOwnerRefForNode1("p4", newerPodCreationTime, nil),
			},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 0,
		},
		{
			description: "Two pods in the default namespace, 1 created 605 seconds ago. 1 should be evicted.",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
			},
			pods: []*v1.Pod{
				buildTestPodWithRSOwnerRefForNode1("p5", newerPodCreationTime, nil),
				buildTestPodWithRSOwnerRefForNode1("p6", metav1.NewTime(time.Now().Add(-time.Second*605)), nil),
			},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 1,
			expectedEvictedPods:     []string{"p6"},
		},
		{
			description: "Two pods in the default namespace, 1 created 595 seconds ago. 0 should be evicted.",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: &maxLifeTime,
			},
			pods: []*v1.Pod{
				buildTestPodForNode1("p7", newerPodCreationTime, nil),
				buildTestPodForNode1("p8", metav1.NewTime(time.Now().Add(-time.Second*595)), nil),
			},
			nodes:                   []*v1.Node{buildTestNode1()},
			expectedEvictedPodCount: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			runPodLifeTimeTest(t, tc)
		})
	}
}
