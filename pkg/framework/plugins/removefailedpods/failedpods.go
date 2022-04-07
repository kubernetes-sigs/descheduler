package removefailedpods

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/descheduler/strategies/validation"
	"sigs.k8s.io/descheduler/pkg/framework"
	"sigs.k8s.io/descheduler/pkg/utils"
)

const PluginName = "RemoveFailedPods"

// RemoveFailedPods removes Pods that are in failed status phase.
type RemoveFailedPods struct {
	handle            framework.Handle
	args              *framework.RemoveFailedPodsArg
	reasons           sets.String
	excludeOwnerKinds sets.String
	podFilter         podutil.FilterFunc
}

var _ framework.Plugin = &RemoveFailedPods{}
var _ framework.DeschedulePlugin = &RemoveFailedPods{}

func New(args runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	failedPodsArg, ok := args.(*framework.RemoveFailedPodsArg)
	if !ok {
		return nil, fmt.Errorf("want args to be of type RemoveFailedPodsArg, got %T", args)
	}

	if err := framework.ValidateCommonArgs(failedPodsArg.CommonArgs); err != nil {
		return nil, err
	}

	thresholdPriority, err := utils.GetPriorityValueFromPriorityThreshold(context.TODO(), handle.ClientSet(), failedPodsArg.PriorityThreshold)
	if err != nil {
		return nil, fmt.Errorf("failed to get priority threshold: %v", err)
	}

	var selector labels.Selector
	if failedPodsArg.LabelSelector != nil {
		selector, err = metav1.LabelSelectorAsSelector(failedPodsArg.LabelSelector)
		if err != nil {
			return nil, fmt.Errorf("failed to get label selectors: %v", err)
		}
	}

	evictable := handle.PodEvictor().Evictable(
		evictions.WithPriorityThreshold(thresholdPriority),
		evictions.WithNodeFit(failedPodsArg.NodeFit),
		evictions.WithLabelSelector(selector),
	)

	var includedNamespaces, excludedNamespaces sets.String
	if failedPodsArg.Namespaces != nil {
		includedNamespaces = sets.NewString(failedPodsArg.Namespaces.Include...)
		excludedNamespaces = sets.NewString(failedPodsArg.Namespaces.Exclude...)
	}

	podFilter, err := podutil.NewOptions().
		WithFilter(evictable.IsEvictable).
		WithNamespaces(includedNamespaces).
		WithoutNamespaces(excludedNamespaces).
		WithLabelSelector(failedPodsArg.LabelSelector).
		BuildFilterFunc()
	if err != nil {
		return nil, fmt.Errorf("error initializing pod filter function: %v", err)
	}

	// Only list failed pods
	phaseFilter := func(pod *v1.Pod) bool { return pod.Status.Phase == v1.PodFailed }
	podFilter = podutil.WrapFilterFuncs(phaseFilter, podFilter)

	return &RemoveFailedPods{
		handle:            handle,
		args:              failedPodsArg,
		excludeOwnerKinds: sets.NewString(failedPodsArg.ExcludeOwnerKinds...),
		reasons:           sets.NewString(failedPodsArg.Reasons...),
		podFilter:         podFilter,
	}, nil
}

func (d *RemoveFailedPods) Name() string {
	return PluginName
}

func (d *RemoveFailedPods) Deschedule(ctx context.Context, nodes []*v1.Node) *framework.Status {
	for _, node := range nodes {
		klog.V(1).InfoS("Processing node", "node", klog.KObj(node))
		pods, err := podutil.ListAllPodsOnANode(node.Name, d.handle.GetPodsAssignedToNodeFunc(), d.podFilter)
		if err != nil {
			klog.ErrorS(err, "Error listing a nodes failed pods", "node", klog.KObj(node))
			continue
		}

		for i, pod := range pods {
			if err = d.validateFailedPodShouldEvict(pod); err != nil {
				klog.V(4).InfoS(fmt.Sprintf("ignoring pod for eviction due to: %s", err.Error()), "pod", klog.KObj(pod))
				continue
			}

			if _, err = d.handle.PodEvictor().EvictPod(ctx, pods[i], node, "FailedPod"); err != nil {
				klog.ErrorS(err, "Error evicting pod", "pod", klog.KObj(pod))
				break
			}
		}
	}
	return nil
}

// validatedFailedPodsStrategyParams contains validated strategy parameters
type validatedFailedPodsStrategyParams struct {
	validation.ValidatedStrategyParams
	includingInitContainers bool
	reasons                 sets.String
	excludeOwnerKinds       sets.String
	minPodLifetimeSeconds   *uint
}

// validateFailedPodShouldEvict looks at strategy params settings to see if the Pod
// should be evicted given the params in the PodFailed policy.
func (d *RemoveFailedPods) validateFailedPodShouldEvict(pod *v1.Pod) error {
	var errs []error

	if d.args.MinPodLifetimeSeconds != nil {
		podAgeSeconds := uint(metav1.Now().Sub(pod.GetCreationTimestamp().Local()).Seconds())
		if podAgeSeconds < *d.args.MinPodLifetimeSeconds {
			errs = append(errs, fmt.Errorf("pod does not exceed the min age seconds of %d", *d.args.MinPodLifetimeSeconds))
		}
	}

	if len(d.excludeOwnerKinds) > 0 {
		ownerRefList := podutil.OwnerRef(pod)
		for _, owner := range ownerRefList {
			if d.excludeOwnerKinds.Has(owner.Kind) {
				errs = append(errs, fmt.Errorf("pod's owner kind of %s is excluded", owner.Kind))
			}
		}
	}

	if len(d.args.Reasons) > 0 {
		reasons := getFailedContainerStatusReasons(pod.Status.ContainerStatuses)

		if pod.Status.Phase == v1.PodFailed && pod.Status.Reason != "" {
			reasons = append(reasons, pod.Status.Reason)
		}

		if d.args.IncludingInitContainers {
			reasons = append(reasons, getFailedContainerStatusReasons(pod.Status.InitContainerStatuses)...)
		}

		if !d.reasons.HasAny(reasons...) {
			errs = append(errs, fmt.Errorf("pod does not match any of the reasons"))
		}
	}

	return utilerrors.NewAggregate(errs)
}

func getFailedContainerStatusReasons(containerStatuses []v1.ContainerStatus) []string {
	reasons := make([]string, 0)

	for _, containerStatus := range containerStatuses {
		if containerStatus.State.Waiting != nil && containerStatus.State.Waiting.Reason != "" {
			reasons = append(reasons, containerStatus.State.Waiting.Reason)
		}
		if containerStatus.State.Terminated != nil && containerStatus.State.Terminated.Reason != "" {
			reasons = append(reasons, containerStatus.State.Terminated.Reason)
		}
	}

	return reasons
}
