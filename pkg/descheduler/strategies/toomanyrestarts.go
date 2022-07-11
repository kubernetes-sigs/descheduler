/*
Copyright 2018 The Kubernetes Authors.

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

package strategies

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
)

func validateRemovePodsHavingTooManyRestartsParams(params *api.StrategyParameters) error {
	if params == nil || params.PodsHavingTooManyRestarts == nil || params.PodsHavingTooManyRestarts.PodRestartThreshold < 1 {
		return fmt.Errorf("PodsHavingTooManyRestarts threshold not set")
	}

	// At most one of include/exclude can be set
	if params.Namespaces != nil && len(params.Namespaces.Include) > 0 && len(params.Namespaces.Exclude) > 0 {
		return fmt.Errorf("only one of Include/Exclude namespaces can be set")
	}
	if params.ThresholdPriority != nil && params.ThresholdPriorityClassName != "" {
		return fmt.Errorf("only one of thresholdPriority and thresholdPriorityClassName can be set")
	}

	return nil
}

// RemovePodsHavingTooManyRestarts removes the pods that have too many restarts on node.
// There are too many cases leading this issue: Volume mount failed, app error due to nodes' different settings.
// As of now, this strategy won't evict daemonsets, mirror pods, critical pods and pods with local storages.
func RemovePodsHavingTooManyRestarts(ctx context.Context, client clientset.Interface, strategy api.DeschedulerStrategy, nodes []*v1.Node, podEvictor *evictions.PodEvictor, evictorFilter *evictions.EvictorFilter, getPodsAssignedToNode podutil.GetPodsAssignedToNodeFunc) {
	if err := validateRemovePodsHavingTooManyRestartsParams(strategy.Params); err != nil {
		klog.ErrorS(err, "Invalid RemovePodsHavingTooManyRestarts parameters")
		return
	}

	var includedNamespaces, excludedNamespaces sets.String
	if strategy.Params.Namespaces != nil {
		includedNamespaces = sets.NewString(strategy.Params.Namespaces.Include...)
		excludedNamespaces = sets.NewString(strategy.Params.Namespaces.Exclude...)
	}

	podFilter, err := podutil.NewOptions().
		WithFilter(evictorFilter.Filter).
		WithNamespaces(includedNamespaces).
		WithoutNamespaces(excludedNamespaces).
		WithLabelSelector(strategy.Params.LabelSelector).
		BuildFilterFunc()
	if err != nil {
		klog.ErrorS(err, "Error initializing pod filter function")
		return
	}

loop:
	for _, node := range nodes {
		klog.V(1).InfoS("Processing node", "node", klog.KObj(node))
		pods, err := podutil.ListPodsOnANode(node.Name, getPodsAssignedToNode, podFilter)
		if err != nil {
			klog.ErrorS(err, "Error listing a nodes pods", "node", klog.KObj(node))
			continue
		}

		for i, pod := range pods {
			restarts, initRestarts := calcContainerRestarts(pod)
			if strategy.Params.PodsHavingTooManyRestarts.IncludingInitContainers {
				if restarts+initRestarts < strategy.Params.PodsHavingTooManyRestarts.PodRestartThreshold {
					continue
				}
			} else if restarts < strategy.Params.PodsHavingTooManyRestarts.PodRestartThreshold {
				continue
			}
			podEvictor.EvictPod(ctx, pods[i], evictions.EvictOptions{})
			if podEvictor.NodeLimitExceeded(node) {
				continue loop
			}
		}
	}
}

// calcContainerRestarts get container restarts and init container restarts.
func calcContainerRestarts(pod *v1.Pod) (int32, int32) {
	var restarts, initRestarts int32

	for _, cs := range pod.Status.ContainerStatuses {
		restarts += cs.RestartCount
	}

	for _, cs := range pod.Status.InitContainerStatuses {
		initRestarts += cs.RestartCount
	}

	return restarts, initRestarts
}
