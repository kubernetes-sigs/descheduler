package utils

import (
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"
)

var supportedQoSComputeResources = sets.New(string(v1.ResourceCPU), string(v1.ResourceMemory))

// QOSList is a set of (resource name, QoS class) pairs.
type QOSList map[v1.ResourceName]v1.PodQOSClass

func isSupportedQoSComputeResource(name v1.ResourceName) bool {
	return supportedQoSComputeResources.Has(string(name))
}

var zeroQuantity = resource.MustParse("0")

// GetPodQOS returns the QoS class of a pod.
// A pod is besteffort if none of its containers have specified any requests or limits.
// A pod is guaranteed only when requests and limits are specified for all the containers and they are equal.
// A pod is burstable if limits and requests do not match across all containers.
func GetPodQOS(pod *v1.Pod) v1.PodQOSClass {
	if len(pod.Status.QOSClass) != 0 {
		return pod.Status.QOSClass
	}

	requests := v1.ResourceList{}
	limits := v1.ResourceList{}
	isGuaranteed := true
	for _, container := range getAllContainers(pod) {
		// Use a logical AND operation to accumulate the isGuaranteed status,
		// ensuring that all containers must meet the Guaranteed condition.
		isGuaranteed = processContainerResources(container, &requests, &limits) && isGuaranteed
	}
	if len(requests) == 0 && len(limits) == 0 {
		return v1.PodQOSBestEffort
	}
	// Check is requests match limits for all resources.
	if isGuaranteed && areRequestsMatchingLimits(requests, limits) {
		return v1.PodQOSGuaranteed
	}
	return v1.PodQOSBurstable
}

// processContainerResources processes the resources of a single container and updates the provided requests and limits lists.
func processContainerResources(container v1.Container, requests, limits *v1.ResourceList) bool {
	isGuaranteed := true
	processResourceList(*requests, container.Resources.Requests)
	qosLimitsFound := getQOSResources(container.Resources.Limits)
	processResourceList(*limits, container.Resources.Limits)
	if !qosLimitsFound.HasAll(string(v1.ResourceMemory), string(v1.ResourceCPU)) {
		isGuaranteed = false
	}
	return isGuaranteed
}

// getQOSResources returns a set of resource names from the provided resource list that:
// 1. Are supported QoS compute resources
// 2. Have quantities greater than zero
func getQOSResources(list v1.ResourceList) sets.Set[string] {
	qosResources := sets.New[string]()
	for name, quantity := range list {
		if !isSupportedQoSComputeResource(name) {
			continue
		}
		if quantity.Cmp(zeroQuantity) == 1 {
			qosResources.Insert(string(name))
		}
	}
	return qosResources
}

func getAllContainers(pod *v1.Pod) []v1.Container {
	allContainers := []v1.Container{}
	allContainers = append(allContainers, pod.Spec.Containers...)
	allContainers = append(allContainers, pod.Spec.InitContainers...)
	return allContainers
}

// areRequestsMatchingLimits checks if all resource requests match their respective limits.
func areRequestsMatchingLimits(requests, limits v1.ResourceList) bool {
	for name, req := range requests {
		if lim, exists := limits[name]; !exists || lim.Cmp(req) != 0 {
			return false
		}
	}
	return len(requests) == len(limits)
}

// processResourceList adds non-zero quantities for supported QoS compute resources
// quantities from newList to list.
func processResourceList(list, newList v1.ResourceList) {
	for name, quantity := range newList {
		if !isSupportedQoSComputeResource(name) {
			continue
		}
		if quantity.Cmp(zeroQuantity) == 1 {
			delta := quantity.DeepCopy()
			if _, exists := list[name]; !exists {
				list[name] = delta
			} else {
				delta.Add(list[name])
				list[name] = delta
			}
		}
	}
}
