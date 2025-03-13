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
	"sigs.k8s.io/descheduler/pkg/framework/plugins/nodeutilization/normalizer"
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
	extendedResourceNames    []v1.ResourceName
	usageClient              usageClient
}

var _ frameworktypes.BalancePlugin = &LowNodeUtilization{}

// NewLowNodeUtilization builds plugin from its arguments while passing a handle
func NewLowNodeUtilization(args runtime.Object, handle frameworktypes.Handle) (frameworktypes.Plugin, error) {
	lowNodeUtilizationArgsArgs, ok := args.(*LowNodeUtilizationArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type LowNodeUtilizationArgs, got %T", args)
	}

	resourceNames := getResourceNames(lowNodeUtilizationArgsArgs.Thresholds)
	extendedResourceNames := resourceNames

	metricsUtilization := lowNodeUtilizationArgsArgs.MetricsUtilization
	if metricsUtilization != nil && metricsUtilization.Source == api.PrometheusMetrics {
		if metricsUtilization.Prometheus != nil && metricsUtilization.Prometheus.Query != "" {
			uResourceNames := getResourceNames(lowNodeUtilizationArgsArgs.Thresholds)
			oResourceNames := getResourceNames(lowNodeUtilizationArgsArgs.TargetThresholds)
			if len(uResourceNames) != 1 || uResourceNames[0] != MetricResource {
				return nil, fmt.Errorf("thresholds are expected to specify a single instance of %q resource, got %v instead", MetricResource, uResourceNames)
			}
			if len(oResourceNames) != 1 || oResourceNames[0] != MetricResource {
				return nil, fmt.Errorf("targetThresholds are expected to specify a single instance of %q resource, got %v instead", MetricResource, oResourceNames)
			}
		} else {
			return nil, fmt.Errorf("prometheus query is missing")
		}
	} else {
		extendedResourceNames = uniquifyResourceNames(append(resourceNames, v1.ResourceCPU, v1.ResourceMemory, v1.ResourcePods))
	}

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

	var usageClient usageClient
	// MetricsServer is deprecated, removed once dropped
	if metricsUtilization != nil {
		switch {
		case metricsUtilization.MetricsServer, metricsUtilization.Source == api.KubernetesMetrics:
			if handle.MetricsCollector() == nil {
				return nil, fmt.Errorf("metrics client not initialized")
			}
			usageClient = newActualUsageClient(extendedResourceNames, handle.GetPodsAssignedToNodeFunc(), handle.MetricsCollector())
		case metricsUtilization.Source == api.PrometheusMetrics:
			if handle.PrometheusClient() == nil {
				return nil, fmt.Errorf("prometheus client not initialized")
			}
			usageClient = newPrometheusUsageClient(handle.GetPodsAssignedToNodeFunc(), handle.PrometheusClient(), metricsUtilization.Prometheus.Query)
		case metricsUtilization.Source != "":
			return nil, fmt.Errorf("unrecognized metrics source")
		default:
			return nil, fmt.Errorf("metrics source is empty")
		}
	} else {
		usageClient = newRequestedUsageClient(extendedResourceNames, handle.GetPodsAssignedToNodeFunc())
	}

	return &LowNodeUtilization{
		handle:                   handle,
		args:                     lowNodeUtilizationArgsArgs,
		underutilizationCriteria: underutilizationCriteria,
		overutilizationCriteria:  overutilizationCriteria,
		resourceNames:            resourceNames,
		extendedResourceNames:    extendedResourceNames,
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
	if err := l.usageClient.sync(ctx, nodes); err != nil {
		return &frameworktypes.Status{
			Err: fmt.Errorf("error getting node usage: %v", err),
		}
	}

	nodesMap, nodesUsageMap, podListMap := getNodeUsageSnapshot(nodes, l.usageClient)
	capacities := referencedResourceListForNodesCapacity(nodes)

	var usage map[string]api.ResourceThresholds
	var thresholds map[string][]api.ResourceThresholds
	if l.args.UseDeviationThresholds {
		usage, thresholds = assessNodesUsagesAndRelativeThresholds(
			filterResourceNamesFromNodeUsage(nodesUsageMap, l.resourceNames),
			capacities,
			l.args.Thresholds,
			l.args.TargetThresholds,
		)
	} else {
		usage, thresholds = assessNodesUsagesAndStaticThresholds(
			filterResourceNamesFromNodeUsage(nodesUsageMap, l.resourceNames),
			capacities,
			l.args.Thresholds,
			l.args.TargetThresholds,
		)
	}

	nodeGroups := classifyNodeUsage(
		usage,
		thresholds,
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
			klog.InfoS(
				fmt.Sprintf("Node is %s", category[i]),
				"node", klog.KObj(nodesMap[nodeName]),
				"usage", nodesUsageMap[nodeName],
				"usagePercentage", normalizer.Round(usage[nodeName]),
			)
			listedNodes[nodeName] = struct{}{}
			nodeInfos[i] = append(nodeInfos[i], NodeInfo{
				NodeUsage: NodeUsage{
					node:    nodesMap[nodeName],
					usage:   nodesUsageMap[nodeName], // get back the original node usage
					allPods: podListMap[nodeName],
				},
				thresholds: NodeThresholds{
					lowResourceThreshold:  resourceThresholdsToNodeUsage(thresholds[nodeName][0], capacities[nodeName], append(l.extendedResourceNames, v1.ResourceCPU, v1.ResourceMemory, v1.ResourcePods)),
					highResourceThreshold: resourceThresholdsToNodeUsage(thresholds[nodeName][1], capacities[nodeName], append(l.extendedResourceNames, v1.ResourceCPU, v1.ResourceMemory, v1.ResourcePods)),
				},
			})
		}
	}

	for nodeName := range nodesMap {
		if _, ok := listedNodes[nodeName]; !ok {
			klog.InfoS(
				"Node is appropriately utilized",
				"node", klog.KObj(nodesMap[nodeName]),
				"usage", nodesUsageMap[nodeName],
				"usagePercentage", normalizer.Round(usage[nodeName]),
			)
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
		l.extendedResourceNames,
		continueEvictionCond,
		l.usageClient,
		nodeLimit,
	)

	return nil
}
