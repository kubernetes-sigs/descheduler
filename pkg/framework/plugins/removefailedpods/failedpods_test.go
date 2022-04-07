package removefailedpods

import (
	"context"
	"testing"

	v1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/framework"
	"sigs.k8s.io/descheduler/test"
)

var (
	OneHourInSeconds uint = 3600
)

type frameworkHandle struct {
	clientset                 clientset.Interface
	podEvictor                *evictions.PodEvictor
	getPodsAssignedToNodeFunc podutil.GetPodsAssignedToNodeFunc
}

func (f frameworkHandle) ClientSet() clientset.Interface {
	return f.clientset
}
func (f frameworkHandle) PodEvictor() *evictions.PodEvictor {
	return f.podEvictor
}
func (f frameworkHandle) GetPodsAssignedToNodeFunc() podutil.GetPodsAssignedToNodeFunc {
	return f.getPodsAssignedToNodeFunc
}

func TestRemoveFailedPods(t *testing.T) {
	tests := []struct {
		description             string
		nodes                   []*v1.Node
		strategy                api.DeschedulerStrategy
		expectedEvictedPodCount uint
		pods                    []*v1.Pod

		includingInitContainers bool
		reasons                 []string
		excludeOwnerKinds       []string
		minPodLifetimeSeconds   *uint
		nodeFit                 bool
	}{
		{
			description:             "0 failures, 0 evictions",
			nodes:                   []*v1.Node{test.BuildTestNode("node1", 2000, 3000, 10, nil)},
			expectedEvictedPodCount: 0,
			pods:                    []*v1.Pod{}, // no pods come back with field selector phase=Failed
		},
		{
			description:             "1 container terminated with reason NodeAffinity, 1 eviction",
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
			includingInitContainers: true,
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
			includingInitContainers: true,
			nodes:                   []*v1.Node{test.BuildTestNode("node1", 2000, 3000, 10, nil)},
			expectedEvictedPodCount: 1,
			pods: []*v1.Pod{
				buildTestPod("p1", "node1", newPodStatus("", "", &v1.ContainerState{
					Waiting: &v1.ContainerStateWaiting{Reason: "CreateContainerConfigError"},
				}, nil), nil),
			},
		},
		{
			description:             "2 init container waiting with reason CreateContainerConfigError, 2 nodes, 2 evictions",
			includingInitContainers: true,
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
			reasons:                 []string{"CreateContainerConfigError"},
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
			reasons:                 []string{"CreateContainerConfigError", "NodeAffinity"},
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
			reasons:                 []string{"CreateContainerConfigError"},
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
			reasons:                 []string{"CreateContainerConfigError"},
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
			minPodLifetimeSeconds:   &OneHourInSeconds,
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
			nodeFit:     true,
			nodes: []*v1.Node{test.BuildTestNode("node1", 2000, 3000, 10, func(node *v1.Node) {
				node.Spec.Unschedulable = true
			})},
			expectedEvictedPodCount: 0,
			pods: []*v1.Pod{
				buildTestPod("p1", "node1", newPodStatus("", "", nil, &v1.ContainerState{
					Terminated: &v1.ContainerStateTerminated{Reason: "NodeAffinity"},
				}), nil),
			},
		},
		{
			description:             "excluded owner kind=ReplicaSet, 1 init container terminated with owner kind=ReplicaSet, 0 eviction",
			includingInitContainers: true,
			excludeOwnerKinds:       []string{"ReplicaSet"},
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
			includingInitContainers: true,
			excludeOwnerKinds:       []string{"DaemonSet"},
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
			includingInitContainers: true,
			excludeOwnerKinds:       []string{"DaemonSet"},
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
			nodeFit:                 true,
			nodes:                   []*v1.Node{test.BuildTestNode("node1", 2000, 3000, 10, nil)},
			expectedEvictedPodCount: 0,
			pods: []*v1.Pod{
				buildTestPod("p1", "node1", newPodStatus("Shutdown", v1.PodFailed, nil, nil), nil),
			},
		},
		{
			description:             "include reason=Shutdown, 2 containers terminated with reason ShutDown, 2 evictions",
			reasons:                 []string{"Shutdown"},
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

			podEvictor := evictions.NewPodEvictor(
				fakeClient,
				policyv1.SchemeGroupVersion.String(),
				false,
				nil,
				nil,
				tc.nodes,
				false,
				false,
				false,
				false,
				false,
			)

			plugin, err := New(&framework.RemoveFailedPodsArg{
				CommonArgs: framework.CommonArgs{
					NodeFit: tc.nodeFit,
				},
				ExcludeOwnerKinds:       tc.excludeOwnerKinds,
				MinPodLifetimeSeconds:   tc.minPodLifetimeSeconds,
				Reasons:                 tc.reasons,
				IncludingInitContainers: tc.includingInitContainers,
			},
				frameworkHandle{
					clientset:                 fakeClient,
					podEvictor:                podEvictor,
					getPodsAssignedToNodeFunc: getPodsAssignedToNode,
				},
			)
			if err != nil {
				t.Fatalf("Unable to initialize the plugin: %v", err)
			}

			plugin.(interface{}).(framework.DeschedulePlugin).Deschedule(ctx, tc.nodes)
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
