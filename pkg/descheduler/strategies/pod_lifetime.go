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
	"k8s.io/apimachinery/pkg/util/sets"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
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
func PodLifeTime(ctx context.Context, client clientset.Interface, strategy api.DeschedulerStrategy, nodes []*v1.Node, podEvictor *evictions.PodEvictor, evictorFilter *evictions.EvictorFilter, getPodsAssignedToNode podutil.GetPodsAssignedToNodeFunc) {
	if err := validatePodLifeTimeParams(strategy.Params); err != nil {
		klog.ErrorS(err, "Invalid PodLifeTime parameters")
		return
	}

	var includedNamespaces, excludedNamespaces sets.String
	if strategy.Params.Namespaces != nil {
		includedNamespaces = sets.NewString(strategy.Params.Namespaces.Include...)
		excludedNamespaces = sets.NewString(strategy.Params.Namespaces.Exclude...)
	}

	filter := evictorFilter.Filter
	if strategy.Params.PodLifeTime.PodStatusPhases != nil {
		filter = func(pod *v1.Pod) bool {
			for _, phase := range strategy.Params.PodLifeTime.PodStatusPhases {
				if string(pod.Status.Phase) == phase {
					return evictorFilter.Filter(pod)
				}
			}
			return false
		}
	}

	podFilter, err := podutil.NewOptions().
		WithFilter(filter).
		WithNamespaces(includedNamespaces).
		WithoutNamespaces(excludedNamespaces).
		WithLabelSelector(strategy.Params.LabelSelector).
		BuildFilterFunc()
	if err != nil {
		klog.ErrorS(err, "Error initializing pod filter function")
		return
	}

	podsToEvict := make([]*v1.Pod, 0)
	nodeMap := make(map[string]*v1.Node, len(nodes))

	for _, node := range nodes {
		nodeMap[node.Name] = node
		klog.V(1).InfoS("Processing node", "node", klog.KObj(node))

		pods := listOldPodsOnNode(node.Name, getPodsAssignedToNode, podFilter, *strategy.Params.PodLifeTime.MaxPodLifeTimeSeconds)
		podsToEvict = append(podsToEvict, pods...)
	}

	// Should sort Pods so that the oldest can be evicted first
	// in the event that PDB or settings such maxNoOfPodsToEvictPer* prevent too much eviction
	podutil.SortPodsBasedOnAge(podsToEvict)

	for _, pod := range podsToEvict {
		success, err := podEvictor.EvictPod(ctx, pod, nodeMap[pod.Spec.NodeName], "PodLifeTime")
		if success {
			klog.V(1).InfoS("Evicted pod because it exceeded its lifetime", "pod", klog.KObj(pod), "maxPodLifeTime", *strategy.Params.PodLifeTime.MaxPodLifeTimeSeconds)
		}

		if err != nil {
			klog.ErrorS(err, "Error evicting pod", "pod", klog.KObj(pod))
			break
		}
	}
}

func listOldPodsOnNode(
	nodeName string,
	getPodsAssignedToNode podutil.GetPodsAssignedToNodeFunc,
	filter podutil.FilterFunc,
	maxPodLifeTimeSeconds uint,
) []*v1.Pod {
	pods, err := podutil.ListPodsOnANode(nodeName, getPodsAssignedToNode, filter)
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
