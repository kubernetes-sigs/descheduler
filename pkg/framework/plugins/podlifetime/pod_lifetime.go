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

package podlifetime

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
)

const PluginName = "PodLifeTime"

var _ frameworktypes.DeschedulePlugin = &PodLifeTime{}

// PodLifeTime evicts pods matching configurable lifetime and status transition criteria.
type PodLifeTime struct {
	logger    klog.Logger
	handle    frameworktypes.Handle
	args      *PodLifeTimeArgs
	podFilter podutil.FilterFunc
}

// New builds plugin from its arguments while passing a handle.
func New(ctx context.Context, args runtime.Object, handle frameworktypes.Handle) (frameworktypes.Plugin, error) {
	podLifeTimeArgs, ok := args.(*PodLifeTimeArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type PodLifeTimeArgs, got %T", args)
	}

	logger := klog.FromContext(ctx).WithValues("plugin", PluginName)

	var includedNamespaces, excludedNamespaces sets.Set[string]
	if podLifeTimeArgs.Namespaces != nil {
		includedNamespaces = sets.New(podLifeTimeArgs.Namespaces.Include...)
		excludedNamespaces = sets.New(podLifeTimeArgs.Namespaces.Exclude...)
	}

	// We can combine Filter and PreEvictionFilter since for this strategy it does not matter where we run PreEvictionFilter
	podFilter, err := podutil.NewOptions().
		WithFilter(podutil.WrapFilterFuncs(handle.Evictor().Filter, handle.Evictor().PreEvictionFilter)).
		WithNamespaces(includedNamespaces).
		WithoutNamespaces(excludedNamespaces).
		WithLabelSelector(podLifeTimeArgs.LabelSelector).
		BuildFilterFunc()
	if err != nil {
		return nil, fmt.Errorf("error initializing pod filter function: %v", err)
	}

	if podLifeTimeArgs.MaxPodLifeTimeSeconds != nil {
		podFilter = podutil.WrapFilterFuncs(podFilter, func(pod *v1.Pod) bool {
			podAge := metav1.Now().Sub(pod.GetCreationTimestamp().Local())
			if podAge < 0 {
				return false
			}
			return uint(podAge.Seconds()) > *podLifeTimeArgs.MaxPodLifeTimeSeconds
		})
	}

	if len(podLifeTimeArgs.States) > 0 {
		states := sets.New(podLifeTimeArgs.States...)
		includeInit := podLifeTimeArgs.IncludingInitContainers
		includeEphemeral := podLifeTimeArgs.IncludingEphemeralContainers
		podFilter = podutil.WrapFilterFuncs(podFilter, func(pod *v1.Pod) bool {
			if states.Has(string(pod.Status.Phase)) {
				return true
			}
			if states.Has(pod.Status.Reason) {
				return true
			}
			if podutil.HasMatchingContainerWaitingState(pod.Status.ContainerStatuses, states) ||
				podutil.HasMatchingContainerTerminatedState(pod.Status.ContainerStatuses, states) {
				return true
			}
			if includeInit && (podutil.HasMatchingContainerWaitingState(pod.Status.InitContainerStatuses, states) ||
				podutil.HasMatchingContainerTerminatedState(pod.Status.InitContainerStatuses, states)) {
				return true
			}
			if includeEphemeral && (podutil.HasMatchingContainerWaitingState(pod.Status.EphemeralContainerStatuses, states) ||
				podutil.HasMatchingContainerTerminatedState(pod.Status.EphemeralContainerStatuses, states)) {
				return true
			}
			return false
		})
	}

	if podLifeTimeArgs.OwnerKinds != nil {
		if len(podLifeTimeArgs.OwnerKinds.Include) > 0 {
			includeKinds := sets.New(podLifeTimeArgs.OwnerKinds.Include...)
			podFilter = podutil.WrapFilterFuncs(podFilter, func(pod *v1.Pod) bool {
				for _, owner := range podutil.OwnerRef(pod) {
					if includeKinds.Has(owner.Kind) {
						return true
					}
				}
				return false
			})
		} else if len(podLifeTimeArgs.OwnerKinds.Exclude) > 0 {
			excludeKinds := sets.New(podLifeTimeArgs.OwnerKinds.Exclude...)
			podFilter = podutil.WrapFilterFuncs(podFilter, func(pod *v1.Pod) bool {
				for _, owner := range podutil.OwnerRef(pod) {
					if excludeKinds.Has(owner.Kind) {
						return false
					}
				}
				return true
			})
		}
	}

	if len(podLifeTimeArgs.Conditions) > 0 {
		podFilter = podutil.WrapFilterFuncs(podFilter, func(pod *v1.Pod) bool {
			return matchesAnyPodConditionFilter(pod, podLifeTimeArgs.Conditions)
		})
	}

	if len(podLifeTimeArgs.ExitCodes) > 0 {
		exitCodesSet := sets.New(podLifeTimeArgs.ExitCodes...)
		podFilter = podutil.WrapFilterFuncs(podFilter, func(pod *v1.Pod) bool {
			return matchesAnyExitCode(pod, exitCodesSet, podLifeTimeArgs.IncludingInitContainers)
		})
	}

	return &PodLifeTime{
		logger:    logger,
		handle:    handle,
		podFilter: podFilter,
		args:      podLifeTimeArgs,
	}, nil
}

// Name retrieves the plugin name
func (d *PodLifeTime) Name() string {
	return PluginName
}

// Deschedule extension point implementation for the plugin
func (d *PodLifeTime) Deschedule(ctx context.Context, nodes []*v1.Node) *frameworktypes.Status {
	podsToEvict := make([]*v1.Pod, 0)
	logger := klog.FromContext(klog.NewContext(ctx, d.logger)).WithValues("ExtensionPoint", frameworktypes.DescheduleExtensionPoint)
	for _, node := range nodes {
		logger.V(2).Info("Processing node", "node", klog.KObj(node))
		pods, err := podutil.ListAllPodsOnANode(node.Name, d.handle.GetPodsAssignedToNodeFunc(), d.podFilter)
		if err != nil {
			// no pods evicted as error encountered retrieving evictable Pods
			return &frameworktypes.Status{
				Err: fmt.Errorf("error listing pods on a node: %v", err),
			}
		}
		podsToEvict = append(podsToEvict, pods...)
	}

	// Should sort Pods so that the oldest can be evicted first
	// in the event that PDB or settings such maxNoOfPodsToEvictPer* prevent too much eviction
	podutil.SortPodsBasedOnAge(podsToEvict)

loop:
	for _, pod := range podsToEvict {
		err := d.handle.Evictor().Evict(ctx, pod, evictions.EvictOptions{StrategyName: PluginName})
		if err == nil {
			continue
		}
		switch err.(type) {
		case *evictions.EvictionNodeLimitError:
			continue loop
		case *evictions.EvictionTotalLimitError:
			return nil
		default:
			logger.Error(err, "eviction failed")
		}
	}

	return nil
}

// matchesAnyPodConditionFilter returns true if the pod has at least one
// condition satisfying any of the given filters (OR across filters).
func matchesAnyPodConditionFilter(pod *v1.Pod, filters []PodConditionFilter) bool {
	for _, f := range filters {
		for _, cond := range pod.Status.Conditions {
			if !matchesConditionFields(cond, f) {
				continue
			}
			if f.MinTimeSinceLastTransitionSeconds != nil {
				if cond.LastTransitionTime.IsZero() {
					continue
				}
				idle := metav1.Now().Sub(cond.LastTransitionTime.Time)
				if idle < 0 || uint(idle.Seconds()) < *f.MinTimeSinceLastTransitionSeconds {
					continue
				}
			}
			return true
		}
	}
	return false
}

// matchesConditionFields checks type, status, and reason fields of a single
// condition against a filter. Unset filter fields are not checked.
func matchesConditionFields(cond v1.PodCondition, filter PodConditionFilter) bool {
	if filter.Type != "" && string(cond.Type) != filter.Type {
		return false
	}
	if filter.Status != "" && string(cond.Status) != filter.Status {
		return false
	}
	if filter.Reason != "" && cond.Reason != filter.Reason {
		return false
	}
	// validation ensures that at least one of type, status, reason, or minTimeSinceLastTransitionSeconds is set
	return true
}

func matchesAnyExitCode(pod *v1.Pod, exitCodes sets.Set[int32], includeInit bool) bool {
	if hasMatchingExitCode(pod.Status.ContainerStatuses, exitCodes) {
		return true
	}
	if includeInit && hasMatchingExitCode(pod.Status.InitContainerStatuses, exitCodes) {
		return true
	}
	return false
}

func hasMatchingExitCode(statuses []v1.ContainerStatus, exitCodes sets.Set[int32]) bool {
	for _, cs := range statuses {
		if cs.State.Terminated != nil && exitCodes.Has(cs.State.Terminated.ExitCode) {
			return true
		}
	}
	return false
}
