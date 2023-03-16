package realutilization

import (
	"context"
	"sort"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/cache"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	"sigs.k8s.io/descheduler/pkg/descheduler/node"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/framework"
	"sigs.k8s.io/descheduler/pkg/utils"
)

type RealNodeInfo struct {
	*cache.NodeUsageMap
	threshold NodeThresholds
}
type NodeThresholds struct {
	lowResourceThreshold  map[v1.ResourceName]*resource.Quantity
	highResourceThreshold map[v1.ResourceName]*resource.Quantity
}

func classifyNodesWithReal(
	nodeUsages []*cache.NodeUsageMap,
	nodeThresholds map[string]NodeThresholds,
	lowThresholdFilter, highThresholdFilter func(node *v1.Node, usage cache.NodeUsageMap, threshold NodeThresholds) bool,
) ([]RealNodeInfo, []RealNodeInfo) {
	var lowNodes, highNodes []RealNodeInfo

	for i, usage := range nodeUsages {
		nodeUsage := *usage
		node := nodeUsage.Node
		nodeInfo := RealNodeInfo{
			NodeUsageMap: nodeUsages[i],
			threshold:    nodeThresholds[node.Name],
		}
		if lowThresholdFilter(nodeUsage.Node, nodeUsage, nodeThresholds[node.Name]) {
			klog.InfoS("Node is underutilized", "node", klog.KObj(nodeUsage.Node), "usage", nodeUsage.CurrentUsage)
			lowNodes = append(lowNodes, nodeInfo)
		} else if highThresholdFilter(nodeUsage.Node, nodeUsage, nodeThresholds[node.Name]) {
			klog.InfoS("Node is overutilized", "node", klog.KObj(nodeUsage.Node), "usage", nodeUsage.CurrentUsage)
			highNodes = append(highNodes, nodeInfo)
		} else {
			klog.InfoS("Node is appropriately utilized", "node", klog.KObj(nodeUsage.Node), "usage", nodeUsage.CurrentUsage)
		}
	}

	return lowNodes, highNodes
}

type checkNodeFunc func(nodeThresholds v1.ResourceList, thresholds map[v1.ResourceName]*resource.Quantity) bool

// any resource big than thresholds will return true
func IsNodeAboveTargetUtilization(nodeThresholds v1.ResourceList, thresholds map[v1.ResourceName]*resource.Quantity) bool {
	for name, nodeValue := range nodeThresholds {
		if name == v1.ResourceCPU || name == v1.ResourceMemory || name == v1.ResourcePods {
			if value, ok := thresholds[name]; !ok {
				continue
			} else if nodeValue.Value() > value.Value() {
				return true
			}
		}
	}
	return false
}

// any resource small than thresholds will return false
func IsNodeWithLowUtilization(nodeThresholds v1.ResourceList, thresholds map[v1.ResourceName]*resource.Quantity) bool {
	for name, nodeValue := range nodeThresholds {
		if name == v1.ResourceCPU || name == v1.ResourceMemory || name == v1.ResourcePods {
			if value, ok := thresholds[name]; !ok {
				continue
			} else if nodeValue.Value() > value.Value() {
				return false
			}
		}
	}
	return true
}

// 检查最近n次的状态是否完全匹配
func CheckNodeByWindowLatestCt(nodeThresholdsList []v1.ResourceList, thresholds map[v1.ResourceName]*resource.Quantity, f checkNodeFunc, latestCt int) bool {
	if len(nodeThresholdsList) == 0 || len(nodeThresholdsList) < latestCt {
		// metrics is empty
		return false
	}
	for _, nodeThresholds := range nodeThresholdsList[len(nodeThresholdsList)-latestCt:] {
		if !f(nodeThresholds, thresholds) {
			return false
		}
	}
	return true
}

type continueEvictionCondReal func(nodeInfo RealNodeInfo, totalAvailableUsage map[v1.ResourceName]*resource.Quantity) bool

func evictPodsFromSourceNodesWithReal(
	ctx context.Context,
	evictableNamespaces *api.Namespaces,
	sourceNodes, destinationNodes []RealNodeInfo,
	podEvictor framework.Evictor,
	podFilter func(pod *v1.Pod) bool,
	resourceNames []v1.ResourceName,
	continueEviction continueEvictionCondReal,
) {
	// upper bound on total number of pods/cpu/memory and optional extended resources to be moved
	totalAvailableUsage := map[v1.ResourceName]*resource.Quantity{
		v1.ResourceCPU:    {},
		v1.ResourceMemory: {},
	}

	taintsOfDestinationNodes := make(map[string][]v1.Taint, len(destinationNodes))
	for _, node := range destinationNodes {
		taintsOfDestinationNodes[node.Node.Name] = node.Node.Spec.Taints

		for _, name := range resourceNames {
			if _, ok := totalAvailableUsage[name]; !ok {
				totalAvailableUsage[name] = resource.NewQuantity(0, resource.DecimalSI)
			}
			totalAvailableUsage[name].Add(*node.threshold.highResourceThreshold[name])
			totalAvailableUsage[name].Sub(node.CurrentUsage[name])
		}
	}

	// log message in one line
	keysAndValues := []interface{}{
		"CPU", totalAvailableUsage[v1.ResourceCPU].MilliValue(),
		"Mem", totalAvailableUsage[v1.ResourceMemory].Value(),
	}
	for name := range totalAvailableUsage {
		if !node.IsBasicResource(name) {
			keysAndValues = append(keysAndValues, string(name), totalAvailableUsage[name].Value())
		}
	}
	klog.V(1).InfoS("Total capacity to be moved", keysAndValues...)

	for _, node := range sourceNodes {
		klog.V(3).InfoS("Evicting pods from node", "node", klog.KObj(node.Node), "usage", node.CurrentUsage)
		nonRemovablePods, removablePods := classifyPodsWithReal(node.AllPods, podFilter)
		klog.V(2).InfoS("Pods on node", "node", klog.KObj(node.Node), "allPods", len(node.AllPods), "nonRemovablePods", len(nonRemovablePods), "removablePods", len(removablePods))

		if len(removablePods) == 0 {
			klog.V(1).InfoS("No removable pods on node, try next node", "node", klog.KObj(node.Node))
			continue
		}

		klog.V(1).InfoS("Evicting pods based on priority, if they have same priority, they'll be evicted based on QoS tiers")
		// sort the evictable Pods based on priority. This also sorts them based on QoS. If there are multiple pods with same priority, they are sorted based on QoS tiers.
		SortPodsBasedOnPriorityLowToHigh(removablePods)
		evictPodsWithReal(ctx, evictableNamespaces, removablePods, node, totalAvailableUsage, taintsOfDestinationNodes, podEvictor, continueEviction)

	}
}

func SortPodsBasedOnPriorityLowToHigh(podsUsage []*cache.PodUsageMap) {
	sort.Slice(podsUsage, func(i, j int) bool {
		pi := podsUsage[i].Pod
		pj := podsUsage[j].Pod
		if pi.Spec.Priority == nil && pj.Spec.Priority != nil {
			return true
		}
		if pj.Spec.Priority == nil && pi.Spec.Priority != nil {
			return false
		}
		if (pj.Spec.Priority == nil && pi.Spec.Priority == nil) || (*pi.Spec.Priority == *pj.Spec.Priority) {
			if podutil.IsBestEffortPod(pi) {
				return true
			}
			if podutil.IsBurstablePod(pi) && podutil.IsGuaranteedPod(pj) {
				return true
			}
			return false
		}
		if *pi.Spec.Priority != *pj.Spec.Priority {
			return *pi.Spec.Priority < *pj.Spec.Priority
		}
		if len(podsUsage[i].UsageList) == 0 {
			return true
		}
		if len(podsUsage[j].UsageList) == 0 {
			return false
		}
		piUsage := podsUsage[i].UsageList[len(podsUsage[i].UsageList)-1]
		pjUsage := podsUsage[j].UsageList[len(podsUsage[j].UsageList)-1]
		return piUsage.Cpu().MilliValue() < pjUsage.Cpu().MilliValue()
	})
}

func classifyPodsWithReal(pods []*cache.PodUsageMap, filter func(pod *v1.Pod) bool) ([]*cache.PodUsageMap, []*cache.PodUsageMap) {
	var nonRemovablePods, removablePods []*cache.PodUsageMap

	for _, podUsage := range pods {
		if !filter(podUsage.Pod) {
			nonRemovablePods = append(nonRemovablePods, podUsage)
		} else {
			removablePods = append(removablePods, podUsage)
		}
	}

	return nonRemovablePods, removablePods
}

func evictPodsWithReal(
	ctx context.Context,
	evictableNamespaces *api.Namespaces,
	inputPods []*cache.PodUsageMap,
	nodeInfo RealNodeInfo,
	totalAvailableUsage map[v1.ResourceName]*resource.Quantity,
	taintsOfLowNodes map[string][]v1.Taint,
	podEvictor framework.Evictor,
	continueEviction continueEvictionCondReal,
) {
	var excludedNamespaces sets.String
	if evictableNamespaces != nil {
		excludedNamespaces = sets.NewString(evictableNamespaces.Exclude...)
	}

	if continueEviction(nodeInfo, totalAvailableUsage) {
		for _, podUsage := range inputPods {
			if len(podUsage.UsageList) == 0 {
				continue
			}
			pod := podUsage.Pod
			if !utils.PodToleratesTaints(pod, taintsOfLowNodes) {
				klog.V(3).InfoS("Skipping eviction for pod, doesn't tolerate node taint", "pod", klog.KObj(pod))
				continue
			}
			podCurrentUsage := podUsage.UsageList[len(podUsage.UsageList)-1]
			preEvictionFilterWithOptions, err := podutil.NewOptions().
				WithFilter(podEvictor.PreEvictionFilter).
				WithoutNamespaces(excludedNamespaces).
				BuildFilterFunc()
			if err != nil {
				klog.ErrorS(err, "could not build preEvictionFilter with namespace exclusion")
				continue
			}
			// if pod too small to make usage to normal is wasteful,skip evict
			matchSmallPod := true
			for name := range nodeInfo.CurrentUsage {
				copyNodeUsage := nodeInfo.CurrentUsage[name].DeepCopy()
				copyNodeUsage.Sub(*nodeInfo.threshold.highResourceThreshold[name])
				podResourceUsage := podCurrentUsage[name]
				if copyNodeUsage.MilliValue() < podResourceUsage.MilliValue()*10 {
					matchSmallPod = false
				}
			}
			if matchSmallPod {
				continue
			}

			if preEvictionFilterWithOptions(pod) {
				if podEvictor.Evict(ctx, pod, evictions.EvictOptions{}) {
					klog.V(3).InfoS("Evicted pods", "pod", klog.KObj(pod))
					for name := range totalAvailableUsage {
						quantity := podCurrentUsage[name]
						currentUsage := nodeInfo.CurrentUsage[name]
						currentUsage.Sub(quantity)
						nodeInfo.CurrentUsage[name] = currentUsage
						totalAvailableUsage[name].Sub(quantity)
					}
					currentNodeUsage := nodeInfo.CurrentUsage
					cpuUsage := currentNodeUsage[v1.ResourceCPU]
					memUsage := currentNodeUsage[v1.ResourceMemory]

					keysAndValues := []interface{}{
						"node", nodeInfo.Node.Name,
						"CPU", cpuUsage.MilliValue(),
						"Mem", memUsage.Value(),
					}

					klog.V(3).InfoS("Updated node usage", keysAndValues...)
					// check if pods can be still evicted
					if !continueEviction(nodeInfo, totalAvailableUsage) {
						break
					}
				}
			}
			if podEvictor.NodeLimitExceeded(nodeInfo.Node) {
				return
			}
		}
	}
}
