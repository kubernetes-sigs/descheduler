package defaultevictor

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/errors"

	nodeutil "sigs.k8s.io/descheduler/pkg/descheduler/node"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/framework"
	"sigs.k8s.io/descheduler/pkg/utils"

	"k8s.io/klog/v2"
)

const (
	PluginName            = "DefaultEvictor"
	evictPodAnnotationKey = "descheduler.alpha.kubernetes.io/evict"
)

type constraint func(pod *v1.Pod) error

// PodLifeTime evicts pods on nodes that were created more than strategy.Params.MaxPodLifeTimeSeconds seconds ago.
type DefaultEvictor struct {
	handle      framework.Handle
	constraints []constraint
}

var _ framework.Plugin = &DefaultEvictor{}

var _ framework.EvictPlugin = &DefaultEvictor{}
var _ framework.SortPlugin = &DefaultEvictor{}

func New(args runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	evictorArgs, ok := args.(*framework.DefaultEvictorArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type DefaultEvictorArgs, got %T", args)
	}

	constraints := []constraint{}

	if evictorArgs.EvictFailedBarePods {
		klog.V(1).InfoS("Warning: EvictFailedBarePods is set to True. This could cause eviction of pods without ownerReferences.")
		constraints = append(constraints, func(pod *v1.Pod) error {
			ownerRefList := podutil.OwnerRef(pod)
			// Enable evictFailedBarePods to evict bare pods in failed phase
			if len(ownerRefList) == 0 && pod.Status.Phase != v1.PodFailed {
				return fmt.Errorf("pod does not have any ownerRefs and is not in failed phase")
			}
			return nil
		})
	} else {
		constraints = append(constraints, func(pod *v1.Pod) error {
			ownerRefList := podutil.OwnerRef(pod)
			// Moved from IsEvictable function for backward compatibility
			if len(ownerRefList) == 0 {
				return fmt.Errorf("pod does not have any ownerRefs")
			}
			return nil
		})
	}

	if !evictorArgs.EvictSystemCriticalPods {
		klog.V(1).InfoS("Warning: EvictSystemCriticalPods is set to True. This could cause eviction of Kubernetes system pods.")
		constraints = append(constraints, func(pod *v1.Pod) error {
			// Moved from IsEvictable function to allow for disabling
			if utils.IsCriticalPriorityPod(pod) {
				return fmt.Errorf("pod has system critical priority")
			}
			return nil
		})

		if evictorArgs.PriorityThreshold != nil {
			thresholdPriority, err := utils.GetPriorityValueFromPriorityThreshold(context.TODO(), handle.ClientSet(), evictorArgs.PriorityThreshold)
			if err != nil {
				return nil, fmt.Errorf("failed to get priority threshold: %v", err)
			}
			constraints = append(constraints, func(pod *v1.Pod) error {
				if isPodEvictableBasedOnPriority(pod, thresholdPriority) {
					return nil
				}
				return fmt.Errorf("pod has higher priority than specified priority class threshold")
			})
		}
	}

	if !evictorArgs.EvictLocalStoragePods {
		constraints = append(constraints, func(pod *v1.Pod) error {
			if utils.IsPodWithLocalStorage(pod) {
				return fmt.Errorf("pod has local storage and descheduler is not configured with evictLocalStoragePods")
			}
			return nil
		})
	}
	if evictorArgs.IgnorePvcPods {
		constraints = append(constraints, func(pod *v1.Pod) error {
			if utils.IsPodWithPVC(pod) {
				return fmt.Errorf("pod has a PVC and descheduler is configured to ignore PVC pods")
			}
			return nil
		})
	}
	if evictorArgs.NodeFit {
		constraints = append(constraints, func(pod *v1.Pod) error {
			// TODO(jchaloup): should the list of ready nodes be captured? Or, do we want the latest greatest about the nodes?
			nodes, err := nodeutil.ReadyNodes(context.TODO(), handle.ClientSet(), handle.SharedInformerFactory().Core().V1().Nodes(), evictorArgs.NodeSelector)
			if err != nil {
				return fmt.Errorf("unable to list ready nodes: %v", err)
			}
			if !nodeutil.PodFitsAnyOtherNode(pod, nodes) {
				return fmt.Errorf("pod does not fit on any other node because of nodeSelector(s), Taint(s), or nodes marked as unschedulable")
			}
			return nil
		})
	}

	if evictorArgs.LabelSelector != nil {
		selector, err := metav1.LabelSelectorAsSelector(evictorArgs.LabelSelector)
		if err != nil {
			return nil, fmt.Errorf("failed to get label selectors: %v", err)
		}
		if !selector.Empty() {
			constraints = append(constraints, func(pod *v1.Pod) error {
				if !selector.Matches(labels.Set(pod.Labels)) {
					return fmt.Errorf("pod labels do not match the labelSelector filter in the policy parameter")
				}
				return nil
			})
		}
	}

	return &DefaultEvictor{
		handle:      handle,
		constraints: constraints,
	}, nil
}

func (d *DefaultEvictor) Name() string {
	return PluginName
}

// sort based on priority
func (de *DefaultEvictor) Less(podi *v1.Pod, podj *v1.Pod) bool {
	if podi.Spec.Priority == nil && podj.Spec.Priority != nil {
		return true
	}
	if podj.Spec.Priority == nil && podi.Spec.Priority != nil {
		return false
	}
	if (podj.Spec.Priority == nil && podi.Spec.Priority == nil) || (*podi.Spec.Priority == *podj.Spec.Priority) {
		if isBestEffortPod(podi) {
			return true
		}
		if isBurstablePod(podi) && isGuaranteedPod(podj) {
			return true
		}
		return false
	}
	return *podi.Spec.Priority < *podj.Spec.Priority
}

func isBestEffortPod(pod *v1.Pod) bool {
	return utils.GetPodQOS(pod) == v1.PodQOSBestEffort
}

func isBurstablePod(pod *v1.Pod) bool {
	return utils.GetPodQOS(pod) == v1.PodQOSBurstable
}

func isGuaranteedPod(pod *v1.Pod) bool {
	return utils.GetPodQOS(pod) == v1.PodQOSGuaranteed
}

func (de *DefaultEvictor) Filter(pod *v1.Pod) bool {
	checkErrs := []error{}

	ownerRefList := podutil.OwnerRef(pod)
	if utils.IsDaemonsetPod(ownerRefList) {
		checkErrs = append(checkErrs, fmt.Errorf("pod is a DaemonSet pod"))
	}

	if utils.IsMirrorPod(pod) {
		checkErrs = append(checkErrs, fmt.Errorf("pod is a mirror pod"))
	}

	if utils.IsStaticPod(pod) {
		checkErrs = append(checkErrs, fmt.Errorf("pod is a static pod"))
	}

	if utils.IsPodTerminating(pod) {
		checkErrs = append(checkErrs, fmt.Errorf("pod is terminating"))
	}

	if len(checkErrs) == 0 {
		for _, c := range de.constraints {
			if err := c(pod); err != nil {
				checkErrs = append(checkErrs, err)
			}
		}
	}

	if len(checkErrs) > 0 && !haveEvictAnnotation(pod) {
		klog.V(4).InfoS("Pod lacks an eviction annotation and fails the following checks", "pod", klog.KObj(pod), "checks", errors.NewAggregate(checkErrs).Error())
		return false
	}

	return true
}

// isPodEvictableBasedOnPriority checks if the given pod is evictable based on priority resolved from pod Spec.
func isPodEvictableBasedOnPriority(pod *v1.Pod, priority int32) bool {
	return pod.Spec.Priority == nil || *pod.Spec.Priority < priority
}

// HaveEvictAnnotation checks if the pod have evict annotation
func haveEvictAnnotation(pod *v1.Pod) bool {
	_, found := pod.ObjectMeta.Annotations[evictPodAnnotationKey]
	return found
}
