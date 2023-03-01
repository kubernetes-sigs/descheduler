package nodeutilization

import (
	"context"
	"fmt"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"sigs.k8s.io/descheduler/pkg/descheduler/cache"
	nodeutil "sigs.k8s.io/descheduler/pkg/descheduler/node"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/framework"
)

const RealLowNodeUtilizationPluginName = "RealLowNodeUtilization"

// LowNodeUtilization evicts pods from overutilized nodes to underutilized nodes. Note that CPU/Memory requests are used
// to calculate nodes' utilization and not the actual resource usage.

type RealLowNodeUtilization struct {
	handle    framework.Handle
	args      *LowNodeUtilizationArgs
	podFilter func(pod *v1.Pod) bool
}

var _ framework.BalancePlugin = &RealLowNodeUtilization{}

// NewLowNodeUtilization builds plugin from its arguments while passing a handle
func NewRealLowNodeUtilization(args runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	lowNodeUtilizationArgsArgs, ok := args.(*LowNodeUtilizationArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type LowNodeUtilizationArgs, got %T", args)
	}

	podFilter, err := podutil.NewOptions().
		WithFilter(handle.Evictor().Filter).
		BuildFilterFunc()
	if err != nil {
		return nil, fmt.Errorf("error initializing pod filter function: %v", err)
	}

	return &RealLowNodeUtilization{
		handle:    handle,
		args:      lowNodeUtilizationArgsArgs,
		podFilter: podFilter,
	}, nil
}

// Name retrieves the plugin name
func (l *RealLowNodeUtilization) Name() string {
	return LowNodeUtilizationPluginName
}

// Balance extension point implementation for the plugin
func (l *RealLowNodeUtilization) Balance(ctx context.Context, nodes []*v1.Node) *framework.Status {
	useDeviationThresholds := l.args.UseDeviationThresholds
	thresholds := l.args.Thresholds
	targetThresholds := l.args.TargetThresholds

	// check if Pods/CPU/Mem are set, if not, set them to 100
	if _, ok := thresholds[v1.ResourcePods]; ok {
		delete(thresholds, v1.ResourcePods)
		delete(targetThresholds, v1.ResourcePods)
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
	resourceNames := getResourceNames(thresholds)

	lowNodes, sourceNodes := classifyNodesWithReal(
		getNodeRealUsage(nodes),
		getNodeThresholds(nodes, thresholds, targetThresholds, resourceNames, l.handle.GetPodsAssignedToNodeFunc(), false),
		// The node has to be schedulable (to be able to move workload there)
		func(node *v1.Node, usage cache.NodeUsageMap, threshold NodeThresholds) bool {
			if nodeutil.IsNodeUnschedulable(node) {
				klog.V(2).InfoS("Node is unschedulable, thus not considered as underutilized", "node", klog.KObj(node))
				return false
			}
			return CheckNodeByWindowLatestCt(usage.UsageList, threshold.highResourceThreshold, IsNodeWithLowUtilization, cache.LATEST_REAL_METRICS_STORE)
		},
		func(node *v1.Node, usage cache.NodeUsageMap, threshold NodeThresholds) bool {
			return CheckNodeByWindowLatestCt(usage.UsageList, threshold.lowResourceThreshold, IsNodeAboveTargetUtilization, cache.LATEST_REAL_METRICS_STORE)
		},
	)

	// only cpu/mem
	underutilizationCriteria := []interface{}{
		"CPU", thresholds[v1.ResourceCPU],
		"Mem", thresholds[v1.ResourceMemory],
	}

	klog.V(1).InfoS("Criteria for a node under utilization", underutilizationCriteria...)
	klog.V(1).InfoS("Number of underutilized nodes", "totalNumber", len(lowNodes))

	// log message for over utilized nodes
	overutilizationCriteria := []interface{}{
		"CPU", targetThresholds[v1.ResourceCPU],
		"Mem", targetThresholds[v1.ResourceMemory],
	}

	klog.V(1).InfoS("Criteria for a node above target utilization", overutilizationCriteria...)
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
	continueEvictionCond := func(nodeInfo RealNodeInfo, totalAvailableUsage map[v1.ResourceName]*resource.Quantity) bool {
		if !IsNodeAboveTargetUtilization(nodeInfo.CurrentUsage, nodeInfo.threshold.highResourceThreshold) {
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
	//sortNodesByUsage(sourceNodes, false)

	evictPodsFromSourceNodesWithReal(
		ctx,
		l.args.EvictableNamespaces,
		sourceNodes,
		lowNodes,
		l.handle.Evictor(),
		l.podFilter,
		resourceNames,
		continueEvictionCond)

	return nil
}

func getNodeRealUsage(
	nodes []*v1.Node,
) []*cache.NodeUsageMap {
	usageCache := cache.GetCache()
	var nodeUsageList []*cache.NodeUsageMap
	nodeRealUsage := usageCache.GetReadyNodeUsage(&cache.QueryCacheOption{})
	for _, node := range nodes {
		if _, exists := nodeRealUsage[node.Name]; !exists {
			continue
		}
		nodeUsageList = append(nodeUsageList, nodeRealUsage[node.Name])
	}
	return nodeUsageList
}
