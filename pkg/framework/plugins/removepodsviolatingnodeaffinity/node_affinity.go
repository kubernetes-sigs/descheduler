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

package removepodsviolatingnodeaffinity

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"

	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	nodeutil "sigs.k8s.io/descheduler/pkg/descheduler/node"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
	"sigs.k8s.io/descheduler/pkg/utils"
)

const PluginName = "RemovePodsViolatingNodeAffinity"

// RemovePodsViolatingNodeAffinity evicts pods on the node which violate node affinity
type RemovePodsViolatingNodeAffinity struct {
	handle    frameworktypes.Handle
	args      *RemovePodsViolatingNodeAffinityArgs
	podFilter podutil.FilterFunc
}

var _ frameworktypes.DeschedulePlugin = &RemovePodsViolatingNodeAffinity{}

// New builds plugin from its arguments while passing a handle
func New(args runtime.Object, handle frameworktypes.Handle) (frameworktypes.Plugin, error) {
	nodeAffinityArgs, ok := args.(*RemovePodsViolatingNodeAffinityArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type RemovePodsViolatingNodeAffinityArgs, got %T", args)
	}

	var includedNamespaces, excludedNamespaces sets.Set[string]
	if nodeAffinityArgs.Namespaces != nil {
		includedNamespaces = sets.New(nodeAffinityArgs.Namespaces.Include...)
		excludedNamespaces = sets.New(nodeAffinityArgs.Namespaces.Exclude...)
	}

	// We can combine Filter and PreEvictionFilter since for this strategy it does not matter where we run PreEvictionFilter
	podFilter, err := podutil.NewOptions().
		WithFilter(podutil.WrapFilterFuncs(handle.Evictor().Filter, handle.Evictor().PreEvictionFilter)).
		WithNamespaces(includedNamespaces).
		WithoutNamespaces(excludedNamespaces).
		WithLabelSelector(nodeAffinityArgs.LabelSelector).
		BuildFilterFunc()
	if err != nil {
		return nil, fmt.Errorf("error initializing pod filter function: %v", err)
	}

	return &RemovePodsViolatingNodeAffinity{
		handle:    handle,
		podFilter: podFilter,
		args:      nodeAffinityArgs,
	}, nil
}

// Name retrieves the plugin name
func (d *RemovePodsViolatingNodeAffinity) Name() string {
	return PluginName
}

func (d *RemovePodsViolatingNodeAffinity) Deschedule(ctx context.Context, nodes []*v1.Node) *frameworktypes.Status {
	for _, nodeAffinity := range d.args.NodeAffinityType {
		klog.V(2).InfoS("Executing for nodeAffinityType", "nodeAffinity", nodeAffinity)
		var err *frameworktypes.Status = nil

		// The pods that we'll evict must be evictable. For example, the current number of replicas
		// must be greater than the pdb.minValue.
		// The pods must be able to get scheduled on a different node. Otherwise, it doesn't make much
		// sense to evict them.
		switch nodeAffinity {
		case "requiredDuringSchedulingIgnoredDuringExecution":
			// In this specific case, the pod must also violate the nodeSelector to be evicted
			filterFunc := func(pod *v1.Pod, node *v1.Node, nodes []*v1.Node) bool {
				return utils.PodHasNodeAffinity(pod, utils.RequiredDuringSchedulingIgnoredDuringExecution) &&
					d.handle.Evictor().Filter(pod) &&
					nodeutil.PodFitsAnyNode(d.handle.GetPodsAssignedToNodeFunc(), pod, nodes) &&
					!nodeutil.PodMatchNodeSelector(pod, node)
			}
			err = d.processNodes(ctx, nodes, filterFunc)
		case "preferredDuringSchedulingIgnoredDuringExecution":
			// In this specific case, the pod must have a better fit on another node than
			// in the current one based on the preferred node affinity
			filterFunc := func(pod *v1.Pod, node *v1.Node, nodes []*v1.Node) bool {
				return utils.PodHasNodeAffinity(pod, utils.PreferredDuringSchedulingIgnoredDuringExecution) &&
					d.handle.Evictor().Filter(pod) &&
					nodeutil.PodFitsAnyNode(d.handle.GetPodsAssignedToNodeFunc(), pod, nodes) &&
					(nodeutil.GetBestNodeWeightGivenPodPreferredAffinity(pod, nodes) > nodeutil.GetNodeWeightGivenPodPreferredAffinity(pod, node))
			}
			err = d.processNodes(ctx, nodes, filterFunc)
		default:
			klog.ErrorS(nil, "Invalid nodeAffinityType", "nodeAffinity", nodeAffinity)
		}

		if err != nil {
			return err
		}
	}
	return nil
}

func (d *RemovePodsViolatingNodeAffinity) processNodes(ctx context.Context, nodes []*v1.Node, filterFunc func(*v1.Pod, *v1.Node, []*v1.Node) bool) *frameworktypes.Status {
	for _, node := range nodes {
		klog.V(2).InfoS("Processing node", "node", klog.KObj(node))

		// Potentially evictable pods
		pods, err := podutil.ListPodsOnANode(
			node.Name,
			d.handle.GetPodsAssignedToNodeFunc(),
			podutil.WrapFilterFuncs(d.podFilter, func(pod *v1.Pod) bool {
				return filterFunc(pod, node, nodes)
			}),
		)
		if err != nil {
			return &frameworktypes.Status{
				Err: fmt.Errorf("error listing pods on a node: %v", err),
			}
		}

	loop:
		for _, pod := range pods {
			klog.V(1).InfoS("Evicting pod", "pod", klog.KObj(pod))
			err := d.handle.Evictor().Evict(ctx, pod, evictions.EvictOptions{StrategyName: PluginName})
			if err == nil {
				continue
			}
			switch err.(type) {
			case *evictions.EvictionNodeLimitError:
				break loop
			case *evictions.EvictionTotalLimitError:
				return nil
			default:
				klog.Errorf("eviction failed: %v", err)
			}
		}
	}
	return nil
}
