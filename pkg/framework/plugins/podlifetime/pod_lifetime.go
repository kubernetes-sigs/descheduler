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

	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"

	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
)

const PluginName = "PodLifeTime"

var _ frameworktypes.DeschedulePlugin = &PodLifeTime{}

// PodLifeTime evicts pods on the node that violate the max pod lifetime threshold
type PodLifeTime struct {
	handle    frameworktypes.Handle
	args      *PodLifeTimeArgs
	podFilter podutil.FilterFunc
}

// New builds plugin from its arguments while passing a handle
func New(_ context.Context, args runtime.Object, handle frameworktypes.Handle) (frameworktypes.Plugin, error) {
	podLifeTimeArgs, ok := args.(*PodLifeTimeArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type PodLifeTimeArgs, got %T", args)
	}

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

	podFilter = podutil.WrapFilterFuncs(podFilter, func(_ context.Context, pod *v1.Pod) bool {
		podAgeSeconds := int(metav1.Now().Sub(pod.GetCreationTimestamp().Local()).Seconds())
		return podAgeSeconds > int(*podLifeTimeArgs.MaxPodLifeTimeSeconds)
	})

	if len(podLifeTimeArgs.States) > 0 {
		states := sets.New(podLifeTimeArgs.States...)
		podFilter = podutil.WrapFilterFuncs(podFilter, func(_ context.Context, pod *v1.Pod) bool {
			// Pod Status Phase
			if states.Has(string(pod.Status.Phase)) {
				return true
			}

			// Pod Status Reason
			if states.Has(pod.Status.Reason) {
				return true
			}

			// Init Container Status Reason
			if podLifeTimeArgs.IncludingInitContainers {
				for _, containerStatus := range pod.Status.InitContainerStatuses {
					if containerStatus.State.Waiting != nil && states.Has(containerStatus.State.Waiting.Reason) {
						return true
					}
				}
			}

			// Ephemeral Container Status Reason
			if podLifeTimeArgs.IncludingEphemeralContainers {
				for _, containerStatus := range pod.Status.EphemeralContainerStatuses {
					if containerStatus.State.Waiting != nil && states.Has(containerStatus.State.Waiting.Reason) {
						return true
					}
				}
			}

			// Container Status Reason
			for _, containerStatus := range pod.Status.ContainerStatuses {
				if containerStatus.State.Waiting != nil && states.Has(containerStatus.State.Waiting.Reason) {
					return true
				}
			}

			return false
		})
	}

	return &PodLifeTime{
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
	logger := klog.FromContext(ctx)
	podsToEvict := make([]*v1.Pod, 0)
	nodeMap := make(map[string]*v1.Node, len(nodes))

	for _, node := range nodes {
		logger.V(2).Info("Processing node", "node", klog.KObj(node))
		pods, err := podutil.ListAllPodsOnANode(ctx, node.Name, d.handle.GetPodsAssignedToNodeFunc(), d.podFilter)
		if err != nil {
			// no pods evicted as error encountered retrieving evictable Pods
			return &frameworktypes.Status{
				Err: fmt.Errorf("error listing pods on a node: %v", err),
			}
		}

		nodeMap[node.Name] = node
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
			logger.Error(err, "unable to evict pod", "pod", klog.KObj(pod))
		}
	}

	return nil
}
