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

	"fmt"
	"github.com/kubernetes-incubator/descheduler/test"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	core "k8s.io/client-go/testing"
	"k8s.io/kubernetes/pkg/api/v1"
	"k8s.io/kubernetes/pkg/client/clientset_generated/clientset/fake"
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

	// All the following pods expect for one will be evicted.
	p1.Annotations = test.GetReplicaSetAnnotation()
	p2.Annotations = test.GetReplicaSetAnnotation()
	p3.Annotations = test.GetReplicaSetAnnotation()

	// The following 4 pods won't get evicted.
	// A daemonset.
	p4.Annotations = test.GetDaemonSetAnnotation()
	// A pod with local storage.
	p5.Annotations = test.GetNormalPodAnnotation()
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
		return true, &v1.PodList{Items: []v1.Pod{*p1, *p2, *p3, *p4, *p5, *p6, *p7}}, nil
	})
	fakeClient.Fake.AddReactor("get", "nodes", func(action core.Action) (bool, runtime.Object, error) {
		return true, node, nil
	})
	podsEvicted := deleteDuplicatePods(fakeClient, "v1", []*v1.Node{node}, false)
	if podsEvicted != expectedEvictedPodCount {
		t.Errorf("Unexpected no of pods evicted")
	}

}

// BenchmarkDuplicatePodsOnANode is used for benchmarking duplicate pod removal strategy.
func BenchmarkDuplicatePodsOnANode(b *testing.B) {
	podList := make([]v1.Pod, 0)
	nodeList := make([]v1.Node, 0)
	nl := make([]*v1.Node, 0)
	// Simulating a 250 node cluster with 100 pods on each node.
	for i := 0; i < 250; i++ {
		nodeGenerated := test.BuildTestNode("node%d", 2000, 3000, 100)
		nodeList = append(nodeList, *nodeGenerated)
		nl = append(nl, nodeGenerated)
		for i := 0; i < 100; i++ {
			podGenerated := test.BuildTestPod(fmt.Sprintf("pod%d", i), 0, 0, nodeGenerated.Name)
			podGenerated.Annotations = test.GetReplicaSetAnnotation()
			podList = append(podList, *podGenerated)
		}
	}
	fakeClient := &fake.Clientset{}
	fakeClient.Fake.AddReactor("list", "pods", func(action core.Action) (bool, runtime.Object, error) {
		return true, &v1.PodList{Items: podList}, nil
	})
	fakeClient.Fake.AddReactor("get", "nodes", func(action core.Action) (bool, runtime.Object, error) {
		return true, &v1.NodeList{Items: nodeList}, nil
	})
	podsEvicted := deleteDuplicatePods(fakeClient, "v1", nl, false)
	fmt.Println(podsEvicted)
}
