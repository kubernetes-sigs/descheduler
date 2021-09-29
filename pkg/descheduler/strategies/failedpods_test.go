package strategies

import (
	"context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"

	v1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	"sigs.k8s.io/descheduler/test"
)

var (
	OneHourInSeconds uint = 3600
)

func TestRemoveFailedPods(t *testing.T) {
	ctx := context.Background()

	createStrategy := func(enabled, includingInitContainers bool, reasons, excludeKinds []string, minAgeSeconds *uint, nodeFit bool) api.DeschedulerStrategy {
		return api.DeschedulerStrategy{
			Enabled: enabled,
			Params: &api.StrategyParameters{
				FailedPods: &api.FailedPods{
					Reasons:                 reasons,
					IncludingInitContainers: includingInitContainers,
					ExcludeOwnerKinds:       excludeKinds,
					MinPodLifetimeSeconds:   minAgeSeconds,
				},
				NodeFit: nodeFit,
			},
		}
	}

	tests := []struct {
		description             string
		nodes                   []*v1.Node
		strategy                api.DeschedulerStrategy
		expectedEvictedPodCount int
		pods                    []v1.Pod
	}{
		{
			description:             "default empty strategy, 0 failures, 0 evictions",
			strategy:                api.DeschedulerStrategy{},
			nodes:                   []*v1.Node{test.BuildTestNode("node1", 2000, 3000, 10, nil)},
			expectedEvictedPodCount: 0,
			pods:                    []v1.Pod{}, // no pods come back with field selector phase=Failed
		},
		{
			description:             "0 failures, 0 evictions",
			strategy:                createStrategy(true, false, nil, nil, nil, false),
			nodes:                   []*v1.Node{test.BuildTestNode("node1", 2000, 3000, 10, nil)},
			expectedEvictedPodCount: 0,
			pods:                    []v1.Pod{}, // no pods come back with field selector phase=Failed
		},
		{
			description:             "1 container terminated with reason NodeAffinity, 1 eviction",
			strategy:                createStrategy(true, false, nil, nil, nil, false),
			nodes:                   []*v1.Node{test.BuildTestNode("node1", 2000, 3000, 10, nil)},
			expectedEvictedPodCount: 1,
			pods: []v1.Pod{
				buildTestPod("p1", "node1", nil, &v1.ContainerState{
					Terminated: &v1.ContainerStateTerminated{Reason: "NodeAffinity"},
				}),
			},
		},
		{
			description:             "1 init container terminated with reason NodeAffinity, 1 eviction",
			strategy:                createStrategy(true, true, nil, nil, nil, false),
			nodes:                   []*v1.Node{test.BuildTestNode("node1", 2000, 3000, 10, nil)},
			expectedEvictedPodCount: 1,
			pods: []v1.Pod{
				buildTestPod("p1", "node1", &v1.ContainerState{
					Terminated: &v1.ContainerStateTerminated{Reason: "NodeAffinity"},
				}, nil),
			},
		},
		{
			description:             "1 init container waiting with reason CreateContainerConfigError, 1 eviction",
			strategy:                createStrategy(true, true, nil, nil, nil, false),
			nodes:                   []*v1.Node{test.BuildTestNode("node1", 2000, 3000, 10, nil)},
			expectedEvictedPodCount: 1,
			pods: []v1.Pod{
				buildTestPod("p1", "node1", &v1.ContainerState{
					Waiting: &v1.ContainerStateWaiting{Reason: "CreateContainerConfigError"},
				}, nil),
			},
		},
		{
			description: "2 init container waiting with reason CreateContainerConfigError, 2 nodes, 2 evictions",
			strategy:    createStrategy(true, true, nil, nil, nil, false),
			nodes: []*v1.Node{
				test.BuildTestNode("node1", 2000, 3000, 10, nil),
				test.BuildTestNode("node2", 2000, 3000, 10, nil),
			},
			expectedEvictedPodCount: 2,
			pods: []v1.Pod{
				buildTestPod("p1", "node1", &v1.ContainerState{
					Terminated: &v1.ContainerStateTerminated{Reason: "CreateContainerConfigError"},
				}, nil),
				buildTestPod("p2", "node2", &v1.ContainerState{
					Terminated: &v1.ContainerStateTerminated{Reason: "CreateContainerConfigError"},
				}, nil),
			},
		},
		{
			description:             "include reason=CreateContainerConfigError, 1 container terminated with reason CreateContainerConfigError, 1 eviction",
			strategy:                createStrategy(true, false, []string{"CreateContainerConfigError"}, nil, nil, false),
			nodes:                   []*v1.Node{test.BuildTestNode("node1", 2000, 3000, 10, nil)},
			expectedEvictedPodCount: 1,
			pods: []v1.Pod{
				buildTestPod("p1", "node1", nil, &v1.ContainerState{
					Terminated: &v1.ContainerStateTerminated{Reason: "CreateContainerConfigError"},
				}),
			},
		},
		{
			description:             "include reason=CreateContainerConfigError+NodeAffinity, 1 container terminated with reason CreateContainerConfigError, 1 eviction",
			strategy:                createStrategy(true, false, []string{"CreateContainerConfigError", "NodeAffinity"}, nil, nil, false),
			nodes:                   []*v1.Node{test.BuildTestNode("node1", 2000, 3000, 10, nil)},
			expectedEvictedPodCount: 1,
			pods: []v1.Pod{
				buildTestPod("p1", "node1", nil, &v1.ContainerState{
					Terminated: &v1.ContainerStateTerminated{Reason: "CreateContainerConfigError"},
				}),
			},
		},
		{
			description:             "include reason=CreateContainerConfigError, 1 container terminated with reason NodeAffinity, 0 eviction",
			strategy:                createStrategy(true, false, []string{"CreateContainerConfigError"}, nil, nil, false),
			nodes:                   []*v1.Node{test.BuildTestNode("node1", 2000, 3000, 10, nil)},
			expectedEvictedPodCount: 0,
			pods: []v1.Pod{
				buildTestPod("p1", "node1", nil, &v1.ContainerState{
					Terminated: &v1.ContainerStateTerminated{Reason: "NodeAffinity"},
				}),
			},
		},
		{
			description:             "include init container=false, 1 init container waiting with reason CreateContainerConfigError, 0 eviction",
			strategy:                createStrategy(true, false, []string{"CreateContainerConfigError"}, nil, nil, false),
			nodes:                   []*v1.Node{test.BuildTestNode("node1", 2000, 3000, 10, nil)},
			expectedEvictedPodCount: 0,
			pods: []v1.Pod{
				buildTestPod("p1", "node1", &v1.ContainerState{
					Waiting: &v1.ContainerStateWaiting{Reason: "CreateContainerConfigError"},
				}, nil),
			},
		},
		{
			description:             "lifetime 1 hour, 1 container terminated with reason NodeAffinity, 0 eviction",
			strategy:                createStrategy(true, false, nil, nil, &OneHourInSeconds, false),
			nodes:                   []*v1.Node{test.BuildTestNode("node1", 2000, 3000, 10, nil)},
			expectedEvictedPodCount: 0,
			pods: []v1.Pod{
				buildTestPod("p1", "node1", nil, &v1.ContainerState{
					Terminated: &v1.ContainerStateTerminated{Reason: "NodeAffinity"},
				}),
			},
		},
		{
			description: "nodeFit=true, 1 unschedulable node, 1 container terminated with reason NodeAffinity, 0 eviction",
			strategy:    createStrategy(true, false, nil, nil, nil, true),
			nodes: []*v1.Node{test.BuildTestNode("node1", 2000, 3000, 10, func(node *v1.Node) {
				node.Spec.Unschedulable = true
			})},
			expectedEvictedPodCount: 0,
			pods: []v1.Pod{
				buildTestPod("p1", "node1", nil, &v1.ContainerState{
					Terminated: &v1.ContainerStateTerminated{Reason: "NodeAffinity"},
				}),
			},
		},
		{
			description:             "excluded owner kind=ReplicaSet, 1 init container terminated with owner kind=ReplicaSet, 0 eviction",
			strategy:                createStrategy(true, true, nil, []string{"ReplicaSet"}, nil, false),
			nodes:                   []*v1.Node{test.BuildTestNode("node1", 2000, 3000, 10, nil)},
			expectedEvictedPodCount: 0,
			pods: []v1.Pod{
				buildTestPod("p1", "node1", &v1.ContainerState{
					Terminated: &v1.ContainerStateTerminated{Reason: "NodeAffinity"},
				}, nil),
			},
		},
		{
			description:             "excluded owner kind=DaemonSet, 1 init container terminated with owner kind=ReplicaSet, 1 eviction",
			strategy:                createStrategy(true, true, nil, []string{"DaemonSet"}, nil, false),
			nodes:                   []*v1.Node{test.BuildTestNode("node1", 2000, 3000, 10, nil)},
			expectedEvictedPodCount: 1,
			pods: []v1.Pod{
				buildTestPod("p1", "node1", &v1.ContainerState{
					Terminated: &v1.ContainerStateTerminated{Reason: "NodeAffinity"},
				}, nil),
			},
		},
	}
	for _, tc := range tests {
		fakeClient := &fake.Clientset{}
		fakeClient.Fake.AddReactor("list", "pods", func(action core.Action) (bool, runtime.Object, error) {
			return true, &v1.PodList{Items: tc.pods}, nil
		})

		podEvictor := evictions.NewPodEvictor(
			fakeClient,
			policyv1.SchemeGroupVersion.String(),
			false,
			100,
			tc.nodes,
			false,
			false,
			false,
		)

		RemoveFailedPods(ctx, fakeClient, tc.strategy, tc.nodes, podEvictor)
		actualEvictedPodCount := podEvictor.TotalEvicted()
		if actualEvictedPodCount != tc.expectedEvictedPodCount {
			t.Errorf("Test %#v failed, expected %v pod evictions, but got %v pod evictions\n", tc.description, tc.expectedEvictedPodCount, actualEvictedPodCount)
		}
	}
}

func TestValidRemoveFailedPodsParams(t *testing.T) {
	ctx := context.Background()
	fakeClient := &fake.Clientset{}
	testCases := []struct {
		name   string
		params *api.StrategyParameters
	}{
		{name: "validate nil params", params: nil},
		{name: "validate empty params", params: &api.StrategyParameters{}},
		{name: "validate reasons params", params: &api.StrategyParameters{FailedPods: &api.FailedPods{
			Reasons: []string{"CreateContainerConfigError"},
		}}},
		{name: "validate includingInitContainers params", params: &api.StrategyParameters{FailedPods: &api.FailedPods{
			IncludingInitContainers: true,
		}}},
		{name: "validate excludeOwnerKinds params", params: &api.StrategyParameters{FailedPods: &api.FailedPods{
			ExcludeOwnerKinds: []string{"Job"},
		}}},
		{name: "validate excludeOwnerKinds params", params: &api.StrategyParameters{FailedPods: &api.FailedPods{
			MinPodLifetimeSeconds: &OneHourInSeconds,
		}}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			params, err := validateAndParseRemoveFailedPodsParams(ctx, fakeClient, tc.params)
			if err != nil {
				t.Errorf("strategy params should be valid but got err: %v", err.Error())
			}
			if params == nil {
				t.Errorf("strategy params should return a ValidatedFailedPodsStrategyParams but got nil")
			}
		})
	}
}

func buildTestPod(podName, nodeName string, initContainerState, containerState *v1.ContainerState) v1.Pod {
	pod := test.BuildTestPod(podName, 1, 1, nodeName, func(p *v1.Pod) {
		ps := v1.PodStatus{}

		if initContainerState != nil {
			ps.InitContainerStatuses = []v1.ContainerStatus{{State: *initContainerState}}
			ps.Phase = v1.PodFailed
		}

		if containerState != nil {
			ps.ContainerStatuses = []v1.ContainerStatus{{State: *containerState}}
			ps.Phase = v1.PodFailed
		}

		p.Status = ps
	})
	pod.ObjectMeta.OwnerReferences = test.GetReplicaSetOwnerRefList()
	pod.ObjectMeta.SetCreationTimestamp(metav1.Now())
	return *pod
}
