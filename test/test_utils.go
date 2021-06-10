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

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BuildTestPod creates a test pod with given parameters.
func BuildTestPod(name string, cpu int64, memory int64, nodeName string, apply func(*v1.Pod)) *v1.Pod {
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
	if apply != nil {
		apply(pod)
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
	ownerRefList = append(ownerRefList, metav1.OwnerReference{Kind: "ReplicaSet", APIVersion: "v1", Name: "replicaset-1"})
	return ownerRefList
}

// GetStatefulSetOwnerRefList returns the ownerRef needed for statefulset pod.
func GetStatefulSetOwnerRefList() []metav1.OwnerReference {
	ownerRefList := make([]metav1.OwnerReference, 0)
	ownerRefList = append(ownerRefList, metav1.OwnerReference{Kind: "StatefulSet", APIVersion: "v1", Name: "statefulset-1"})
	return ownerRefList
}

// GetDaemonSetOwnerRefList returns the ownerRef needed for daemonset pod.
func GetDaemonSetOwnerRefList() []metav1.OwnerReference {
	ownerRefList := make([]metav1.OwnerReference, 0)
	ownerRefList = append(ownerRefList, metav1.OwnerReference{Kind: "DaemonSet", APIVersion: "v1"})
	return ownerRefList
}

// BuildTestNode creates a node with specified capacity.
func BuildTestNode(name string, millicpu int64, mem int64, pods int64, apply func(*v1.Node)) *v1.Node {
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
	if apply != nil {
		apply(node)
	}
	return node
}

// MakeBestEffortPod makes the given pod a BestEffort pod
func MakeBestEffortPod(pod *v1.Pod) {
	pod.Spec.Containers[0].Resources.Requests = nil
	pod.Spec.Containers[0].Resources.Requests = nil
	pod.Spec.Containers[0].Resources.Limits = nil
	pod.Spec.Containers[0].Resources.Limits = nil
}

// MakeBurstablePod makes the given pod a Burstable pod
func MakeBurstablePod(pod *v1.Pod) {
	pod.Spec.Containers[0].Resources.Limits = nil
	pod.Spec.Containers[0].Resources.Limits = nil
}

// MakeGuaranteedPod makes the given pod an Guaranteed pod
func MakeGuaranteedPod(pod *v1.Pod) {
	pod.Spec.Containers[0].Resources.Limits[v1.ResourceCPU] = pod.Spec.Containers[0].Resources.Requests[v1.ResourceCPU]
	pod.Spec.Containers[0].Resources.Limits[v1.ResourceMemory] = pod.Spec.Containers[0].Resources.Requests[v1.ResourceMemory]
}

// SetRSOwnerRef sets the given pod's owner to ReplicaSet
func SetRSOwnerRef(pod *v1.Pod) {
	pod.ObjectMeta.OwnerReferences = GetReplicaSetOwnerRefList()
}

// SetSSOwnerRef sets the given pod's owner to StatefulSet
func SetSSOwnerRef(pod *v1.Pod) {
	pod.ObjectMeta.OwnerReferences = GetStatefulSetOwnerRefList()
}

// SetDSOwnerRef sets the given pod's owner to DaemonSet
func SetDSOwnerRef(pod *v1.Pod) {
	pod.ObjectMeta.OwnerReferences = GetDaemonSetOwnerRefList()
}

// SetNormalOwnerRef sets the given pod's owner to Pod
func SetNormalOwnerRef(pod *v1.Pod) {
	pod.ObjectMeta.OwnerReferences = GetNormalPodOwnerRefList()
}

// SetPodPriority sets the given pod's priority
func SetPodPriority(pod *v1.Pod, priority int32) {
	pod.Spec.Priority = &priority
}

// SetNodeUnschedulable sets the given node unschedulable
func SetNodeUnschedulable(node *v1.Node) {
	node.Spec.Unschedulable = true
}

// SetPodExtendedResourceRequest sets the given pod's extended resources
func SetPodExtendedResourceRequest(pod *v1.Pod, resourceName v1.ResourceName, requestQuantity int64) {
	pod.Spec.Containers[0].Resources.Requests[resourceName] = *resource.NewQuantity(requestQuantity, resource.DecimalSI)
}

// SetNodeExtendedResouces sets the given node's extended resources
func SetNodeExtendedResource(node *v1.Node, resourceName v1.ResourceName, requestQuantity int64) {
	node.Status.Capacity[resourceName] = *resource.NewQuantity(requestQuantity, resource.DecimalSI)
	node.Status.Allocatable[resourceName] = *resource.NewQuantity(requestQuantity, resource.DecimalSI)
}
