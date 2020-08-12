package utils

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	clientset "k8s.io/client-go/kubernetes"
	"sigs.k8s.io/descheduler/pkg/api"
)

const SystemCriticalPriority = 2 * int32(1000000000)

// GetNamespacesFromPodAffinityTerm returns a set of names
// according to the namespaces indicated in podAffinityTerm.
// If namespaces is empty it considers the given pod's namespace.
func GetNamespacesFromPodAffinityTerm(pod *v1.Pod, podAffinityTerm *v1.PodAffinityTerm) sets.String {
	names := sets.String{}
	if len(podAffinityTerm.Namespaces) == 0 {
		names.Insert(pod.Namespace)
	} else {
		names.Insert(podAffinityTerm.Namespaces...)
	}
	return names
}

// PodMatchesTermsNamespaceAndSelector returns true if the given <pod>
// matches the namespace and selector defined by <affinityPod>`s <term>.
func PodMatchesTermsNamespaceAndSelector(pod *v1.Pod, namespaces sets.String, selector labels.Selector) bool {
	if !namespaces.Has(pod.Namespace) {
		return false
	}

	if !selector.Matches(labels.Set(pod.Labels)) {
		return false
	}
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

// GetPriorityFromStrategyParams gets priority from the given StrategyParameters.
// It will return SystemCriticalPriority by default.
func GetPriorityFromStrategyParams(ctx context.Context, client clientset.Interface, params *api.StrategyParameters) (priority int32, err error) {
	if params == nil {
		return SystemCriticalPriority, nil
	}
	if params.ThresholdPriority != nil {
		priority = *params.ThresholdPriority
	} else {
		priority, err = GetPriorityFromPriorityClass(ctx, client, params.ThresholdPriorityClassName)
		if err != nil {
			return
		}
	}
	if priority > SystemCriticalPriority {
		return 0, fmt.Errorf("Priority threshold can't be greater than %d", SystemCriticalPriority)
	}
	return
}
