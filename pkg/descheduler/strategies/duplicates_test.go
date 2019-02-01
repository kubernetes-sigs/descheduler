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
	"fmt"
	"strings"
	"testing"

	"github.com/kubernetes-incubator/descheduler/test"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
)

//TODO:@ravisantoshgudimetla This could be made table driven.
func TestFindDuplicatePods(t *testing.T) {
	n1 := test.BuildTestNode("n1", 2000, 3000, 10)
	n2 := test.BuildTestNode("n2", 2000, 3000, 10)

	p1 := test.BuildTestPod("p1", 100, 0, n1.Name)
	p2 := test.BuildTestPod("p2", 100, 0, n1.Name)
	p3 := test.BuildTestPod("p3", 100, 0, n1.Name)
	p4 := test.BuildTestPod("p4", 100, 0, n1.Name)
	p5 := test.BuildTestPod("p5", 100, 0, n1.Name)
	p6 := test.BuildTestPod("p6", 100, 0, n1.Name)
	p7 := test.BuildTestPod("p7", 100, 0, n1.Name)
	p8 := test.BuildTestPod("p8", 100, 0, n1.Name)
	p9 := test.BuildTestPod("p9", 100, 0, n1.Name)

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
		list := action.(core.ListAction)
		fieldString := list.GetListRestrictions().Fields.String()
		if strings.Contains(fieldString, "n1") {
			return true, &v1.PodList{Items: []v1.Pod{*p1, *p2, *p3, *p4, *p5, *p6, *p7, *p8, *p9}}, nil
		}
		if strings.Contains(fieldString, "n2") {
			return true, &v1.PodList{Items: []v1.Pod{}}, nil
		}
		return true, nil, fmt.Errorf("Failed to list: %v", list)
	})
	fakeClient.Fake.AddReactor("get", "nodes", func(action core.Action) (bool, runtime.Object, error) {
		return true, &v1.NodeList{Items: []v1.Node{*n1, *n2}}, nil
	})
	npe := nodePodEvictedCount{}
	npe[n1] = 0
	npe[n2] = 0
	podsEvicted := deleteDuplicatePods(fakeClient, "v1", []*v1.Node{n1, n2}, false, npe, 2)
	if podsEvicted != expectedEvictedPodCount {
		t.Errorf("Unexpected no of pods evicted")
	}

}

func TestFindDuplicatePodsWithMaster(t *testing.T) {
	n0 := test.BuildTestNode("n0", 2000, 3000, 10)
	n1 := test.BuildTestNode("n1", 2000, 3000, 10)
	n2 := test.BuildTestNode("n2", 2000, 3000, 10)

	masterTaint := v1.Taint{
		Key:    "node-role.kubernetes.io/master",
		Effect: "NoSchedule",
	}
	n0.Spec.Taints = append(n0.Spec.Taints, masterTaint)

	p1 := test.BuildTestPod("p1", 100, 0, n1.Name)
	p2 := test.BuildTestPod("p2", 100, 0, n1.Name)
	p3 := test.BuildTestPod("p3", 100, 0, n2.Name)
	p4 := test.BuildTestPod("p4", 100, 0, n2.Name)
	p5 := test.BuildTestPod("p5", 100, 0, n2.Name)

	p1.ObjectMeta.OwnerReferences = test.GetReplicaSetOwnerRefList()
	p2.ObjectMeta.OwnerReferences = test.GetReplicaSetOwnerRefList()
	p3.ObjectMeta.OwnerReferences = test.GetReplicaSetOwnerRefList()
	p4.ObjectMeta.OwnerReferences = test.GetReplicaSetOwnerRefList()
	p5.ObjectMeta.OwnerReferences = test.GetReplicaSetOwnerRefList()

	// As 'RS1' pods are present on all possible nodes, no evictions will happen for them
	p1.ObjectMeta.OwnerReferences[0].Name = "RS1"
	p2.ObjectMeta.OwnerReferences[0].Name = "RS1"
	p3.ObjectMeta.OwnerReferences[0].Name = "RS1"

	// As 'RS2' pods are present on only node 'n2', one pod should be evicted
	p4.ObjectMeta.OwnerReferences[0].Name = "RS2"
	p5.ObjectMeta.OwnerReferences[0].Name = "RS2"

	fakeClient := &fake.Clientset{}
	fakeClient.Fake.AddReactor("list", "pods", func(action core.Action) (bool, runtime.Object, error) {
		list := action.(core.ListAction)
		fieldString := list.GetListRestrictions().Fields.String()
		if strings.Contains(fieldString, "n1") {
			return true, &v1.PodList{Items: []v1.Pod{*p1, *p2}}, nil
		}
		if strings.Contains(fieldString, "n2") {
			return true, &v1.PodList{Items: []v1.Pod{*p3, *p4, *p5}}, nil
		}
		return true, nil, fmt.Errorf("Failed to list: %v", list)
	})
	fakeClient.Fake.AddReactor("get", "nodes", func(action core.Action) (bool, runtime.Object, error) {
		return true, &v1.NodeList{Items: []v1.Node{*n0, *n1, *n2}}, nil
	})

	npe := nodePodEvictedCount{}
	npe[n0] = 0
	npe[n1] = 0
	npe[n2] = 0

	expectedEvictedPodCount := 1
	podsEvicted := deleteDuplicatePods(fakeClient, "v1", []*v1.Node{n0, n1, n2}, false, npe, 2)
	if podsEvicted != expectedEvictedPodCount {
		t.Errorf("Unexpected no of pods evicted")
	}
}
