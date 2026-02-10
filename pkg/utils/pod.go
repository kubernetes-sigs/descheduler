package utils

import (
	"context"
	"fmt"

	policy "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/labels"

	policyv1 "k8s.io/client-go/listers/policy/v1"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	resourcehelper "k8s.io/component-helpers/resource"
	"k8s.io/klog/v2"
)

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

	if rQuantity, ok := resourcehelper.PodRequests(pod, resourcehelper.PodResourcesOptions{})[resourceName]; ok {
		requestQuantity.Add(rQuantity)
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

// IsPodWithResourceClaims returns true if the pod has resource claims.
func IsPodWithResourceClaims(pod *v1.Pod) bool {
	return len(pod.Spec.ResourceClaims) != 0
}

// IsPodCoveredByPDB returns true if the pod is covered by at least one PodDisruptionBudget.
func IsPodCoveredByPDB(pod *v1.Pod, lister policyv1.PodDisruptionBudgetLister) (bool, error) {
	// We can't use the GetPodPodDisruptionBudgets expansion method here because it treats no pdb as an error,
	// but we want to return false.

	list, err := lister.PodDisruptionBudgets(pod.Namespace).List(labels.Everything())
	if err != nil {
		return false, err
	}

	if len(list) == 0 {
		return false, nil
	}

	podLabels := labels.Set(pod.Labels)
	var pdbList []*policy.PodDisruptionBudget
	for _, pdb := range list {
		selector, err := metav1.LabelSelectorAsSelector(pdb.Spec.Selector)
		if err != nil {
			// This object has an invalid selector, it will never match the pod
			continue
		}

		if !selector.Matches(podLabels) {
			continue
		}
		pdbList = append(pdbList, pdb)
	}

	return len(pdbList) > 0, nil
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
func PodRequestsAndLimits(pod *v1.Pod) (v1.ResourceList, v1.ResourceList) {
	opts := resourcehelper.PodResourcesOptions{}
	return resourcehelper.PodRequests(pod, opts), resourcehelper.PodLimits(pod, opts)
}

// PodToleratesTaints returns true if a pod tolerates one node's taints
func PodToleratesTaints(ctx context.Context, pod *v1.Pod, taintsOfNodes map[string][]v1.Taint) bool {
	for nodeName, taintsForNode := range taintsOfNodes {
		if len(pod.Spec.Tolerations) >= len(taintsForNode) {

			if TolerationsTolerateTaintsWithFilter(ctx, pod.Spec.Tolerations, taintsForNode, nil) {
				return true
			}

			if klog.V(5).Enabled() {
				for i := range taintsForNode {
					if !TolerationsTolerateTaint(ctx, pod.Spec.Tolerations, &taintsForNode[i]) {
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
