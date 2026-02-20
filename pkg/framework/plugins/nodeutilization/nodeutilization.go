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
	"fmt"
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

// []NodeUsage is a snapshot, so allPods can not be read any time to avoid
// breaking consistency between the node's actual usage and available pods.
//
// New data model:
// - node usage: map[string]api.ReferencedResourceList
// - thresholds: map[string]api.ReferencedResourceList
// - all pods:   map[string][]*v1.Pod
//
// After classification:
//   - each group will have its own (smaller) node usage and thresholds and
//     allPods.
//
// Both node usage and thresholds are needed to compute the remaining resources
// that can be evicted/can accepted evicted pods.
//
//  1. translate node usages into percentages as float or int64 (how much
//     precision is lost?, maybe use BigInt?).
//  2. produce thresholds (if they need to be computed, otherwise use user
//     provided, they are already in percentages).
//  3. classify nodes into groups.
//  4. produces a list of nodes (sorted as before) that have the node usage,
//     the threshold (only one this time) and the snapshottted pod list
//     present.
//
// Data wise
// Produce separated maps for:
// - nodes: map[string]*v1.Node
// - node usage: map[string]api.ReferencedResourceList
// - thresholds: map[string][]api.ReferencedResourceList
// - pod list: map[string][]*v1.Pod
//
// Once the nodes are classified produce the original []NodeInfo so the code is
// not that much changed (postponing further refactoring once it is needed).

const (
	// MetricResource is a special resource name we use to keep track of a
	// metric obtained from a third party entity.
	MetricResource = v1.ResourceName("MetricResource")
	// MinResourcePercentage is the minimum value of a resource's percentage
	MinResourcePercentage = 0
	// MaxResourcePercentage is the maximum value of a resource's percentage
	MaxResourcePercentage = 100
)

// NodeUsage stores a node's info, pods on it, thresholds and its resource
// usage.
type NodeUsage struct {
	node    *v1.Node
	usage   api.ReferencedResourceList
	allPods []*v1.Pod
}

// NodeInfo is an entity we use to gather information about a given node. here
// we have its resource usage as well as the amount of available resources.
// we use this struct to carry information around and to make it easier to
// process.
type NodeInfo struct {
	NodeUsage
	available api.ReferencedResourceList
}

// continueEvictionCont is a function that determines if we should keep
// evicting pods or not.
type continueEvictionCond func(NodeInfo, api.ReferencedResourceList) bool

// getNodeUsageSnapshot separates the snapshot into easily accesible data
// chunks so the node usage can be processed separately. returns a map of
// nodes, a map of their usage and a map of their pods. maps are indexed
// by node name.
func getNodeUsageSnapshot(
	nodes []*v1.Node,
	usageClient usageClient,
) (
	map[string]*v1.Node,
	map[string]api.ReferencedResourceList,
	map[string][]*v1.Pod,
) {
	// XXX node usage needs to be kept in the original resource quantity
	// since converting to percentages and back is losing precision.
	nodesUsageMap := make(map[string]api.ReferencedResourceList)
	podListMap := make(map[string][]*v1.Pod)
	nodesMap := make(map[string]*v1.Node)

	for _, node := range nodes {
		nodesMap[node.Name] = node
		nodesUsageMap[node.Name] = usageClient.nodeUtilization(node.Name)
		podListMap[node.Name] = usageClient.pods(node.Name)
	}

	return nodesMap, nodesUsageMap, podListMap
}

// thresholdsToKeysAndValues converts a ResourceThresholds into a list of keys
// and values. this is useful for logging.
func thresholdsToKeysAndValues(thresholds api.ResourceThresholds) []any {
	result := []any{}
	for name, value := range thresholds {
		result = append(result, name, fmt.Sprintf("%.2f%%", value))
	}
	return result
}

// usageToKeysAndValues converts a ReferencedResourceList into a list of
// keys and values. this is useful for logging.
func usageToKeysAndValues(usage api.ReferencedResourceList) []any {
	keysAndValues := []any{}
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
			keysAndValues = append(keysAndValues, name, usage[name].Value())
		}
	}
	return keysAndValues
}

// evictPodsFromSourceNodes evicts pods based on priority, if all the pods on
// the node have priority, if not evicts them based on QoS as fallback option.
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
	logger := klog.FromContext(ctx)
	available, err := assessAvailableResourceInNodes(destinationNodes, resourceNames)
	if err != nil {
		logger.Error(err, "unable to assess available resources in nodes")
		return
	}

	logger.V(1).Info("Total capacity to be moved", usageToKeysAndValues(available)...)

	destinationTaints := make(map[string][]v1.Taint, len(destinationNodes))
	for _, node := range destinationNodes {
		destinationTaints[node.node.Name] = node.node.Spec.Taints
	}

	for _, node := range sourceNodes {
		logger.V(3).Info(
			"Evicting pods from node",
			"node", klog.KObj(node.node),
			"usage", node.usage,
		)

		nonRemovablePods, removablePods := classifyPods(node.allPods, podFilter)
		logger.V(2).Info(
			"Pods on node",
			"node", klog.KObj(node.node),
			"allPods", len(node.allPods),
			"nonRemovablePods", len(nonRemovablePods),
			"removablePods", len(removablePods),
		)

		if len(removablePods) == 0 {
			logger.V(1).Info(
				"No removable pods on node, try next node",
				"node", klog.KObj(node.node),
			)
			continue
		}

		logger.V(1).Info(
			"Evicting pods based on priority, if they have same priority, they'll be evicted based on QoS tiers",
		)

		// sort the evictable Pods based on priority. This also sorts
		// them based on QoS. If there are multiple pods with same
		// priority, they are sorted based on QoS tiers.
		podutil.SortPodsBasedOnPriorityLowToHigh(removablePods)

		if err := evictPods(
			ctx,
			evictableNamespaces,
			removablePods,
			node,
			available,
			destinationTaints,
			podEvictor,
			evictOptions,
			continueEviction,
			usageClient,
			maxNoOfPodsToEvictPerNode,
		); err != nil {
			switch err.(type) {
			case *evictions.EvictionTotalLimitError:
				return
			default:
			}
		}
	}
}

// evictPods keeps evicting pods until the continueEviction function returns
// false or we can't or shouldn't evict any more pods. available node resources
// are updated after each eviction.
func evictPods(
	ctx context.Context,
	evictableNamespaces *api.Namespaces,
	inputPods []*v1.Pod,
	nodeInfo NodeInfo,
	totalAvailableUsage api.ReferencedResourceList,
	destinationTaints map[string][]v1.Taint,
	podEvictor frameworktypes.Evictor,
	evictOptions evictions.EvictOptions,
	continueEviction continueEvictionCond,
	usageClient usageClient,
	maxNoOfPodsToEvictPerNode *uint,
) error {
	logger := klog.FromContext(ctx)
	// preemptive check to see if we should continue evicting pods.
	if !continueEviction(nodeInfo, totalAvailableUsage) {
		return nil
	}

	// some namespaces can be excluded from the eviction process.
	var excludedNamespaces sets.Set[string]
	if evictableNamespaces != nil {
		excludedNamespaces = sets.New(evictableNamespaces.Exclude...)
	}

	var evictionCounter uint = 0
	for _, pod := range inputPods {
		if maxNoOfPodsToEvictPerNode != nil && evictionCounter >= *maxNoOfPodsToEvictPerNode {
			logger.V(3).Info(
				"Max number of evictions per node per plugin reached",
				"limit", *maxNoOfPodsToEvictPerNode,
			)
			break
		}

		if !utils.PodToleratesTaints(ctx, pod, destinationTaints) {
			logger.V(3).Info(
				"Skipping eviction for pod, doesn't tolerate node taint",
				"pod", klog.KObj(pod),
			)
			continue
		}

		// verify if we can evict the pod based on the pod evictor
		// filter and on the excluded namespaces.
		preEvictionFilterWithOptions, err := podutil.
			NewOptions().
			WithFilter(podEvictor.PreEvictionFilter).
			WithoutNamespaces(excludedNamespaces).
			BuildFilterFunc()
		if err != nil {
			logger.Error(err, "could not build preEvictionFilter with namespace exclusion")
			continue
		}

		if !preEvictionFilterWithOptions(pod) {
			continue
		}

		// in case podUsage does not support resource counting (e.g.
		// provided metric does not quantify pod resource utilization).
		unconstrainedResourceEviction := false
		podUsage, err := usageClient.podUsage(pod)
		if err != nil {
			if _, ok := err.(*notSupportedError); !ok {
				logger.Error(err,
					"unable to get pod usage", "pod", klog.KObj(pod),
				)
				continue
			}
			unconstrainedResourceEviction = true
		}

		if err := podEvictor.Evict(ctx, pod, evictOptions); err != nil {
			switch err.(type) {
			case *evictions.EvictionNodeLimitError, *evictions.EvictionTotalLimitError:
				return err
			default:
				logger.Error(err, "eviction failed")
				continue
			}
		}

		if maxNoOfPodsToEvictPerNode == nil && unconstrainedResourceEviction {
			logger.V(3).Info("Currently, only a single pod eviction is allowed")
			break
		}

		evictionCounter++
		logger.V(3).Info("Evicted pods", "pod", klog.KObj(pod))
		if unconstrainedResourceEviction {
			continue
		}

		subtractPodUsageFromNodeAvailability(totalAvailableUsage, &nodeInfo, podUsage)

		keysAndValues := []any{"node", nodeInfo.node.Name}
		keysAndValues = append(keysAndValues, usageToKeysAndValues(nodeInfo.usage)...)
		logger.V(3).Info("Updated node usage", keysAndValues...)

		// make sure we should continue evicting pods.
		if !continueEviction(nodeInfo, totalAvailableUsage) {
			break
		}
	}
	return nil
}

// subtractPodUsageFromNodeAvailability subtracts the pod usage from the node
// available resources. this is done to keep track of the remaining resources
// that can be used to move pods around.
func subtractPodUsageFromNodeAvailability(
	available api.ReferencedResourceList,
	nodeInfo *NodeInfo,
	podUsage api.ReferencedResourceList,
) {
	for name := range available {
		if name == v1.ResourcePods {
			nodeInfo.usage[name].Sub(*resource.NewQuantity(1, resource.DecimalSI))
			available[name].Sub(*resource.NewQuantity(1, resource.DecimalSI))
			continue
		}
		nodeInfo.usage[name].Sub(*podUsage[name])
		available[name].Sub(*podUsage[name])
	}
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

// classifyPods classify them in two lists: removable and non-removable.
// Removable pods are those that can be evicted.
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

	// calculate the average usage.
	average := normalizer.Average(usage)
	klog.V(3).InfoS(
		"Assessed average usage",
		thresholdsToKeysAndValues(average)...,
	)

	// decrease the provided threshold from the average to get the low
	// span. also make sure the resulting values are between 0 and 100.
	lowerThresholds := normalizer.Clamp(
		normalizer.Sum(average, normalizer.Negate(lowSpan)), 0, 100,
	)
	klog.V(3).InfoS(
		"Assessed thresholds for underutilized nodes",
		thresholdsToKeysAndValues(lowerThresholds)...,
	)

	// increase the provided threshold from the average to get the high
	// span. also make sure the resulting values are between 0 and 100.
	higherThresholds := normalizer.Clamp(
		normalizer.Sum(average, highSpan), 0, 100,
	)
	klog.V(3).InfoS(
		"Assessed thresholds for overutilized nodes",
		thresholdsToKeysAndValues(higherThresholds)...,
	)

	// replicate the same assessed thresholds to all nodes.
	thresholds := normalizer.Replicate(
		slices.Collect(maps.Keys(usage)),
		[]api.ResourceThresholds{lowerThresholds, higherThresholds},
	)

	return usage, thresholds
}

// referencedResourceListForNodesCapacity returns a ReferencedResourceList for
// the capacity of a list of nodes. If allocatable resources are present, they
// are used instead of capacity.
func referencedResourceListForNodesCapacity(nodes []*v1.Node) map[string]api.ReferencedResourceList {
	capacities := map[string]api.ReferencedResourceList{}
	for _, node := range nodes {
		capacities[node.Name] = referencedResourceListForNodeCapacity(node)
	}
	return capacities
}

// referencedResourceListForNodeCapacity returns a ReferencedResourceList for
// the capacity of a node. If allocatable resources are present, they are used
// instead of capacity.
func referencedResourceListForNodeCapacity(node *v1.Node) api.ReferencedResourceList {
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

	return referenced
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
	return slices.Collect(maps.Keys(resourceNamesMap))
}

// filterResourceNamesFromNodeUsage removes from the node usage slice all keys
// that are not present in the resourceNames slice.
func filterResourceNames(
	from map[string]api.ReferencedResourceList, resourceNames []v1.ResourceName,
) map[string]api.ReferencedResourceList {
	newNodeUsage := make(map[string]api.ReferencedResourceList)
	for nodeName, usage := range from {
		newNodeUsage[nodeName] = api.ReferencedResourceList{}
		for _, resourceName := range resourceNames {
			if _, exists := usage[resourceName]; exists {
				newNodeUsage[nodeName][resourceName] = usage[resourceName]
			}
		}
	}
	return newNodeUsage
}

// capNodeCapacitiesToThreshold caps the node capacities to the given
// thresholds. if a threshold is not set for a resource, the full capacity is
// returned.
func capNodeCapacitiesToThreshold(
	node *v1.Node,
	thresholds api.ResourceThresholds,
	resourceNames []v1.ResourceName,
) api.ReferencedResourceList {
	capped := api.ReferencedResourceList{}
	for _, resourceName := range resourceNames {
		capped[resourceName] = capNodeCapacityToThreshold(
			node, thresholds, resourceName,
		)
	}
	return capped
}

// capNodeCapacityToThreshold caps the node capacity to the given threshold. if
// no threshold is set for the resource, the full capacity is returned.
func capNodeCapacityToThreshold(
	node *v1.Node, thresholds api.ResourceThresholds, resourceName v1.ResourceName,
) *resource.Quantity {
	capacities := referencedResourceListForNodeCapacity(node)
	if _, ok := capacities[resourceName]; !ok {
		// if the node knows nothing about the resource we return a
		// zero capacity for it.
		return resource.NewQuantity(0, resource.DecimalSI)
	}

	// if no threshold is set then we simply return the full capacity.
	if _, ok := thresholds[resourceName]; !ok {
		return capacities[resourceName]
	}

	// now that we have a capacity and a threshold we need to do the math
	// to cap the former to the latter.
	quantity := capacities[resourceName]
	threshold := thresholds[resourceName]

	// we have a different format for memory. all the other resources are
	// in the DecimalSI format.
	format := resource.DecimalSI
	if resourceName == v1.ResourceMemory {
		format = resource.BinarySI
	}

	// this is what we use to cap the capacity. thresholds are expected to
	// be in the <0;100> interval.
	fraction := func(threshold api.Percentage, capacity int64) int64 {
		return int64(float64(threshold) * 0.01 * float64(capacity))
	}

	// here we also vary a little bit. milli is used for cpu, all the rest
	// goes with the default.
	if resourceName == v1.ResourceCPU {
		return resource.NewMilliQuantity(
			fraction(threshold, quantity.MilliValue()),
			format,
		)
	}

	return resource.NewQuantity(
		fraction(threshold, quantity.Value()),
		format,
	)
}

// assessAvailableResourceInNodes computes the available resources in all the
// nodes. this is done by summing up all the available resources in all the
// nodes and then subtracting the usage from it.
func assessAvailableResourceInNodes(
	nodes []NodeInfo, resources []v1.ResourceName,
) (api.ReferencedResourceList, error) {
	// available holds a sum of all the resources that can be used to move
	// pods around. e.g. the sum of all available cpu and memory in all
	// cluster nodes.
	available := api.ReferencedResourceList{}
	for _, node := range nodes {
		for _, resourceName := range resources {
			if _, exists := node.usage[resourceName]; !exists {
				return nil, fmt.Errorf(
					"unable to find %s resource in node's %s usage, terminating eviction",
					resourceName, node.node.Name,
				)
			}

			// XXX this should never happen. we better bail out
			// here than hard crash with a segfault.
			if node.usage[resourceName] == nil {
				return nil, fmt.Errorf(
					"unable to find %s usage resources, terminating eviction",
					resourceName,
				)
			}

			// keep the current usage around so we can subtract it
			// from the available resources.
			usage := *node.usage[resourceName]

			// first time seeing this resource, initialize it.
			if _, ok := available[resourceName]; !ok {
				available[resourceName] = resource.NewQuantity(
					0, resource.DecimalSI,
				)
			}

			// XXX this should never happen. we better bail out
			// here than hard crash with a segfault.
			if node.available[resourceName] == nil {
				return nil, fmt.Errorf(
					"unable to find %s available resources, terminating eviction",
					resourceName,
				)
			}

			// now we add the capacity and then subtract the usage.
			available[resourceName].Add(*node.available[resourceName])
			available[resourceName].Sub(usage)
		}
	}

	return available, nil
}

// withResourceRequestForAny returns a filter function that checks if a pod
// has a resource request specified for any of the given resources names.
func withResourceRequestForAny(names ...v1.ResourceName) podutil.FilterFunc {
	return func(pod *v1.Pod) bool {
		all := append(pod.Spec.Containers, pod.Spec.InitContainers...)
		for _, name := range names {
			for _, container := range all {
				if _, ok := container.Resources.Requests[name]; ok {
					return true
				}
			}
		}
		return false
	}
}
