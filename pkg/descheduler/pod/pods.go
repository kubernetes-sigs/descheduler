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
	"fmt"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/errors"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	"sigs.k8s.io/descheduler/pkg/descheduler/strategies/options"
	"sigs.k8s.io/descheduler/pkg/utils"
)

const (
	evictPodAnnotationKey = "descheduler.alpha.kubernetes.io/evict"
)

// IsEvictable checks if a pod is evictable or not.
func IsEvictable(pod *v1.Pod, evictLocalStoragePods bool) bool {
	checkErrs := []error{}
	if IsCriticalPod(pod) {
		checkErrs = append(checkErrs, fmt.Errorf("pod is critical"))
	}

	ownerRefList := OwnerRef(pod)
	if IsDaemonsetPod(ownerRefList) {
		checkErrs = append(checkErrs, fmt.Errorf("pod is a DaemonSet pod"))
	}

	if len(ownerRefList) == 0 {
		checkErrs = append(checkErrs, fmt.Errorf("pod does not have any ownerrefs"))
	}

	if !evictLocalStoragePods && IsPodWithLocalStorage(pod) {
		checkErrs = append(checkErrs, fmt.Errorf("pod has local storage and descheduler is not configured with --evict-local-storage-pods"))
	}

	if IsMirrorPod(pod) {
		checkErrs = append(checkErrs, fmt.Errorf("pod is a mirror pod"))
	}

	if len(checkErrs) > 0 && !HaveEvictAnnotation(pod) {
		klog.V(4).Infof("Pod %s in namespace %s is not evictable: Pod lacks an eviction annotation and fails the following checks: %v", pod.Name, pod.Namespace, errors.NewAggregate(checkErrs).Error())
		return false
	}
	return true
}

// ListEvictablePodsOnNode returns the list of evictable pods on node.
func ListEvictablePodsOnNode(ctx context.Context, client clientset.Interface, node *v1.Node, opts options.Options) ([]*v1.Pod, error) {
	pods, err := ListPodsOnANode(ctx, client, node)
	if err != nil {
		return []*v1.Pod{}, err
	}
	evictablePods := make([]*v1.Pod, 0)
	for _, pod := range pods {
		if !IsEvictable(pod, opts.EvictLocalStoragePods) {
			continue
		} else {
			evictablePods = append(evictablePods, pod)
		}
	}
	return evictablePods, nil
}

func ListPodsOnANode(ctx context.Context, client clientset.Interface, node *v1.Node) ([]*v1.Pod, error) {
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
		pods = append(pods, &podList.Items[i])
	}
	return pods, nil
}

func IsCriticalPod(pod *v1.Pod) bool {
	return utils.IsCriticalPod(pod)
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
	return utils.IsMirrorPod(pod)
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
