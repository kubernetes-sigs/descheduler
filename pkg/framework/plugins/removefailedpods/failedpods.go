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
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/pkg/apis/componentconfig"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/framework"
)

const PluginName = "RemoveFailedPods"

// RemoveFailedPods evicts pods on the node which violate NoSchedule Taints on nodes
type RemoveFailedPods struct {
	handle    framework.Handle
	args      *componentconfig.RemoveFailedPodsArgs
	podFilter podutil.FilterFunc
}

var _ framework.Plugin = &RemoveFailedPods{}
var _ framework.DeschedulePlugin = &RemoveFailedPods{}

// New builds plugin from its arguments while passing a handle
func New(args runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	failedPodsArgs, ok := args.(*componentconfig.RemoveFailedPodsArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type RemoveFailedPodsArgs, got %T", args)
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

	podFilter = podutil.WrapFilterFuncs(func(pod *v1.Pod) bool { return pod.Status.Phase == v1.PodFailed }, podFilter)

	if len(failedPodsArgs.ExcludeOwnerKinds) > 0 {
		excludeOwnerKinds := sets.NewString(failedPodsArgs.ExcludeOwnerKinds...)
		podFilter = podutil.WrapFilterFuncs(podFilter, func(pod *v1.Pod) bool {
			ownerRefList := podutil.OwnerRef(pod)
			for _, owner := range ownerRefList {
				if excludeOwnerKinds.Has(owner.Kind) {
					return false
				}
			}
			return true
		})
	}

	if failedPodsArgs.MinPodLifetimeSeconds != nil {
		minPodLifetimeSeconds := *failedPodsArgs.MinPodLifetimeSeconds
		podFilter = podutil.WrapFilterFuncs(podFilter, func(pod *v1.Pod) bool {
			podAgeSeconds := uint(metav1.Now().Sub(pod.GetCreationTimestamp().Local()).Seconds())
			return podAgeSeconds > minPodLifetimeSeconds
		})
	}

	if len(failedPodsArgs.Reasons) > 0 {
		failedPodsReasons := sets.NewString(failedPodsArgs.Reasons...)
		podFilter = podutil.WrapFilterFuncs(podFilter, func(pod *v1.Pod) bool {
			reasons := getFailedContainerStatusReasons(pod.Status.ContainerStatuses)

			if pod.Status.Phase == v1.PodFailed && pod.Status.Reason != "" {
				reasons = append(reasons, pod.Status.Reason)
			}

			if failedPodsArgs.IncludingInitContainers {
				reasons = append(reasons, getFailedContainerStatusReasons(pod.Status.InitContainerStatuses)...)
			}

			return failedPodsReasons.HasAny(reasons...)
		})
	}

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
func (d *RemoveFailedPods) Deschedule(ctx context.Context, nodes []*v1.Node) *framework.Status {
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
		}
	}
	return nil
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
