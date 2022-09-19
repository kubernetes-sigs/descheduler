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

package removefailedpods

import (
	"context"
	"testing"

	v1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/descheduler/pkg/framework"

	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	frameworkfake "sigs.k8s.io/descheduler/pkg/framework/fake"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	"sigs.k8s.io/descheduler/test"
)

var (
	OneHourInSeconds uint = 3600
)

func TestRemoveFailedPods(t *testing.T) {
	createRemoveFailedPodsArgs := func(
		includingInitContainers bool,
		reasons, excludeKinds []string,
		minAgeSeconds *uint) RemoveFailedPodsArgs {
		return RemoveFailedPodsArgs{
			IncludingInitContainers: includingInitContainers,
			Reasons:                 reasons,
			MinPodLifetimeSeconds:   minAgeSeconds,
			ExcludeOwnerKinds:       excludeKinds,
		}
	}

	tests := []struct {
		description             string
		nodes                   []*v1.Node
		args                    RemoveFailedPodsArgs
		expectedEvictedPodCount uint
		pods                    []*v1.Pod
		nodeFit                 bool
	}{
		{
			description:             "default empty args, 0 failures, 0 evictions",
			args:                    RemoveFailedPodsArgs{},
			nodes:                   []*v1.Node{test.BuildTestNode("node1", 2000, 3000, 10, nil)},
			expectedEvictedPodCount: 0,
			pods:                    []*v1.Pod{}, // no pods come back with field selector phase=Failed
		},
		{
			description:             "0 failures, 0 evictions",
			args:                    createRemoveFailedPodsArgs(false, nil, nil, nil),
			nodes:                   []*v1.Node{test.BuildTestNode("node1", 2000, 3000, 10, nil)},
			expectedEvictedPodCount: 0,
			pods:                    []*v1.Pod{}, // no pods come back with field selector phase=Failed
		},
		{
			description:             "1 container terminated with reason NodeAffinity, 1 eviction",
			args:                    createRemoveFailedPodsArgs(false, nil, nil, nil),
			nodes:                   []*v1.Node{test.BuildTestNode("node1", 2000, 3000, 10, nil)},
			expectedEvictedPodCount: 1,
			pods: []*v1.Pod{
				buildTestPod("p1", "node1", newPodStatus("", "", nil, &v1.ContainerState{
					Terminated: &v1.ContainerStateTerminated{Reason: "NodeAffinity"},
				}), nil),
			},
		},
		{
			description:             "1 init container terminated with reason NodeAffinity, 1 eviction",
			args:                    createRemoveFailedPodsArgs(true, nil, nil, nil),
			nodes:                   []*v1.Node{test.BuildTestNode("node1", 2000, 3000, 10, nil)},
			expectedEvictedPodCount: 1,
			pods: []*v1.Pod{
				buildTestPod("p1", "node1", newPodStatus("", "", &v1.ContainerState{
					Terminated: &v1.ContainerStateTerminated{Reason: "NodeAffinity"},
				}, nil), nil),
			},
		},
		{
			description:             "1 init container waiting with reason CreateContainerConfigError, 1 eviction",
			args:                    createRemoveFailedPodsArgs(true, nil, nil, nil),
			nodes:                   []*v1.Node{test.BuildTestNode("node1", 2000, 3000, 10, nil)},
			expectedEvictedPodCount: 1,
			pods: []*v1.Pod{
				buildTestPod("p1", "node1", newPodStatus("", "", &v1.ContainerState{
					Waiting: &v1.ContainerStateWaiting{Reason: "CreateContainerConfigError"},
				}, nil), nil),
			},
		},
		{
			description: "2 init container waiting with reason CreateContainerConfigError, 2 nodes, 2 evictions",
			args:        createRemoveFailedPodsArgs(true, nil, nil, nil),
			nodes: []*v1.Node{
				test.BuildTestNode("node1", 2000, 3000, 10, nil),
				test.BuildTestNode("node2", 2000, 3000, 10, nil),
			},
			expectedEvictedPodCount: 2,
			pods: []*v1.Pod{
				buildTestPod("p1", "node1", newPodStatus("", "", &v1.ContainerState{
					Terminated: &v1.ContainerStateTerminated{Reason: "CreateContainerConfigError"},
				}, nil), nil),
				buildTestPod("p2", "node2", newPodStatus("", "", &v1.ContainerState{
					Terminated: &v1.ContainerStateTerminated{Reason: "CreateContainerConfigError"},
				}, nil), nil),
			},
		},
		{
			description:             "include reason=CreateContainerConfigError, 1 container terminated with reason CreateContainerConfigError, 1 eviction",
			args:                    createRemoveFailedPodsArgs(false, []string{"CreateContainerConfigError"}, nil, nil),
			nodes:                   []*v1.Node{test.BuildTestNode("node1", 2000, 3000, 10, nil)},
			expectedEvictedPodCount: 1,
			pods: []*v1.Pod{
				buildTestPod("p1", "node1", newPodStatus("", "", nil, &v1.ContainerState{
					Terminated: &v1.ContainerStateTerminated{Reason: "CreateContainerConfigError"},
				}), nil),
			},
		},
		{
			description:             "include reason=CreateContainerConfigError+NodeAffinity, 1 container terminated with reason CreateContainerConfigError, 1 eviction",
			args:                    createRemoveFailedPodsArgs(false, []string{"CreateContainerConfigError", "NodeAffinity"}, nil, nil),
			nodes:                   []*v1.Node{test.BuildTestNode("node1", 2000, 3000, 10, nil)},
			expectedEvictedPodCount: 1,
			pods: []*v1.Pod{
				buildTestPod("p1", "node1", newPodStatus("", "", nil, &v1.ContainerState{
					Terminated: &v1.ContainerStateTerminated{Reason: "CreateContainerConfigError"},
				}), nil),
			},
		},
		{
			description:             "include reason=CreateContainerConfigError, 1 container terminated with reason NodeAffinity, 0 eviction",
			args:                    createRemoveFailedPodsArgs(false, []string{"CreateContainerConfigError"}, nil, nil),
			nodes:                   []*v1.Node{test.BuildTestNode("node1", 2000, 3000, 10, nil)},
			expectedEvictedPodCount: 0,
			pods: []*v1.Pod{
				buildTestPod("p1", "node1", newPodStatus("", "", nil, &v1.ContainerState{
					Terminated: &v1.ContainerStateTerminated{Reason: "NodeAffinity"},
				}), nil),
			},
		},
		{
			description:             "include init container=false, 1 init container waiting with reason CreateContainerConfigError, 0 eviction",
			args:                    createRemoveFailedPodsArgs(false, []string{"CreateContainerConfigError"}, nil, nil),
			nodes:                   []*v1.Node{test.BuildTestNode("node1", 2000, 3000, 10, nil)},
			expectedEvictedPodCount: 0,
			pods: []*v1.Pod{
				buildTestPod("p1", "node1", newPodStatus("", "", &v1.ContainerState{
					Waiting: &v1.ContainerStateWaiting{Reason: "CreateContainerConfigError"},
				}, nil), nil),
			},
		},
		{
			description:             "lifetime 1 hour, 1 container terminated with reason NodeAffinity, 0 eviction",
			args:                    createRemoveFailedPodsArgs(false, nil, nil, &OneHourInSeconds),
			nodes:                   []*v1.Node{test.BuildTestNode("node1", 2000, 3000, 10, nil)},
			expectedEvictedPodCount: 0,
			pods: []*v1.Pod{
				buildTestPod("p1", "node1", newPodStatus("", "", nil, &v1.ContainerState{
					Terminated: &v1.ContainerStateTerminated{Reason: "NodeAffinity"},
				}), nil),
			},
		},
		{
			description: "nodeFit=true, 1 unschedulable node, 1 container terminated with reason NodeAffinity, 0 eviction",
			args:        createRemoveFailedPodsArgs(false, nil, nil, nil),
			nodes: []*v1.Node{
				test.BuildTestNode("node1", 2000, 3000, 10, nil),
				test.BuildTestNode("node2", 2000, 2000, 10, func(node *v1.Node) {
					node.Spec.Unschedulable = true
				}),
			},
			expectedEvictedPodCount: 0,
			pods: []*v1.Pod{
				buildTestPod("p1", "node1", newPodStatus("", "", nil, &v1.ContainerState{
					Terminated: &v1.ContainerStateTerminated{Reason: "NodeAffinity"},
				}), nil),
			},
			nodeFit: true,
		},
		{
			description:             "nodeFit=true, only available node does not have enough resources, 1 container terminated with reason CreateContainerConfigError, 0 eviction",
			args:                    createRemoveFailedPodsArgs(false, []string{"CreateContainerConfigError"}, nil, nil),
			nodes:                   []*v1.Node{test.BuildTestNode("node1", 1, 1, 10, nil), test.BuildTestNode("node2", 0, 0, 10, nil)},
			expectedEvictedPodCount: 0,
			pods: []*v1.Pod{
				buildTestPod("p1", "node1", newPodStatus("", "", nil, &v1.ContainerState{
					Terminated: &v1.ContainerStateTerminated{Reason: "CreateContainerConfigError"},
				}), nil),
			},
			nodeFit: true,
		},
		{
			description:             "excluded owner kind=ReplicaSet, 1 init container terminated with owner kind=ReplicaSet, 0 eviction",
			args:                    createRemoveFailedPodsArgs(true, nil, []string{"ReplicaSet"}, nil),
			nodes:                   []*v1.Node{test.BuildTestNode("node1", 2000, 3000, 10, nil)},
			expectedEvictedPodCount: 0,
			pods: []*v1.Pod{
				buildTestPod("p1", "node1", newPodStatus("", "", &v1.ContainerState{
					Terminated: &v1.ContainerStateTerminated{Reason: "NodeAffinity"},
				}, nil), nil),
			},
		},
		{
			description:             "excluded owner kind=DaemonSet, 1 init container terminated with owner kind=ReplicaSet, 1 eviction",
			args:                    createRemoveFailedPodsArgs(true, nil, []string{"DaemonSet"}, nil),
			nodes:                   []*v1.Node{test.BuildTestNode("node1", 2000, 3000, 10, nil)},
			expectedEvictedPodCount: 1,
			pods: []*v1.Pod{
				buildTestPod("p1", "node1", newPodStatus("", "", &v1.ContainerState{
					Terminated: &v1.ContainerStateTerminated{Reason: "NodeAffinity"},
				}, nil), nil),
			},
		},
		{
			description:             "excluded owner kind=DaemonSet, 1 init container terminated with owner kind=ReplicaSet, 1 pod in termination; nothing should be moved",
			args:                    createRemoveFailedPodsArgs(true, nil, []string{"DaemonSet"}, nil),
			nodes:                   []*v1.Node{test.BuildTestNode("node1", 2000, 3000, 10, nil)},
			expectedEvictedPodCount: 0,
			pods: []*v1.Pod{
				buildTestPod("p1", "node1", newPodStatus("", "", &v1.ContainerState{
					Terminated: &v1.ContainerStateTerminated{Reason: "NodeAffinity"},
				}, nil), &metav1.Time{}),
			},
		},
		{
			description:             "1 container terminated with reason ShutDown, 0 evictions",
			args:                    createRemoveFailedPodsArgs(false, nil, nil, nil),
			nodes:                   []*v1.Node{test.BuildTestNode("node1", 2000, 3000, 10, nil)},
			expectedEvictedPodCount: 0,
			pods: []*v1.Pod{
				buildTestPod("p1", "node1", newPodStatus("Shutdown", v1.PodFailed, nil, nil), nil),
			},
			nodeFit: true,
		},
		{
			description:             "include reason=Shutdown, 2 containers terminated with reason ShutDown, 2 evictions",
			args:                    createRemoveFailedPodsArgs(false, []string{"Shutdown"}, nil, nil),
			nodes:                   []*v1.Node{test.BuildTestNode("node1", 2000, 3000, 10, nil)},
			expectedEvictedPodCount: 2,
			pods: []*v1.Pod{
				buildTestPod("p1", "node1", newPodStatus("Shutdown", v1.PodFailed, nil, nil), nil),
				buildTestPod("p2", "node1", newPodStatus("Shutdown", v1.PodFailed, nil, nil), nil),
			},
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

			sharedInformerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
			podInformer := sharedInformerFactory.Core().V1().Pods()

			getPodsAssignedToNode, err := podutil.BuildGetPodsAssignedToNodeFunc(podInformer)
			if err != nil {
				t.Errorf("Build get pods assigned to node function error: %v", err)
			}

			sharedInformerFactory.Start(ctx.Done())
			sharedInformerFactory.WaitForCacheSync(ctx.Done())

			eventRecorder := &events.FakeRecorder{}

			podEvictor := evictions.NewPodEvictor(
				fakeClient,
				policyv1.SchemeGroupVersion.String(),
				false,
				nil,
				nil,
				tc.nodes,
				false,
				eventRecorder,
			)

			defaultevictorArgs := &defaultevictor.DefaultEvictorArgs{
				EvictLocalStoragePods:   false,
				EvictSystemCriticalPods: false,
				IgnorePvcPods:           false,
				EvictFailedBarePods:     false,
				NodeFit:                 tc.nodeFit,
			}

			evictorFilter, err := defaultevictor.New(
				defaultevictorArgs,
				&frameworkfake.HandleImpl{
					ClientsetImpl:                 fakeClient,
					GetPodsAssignedToNodeFuncImpl: getPodsAssignedToNode,
					SharedInformerFactoryImpl:     sharedInformerFactory,
				},
			)

			if err != nil {
				t.Fatalf("Unable to initialize the plugin: %v", err)
			}

			plugin, err := New(&RemoveFailedPodsArgs{
				Reasons:                 tc.args.Reasons,
				MinPodLifetimeSeconds:   tc.args.MinPodLifetimeSeconds,
				IncludingInitContainers: tc.args.IncludingInitContainers,
				ExcludeOwnerKinds:       tc.args.ExcludeOwnerKinds,
				LabelSelector:           tc.args.LabelSelector,
				Namespaces:              tc.args.Namespaces,
			},
				&frameworkfake.HandleImpl{
					ClientsetImpl:                 fakeClient,
					PodEvictorImpl:                podEvictor,
					EvictorFilterImpl:             evictorFilter.(framework.EvictorPlugin),
					SharedInformerFactoryImpl:     sharedInformerFactory,
					GetPodsAssignedToNodeFuncImpl: getPodsAssignedToNode,
				},
			)
			if err != nil {
				t.Fatalf("Unable to initialize the plugin: %v", err)
			}

			plugin.(framework.DeschedulePlugin).Deschedule(ctx, tc.nodes)
			actualEvictedPodCount := podEvictor.TotalEvicted()
			if actualEvictedPodCount != tc.expectedEvictedPodCount {
				t.Errorf("Test %#v failed, expected %v pod evictions, but got %v pod evictions\n", tc.description, tc.expectedEvictedPodCount, actualEvictedPodCount)
			}
		})
	}
}

func newPodStatus(reason string, phase v1.PodPhase, initContainerState, containerState *v1.ContainerState) v1.PodStatus {
	ps := v1.PodStatus{
		Reason: reason,
		Phase:  phase,
	}

	if initContainerState != nil {
		ps.InitContainerStatuses = []v1.ContainerStatus{{State: *initContainerState}}
		ps.Phase = v1.PodFailed
	}

	if containerState != nil {
		ps.ContainerStatuses = []v1.ContainerStatus{{State: *containerState}}
		ps.Phase = v1.PodFailed
	}

	return ps
}

func buildTestPod(podName, nodeName string, podStatus v1.PodStatus, deletionTimestamp *metav1.Time) *v1.Pod {
	pod := test.BuildTestPod(podName, 1, 1, nodeName, func(p *v1.Pod) {
		p.Status = podStatus
	})
	pod.ObjectMeta.OwnerReferences = test.GetReplicaSetOwnerRefList()
	pod.ObjectMeta.SetCreationTimestamp(metav1.Now())
	pod.DeletionTimestamp = deletionTimestamp
	return pod
}
