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

package nodeutilization

import (
	"context"
	"fmt"
	"slices"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	nodeutil "sigs.k8s.io/descheduler/pkg/descheduler/node"

	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/nodeutilization/classifier"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/nodeutilization/normalizer"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
)

const HighNodeUtilizationPluginName = "HighNodeUtilization"

// this lines makes sure that HighNodeUtilization implements the BalancePlugin
// interface.
var _ frameworktypes.BalancePlugin = &HighNodeUtilization{}

// HighNodeUtilization evicts pods from under utilized nodes so that scheduler
// can schedule according to its plugin. Note that CPU/Memory requests are used
// to calculate nodes' utilization and not the actual resource usage.
type HighNodeUtilization struct {
	handle         frameworktypes.Handle
	args           *HighNodeUtilizationArgs
	podFilter      func(pod *v1.Pod) bool
	criteria       []any
	resourceNames  []v1.ResourceName
	highThresholds api.ResourceThresholds
	usageClient    usageClient
}

// NewHighNodeUtilization builds plugin from its arguments while passing a handle.
func NewHighNodeUtilization(
	genericArgs runtime.Object, handle frameworktypes.Handle,
) (frameworktypes.Plugin, error) {
	args, ok := genericArgs.(*HighNodeUtilizationArgs)
	if !ok {
		return nil, fmt.Errorf(
			"want args to be of type HighNodeUtilizationArgs, got %T",
			genericArgs,
		)
	}

	// this plugins worries only about thresholds but the nodeplugins
	// package was made to take two thresholds into account, one for low
	// and another for high usage. here we make sure we set the high
	// threshold to the maximum value for all resources for which we have a
	// threshold.
	highThresholds := make(api.ResourceThresholds)
	for rname := range args.Thresholds {
		highThresholds[rname] = MaxResourcePercentage
	}

	// get the resource names for which we have a threshold. this is
	// later used when determining if we are going to evict a pod.
	resourceThresholds := getResourceNames(args.Thresholds)

	// by default we evict pods from the under utilized nodes even if they
	// don't define a request for a given threshold. this works most of the
	// times and there is an use case for it. When using the restrict mode
	// we evaluate if the pod has a request for any of the resources the
	// user has provided as threshold.
	filters := []podutil.FilterFunc{handle.Evictor().Filter}
	if slices.Contains(args.EvictionModes, EvictionModeOnlyThresholdingResources) {
		filters = append(
			filters,
			withResourceRequestForAny(resourceThresholds...),
		)
	}

	podFilter, err := podutil.
		NewOptions().
		WithFilter(podutil.WrapFilterFuncs(filters...)).
		BuildFilterFunc()
	if err != nil {
		return nil, fmt.Errorf("error initializing pod filter function: %v", err)
	}

	// resourceNames is a list of all resource names this plugin cares
	// about. we care about the resources for which we have a threshold and
	// all we consider the basic resources (cpu, memory, pods).
	resourceNames := uniquifyResourceNames(
		append(
			resourceThresholds,
			v1.ResourceCPU,
			v1.ResourceMemory,
			v1.ResourcePods,
		),
	)

	return &HighNodeUtilization{
		handle:         handle,
		args:           args,
		resourceNames:  resourceNames,
		highThresholds: highThresholds,
		criteria:       thresholdsToKeysAndValues(args.Thresholds),
		podFilter:      podFilter,
		usageClient: newRequestedUsageClient(
			resourceNames,
			handle.GetPodsAssignedToNodeFunc(),
		),
	}, nil
}

// Name retrieves the plugin name.
func (h *HighNodeUtilization) Name() string {
	return HighNodeUtilizationPluginName
}

// Balance holds the main logic of the plugin. It evicts pods from under
// utilized nodes. The goal here is to concentrate pods in fewer nodes so that
// less nodes are used.
func (h *HighNodeUtilization) Balance(ctx context.Context, nodes []*v1.Node) *frameworktypes.Status {
	if err := h.usageClient.sync(ctx, nodes); err != nil {
		return &frameworktypes.Status{
			Err: fmt.Errorf("error getting node usage: %v", err),
		}
	}

	// take a picture of the current state of the nodes, everything else
	// here is based on this snapshot.
	nodesMap, nodesUsageMap, podListMap := getNodeUsageSnapshot(nodes, h.usageClient)
	capacities := referencedResourceListForNodesCapacity(nodes)

	// node usages are not presented as percentages over the capacity.
	// we need to normalize them to be able to compare them with the
	// thresholds. thresholds are already provided by the user in
	// percentage.
	usage, thresholds := assessNodesUsagesAndStaticThresholds(
		nodesUsageMap, capacities, h.args.Thresholds, h.highThresholds,
	)

	// classify nodes in two groups: underutilized and schedulable. we will
	// later try to move pods from the first group to the second.
	nodeGroups := classifier.Classify(
		usage, thresholds,
		// underutilized nodes.
		func(nodeName string, usage, threshold api.ResourceThresholds) bool {
			return isNodeBelowThreshold(usage, threshold)
		},
		// schedulable nodes.
		func(nodeName string, usage, threshold api.ResourceThresholds) bool {
			if nodeutil.IsNodeUnschedulable(nodesMap[nodeName]) {
				klog.V(2).InfoS(
					"Node is unschedulable",
					"node", klog.KObj(nodesMap[nodeName]),
				)
				return false
			}
			return true
		},
	)

	// the nodeplugin package works by means of NodeInfo structures. these
	// structures hold a series of information about the nodes. now that
	// we have classified the nodes, we can build the NodeInfo structures
	// for each group. NodeInfo structs carry usage and available resources
	// for each node.
	nodeInfos := make([][]NodeInfo, 2)
	category := []string{"underutilized", "overutilized"}
	for i := range nodeGroups {
		for nodeName := range nodeGroups[i] {
			klog.InfoS(
				"Node has been classified",
				"category", category[i],
				"node", klog.KObj(nodesMap[nodeName]),
				"usage", nodesUsageMap[nodeName],
				"usagePercentage", normalizer.Round(usage[nodeName]),
			)
			nodeInfos[i] = append(nodeInfos[i], NodeInfo{
				NodeUsage: NodeUsage{
					node:    nodesMap[nodeName],
					usage:   nodesUsageMap[nodeName],
					allPods: podListMap[nodeName],
				},
				available: capNodeCapacitiesToThreshold(
					nodesMap[nodeName],
					thresholds[nodeName][1],
					h.resourceNames,
				),
			})
		}
	}

	lowNodes, schedulableNodes := nodeInfos[0], nodeInfos[1]

	klog.V(1).InfoS("Criteria for a node below target utilization", h.criteria...)
	klog.V(1).InfoS("Number of underutilized nodes", "totalNumber", len(lowNodes))

	if len(lowNodes) == 0 {
		klog.V(1).InfoS(
			"No node is underutilized, nothing to do here, you might tune your thresholds further",
		)
		return nil
	}

	if len(lowNodes) <= h.args.NumberOfNodes {
		klog.V(1).InfoS(
			"Number of nodes underutilized is less or equal than NumberOfNodes, nothing to do here",
			"underutilizedNodes", len(lowNodes),
			"numberOfNodes", h.args.NumberOfNodes,
		)
		return nil
	}

	if len(lowNodes) == len(nodes) {
		klog.V(1).InfoS("All nodes are underutilized, nothing to do here")
		return nil
	}

	if len(schedulableNodes) == 0 {
		klog.V(1).InfoS("No node is available to schedule the pods, nothing to do here")
		return nil
	}

	// stops the eviction process if the total available capacity sage has
	// dropped to zero - no more pods can be scheduled. this will signalize
	// to stop if any of the available resources has dropped to zero.
	continueEvictionCond := func(_ NodeInfo, avail api.ReferencedResourceList) bool {
		for name := range avail {
			if avail[name].CmpInt64(0) < 1 {
				return false
			}
		}
		return true
	}

	// sorts the nodes by the usage in ascending order.
	sortNodesByUsage(lowNodes, true)

	evictPodsFromSourceNodes(
		ctx,
		h.args.EvictableNamespaces,
		lowNodes,
		schedulableNodes,
		h.handle.Evictor(),
		evictions.EvictOptions{StrategyName: HighNodeUtilizationPluginName},
		h.podFilter,
		h.resourceNames,
		continueEvictionCond,
		h.usageClient,
		nil,
	)

	return nil
}
