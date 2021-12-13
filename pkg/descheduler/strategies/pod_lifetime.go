/*
Copyright 2020 The Kubernetes Authors.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/utils"
)

func validatePodLifeTimeParams(params *api.StrategyParameters) error {
	if params == nil || params.PodLifeTime == nil || params.PodLifeTime.MaxPodLifeTimeSeconds == nil {
		return fmt.Errorf("MaxPodLifeTimeSeconds not set")
	}

	if params.PodLifeTime.PodStatusPhases != nil {
		for _, phase := range params.PodLifeTime.PodStatusPhases {
			if phase != string(v1.PodPending) && phase != string(v1.PodRunning) {
				return fmt.Errorf("only Pending and Running phases are supported in PodLifeTime")
			}
		}
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

// PodLifeTime evicts pods on nodes that were created more than strategy.Params.MaxPodLifeTimeSeconds seconds ago.
func PodLifeTime(ctx context.Context, client clientset.Interface, podLister listersv1.PodLister, strategy api.DeschedulerStrategy, nodes []*v1.Node, podEvictor *evictions.PodEvictor) {
	if err := validatePodLifeTimeParams(strategy.Params); err != nil {
		klog.ErrorS(err, "Invalid PodLifeTime parameters")
		return
	}

	thresholdPriority, err := utils.GetPriorityFromStrategyParams(ctx, client, strategy.Params)
	if err != nil {
		klog.ErrorS(err, "Failed to get threshold priority from strategy's params")
		return
	}

	var includedNamespaces, excludedNamespaces []string
	if strategy.Params.Namespaces != nil {
		includedNamespaces = strategy.Params.Namespaces.Include
		excludedNamespaces = strategy.Params.Namespaces.Exclude
	}

	evictable := podEvictor.Evictable(evictions.WithPriorityThreshold(thresholdPriority))

	filter := evictable.IsEvictable
	if strategy.Params.PodLifeTime.PodStatusPhases != nil {
		filter = func(pod *v1.Pod) bool {
			for _, phase := range strategy.Params.PodLifeTime.PodStatusPhases {
				if string(pod.Status.Phase) == phase {
					return evictable.IsEvictable(pod)
				}
			}
			return false
		}
	}

	for _, node := range nodes {
		klog.V(1).InfoS("Processing node", "node", klog.KObj(node))

		pods := listOldPodsOnNode(ctx, podLister, node, includedNamespaces, excludedNamespaces, strategy.Params.LabelSelector, *strategy.Params.PodLifeTime.MaxPodLifeTimeSeconds, filter)
		for _, pod := range pods {
			success, err := podEvictor.EvictPod(ctx, pod, node, "PodLifeTime")
			if success {
				klog.V(1).InfoS("Evicted pod because it exceeded its lifetime", "pod", klog.KObj(pod), "maxPodLifeTime", *strategy.Params.PodLifeTime.MaxPodLifeTimeSeconds)
			}

			if err != nil {
				klog.ErrorS(err, "Error evicting pod", "pod", klog.KObj(pod))
				break
			}
		}

	}
}

func listOldPodsOnNode(
	ctx context.Context,
	podLister listersv1.PodLister,
	node *v1.Node,
	includedNamespaces, excludedNamespaces []string,
	labelSelector *metav1.LabelSelector,
	maxPodLifeTimeSeconds uint,
	filter func(pod *v1.Pod) bool,
) []*v1.Pod {
	pods, err := podutil.ListPodsOnANode(
		ctx,
		podLister,
		node,
		podutil.WithFilter(filter),
		podutil.WithNamespaces(includedNamespaces),
		podutil.WithoutNamespaces(excludedNamespaces),
		podutil.WithLabelSelector(labelSelector),
	)
	if err != nil {
		return nil
	}

	var oldPods []*v1.Pod
	for _, pod := range pods {
		podAgeSeconds := uint(metav1.Now().Sub(pod.GetCreationTimestamp().Local()).Seconds())
		if podAgeSeconds > maxPodLifeTimeSeconds {
			oldPods = append(oldPods, pod)
		}
	}

	return oldPods
}
