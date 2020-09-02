/*
Copyright 2017 The Kubernetes Authors.

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
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	nodeutil "sigs.k8s.io/descheduler/pkg/descheduler/node"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/utils"
)

func validatePodsViolatingNodeAffinityParams(params *api.StrategyParameters) error {
	if params == nil || len(params.NodeAffinityType) == 0 {
		return fmt.Errorf("NodeAffinityType is empty")
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

// RemovePodsViolatingNodeAffinity evicts pods on nodes which violate node affinity
func RemovePodsViolatingNodeAffinity(ctx context.Context, client clientset.Interface, strategy api.DeschedulerStrategy, nodes []*v1.Node, podEvictor *evictions.PodEvictor) {
	if err := validatePodsViolatingNodeAffinityParams(strategy.Params); err != nil {
		klog.V(1).Info(err)
		return
	}
	thresholdPriority, err := utils.GetPriorityFromStrategyParams(ctx, client, strategy.Params)
	if err != nil {
		klog.V(1).InfoS("Failed to get threshold priority from strategy's params", "err", err)
		return
	}

	var includedNamespaces, excludedNamespaces []string
	if strategy.Params.Namespaces != nil {
		includedNamespaces = strategy.Params.Namespaces.Include
		excludedNamespaces = strategy.Params.Namespaces.Exclude
	}

	evictable := podEvictor.Evictable(evictions.WithPriorityThreshold(thresholdPriority))

	for _, nodeAffinity := range strategy.Params.NodeAffinityType {
		klog.V(2).InfoS("Executing for nodeAffinityType", "nodeAffinity", nodeAffinity)

		switch nodeAffinity {
		case "requiredDuringSchedulingIgnoredDuringExecution":
			for _, node := range nodes {
				klog.V(1).InfoS("Processing node", "node", klog.KObj(node))

				pods, err := podutil.ListPodsOnANode(
					ctx,
					client,
					node,
					podutil.WithFilter(func(pod *v1.Pod) bool {
						return evictable.IsEvictable(pod) &&
							!nodeutil.PodFitsCurrentNode(pod, node) &&
							nodeutil.PodFitsAnyNode(pod, nodes)
					}),
					podutil.WithNamespaces(includedNamespaces),
					podutil.WithoutNamespaces(excludedNamespaces),
				)
				if err != nil {
					klog.Errorf("failed to get pods from %v: %v", node.Name, err)
				}

				for _, pod := range pods {
					if pod.Spec.Affinity != nil && pod.Spec.Affinity.NodeAffinity != nil && pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
						klog.V(1).InfoS("Evicting pod", "pod", klog.KObj(pod))
						if _, err := podEvictor.EvictPod(ctx, pod, node, "NodeAffinity"); err != nil {
							klog.ErrorS(err, "Error evicting pod")
							break
						}
					}
				}
			}
		default:
			klog.Errorf("invalid nodeAffinityType: %v", nodeAffinity)
		}
	}
	klog.V(1).Infof("Evicted %v pods", podEvictor.TotalEvicted())
}
