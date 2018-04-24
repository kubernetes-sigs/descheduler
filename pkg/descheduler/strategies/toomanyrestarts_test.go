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
	"testing"

	"fmt"
	"github.com/kubernetes-incubator/descheduler/cmd/descheduler/app/options"
	"github.com/kubernetes-incubator/descheduler/pkg/api"
	"github.com/kubernetes-incubator/descheduler/pkg/apis/componentconfig"
	"github.com/kubernetes-incubator/descheduler/test"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
)

func initPods(node *v1.Node) []v1.Pod {

	pods := make([]v1.Pod, 0)

	var i int32 = 0
	for i <= 10 {
		pod := test.BuildTestPod(fmt.Sprintf("pod-%d", i), 100, 0, node.Name)
		pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()

		// pod i will has 25 * i restarts.
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
		i++
	}

	// The following 4 pods won't get evicted.
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
	// A Critical Pod.
	pods[9].Namespace = "kube-system"
	pods[9].Annotations = test.GetCriticalPodAnnotation()

	return pods
}

func TestRemovePodsHavingTooManyRestarts(t *testing.T) {

	createStrategy := func(enabled, includingInitContainers bool, restartThresholds int32) api.DeschedulerStrategy {
		return api.DeschedulerStrategy{
			Enabled: enabled,
			Params: api.StrategyParameters{
				PodsHavingTooManyRestarts: api.PodsHavingTooManyRestarts{
					PodeRestartThresholds:   restartThresholds,
					IncludingInitContainers: includingInitContainers,
				},
			},
		}
	}

	node := test.BuildTestNode("node1", 2000, 3000, 10)
	pods := initPods(node)

	fakeClient := &fake.Clientset{}

	fakeClient.Fake.AddReactor("list", "pods", func(action core.Action) (bool, runtime.Object, error) {
		return true, &v1.PodList{Items: pods}, nil
	})

	tests := []struct {
		description             string
		pods                    []v1.Pod
		strategy                api.DeschedulerStrategy
		expectedEvictedPodCount int
		maxPodsToEvict          int
	}{
		{
			description: "Not a PodsHavingTooManyRestarts strategy, no pod evictions",
			strategy: api.DeschedulerStrategy{
				Enabled: true,
				Params: api.StrategyParameters{
					NodeAffinityType: []string{
						"requiredDuringSchedulingRequiredDuringExecution",
					},
				},
			},
			expectedEvictedPodCount: 0,
			maxPodsToEvict:          0,
		},
		{
			description:             "Strategy is disabled, no pod evictions",
			strategy:                createStrategy(false, true, 10),
			expectedEvictedPodCount: 0,
			maxPodsToEvict:          0,
		},
		{
			description:             "All pods have total restarts under threshold, no pod evictions",
			strategy:                createStrategy(true, true, 10000),
			expectedEvictedPodCount: 0,
			maxPodsToEvict:          0,
		},
		{
			description:             "One pod have total restarts bigger than threshold, 6 pod evictions",
			strategy:                createStrategy(true, true, 1),
			expectedEvictedPodCount: 6,
			maxPodsToEvict:          0,
		},
		{
			description:             "Nine pods have total restarts equals threshold(includingInitContainers=true), 5 pods evictions",
			strategy:                createStrategy(true, true, 1*25),
			expectedEvictedPodCount: 5,
			maxPodsToEvict:          0,
		},
		{
			description:             "Nine pods have total restarts equals threshold(includingInitContainers=false), 5 pods evictions",
			strategy:                createStrategy(true, false, 1*25),
			expectedEvictedPodCount: 5,
			maxPodsToEvict:          0,
		},
		{
			description:             "All pods have total restarts equals threshold(includingInitContainers=true), 6 pods evictions",
			strategy:                createStrategy(true, true, 1*20),
			expectedEvictedPodCount: 6,
			maxPodsToEvict:          0,
		},
		{
			description:             "Nine pods have total restarts equals threshold(includingInitContainers=false), 5 pods evictions",
			strategy:                createStrategy(true, false, 1*20),
			expectedEvictedPodCount: 5,
			maxPodsToEvict:          0,
		},
		{
			description:             "Five pods have total restarts bigger than threshold(includingInitContainers=true), but only 1 pod eviction",
			strategy:                createStrategy(true, true, 5*25+1),
			expectedEvictedPodCount: 1,
			maxPodsToEvict:          0,
		},
		{
			description:             "Five pods have total restarts bigger than threshold(includingInitContainers=false), but only 1 pod eviction",
			strategy:                createStrategy(true, false, 5*20+1),
			expectedEvictedPodCount: 1,
			maxPodsToEvict:          0,
		},
		{
			description:             "All pods have total restarts equals threshold(maxPodsToEvict=3), 3 pods evictions",
			strategy:                createStrategy(true, true, 1),
			expectedEvictedPodCount: 3,
			maxPodsToEvict:          3,
		},
	}

	for _, tc := range tests {

		ds := options.DeschedulerServer{
			DeschedulerConfiguration: componentconfig.DeschedulerConfiguration{
				MaxNoOfPodsToEvictPerNode: tc.maxPodsToEvict,
			},
			Client: fakeClient,
		}

		nodePodCount := InitializeNodePodCount([]*v1.Node{node})

		actualEvictedPodCount := removePodsHavingTooManyRestarts(&ds, tc.strategy, "v1", []*v1.Node{node}, nodePodCount)
		if actualEvictedPodCount != tc.expectedEvictedPodCount {
			t.Errorf("Test %#v failed, expected %v pod evictions, but got %v pod evictions\n", tc.description, tc.expectedEvictedPodCount, actualEvictedPodCount)
		}
	}

}
