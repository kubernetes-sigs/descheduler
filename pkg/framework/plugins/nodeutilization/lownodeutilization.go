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
	"sigs.k8s.io/descheduler/pkg/framework/plugins/nodeutilization/classifier"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/nodeutilization/normalizer"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
)

const LowNodeUtilizationPluginName = "LowNodeUtilization"

// this lines makes sure that HighNodeUtilization implements the BalancePlugin
// interface.
var _ frameworktypes.BalancePlugin = &LowNodeUtilization{}

// LowNodeUtilization evicts pods from overutilized nodes to underutilized
// nodes. Note that CPU/Memory requests are used to calculate nodes'
// utilization and not the actual resource usage.
type LowNodeUtilization struct {
	handle                frameworktypes.Handle
	args                  *LowNodeUtilizationArgs
	podFilter             func(pod *v1.Pod) bool
	underCriteria         []any
	overCriteria          []any
	resourceNames         []v1.ResourceName
	extendedResourceNames []v1.ResourceName
	usageClient           usageClient
}

// NewLowNodeUtilization builds plugin from its arguments while passing a
// handle. this plugin aims to move workload from overutilized nodes to
// underutilized nodes.
func NewLowNodeUtilization(
	genericArgs runtime.Object, handle frameworktypes.Handle,
) (frameworktypes.Plugin, error) {
	args, ok := genericArgs.(*LowNodeUtilizationArgs)
	if !ok {
		return nil, fmt.Errorf(
			"want args to be of type LowNodeUtilizationArgs, got %T",
			genericArgs,
		)
	}

	// resourceNames holds a list of resources for which the user has
	// provided thresholds for. extendedResourceNames holds those as well
	// as cpu, memory and pods if no prometheus collection is used.
	resourceNames := getResourceNames(args.Thresholds)
	extendedResourceNames := resourceNames

	// if we are using prometheus we need to validate we have everything we
	// need. if we aren't then we need to make sure we are also collecting
	// data for cpu, memory and pods.
	metrics := args.MetricsUtilization
	if metrics != nil && metrics.Source == api.PrometheusMetrics {
		if err := validatePrometheusMetricsUtilization(args); err != nil {
			return nil, err
		}
	} else {
		extendedResourceNames = uniquifyResourceNames(
			append(
				resourceNames,
				v1.ResourceCPU,
				v1.ResourceMemory,
				v1.ResourcePods,
			),
		)
	}

	// underCriteria and overCriteria are slices used for logging purposes.
	// we assemble them only once.
	underCriteria, overCriteria := []any{}, []any{}
	for name := range args.Thresholds {
		underCriteria = append(underCriteria, name, args.Thresholds[name])
	}
	for name := range args.TargetThresholds {
		overCriteria = append(overCriteria, name, args.TargetThresholds[name])
	}

	podFilter, err := podutil.
		NewOptions().
		WithFilter(handle.Evictor().Filter).
		BuildFilterFunc()
	if err != nil {
		return nil, fmt.Errorf("error initializing pod filter function: %v", err)
	}

	// this plugins supports different ways of collecting usage data. each
	// different way provides its own "usageClient". here we make sure we
	// have the correct one or an error is triggered. XXX MetricsServer is
	// deprecated, removed once dropped.
	var usageClient usageClient = newRequestedUsageClient(
		extendedResourceNames, handle.GetPodsAssignedToNodeFunc(),
	)
	if metrics != nil {
		usageClient, err = usageClientForMetrics(args, handle, extendedResourceNames)
		if err != nil {
			return nil, err
		}
	}

	return &LowNodeUtilization{
		handle:                handle,
		args:                  args,
		underCriteria:         underCriteria,
		overCriteria:          overCriteria,
		resourceNames:         resourceNames,
		extendedResourceNames: extendedResourceNames,
		podFilter:             podFilter,
		usageClient:           usageClient,
	}, nil
}

// Name retrieves the plugin name.
func (l *LowNodeUtilization) Name() string {
	return LowNodeUtilizationPluginName
}

// Balance holds the main logic of the plugin. It evicts pods from over
// utilized nodes to under utilized nodes. The goal here is to evenly
// distribute pods across nodes.
func (l *LowNodeUtilization) Balance(ctx context.Context, nodes []*v1.Node) *frameworktypes.Status {
	if err := l.usageClient.sync(ctx, nodes); err != nil {
		return &frameworktypes.Status{
			Err: fmt.Errorf("error getting node usage: %v", err),
		}
	}

	// starts by taking a snapshot ofthe nodes usage. we will use this
	// snapshot to assess the nodes usage and classify them as
	// underutilized or overutilized.
	nodesMap, nodesUsageMap, podListMap := getNodeUsageSnapshot(nodes, l.usageClient)
	capacities := referencedResourceListForNodesCapacity(nodes)

	// usage, by default, is exposed in absolute values. we need to normalize
	// them (convert them to percentages) to be able to compare them with the
	// user provided thresholds. thresholds are already provided in percentage
	// in the <0; 100> interval.
	var usage map[string]api.ResourceThresholds
	var thresholds map[string][]api.ResourceThresholds
	if l.args.UseDeviationThresholds {
		// here the thresholds provided by the user represent
		// deviations from the average so we need to treat them
		// differently. when calculating the average we only
		// need to consider the resources for which the user
		// has provided thresholds.
		usage, thresholds = assessNodesUsagesAndRelativeThresholds(
			filterResourceNames(nodesUsageMap, l.resourceNames),
			capacities,
			l.args.Thresholds,
			l.args.TargetThresholds,
		)
	} else {
		usage, thresholds = assessNodesUsagesAndStaticThresholds(
			nodesUsageMap,
			capacities,
			l.args.Thresholds,
			l.args.TargetThresholds,
		)
	}

	// classify nodes in under and over utilized. we will later try to move
	// pods from the overutilized nodes to the underutilized ones.
	nodeGroups := classifier.Classify(
		usage, thresholds,
		// underutilization criteria processing. nodes that are
		// underutilized but aren't schedulable are ignored.
		func(nodeName string, usage, threshold api.ResourceThresholds) bool {
			if nodeutil.IsNodeUnschedulable(nodesMap[nodeName]) {
				klog.V(2).InfoS(
					"Node is unschedulable, thus not considered as underutilized",
					"node", klog.KObj(nodesMap[nodeName]),
				)
				return false
			}
			return isNodeBelowThreshold(usage, threshold)
		},
		// overutilization criteria evaluation.
		func(nodeName string, usage, threshold api.ResourceThresholds) bool {
			return isNodeAboveThreshold(usage, threshold)
		},
	)

	// the nodeutilization package was designed to work with NodeInfo
	// structs. these structs holds information about how utilized a node
	// is. we need to go through the result of the classification and turn
	// it into NodeInfo structs.
	nodeInfos := make([][]NodeInfo, 2)
	categories := []string{"underutilized", "overutilized"}
	classifiedNodes := map[string]bool{}
	for i := range nodeGroups {
		for nodeName := range nodeGroups[i] {
			classifiedNodes[nodeName] = true

			klog.InfoS(
				"Node has been classified",
				"category", categories[i],
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
					l.extendedResourceNames,
				),
			})
		}
	}

	// log nodes that are appropriately utilized.
	for nodeName := range nodesMap {
		if !classifiedNodes[nodeName] {
			klog.InfoS(
				"Node is appropriately utilized",
				"node", klog.KObj(nodesMap[nodeName]),
				"usage", nodesUsageMap[nodeName],
				"usagePercentage", normalizer.Round(usage[nodeName]),
			)
		}
	}

	lowNodes, highNodes := nodeInfos[0], nodeInfos[1]

	// log messages for nodes with low and high utilization
	klog.V(1).InfoS("Criteria for a node under utilization", l.underCriteria...)
	klog.V(1).InfoS("Number of underutilized nodes", "totalNumber", len(lowNodes))
	klog.V(1).InfoS("Criteria for a node above target utilization", l.overCriteria...)
	klog.V(1).InfoS("Number of overutilized nodes", "totalNumber", len(highNodes))

	if len(lowNodes) == 0 {
		klog.V(1).InfoS(
			"No node is underutilized, nothing to do here, you might tune your thresholds further",
		)
		return nil
	}

	if len(lowNodes) <= l.args.NumberOfNodes {
		klog.V(1).InfoS(
			"Number of nodes underutilized is less or equal than NumberOfNodes, nothing to do here",
			"underutilizedNodes", len(lowNodes),
			"numberOfNodes", l.args.NumberOfNodes,
		)
		return nil
	}

	if len(lowNodes) == len(nodes) {
		klog.V(1).InfoS("All nodes are underutilized, nothing to do here")
		return nil
	}

	if len(highNodes) == 0 {
		klog.V(1).InfoS("All nodes are under target utilization, nothing to do here")
		return nil
	}

	// this is a stop condition for the eviction process. we stop as soon
	// as the node usage drops below the threshold.
	continueEvictionCond := func(nodeInfo NodeInfo, totalAvailableUsage api.ReferencedResourceList) bool {
		if !isNodeAboveTargetUtilization(nodeInfo.NodeUsage, nodeInfo.available) {
			return false
		}
		for name := range totalAvailableUsage {
			if totalAvailableUsage[name].CmpInt64(0) < 1 {
				return false
			}
		}

		return true
	}

	// sort the nodes by the usage in descending order
	sortNodesByUsage(highNodes, false)

	var nodeLimit *uint
	if l.args.EvictionLimits != nil {
		nodeLimit = l.args.EvictionLimits.Node
	}

	evictPodsFromSourceNodes(
		ctx,
		l.args.EvictableNamespaces,
		highNodes,
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

// validatePrometheusMetricsUtilization validates the Prometheus metrics
// utilization. XXX this should be done way earlier than this.
func validatePrometheusMetricsUtilization(args *LowNodeUtilizationArgs) error {
	if args.MetricsUtilization.Prometheus == nil {
		return fmt.Errorf("prometheus property is missing")
	}

	if args.MetricsUtilization.Prometheus.Query == "" {
		return fmt.Errorf("prometheus query is missing")
	}

	uResourceNames := getResourceNames(args.Thresholds)
	oResourceNames := getResourceNames(args.TargetThresholds)
	if len(uResourceNames) != 1 || uResourceNames[0] != MetricResource {
		return fmt.Errorf(
			"thresholds are expected to specify a single instance of %q resource, got %v instead",
			MetricResource, uResourceNames,
		)
	}

	if len(oResourceNames) != 1 || oResourceNames[0] != MetricResource {
		return fmt.Errorf(
			"targetThresholds are expected to specify a single instance of %q resource, got %v instead",
			MetricResource, oResourceNames,
		)
	}

	return nil
}

// usageClientForMetrics returns the correct usage client based on the
// metrics source. XXX MetricsServer is deprecated, removed once dropped.
func usageClientForMetrics(
	args *LowNodeUtilizationArgs, handle frameworktypes.Handle, resources []v1.ResourceName,
) (usageClient, error) {
	metrics := args.MetricsUtilization
	switch {
	case metrics.MetricsServer, metrics.Source == api.KubernetesMetrics:
		if handle.MetricsCollector() == nil {
			return nil, fmt.Errorf("metrics client not initialized")
		}
		return newActualUsageClient(
			resources,
			handle.GetPodsAssignedToNodeFunc(),
			handle.MetricsCollector(),
		), nil

	case metrics.Source == api.PrometheusMetrics:
		if handle.PrometheusClient() == nil {
			return nil, fmt.Errorf("prometheus client not initialized")
		}
		return newPrometheusUsageClient(
			handle.GetPodsAssignedToNodeFunc(),
			handle.PrometheusClient(),
			metrics.Prometheus.Query,
		), nil
	case metrics.Source != "":
		return nil, fmt.Errorf("unrecognized metrics source")
	default:
		return nil, fmt.Errorf("metrics source is empty")
	}
}
