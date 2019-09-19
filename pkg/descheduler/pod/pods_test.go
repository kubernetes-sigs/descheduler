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

package pod

import (
	"testing"

	"sigs.k8s.io/descheduler/test"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestPodTypes(t *testing.T) {
	n1 := test.BuildTestNode("node1", 1000, 2000, 9)
	p1 := test.BuildTestPod("p1", 400, 0, n1.Name)

	// These won't be evicted.
	p2 := test.BuildTestPod("p2", 400, 0, n1.Name)
	p3 := test.BuildTestPod("p3", 400, 0, n1.Name)
	p4 := test.BuildTestPod("p4", 400, 0, n1.Name)
	p5 := test.BuildTestPod("p5", 400, 0, n1.Name)
	p6 := test.BuildTestPod("p6", 400, 0, n1.Name)
	p6.Spec.Containers[0].Resources.Requests[v1.ResourceNvidiaGPU] = *resource.NewMilliQuantity(3, resource.DecimalSI)

	p6.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()

	p1.ObjectMeta.OwnerReferences = test.GetReplicaSetOwnerRefList()
	// The following 4 pods won't get evicted.
	// A daemonset.
	//p2.Annotations = test.GetDaemonSetAnnotation()
	p2.ObjectMeta.OwnerReferences = test.GetDaemonSetOwnerRefList()
	// A pod with local storage.
	p3.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
	p3.Spec.Volumes = []v1.Volume{
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
	p4.Annotations = test.GetMirrorPodAnnotation()
	// A Critical Pod.
	p5.Namespace = "kube-system"
	p5.Annotations = test.GetCriticalPodAnnotation()
	if !IsMirrorPod(p4) {
		t.Errorf("Expected p4 to be a mirror pod.")
	}
	if !IsCriticalPod(p5) {
		t.Errorf("Expected p5 to be a critical pod.")
	}
	if !IsPodWithLocalStorage(p3) {
		t.Errorf("Expected p3 to be a pod with local storage.")
	}
	ownerRefList := OwnerRef(p2)
	if !IsDaemonsetPod(ownerRefList) {
		t.Errorf("Expected p2 to be a daemonset pod.")
	}
	ownerRefList = OwnerRef(p1)
	if IsDaemonsetPod(ownerRefList) || IsPodWithLocalStorage(p1) || IsCriticalPod(p1) || IsMirrorPod(p1) {
		t.Errorf("Expected p1 to be a normal pod.")
	}
	if !IsLatencySensitivePod(p6) {
		t.Errorf("Expected p6 to be latency sensitive pod")
	}

}
