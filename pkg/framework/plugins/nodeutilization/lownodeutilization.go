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

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	nodeutil "sigs.k8s.io/descheduler/pkg/descheduler/node"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
)

const LowNodeUtilizationPluginName = "LowNodeUtilization"

// LowNodeUtilization evicts pods from overutilized nodes to underutilized nodes. Note that CPU/Memory requests are used
// to calculate nodes' utilization and not the actual resource usage.

type LowNodeUtilization struct {
	handle                   frameworktypes.Handle
	args                     *LowNodeUtilizationArgs
	podFilter                func(pod *v1.Pod) bool
	underutilizationCriteria []interface{}
	overutilizationCriteria  []interface{}
	resourceNames            []v1.ResourceName
	usageClient              usageClient
}

var _ frameworktypes.BalancePlugin = &LowNodeUtilization{}

// NewLowNodeUtilization builds plugin from its arguments while passing a handle
func NewLowNodeUtilization(args runtime.Object, handle frameworktypes.Handle) (frameworktypes.Plugin, error) {
	lowNodeUtilizationArgsArgs, ok := args.(*LowNodeUtilizationArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type LowNodeUtilizationArgs, got %T", args)
	}

	setDefaultForLNUThresholds(lowNodeUtilizationArgsArgs.Thresholds, lowNodeUtilizationArgsArgs.TargetThresholds, lowNodeUtilizationArgsArgs.UseDeviationThresholds)

	underutilizationCriteria := []interface{}{
		"CPU", lowNodeUtilizationArgsArgs.Thresholds[v1.ResourceCPU],
		"Mem", lowNodeUtilizationArgsArgs.Thresholds[v1.ResourceMemory],
		"Pods", lowNodeUtilizationArgsArgs.Thresholds[v1.ResourcePods],
	}
	for name := range lowNodeUtilizationArgsArgs.Thresholds {
		if !nodeutil.IsBasicResource(name) {
			underutilizationCriteria = append(underutilizationCriteria, string(name), int64(lowNodeUtilizationArgsArgs.Thresholds[name]))
		}
	}

	overutilizationCriteria := []interface{}{
		"CPU", lowNodeUtilizationArgsArgs.TargetThresholds[v1.ResourceCPU],
		"Mem", lowNodeUtilizationArgsArgs.TargetThresholds[v1.ResourceMemory],
		"Pods", lowNodeUtilizationArgsArgs.TargetThresholds[v1.ResourcePods],
	}
	for name := range lowNodeUtilizationArgsArgs.TargetThresholds {
		if !nodeutil.IsBasicResource(name) {
			overutilizationCriteria = append(overutilizationCriteria, string(name), int64(lowNodeUtilizationArgsArgs.TargetThresholds[name]))
		}
	}

	podFilter, err := podutil.NewOptions().
		WithFilter(handle.Evictor().Filter).
		BuildFilterFunc()
	if err != nil {
		return nil, fmt.Errorf("error initializing pod filter function: %v", err)
	}

	resourceNames := getResourceNames(lowNodeUtilizationArgsArgs.Thresholds)

	var usageClient usageClient
	// MetricsServer is deprecated, removed once dropped
	if lowNodeUtilizationArgsArgs.MetricsUtilization != nil && (lowNodeUtilizationArgsArgs.MetricsUtilization.MetricsServer || lowNodeUtilizationArgsArgs.MetricsUtilization.Source == api.KubernetesMetrics) {
		if handle.MetricsCollector() == nil {
			return nil, fmt.Errorf("metrics client not initialized")
		}
		usageClient = newActualUsageClient(resourceNames, handle.GetPodsAssignedToNodeFunc(), handle.MetricsCollector())
	} else {
		usageClient = newRequestedUsageClient(resourceNames, handle.GetPodsAssignedToNodeFunc())
	}

	return &LowNodeUtilization{
		handle:                   handle,
		args:                     lowNodeUtilizationArgsArgs,
		underutilizationCriteria: underutilizationCriteria,
		overutilizationCriteria:  overutilizationCriteria,
		resourceNames:            resourceNames,
		podFilter:                podFilter,
		usageClient:              usageClient,
	}, nil
}

// Name retrieves the plugin name
func (l *LowNodeUtilization) Name() string {
	return LowNodeUtilizationPluginName
}

// Balance extension point implementation for the plugin
func (l *LowNodeUtilization) Balance(ctx context.Context, nodes []*v1.Node) *frameworktypes.Status {
	if err := l.usageClient.sync(nodes); err != nil {
		return &frameworktypes.Status{
			Err: fmt.Errorf("error getting node usage: %v", err),
		}
	}

	nodesMap, nodesUsageMap, podListMap := getNodeUsageSnapshot(nodes, l.usageClient)
	var nodeThresholdsMap map[string][]api.ResourceThresholds
	if l.args.UseDeviationThresholds {
		nodeThresholdsMap = getNodeThresholdsFromAverageNodeUsage(nodes, l.usageClient, l.args.Thresholds, l.args.TargetThresholds)
	} else {
		nodeThresholdsMap = getStaticNodeThresholds(nodes, l.args.Thresholds, l.args.TargetThresholds)
	}
	nodesUsageAsNodeThresholdsMap := nodeUsageToResourceThresholds(nodesUsageMap, nodesMap)

	nodeGroups := classifyNodeUsage(
		nodesUsageAsNodeThresholdsMap,
		nodeThresholdsMap,
		[]classifierFnc{
			// underutilization
			func(nodeName string, usage, threshold api.ResourceThresholds) bool {
				if nodeutil.IsNodeUnschedulable(nodesMap[nodeName]) {
					klog.V(2).InfoS("Node is unschedulable, thus not considered as underutilized", "node", klog.KObj(nodesMap[nodeName]))
					return false
				}
				return isNodeBelowThreshold(usage, threshold)
			},
			// overutilization
			func(nodeName string, usage, threshold api.ResourceThresholds) bool {
				return isNodeAboveThreshold(usage, threshold)
			},
		},
	)

	// convert groups node []NodeInfo
	nodeInfos := make([][]NodeInfo, 2)
	category := []string{"underutilized", "overutilized"}
	listedNodes := map[string]struct{}{}
	for i := range nodeGroups {
		for nodeName := range nodeGroups[i] {
			klog.InfoS("Node is "+category[i], "node", klog.KObj(nodesMap[nodeName]), "usage", nodesUsageMap[nodeName], "usagePercentage", resourceUsagePercentages(nodesUsageMap[nodeName], nodesMap[nodeName], true))
			listedNodes[nodeName] = struct{}{}
			nodeInfos[i] = append(nodeInfos[i], NodeInfo{
				NodeUsage: NodeUsage{
					node:    nodesMap[nodeName],
					usage:   nodesUsageMap[nodeName], // get back the original node usage
					allPods: podListMap[nodeName],
				},
				thresholds: NodeThresholds{
					lowResourceThreshold:  resourceThresholdsToNodeUsage(nodeThresholdsMap[nodeName][0], nodesMap[nodeName]),
					highResourceThreshold: resourceThresholdsToNodeUsage(nodeThresholdsMap[nodeName][1], nodesMap[nodeName]),
				},
			})
		}
	}
	for nodeName := range nodesMap {
		if _, ok := listedNodes[nodeName]; !ok {
			klog.InfoS("Node is appropriately utilized", "node", klog.KObj(nodesMap[nodeName]), "usage", nodesUsageMap[nodeName], "usagePercentage", resourceUsagePercentages(nodesUsageMap[nodeName], nodesMap[nodeName], true))
		}
	}

	lowNodes := nodeInfos[0]
	sourceNodes := nodeInfos[1]

	// log message for nodes with low utilization
	klog.V(1).InfoS("Criteria for a node under utilization", l.underutilizationCriteria...)
	klog.V(1).InfoS("Number of underutilized nodes", "totalNumber", len(lowNodes))

	// log message for over utilized nodes
	klog.V(1).InfoS("Criteria for a node above target utilization", l.overutilizationCriteria...)
	klog.V(1).InfoS("Number of overutilized nodes", "totalNumber", len(sourceNodes))

	if len(lowNodes) == 0 {
		klog.V(1).InfoS("No node is underutilized, nothing to do here, you might tune your thresholds further")
		return nil
	}

	if len(lowNodes) <= l.args.NumberOfNodes {
		klog.V(1).InfoS("Number of nodes underutilized is less or equal than NumberOfNodes, nothing to do here", "underutilizedNodes", len(lowNodes), "numberOfNodes", l.args.NumberOfNodes)
		return nil
	}

	if len(lowNodes) == len(nodes) {
		klog.V(1).InfoS("All nodes are underutilized, nothing to do here")
		return nil
	}

	if len(sourceNodes) == 0 {
		klog.V(1).InfoS("All nodes are under target utilization, nothing to do here")
		return nil
	}

	// stop if node utilization drops below target threshold or any of required capacity (cpu, memory, pods) is moved
	continueEvictionCond := func(nodeInfo NodeInfo, totalAvailableUsage api.ReferencedResourceList) bool {
		if !isNodeAboveTargetUtilization(nodeInfo.NodeUsage, nodeInfo.thresholds.highResourceThreshold) {
			return false
		}
		for name := range totalAvailableUsage {
			if totalAvailableUsage[name].CmpInt64(0) < 1 {
				return false
			}
		}

		return true
	}

	// Sort the nodes by the usage in descending order
	sortNodesByUsage(sourceNodes, false)

	var nodeLimit *uint
	if l.args.EvictionLimits != nil {
		nodeLimit = l.args.EvictionLimits.Node
	}

	evictPodsFromSourceNodes(
		ctx,
		l.args.EvictableNamespaces,
		sourceNodes,
		lowNodes,
		l.handle.Evictor(),
		evictions.EvictOptions{StrategyName: LowNodeUtilizationPluginName},
		l.podFilter,
		l.resourceNames,
		continueEvictionCond,
		l.usageClient,
		nodeLimit,
	)

	return nil
}

func setDefaultForLNUThresholds(thresholds, targetThresholds api.ResourceThresholds, useDeviationThresholds bool) {
	// check if Pods/CPU/Mem are set, if not, set them to 100
	if _, ok := thresholds[v1.ResourcePods]; !ok {
		if useDeviationThresholds {
			thresholds[v1.ResourcePods] = MinResourcePercentage
			targetThresholds[v1.ResourcePods] = MinResourcePercentage
		} else {
			thresholds[v1.ResourcePods] = MaxResourcePercentage
			targetThresholds[v1.ResourcePods] = MaxResourcePercentage
		}
	}
	if _, ok := thresholds[v1.ResourceCPU]; !ok {
		if useDeviationThresholds {
			thresholds[v1.ResourceCPU] = MinResourcePercentage
			targetThresholds[v1.ResourceCPU] = MinResourcePercentage
		} else {
			thresholds[v1.ResourceCPU] = MaxResourcePercentage
			targetThresholds[v1.ResourceCPU] = MaxResourcePercentage
		}
	}
	if _, ok := thresholds[v1.ResourceMemory]; !ok {
		if useDeviationThresholds {
			thresholds[v1.ResourceMemory] = MinResourcePercentage
			targetThresholds[v1.ResourceMemory] = MinResourcePercentage
		} else {
			thresholds[v1.ResourceMemory] = MaxResourcePercentage
			targetThresholds[v1.ResourceMemory] = MaxResourcePercentage
		}
	}
}
