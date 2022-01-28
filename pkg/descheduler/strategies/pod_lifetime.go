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
	"errors"
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	clientset "k8s.io/client-go/kubernetes"
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

type PodLifeTimeStrategy struct {
	strategy api.DeschedulerStrategy
	client   clientset.Interface
}

func NewPodLifeTimeStrategy(client clientset.Interface, strategyList api.StrategyList) (*PodLifeTimeStrategy, error) {
	s := &PodLifeTimeStrategy{}
	strategy, ok := strategyList[s.Name()]
	if !ok {
		return nil, errors.New("")
	}
	s.strategy = strategy

	return s, nil
}

func (s *PodLifeTimeStrategy) Name() api.StrategyName {
	return PodLifeTime
}

func (s *PodLifeTimeStrategy) Enabled() bool {
	return s.strategy.Enabled
}

func (s *PodLifeTimeStrategy) Validate() error {
	err := validatePodLifeTimeParams(s.strategy.Params)
	if err != nil {
		klog.ErrorS(err, "Invalid PodLifeTime parameters")
		return err
	}
	return nil
}

// Run evicts pods on nodes that were created more than strategy.Params.MaxPodLifeTimeSeconds seconds ago.
func (s *PodLifeTimeStrategy) Run(ctx context.Context, client clientset.Interface, strategy api.DeschedulerStrategy, nodes []*v1.Node, podEvictor *evictions.PodEvictor, getPodsAssignedToNode podutil.GetPodsAssignedToNodeFunc) {
	if err := validatePodLifeTimeParams(strategy.Params); err != nil {
		klog.ErrorS(err, "Invalid PodLifeTime parameters")
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

	for _, node := range nodes {
		klog.V(1).InfoS("Processing node", "node", klog.KObj(node))

		pods := listOldPodsOnNode(node.Name, getPodsAssignedToNode, podFilter, *strategy.Params.PodLifeTime.MaxPodLifeTimeSeconds)
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
