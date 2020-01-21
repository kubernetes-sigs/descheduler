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
	"sigs.k8s.io/descheduler/test"
)

//TODO:@ravisantoshgudimetla This could be made table driven.
func TestFindDuplicatePods(t *testing.T) {
	node := test.BuildTestNode("n1", 2000, 3000, 10)
	p1 := test.BuildTestPod("p1", 100, 0, node.Name)
	p2 := test.BuildTestPod("p2", 100, 0, node.Name)
	p3 := test.BuildTestPod("p3", 100, 0, node.Name)
	p4 := test.BuildTestPod("p4", 100, 0, node.Name)
	p5 := test.BuildTestPod("p5", 100, 0, node.Name)
	p6 := test.BuildTestPod("p6", 100, 0, node.Name)
	p7 := test.BuildTestPod("p7", 100, 0, node.Name)
	p8 := test.BuildTestPod("p8", 100, 0, node.Name)
	p9 := test.BuildTestPod("p9", 100, 0, node.Name)

	// All the following pods expect for one will be evicted.
	p1.ObjectMeta.OwnerReferences = test.GetReplicaSetOwnerRefList()
	p2.ObjectMeta.OwnerReferences = test.GetReplicaSetOwnerRefList()
	p3.ObjectMeta.OwnerReferences = test.GetReplicaSetOwnerRefList()
	p8.ObjectMeta.OwnerReferences = test.GetReplicaSetOwnerRefList()
	p9.ObjectMeta.OwnerReferences = test.GetReplicaSetOwnerRefList()

	// The following 4 pods won't get evicted.
	// A daemonset.
	p4.ObjectMeta.OwnerReferences = test.GetDaemonSetOwnerRefList()
	// A pod with local storage.
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
	p7.Namespace = "kube-system"
	p7.Annotations = test.GetCriticalPodAnnotation()
	expectedEvictedPodCount := 2
	fakeClient := &fake.Clientset{}
	fakeClient.Fake.AddReactor("list", "pods", func(action core.Action) (bool, runtime.Object, error) {
		return true, &v1.PodList{Items: []v1.Pod{*p1, *p2, *p3, *p4, *p5, *p6, *p7, *p8, *p9}}, nil
	})
	fakeClient.Fake.AddReactor("get", "nodes", func(action core.Action) (bool, runtime.Object, error) {
		return true, node, nil
	})
	npe := nodePodEvictedCount{}
	npe[node] = 0
	podsEvicted := deleteDuplicatePods(fakeClient, "v1", []*v1.Node{node}, false, npe, 2)
	if podsEvicted != expectedEvictedPodCount {
		t.Errorf("Unexpected no of pods evicted")
	}

}
