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

package removepodshavingtoomanyrestarts

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/pkg/apis/componentconfig"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/framework"
)

const PluginName = "RemovePodsHavingTooManyRestarts"

// RemovePodsHavingTooManyRestarts removes the pods that have too many restarts on node.
// There are too many cases leading this issue: Volume mount failed, app error due to nodes' different settings.
// As of now, this strategy won't evict daemonsets, mirror pods, critical pods and pods with local storages.
type RemovePodsHavingTooManyRestarts struct {
	handle    framework.Handle
	args      *componentconfig.RemovePodsHavingTooManyRestartsArgs
	podFilter podutil.FilterFunc
}

var _ framework.DeschedulePlugin = &RemovePodsHavingTooManyRestarts{}

// New builds plugin from its arguments while passing a handle
func New(args runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	tooManyRestartsArgs, ok := args.(*componentconfig.RemovePodsHavingTooManyRestartsArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type RemovePodsHavingTooManyRestartsArgs, got %T", args)
	}

	var includedNamespaces, excludedNamespaces sets.String
	if tooManyRestartsArgs.Namespaces != nil {
		includedNamespaces = sets.NewString(tooManyRestartsArgs.Namespaces.Include...)
		excludedNamespaces = sets.NewString(tooManyRestartsArgs.Namespaces.Exclude...)
	}

	// We can combine Filter and PreEvictionFilter since for this strategy it does not matter where we run PreEvictionFilter
	podFilter, err := podutil.NewOptions().
		WithFilter(podutil.WrapFilterFuncs(handle.Evictor().Filter, handle.Evictor().PreEvictionFilter)).
		WithNamespaces(includedNamespaces).
		WithoutNamespaces(excludedNamespaces).
		WithLabelSelector(tooManyRestartsArgs.LabelSelector).
		BuildFilterFunc()
	if err != nil {
		return nil, fmt.Errorf("error initializing pod filter function: %v", err)
	}

	podFilter = podutil.WrapFilterFuncs(podFilter, func(pod *v1.Pod) bool {
		if err := validateCanEvict(pod, tooManyRestartsArgs); err != nil {
			klog.V(4).InfoS(fmt.Sprintf("ignoring pod for eviction due to: %s", err.Error()), "pod", klog.KObj(pod))
			return false
		}
		return true
	})

	return &RemovePodsHavingTooManyRestarts{
		handle:    handle,
		args:      tooManyRestartsArgs,
		podFilter: podFilter,
	}, nil
}

// Name retrieves the plugin name
func (d *RemovePodsHavingTooManyRestarts) Name() string {
	return PluginName
}

// Deschedule extension point implementation for the plugin
func (d *RemovePodsHavingTooManyRestarts) Deschedule(ctx context.Context, nodes []*v1.Node) *framework.Status {
	for _, node := range nodes {
		klog.V(1).InfoS("Processing node", "node", klog.KObj(node))
		pods, err := podutil.ListAllPodsOnANode(node.Name, d.handle.GetPodsAssignedToNodeFunc(), d.podFilter)
		if err != nil {
			// no pods evicted as error encountered retrieving evictable Pods
			return &framework.Status{
				Err: fmt.Errorf("error listing pods on a node: %v", err),
			}
		}
		totalPods := len(pods)
		for i := 0; i < totalPods; i++ {
			d.handle.Evictor().Evict(ctx, pods[i], evictions.EvictOptions{})
			if d.handle.Evictor().NodeLimitExceeded(node) {
				break
			}
		}
	}
	return nil
}

// validateCanEvict looks at tooManyRestartsArgs to see if pod can be evicted given the args.
func validateCanEvict(pod *v1.Pod, tooManyRestartsArgs *componentconfig.RemovePodsHavingTooManyRestartsArgs) error {
	var err error

	restarts := calcContainerRestartsFromStatuses(pod.Status.ContainerStatuses)
	if tooManyRestartsArgs.IncludingInitContainers {
		restarts += calcContainerRestartsFromStatuses(pod.Status.InitContainerStatuses)
	}

	if restarts < tooManyRestartsArgs.PodRestartThreshold {
		err = fmt.Errorf("number of container restarts (%v) not exceeding the threshold", restarts)
	}

	return err
}

// calcContainerRestartsFromStatuses get container restarts from container statuses.
func calcContainerRestartsFromStatuses(statuses []v1.ContainerStatus) int32 {
	var restarts int32
	for _, cs := range statuses {
		restarts += cs.RestartCount
	}
	return restarts
}
