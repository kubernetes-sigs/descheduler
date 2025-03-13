/*
Copyright 2021 The Kubernetes Authors.

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
	"maps"
	"slices"
	"sort"

	"sigs.k8s.io/descheduler/pkg/api"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	nodeutil "sigs.k8s.io/descheduler/pkg/descheduler/node"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/nodeutilization/normalizer"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
	"sigs.k8s.io/descheduler/pkg/utils"
)

// []NodeUsage is a snapshot, so allPods can not be read any time to avoid breaking consistency between the node's actual usage and available pods
//
// New data model:
// - node usage: map[string]api.ReferencedResourceList
// - thresholds: map[string]api.ReferencedResourceList
// - all pods:   map[string][]*v1.Pod
// After classification:
// - each group will have its own (smaller) node usage and thresholds and allPods
// Both node usage and thresholds are needed to compute the remaining resources that can be evicted/can accepted evicted pods
//
// 1. translate node usages into percentages as float or int64 (how much precision is lost?, maybe use BigInt?)
// 2. produce thresholds (if they need to be computed, otherwise use user provided, they are already in percentages)
// 3. classify nodes into groups
// 4. produces a list of nodes (sorted as before) that have the node usage, the threshold (only one this time) and the snapshottted pod list present

// Data wise
// Produce separated maps for:
// - nodes: map[string]*v1.Node
// - node usage: map[string]api.ReferencedResourceList
// - thresholds: map[string][]api.ReferencedResourceList
// - pod list: map[string][]*v1.Pod
// Once the nodes are classified produce the original []NodeInfo so the code is not that much changed (postponing further refactoring once it is needed)
const MetricResource = v1.ResourceName("MetricResource")

// NodeUsage stores a node's info, pods on it, thresholds and its resource usage
type NodeUsage struct {
	node    *v1.Node
	usage   api.ReferencedResourceList
	allPods []*v1.Pod
}

type NodeThresholds struct {
	lowResourceThreshold  api.ReferencedResourceList
	highResourceThreshold api.ReferencedResourceList
}

type NodeInfo struct {
	NodeUsage
	thresholds NodeThresholds
}

type continueEvictionCond func(nodeInfo NodeInfo, totalAvailableUsage api.ReferencedResourceList) bool

const (
	// MinResourcePercentage is the minimum value of a resource's percentage
	MinResourcePercentage = 0
	// MaxResourcePercentage is the maximum value of a resource's percentage
	MaxResourcePercentage = 100
)

// getNodeUsageSnapshot separates the snapshot into easily accesible
// data chunks so the node usage can be processed separately.
func getNodeUsageSnapshot(
	nodes []*v1.Node,
	usageClient usageClient,
) (
	map[string]*v1.Node,
	map[string]api.ReferencedResourceList,
	map[string][]*v1.Pod,
) {
	nodesMap := make(map[string]*v1.Node)
	// node usage needs to be kept in the original resource quantity since converting to percentages and back is losing precision
	nodesUsageMap := make(map[string]api.ReferencedResourceList)
	podListMap := make(map[string][]*v1.Pod)

	for _, node := range nodes {
		nodesMap[node.Name] = node
		nodesUsageMap[node.Name] = usageClient.nodeUtilization(node.Name)
		podListMap[node.Name] = usageClient.pods(node.Name)
	}

	return nodesMap, nodesUsageMap, podListMap
}

func resourceThreshold(nodeCapacity api.ReferencedResourceList, resourceName v1.ResourceName, threshold api.Percentage) *resource.Quantity {
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

	resourceCapacityQuantity := &resource.Quantity{Format: defaultFormat}
	if _, ok := nodeCapacity[resourceName]; ok {
		resourceCapacityQuantity = nodeCapacity[resourceName]
	}

	if resourceName == v1.ResourceCPU {
		return resource.NewMilliQuantity(resourceCapacityFraction(resourceCapacityQuantity.MilliValue()), defaultFormat)
	}
	return resource.NewQuantity(resourceCapacityFraction(resourceCapacityQuantity.Value()), defaultFormat)
}

func resourceThresholdsToNodeUsage(resourceThresholds api.ResourceThresholds, capacity api.ReferencedResourceList, resourceNames []v1.ResourceName) api.ReferencedResourceList {
	nodeUsage := make(api.ReferencedResourceList)
	for resourceName, threshold := range resourceThresholds {
		nodeUsage[resourceName] = resourceThreshold(capacity, resourceName, threshold)
	}
	for _, resourceName := range resourceNames {
		if _, exists := nodeUsage[resourceName]; !exists {
			nodeUsage[resourceName] = capacity[resourceName]
		}
	}
	return nodeUsage
}

type classifierFnc func(nodeName string, value, threshold api.ResourceThresholds) bool

func classifyNodeUsage(
	nodeUsageAsNodeThresholds map[string]api.ResourceThresholds,
	nodeThresholdsMap map[string][]api.ResourceThresholds,
	classifiers []classifierFnc,
) []map[string]api.ResourceThresholds {
	nodeGroups := make([]map[string]api.ResourceThresholds, len(classifiers))
	for i := range len(classifiers) {
		nodeGroups[i] = make(map[string]api.ResourceThresholds)
	}

	for nodeName, nodeUsage := range nodeUsageAsNodeThresholds {
		for idx, classFnc := range classifiers {
			if classFnc(nodeName, nodeUsage, nodeThresholdsMap[nodeName][idx]) {
				nodeGroups[idx][nodeName] = nodeUsage
				break
			}
		}
	}

	return nodeGroups
}

func usageToKeysAndValues(usage api.ReferencedResourceList) []interface{} {
	// log message in one line
	keysAndValues := []interface{}{}
	if quantity, exists := usage[v1.ResourceCPU]; exists {
		keysAndValues = append(keysAndValues, "CPU", quantity.MilliValue())
	}
	if quantity, exists := usage[v1.ResourceMemory]; exists {
		keysAndValues = append(keysAndValues, "Mem", quantity.Value())
	}
	if quantity, exists := usage[v1.ResourcePods]; exists {
		keysAndValues = append(keysAndValues, "Pods", quantity.Value())
	}
	for name := range usage {
		if !nodeutil.IsBasicResource(name) {
			keysAndValues = append(keysAndValues, string(name), usage[name].Value())
		}
	}
	return keysAndValues
}

// evictPodsFromSourceNodes evicts pods based on priority, if all the pods on the node have priority, if not
// evicts them based on QoS as fallback option.
// TODO: @ravig Break this function into smaller functions.
func evictPodsFromSourceNodes(
	ctx context.Context,
	evictableNamespaces *api.Namespaces,
	sourceNodes, destinationNodes []NodeInfo,
	podEvictor frameworktypes.Evictor,
	evictOptions evictions.EvictOptions,
	podFilter func(pod *v1.Pod) bool,
	resourceNames []v1.ResourceName,
	continueEviction continueEvictionCond,
	usageClient usageClient,
	maxNoOfPodsToEvictPerNode *uint,
) {
	// upper bound on total number of pods/cpu/memory and optional extended resources to be moved
	totalAvailableUsage := api.ReferencedResourceList{}
	for _, resourceName := range resourceNames {
		totalAvailableUsage[resourceName] = &resource.Quantity{}
	}

	taintsOfDestinationNodes := make(map[string][]v1.Taint, len(destinationNodes))
	for _, node := range destinationNodes {
		taintsOfDestinationNodes[node.node.Name] = node.node.Spec.Taints

		for _, name := range resourceNames {
			if _, exists := node.usage[name]; !exists {
				klog.Errorf("unable to find %q resource in node's %q usage, terminating eviction", name, node.node.Name)
				return
			}
			if _, ok := totalAvailableUsage[name]; !ok {
				totalAvailableUsage[name] = resource.NewQuantity(0, resource.DecimalSI)
			}
			totalAvailableUsage[name].Add(*node.thresholds.highResourceThreshold[name])
			totalAvailableUsage[name].Sub(*node.usage[name])
		}
	}

	// log message in one line
	klog.V(1).InfoS("Total capacity to be moved", usageToKeysAndValues(totalAvailableUsage)...)

	for _, node := range sourceNodes {
		klog.V(3).InfoS("Evicting pods from node", "node", klog.KObj(node.node), "usage", node.usage)

		nonRemovablePods, removablePods := classifyPods(node.allPods, podFilter)
		klog.V(2).InfoS("Pods on node", "node", klog.KObj(node.node), "allPods", len(node.allPods), "nonRemovablePods", len(nonRemovablePods), "removablePods", len(removablePods))

		if len(removablePods) == 0 {
			klog.V(1).InfoS("No removable pods on node, try next node", "node", klog.KObj(node.node))
			continue
		}

		klog.V(1).InfoS("Evicting pods based on priority, if they have same priority, they'll be evicted based on QoS tiers")
		// sort the evictable Pods based on priority. This also sorts them based on QoS. If there are multiple pods with same priority, they are sorted based on QoS tiers.
		podutil.SortPodsBasedOnPriorityLowToHigh(removablePods)
		err := evictPods(ctx, evictableNamespaces, removablePods, node, totalAvailableUsage, taintsOfDestinationNodes, podEvictor, evictOptions, continueEviction, usageClient, maxNoOfPodsToEvictPerNode)
		if err != nil {
			switch err.(type) {
			case *evictions.EvictionTotalLimitError:
				return
			default:
			}
		}
	}
}

func evictPods(
	ctx context.Context,
	evictableNamespaces *api.Namespaces,
	inputPods []*v1.Pod,
	nodeInfo NodeInfo,
	totalAvailableUsage api.ReferencedResourceList,
	taintsOfLowNodes map[string][]v1.Taint,
	podEvictor frameworktypes.Evictor,
	evictOptions evictions.EvictOptions,
	continueEviction continueEvictionCond,
	usageClient usageClient,
	maxNoOfPodsToEvictPerNode *uint,
) error {
	var excludedNamespaces sets.Set[string]
	if evictableNamespaces != nil {
		excludedNamespaces = sets.New(evictableNamespaces.Exclude...)
	}

	var evictionCounter uint = 0
	if continueEviction(nodeInfo, totalAvailableUsage) {
		for _, pod := range inputPods {
			if maxNoOfPodsToEvictPerNode != nil && evictionCounter >= *maxNoOfPodsToEvictPerNode {
				klog.V(3).InfoS("Max number of evictions per node per plugin reached", "limit", *maxNoOfPodsToEvictPerNode)
				break
			}
			if !utils.PodToleratesTaints(pod, taintsOfLowNodes) {
				klog.V(3).InfoS("Skipping eviction for pod, doesn't tolerate node taint", "pod", klog.KObj(pod))
				continue
			}

			preEvictionFilterWithOptions, err := podutil.NewOptions().
				WithFilter(podEvictor.PreEvictionFilter).
				WithoutNamespaces(excludedNamespaces).
				BuildFilterFunc()
			if err != nil {
				klog.ErrorS(err, "could not build preEvictionFilter with namespace exclusion")
				continue
			}

			if !preEvictionFilterWithOptions(pod) {
				continue
			}

			// In case podUsage does not support resource counting (e.g. provided metric
			// does not quantify pod resource utilization).
			unconstrainedResourceEviction := false
			podUsage, err := usageClient.podUsage(pod)
			if err != nil {
				if _, ok := err.(*notSupportedError); !ok {
					klog.Errorf("unable to get pod usage for %v/%v: %v", pod.Namespace, pod.Name, err)
					continue
				}
				unconstrainedResourceEviction = true
			}
			err = podEvictor.Evict(ctx, pod, evictOptions)
			if err == nil {
				if maxNoOfPodsToEvictPerNode == nil && unconstrainedResourceEviction {
					klog.V(3).InfoS("Currently, only a single pod eviction is allowed")
					break
				}
				evictionCounter++
				klog.V(3).InfoS("Evicted pods", "pod", klog.KObj(pod))
				if unconstrainedResourceEviction {
					continue
				}
				for name := range totalAvailableUsage {
					if name == v1.ResourcePods {
						nodeInfo.usage[name].Sub(*resource.NewQuantity(1, resource.DecimalSI))
						totalAvailableUsage[name].Sub(*resource.NewQuantity(1, resource.DecimalSI))
					} else {
						nodeInfo.usage[name].Sub(*podUsage[name])
						totalAvailableUsage[name].Sub(*podUsage[name])
					}
				}

				keysAndValues := []interface{}{
					"node", nodeInfo.node.Name,
				}
				keysAndValues = append(keysAndValues, usageToKeysAndValues(nodeInfo.usage)...)
				klog.V(3).InfoS("Updated node usage", keysAndValues...)
				// check if pods can be still evicted
				if !continueEviction(nodeInfo, totalAvailableUsage) {
					break
				}
				continue
			}
			switch err.(type) {
			case *evictions.EvictionNodeLimitError, *evictions.EvictionTotalLimitError:
				return err
			default:
				klog.Errorf("eviction failed: %v", err)
			}
		}
	}
	return nil
}

// sortNodesByUsage sorts nodes based on usage according to the given plugin.
func sortNodesByUsage(nodes []NodeInfo, ascending bool) {
	sort.Slice(nodes, func(i, j int) bool {
		ti := resource.NewQuantity(0, resource.DecimalSI).Value()
		tj := resource.NewQuantity(0, resource.DecimalSI).Value()
		for resourceName := range nodes[i].usage {
			if resourceName == v1.ResourceCPU {
				ti += nodes[i].usage[resourceName].MilliValue()
			} else {
				ti += nodes[i].usage[resourceName].Value()
			}
		}
		for resourceName := range nodes[j].usage {
			if resourceName == v1.ResourceCPU {
				tj += nodes[j].usage[resourceName].MilliValue()
			} else {
				tj += nodes[j].usage[resourceName].Value()
			}
		}

		// Return ascending order for HighNodeUtilization plugin
		if ascending {
			return ti < tj
		}

		// Return descending order for LowNodeUtilization plugin
		return ti > tj
	})
}

// isNodeAboveTargetUtilization checks if a node is overutilized
// At least one resource has to be above the high threshold
func isNodeAboveTargetUtilization(usage NodeUsage, threshold api.ReferencedResourceList) bool {
	for name, nodeValue := range usage.usage {
		// usage.highResourceThreshold[name] < nodeValue
		if threshold[name].Cmp(*nodeValue) == -1 {
			return true
		}
	}
	return false
}

// isNodeAboveThreshold checks if a node is over a threshold
// At least one resource has to be above the threshold
func isNodeAboveThreshold(usage, threshold api.ResourceThresholds) bool {
	for name := range threshold {
		if threshold[name] < usage[name] {
			return true
		}
	}
	return false
}

// isNodeBelowThreshold checks if a node is under a threshold
// All resources have to be below the threshold
func isNodeBelowThreshold(usage, threshold api.ResourceThresholds) bool {
	for name := range threshold {
		if threshold[name] < usage[name] {
			return false
		}
	}
	return true
}

// getResourceNames returns list of resource names in resource thresholds
func getResourceNames(thresholds api.ResourceThresholds) []v1.ResourceName {
	resourceNames := make([]v1.ResourceName, 0, len(thresholds))
	for name := range thresholds {
		resourceNames = append(resourceNames, name)
	}
	return resourceNames
}

func classifyPods(pods []*v1.Pod, filter func(pod *v1.Pod) bool) ([]*v1.Pod, []*v1.Pod) {
	var nonRemovablePods, removablePods []*v1.Pod

	for _, pod := range pods {
		if !filter(pod) {
			nonRemovablePods = append(nonRemovablePods, pod)
		} else {
			removablePods = append(removablePods, pod)
		}
	}

	return nonRemovablePods, removablePods
}

// assessNodesUsagesAndStaticThresholds converts the raw usage data into
// percentage. Returns the usage (pct) and the thresholds (pct) for each
// node.
func assessNodesUsagesAndStaticThresholds(
	rawUsages, rawCapacities map[string]api.ReferencedResourceList,
	lowSpan, highSpan api.ResourceThresholds,
) (map[string]api.ResourceThresholds, map[string][]api.ResourceThresholds) {
	// first we normalize the node usage from the raw data (Mi, Gi, etc)
	// into api.Percentage values.
	usage := normalizer.Normalize(
		rawUsages, rawCapacities, ResourceUsageToResourceThreshold,
	)

	// we are not taking the average and applying deviations to it we can
	// simply replicate the same threshold across all nodes and return.
	thresholds := normalizer.Replicate(
		slices.Collect(maps.Keys(usage)),
		[]api.ResourceThresholds{lowSpan, highSpan},
	)
	return usage, thresholds
}

// assessNodesUsagesAndRelativeThresholds converts the raw usage data into
// percentage. Thresholds are calculated based on the average usage. Returns
// the usage (pct) and the thresholds (pct) for each node.
func assessNodesUsagesAndRelativeThresholds(
	rawUsages, rawCapacities map[string]api.ReferencedResourceList,
	lowSpan, highSpan api.ResourceThresholds,
) (map[string]api.ResourceThresholds, map[string][]api.ResourceThresholds) {
	// first we normalize the node usage from the raw data (Mi, Gi, etc)
	// into api.Percentage values.
	usage := normalizer.Normalize(
		rawUsages, rawCapacities, ResourceUsageToResourceThreshold,
	)

	// calculate the average usage and then deviate it according to the
	// user provided thresholds.
	average := normalizer.Average(usage)

	// calculate the average usage and then deviate it according to the
	// user provided thresholds. We also ensure that the value after the
	// deviation is at least 1%. this call also replicates the thresholds
	// across all nodes.
	thresholds := normalizer.Replicate(
		slices.Collect(maps.Keys(usage)),
		normalizer.Map(
			[]api.ResourceThresholds{
				normalizer.Sum(average, normalizer.Negate(lowSpan)),
				normalizer.Sum(average, highSpan),
			},
			func(thresholds api.ResourceThresholds) api.ResourceThresholds {
				return normalizer.Clamp(thresholds, 0, 100)
			},
		),
	)

	return usage, thresholds
}

// referencedResourceListForNodesCapacity returns a ReferencedResourceList for
// the capacity of a list of nodes. If allocatable resources are present, they
// are used instead of capacity.
func referencedResourceListForNodesCapacity(nodes []*v1.Node) map[string]api.ReferencedResourceList {
	capacities := map[string]api.ReferencedResourceList{}
	for _, node := range nodes {
		capacity := node.Status.Capacity
		if len(node.Status.Allocatable) > 0 {
			capacity = node.Status.Allocatable
		}

		referenced := api.ReferencedResourceList{}
		for name, quantity := range capacity {
			referenced[name] = ptr.To(quantity)
		}

		// XXX the descheduler also manages monitoring queries that are
		// supposed to return a value representing a percentage of the
		// resource usage. In this case we need to provide a value for
		// the MetricResource, which is not present in the node capacity.
		referenced[MetricResource] = resource.NewQuantity(
			100, resource.DecimalSI,
		)

		capacities[node.Name] = referenced
	}
	return capacities
}

// ResourceUsage2ResourceThreshold is an implementation of a Normalizer that
// converts a set of resource usages and totals into percentage. This function
// operates on Quantity Value() for all the resources except CPU, where it uses
// MilliValue().
func ResourceUsageToResourceThreshold(
	usages, totals api.ReferencedResourceList,
) api.ResourceThresholds {
	result := api.ResourceThresholds{}
	for rname, value := range usages {
		if value == nil || totals[rname] == nil {
			continue
		}

		total := totals[rname]
		used, capacity := value.Value(), total.Value()
		if rname == v1.ResourceCPU {
			used, capacity = value.MilliValue(), total.MilliValue()
		}

		var percent float64
		if capacity > 0 {
			percent = float64(used) / float64(capacity) * 100
		}

		result[rname] = api.Percentage(percent)
	}
	return result
}

// uniquifyResourceNames returns a slice of resource names with duplicates
// removed.
func uniquifyResourceNames(resourceNames []v1.ResourceName) []v1.ResourceName {
	resourceNamesMap := map[v1.ResourceName]bool{
		v1.ResourceCPU:    true,
		v1.ResourceMemory: true,
		v1.ResourcePods:   true,
	}
	for _, resourceName := range resourceNames {
		resourceNamesMap[resourceName] = true
	}
	extendedResourceNames := []v1.ResourceName{}
	for resourceName := range resourceNamesMap {
		extendedResourceNames = append(extendedResourceNames, resourceName)
	}
	return extendedResourceNames
}

// filterResourceNamesFromNodeUsage removes from the node usage slice all keys
// that are not present in the resourceNames slice.
func filterResourceNamesFromNodeUsage(
	nodeUsage map[string]api.ReferencedResourceList, resourceNames []v1.ResourceName,
) map[string]api.ReferencedResourceList {
	newNodeUsage := make(map[string]api.ReferencedResourceList)
	for nodeName, usage := range nodeUsage {
		newNodeUsage[nodeName] = api.ReferencedResourceList{}
		for _, resourceName := range resourceNames {
			if _, exists := usage[resourceName]; exists {
				newNodeUsage[nodeName][resourceName] = usage[resourceName]
			}
		}
	}
	return newNodeUsage
}
