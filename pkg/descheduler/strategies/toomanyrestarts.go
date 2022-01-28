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
	"errors"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/utils"
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

type RemovePodsHavingTooManyRestartsStrategy struct {
	strategy api.DeschedulerStrategy
	client   clientset.Interface
}

func NewRemovePodsHavingTooManyRestartsStrategy(client clientset.Interface, strategyList api.StrategyList) (*RemovePodsHavingTooManyRestartsStrategy, error) {
	s := &RemovePodsHavingTooManyRestartsStrategy{}
	strategy, ok := strategyList[s.Name()]
	if !ok {
		return nil, errors.New("")
	}
	s.strategy = strategy

	return s, nil
}

func (s *RemovePodsHavingTooManyRestartsStrategy) Name() api.StrategyName {
	return RemovePodsHavingTooManyRestarts
}

func (s *RemovePodsHavingTooManyRestartsStrategy) Enabled() bool {
	return s.strategy.Enabled
}

func (s *RemovePodsHavingTooManyRestartsStrategy) Validate() error {
	err := validateRemovePodsHavingTooManyRestartsParams(s.strategy.Params)
	if err != nil {
		klog.ErrorS(err, "Invalid RemovePodsHavingTooManyRestarts parameters")
		return err
	}
	return nil
}

// RemovePodsHavingTooManyRestarts removes the pods that have too many restarts on node.
// There are too many cases leading this issue: Volume mount failed, app error due to nodes' different settings.
// As of now, this strategy won't evict daemonsets, mirror pods, critical pods and pods with local storages.
func (s *RemovePodsHavingTooManyRestartsStrategy) Run(ctx context.Context, nodes []*v1.Node, podEvictor *evictions.PodEvictor, getPodsAssignedToNode podutil.GetPodsAssignedToNodeFunc) {
	if err := validateRemovePodsHavingTooManyRestartsParams(s.strategy.Params); err != nil {
		klog.ErrorS(err, "Invalid RemovePodsHavingTooManyRestarts parameters")
		return
	}

	thresholdPriority, err := utils.GetPriorityFromStrategyParams(ctx, s.client, s.strategy.Params)
	if err != nil {
		klog.ErrorS(err, "Failed to get threshold priority from strategy's params")
		return
	}

	var includedNamespaces, excludedNamespaces sets.String
	if s.strategy.Params.Namespaces != nil {
		includedNamespaces = sets.NewString(s.strategy.Params.Namespaces.Include...)
		excludedNamespaces = sets.NewString(s.strategy.Params.Namespaces.Exclude...)
	}

	nodeFit := false
	if s.strategy.Params != nil {
		nodeFit = s.strategy.Params.NodeFit
	}

	evictable := podEvictor.Evictable(evictions.WithPriorityThreshold(thresholdPriority), evictions.WithNodeFit(nodeFit))

	podFilter, err := podutil.NewOptions().
		WithFilter(evictable.IsEvictable).
		WithNamespaces(includedNamespaces).
		WithoutNamespaces(excludedNamespaces).
		WithLabelSelector(s.strategy.Params.LabelSelector).
		BuildFilterFunc()
	if err != nil {
		klog.ErrorS(err, "Error initializing pod filter function")
		return
	}

	for _, node := range nodes {
		klog.V(1).InfoS("Processing node", "node", klog.KObj(node))
		pods, err := podutil.ListPodsOnANode(node.Name, getPodsAssignedToNode, podFilter)
		if err != nil {
			klog.ErrorS(err, "Error listing a nodes pods", "node", klog.KObj(node))
			continue
		}

		for i, pod := range pods {
			restarts, initRestarts := calcContainerRestarts(pod)
			if s.strategy.Params.PodsHavingTooManyRestarts.IncludingInitContainers {
				if restarts+initRestarts < s.strategy.Params.PodsHavingTooManyRestarts.PodRestartThreshold {
					continue
				}
			} else if restarts < s.strategy.Params.PodsHavingTooManyRestarts.PodRestartThreshold {
				continue
			}
			if _, err := podEvictor.EvictPod(ctx, pods[i], node, "TooManyRestarts"); err != nil {
				klog.ErrorS(err, "Error evicting pod", "pod", klog.KObj(pod))
				break
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
