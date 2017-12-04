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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/v1"
	"k8s.io/kubernetes/pkg/api/v1/helper/qos"
	"k8s.io/kubernetes/pkg/client/clientset_generated/clientset"
	"k8s.io/kubernetes/pkg/kubelet/types"
)

// Map to hold pod Name to serialized reference.
type PodRefMap map[string]*v1.SerializedReference

// A global variable to hold pod Name to serialized reference.
// TODO: Make this metadata accessible across all strategies. Some items that could go in include podlist on node etc.
var NewPodRefMap = PodRefMap{}

// IsEvictable checks if a pod is evictable or not.
func IsEvictable(pod *v1.Pod) bool {
	sr, err := CreatorRef(pod)
	if err != nil {
		sr = nil
	}
	if IsMirrorPod(pod) || IsPodWithLocalStorage(pod) || sr == nil || IsDaemonsetPod(sr) || IsCriticalPod(pod) {
		return false
	}
	NewPodRefMap[pod.Name] = sr
	return true
}

// ListEvictablePodsOnNode returns the list of evictable pods on node.
func ListEvictablePodsOnNode(client clientset.Interface, node *v1.Node) ([]*v1.Pod, error) {
	pods, err := ListPodsOnANode(client, node)
	if err != nil {
		return []*v1.Pod{}, err
	}
	evictablePods := make([]*v1.Pod, 0)
	for _, pod := range pods {
		if !IsEvictable(pod) {
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
	return types.IsCriticalPod(pod)
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

func IsDaemonsetPod(sr *v1.SerializedReference) bool {
	if sr != nil {
		return sr.Reference.Kind == "DaemonSet"
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

// CreatorRef returns the kind of the creator reference of the pod.
func CreatorRef(pod *v1.Pod) (*v1.SerializedReference, error) {
	creatorRef, found := pod.ObjectMeta.Annotations[v1.CreatedByAnnotation]
	if !found {
		return nil, nil
	}
	var sr v1.SerializedReference
	if err := runtime.DecodeInto(api.Codecs.UniversalDecoder(), []byte(creatorRef), &sr); err != nil {
		return nil, err
	}
	return &sr, nil
}
