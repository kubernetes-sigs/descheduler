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
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	clientset "k8s.io/client-go/kubernetes"
	api "k8s.io/kubernetes/pkg/apis/core"
	"k8s.io/kubernetes/pkg/apis/core/v1/helper/qos"
	"k8s.io/kubernetes/pkg/kubelet/types"
)

// checkLatencySensitiveResourcesForAContainer checks if there are any latency sensitive resources like GPUs.
func checkLatencySensitiveResourcesForAContainer(rl v1.ResourceList) bool {
	if rl == nil {
		return false
	}
	for rName := range rl {
		if rName == v1.ResourceNvidiaGPU {
			return true
		}
		// TODO: Add support for other high value resources like hugepages etc. once kube is rebased to 1.8.
	}
	return false
}

// IsLatencySensitivePod checks if a pod consumes high value devices like GPUs, hugepages or when cpu pinning enabled.
func IsLatencySensitivePod(pod *v1.Pod) bool {
	for _, container := range pod.Spec.Containers {
		resourceList := container.Resources.Requests
		if checkLatencySensitiveResourcesForAContainer(resourceList) {
			return true
		}
	}
	return false
}

// IsEvictable checks if a pod is evictable or not.
func IsEvictable(annotationsPrefix string, pod *v1.Pod) bool {
	ownerRefList := OwnerRef(pod)
	if IsMirrorPod(pod) || IsPodWithLocalStorage(pod) || len(ownerRefList) == 0 || IsDaemonsetPod(ownerRefList) || IsCriticalPod(annotationsPrefix, pod) {
		return false
	}
	return true
}

// ListEvictablePodsOnNode returns the list of evictable pods on node.
func ListEvictablePodsOnNode(client clientset.Interface, annotationsPrefix string, node *v1.Node) ([]*v1.Pod, error) {
	pods, err := ListPodsOnANode(client, node)
	if err != nil {
		return []*v1.Pod{}, err
	}
	evictablePods := make([]*v1.Pod, 0)
	for _, pod := range pods {
		if !IsEvictable(annotationsPrefix, pod) {
			continue
		} else {
			evictablePods = append(evictablePods, pod)
		}
	}
	return evictablePods, nil
}

func ListPodsOnANode(client clientset.Interface, node *v1.Node) ([]*v1.Pod, error) {
	fieldSelector, err := fields.ParseSelector("spec.nodeName=" + node.Name + ",status.phase!=" + string(api.PodSucceeded) + ",status.phase!=" + string(api.PodFailed))
	if err != nil {
		return []*v1.Pod{}, err
	}

	podList, err := client.CoreV1().Pods(v1.NamespaceAll).List(
		metav1.ListOptions{FieldSelector: fieldSelector.String()})
	if err != nil {
		return []*v1.Pod{}, err
	}

	pods := make([]*v1.Pod, 0)
	for i := range podList.Items {
		pods = append(pods, &podList.Items[i])
	}
	return pods, nil
}

func IsCriticalPod(annotationsPrefix string, pod *v1.Pod) bool {
	if types.IsCriticalPod(pod) {
		return true
	}

	val, ok := pod.ObjectMeta.Annotations[annotationsPrefix+"/critical-pod"]
	return ok && val == ""
}

func IsBestEffortPod(pod *v1.Pod) bool {
	return qos.GetPodQOS(pod) == v1.PodQOSBestEffort
}

func IsBurstablePod(pod *v1.Pod) bool {
	return qos.GetPodQOS(pod) == v1.PodQOSBurstable
}

func IsGuaranteedPod(pod *v1.Pod) bool {
	return qos.GetPodQOS(pod) == v1.PodQOSGuaranteed
}

func IsDaemonsetPod(ownerRefList []metav1.OwnerReference) bool {
	for _, ownerRef := range ownerRefList {
		if ownerRef.Kind == "DaemonSet" {
			return true
		}
	}
	return false
}

// IsMirrorPod checks whether the pod is a mirror pod.
func IsMirrorPod(pod *v1.Pod) bool {
	_, found := pod.ObjectMeta.Annotations[types.ConfigMirrorAnnotationKey]
	return found
}

func IsPodWithLocalStorage(pod *v1.Pod) bool {
	for _, volume := range pod.Spec.Volumes {
		if volume.HostPath != nil || volume.EmptyDir != nil {
			return true
		}
	}

	return false
}

// OwnerRef returns the ownerRefList for the pod.
func OwnerRef(pod *v1.Pod) []metav1.OwnerReference {
	return pod.ObjectMeta.GetOwnerReferences()
}
