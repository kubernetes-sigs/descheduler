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

package strategies

import (
	"context"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	"sigs.k8s.io/descheduler/pkg/utils"
	"sigs.k8s.io/descheduler/test"
)

func TestFindDuplicatePods(t *testing.T) {
	ctx := context.Background()
	// first setup pods
	node := test.BuildTestNode("n1", 2000, 3000, 10, nil)
	p1 := test.BuildTestPod("p1", 100, 0, node.Name, nil)
	p1.Namespace = "dev"
	p2 := test.BuildTestPod("p2", 100, 0, node.Name, nil)
	p2.Namespace = "dev"
	p3 := test.BuildTestPod("p3", 100, 0, node.Name, nil)
	p3.Namespace = "dev"
	p4 := test.BuildTestPod("p4", 100, 0, node.Name, nil)
	p5 := test.BuildTestPod("p5", 100, 0, node.Name, nil)
	p6 := test.BuildTestPod("p6", 100, 0, node.Name, nil)
	p7 := test.BuildTestPod("p7", 100, 0, node.Name, nil)
	p7.Namespace = "kube-system"
	p8 := test.BuildTestPod("p8", 100, 0, node.Name, nil)
	p8.Namespace = "test"
	p9 := test.BuildTestPod("p9", 100, 0, node.Name, nil)
	p9.Namespace = "test"
	p10 := test.BuildTestPod("p10", 100, 0, node.Name, nil)
	p10.Namespace = "test"
	p11 := test.BuildTestPod("p11", 100, 0, node.Name, nil)
	p11.Namespace = "different-images"
	p12 := test.BuildTestPod("p12", 100, 0, node.Name, nil)
	p12.Namespace = "different-images"
	p13 := test.BuildTestPod("p13", 100, 0, node.Name, nil)
	p13.Namespace = "different-images"
	p14 := test.BuildTestPod("p14", 100, 0, node.Name, nil)
	p14.Namespace = "different-images"
	p15 := test.BuildTestPod("p15", 100, 0, node.Name, nil)
	p16 := test.BuildTestPod("p16", 100, 0, node.Name, nil)
	p17 := test.BuildTestPod("p17", 100, 0, node.Name, nil)
	p18 := test.BuildTestPod("p18", 100, 0, node.Name, nil)

	// ### Evictable Pods ###

	// Three Pods in the "default" Namespace, bound to same ReplicaSet. 2 should be evicted.
	ownerRef1 := test.GetReplicaSetOwnerRefList()
	p1.ObjectMeta.OwnerReferences = ownerRef1
	p2.ObjectMeta.OwnerReferences = ownerRef1
	p3.ObjectMeta.OwnerReferences = ownerRef1

	// Three Pods in the "test" Namespace, bound to same ReplicaSet. 2 should be evicted.
	ownerRef2 := test.GetReplicaSetOwnerRefList()
	p8.ObjectMeta.OwnerReferences = ownerRef2
	p9.ObjectMeta.OwnerReferences = ownerRef2
	p10.ObjectMeta.OwnerReferences = ownerRef2

	// ### Non-evictable Pods ###

	// A DaemonSet.
	p4.ObjectMeta.OwnerReferences = test.GetDaemonSetOwnerRefList()

	// A Pod with local storage.
	p5.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
	p5.Spec.Volumes = []v1.Volume{
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
	p6.Annotations = test.GetMirrorPodAnnotation()

	// A Critical Pod.
	priority := utils.SystemCriticalPriority
	p7.Spec.Priority = &priority

	// Same owners, but different images
	p11.Spec.Containers[0].Image = "foo"
	p11.ObjectMeta.OwnerReferences = ownerRef1
	p12.Spec.Containers[0].Image = "bar"
	p12.ObjectMeta.OwnerReferences = ownerRef1

	// Multiple containers
	p13.ObjectMeta.OwnerReferences = ownerRef1
	p13.Spec.Containers = append(p13.Spec.Containers, v1.Container{
		Name:  "foo",
		Image: "foo",
	})

	// Two Pods in the "default" Namespace in same node, bound to ReplicaSet which has replication factor 2. Should not be evicted.
	rsOwnerRef := test.GetReplicaSetOwnerRefList()
	p15.ObjectMeta.OwnerReferences = rsOwnerRef
	p16.ObjectMeta.OwnerReferences = rsOwnerRef

	// Two Pods in the "default" Namespace in same node, bound to ReplicationController which has replication factor 2. Should not be evicted.
	rcOwnerRef := test.GetReplicationControllerOwnerRefList()
	p17.ObjectMeta.OwnerReferences = rcOwnerRef
	p18.ObjectMeta.OwnerReferences = rcOwnerRef

	testCases := []struct {
		description             string
		maxPodsToEvict          int
		pods                    []v1.Pod
		expectedEvictedPodCount int
		strategy                api.DeschedulerStrategy
	}{
		{
			description:             "Three pods in the `dev` Namespace, bound to same ReplicaSet. 2 should be evicted.",
			maxPodsToEvict:          5,
			pods:                    []v1.Pod{*p1, *p2, *p3},
			expectedEvictedPodCount: 2,
			strategy:                api.DeschedulerStrategy{},
		},
		{
			description:             "Three pods in the `dev` Namespace, bound to same ReplicaSet, but ReplicaSet kind is excluded. 0 should be evicted.",
			maxPodsToEvict:          5,
			pods:                    []v1.Pod{*p1, *p2, *p3},
			expectedEvictedPodCount: 0,
			strategy:                api.DeschedulerStrategy{Params: &api.StrategyParameters{RemoveDuplicates: &api.RemoveDuplicates{ExcludeOwnerKinds: []string{"ReplicaSet"}}}},
		},
		{
			description:             "Three Pods in the `test` Namespace, bound to same ReplicaSet. 2 should be evicted.",
			maxPodsToEvict:          5,
			pods:                    []v1.Pod{*p8, *p9, *p10},
			expectedEvictedPodCount: 2,
			strategy:                api.DeschedulerStrategy{},
		},
		{
			description:             "Three Pods in the `dev` Namespace, three Pods in the `test` Namespace. Bound to ReplicaSet with same name. 4 should be evicted.",
			maxPodsToEvict:          5,
			pods:                    []v1.Pod{*p1, *p2, *p3, *p8, *p9, *p10},
			expectedEvictedPodCount: 4,
			strategy:                api.DeschedulerStrategy{},
		},
		{
			description:             "Pods are: part of DaemonSet, with local storage, mirror pod annotation, critical pod annotation - none should be evicted.",
			maxPodsToEvict:          2,
			pods:                    []v1.Pod{*p4, *p5, *p6, *p7},
			expectedEvictedPodCount: 0,
			strategy:                api.DeschedulerStrategy{},
		},
		{
			description:             "Test all Pods: 4 should be evicted.",
			maxPodsToEvict:          5,
			pods:                    []v1.Pod{*p1, *p2, *p3, *p4, *p5, *p6, *p7, *p8, *p9, *p10},
			expectedEvictedPodCount: 4,
			strategy:                api.DeschedulerStrategy{},
		},
		{
			description:             "Pods with the same owner but different images should not be evicted",
			maxPodsToEvict:          5,
			pods:                    []v1.Pod{*p11, *p12},
			expectedEvictedPodCount: 0,
			strategy:                api.DeschedulerStrategy{},
		},
		{
			description:             "Pods with multiple containers should not match themselves",
			maxPodsToEvict:          5,
			pods:                    []v1.Pod{*p13},
			expectedEvictedPodCount: 0,
			strategy:                api.DeschedulerStrategy{},
		},
		{
			description:             "Pods with matching ownerrefs and at not all matching image should not trigger an eviction",
			maxPodsToEvict:          5,
			pods:                    []v1.Pod{*p11, *p13},
			expectedEvictedPodCount: 0,
			strategy:                api.DeschedulerStrategy{},
		},
		{
			description:             "Pods managed by Replicaset having replicas greater than available nodes should not trigger an eviction",
			maxPodsToEvict:          5,
			pods:                    []v1.Pod{*p15, *p16},
			expectedEvictedPodCount: 0,
			strategy:                api.DeschedulerStrategy{},
		},
		{
			description:             "Pods managed by ReplicationController having replicas greater than available nodes should not trigger an eviction",
			maxPodsToEvict:          5,
			pods:                    []v1.Pod{*p17, *p18},
			expectedEvictedPodCount: 0,
			strategy:                api.DeschedulerStrategy{},
		},
	}

	for _, testCase := range testCases {
		fakeClient := &fake.Clientset{}
		fakeClient.Fake.AddReactor("list", "pods", func(action core.Action) (bool, runtime.Object, error) {
			return true, &v1.PodList{Items: testCase.pods}, nil
		})
		fakeClient.Fake.AddReactor("get", "nodes", func(action core.Action) (bool, runtime.Object, error) {
			return true, node, nil
		})
		// //TODO how to pass replicaas as int in reactor action
		// fakeClient.Fake.AddReactor("get", "replicationcontrollers", func(action core.Action) (bool, runtime.Object, error) {
		// 	return true, node, nil
		// })
		// fakeClient.Fake.AddReactor("get", "replicaset", func(action core.Action) (bool, runtime.Object, error) {
		// 	return true, nil, nil
		// })
		podEvictor := evictions.NewPodEvictor(
			fakeClient,
			"v1",
			false,
			testCase.maxPodsToEvict,
			[]*v1.Node{node},
			false,
		)

		RemoveDuplicatePods(ctx, fakeClient, testCase.strategy, []*v1.Node{node}, podEvictor)
		podsEvicted := podEvictor.TotalEvicted()
		if podsEvicted != testCase.expectedEvictedPodCount {
			t.Errorf("Test error for description: %s. Expected evicted pods count %v, got %v", testCase.description, testCase.expectedEvictedPodCount, podsEvicted)
		}
	}

}
