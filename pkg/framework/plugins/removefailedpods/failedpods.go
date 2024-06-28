/*
Copyright 2022 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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

	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
)

const PluginName = "RemoveFailedPods"

// RemoveFailedPods evicts pods in failed status phase that match the given args criteria
type RemoveFailedPods struct {
	handle    frameworktypes.Handle
	args      *RemoveFailedPodsArgs
	podFilter podutil.FilterFunc
}

var _ frameworktypes.DeschedulePlugin = &RemoveFailedPods{}

// New builds plugin from its arguments while passing a handle
func New(args runtime.Object, handle frameworktypes.Handle) (frameworktypes.Plugin, error) {
	failedPodsArgs, ok := args.(*RemoveFailedPodsArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type RemoveFailedPodsArgs, got %T", args)
	}

	var includedNamespaces, excludedNamespaces sets.Set[string]
	if failedPodsArgs.Namespaces != nil {
		includedNamespaces = sets.New(failedPodsArgs.Namespaces.Include...)
		excludedNamespaces = sets.New(failedPodsArgs.Namespaces.Exclude...)
	}

	// We can combine Filter and PreEvictionFilter since for this strategy it does not matter where we run PreEvictionFilter
	podFilter, err := podutil.NewOptions().
		WithFilter(podutil.WrapFilterFuncs(handle.Evictor().Filter, handle.Evictor().PreEvictionFilter)).
		WithNamespaces(includedNamespaces).
		WithoutNamespaces(excludedNamespaces).
		WithLabelSelector(failedPodsArgs.LabelSelector).
		BuildFilterFunc()
	if err != nil {
		return nil, fmt.Errorf("error initializing pod filter function: %v", err)
	}

	podFilter = podutil.WrapFilterFuncs(func(pod *v1.Pod) bool { return pod.Status.Phase == v1.PodFailed }, podFilter)

	podFilter = podutil.WrapFilterFuncs(podFilter, func(pod *v1.Pod) bool {
		if err := validateCanEvict(pod, failedPodsArgs); err != nil {
			klog.V(4).InfoS(fmt.Sprintf("ignoring pod for eviction due to: %s", err.Error()), "pod", klog.KObj(pod))
			return false
		}

		return true
	})

	return &RemoveFailedPods{
		handle:    handle,
		podFilter: podFilter,
		args:      failedPodsArgs,
	}, nil
}

// Name retrieves the plugin name
func (d *RemoveFailedPods) Name() string {
	return PluginName
}

// Deschedule extension point implementation for the plugin
func (d *RemoveFailedPods) Deschedule(ctx context.Context, nodes []*v1.Node) *frameworktypes.Status {
	for _, node := range nodes {
		klog.V(2).InfoS("Processing node", "node", klog.KObj(node))
		pods, err := podutil.ListAllPodsOnANode(node.Name, d.handle.GetPodsAssignedToNodeFunc(), d.podFilter)
		if err != nil {
			// no pods evicted as error encountered retrieving evictable Pods
			return &frameworktypes.Status{
				Err: fmt.Errorf("error listing pods on a node: %v", err),
			}
		}
		totalPods := len(pods)
	loop:
		for i := 0; i < totalPods; i++ {
			err := d.handle.Evictor().Evict(ctx, pods[i], evictions.EvictOptions{StrategyName: PluginName})
			if err == nil {
				continue
			}
			switch err.(type) {
			case *evictions.EvictionNodeLimitError:
				break loop
			case *evictions.EvictionTotalLimitError:
				return nil
			default:
				klog.Errorf("eviction failed: %v", err)
			}
		}
	}
	return nil
}

// validateCanEvict looks at failedPodArgs to see if pod can be evicted given the args.
func validateCanEvict(pod *v1.Pod, failedPodArgs *RemoveFailedPodsArgs) error {
	var errs []error

	if failedPodArgs.MinPodLifetimeSeconds != nil {
		podAgeSeconds := uint(metav1.Now().Sub(pod.GetCreationTimestamp().Local()).Seconds())
		if podAgeSeconds < *failedPodArgs.MinPodLifetimeSeconds {
			errs = append(errs, fmt.Errorf("pod does not exceed the min age seconds of %d", *failedPodArgs.MinPodLifetimeSeconds))
		}
	}

	if len(failedPodArgs.ExcludeOwnerKinds) > 0 {
		ownerRefList := podutil.OwnerRef(pod)
		for _, owner := range ownerRefList {
			if sets.New(failedPodArgs.ExcludeOwnerKinds...).Has(owner.Kind) {
				errs = append(errs, fmt.Errorf("pod's owner kind of %s is excluded", owner.Kind))
			}
		}
	}

	if len(failedPodArgs.Reasons) > 0 {
		reasons := getFailedContainerStatusReasons(pod.Status.ContainerStatuses)

		if pod.Status.Phase == v1.PodFailed && pod.Status.Reason != "" {
			reasons = append(reasons, pod.Status.Reason)
		}

		if failedPodArgs.IncludingInitContainers {
			reasons = append(reasons, getFailedContainerStatusReasons(pod.Status.InitContainerStatuses)...)
		}

		if !sets.New(failedPodArgs.Reasons...).HasAny(reasons...) {
			errs = append(errs, fmt.Errorf("pod does not match any of the reasons"))
		}
	}

	if len(failedPodArgs.ExitCodes) > 0 {
		exitCodes := getFailedContainerStatusExitCodes(pod.Status.ContainerStatuses)
		if failedPodArgs.IncludingInitContainers {
			exitCodes = append(exitCodes, getFailedContainerStatusExitCodes(pod.Status.InitContainerStatuses)...)
		}

		if !sets.New(failedPodArgs.ExitCodes...).HasAny(exitCodes...) {
			errs = append(errs, fmt.Errorf("pod does not match any of the exitCodes"))
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

func getFailedContainerStatusExitCodes(containerStatuses []v1.ContainerStatus) []int32 {
	exitCodes := make([]int32, 0)

	for _, containerStatus := range containerStatuses {
		if containerStatus.State.Terminated != nil {
			exitCodes = append(exitCodes, containerStatus.State.Terminated.ExitCode)
		}
	}

	return exitCodes
}
