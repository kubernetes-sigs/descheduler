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

	"sigs.k8s.io/descheduler/pkg/api"
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
	oldTransitionTime    = metav1.NewTime(time.Now().Add(-2 * time.Hour))
	newTransitionTime    = metav1.NewTime(time.Now().Add(-1 * time.Minute))
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

func buildPod(name, nodeName string, apply func(*v1.Pod)) *v1.Pod {
	pod := test.BuildTestPod(name, 1, 1, nodeName, func(p *v1.Pod) {
		p.ObjectMeta.OwnerReferences = test.GetReplicaSetOwnerRefList()
		if apply != nil {
			apply(p)
		}
	})
	return pod
}

type podLifeTimeTestCase struct {
	description                string
	args                       *PodLifeTimeArgs
	pods                       []*v1.Pod
	nodes                      []*v1.Node
	expectedEvictedPods        []string
	expectedEvictedPodCount    uint
	ignorePvcPods              bool
	nodeFit                    bool
	maxPodsToEvictPerNode      *uint
	maxPodsToEvictPerNamespace *uint
	maxPodsToEvictTotal        *uint
}

func runPodLifeTimeTest(t *testing.T, tc podLifeTimeTestCase) {
	t.Helper()
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
		defaultevictor.DefaultEvictorArgs{IgnorePvcPods: tc.ignorePvcPods, NodeFit: tc.nodeFit},
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
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			runPodLifeTimeTest(t, tc)
		})
	}
}

func TestPodLifeTime_EvictorConfiguration(t *testing.T) {
	var maxLifeTime uint = 600
	testCases := []podLifeTimeTestCase{
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
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			runPodLifeTimeTest(t, tc)
		})
	}
}

func TestPodLifeTime_GenericFiltering(t *testing.T) {
	var maxLifeTime uint = 600
	testCases := []podLifeTimeTestCase{
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
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			runPodLifeTimeTest(t, tc)
		})
	}
}

func TestPodLifeTime_EvictionLimits(t *testing.T) {
	var maxLifeTime uint = 600
	testCases := []podLifeTimeTestCase{
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
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			runPodLifeTimeTest(t, tc)
		})
	}
}

func TestPodLifeTime_ContainerWaitingReasons(t *testing.T) {
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

func TestPodLifeTime_PodStatusReasons(t *testing.T) {
	var maxLifeTime uint = 600
	testCases := []podLifeTimeTestCase{
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

func TestPodLifeTime_PodPhaseStates(t *testing.T) {
	var maxLifeTime uint = 600
	testCases := []podLifeTimeTestCase{
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
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			runPodLifeTimeTest(t, tc)
		})
	}
}

// Tests for new fields (Conditions, ExitCodes, OwnerKinds, MinTimeSinceLastTransitionSeconds)
// and extended States behavior (terminated reason matching)

func TestStatesWithTerminatedReasons(t *testing.T) {
	node := test.BuildTestNode("node1", 2000, 3000, 10, nil)
	testCases := []podLifeTimeTestCase{
		{
			description: "evict pod with matching terminated reason via states",
			args: &PodLifeTimeArgs{
				States: []string{string(v1.PodFailed), "NodeAffinity"},
			},
			nodes: []*v1.Node{node},
			pods: []*v1.Pod{
				buildPod("p1", "node1", func(p *v1.Pod) {
					p.Status.Phase = v1.PodFailed
					p.Status.ContainerStatuses = []v1.ContainerStatus{
						{State: v1.ContainerState{Terminated: &v1.ContainerStateTerminated{Reason: "NodeAffinity"}}},
					}
				}),
			},
			expectedEvictedPodCount: 1,
		},
		{
			description: "evict pod with matching pod status reason via states",
			args: &PodLifeTimeArgs{
				States: []string{"Shutdown"},
			},
			nodes: []*v1.Node{node},
			pods: []*v1.Pod{
				buildPod("p1", "node1", func(p *v1.Pod) {
					p.Status.Phase = v1.PodFailed
					p.Status.Reason = "Shutdown"
				}),
			},
			expectedEvictedPodCount: 1,
		},
		{
			description: "states matches terminated reason on init container",
			args: &PodLifeTimeArgs{
				States:                  []string{"CreateContainerConfigError"},
				IncludingInitContainers: true,
			},
			nodes: []*v1.Node{node},
			pods: []*v1.Pod{
				buildPod("p1", "node1", func(p *v1.Pod) {
					p.Status.InitContainerStatuses = []v1.ContainerStatus{
						{State: v1.ContainerState{Terminated: &v1.ContainerStateTerminated{Reason: "CreateContainerConfigError"}}},
					}
				}),
			},
			expectedEvictedPodCount: 1,
		},
		{
			description: "states does not match terminated reason on init container without flag",
			args: &PodLifeTimeArgs{
				States: []string{"CreateContainerConfigError"},
			},
			nodes: []*v1.Node{node},
			pods: []*v1.Pod{
				buildPod("p1", "node1", func(p *v1.Pod) {
					p.Status.InitContainerStatuses = []v1.ContainerStatus{
						{State: v1.ContainerState{Terminated: &v1.ContainerStateTerminated{Reason: "CreateContainerConfigError"}}},
					}
				}),
			},
			expectedEvictedPodCount: 0,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			runPodLifeTimeTest(t, tc)
		})
	}
}

func TestConditionFiltering(t *testing.T) {
	node := test.BuildTestNode("node1", 2000, 3000, 10, nil)
	testCases := []podLifeTimeTestCase{
		{
			description: "evict pod with matching condition reason",
			args: &PodLifeTimeArgs{
				Conditions: []PodConditionFilter{
					{Reason: "PodCompleted", Status: "True"},
				},
			},
			nodes: []*v1.Node{node},
			pods: []*v1.Pod{
				buildPod("p1", "node1", func(p *v1.Pod) {
					p.Status.Conditions = []v1.PodCondition{
						{Type: v1.PodInitialized, Status: v1.ConditionTrue, Reason: "PodCompleted", LastTransitionTime: oldTransitionTime},
					}
				}),
				buildPod("p2", "node1", func(p *v1.Pod) {
					p.Status.Conditions = []v1.PodCondition{
						{Type: v1.PodInitialized, Status: v1.ConditionTrue, Reason: "OtherReason", LastTransitionTime: oldTransitionTime},
					}
				}),
			},
			expectedEvictedPodCount: 1,
			expectedEvictedPods:     []string{"p1"},
		},
		{
			description: "evict pod matching condition type only",
			args: &PodLifeTimeArgs{
				Conditions: []PodConditionFilter{
					{Type: string(v1.PodReady)},
				},
			},
			nodes: []*v1.Node{node},
			pods: []*v1.Pod{
				buildPod("p1", "node1", func(p *v1.Pod) {
					p.Status.Conditions = []v1.PodCondition{
						{Type: v1.PodReady, Status: v1.ConditionFalse, LastTransitionTime: oldTransitionTime},
					}
				}),
			},
			expectedEvictedPodCount: 1,
		},
		{
			description: "no matching conditions, 0 evictions",
			args: &PodLifeTimeArgs{
				Conditions: []PodConditionFilter{
					{Reason: "PodCompleted"},
				},
			},
			nodes: []*v1.Node{node},
			pods: []*v1.Pod{
				buildPod("p1", "node1", func(p *v1.Pod) {
					p.Status.Conditions = []v1.PodCondition{
						{Type: v1.PodReady, Status: v1.ConditionTrue, Reason: "SomethingElse"},
					}
				}),
			},
			expectedEvictedPodCount: 0,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			runPodLifeTimeTest(t, tc)
		})
	}
}

func TestTransitionTimeFiltering(t *testing.T) {
	node := test.BuildTestNode("node1", 2000, 3000, 10, nil)
	var fiveMinutes uint = 300
	var fourHours uint = 14400

	testCases := []podLifeTimeTestCase{
		{
			description: "evict pod with old transition time",
			args: &PodLifeTimeArgs{
				States: []string{string(v1.PodSucceeded)},
				Conditions: []PodConditionFilter{
					{Reason: "PodCompleted", Status: "True", MinTimeSinceLastTransitionSeconds: &fiveMinutes},
				},
			},
			nodes: []*v1.Node{node},
			pods: []*v1.Pod{
				buildPod("p1", "node1", func(p *v1.Pod) {
					p.Status.Phase = v1.PodSucceeded
					p.Status.Conditions = []v1.PodCondition{
						{Type: v1.PodInitialized, Status: v1.ConditionTrue, Reason: "PodCompleted", LastTransitionTime: oldTransitionTime},
						{Type: v1.PodReady, Status: v1.ConditionFalse, Reason: "PodCompleted", LastTransitionTime: oldTransitionTime},
					}
				}),
			},
			expectedEvictedPodCount: 1,
			expectedEvictedPods:     []string{"p1"},
		},
		{
			description: "do not evict pod with recent transition time",
			args: &PodLifeTimeArgs{
				States: []string{string(v1.PodSucceeded)},
				Conditions: []PodConditionFilter{
					{MinTimeSinceLastTransitionSeconds: &fourHours},
				},
			},
			nodes: []*v1.Node{node},
			pods: []*v1.Pod{
				buildPod("p1", "node1", func(p *v1.Pod) {
					p.Status.Phase = v1.PodSucceeded
					p.Status.Conditions = []v1.PodCondition{
						{Type: v1.PodReady, Status: v1.ConditionFalse, LastTransitionTime: newTransitionTime},
					}
				}),
			},
			expectedEvictedPodCount: 0,
		},
		{
			description: "transition time scoped to matching conditions only",
			args: &PodLifeTimeArgs{
				Conditions: []PodConditionFilter{
					{Reason: "PodCompleted", MinTimeSinceLastTransitionSeconds: &fiveMinutes},
				},
			},
			nodes: []*v1.Node{node},
			pods: []*v1.Pod{
				buildPod("p1", "node1", func(p *v1.Pod) {
					p.Status.Conditions = []v1.PodCondition{
						{Type: v1.PodInitialized, Reason: "PodCompleted", LastTransitionTime: oldTransitionTime},
						{Type: v1.PodReady, Reason: "OtherReason", LastTransitionTime: newTransitionTime},
					}
				}),
			},
			expectedEvictedPodCount: 1,
			expectedEvictedPods:     []string{"p1"},
		},
		{
			description: "no conditions on pod, transition time filter returns false",
			args: &PodLifeTimeArgs{
				States: []string{string(v1.PodRunning)},
				Conditions: []PodConditionFilter{
					{MinTimeSinceLastTransitionSeconds: &fiveMinutes},
				},
			},
			nodes: []*v1.Node{node},
			pods: []*v1.Pod{
				buildPod("p1", "node1", func(p *v1.Pod) {
					p.Status.Phase = v1.PodRunning
				}),
			},
			expectedEvictedPodCount: 0,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			runPodLifeTimeTest(t, tc)
		})
	}
}

func TestOwnerKindsFiltering(t *testing.T) {
	node := test.BuildTestNode("node1", 2000, 3000, 10, nil)
	testCases := []podLifeTimeTestCase{
		{
			description: "exclude Job owner kind",
			args: &PodLifeTimeArgs{
				States:     []string{string(v1.PodFailed)},
				OwnerKinds: &OwnerKinds{Exclude: []string{"Job"}},
			},
			nodes: []*v1.Node{node},
			pods: []*v1.Pod{
				buildPod("p1", "node1", func(p *v1.Pod) {
					p.Status.Phase = v1.PodFailed
					p.ObjectMeta.OwnerReferences = []metav1.OwnerReference{{Kind: "Job", Name: "job1", APIVersion: "batch/v1"}}
				}),
				buildPod("p2", "node1", func(p *v1.Pod) {
					p.Status.Phase = v1.PodFailed
				}),
			},
			expectedEvictedPodCount: 1,
			expectedEvictedPods:     []string{"p2"},
		},
		{
			description: "include only Job owner kind",
			args: &PodLifeTimeArgs{
				States:     []string{string(v1.PodFailed)},
				OwnerKinds: &OwnerKinds{Include: []string{"Job"}},
			},
			nodes: []*v1.Node{node},
			pods: []*v1.Pod{
				buildPod("p1", "node1", func(p *v1.Pod) {
					p.Status.Phase = v1.PodFailed
					p.ObjectMeta.OwnerReferences = []metav1.OwnerReference{{Kind: "Job", Name: "job1", APIVersion: "batch/v1"}}
				}),
				buildPod("p2", "node1", func(p *v1.Pod) {
					p.Status.Phase = v1.PodFailed
				}),
			},
			expectedEvictedPodCount: 1,
			expectedEvictedPods:     []string{"p1"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			runPodLifeTimeTest(t, tc)
		})
	}
}

func TestStatesWithEphemeralContainers(t *testing.T) {
	node := test.BuildTestNode("node1", 2000, 3000, 10, nil)
	testCases := []podLifeTimeTestCase{
		{
			description: "states matches terminated reason on ephemeral container",
			args: &PodLifeTimeArgs{
				States:                       []string{"OOMKilled"},
				IncludingEphemeralContainers: true,
			},
			nodes: []*v1.Node{node},
			pods: []*v1.Pod{
				buildPod("p1", "node1", func(p *v1.Pod) {
					p.Status.EphemeralContainerStatuses = []v1.ContainerStatus{
						{State: v1.ContainerState{Terminated: &v1.ContainerStateTerminated{Reason: "OOMKilled"}}},
					}
				}),
			},
			expectedEvictedPodCount: 1,
		},
		{
			description: "states does not match ephemeral container without flag",
			args: &PodLifeTimeArgs{
				States: []string{"OOMKilled"},
			},
			nodes: []*v1.Node{node},
			pods: []*v1.Pod{
				buildPod("p1", "node1", func(p *v1.Pod) {
					p.Status.EphemeralContainerStatuses = []v1.ContainerStatus{
						{State: v1.ContainerState{Terminated: &v1.ContainerStateTerminated{Reason: "OOMKilled"}}},
					}
				}),
			},
			expectedEvictedPodCount: 0,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			runPodLifeTimeTest(t, tc)
		})
	}
}

func TestExitCodesFiltering(t *testing.T) {
	node := test.BuildTestNode("node1", 2000, 3000, 10, nil)
	testCases := []podLifeTimeTestCase{
		{
			description: "evict pod with matching exit code",
			args: &PodLifeTimeArgs{
				States:    []string{string(v1.PodFailed)},
				ExitCodes: []int32{1},
			},
			nodes: []*v1.Node{node},
			pods: []*v1.Pod{
				buildPod("p1", "node1", func(p *v1.Pod) {
					p.Status.Phase = v1.PodFailed
					p.Status.ContainerStatuses = []v1.ContainerStatus{
						{State: v1.ContainerState{Terminated: &v1.ContainerStateTerminated{ExitCode: 1}}},
					}
				}),
			},
			expectedEvictedPodCount: 1,
		},
		{
			description: "exit code not matched, 0 evictions",
			args: &PodLifeTimeArgs{
				States:    []string{string(v1.PodFailed)},
				ExitCodes: []int32{2},
			},
			nodes: []*v1.Node{node},
			pods: []*v1.Pod{
				buildPod("p1", "node1", func(p *v1.Pod) {
					p.Status.Phase = v1.PodFailed
					p.Status.ContainerStatuses = []v1.ContainerStatus{
						{State: v1.ContainerState{Terminated: &v1.ContainerStateTerminated{ExitCode: 1}}},
					}
				}),
			},
			expectedEvictedPodCount: 0,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			runPodLifeTimeTest(t, tc)
		})
	}
}

func TestCombinedFilters(t *testing.T) {
	node := test.BuildTestNode("node1", 2000, 3000, 10, nil)
	var fiveMinutes uint = 300

	testCases := []podLifeTimeTestCase{
		{
			description: "user scenario: Succeeded + PodCompleted condition + transition time threshold",
			args: &PodLifeTimeArgs{
				States: []string{string(v1.PodSucceeded)},
				Conditions: []PodConditionFilter{
					{Reason: "PodCompleted", Status: "True", MinTimeSinceLastTransitionSeconds: &fiveMinutes},
				},
			},
			nodes: []*v1.Node{node},
			pods: []*v1.Pod{
				buildPod("stale-completed", "node1", func(p *v1.Pod) {
					p.Status.Phase = v1.PodSucceeded
					p.Status.Conditions = []v1.PodCondition{
						{Type: v1.PodInitialized, Status: v1.ConditionTrue, Reason: "PodCompleted", LastTransitionTime: oldTransitionTime},
						{Type: v1.PodReady, Status: v1.ConditionFalse, Reason: "PodCompleted", LastTransitionTime: oldTransitionTime},
					}
				}),
				buildPod("recent-completed", "node1", func(p *v1.Pod) {
					p.Status.Phase = v1.PodSucceeded
					p.Status.Conditions = []v1.PodCondition{
						{Type: v1.PodInitialized, Status: v1.ConditionTrue, Reason: "PodCompleted", LastTransitionTime: newTransitionTime},
					}
				}),
				buildPod("running-pod", "node1", func(p *v1.Pod) {
					p.Status.Phase = v1.PodRunning
				}),
			},
			expectedEvictedPodCount: 1,
			expectedEvictedPods:     []string{"stale-completed"},
		},
		{
			description: "failed pod removal compat: Failed + min age + exclude Job",
			args: &PodLifeTimeArgs{
				States:                []string{string(v1.PodFailed)},
				MaxPodLifeTimeSeconds: utilptr.To[uint](0),
				OwnerKinds:            &OwnerKinds{Exclude: []string{"Job"}},
			},
			nodes: []*v1.Node{node},
			pods: []*v1.Pod{
				buildPod("failed-rs", "node1", func(p *v1.Pod) {
					p.Status.Phase = v1.PodFailed
					p.Status.ContainerStatuses = []v1.ContainerStatus{
						{State: v1.ContainerState{Terminated: &v1.ContainerStateTerminated{Reason: "Error"}}},
					}
				}),
				buildPod("failed-job", "node1", func(p *v1.Pod) {
					p.Status.Phase = v1.PodFailed
					p.ObjectMeta.OwnerReferences = []metav1.OwnerReference{{Kind: "Job", Name: "job1", APIVersion: "batch/v1"}}
				}),
			},
			expectedEvictedPodCount: 1,
			expectedEvictedPods:     []string{"failed-rs"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			runPodLifeTimeTest(t, tc)
		})
	}
}

func TestValidation(t *testing.T) {
	testCases := []struct {
		description string
		args        *PodLifeTimeArgs
		expectError bool
	}{
		{
			description: "valid: states set",
			args:        &PodLifeTimeArgs{States: []string{"Running"}},
			expectError: false,
		},
		{
			description: "valid: conditions set",
			args:        &PodLifeTimeArgs{Conditions: []PodConditionFilter{{Reason: "PodCompleted"}}},
			expectError: false,
		},
		{
			description: "valid: maxPodLifeTimeSeconds set",
			args:        &PodLifeTimeArgs{MaxPodLifeTimeSeconds: utilptr.To[uint](600)},
			expectError: false,
		},
		{
			description: "valid: states with maxPodLifeTimeSeconds",
			args:        &PodLifeTimeArgs{States: []string{"Running"}, MaxPodLifeTimeSeconds: utilptr.To[uint](600)},
			expectError: false,
		},
		{
			description: "valid: states with conditions",
			args:        &PodLifeTimeArgs{States: []string{"Succeeded"}, Conditions: []PodConditionFilter{{Reason: "PodCompleted"}}},
			expectError: false,
		},
		{
			description: "valid: exitCodes only",
			args:        &PodLifeTimeArgs{ExitCodes: []int32{1}},
			expectError: false,
		},
		{
			description: "valid: condition with minTimeSinceLastTransitionSeconds",
			args: &PodLifeTimeArgs{
				Conditions: []PodConditionFilter{{Reason: "PodCompleted", MinTimeSinceLastTransitionSeconds: utilptr.To[uint](300)}},
			},
			expectError: false,
		},
		{
			description: "valid: condition with only minTimeSinceLastTransitionSeconds",
			args: &PodLifeTimeArgs{
				Conditions: []PodConditionFilter{{MinTimeSinceLastTransitionSeconds: utilptr.To[uint](300)}},
			},
			expectError: false,
		},
		{
			description: "invalid: no filter criteria",
			args:        &PodLifeTimeArgs{},
			expectError: true,
		},
		{
			description: "invalid: both include and exclude namespaces",
			args: &PodLifeTimeArgs{
				States:     []string{"Running"},
				Namespaces: &api.Namespaces{Include: []string{"a"}, Exclude: []string{"b"}},
			},
			expectError: true,
		},
		{
			description: "invalid: both include and exclude ownerKinds",
			args: &PodLifeTimeArgs{
				States:     []string{"Running"},
				OwnerKinds: &OwnerKinds{Include: []string{"Job"}, Exclude: []string{"ReplicaSet"}},
			},
			expectError: true,
		},
		{
			description: "invalid: bad state name",
			args:        &PodLifeTimeArgs{States: []string{"NotAState"}},
			expectError: true,
		},
		{
			description: "invalid: empty condition filter",
			args: &PodLifeTimeArgs{
				Conditions: []PodConditionFilter{{}},
			},
			expectError: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			err := ValidatePodLifeTimeArgs(tc.args)
			if tc.expectError && err == nil {
				t.Error("Expected validation error but got nil")
			}
			if !tc.expectError && err != nil {
				t.Errorf("Expected no validation error but got: %v", err)
			}
		})
	}
}
