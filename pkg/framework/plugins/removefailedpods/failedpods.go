package removefailedpods

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"

	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/framework"
)

const PluginName = "RemoveFailedPods"

// RemoveFailedPods removes Pods that are in failed status phase.
type RemoveFailedPods struct {
	handle            framework.Handle
	args              *framework.RemoveFailedPodsArgs
	reasons           sets.String
	excludeOwnerKinds sets.String
	podFilter         podutil.FilterFunc
}

var _ framework.Plugin = &RemoveFailedPods{}
var _ framework.DeschedulePlugin = &RemoveFailedPods{}

func New(args runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	failedPodsArgs, ok := args.(*framework.RemoveFailedPodsArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type RemoveFailedPodsArgs, got %T", args)
	}

	if err := framework.ValidateCommonArgs(failedPodsArgs.CommonArgs); err != nil {
		return nil, err
	}

	var includedNamespaces, excludedNamespaces sets.String
	if failedPodsArgs.Namespaces != nil {
		includedNamespaces = sets.NewString(failedPodsArgs.Namespaces.Include...)
		excludedNamespaces = sets.NewString(failedPodsArgs.Namespaces.Exclude...)
	}

	podFilter, err := podutil.NewOptions().
		WithFilter(handle.Evictor().Filter).
		WithNamespaces(includedNamespaces).
		WithoutNamespaces(excludedNamespaces).
		WithLabelSelector(failedPodsArgs.LabelSelector).
		BuildFilterFunc()
	if err != nil {
		return nil, fmt.Errorf("error initializing pod filter function: %v", err)
	}

	// Only list failed pods
	phaseFilter := func(pod *v1.Pod) bool { return pod.Status.Phase == v1.PodFailed }
	podFilter = podutil.WrapFilterFuncs(phaseFilter, podFilter)

	return &RemoveFailedPods{
		handle:            handle,
		args:              failedPodsArgs,
		excludeOwnerKinds: sets.NewString(failedPodsArgs.ExcludeOwnerKinds...),
		reasons:           sets.NewString(failedPodsArgs.Reasons...),
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

			d.handle.Evictor().Evict(ctx, pods[i])
		}
	}
	return nil
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
