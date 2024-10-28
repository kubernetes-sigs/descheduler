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
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	utilptr "k8s.io/utils/ptr"
)

// BuildTestNamespace creates a test namespace with given parameters.
func BuildTestNamespace(name string) *v1.Namespace {
	namespace := &v1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			UID:    uuid.NewUUID(),
			Labels: map[string]string{"kubernetes.io/metadata.name": name},
		},
	}
	return namespace
}

// BuildTestPod creates a test pod with given parameters.
func BuildTestPod(name string, cpu, memory int64, nodeName string, apply func(*v1.Pod)) *v1.Pod {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      name,
			SelfLink:  fmt.Sprintf("/api/v1/namespaces/default/pods/%s", name),
			UID:       uuid.NewUUID(),
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

func BuildTestPDB(name, appLabel string) *policyv1.PodDisruptionBudget {
	maxUnavailable := intstr.FromInt32(1)
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      name,
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": appLabel,
				},
			},
			MaxUnavailable: &maxUnavailable,
		},
	}
	return pdb
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
func BuildTestNode(name string, millicpu, mem, pods int64, apply func(*v1.Node)) *v1.Node {
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

// SetNodeExtendedResource sets the given node's extended resources
func SetNodeExtendedResource(node *v1.Node, resourceName v1.ResourceName, requestQuantity int64) {
	node.Status.Capacity[resourceName] = *resource.NewQuantity(requestQuantity, resource.DecimalSI)
	node.Status.Allocatable[resourceName] = *resource.NewQuantity(requestQuantity, resource.DecimalSI)
}

func DeleteDeployment(ctx context.Context, t *testing.T, clientSet clientset.Interface, deployment *appsv1.Deployment) {
	// set number of replicas to 0
	deploymentCopy := deployment.DeepCopy()
	deploymentCopy.Spec.Replicas = utilptr.To[int32](0)
	if _, err := clientSet.AppsV1().Deployments(deploymentCopy.Namespace).Update(ctx, deploymentCopy, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("Error updating replica controller %v", err)
	}

	// wait 30 seconds until all pods are deleted
	if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 30*time.Second, true, func(c context.Context) (bool, error) {
		scale, err := clientSet.AppsV1().Deployments(deployment.Namespace).GetScale(c, deployment.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		return scale.Spec.Replicas == 0, nil
	}); err != nil {
		t.Fatalf("Error deleting Deployment pods %v", err)
	}

	if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 30*time.Second, true, func(c context.Context) (bool, error) {
		podList, _ := clientSet.CoreV1().Pods(deployment.Namespace).List(ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(deployment.Spec.Template.Labels).String()})
		t.Logf("Waiting for %v Deployment pods to disappear, still %v remaining", deployment.Name, len(podList.Items))
		if len(podList.Items) > 0 {
			return false, nil
		}
		return true, nil
	}); err != nil {
		t.Fatalf("Error waiting for Deployment pods to disappear: %v", err)
	}

	if err := clientSet.AppsV1().Deployments(deployment.Namespace).Delete(ctx, deployment.Name, metav1.DeleteOptions{}); err != nil {
		t.Fatalf("Error deleting Deployment %v", err)
	}

	if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 30*time.Second, true, func(c context.Context) (bool, error) {
		_, err := clientSet.AppsV1().Deployments(deployment.Namespace).Get(ctx, deployment.Name, metav1.GetOptions{})
		if err != nil && strings.Contains(err.Error(), "not found") {
			return true, nil
		}
		return false, nil
	}); err != nil {
		t.Fatalf("Error deleting Deployment %v", err)
	}
}

func SetPodAntiAffinity(inputPod *v1.Pod, labelKey, labelValue string) {
	inputPod.Spec.Affinity = &v1.Affinity{
		PodAntiAffinity: &v1.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
				{
					LabelSelector: &metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      labelKey,
								Operator: metav1.LabelSelectorOpIn,
								Values:   []string{labelValue},
							},
						},
					},
					TopologyKey: "region",
				},
			},
		},
	}
}

func PodWithPodAntiAffinity(inputPod *v1.Pod, labelKey, labelValue string) *v1.Pod {
	SetPodAntiAffinity(inputPod, labelKey, labelValue)
	inputPod.Labels = map[string]string{labelKey: labelValue}
	return inputPod
}
