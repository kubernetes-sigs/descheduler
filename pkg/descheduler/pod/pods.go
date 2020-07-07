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
	"context"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"sigs.k8s.io/descheduler/pkg/utils"
	"sort"
)

// ListPodsOnANode lists all of the pods on a node
// It also accepts an optional "filter" function which can be used to further limit the pods that are returned.
// (Usually this is podEvictor.IsEvictable, in order to only list the evictable pods on a node, but can
// be used by strategies to extend IsEvictable if there are further restrictions, such as with NodeAffinity).
// The filter function should return true if the pod should be returned from ListPodsOnANode
func ListPodsOnANode(ctx context.Context, client clientset.Interface, node *v1.Node, filter func(pod *v1.Pod) bool) ([]*v1.Pod, error) {
	fieldSelector, err := fields.ParseSelector("spec.nodeName=" + node.Name + ",status.phase!=" + string(v1.PodSucceeded) + ",status.phase!=" + string(v1.PodFailed))
	if err != nil {
		return []*v1.Pod{}, err
	}

	podList, err := client.CoreV1().Pods(v1.NamespaceAll).List(ctx,
		metav1.ListOptions{FieldSelector: fieldSelector.String()})
	if err != nil {
		return []*v1.Pod{}, err
	}

	pods := make([]*v1.Pod, 0)
	for i := range podList.Items {
		if filter != nil && !filter(&podList.Items[i]) {
			continue
		}
		pods = append(pods, &podList.Items[i])
	}
	return pods, nil
}

// GetPodOwnerReplicationCount returns the pod owner's replica count
func GetPodOwnerReplicationCount(ctx context.Context, client clientset.Interface, ownerRef metav1.OwnerReference) (int, error) {
	switch ownerRef.Kind {
	case "ReplicaSet":
		owner, err := client.AppsV1().ReplicaSets(v1.NamespaceAll).Get(ctx, ownerRef.Name, metav1.GetOptions{})
		if err != nil {
			return 0, err
		}
		return int(owner.Status.Replicas), nil
	case "ReplicationController":
		owner, err := client.CoreV1().ReplicationControllers(v1.NamespaceAll).Get(ctx, ownerRef.Name, metav1.GetOptions{})
		if err != nil {
			return 0, err
		}
		return int(owner.Status.Replicas), nil
	default:
		klog.Infof("pod is non managed by RS or RC - owner name %s kind %s", ownerRef.Name, ownerRef.Kind)
		// Returning default value as 1 as its a single pod non managed by a rc or rs
		return 1, nil
	}
}

// OwnerRef returns the ownerRefList for the pod.
func OwnerRef(pod *v1.Pod) []metav1.OwnerReference {
	return pod.ObjectMeta.GetOwnerReferences()
}

func IsBestEffortPod(pod *v1.Pod) bool {
	return utils.GetPodQOS(pod) == v1.PodQOSBestEffort
}

func IsBurstablePod(pod *v1.Pod) bool {
	return utils.GetPodQOS(pod) == v1.PodQOSBurstable
}

func IsGuaranteedPod(pod *v1.Pod) bool {
	return utils.GetPodQOS(pod) == v1.PodQOSGuaranteed
}

// SortPodsBasedOnPriorityLowToHigh sorts pods based on their priorities from low to high.
// If pods have same priorities, they will be sorted by QoS in the following order:
// BestEffort, Burstable, Guaranteed
func SortPodsBasedOnPriorityLowToHigh(pods []*v1.Pod) {
	sort.Slice(pods, func(i, j int) bool {
		if pods[i].Spec.Priority == nil && pods[j].Spec.Priority != nil {
			return true
		}
		if pods[j].Spec.Priority == nil && pods[i].Spec.Priority != nil {
			return false
		}
		if (pods[j].Spec.Priority == nil && pods[i].Spec.Priority == nil) || (*pods[i].Spec.Priority == *pods[j].Spec.Priority) {
			if IsBestEffortPod(pods[i]) {
				return true
			}
			if IsBurstablePod(pods[i]) && IsGuaranteedPod(pods[j]) {
				return true
			}
			return false
		}
		return *pods[i].Spec.Priority < *pods[j].Spec.Priority
	})
}
