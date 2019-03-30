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

package test

import (
	"fmt"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BuildTestPod creates a test pod with given parameters.
func BuildTestPod(name string, cpu int64, memory int64, nodeName string) *v1.Pod {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "default",
			Name:        name,
			SelfLink:    fmt.Sprintf("/api/v1/namespaces/default/pods/%s", name),
			Annotations: map[string]string{},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{},
						Limits:   v1.ResourceList{},
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

// GetMirrorPodAnnotation returns the annotation needed for mirror pod.
func GetMirrorPodAnnotation() map[string]string {
	return map[string]string{
		"kubernetes.io/created-by":    "{\"kind\":\"SerializedReference\",\"apiVersion\":\"v1\",\"reference\":{\"kind\":\"Pod\"}}",
		"kubernetes.io/config.source": "api",
		"kubernetes.io/config.mirror": "mirror",
	}
}

// GetNormalPodOwnerRefList returns the ownerRef needed for a pod.
func GetNormalPodOwnerRefList() []metav1.OwnerReference {
	ownerRefList := make([]metav1.OwnerReference, 0)
	ownerRefList = append(ownerRefList, metav1.OwnerReference{Kind: "Pod", APIVersion: "v1"})
	return ownerRefList
}

// GetReplicaSetOwnerRefList returns the ownerRef needed for replicaset pod.
func GetReplicaSetOwnerRefList() []metav1.OwnerReference {
	ownerRefList := make([]metav1.OwnerReference, 0)
	ownerRefList = append(ownerRefList, metav1.OwnerReference{Kind: "ReplicaSet", APIVersion: "v1"})
	return ownerRefList
}

// GetDaemonSetOwnerRefList returns the ownerRef needed for daemonset pod.
func GetDaemonSetOwnerRefList() []metav1.OwnerReference {
	ownerRefList := make([]metav1.OwnerReference, 0)
	ownerRefList = append(ownerRefList, metav1.OwnerReference{Kind: "DaemonSet", APIVersion: "v1"})
	return ownerRefList
}

// GetCriticalPodAnnotation returns the annotation needed for critical pod.
func GetCriticalPodAnnotation() map[string]string {
	return map[string]string{
		"kubernetes.io/created-by":                   "{\"kind\":\"SerializedReference\",\"apiVersion\":\"v1\",\"reference\":{\"kind\":\"Pod\"}}",
		"scheduler.alpha.kubernetes.io/critical-pod": "",
	}
}

// BuildTestNode creates a node with specified capacity.
func BuildTestNode(name string, millicpu int64, mem int64, pods int64) *v1.Node {
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
