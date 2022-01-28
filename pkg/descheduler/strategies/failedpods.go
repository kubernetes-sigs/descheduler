package strategies

import (
	"context"
	"errors"
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
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
	minPodLifetimeSeconds   *uint
}

type RemoveFailedPodsStrategy struct {
	strategy       api.DeschedulerStrategy
	client         clientset.Interface
	strategyParams *validatedFailedPodsStrategyParams
}

func NewRemoveFailedPodsStrategy(client clientset.Interface, strategyList api.StrategyList) (*RemoveFailedPodsStrategy, error) {
	s := &RemoveFailedPodsStrategy{
		client: client,
	}
	strategy, ok := strategyList[s.Name()]
	if !ok {
		return nil, errors.New("todo")
	}
	s.strategy = strategy

	return s, nil
}

func (s *RemoveFailedPodsStrategy) Name() api.StrategyName {
	return RemoveFailedPods
}

func (s *RemoveFailedPodsStrategy) Enabled() bool {
	return s.strategy.Enabled
}

func (s *RemoveFailedPodsStrategy) Validate() error {
	strategyParams, err := validateAndParseRemoveFailedPodsParams(context.TODO(), s.client, s.strategy.Params)
	if err != nil {
		klog.ErrorS(err, "Invalid RemoveFailedPods parameters")
		return err
	}
	s.strategyParams = strategyParams
	return nil
}

// Run removes Pods that are in failed status phase.
func (s *RemoveFailedPodsStrategy) Run(
	ctx context.Context,
	client clientset.Interface,
	strategy api.DeschedulerStrategy,
	nodes []*v1.Node,
	podEvictor *evictions.PodEvictor,
	getPodsAssignedToNode podutil.GetPodsAssignedToNodeFunc,
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

	podFilter, err := podutil.NewOptions().
		WithFilter(evictable.IsEvictable).
		WithNamespaces(strategyParams.IncludedNamespaces).
		WithoutNamespaces(strategyParams.ExcludedNamespaces).
		WithLabelSelector(labelSelector).
		BuildFilterFunc()
	if err != nil {
		klog.ErrorS(err, "Error initializing pod filter function")
		return
	}
	// Only list failed pods
	phaseFilter := func(pod *v1.Pod) bool { return pod.Status.Phase == v1.PodFailed }
	podFilter = podutil.WrapFilterFuncs(phaseFilter, podFilter)

	for _, node := range nodes {
		klog.V(1).InfoS("Processing node", "node", klog.KObj(node))
		pods, err := podutil.ListAllPodsOnANode(node.Name, getPodsAssignedToNode, podFilter)
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
	var minPodLifetimeSeconds *uint
	if params.FailedPods != nil {
		reasons = sets.NewString(params.FailedPods.Reasons...)
		includingInitContainers = params.FailedPods.IncludingInitContainers
		excludeOwnerKinds = sets.NewString(params.FailedPods.ExcludeOwnerKinds...)
		minPodLifetimeSeconds = params.FailedPods.MinPodLifetimeSeconds
	}

	return &validatedFailedPodsStrategyParams{
		ValidatedStrategyParams: *strategyParams,
		includingInitContainers: includingInitContainers,
		reasons:                 reasons,
		excludeOwnerKinds:       excludeOwnerKinds,
		minPodLifetimeSeconds:   minPodLifetimeSeconds,
	}, nil
}

// validateFailedPodShouldEvict looks at strategy params settings to see if the Pod
// should be evicted given the params in the PodFailed policy.
func validateFailedPodShouldEvict(pod *v1.Pod, strategyParams validatedFailedPodsStrategyParams) error {
	var errs []error

	if strategyParams.minPodLifetimeSeconds != nil {
		podAgeSeconds := uint(metav1.Now().Sub(pod.GetCreationTimestamp().Local()).Seconds())
		if podAgeSeconds < *strategyParams.minPodLifetimeSeconds {
			errs = append(errs, fmt.Errorf("pod does not exceed the min age seconds of %d", *strategyParams.minPodLifetimeSeconds))
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

		if pod.Status.Phase == v1.PodFailed && pod.Status.Reason != "" {
			reasons = append(reasons, pod.Status.Reason)
		}

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
