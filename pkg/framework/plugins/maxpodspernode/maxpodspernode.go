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

package maxpodspernode

import (
	"context"
	"fmt"
	"sort"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
)

const PluginName = "MaxPodsPerNode"

// MaxPodsPerNode evicts pods from nodes that exceed the configured maximum pod count
type MaxPodsPerNode struct {
	logger    klog.Logger
	handle    frameworktypes.Handle
	args      *MaxPodsPerNodeArgs
	podFilter podutil.FilterFunc
}

var _ frameworktypes.DeschedulePlugin = &MaxPodsPerNode{}

// New builds plugin from its arguments while passing a handle
func New(ctx context.Context, args runtime.Object, handle frameworktypes.Handle) (frameworktypes.Plugin, error) {
	maxPodsArgs, ok := args.(*MaxPodsPerNodeArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type MaxPodsPerNodeArgs, got %T", args)
	}
	logger := klog.FromContext(ctx).WithValues("plugin", PluginName)

	var includedNamespaces, excludedNamespaces sets.Set[string]
	if maxPodsArgs.Namespaces != nil {
		includedNamespaces = sets.New(maxPodsArgs.Namespaces.Include...)
		excludedNamespaces = sets.New(maxPodsArgs.Namespaces.Exclude...)
	}

	// We can combine Filter and PreEvictionFilter since for this strategy it does not matter where we run PreEvictionFilter
	podFilter, err := podutil.NewOptions().
		WithNamespaces(includedNamespaces).
		WithoutNamespaces(excludedNamespaces).
		WithLabelSelector(maxPodsArgs.LabelSelector).
		BuildFilterFunc()
	if err != nil {
		return nil, fmt.Errorf("error initializing pod filter function: %v", err)
	}

	return &MaxPodsPerNode{
		logger:    logger,
		handle:    handle,
		podFilter: podFilter,
		args:      maxPodsArgs,
	}, nil
}

// Name retrieves the plugin name
func (d *MaxPodsPerNode) Name() string {
	return PluginName
}

// Deschedule extension point implementation for the plugin
func (d *MaxPodsPerNode) Deschedule(ctx context.Context, nodes []*v1.Node) *frameworktypes.Status {
	logger := klog.FromContext(klog.NewContext(ctx, d.logger)).WithValues("ExtensionPoint", frameworktypes.DescheduleExtensionPoint)

	for _, node := range nodes {
		logger.V(2).Info("Processing node", "node", klog.KObj(node))

		pods, err := podutil.ListAllPodsOnANode(node.Name, d.handle.GetPodsAssignedToNodeFunc(), d.podFilter)
		if err != nil {
			return &frameworktypes.Status{
				Err: fmt.Errorf("error listing pods on a node: %v", err),
			}
		}

		podCount := len(pods)
		if podCount <= d.args.MaxPods {
			logger.V(2).Info("Node pod count within limit", "node", klog.KObj(node), "podCount", podCount, "maxPods", d.args.MaxPods)
			continue
		}

		excessPods := podCount - d.args.MaxPods
		logger.V(1).Info("Node exceeds max pods limit", "node", klog.KObj(node), "podCount", podCount, "maxPods", d.args.MaxPods, "excessPods", excessPods)

		// Sort pods by creation timestamp (newest first) so we evict the most recently created pods
		sort.Slice(pods, func(i, j int) bool {
			return pods[j].CreationTimestamp.Before(&pods[i].CreationTimestamp)
		})

		evicted := 0
	loop:
		for i := 0; i < len(pods) && evicted < excessPods; i++ {
			err := d.handle.Evictor().Evict(ctx, pods[i], evictions.EvictOptions{StrategyName: PluginName})
			if err == nil {
				evicted++
				continue
			}
			switch err.(type) {
			case *evictions.EvictionNodeLimitError:
				break loop
			case *evictions.EvictionTotalLimitError:
				return nil
			default:
				logger.Error(err, "eviction failed", "pod", klog.KObj(pods[i]))
			}
		}

		logger.V(1).Info("Evicted pods from node", "node", klog.KObj(node), "evictedCount", evicted)
	}
	return nil
}
