package strategies

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/util/sets"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/descheduler/strategies/validation"
)

// validatedFailedPodsStrategyParams contains validated strategy parameters
type validatedFailedPodsStrategyParams struct {
	validation.ValidatedStrategyParams
	includingInitContainers bool
	reasons                 sets.String
	excludeOwnerKinds       sets.String
	minPodLifeTimeSeconds   *uint
}

// RemoveFailedPods removes Pods that are in failed status phase.
func RemoveFailedPods(
	ctx context.Context,
	client clientset.Interface,
	strategy api.DeschedulerStrategy,
	nodes []*v1.Node,
	podEvictor *evictions.PodEvictor,
) {
	strategyParams, err := validateAndParseRemoveFailedPodsParams(ctx, client, strategy.Params)
	if err != nil {
		klog.ErrorS(err, "Invalid RemoveFailedPods parameters")
		return
	}

	evictable := podEvictor.Evictable(
		evictions.WithPriorityThreshold(strategyParams.ThresholdPriority),
		evictions.WithNodeFit(strategyParams.NodeFit),
		evictions.WithLabelSelector(strategyParams.LabelSelector),
	)

	var labelSelector *metav1.LabelSelector
	if strategy.Params != nil {
		labelSelector = strategy.Params.LabelSelector
	}

	for _, node := range nodes {
		klog.V(1).InfoS("Processing node", "node", klog.KObj(node))
		fieldSelectorString := "spec.nodeName=" + node.Name + ",status.phase=" + string(v1.PodFailed)

		pods, err := podutil.ListPodsOnANodeWithFieldSelector(
			ctx,
			client,
			node,
			fieldSelectorString,
			podutil.WithFilter(evictable.IsEvictable),
			podutil.WithNamespaces(strategyParams.IncludedNamespaces.UnsortedList()),
			podutil.WithoutNamespaces(strategyParams.ExcludedNamespaces.UnsortedList()),
			podutil.WithLabelSelector(labelSelector),
		)
		if err != nil {
			klog.ErrorS(err, "Error listing a nodes failed pods", "node", klog.KObj(node))
			continue
		}

		for i, pod := range pods {
			if err = validateFailedPodShouldEvict(pod, *strategyParams); err != nil {
				klog.V(4).InfoS(fmt.Sprintf("ignoring pod for eviction due to: %s", err.Error()), "pod", klog.KObj(pod))
				continue
			}

			if _, err = podEvictor.EvictPod(ctx, pods[i], node, "FailedPod"); err != nil {
				klog.ErrorS(err, "Error evicting pod", "pod", klog.KObj(pod))
				break
			}
		}
	}
}

func validateAndParseRemoveFailedPodsParams(
	ctx context.Context,
	client clientset.Interface,
	params *api.StrategyParameters,
) (*validatedFailedPodsStrategyParams, error) {
	if params == nil {
		return &validatedFailedPodsStrategyParams{
			ValidatedStrategyParams: validation.DefaultValidatedStrategyParams(),
		}, nil
	}

	strategyParams, err := validation.ValidateAndParseStrategyParams(ctx, client, params)
	if err != nil {
		return nil, err
	}

	var reasons, excludeOwnerKinds sets.String
	var includingInitContainers bool
	var minPodLifeTimeSeconds *uint
	if params.FailedPods != nil {
		reasons = sets.NewString(params.FailedPods.Reasons...)
		includingInitContainers = params.FailedPods.IncludingInitContainers
		excludeOwnerKinds = sets.NewString(params.FailedPods.ExcludeOwnerKinds...)
		minPodLifeTimeSeconds = params.FailedPods.MinPodLifeTimeSeconds
	}

	return &validatedFailedPodsStrategyParams{
		ValidatedStrategyParams: *strategyParams,
		includingInitContainers: includingInitContainers,
		reasons:                 reasons,
		excludeOwnerKinds:       excludeOwnerKinds,
		minPodLifeTimeSeconds:   minPodLifeTimeSeconds,
	}, nil
}

// validateFailedPodShouldEvict looks at strategy params settings to see if the Pod
// should be evicted given the params in the PodFailed policy.
func validateFailedPodShouldEvict(pod *v1.Pod, strategyParams validatedFailedPodsStrategyParams) error {
	var errs []error

	if strategyParams.minPodLifeTimeSeconds != nil {
		podAgeSeconds := uint(metav1.Now().Sub(pod.GetCreationTimestamp().Local()).Seconds())
		if podAgeSeconds < *strategyParams.minPodLifeTimeSeconds {
			errs = append(errs, fmt.Errorf("pod does not exceed the min age seconds of %d", *strategyParams.minPodLifeTimeSeconds))
		}
	}

	if len(strategyParams.excludeOwnerKinds) > 0 {
		ownerRefList := podutil.OwnerRef(pod)
		for _, owner := range ownerRefList {
			if strategyParams.excludeOwnerKinds.Has(owner.Kind) {
				errs = append(errs, fmt.Errorf("pod's owner kind of %s is excluded", owner.Kind))
			}
		}
	}

	if len(strategyParams.reasons) > 0 {
		reasons := getFailedContainerStatusReasons(pod.Status.ContainerStatuses)

		if strategyParams.includingInitContainers {
			reasons = append(reasons, getFailedContainerStatusReasons(pod.Status.InitContainerStatuses)...)
		}

		if !strategyParams.reasons.HasAny(reasons...) {
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
