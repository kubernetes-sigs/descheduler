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
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	clientset "k8s.io/client-go/kubernetes"
	api "k8s.io/kubernetes/pkg/apis/core"
	"k8s.io/kubernetes/pkg/apis/core/v1/helper/qos"
	"k8s.io/kubernetes/pkg/kubelet/types"
)

const (
	evictPodAnnotationKey = "descheduler.alpha.kubernetes.io/evict"
)

// IsEvictable checks if a pod is evictable or not.
func IsEvictable(pod *v1.Pod, evictLocalStoragePods bool) bool {
	ownerRefList := OwnerRef(pod)
	if !HaveEvictAnnotation(pod) && (IsMirrorPod(pod) || (!evictLocalStoragePods && IsPodWithLocalStorage(pod)) || len(ownerRefList) == 0 || IsDaemonsetPod(ownerRefList) || IsCriticalPod(pod)) {
		return false
	}
	return true
}

// ListEvictablePodsOnNode returns the list of evictable pods on node.
func ListEvictablePodsOnNode(client clientset.Interface, node *v1.Node, evictLocalStoragePods bool) ([]*v1.Pod, error) {
	pods, err := ListPodsOnANode(client, node)
	if err != nil {
		return []*v1.Pod{}, err
	}
	evictablePods := make([]*v1.Pod, 0)
	for _, pod := range pods {
		if !IsEvictable(pod, evictLocalStoragePods) {
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

func IsCriticalPod(pod *v1.Pod) bool {
	return IsCritical(pod.Namespace, pod.Annotations)
}

func IsCritical(ns string, annotations map[string]string) bool {
	// Critical pods are restricted to "kube-system" and "openshift-*" namespaces as of now.
	// for openshift-* namespaces, see: https://github.com/openshift/origin/pull/19104
	if ns != api.NamespaceSystem && !strings.HasPrefix(ns, "openshift-") {
		return false
	}
	val, ok := annotations[types.CriticalPodAnnotationKey]
	if ok && val == "" {
		return true
	}
	return false
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

// HaveEvictAnnotation checks if the pod have evict annotation
func HaveEvictAnnotation(pod *v1.Pod) bool {
	_, found := pod.ObjectMeta.Annotations[evictPodAnnotationKey]
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
