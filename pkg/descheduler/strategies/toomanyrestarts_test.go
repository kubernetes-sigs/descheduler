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

package strategies

import (
	"context"
	"testing"

	"fmt"

	v1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	"sigs.k8s.io/descheduler/test"
)

func initPods(node *v1.Node) []v1.Pod {
	pods := make([]v1.Pod, 0)

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
		pods = append(pods, *pod)
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
	ctx := context.Background()

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

	createStrategy := func(enabled, includingInitContainers bool, restartThresholds int32, nodeFit bool) api.DeschedulerStrategy {
		return api.DeschedulerStrategy{
			Enabled: enabled,
			Params: &api.StrategyParameters{
				PodsHavingTooManyRestarts: &api.PodsHavingTooManyRestarts{
					PodRestartThreshold:     restartThresholds,
					IncludingInitContainers: includingInitContainers,
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
		maxPodsToEvictPerNode   int
	}{
		{
			description:             "All pods have total restarts under threshold, no pod evictions",
			strategy:                createStrategy(true, true, 10000, false),
			nodes:                   []*v1.Node{node1},
			expectedEvictedPodCount: 0,
			maxPodsToEvictPerNode:   0,
		},
		{
			description:             "Some pods have total restarts bigger than threshold",
			strategy:                createStrategy(true, true, 1, false),
			nodes:                   []*v1.Node{node1},
			expectedEvictedPodCount: 6,
			maxPodsToEvictPerNode:   0,
		},
		{
			description:             "Nine pods have total restarts equals threshold(includingInitContainers=true), 6 pod evictions",
			strategy:                createStrategy(true, true, 1*25, false),
			nodes:                   []*v1.Node{node1},
			expectedEvictedPodCount: 6,
			maxPodsToEvictPerNode:   0,
		},
		{
			description:             "Nine pods have total restarts equals threshold(includingInitContainers=false), 5 pod evictions",
			strategy:                createStrategy(true, false, 1*25, false),
			nodes:                   []*v1.Node{node1},
			expectedEvictedPodCount: 5,
			maxPodsToEvictPerNode:   0,
		},
		{
			description:             "All pods have total restarts equals threshold(includingInitContainers=true), 6 pod evictions",
			strategy:                createStrategy(true, true, 1*20, false),
			nodes:                   []*v1.Node{node1},
			expectedEvictedPodCount: 6,
			maxPodsToEvictPerNode:   0,
		},
		{
			description:             "Nine pods have total restarts equals threshold(includingInitContainers=false), 6 pod evictions",
			strategy:                createStrategy(true, false, 1*20, false),
			nodes:                   []*v1.Node{node1},
			expectedEvictedPodCount: 6,
			maxPodsToEvictPerNode:   0,
		},
		{
			description:             "Five pods have total restarts bigger than threshold(includingInitContainers=true), but only 1 pod eviction",
			strategy:                createStrategy(true, true, 5*25+1, false),
			nodes:                   []*v1.Node{node1},
			expectedEvictedPodCount: 1,
			maxPodsToEvictPerNode:   0,
		},
		{
			description:             "Five pods have total restarts bigger than threshold(includingInitContainers=false), but only 1 pod eviction",
			strategy:                createStrategy(true, false, 5*20+1, false),
			nodes:                   []*v1.Node{node1},
			expectedEvictedPodCount: 1,
			maxPodsToEvictPerNode:   0,
		},
		{
			description:             "All pods have total restarts equals threshold(maxPodsToEvictPerNode=3), 3 pod evictions",
			strategy:                createStrategy(true, true, 1, false),
			nodes:                   []*v1.Node{node1},
			expectedEvictedPodCount: 3,
			maxPodsToEvictPerNode:   3,
		},
		{
			description:             "All pods have total restarts equals threshold(maxPodsToEvictPerNode=3) but the only other node is tained, 0 pod evictions",
			strategy:                createStrategy(true, true, 1, true),
			nodes:                   []*v1.Node{node1, node2},
			expectedEvictedPodCount: 0,
			maxPodsToEvictPerNode:   3,
		},
		{
			description:             "All pods have total restarts equals threshold(maxPodsToEvictPerNode=3) but the only other node is not schedulable, 0 pod evictions",
			strategy:                createStrategy(true, true, 1, true),
			nodes:                   []*v1.Node{node1, node3},
			expectedEvictedPodCount: 0,
			maxPodsToEvictPerNode:   3,
		},
	}

	for _, tc := range tests {

		fakeClient := &fake.Clientset{}
		fakeClient.Fake.AddReactor("list", "pods", func(action core.Action) (bool, runtime.Object, error) {
			return true, &v1.PodList{Items: pods}, nil
		})

		podEvictor := evictions.NewPodEvictor(
			fakeClient,
			policyv1.SchemeGroupVersion.String(),
			false,
			tc.maxPodsToEvictPerNode,
			tc.nodes,
			false,
			false,
			false,
		)

		RemovePodsHavingTooManyRestarts(ctx, fakeClient, tc.strategy, tc.nodes, podEvictor)
		actualEvictedPodCount := podEvictor.TotalEvicted()
		if actualEvictedPodCount != tc.expectedEvictedPodCount {
			t.Errorf("Test %#v failed, expected %v pod evictions, but got %v pod evictions\n", tc.description, tc.expectedEvictedPodCount, actualEvictedPodCount)
		}
	}

}
