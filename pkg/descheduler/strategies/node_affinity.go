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
	"k8s.io/apimachinery/pkg/util/sets"
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
func RemovePodsViolatingNodeAffinity(ctx context.Context, client clientset.Interface, strategy api.DeschedulerStrategy, nodes []*v1.Node, podEvictor *evictions.PodEvictor, getPodsAssignedToNode podutil.GetPodsAssignedToNodeFunc) {
	if err := validatePodsViolatingNodeAffinityParams(strategy.Params); err != nil {
		klog.ErrorS(err, "Invalid RemovePodsViolatingNodeAffinity parameters")
		return
	}
	thresholdPriority, err := utils.GetPriorityFromStrategyParams(ctx, client, strategy.Params)
	if err != nil {
		klog.ErrorS(err, "Failed to get threshold priority from strategy's params")
		return
	}

	var includedNamespaces, excludedNamespaces sets.String
	if strategy.Params.Namespaces != nil {
		includedNamespaces = sets.NewString(strategy.Params.Namespaces.Include...)
		excludedNamespaces = sets.NewString(strategy.Params.Namespaces.Exclude...)
	}

	nodeFit := false
	if strategy.Params != nil {
		nodeFit = strategy.Params.NodeFit
	}

	evictable := podEvictor.Evictable(evictions.WithPriorityThreshold(thresholdPriority), evictions.WithNodeFit(nodeFit))

	podFilter, err := podutil.NewOptions().
		WithNamespaces(includedNamespaces).
		WithoutNamespaces(excludedNamespaces).
		WithLabelSelector(strategy.Params.LabelSelector).
		BuildFilterFunc()
	if err != nil {
		klog.ErrorS(err, "Error initializing pod filter function")
		return
	}

	for _, nodeAffinity := range strategy.Params.NodeAffinityType {
		klog.V(2).InfoS("Executing for nodeAffinityType", "nodeAffinity", nodeAffinity)

		switch nodeAffinity {
		case "requiredDuringSchedulingIgnoredDuringExecution":
			for _, node := range nodes {
				klog.V(1).InfoS("Processing node", "node", klog.KObj(node))

				pods, err := podutil.ListPodsOnANode(
					node.Name,
					getPodsAssignedToNode,
					podutil.WrapFilterFuncs(podFilter, func(pod *v1.Pod) bool {
						return evictable.IsEvictable(pod) &&
							!nodeutil.PodFitsCurrentNode(getPodsAssignedToNode, pod, node) &&
							nodeutil.PodFitsAnyNode(getPodsAssignedToNode, pod, nodes)
					}),
				)
				if err != nil {
					klog.ErrorS(err, "Failed to get pods", "node", klog.KObj(node))
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
			klog.ErrorS(nil, "Invalid nodeAffinityType", "nodeAffinity", nodeAffinity)
		}
	}
}
