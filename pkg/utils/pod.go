package utils

import (
	"fmt"

	policyv1 "k8s.io/client-go/listers/policy/v1"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// GetResourceRequest finds and returns the request value for a specific resource.
func GetResourceRequest(pod *v1.Pod, resource v1.ResourceName) int64 {
	if resource == v1.ResourcePods {
		return 1
	}

	requestQuantity := GetResourceRequestQuantity(pod, resource)

	if resource == v1.ResourceCPU {
		return requestQuantity.MilliValue()
	}

	return requestQuantity.Value()
}

// GetResourceRequestQuantity finds and returns the request quantity for a specific resource.
func GetResourceRequestQuantity(pod *v1.Pod, resourceName v1.ResourceName) resource.Quantity {
	requestQuantity := resource.Quantity{}

	switch resourceName {
	case v1.ResourceCPU:
		requestQuantity = resource.Quantity{Format: resource.DecimalSI}
	case v1.ResourceMemory, v1.ResourceStorage, v1.ResourceEphemeralStorage:
		requestQuantity = resource.Quantity{Format: resource.BinarySI}
	default:
		requestQuantity = resource.Quantity{Format: resource.DecimalSI}
	}

	for _, container := range pod.Spec.Containers {
		if rQuantity, ok := container.Resources.Requests[resourceName]; ok {
			requestQuantity.Add(rQuantity)
		}
	}

	for _, container := range pod.Spec.InitContainers {
		if rQuantity, ok := container.Resources.Requests[resourceName]; ok {
			if requestQuantity.Cmp(rQuantity) < 0 {
				requestQuantity = rQuantity.DeepCopy()
			}
		}
	}

	// We assume pod overhead feature gate is enabled.
	// We can't import the scheduler settings so we will inherit the default.
	if pod.Spec.Overhead != nil {
		if podOverhead, ok := pod.Spec.Overhead[resourceName]; ok && !requestQuantity.IsZero() {
			requestQuantity.Add(podOverhead)
		}
	}

	return requestQuantity
}

// IsMirrorPod returns true if the pod is a Mirror Pod.
func IsMirrorPod(pod *v1.Pod) bool {
	_, ok := pod.Annotations[v1.MirrorPodAnnotationKey]
	return ok
}

// IsPodTerminating returns true if the pod DeletionTimestamp is set.
func IsPodTerminating(pod *v1.Pod) bool {
	return pod.DeletionTimestamp != nil
}

// IsStaticPod returns true if the pod is a static pod.
func IsStaticPod(pod *v1.Pod) bool {
	source, err := GetPodSource(pod)
	return err == nil && source != "api"
}

// IsCriticalPriorityPod returns true if the pod has critical priority.
func IsCriticalPriorityPod(pod *v1.Pod) bool {
	return pod.Spec.Priority != nil && *pod.Spec.Priority >= SystemCriticalPriority
}

// IsDaemonsetPod returns true if the pod is a IsDaemonsetPod.
func IsDaemonsetPod(ownerRefList []metav1.OwnerReference) bool {
	for _, ownerRef := range ownerRefList {
		if ownerRef.Kind == "DaemonSet" {
			return true
		}
	}
	return false
}

// IsPodWithLocalStorage returns true if the pod has local storage.
func IsPodWithLocalStorage(pod *v1.Pod) bool {
	for _, volume := range pod.Spec.Volumes {
		if volume.HostPath != nil || volume.EmptyDir != nil {
			return true
		}
	}

	return false
}

// IsPodWithPVC returns true if the pod has claimed a persistent volume.
func IsPodWithPVC(pod *v1.Pod) bool {
	for _, volume := range pod.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil {
			return true
		}
	}
	return false
}

// IsPodCoveredByPDB returns true if the pod is covered by at least one PodDisruptionBudget.
func IsPodCoveredByPDB(pod *v1.Pod, lister policyv1.PodDisruptionBudgetLister) (bool, error) {
	budgets, err := lister.GetPodPodDisruptionBudgets(pod)
	return len(budgets) > 0, err
}

// GetPodSource returns the source of the pod based on the annotation.
func GetPodSource(pod *v1.Pod) (string, error) {
	if pod.Annotations != nil {
		if source, ok := pod.Annotations["kubernetes.io/config.source"]; ok {
			return source, nil
		}
	}
	return "", fmt.Errorf("cannot get source of pod %q", pod.UID)
}

// PodRequestsAndLimits returns a dictionary of all defined resources summed up for all
// containers of the pod. If PodOverhead feature is enabled, pod overhead is added to the
// total container resource requests and to the total container limits which have a
// non-zero quantity.
func PodRequestsAndLimits(pod *v1.Pod) (reqs, limits v1.ResourceList) {
	reqs, limits = v1.ResourceList{}, v1.ResourceList{}
	for _, container := range pod.Spec.Containers {
		addResourceList(reqs, container.Resources.Requests)
		addResourceList(limits, container.Resources.Limits)
	}
	// init containers define the minimum of any resource
	for _, container := range pod.Spec.InitContainers {
		maxResourceList(reqs, container.Resources.Requests)
		maxResourceList(limits, container.Resources.Limits)
	}

	// We assume pod overhead feature gate is enabled.
	// We can't import the scheduler settings so we will inherit the default.
	if pod.Spec.Overhead != nil {
		addResourceList(reqs, pod.Spec.Overhead)

		for name, quantity := range pod.Spec.Overhead {
			if value, ok := limits[name]; ok && !value.IsZero() {
				value.Add(quantity)
				limits[name] = value
			}
		}
	}

	return
}

// addResourceList adds the resources in newList to list
func addResourceList(list, newList v1.ResourceList) {
	for name, quantity := range newList {
		if value, ok := list[name]; !ok {
			list[name] = quantity.DeepCopy()
		} else {
			value.Add(quantity)
			list[name] = value
		}
	}
}

// maxResourceList sets list to the greater of list/newList for every resource
// either list
func maxResourceList(list, new v1.ResourceList) {
	for name, quantity := range new {
		if value, ok := list[name]; !ok {
			list[name] = quantity.DeepCopy()
			continue
		} else {
			if quantity.Cmp(value) > 0 {
				list[name] = quantity.DeepCopy()
			}
		}
	}
}

// PodToleratesTaints returns true if a pod tolerates one node's taints
func PodToleratesTaints(pod *v1.Pod, taintsOfNodes map[string][]v1.Taint) bool {
	for nodeName, taintsForNode := range taintsOfNodes {
		if len(pod.Spec.Tolerations) >= len(taintsForNode) {

			if TolerationsTolerateTaintsWithFilter(pod.Spec.Tolerations, taintsForNode, nil) {
				return true
			}

			if klog.V(5).Enabled() {
				for i := range taintsForNode {
					if !TolerationsTolerateTaint(pod.Spec.Tolerations, &taintsForNode[i]) {
						klog.V(5).InfoS("Pod doesn't tolerate node taint",
							"pod", klog.KObj(pod),
							"nodeName", nodeName,
							"taint", fmt.Sprintf("%s:%s=%s", taintsForNode[i].Key, taintsForNode[i].Value, taintsForNode[i].Effect),
						)
					}
				}
			}
		} else {
			klog.V(5).InfoS("Pod doesn't tolerate nodes taint, count mismatch",
				"pod", klog.KObj(pod),
				"nodeName", nodeName,
			)
		}
	}
	return false
}

// PodHasNodeAffinity returns true if the pod has a node affinity of type
// `nodeAffinityType` defined. The nodeAffinityType param can take this two values:
// "requiredDuringSchedulingIgnoredDuringExecution" or "requiredDuringSchedulingIgnoredDuringExecution"
func PodHasNodeAffinity(pod *v1.Pod, nodeAffinityType NodeAffinityType) bool {
	if pod.Spec.Affinity == nil {
		return false
	}
	if pod.Spec.Affinity.NodeAffinity == nil {
		return false
	}
	if nodeAffinityType == RequiredDuringSchedulingIgnoredDuringExecution {
		return pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil
	} else if nodeAffinityType == PreferredDuringSchedulingIgnoredDuringExecution {
		return len(pod.Spec.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution) > 0
	}
	return false
}

type NodeAffinityType string

const (
	RequiredDuringSchedulingIgnoredDuringExecution  NodeAffinityType = "requiredDuringSchedulingIgnoredDuringExecution"
	PreferredDuringSchedulingIgnoredDuringExecution NodeAffinityType = "preferredDuringSchedulingIgnoredDuringExecution"
)
