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
	v1meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/utils"
)

func validatePodLifeTimeParams(params *api.StrategyParameters) error {
	if params == nil || params.MaxPodLifeTimeSeconds == nil {
		return fmt.Errorf("MaxPodLifeTimeSeconds not set")
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
func PodLifeTime(ctx context.Context, client clientset.Interface, strategy api.DeschedulerStrategy, nodes []*v1.Node, podEvictor *evictions.PodEvictor) {
	if err := validatePodLifeTimeParams(strategy.Params); err != nil {
		klog.V(1).Info(err)
		return
	}

	thresholdPriority, err := utils.GetPriorityFromStrategyParams(ctx, client, strategy.Params)
	if err != nil {
		klog.V(1).Infof("failed to get threshold priority from strategy's params: %#v", err)
		return
	}

	var includedNamespaces, excludedNamespaces []string
	if strategy.Params.Namespaces != nil {
		includedNamespaces = strategy.Params.Namespaces.Include
		excludedNamespaces = strategy.Params.Namespaces.Exclude
	}

	evictable := podEvictor.Evictable(evictions.WithPriorityThreshold(thresholdPriority))

	for _, node := range nodes {
		klog.V(1).Infof("Processing node: %#v", node.Name)

		pods := listOldPodsOnNode(ctx, client, node, includedNamespaces, excludedNamespaces, *strategy.Params.MaxPodLifeTimeSeconds, evictable.IsEvictable)
		for _, pod := range pods {
			success, err := podEvictor.EvictPod(ctx, pod, node, "PodLifeTime")
			if success {
				klog.V(1).Infof("Evicted pod: %#v because it was created more than %v seconds ago", pod.Name, *strategy.Params.MaxPodLifeTimeSeconds)
			}

			if err != nil {
				klog.Errorf("Error evicting pod: (%#v)", err)
				break
			}
		}

	}
}

func listOldPodsOnNode(ctx context.Context, client clientset.Interface, node *v1.Node, includedNamespaces, excludedNamespaces []string, maxPodLifeTimeSeconds uint, filter func(pod *v1.Pod) bool) []*v1.Pod {
	pods, err := podutil.ListPodsOnANode(
		ctx,
		client,
		node,
		podutil.WithFilter(filter),
		podutil.WithNamespaces(includedNamespaces),
		podutil.WithoutNamespaces(excludedNamespaces),
	)
	if err != nil {
		return nil
	}

	var oldPods []*v1.Pod
	for _, pod := range pods {
		podAgeSeconds := uint(v1meta.Now().Sub(pod.GetCreationTimestamp().Local()).Seconds())
		if podAgeSeconds > maxPodLifeTimeSeconds {
			oldPods = append(oldPods, pod)
		}
	}

	return oldPods
}
