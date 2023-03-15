package realutilization

import (
	"context"
	"fmt"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/cache"
	nodeutil "sigs.k8s.io/descheduler/pkg/descheduler/node"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/framework"
)

// LowNodeUtilization evicts pods from overutilized nodes to underutilized nodes. Note that CPU/Memory requests are used
// to calculate nodes' utilization and not the actual resource usage.

type RealLowNodeUtilization struct {
	handle    framework.Handle
	args      *LowNodeRealUtilizationArgs
	podFilter func(pod *v1.Pod) bool
}

var _ framework.BalancePlugin = &RealLowNodeUtilization{}

// NewRealLowNodeUtilization builds plugin from its arguments while passing a handle
func NewRealLowNodeUtilization(args runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	lowNodeRealUtilizationArgsArgs, ok := args.(*LowNodeRealUtilizationArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type LowNodeRealUtilizationArgsArgs, got %T", args)
	}

	podFilter, err := podutil.NewOptions().
		WithFilter(handle.Evictor().Filter).
		BuildFilterFunc()
	if err != nil {
		return nil, fmt.Errorf("error initializing pod filter function: %v", err)
	}

	return &RealLowNodeUtilization{
		handle:    handle,
		args:      lowNodeRealUtilizationArgsArgs,
		podFilter: podFilter,
	}, nil
}

const LowNodeRealUtilizationPluginName = "LowNodeRealUtilization"

// Name retrieves the plugin name
func (l *RealLowNodeUtilization) Name() string {
	return LowNodeRealUtilizationPluginName
}

// getResourceNames returns list of resource names in resource thresholds
func getResourceNames(thresholds api.ResourceThresholds) []v1.ResourceName {
	resourceNames := make([]v1.ResourceName, 0, len(thresholds))
	for name := range thresholds {
		resourceNames = append(resourceNames, name)
	}
	return resourceNames
}

const (
	// MinResourcePercentage is the minimum value of a resource's percentage
	MinResourcePercentage = 0
	// MaxResourcePercentage is the maximum value of a resource's percentage
	MaxResourcePercentage = 100
)

// Balance extension point implementation for the plugin
func (l *RealLowNodeUtilization) Balance(ctx context.Context, nodes []*v1.Node) *framework.Status {
	thresholds := l.args.Thresholds
	targetThresholds := l.args.TargetThresholds

	// check if Pods/CPU/Mem are set, if not, set them to 100
	if _, ok := thresholds[v1.ResourcePods]; ok {
		delete(thresholds, v1.ResourcePods)
		delete(targetThresholds, v1.ResourcePods)
	}
	if _, ok := thresholds[v1.ResourceCPU]; !ok {
		thresholds[v1.ResourceCPU] = MaxResourcePercentage
		targetThresholds[v1.ResourceCPU] = MaxResourcePercentage
	}
	if _, ok := thresholds[v1.ResourceMemory]; !ok {
		thresholds[v1.ResourceMemory] = MaxResourcePercentage
		targetThresholds[v1.ResourceMemory] = MaxResourcePercentage
	}
	resourceNames := getResourceNames(thresholds)

	lowNodes, sourceNodes := classifyNodesWithReal(
		getNodeRealUsage(nodes),
		getNodeThresholds(nodes, thresholds, targetThresholds, resourceNames),
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

func getNodeThresholds(
	nodes []*v1.Node,
	lowThreshold, highThreshold api.ResourceThresholds,
	resourceNames []v1.ResourceName,
) map[string]NodeThresholds {
	nodeThresholdsMap := map[string]NodeThresholds{}

	for _, node := range nodes {
		nodeCapacity := node.Status.Capacity
		if len(node.Status.Allocatable) > 0 {
			nodeCapacity = node.Status.Allocatable
		}

		nodeThresholdsMap[node.Name] = NodeThresholds{
			lowResourceThreshold:  map[v1.ResourceName]*resource.Quantity{},
			highResourceThreshold: map[v1.ResourceName]*resource.Quantity{},
		}

		for _, resourceName := range resourceNames {
			nodeThresholdsMap[node.Name].lowResourceThreshold[resourceName] = resourceThreshold(nodeCapacity, resourceName, lowThreshold[resourceName])
			nodeThresholdsMap[node.Name].highResourceThreshold[resourceName] = resourceThreshold(nodeCapacity, resourceName, highThreshold[resourceName])
		}

	}
	return nodeThresholdsMap
}

func normalizePercentage(percent api.Percentage) api.Percentage {
	if percent > MaxResourcePercentage {
		return MaxResourcePercentage
	}
	if percent < MinResourcePercentage {
		return MinResourcePercentage
	}
	return percent
}

func resourceThreshold(nodeCapacity v1.ResourceList, resourceName v1.ResourceName, threshold api.Percentage) *resource.Quantity {
	defaultFormat := resource.DecimalSI
	if resourceName == v1.ResourceMemory {
		defaultFormat = resource.BinarySI
	}

	resourceCapacityFraction := func(resourceNodeCapacity int64) int64 {
		// A threshold is in percentages but in <0;100> interval.
		// Performing `threshold * 0.01` will convert <0;100> interval into <0;1>.
		// Multiplying it with capacity will give fraction of the capacity corresponding to the given resource threshold in Quantity units.
		return int64(float64(threshold) * 0.01 * float64(resourceNodeCapacity))
	}

	resourceCapacityQuantity := nodeCapacity.Name(resourceName, defaultFormat)

	if resourceName == v1.ResourceCPU {
		return resource.NewMilliQuantity(resourceCapacityFraction(resourceCapacityQuantity.MilliValue()), defaultFormat)
	}
	return resource.NewQuantity(resourceCapacityFraction(resourceCapacityQuantity.Value()), defaultFormat)
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
