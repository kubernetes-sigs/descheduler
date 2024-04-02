package utils

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"sigs.k8s.io/descheduler/pkg/api"
)

const SystemCriticalPriority = 2 * int32(1000000000)

// getNamespacesFromPodAffinityTerm returns a set of names
// according to the namespaces indicated in podAffinityTerm.
// If namespaces is empty it considers the given pod's namespace.
func getNamespacesFromPodAffinityTerm(pod *v1.Pod, podAffinityTerm *v1.PodAffinityTerm) sets.Set[string] {
	names := sets.New[string]()
	if len(podAffinityTerm.Namespaces) == 0 {
		names.Insert(pod.Namespace)
	} else {
		names.Insert(podAffinityTerm.Namespaces...)
	}
	return names
}

// podMatchesTermsNamespaceAndSelector returns true if the given <pod>
// matches the namespace and selector defined by <affinityPod>`s <term>.
func podMatchesTermsNamespaceAndSelector(existingPod, evaluatedPod *v1.Pod, namespaces sets.Set[string], selector labels.Selector) bool {
	if !namespaces.Has(existingPod.Namespace) {
		klog.V(4).InfoS("antiaffinity check: pod does not match namespace", "pod", klog.KObj(existingPod), "given namespaces", klog.KObjs(namespaces))
		return false
	}

	if !selector.Matches(labels.Set(existingPod.Labels)) {
		klog.V(4).InfoS("antiaffinity check: selector of Pod being evaluated does not match existing Pod's labels", "evaluated pod", klog.KObj(evaluatedPod), "existing pod", klog.KObj(existingPod))
		return false
	}

	klog.V(4).InfoS("antiaffinity check: selector of Pod being evaluated matches labels of existing Pod", "evaluated pod", klog.KObj(evaluatedPod), "existing pod", klog.KObj(existingPod))
	return true
}

// GetPriorityFromPriorityClass gets priority from the given priority class.
// If no priority class is provided, it will return SystemCriticalPriority by default.
func GetPriorityFromPriorityClass(ctx context.Context, client clientset.Interface, name string) (int32, error) {
	if name != "" {
		priorityClass, err := client.SchedulingV1().PriorityClasses().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return 0, err
		}
		return priorityClass.Value, nil
	}
	return SystemCriticalPriority, nil
}

// GetPriorityValueFromPriorityThreshold gets priority from the given PriorityThreshold.
// It will return SystemCriticalPriority by default.
func GetPriorityValueFromPriorityThreshold(ctx context.Context, client clientset.Interface, priorityThreshold *api.PriorityThreshold) (priority int32, err error) {
	if priorityThreshold == nil {
		return SystemCriticalPriority, nil
	}
	if priorityThreshold.Value != nil {
		priority = *priorityThreshold.Value
	} else {
		priority, err = GetPriorityFromPriorityClass(ctx, client, priorityThreshold.Name)
		if err != nil {
			return 0, fmt.Errorf("unable to get priority value from the priority class: %v", err)
		}
	}
	if priority > SystemCriticalPriority {
		return 0, fmt.Errorf("priority threshold can't be greater than %d", SystemCriticalPriority)
	}
	return
}
