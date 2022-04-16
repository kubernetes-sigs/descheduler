/*
Copyright 2018 The Kubernetes Authors.

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
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"sigs.k8s.io/descheduler/pkg/api"
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

func initPods(node *v1.Node) []*v1.Pod {
	pods := make([]*v1.Pod, 0)

	for i := int32(0); i <= 9; i++ {
		pod := test.BuildTestPod(fmt.Sprintf("pod-%d", i), 100, 0, node.Name, nil)
		pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()

		// pod at index i will have 25 * i restarts.
		pod.Status = v1.PodStatus{
			InitContainerStatuses: []v1.ContainerStatus{
				{
					RestartCount: 5 * i,
				},
			},
			ContainerStatuses: []v1.ContainerStatus{
				{
					RestartCount: 10 * i,
				},
				{
					RestartCount: 10 * i,
				},
			},
		}
		pods = append(pods, pod)
	}

	// The following 3 pods won't get evicted.
	// A daemonset.
	pods[6].ObjectMeta.OwnerReferences = test.GetDaemonSetOwnerRefList()
	// A pod with local storage.
	pods[7].ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
	pods[7].Spec.Volumes = []v1.Volume{
		{
			Name: "sample",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{Path: "somePath"},
				EmptyDir: &v1.EmptyDirVolumeSource{
					SizeLimit: resource.NewQuantity(int64(10), resource.BinarySI)},
			},
		},
	}
	// A Mirror Pod.
	pods[8].Annotations = test.GetMirrorPodAnnotation()

	return pods
}

func TestRemovePodsHavingTooManyRestarts(t *testing.T) {
	node1 := test.BuildTestNode("node1", 2000, 3000, 10, nil)
	node2 := test.BuildTestNode("node2", 2000, 3000, 10, func(node *v1.Node) {
		node.Spec.Taints = []v1.Taint{
			{
				Key:    "hardware",
				Value:  "gpu",
				Effect: v1.TaintEffectNoSchedule,
			},
		}
	})
	node3 := test.BuildTestNode("node3", 2000, 3000, 10, func(node *v1.Node) {
		node.Spec = v1.NodeSpec{
			Unschedulable: true,
		}
	})

	pods := initPods(node1)

	var uint3 uint = 3

	tests := []struct {
		description                    string
		nodes                          []*v1.Node
		strategy                       api.DeschedulerStrategy
		expectedEvictedPodCount        uint
		maxPodsToEvictPerNode          *uint
		maxNoOfPodsToEvictPerNamespace *uint

		includingInitContainers bool
		podRestartThreshold     int32
		nodeFit                 bool
	}{
		{
			description:             "All pods have total restarts under threshold, no pod evictions",
			includingInitContainers: true,
			podRestartThreshold:     10000,
			nodes:                   []*v1.Node{node1},
			expectedEvictedPodCount: 0,
		},
		{
			description:             "Some pods have total restarts bigger than threshold",
			includingInitContainers: true,
			podRestartThreshold:     1,
			nodes:                   []*v1.Node{node1},
			expectedEvictedPodCount: 6,
		},
		{
			description:             "Nine pods have total restarts equals threshold(includingInitContainers=true), 6 pod evictions",
			includingInitContainers: true,
			podRestartThreshold:     1 * 25,
			nodes:                   []*v1.Node{node1},
			expectedEvictedPodCount: 6,
		},
		{
			description:             "Nine pods have total restarts equals threshold(includingInitContainers=false), 5 pod evictions",
			podRestartThreshold:     1 * 25,
			nodes:                   []*v1.Node{node1},
			expectedEvictedPodCount: 5,
		},
		{
			description:             "All pods have total restarts equals threshold(includingInitContainers=true), 6 pod evictions",
			includingInitContainers: true,
			podRestartThreshold:     1 * 20,
			nodes:                   []*v1.Node{node1},
			expectedEvictedPodCount: 6,
		},
		{
			description:             "Nine pods have total restarts equals threshold(includingInitContainers=false), 6 pod evictions",
			podRestartThreshold:     1 * 20,
			nodes:                   []*v1.Node{node1},
			expectedEvictedPodCount: 6,
		},
		{
			description:             "Five pods have total restarts bigger than threshold(includingInitContainers=true), but only 1 pod eviction",
			includingInitContainers: true,
			podRestartThreshold:     5*25 + 1,
			nodes:                   []*v1.Node{node1},
			expectedEvictedPodCount: 1,
		},
		{
			description:             "Five pods have total restarts bigger than threshold(includingInitContainers=false), but only 1 pod eviction",
			podRestartThreshold:     5*20 + 1,
			nodes:                   []*v1.Node{node1},
			expectedEvictedPodCount: 1,
		},
		{
			description:             "All pods have total restarts equals threshold(maxPodsToEvictPerNode=3), 3 pod evictions",
			includingInitContainers: true,
			podRestartThreshold:     1,
			nodes:                   []*v1.Node{node1},
			expectedEvictedPodCount: 3,
			maxPodsToEvictPerNode:   &uint3,
		},
		{
			description:                    "All pods have total restarts equals threshold(maxNoOfPodsToEvictPerNamespace=3), 3 pod evictions",
			includingInitContainers:        true,
			podRestartThreshold:            1,
			nodes:                          []*v1.Node{node1},
			expectedEvictedPodCount:        3,
			maxNoOfPodsToEvictPerNamespace: &uint3,
		},
		{
			description:             "All pods have total restarts equals threshold(maxPodsToEvictPerNode=3) but the only other node is tained, 0 pod evictions",
			includingInitContainers: true,
			podRestartThreshold:     1,
			nodeFit:                 true,
			nodes:                   []*v1.Node{node1, node2},
			expectedEvictedPodCount: 0,
			maxPodsToEvictPerNode:   &uint3,
		},
		{
			description:             "All pods have total restarts equals threshold(maxPodsToEvictPerNode=3) but the only other node is not schedulable, 0 pod evictions",
			includingInitContainers: true,
			podRestartThreshold:     1,
			nodeFit:                 true,
			nodes:                   []*v1.Node{node1, node3},
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
			for _, pod := range pods {
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

			plugin, err := New(&framework.RemovePodsHavingTooManyRestartsArgs{
				PodRestartThreshold:     tc.podRestartThreshold,
				IncludingInitContainers: tc.includingInitContainers,
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
