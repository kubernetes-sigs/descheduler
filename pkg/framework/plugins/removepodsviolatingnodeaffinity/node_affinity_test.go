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

package removepodsviolatingnodeaffinity

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

	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/framework"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	fakehandler "sigs.k8s.io/descheduler/pkg/framework/profile/fake"
	"sigs.k8s.io/descheduler/test"
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

func TestRemovePodsViolatingNodeAffinity(t *testing.T) {
	nodeLabelKey := "kubernetes.io/desiredNode"
	nodeLabelValue := "yes"
	nodeWithLabels := test.BuildTestNode("nodeWithLabels", 2000, 3000, 10, nil)
	nodeWithLabels.Labels[nodeLabelKey] = nodeLabelValue

	nodeWithoutLabels := test.BuildTestNode("nodeWithoutLabels", 2000, 3000, 10, nil)

	unschedulableNodeWithLabels := test.BuildTestNode("unschedulableNodeWithLabels", 2000, 3000, 10, nil)
	unschedulableNodeWithLabels.Labels[nodeLabelKey] = nodeLabelValue
	unschedulableNodeWithLabels.Spec.Unschedulable = true

	addPodsToNode := func(node *v1.Node, deletionTimestamp *metav1.Time) []*v1.Pod {
		podWithNodeAffinity := test.BuildTestPod("podWithNodeAffinity", 100, 0, node.Name, nil)
		podWithNodeAffinity.Spec.Affinity = &v1.Affinity{
			NodeAffinity: &v1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
					NodeSelectorTerms: []v1.NodeSelectorTerm{
						{
							MatchExpressions: []v1.NodeSelectorRequirement{
								{
									Key:      nodeLabelKey,
									Operator: "In",
									Values: []string{
										nodeLabelValue,
									},
								},
							},
						},
					},
				},
			},
		}

		pod1 := test.BuildTestPod("pod1", 100, 0, node.Name, nil)
		pod2 := test.BuildTestPod("pod2", 100, 0, node.Name, nil)

		podWithNodeAffinity.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
		pod1.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
		pod2.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()

		pod1.DeletionTimestamp = deletionTimestamp
		pod2.DeletionTimestamp = deletionTimestamp

		return []*v1.Pod{
			podWithNodeAffinity,
			pod1,
			pod2,
		}
	}

	var uint1 uint = 1
	tests := []struct {
		description                    string
		nodes                          []*v1.Node
		pods                           []*v1.Pod
		expectedEvictedPodCount        uint
		maxPodsToEvictPerNode          *uint
		maxNoOfPodsToEvictPerNamespace *uint

		nodeAffinityType []string
		nodeFit          bool
	}{
		{
			description:             "Invalid strategy type, should not evict any pods",
			nodeAffinityType:        []string{"requiredDuringSchedulingRequiredDuringExecution"},
			expectedEvictedPodCount: 0,
			pods:                    addPodsToNode(nodeWithoutLabels, nil),
			nodes:                   []*v1.Node{nodeWithoutLabels, nodeWithLabels},
		},
		{
			description:             "Pod is correctly scheduled on node, no eviction expected",
			nodeAffinityType:        []string{"requiredDuringSchedulingIgnoredDuringExecution"},
			expectedEvictedPodCount: 0,
			pods:                    addPodsToNode(nodeWithLabels, nil),
			nodes:                   []*v1.Node{nodeWithLabels},
		},
		{
			description:             "Pod is scheduled on node without matching labels, another schedulable node available, should be evicted",
			expectedEvictedPodCount: 1,
			nodeAffinityType:        []string{"requiredDuringSchedulingIgnoredDuringExecution"},
			pods:                    addPodsToNode(nodeWithoutLabels, nil),
			nodes:                   []*v1.Node{nodeWithoutLabels, nodeWithLabels},
		},
		{
			description:             "Pod is scheduled on node without matching labels, another schedulable node available, maxPodsToEvictPerNode set to 1, should not be evicted",
			expectedEvictedPodCount: 1,
			nodeAffinityType:        []string{"requiredDuringSchedulingIgnoredDuringExecution"},
			pods:                    addPodsToNode(nodeWithoutLabels, nil),
			nodes:                   []*v1.Node{nodeWithoutLabels, nodeWithLabels},
			maxPodsToEvictPerNode:   &uint1,
		},
		{
			description:             "Pod is scheduled on node without matching labels, another schedulable node available, maxPodsToEvictPerNode set to 1, no pod evicted since pod terminting",
			expectedEvictedPodCount: 1,
			nodeAffinityType:        []string{"requiredDuringSchedulingIgnoredDuringExecution"},
			pods:                    addPodsToNode(nodeWithoutLabels, &metav1.Time{}),
			nodes:                   []*v1.Node{nodeWithoutLabels, nodeWithLabels},
			maxPodsToEvictPerNode:   &uint1,
		},
		{
			description:                    "Pod is scheduled on node without matching labels, another schedulable node available, maxNoOfPodsToEvictPerNamespace set to 1, should not be evicted",
			expectedEvictedPodCount:        1,
			nodeAffinityType:               []string{"requiredDuringSchedulingIgnoredDuringExecution"},
			pods:                           addPodsToNode(nodeWithoutLabels, nil),
			nodes:                          []*v1.Node{nodeWithoutLabels, nodeWithLabels},
			maxNoOfPodsToEvictPerNamespace: &uint1,
		},
		{
			description:                    "Pod is scheduled on node without matching labels, another schedulable node available, maxNoOfPodsToEvictPerNamespace set to 1, no pod evicted since pod terminting",
			expectedEvictedPodCount:        1,
			nodeAffinityType:               []string{"requiredDuringSchedulingIgnoredDuringExecution"},
			pods:                           addPodsToNode(nodeWithoutLabels, &metav1.Time{}),
			nodes:                          []*v1.Node{nodeWithoutLabels, nodeWithLabels},
			maxNoOfPodsToEvictPerNamespace: &uint1,
		},
		{
			description:             "Pod is scheduled on node without matching labels, but no node where pod fits is available, should not evict",
			expectedEvictedPodCount: 0,
			nodeAffinityType:        []string{"requiredDuringSchedulingIgnoredDuringExecution"},
			nodeFit:                 true,
			pods:                    addPodsToNode(nodeWithoutLabels, nil),
			nodes:                   []*v1.Node{nodeWithoutLabels, unschedulableNodeWithLabels},
		},
		{
			description:             "Pod is scheduled on node without matching labels, and node where pod fits is available, should evict",
			expectedEvictedPodCount: 0,
			nodeAffinityType:        []string{"requiredDuringSchedulingIgnoredDuringExecution"},
			nodeFit:                 true,
			pods:                    addPodsToNode(nodeWithoutLabels, nil),
			nodes:                   []*v1.Node{nodeWithLabels, unschedulableNodeWithLabels},
			maxPodsToEvictPerNode:   &uint1,
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

			podEvictor := framework.NewPodEvictor(
				fakeClient,
				policyv1.SchemeGroupVersion.String(),
				false,
				tc.maxPodsToEvictPerNode,
				tc.maxNoOfPodsToEvictPerNamespace,
				false,
			)

			handle := &fakehandler.FrameworkHandle{
				ClientsetImpl:                 fakeClient,
				EvictorImpl:                   podEvictor,
				GetPodsAssignedToNodeFuncImpl: getPodsAssignedToNode,
				SharedInformerFactoryImpl:     sharedInformerFactory,
			}

			defaultEvictor, err := defaultevictor.New(&framework.DefaultEvictorArgs{
				NodeFit: tc.nodeFit,
			}, handle)
			if err != nil {
				t.Fatalf("Unable to initialize the default evictor: %v", err)
			}

			handle.EvictPlugin = defaultEvictor.(framework.EvictPlugin)
			handle.SortPlugin = defaultEvictor.(framework.SortPlugin)

			plugin, err := New(&framework.RemovePodsViolatingNodeAffinityArgs{
				NodeAffinityType: tc.nodeAffinityType,
			},
				handle,
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
