/*
Copyright 2020 The Kubernetes Authors.

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
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	"sigs.k8s.io/descheduler/test"
)

func TestPodLifeTime(t *testing.T) {
	ctx := context.Background()
	node := test.BuildTestNode("n1", 2000, 3000, 10, nil)
	olderPodCreationTime := metav1.NewTime(time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC))
	newerPodCreationTime := metav1.NewTime(time.Now())

	// Setup pods, one should be evicted
	p1 := test.BuildTestPod("p1", 100, 0, node.Name, nil)
	p1.Namespace = "dev"
	p1.ObjectMeta.CreationTimestamp = newerPodCreationTime
	p2 := test.BuildTestPod("p2", 100, 0, node.Name, nil)
	p2.Namespace = "dev"
	p2.ObjectMeta.CreationTimestamp = olderPodCreationTime

	ownerRef1 := test.GetReplicaSetOwnerRefList()
	p1.ObjectMeta.OwnerReferences = ownerRef1
	p2.ObjectMeta.OwnerReferences = ownerRef1

	// Setup pods, zero should be evicted
	p3 := test.BuildTestPod("p3", 100, 0, node.Name, nil)
	p3.Namespace = "dev"
	p3.ObjectMeta.CreationTimestamp = newerPodCreationTime
	p4 := test.BuildTestPod("p4", 100, 0, node.Name, nil)
	p4.Namespace = "dev"
	p4.ObjectMeta.CreationTimestamp = newerPodCreationTime

	ownerRef2 := test.GetReplicaSetOwnerRefList()
	p3.ObjectMeta.OwnerReferences = ownerRef2
	p4.ObjectMeta.OwnerReferences = ownerRef2

	// Setup pods, one should be evicted
	p5 := test.BuildTestPod("p5", 100, 0, node.Name, nil)
	p5.Namespace = "dev"
	p5.ObjectMeta.CreationTimestamp = newerPodCreationTime
	p6 := test.BuildTestPod("p6", 100, 0, node.Name, nil)
	p6.Namespace = "dev"
	p6.ObjectMeta.CreationTimestamp = metav1.NewTime(time.Now().Add(time.Second * 605))

	ownerRef3 := test.GetReplicaSetOwnerRefList()
	p5.ObjectMeta.OwnerReferences = ownerRef3
	p6.ObjectMeta.OwnerReferences = ownerRef3

	// Setup pods, zero should be evicted
	p7 := test.BuildTestPod("p7", 100, 0, node.Name, nil)
	p7.Namespace = "dev"
	p7.ObjectMeta.CreationTimestamp = newerPodCreationTime
	p8 := test.BuildTestPod("p8", 100, 0, node.Name, nil)
	p8.Namespace = "dev"
	p8.ObjectMeta.CreationTimestamp = metav1.NewTime(time.Now().Add(time.Second * 595))

	ownerRef4 := test.GetReplicaSetOwnerRefList()
	p5.ObjectMeta.OwnerReferences = ownerRef4
	p6.ObjectMeta.OwnerReferences = ownerRef4

	var maxLifeTime uint = 600
	testCases := []struct {
		description             string
		strategy                api.DeschedulerStrategy
		maxPodsToEvict          int
		pods                    []v1.Pod
		expectedEvictedPodCount int
	}{
		{
			description: "Two pods in the `dev` Namespace, 1 is new and 1 very is old. 1 should be evicted.",
			strategy: api.DeschedulerStrategy{
				Enabled: true,
				Params: api.StrategyParameters{
					MaxPodLifeTimeSeconds: &maxLifeTime,
				},
			},
			maxPodsToEvict:          5,
			pods:                    []v1.Pod{*p1, *p2},
			expectedEvictedPodCount: 1,
		},
		{
			description: "Two pods in the `dev` Namespace, 2 are new and 0 are old. 0 should be evicted.",
			strategy: api.DeschedulerStrategy{
				Enabled: true,
				Params: api.StrategyParameters{
					MaxPodLifeTimeSeconds: &maxLifeTime,
				},
			},
			maxPodsToEvict:          5,
			pods:                    []v1.Pod{*p3, *p4},
			expectedEvictedPodCount: 0,
		},
		{
			description: "Two pods in the `dev` Namespace, 1 created 605 seconds ago. 1 should be evicted.",
			strategy: api.DeschedulerStrategy{
				Enabled: true,
				Params: api.StrategyParameters{
					MaxPodLifeTimeSeconds: &maxLifeTime,
				},
			},
			maxPodsToEvict:          5,
			pods:                    []v1.Pod{*p5, *p6},
			expectedEvictedPodCount: 1,
		},
		{
			description: "Two pods in the `dev` Namespace, 1 created 595 seconds ago. 0 should be evicted.",
			strategy: api.DeschedulerStrategy{
				Enabled: true,
				Params: api.StrategyParameters{
					MaxPodLifeTimeSeconds: &maxLifeTime,
				},
			},
			maxPodsToEvict:          5,
			pods:                    []v1.Pod{*p7, *p8},
			expectedEvictedPodCount: 0,
		},
	}

	for _, tc := range testCases {
		fakeClient := &fake.Clientset{}
		fakeClient.Fake.AddReactor("list", "pods", func(action core.Action) (bool, runtime.Object, error) {
			return true, &v1.PodList{Items: tc.pods}, nil
		})
		fakeClient.Fake.AddReactor("get", "nodes", func(action core.Action) (bool, runtime.Object, error) {
			return true, node, nil
		})
		podEvictor := evictions.NewPodEvictor(
			fakeClient,
			"v1",
			false,
			false,
			tc.maxPodsToEvict,
			[]*v1.Node{node},
		)

		PodLifeTime(ctx, fakeClient, tc.strategy, []*v1.Node{node}, false, podEvictor)
		podsEvicted := podEvictor.TotalEvicted()
		if podsEvicted != tc.expectedEvictedPodCount {
			t.Errorf("Test error for description: %s. Expected evicted pods count %v, got %v", tc.description, tc.expectedEvictedPodCount, podsEvicted)
		}
	}

}
