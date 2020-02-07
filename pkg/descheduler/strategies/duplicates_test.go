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
	"testing"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"sigs.k8s.io/descheduler/pkg/utils"
	"sigs.k8s.io/descheduler/test"
)

func TestFindDuplicatePods(t *testing.T) {
	// first setup pods
	node := test.BuildTestNode("n1", 2000, 3000, 10)
	p1 := test.BuildTestPod("p1", 100, 0, node.Name)
	p1.Namespace = "dev"
	p2 := test.BuildTestPod("p2", 100, 0, node.Name)
	p2.Namespace = "dev"
	p3 := test.BuildTestPod("p3", 100, 0, node.Name)
	p3.Namespace = "dev"
	p4 := test.BuildTestPod("p4", 100, 0, node.Name)
	p5 := test.BuildTestPod("p5", 100, 0, node.Name)
	p6 := test.BuildTestPod("p6", 100, 0, node.Name)
	p7 := test.BuildTestPod("p7", 100, 0, node.Name)
	p7.Namespace = "kube-system"
	p8 := test.BuildTestPod("p8", 100, 0, node.Name)
	p8.Namespace = "test"
	p9 := test.BuildTestPod("p9", 100, 0, node.Name)
	p9.Namespace = "test"
	p10 := test.BuildTestPod("p10", 100, 0, node.Name)
	p10.Namespace = "test"

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

	testCases := []struct {
		description             string
		maxPodsToEvict          int
		pods                    []v1.Pod
		expectedEvictedPodCount int
	}{
		{
			description:             "Three pods in the `dev` Namespace, bound to same ReplicaSet. 2 should be evicted.",
			maxPodsToEvict:          5,
			pods:                    []v1.Pod{*p1, *p2, *p3},
			expectedEvictedPodCount: 2,
		},
		{
			description:             "Three Pods in the `test` Namespace, bound to same ReplicaSet. 2 should be evicted.",
			maxPodsToEvict:          5,
			pods:                    []v1.Pod{*p8, *p9, *p10},
			expectedEvictedPodCount: 2,
		},
		{
			description:             "Three Pods in the `dev` Namespace, three Pods in the `test` Namespace. Bound to ReplicaSet with same name. 4 should be evicted.",
			maxPodsToEvict:          5,
			pods:                    []v1.Pod{*p1, *p2, *p3, *p8, *p9, *p10},
			expectedEvictedPodCount: 4,
		},
		{
			description:             "Pods are: part of DaemonSet, with local storage, mirror pod annotation, critical pod annotation - none should be evicted.",
			maxPodsToEvict:          2,
			pods:                    []v1.Pod{*p4, *p5, *p6, *p7},
			expectedEvictedPodCount: 0,
		},
		{
			description:             "Test all Pods: 4 should be evicted.",
			maxPodsToEvict:          5,
			pods:                    []v1.Pod{*p1, *p2, *p3, *p4, *p5, *p6, *p7, *p8, *p9, *p10},
			expectedEvictedPodCount: 4,
		},
	}

	for _, testCase := range testCases {

		npe := utils.NodePodEvictedCount{}
		npe[node] = 0
		fakeClient := &fake.Clientset{}
		fakeClient.Fake.AddReactor("list", "pods", func(action core.Action) (bool, runtime.Object, error) {
			return true, &v1.PodList{Items: testCase.pods}, nil
		})
		fakeClient.Fake.AddReactor("get", "nodes", func(action core.Action) (bool, runtime.Object, error) {
			return true, node, nil
		})
		podsEvicted := deleteDuplicatePods(fakeClient, "v1", []*v1.Node{node}, false, npe, testCase.maxPodsToEvict, false)
		if podsEvicted != testCase.expectedEvictedPodCount {
			t.Errorf("Test error for description: %s. Expected evicted pods count %v, got %v", testCase.description, testCase.expectedEvictedPodCount, podsEvicted)
		}
	}

}
