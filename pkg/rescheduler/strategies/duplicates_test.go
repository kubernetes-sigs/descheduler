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
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	core "k8s.io/client-go/testing"
	"k8s.io/kubernetes/pkg/api/v1"
	"k8s.io/kubernetes/pkg/client/clientset_generated/clientset/fake"
)

// TODO:@ravisantoshgudimetla. As of now building some test pods here. This needs to
// move to utils after refactor.
// buildTestPod creates a test pod with given parameters.
func buildTestPod(name string, cpu int64, memory int64, nodeName string) *v1.Pod {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      name,
			SelfLink:  fmt.Sprintf("/api/v1/namespaces/default/pods/%s", name),
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{},
					},
				},
			},
			NodeName: nodeName,
		},
	}
	if cpu >= 0 {
		pod.Spec.Containers[0].Resources.Requests[v1.ResourceCPU] = *resource.NewMilliQuantity(cpu, resource.DecimalSI)
	}
	if memory >= 0 {
		pod.Spec.Containers[0].Resources.Requests[v1.ResourceMemory] = *resource.NewQuantity(memory, resource.DecimalSI)
	}

	return pod
}

// buildTestNode creates a node with specified capacity.
func buildTestNode(name string, millicpu int64, mem int64, pods int64) *v1.Node {
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:     name,
			SelfLink: fmt.Sprintf("/api/v1/nodes/%s", name),
			Labels:   map[string]string{},
		},
		Status: v1.NodeStatus{
			Capacity: v1.ResourceList{
				v1.ResourcePods:   *resource.NewQuantity(pods, resource.DecimalSI),
				v1.ResourceCPU:    *resource.NewMilliQuantity(millicpu, resource.DecimalSI),
				v1.ResourceMemory: *resource.NewQuantity(mem, resource.DecimalSI),
			},
			Allocatable: v1.ResourceList{
				v1.ResourcePods:   *resource.NewQuantity(pods, resource.DecimalSI),
				v1.ResourceCPU:    *resource.NewMilliQuantity(millicpu, resource.DecimalSI),
				v1.ResourceMemory: *resource.NewQuantity(mem, resource.DecimalSI),
			},
			Phase: v1.NodeRunning,
			Conditions: []v1.NodeCondition{
				{Type: v1.NodeReady, Status: v1.ConditionTrue},
			},
		},
	}
	return node
}

// getMirrorPodAnnotation returns the annotation needed for mirror pod.
func getMirrorPodAnnotation() map[string]string {
	return map[string]string{
		"kubernetes.io/created-by":    "{\"kind\":\"SerializedReference\",\"apiVersion\":\"v1\",\"reference\":{\"kind\":\"Pod\"}}",
		"kubernetes.io/config.source": "api",
		"kubernetes.io/config.mirror": "mirror",
	}
}

// getNormalPodAnnotation returns the annotation needed for a pod.
func getNormalPodAnnotation() map[string]string {
	return map[string]string{
		"kubernetes.io/created-by": "{\"kind\":\"SerializedReference\",\"apiVersion\":\"v1\",\"reference\":{\"kind\":\"Pod\"}}",
	}
}

// getReplicaSetAnnotation returns the annotation needed for replicaset pod.
func getReplicaSetAnnotation() map[string]string {
	return map[string]string{
		"kubernetes.io/created-by": "{\"kind\":\"SerializedReference\",\"apiVersion\":\"v1\",\"reference\":{\"kind\":\"ReplicaSet\"}}",
	}
}

// getDaemonSetAnnotation returns the annotation needed for daemonset pod.
func getDaemonSetAnnotation() map[string]string {
	return map[string]string{
		"kubernetes.io/created-by": "{\"kind\":\"SerializedReference\",\"apiVersion\":\"v1\",\"reference\":{\"kind\":\"DaemonSet\"}}",
	}
}

// getCriticalPodAnnotation returns the annotation needed for critical pod.
func getCriticalPodAnnotation() map[string]string {
	return map[string]string{
		"kubernetes.io/created-by":                   "{\"kind\":\"SerializedReference\",\"apiVersion\":\"v1\",\"reference\":{\"kind\":\"Pod\"}}",
		"scheduler.alpha.kubernetes.io/critical-pod": "",
	}
}

//TODO:@ravisantoshgudimetla This could be made table driven.
func TestFindDuplicatePods(t *testing.T) {
	node := buildTestNode("n1", 2000, 3000, 10)
	p1 := buildTestPod("p1", 100, 0, node.Name)
	p2 := buildTestPod("p2", 100, 0, node.Name)
	p3 := buildTestPod("p3", 100, 0, node.Name)
	p4 := buildTestPod("p4", 100, 0, node.Name)
	p5 := buildTestPod("p5", 100, 0, node.Name)
	p6 := buildTestPod("p6", 100, 0, node.Name)
	p7 := buildTestPod("p7", 100, 0, node.Name)

	// All the following pods expect for one will be evicted.
	p1.Annotations = getReplicaSetAnnotation()
	p2.Annotations = getReplicaSetAnnotation()
	p3.Annotations = getReplicaSetAnnotation()

	// The following 4 pods won't get evicted.
	// A daemonset.
	p4.Annotations = getDaemonSetAnnotation()
	// A pod with local storage.
	p5.Annotations = getNormalPodAnnotation()
	p5.Spec.Volumes = []v1.Volume{
		{
			Name: "sample",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{Path: "somePath"},
				EmptyDir: &v1.EmptyDirVolumeSource{
					SizeLimit: *resource.NewQuantity(int64(10), resource.BinarySI)},
			},
		},
	}
	// A Mirror Pod.
	p6.Annotations = getMirrorPodAnnotation()
	// A Critical Pod.
	p7.Namespace = "kube-system"
	p7.Annotations = getCriticalPodAnnotation()
	expectedEvictedPodCount := 2
	fakeClient := &fake.Clientset{}
	fakeClient.Fake.AddReactor("list", "pods", func(action core.Action) (bool, runtime.Object, error) {
		return true, &v1.PodList{Items: []v1.Pod{*p1, *p2, *p3, *p4, *p5, *p6, *p7}}, nil
	})
	fakeClient.Fake.AddReactor("get", "nodes", func(action core.Action) (bool, runtime.Object, error) {
		return true, node, nil
	})
	podsEvicted := deleteDuplicatePods(fakeClient, "v1", []*v1.Node{node})
	if podsEvicted != expectedEvictedPodCount {
		t.Errorf("Unexpected no of pods evicted")
	}

}
