/*
Copyright 2020 The Kubernetes Authors.

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

package strategies

import (
	"context"
	"fmt"
	"math"
	"sort"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	clientset "k8s.io/client-go/kubernetes"
	klog "k8s.io/klog/v2"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	nodeutil "sigs.k8s.io/descheduler/pkg/descheduler/node"
	"sigs.k8s.io/descheduler/pkg/utils"
)

// AntiAffinityTerm's topology key value used in predicate metadata
type topologyPair struct {
	key   string
	value string
}

type topology struct {
	pair topologyPair
	pods []*v1.Pod
}

func validateAndParseTopologySpreadParams(ctx context.Context, client clientset.Interface, params *api.StrategyParameters) (int32, sets.String, sets.String, error) {
	var includedNamespaces, excludedNamespaces sets.String
	if params == nil {
		return 0, includedNamespaces, excludedNamespaces, nil
	}
	// At most one of include/exclude can be set
	if params.Namespaces != nil && len(params.Namespaces.Include) > 0 && len(params.Namespaces.Exclude) > 0 {
		return 0, includedNamespaces, excludedNamespaces, fmt.Errorf("only one of Include/Exclude namespaces can be set")
	}
	if params.ThresholdPriority != nil && params.ThresholdPriorityClassName != "" {
		return 0, includedNamespaces, excludedNamespaces, fmt.Errorf("only one of thresholdPriority and thresholdPriorityClassName can be set")
	}
	thresholdPriority, err := utils.GetPriorityFromStrategyParams(ctx, client, params)
	if err != nil {
		return 0, includedNamespaces, excludedNamespaces, fmt.Errorf("failed to get threshold priority from strategy's params: %+v", err)
	}
	if params.Namespaces != nil {
		includedNamespaces = sets.NewString(params.Namespaces.Include...)
		excludedNamespaces = sets.NewString(params.Namespaces.Exclude...)
	}

	return thresholdPriority, includedNamespaces, excludedNamespaces, nil
}

func RemovePodsViolatingTopologySpreadConstraint(
	ctx context.Context,
	client clientset.Interface,
	strategy api.DeschedulerStrategy,
	nodes []*v1.Node,
	podEvictor *evictions.PodEvictor,
	sopts ...StrategyOption,
) {
	thresholdPriority, includedNamespaces, excludedNamespaces, err := validateAndParseTopologySpreadParams(ctx, client, strategy.Params)
	if err != nil {
		klog.ErrorS(err, "Invalid RemovePodsViolatingTopologySpreadConstraint parameters")
		return
	}

	nodeMap := make(map[string]*v1.Node, len(nodes))
	for _, node := range nodes {
		nodeMap[node.Name] = node
	}
	evictable := podEvictor.Evictable(evictions.WithPriorityThreshold(thresholdPriority))

	// 1. for each namespace for which there is Topology Constraint
	// 2. for each TopologySpreadConstraint in that namespace
	//  { find all evictable pods in that namespace
	//  { 3. for each evictable pod in that namespace
	// 4. If the pod matches this TopologySpreadConstraint LabelSelector
	// 5. If the pod nodeName is present in the nodeMap
	// 6. create a topoPair with key as this TopologySpreadConstraint.TopologyKey and value as this pod's Node Label Value for this TopologyKey
	// 7. add the pod with key as this topoPair
	// 8. find the min number of pods in any topoPair for this topologyKey
	// iterate through all topoPairs for this topologyKey and diff currentPods -minPods <=maxSkew
	// if diff > maxSkew, add this pod in the current bucket for eviction

	// First record all of the constraints by namespace
	namespaces, err := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		klog.ErrorS(err, "Couldn't list namespaces")
		return
	}
	klog.V(1).InfoS("Processing namespaces for topology spread constraints")
	podsForEviction := make(map[*v1.Pod]struct{})
	// 1. for each namespace...
	for _, namespace := range namespaces.Items {
		if (len(includedNamespaces) > 0 && !includedNamespaces.Has(namespace.Name)) ||
			(len(excludedNamespaces) > 0 && excludedNamespaces.Has(namespace.Name)) {
			continue
		}
		namespacePods, err := client.CoreV1().Pods(namespace.Name).List(ctx, metav1.ListOptions{})
		if err != nil {
			klog.ErrorS(err, "Couldn't list pods in namespace", "namespace", namespace)
			continue
		}

		// ...where there is a topology constraint
		//namespaceTopologySpreadConstrainPods := make([]v1.Pod, 0, len(namespacePods.Items))
		namespaceTopologySpreadConstraints := make(map[v1.TopologySpreadConstraint]struct{})
		for _, pod := range namespacePods.Items {
			for _, constraint := range pod.Spec.TopologySpreadConstraints {
				// Ignore soft topology constraints if they are not included
				if (strategy.Params == nil || !strategy.Params.IncludeSoftConstraints) && constraint.WhenUnsatisfiable != v1.DoNotSchedule {
					continue
				}
				namespaceTopologySpreadConstraints[constraint] = struct{}{}
			}
		}
		if len(namespaceTopologySpreadConstraints) == 0 {
			continue
		}

		// 2. for each topologySpreadConstraint in that namespace
		for constraint := range namespaceTopologySpreadConstraints {
			constraintTopologies := make(map[topologyPair][]*v1.Pod)
			// pre-populate the topologyPair map with all the topologies available from the nodeMap
			// (we can't just build it from existing pods' nodes because a topology may have 0 pods)
			for _, node := range nodeMap {
				if val, ok := node.Labels[constraint.TopologyKey]; ok {
					constraintTopologies[topologyPair{key: constraint.TopologyKey, value: val}] = make([]*v1.Pod, 0)
				}
			}

			selector, err := metav1.LabelSelectorAsSelector(constraint.LabelSelector)
			if err != nil {
				klog.ErrorS(err, "Couldn't parse label selector as selector", "selector", constraint.LabelSelector)
				continue
			}

			// 3. for each evictable pod in that namespace
			// (this loop is where we count the number of pods per topologyValue that match this constraint's selector)
			var sumPods float64
			for i := range namespacePods.Items {
				// 4. if the pod matches this TopologySpreadConstraint LabelSelector
				if !selector.Matches(labels.Set(namespacePods.Items[i].Labels)) {
					continue
				}

				// 5. If the pod's node matches this constraint'selector topologyKey, create a topoPair and add the pod
				node, ok := nodeMap[namespacePods.Items[i].Spec.NodeName]
				if !ok {
					// If ok is false, node is nil in which case node.Labels will panic. In which case a pod is yet to be scheduled. So it's safe to just continue here.
					continue
				}
				nodeValue, ok := node.Labels[constraint.TopologyKey]
				if !ok {
					continue
				}
				// 6. create a topoPair with key as this TopologySpreadConstraint
				topoPair := topologyPair{key: constraint.TopologyKey, value: nodeValue}
				// 7. add the pod with key as this topoPair
				constraintTopologies[topoPair] = append(constraintTopologies[topoPair], &namespacePods.Items[i])
				sumPods++
			}
			if topologyIsBalanced(constraintTopologies, constraint) {
				klog.V(2).InfoS("Skipping topology constraint because it is already balanced", "constraint", constraint)
				continue
			}
			balanceDomains(podsForEviction, constraint, constraintTopologies, sumPods, evictable.IsEvictable, nodeMap)
		}
	}

	for pod := range podsForEviction {
		if !evictable.IsEvictable(pod) {
			continue
		}
		if _, err := podEvictor.EvictPod(ctx, pod, nodeMap[pod.Spec.NodeName], "PodTopologySpread"); err != nil {
			klog.ErrorS(err, "Error evicting pod", "pod", klog.KObj(pod))
			break
		}
	}
}

// topologyIsBalanced checks if any domains in the topology differ by more than the MaxSkew
// this is called before any sorting or other calculations and is used to skip topologies that don't need to be balanced
func topologyIsBalanced(topology map[topologyPair][]*v1.Pod, constraint v1.TopologySpreadConstraint) bool {
	minDomainSize := math.MaxInt32
	maxDomainSize := math.MinInt32
	for _, pods := range topology {
		if len(pods) < minDomainSize {
			minDomainSize = len(pods)
		}
		if len(pods) > maxDomainSize {
			maxDomainSize = len(pods)
		}
		if int32(maxDomainSize-minDomainSize) > constraint.MaxSkew {
			return false
		}
	}
	return true
}

// balanceDomains determines how many pods (minimum) should be evicted from large domains to achieve an ideal balance within maxSkew
// To actually determine how many pods need to be moved, we sort the topology domains in ascending length
// [2, 5, 3, 8, 5, 7]
//
// Would end up like:
// [2, 3, 5, 5, 7, 8]
//
// We then start at i=[0] and j=[len(list)-1] and compare the 2 topology sizes.
// If the diff of the size of the domains is more than the maxSkew, we will move up to half that skew,
// or the available pods from the higher domain, or the number required to bring the smaller domain up to the average,
// whichever number is less.
//
// (Note, we will only move as many pods from a domain as possible without bringing it below the ideal average,
//  and we will not bring any smaller domain above the average)
// If the diff is within the skew, we move to the next highest domain.
// If the higher domain can't give any more without falling below the average, we move to the next lowest "high" domain
//
// Following this, the above topology domains end up "sorted" as:
// [5, 5, 5, 5, 5, 5]
// (assuming even distribution by the scheduler of the evicted pods)
func balanceDomains(
	podsForEviction map[*v1.Pod]struct{},
	constraint v1.TopologySpreadConstraint,
	constraintTopologies map[topologyPair][]*v1.Pod,
	sumPods float64,
	isEvictable func(*v1.Pod) bool,
	nodeMap map[string]*v1.Node) {
	idealAvg := sumPods / float64(len(constraintTopologies))
	sortedDomains := sortDomains(constraintTopologies, isEvictable)
	// i is the index for belowOrEqualAvg
	// j is the index for aboveAvg
	i := 0
	j := len(sortedDomains) - 1
	for i < j {
		// if j has no more to give without falling below the ideal average, move to next aboveAvg
		if float64(len(sortedDomains[j].pods)) < idealAvg {
			j--
		}

		// skew = actual difference between the domains
		skew := float64(len(sortedDomains[j].pods) - len(sortedDomains[i].pods))

		// if k and j are within the maxSkew of each other, move to next belowOrEqualAvg
		if int32(skew) <= constraint.MaxSkew {
			i++
			continue
		}

		// the most that can be given from aboveAvg is:
		// 1. up to half the distance between them, minus MaxSkew, rounded up
		// 2. how many it has remaining without falling below the average rounded up, or
		// 3. how many can be added without bringing the smaller domain above the average rounded up,
		// whichever is less
		// (This is the basic principle of keeping all sizes within ~skew of the average)
		aboveAvg := math.Ceil(float64(len(sortedDomains[j].pods)) - idealAvg)
		belowAvg := math.Ceil(idealAvg - float64(len(sortedDomains[i].pods)))
		smallestDiff := math.Min(aboveAvg, belowAvg)
		halfSkew := math.Ceil((skew - float64(constraint.MaxSkew)) / 2)
		movePods := int(math.Min(smallestDiff, halfSkew))
		if movePods <= 0 {
			i++
			continue
		}

		// remove pods from the higher topology and add them to the list of pods to be evicted
		// also (just for tracking), add them to the list of pods in the lower topology
		aboveToEvict := sortedDomains[j].pods[len(sortedDomains[j].pods)-movePods:]
		for k := range aboveToEvict {
			// if the pod has a hard nodeAffinity or nodeSelector that only matches this node,
			// don't bother evicting it as it will just end up back on the same node
			// however we still account for it "being evicted" so the algorithm can complete
			// TODO(@damemi): Since we don't order pods wrt their affinities, we should refactor this to skip the current pod
			// but still try to get the required # of movePods (instead of just chopping that value off the slice above)
			isRequiredDuringSchedulingIgnoredDuringExecution := aboveToEvict[k].Spec.Affinity != nil &&
				aboveToEvict[k].Spec.Affinity.NodeAffinity != nil &&
				aboveToEvict[k].Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil

			if (aboveToEvict[k].Spec.NodeSelector != nil || isRequiredDuringSchedulingIgnoredDuringExecution) &&
				nodesPodFitsOnBesidesCurrent(aboveToEvict[k], nodeMap) == 0 {
				klog.V(2).InfoS("Ignoring pod for eviction due to node selector/affinity", "pod", klog.KObj(aboveToEvict[k]))
				continue
			}
			podsForEviction[aboveToEvict[k]] = struct{}{}
		}
		sortedDomains[j].pods = sortedDomains[j].pods[:len(sortedDomains[j].pods)-movePods]
		sortedDomains[i].pods = append(sortedDomains[i].pods, aboveToEvict...)
	}
}

// nodesPodFitsOnBesidesCurrent counts the number of nodes this pod could fit on based on its affinity
// It excludes the current node because, for the sake of domain balancing only, we care about if there is any other
// place it could theoretically fit.
// If the pod doesn't fit on its current node, that is a job for RemovePodsViolatingNodeAffinity, and irrelevant to Topology Spreading
func nodesPodFitsOnBesidesCurrent(pod *v1.Pod, nodeMap map[string]*v1.Node) int {
	count := 0
	for _, node := range nodeMap {
		if nodeutil.PodFitsCurrentNode(pod, node) && node != nodeMap[pod.Spec.NodeName] {
			count++
		}
	}
	return count
}

// sortDomains sorts and splits the list of topology domains based on their size
// it also sorts the list of pods within the domains based on their node affinity/selector and priority in the following order:
// 1. non-evictable pods
// 2. pods with selectors or affinity
// 3. pods in descending priority
// 4. all other pods
// We then pop pods off the back of the list for eviction
func sortDomains(constraintTopologyPairs map[topologyPair][]*v1.Pod, isEvictable func(*v1.Pod) bool) []topology {
	sortedTopologies := make([]topology, 0, len(constraintTopologyPairs))
	// sort the topologies and return 2 lists: those <= the average and those > the average (> list inverted)
	for pair, list := range constraintTopologyPairs {
		// Sort the pods within the domain so that the lowest priority pods are considered first for eviction,
		// followed by the highest priority,
		// followed by the lowest priority pods with affinity or nodeSelector,
		// followed by the highest priority pods with affinity or nodeSelector
		sort.Slice(list, func(i, j int) bool {
			// any non-evictable pods should be considered last (ie, first in the list)
			if !isEvictable(list[i]) || !isEvictable(list[j]) {
				// false - i is the only non-evictable, so return true to put it first
				// true - j is non-evictable, so return false to put j before i
				// if true and both and non-evictable, order doesn't matter
				return !(isEvictable(list[i]) && !isEvictable(list[j]))
			}

			// if both pods have selectors/affinity, compare them by their priority
			if hasSelectorOrAffinity(*list[i]) == hasSelectorOrAffinity(*list[j]) {
				comparePodsByPriority(list[i], list[j])
			}
			return hasSelectorOrAffinity(*list[i]) && !hasSelectorOrAffinity(*list[j])
		})
		sortedTopologies = append(sortedTopologies, topology{pair: pair, pods: list})
	}

	// create an ascending slice of all key-value toplogy pairs
	sort.Slice(sortedTopologies, func(i, j int) bool {
		return len(sortedTopologies[i].pods) < len(sortedTopologies[j].pods)
	})

	return sortedTopologies
}

func hasSelectorOrAffinity(pod v1.Pod) bool {
	return pod.Spec.NodeSelector != nil || (pod.Spec.Affinity != nil && pod.Spec.Affinity.NodeAffinity != nil)
}

// comparePodsByPriority is a helper to the sort function to compare 2 pods based on their priority values
// It will sort the pods in DESCENDING order of priority, since in our logic we evict pods from the back
// of the list first.
func comparePodsByPriority(iPod, jPod *v1.Pod) bool {
	if iPod.Spec.Priority != nil && jPod.Spec.Priority != nil {
		// a LOWER priority value should be evicted FIRST
		return *iPod.Spec.Priority > *jPod.Spec.Priority
	} else if iPod.Spec.Priority != nil && jPod.Spec.Priority == nil {
		// i should come before j
		return true
	} else if iPod.Spec.Priority == nil && jPod.Spec.Priority != nil {
		// j should come before i
		return false
	} else {
		// it doesn't matter. just return true
		return true
	}
}
