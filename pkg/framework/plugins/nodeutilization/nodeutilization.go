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
	"math"
	"slices"
	"sort"

	"sigs.k8s.io/descheduler/pkg/api"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	nodeutil "sigs.k8s.io/descheduler/pkg/descheduler/node"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
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

func normalizePercentage(percent api.Percentage) api.Percentage {
	if percent > MaxResourcePercentage {
		return MaxResourcePercentage
	}
	if percent < MinResourcePercentage {
		return MinResourcePercentage
	}
	return percent
}

func getNodeThresholdsFromAverageNodeUsage(
	nodes []*v1.Node,
	usageClient usageClient,
	lowSpan, highSpan api.ResourceThresholds,
) map[string][]api.ResourceThresholds {
	total := api.ResourceThresholds{}
	average := api.ResourceThresholds{}
	numberOfNodes := len(nodes)
	for _, node := range nodes {
		usage := usageClient.nodeUtilization(node.Name)
		nodeCapacity := node.Status.Capacity
		if len(node.Status.Allocatable) > 0 {
			nodeCapacity = node.Status.Allocatable
		}
		for resource, value := range usage {
			nodeCapacityValue := nodeCapacity[resource]
			if resource == v1.ResourceCPU {
				total[resource] += api.Percentage(value.MilliValue()) / api.Percentage(nodeCapacityValue.MilliValue()) * 100.0
			} else {
				total[resource] += api.Percentage(value.Value()) / api.Percentage(nodeCapacityValue.Value()) * 100.0
			}
		}
	}
	lowThreshold, highThreshold := api.ResourceThresholds{}, api.ResourceThresholds{}
	for resource, value := range total {
		average[resource] = value / api.Percentage(numberOfNodes)
		// If either of the spans are 0, ignore the resource. I.e. 0%:5% is invalid.
		// Any zero span signifies a resource is either not set or is to be ignored.
		if lowSpan[resource] == MinResourcePercentage || highSpan[resource] == MinResourcePercentage {
			lowThreshold[resource] = 1
			highThreshold[resource] = 1
		} else {
			lowThreshold[resource] = normalizePercentage(average[resource] - lowSpan[resource])
			highThreshold[resource] = normalizePercentage(average[resource] + highSpan[resource])
		}
	}

	nodeThresholds := make(map[string][]api.ResourceThresholds)
	for _, node := range nodes {
		nodeThresholds[node.Name] = []api.ResourceThresholds{
			lowThreshold,
			highThreshold,
		}
	}
	return nodeThresholds
}

func getStaticNodeThresholds(
	nodes []*v1.Node,
	thresholdsList ...api.ResourceThresholds,
) map[string][]api.ResourceThresholds {
	nodeThresholds := make(map[string][]api.ResourceThresholds)
	for _, node := range nodes {
		nodeThresholds[node.Name] = append([]api.ResourceThresholds{}, slices.Clone(thresholdsList)...)
	}
	return nodeThresholds
}

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

func resourceThresholdsToNodeUsage(resourceThresholds api.ResourceThresholds, node *v1.Node) api.ReferencedResourceList {
	nodeUsage := make(api.ReferencedResourceList)

	nodeCapacity := node.Status.Capacity
	if len(node.Status.Allocatable) > 0 {
		nodeCapacity = node.Status.Allocatable
	}
	for resourceName, threshold := range resourceThresholds {
		nodeUsage[resourceName] = resourceThreshold(nodeCapacity, resourceName, threshold)
	}

	return nodeUsage
}

func roundTo2Decimals(percentage float64) float64 {
	return math.Round(percentage*100) / 100
}

func resourceUsagePercentages(nodeUsage api.ReferencedResourceList, node *v1.Node, round bool) api.ResourceThresholds {
	nodeCapacity := node.Status.Capacity
	if len(node.Status.Allocatable) > 0 {
		nodeCapacity = node.Status.Allocatable
	}

	resourceUsagePercentage := api.ResourceThresholds{}
	for resourceName, resourceUsage := range nodeUsage {
		cap := nodeCapacity[resourceName]
		if !cap.IsZero() {
			value := 100 * float64(resourceUsage.MilliValue()) / float64(cap.MilliValue())
			if round {
				value = roundTo2Decimals(float64(value))
			}
			resourceUsagePercentage[resourceName] = api.Percentage(value)
		}
	}
	return resourceUsagePercentage
}

func nodeUsageToResourceThresholds(nodeUsage map[string]api.ReferencedResourceList, nodes map[string]*v1.Node) map[string]api.ResourceThresholds {
	resourceThresholds := make(map[string]api.ResourceThresholds)
	for nodeName, node := range nodes {
		resourceThresholds[nodeName] = resourceUsagePercentages(nodeUsage[nodeName], node, false)
	}
	return resourceThresholds
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
			podUsage, err := usageClient.podUsage(pod)
			if err != nil {
				klog.Errorf("unable to get pod usage for %v/%v: %v", pod.Namespace, pod.Name, err)
				continue
			}
			err = podEvictor.Evict(ctx, pod, evictOptions)
			if err == nil {
				evictionCounter++
				klog.V(3).InfoS("Evicted pods", "pod", klog.KObj(pod))

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
	for name, resourceValue := range usage {
		if threshold[name] < resourceValue {
			return true
		}
	}
	return false
}

// isNodeBelowThreshold checks if a node is under a threshold
// All resources have to be below the threshold
func isNodeBelowThreshold(usage, threshold api.ResourceThresholds) bool {
	for name, resourceValue := range usage {
		if threshold[name] < resourceValue {
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
