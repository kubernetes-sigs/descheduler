package utils

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/component-base/featuregate"
	"k8s.io/klog"
)

const (
	// owner: @jinxu
	// beta: v1.10
	//
	// New local storage types to support local storage capacity isolation
	LocalStorageCapacityIsolation featuregate.Feature = "LocalStorageCapacityIsolation"

	// owner: @egernst
	// alpha: v1.16
	//
	// Enables PodOverhead, for accounting pod overheads which are specific to a given RuntimeClass
	PodOverhead featuregate.Feature = "PodOverhead"
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

	if resourceName == v1.ResourceEphemeralStorage && !utilfeature.DefaultFeatureGate.Enabled(LocalStorageCapacityIsolation) {
		// if the local storage capacity isolation feature gate is disabled, pods request 0 disk
		return requestQuantity
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

	// if PodOverhead feature is supported, add overhead for running a pod
	// to the total requests if the resource total is non-zero
	if pod.Spec.Overhead != nil && utilfeature.DefaultFeatureGate.Enabled(PodOverhead) {
		if podOverhead, ok := pod.Spec.Overhead[resourceName]; ok && !requestQuantity.IsZero() {
			requestQuantity.Add(podOverhead)
		}
	}

	return requestQuantity
}

// IsMirrorPod returns true if the passed Pod is a Mirror Pod.
func IsMirrorPod(pod *v1.Pod) bool {
	_, ok := pod.Annotations[v1.MirrorPodAnnotationKey]
	return ok
}

// IsStaticPod returns true if the pod is a static pod.
func IsStaticPod(pod *v1.Pod) bool {
	source, err := GetPodSource(pod)
	return err == nil && source != "api"
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

// IsCriticalPod returns true if pod's priority is greater than or equal to SystemCriticalPriority.
func IsCriticalPod(pod *v1.Pod) bool {
	if IsStaticPod(pod) {
		return true
	}
	if IsMirrorPod(pod) {
		return true
	}
	if pod.Spec.Priority != nil && IsCriticalPodBasedOnPriority(*pod.Spec.Priority) {
		return true
	}
	return false
}

// IsCriticalPodBasedOnPriority checks if the given pod is a critical pod based on priority resolved from pod Spec.
func IsCriticalPodBasedOnPriority(priority int32) bool {
	return priority >= SystemCriticalPriority
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

	// if PodOverhead feature is supported, add overhead for running a pod
	// to the sum of reqeuests and to non-zero limits:
	if pod.Spec.Overhead != nil && utilfeature.DefaultFeatureGate.Enabled(PodOverhead) {
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
		if len(pod.Spec.Tolerations) >= len(taintsForNode) && TolerationsTolerateTaintsWithFilter(pod.Spec.Tolerations, taintsForNode, nil) {
			return true
		}
		klog.V(5).Infof("pod: %#v doesn't tolerate node %s's taints", pod.Name, nodeName)
	}

	return false
}
