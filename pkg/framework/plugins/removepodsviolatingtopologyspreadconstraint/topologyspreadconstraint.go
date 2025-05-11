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

package removepodsviolatingtopologyspreadconstraint

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"sort"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	utilptr "k8s.io/utils/ptr"

	v1helper "k8s.io/component-helpers/scheduling/corev1"
	"k8s.io/component-helpers/scheduling/corev1/nodeaffinity"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	"sigs.k8s.io/descheduler/pkg/descheduler/node"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
	"sigs.k8s.io/descheduler/pkg/utils"
)

const PluginName = "RemovePodsViolatingTopologySpreadConstraint"

// AntiAffinityTerm's topology key value used in predicate metadata
type topologyPair struct {
	key   string
	value string
}

type topology struct {
	pair topologyPair
	pods []*v1.Pod
}

// topologySpreadConstraint is an internal version for v1.TopologySpreadConstraint
// and where the selector is parsed.
// This mirrors scheduler: https://github.com/kubernetes/kubernetes/blob/release-1.28/pkg/scheduler/framework/plugins/podtopologyspread/common.go#L37
// Fields are exported for structured logging.
type topologySpreadConstraint struct {
	MaxSkew            int32
	TopologyKey        string
	Selector           labels.Selector
	NodeAffinityPolicy v1.NodeInclusionPolicy
	NodeTaintsPolicy   v1.NodeInclusionPolicy
	PodNodeAffinity    nodeaffinity.RequiredNodeAffinity
	PodTolerations     []v1.Toleration
}

// RemovePodsViolatingTopologySpreadConstraint evicts pods which violate their topology spread constraints
type RemovePodsViolatingTopologySpreadConstraint struct {
	handle    frameworktypes.Handle
	args      *RemovePodsViolatingTopologySpreadConstraintArgs
	podFilter podutil.FilterFunc
}

var _ frameworktypes.BalancePlugin = &RemovePodsViolatingTopologySpreadConstraint{}

// New builds plugin from its arguments while passing a handle
func New(args runtime.Object, handle frameworktypes.Handle) (frameworktypes.Plugin, error) {
	pluginArgs, ok := args.(*RemovePodsViolatingTopologySpreadConstraintArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type RemovePodsViolatingTopologySpreadConstraintArgs, got %T", args)
	}

	var includedNamespaces, excludedNamespaces sets.Set[string]
	if pluginArgs.Namespaces != nil {
		includedNamespaces = sets.New(pluginArgs.Namespaces.Include...)
		excludedNamespaces = sets.New(pluginArgs.Namespaces.Exclude...)
	}

	podFilter, err := podutil.NewOptions().
		WithFilter(handle.Evictor().Filter).
		WithLabelSelector(pluginArgs.LabelSelector).
		WithNamespaces(includedNamespaces).
		WithoutNamespaces(excludedNamespaces).
		BuildFilterFunc()
	if err != nil {
		return nil, fmt.Errorf("error initializing pod filter function: %v", err)
	}

	return &RemovePodsViolatingTopologySpreadConstraint{
		handle:    handle,
		podFilter: podFilter,
		args:      pluginArgs,
	}, nil
}

// Name retrieves the plugin name
func (d *RemovePodsViolatingTopologySpreadConstraint) Name() string {
	return PluginName
}

// nolint: gocyclo
func (d *RemovePodsViolatingTopologySpreadConstraint) Balance(ctx context.Context, nodes []*v1.Node) *frameworktypes.Status {
	nodeMap := make(map[string]*v1.Node, len(nodes))
	for _, node := range nodes {
		nodeMap[node.Name] = node
	}

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

	klog.V(1).Info("Processing namespaces for topology spread constraints")
	podsForEviction := make(map[*v1.Pod]struct{})

	pods, err := podutil.ListPodsOnNodes(nodes, d.handle.GetPodsAssignedToNodeFunc(), d.podFilter)
	if err != nil {
		return &frameworktypes.Status{
			Err: fmt.Errorf("error listing all pods: %v", err),
		}
	}

	allowedConstraints := sets.New[v1.UnsatisfiableConstraintAction](d.args.Constraints...)

	namespacedPods := podutil.GroupByNamespace(pods)

	// 1. for each namespace...
	for namespace := range namespacedPods {
		klog.V(4).InfoS("Processing namespace for topology spread constraints", "namespace", namespace)

		// ...where there is a topology constraint
		var namespaceTopologySpreadConstraints []topologySpreadConstraint
		for _, pod := range namespacedPods[namespace] {
			for _, constraint := range pod.Spec.TopologySpreadConstraints {
				// Ignore topology constraints if they are not included
				if !allowedConstraints.Has(constraint.WhenUnsatisfiable) {
					continue
				}

				namespaceTopologySpreadConstraint, err := newTopologySpreadConstraint(constraint, pod)
				if err != nil {
					klog.ErrorS(err, "cannot process topology spread constraint")
					continue
				}

				// Need to check TopologySpreadConstraint deepEquality because
				// TopologySpreadConstraint can haves pointer fields
				// and we don't need to go over duplicated constraints later on
				if hasIdenticalConstraints(namespaceTopologySpreadConstraint, namespaceTopologySpreadConstraints) {
					continue
				}
				namespaceTopologySpreadConstraints = append(namespaceTopologySpreadConstraints, namespaceTopologySpreadConstraint)
			}
		}
		if len(namespaceTopologySpreadConstraints) == 0 {
			continue
		}

		// 2. for each topologySpreadConstraint in that namespace
		for _, tsc := range namespaceTopologySpreadConstraints {
			constraintTopologies := make(map[topologyPair][]*v1.Pod)
			// pre-populate the topologyPair map with all the topologies available from the nodeMap
			// (we can't just build it from existing pods' nodes because a topology may have 0 pods)
			for _, node := range nodeMap {
				if val, ok := node.Labels[tsc.TopologyKey]; ok {
					if matchNodeInclusionPolicies(tsc, node) {
						constraintTopologies[topologyPair{key: tsc.TopologyKey, value: val}] = make([]*v1.Pod, 0)
					}
				}
			}

			// 3. for each evictable pod in that namespace
			// (this loop is where we count the number of pods per topologyValue that match this constraint's selector)
			var sumPods float64
			for _, pod := range namespacedPods[namespace] {
				// skip pods that are being deleted.
				if utils.IsPodTerminating(pod) {
					continue
				}
				// 4. if the pod matches this TopologySpreadConstraint LabelSelector
				if !tsc.Selector.Matches(labels.Set(pod.Labels)) {
					continue
				}

				// 5. If the pod's node matches this constraint's topologyKey, create a topoPair and add the pod
				node, ok := nodeMap[pod.Spec.NodeName]
				if !ok {
					// If ok is false, node is nil in which case node.Labels will panic. In which case a pod is yet to be scheduled. So it's safe to just continue here.
					continue
				}
				nodeValue, ok := node.Labels[tsc.TopologyKey]
				if !ok {
					continue
				}
				// 6. create a topoPair with key as this TopologySpreadConstraint
				topoPair := topologyPair{key: tsc.TopologyKey, value: nodeValue}
				// 7. add the pod with key as this topoPair
				constraintTopologies[topoPair] = append(constraintTopologies[topoPair], pod)
				sumPods++
			}
			if topologyIsBalanced(constraintTopologies, tsc) {
				klog.V(2).InfoS("Skipping topology constraint because it is already balanced", "constraint", tsc)
				continue
			}
			d.balanceDomains(podsForEviction, tsc, constraintTopologies, sumPods, nodes)
		}
	}

	nodeLimitExceeded := map[string]bool{}
	for pod := range podsForEviction {
		if nodeLimitExceeded[pod.Spec.NodeName] {
			continue
		}
		if !d.podFilter(pod) {
			continue
		}

		if d.handle.Evictor().PreEvictionFilter(pod) {
			err := d.handle.Evictor().Evict(ctx, pod, evictions.EvictOptions{StrategyName: PluginName})
			if err == nil {
				continue
			}
			switch err.(type) {
			case *evictions.EvictionNodeLimitError:
				nodeLimitExceeded[pod.Spec.NodeName] = true
			case *evictions.EvictionTotalLimitError:
				return nil
			default:
				klog.Errorf("eviction failed: %v", err)
			}
		}
	}

	return nil
}

// hasIdenticalConstraints checks if we already had an identical TopologySpreadConstraint in namespaceTopologySpreadConstraints slice
func hasIdenticalConstraints(newConstraint topologySpreadConstraint, namespaceTopologySpreadConstraints []topologySpreadConstraint) bool {
	for _, constraint := range namespaceTopologySpreadConstraints {
		if reflect.DeepEqual(newConstraint, constraint) {
			return true
		}
	}
	return false
}

// topologyIsBalanced checks if any domains in the topology differ by more than the MaxSkew
// this is called before any sorting or other calculations and is used to skip topologies that don't need to be balanced
func topologyIsBalanced(topology map[topologyPair][]*v1.Pod, tsc topologySpreadConstraint) bool {
	minDomainSize := math.MaxInt32
	maxDomainSize := math.MinInt32
	for _, pods := range topology {
		if len(pods) < minDomainSize {
			minDomainSize = len(pods)
		}
		if len(pods) > maxDomainSize {
			maxDomainSize = len(pods)
		}
		if int32(maxDomainSize-minDomainSize) > tsc.MaxSkew {
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
// and we will not bring any smaller domain above the average)
// If the diff is within the skew, we move to the next highest domain.
// If the higher domain can't give any more without falling below the average, we move to the next lowest "high" domain
//
// Following this, the above topology domains end up "sorted" as:
// [5, 5, 5, 5, 5, 5]
// (assuming even distribution by the scheduler of the evicted pods)
func (d *RemovePodsViolatingTopologySpreadConstraint) balanceDomains(
	podsForEviction map[*v1.Pod]struct{},
	tsc topologySpreadConstraint,
	constraintTopologies map[topologyPair][]*v1.Pod,
	sumPods float64,
	nodes []*v1.Node,
) {
	idealAvg := sumPods / float64(len(constraintTopologies))
	isEvictable := d.handle.Evictor().Filter
	sortedDomains := sortDomains(constraintTopologies, isEvictable)
	getPodsAssignedToNode := d.handle.GetPodsAssignedToNodeFunc()
	topologyBalanceNodeFit := utilptr.Deref(d.args.TopologyBalanceNodeFit, true)

	eligibleNodes := filterEligibleNodes(nodes, tsc)
	nodesBelowIdealAvg := filterNodesBelowIdealAvg(eligibleNodes, sortedDomains, tsc.TopologyKey, idealAvg)

	// i is the index for belowOrEqualAvg
	// j is the index for aboveAvg
	i := 0
	j := len(sortedDomains) - 1
	for i < j {
		// if j has no more to give without falling below the ideal average, move to next aboveAvg
		if float64(len(sortedDomains[j].pods)) <= idealAvg {
			j--
		}

		// skew = actual difference between the domains
		skew := float64(len(sortedDomains[j].pods) - len(sortedDomains[i].pods))

		// if k and j are within the maxSkew of each other, move to next belowOrEqualAvg
		if int32(skew) <= tsc.MaxSkew {
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
		halfSkew := math.Ceil((skew - float64(tsc.MaxSkew)) / 2)
		movePods := int(math.Min(smallestDiff, halfSkew))
		if movePods <= 0 {
			i++
			continue
		}

		// remove pods from the higher topology and add them to the list of pods to be evicted
		// also (just for tracking), add them to the list of pods in the lower topology
		aboveToEvict := sortedDomains[j].pods[len(sortedDomains[j].pods)-movePods:]
		for k := range aboveToEvict {
			// PodFitsAnyOtherNode excludes the current node because, for the sake of domain balancing only, we care about if there is any other
			// place it could theoretically fit.
			// If the pod doesn't fit on its current node, that is a job for RemovePodsViolatingNodeAffinity, and irrelevant to Topology Spreading
			// Also, if the pod has a hard nodeAffinity/nodeSelector/toleration that only matches this node,
			// don't bother evicting it as it will just end up back on the same node
			// however we still account for it "being evicted" so the algorithm can complete
			// TODO(@damemi): Since we don't order pods wrt their affinities, we should refactor this to skip the current pod
			// but still try to get the required # of movePods (instead of just chopping that value off the slice above).
			// In other words, PTS can perform suboptimally if some of its chosen pods don't fit on other nodes.
			// This is because the chosen pods aren't sorted, but immovable pods still count as "evicted" toward the PTS algorithm.
			// So, a better selection heuristic could improve performance.

			if topologyBalanceNodeFit && !node.PodFitsAnyOtherNode(getPodsAssignedToNode, aboveToEvict[k], nodesBelowIdealAvg) {
				klog.V(2).InfoS("ignoring pod for eviction as it does not fit on any other node", "pod", klog.KObj(aboveToEvict[k]))
				continue
			}

			podsForEviction[aboveToEvict[k]] = struct{}{}
		}
		sortedDomains[j].pods = sortedDomains[j].pods[:len(sortedDomains[j].pods)-movePods]
		sortedDomains[i].pods = append(sortedDomains[i].pods, aboveToEvict...)
	}
}

// filterNodesBelowIdealAvg will return nodes that have fewer pods matching topology domain than the idealAvg count.
// the desired behavior is to not consider nodes in a given topology domain that are already packed.
func filterNodesBelowIdealAvg(nodes []*v1.Node, sortedDomains []topology, topologyKey string, idealAvg float64) []*v1.Node {
	topologyNodesMap := make(map[string][]*v1.Node, len(sortedDomains))
	for _, node := range nodes {
		if topologyDomain, ok := node.Labels[topologyKey]; ok {
			topologyNodesMap[topologyDomain] = append(topologyNodesMap[topologyDomain], node)
		}
	}

	var nodesBelowIdealAvg []*v1.Node
	for _, domain := range sortedDomains {
		if float64(len(domain.pods)) < idealAvg {
			nodesBelowIdealAvg = append(nodesBelowIdealAvg, topologyNodesMap[domain.pair.value]...)
		}
	}
	return nodesBelowIdealAvg
}

// sortDomains sorts and splits the list of topology domains based on their size
// it also sorts the list of pods within the domains based on their node affinity/selector and priority in the following order:
// 1. non-evictable pods
// 2. pods with selectors or affinity
// 3. pods in descending priority
// 4. all other pods
// We then pop pods off the back of the list for eviction
func sortDomains(constraintTopologyPairs map[topologyPair][]*v1.Pod, isEvictable func(pod *v1.Pod) bool) []topology {
	sortedTopologies := make([]topology, 0, len(constraintTopologyPairs))
	// sort the topologies and return 2 lists: those <= the average and those > the average (> list inverted)
	for pair, list := range constraintTopologyPairs {
		// Sort the pods within the domain so that the lowest priority pods are considered first for eviction,
		// followed by the highest priority,
		// followed by the lowest priority pods with affinity or nodeSelector,
		// followed by the highest priority pods with affinity or nodeSelector
		sort.Slice(list, func(i, j int) bool {
			// any non-evictable pods should be considered last (ie, first in the list)
			evictableI := isEvictable(list[i])
			evictableJ := isEvictable(list[j])

			if !evictableI || !evictableJ {
				// false - i is the only non-evictable, so return true to put it first
				// true - j is non-evictable, so return false to put j before i
				// if true and both and non-evictable, order doesn't matter
				return !(evictableI && !evictableJ)
			}
			hasSelectorOrAffinityI := hasSelectorOrAffinity(*list[i])
			hasSelectorOrAffinityJ := hasSelectorOrAffinity(*list[j])
			// if both pods have selectors/affinity, compare them by their priority
			if hasSelectorOrAffinityI == hasSelectorOrAffinityJ {
				// Sort by priority in ascending order (lower priority Pods first)
				return !comparePodsByPriority(list[i], list[j])
			}
			return hasSelectorOrAffinityI && !hasSelectorOrAffinityJ
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

// doNotScheduleTaintsFilterFunc returns the filter function that can
// filter out the node taints that reject scheduling Pod on a Node.
func doNotScheduleTaintsFilterFunc() func(t *v1.Taint) bool {
	return func(t *v1.Taint) bool {
		// PodToleratesNodeTaints is only interested in NoSchedule and NoExecute taints.
		return t.Effect == v1.TaintEffectNoSchedule || t.Effect == v1.TaintEffectNoExecute
	}
}

func filterEligibleNodes(nodes []*v1.Node, tsc topologySpreadConstraint) []*v1.Node {
	var eligibleNodes []*v1.Node
	for _, node := range nodes {
		if matchNodeInclusionPolicies(tsc, node) {
			eligibleNodes = append(eligibleNodes, node)
		}
	}
	return eligibleNodes
}

func matchNodeInclusionPolicies(tsc topologySpreadConstraint, node *v1.Node) bool {
	if tsc.NodeAffinityPolicy == v1.NodeInclusionPolicyHonor {
		// We ignore parsing errors here for backwards compatibility.
		if match, _ := tsc.PodNodeAffinity.Match(node); !match {
			return false
		}
	}

	if tsc.NodeTaintsPolicy == v1.NodeInclusionPolicyHonor {
		if _, untolerated := v1helper.FindMatchingUntoleratedTaint(node.Spec.Taints, tsc.PodTolerations, doNotScheduleTaintsFilterFunc()); untolerated {
			return false
		}
	}
	return true
}

// inspired by Scheduler: https://github.com/kubernetes/kubernetes/blob/release-1.28/pkg/scheduler/framework/plugins/podtopologyspread/common.go#L90
func newTopologySpreadConstraint(constraint v1.TopologySpreadConstraint, pod *v1.Pod) (topologySpreadConstraint, error) {
	selector, err := metav1.LabelSelectorAsSelector(constraint.LabelSelector)
	if err != nil {
		return topologySpreadConstraint{}, err
	}

	if len(constraint.MatchLabelKeys) > 0 && pod.Labels != nil {
		matchLabels := make(labels.Set)
		for _, labelKey := range constraint.MatchLabelKeys {
			if value, ok := pod.Labels[labelKey]; ok {
				matchLabels[labelKey] = value
			}
		}
		if len(matchLabels) > 0 {
			selector = mergeLabelSetWithSelector(matchLabels, selector)
		}
	}

	tsc := topologySpreadConstraint{
		MaxSkew:            constraint.MaxSkew,
		TopologyKey:        constraint.TopologyKey,
		Selector:           selector,
		NodeAffinityPolicy: v1.NodeInclusionPolicyHonor,  // If NodeAffinityPolicy is nil, we treat NodeAffinityPolicy as "Honor".
		NodeTaintsPolicy:   v1.NodeInclusionPolicyIgnore, // If NodeTaintsPolicy is nil, we treat NodeTaintsPolicy as "Ignore".
		PodNodeAffinity:    nodeaffinity.GetRequiredNodeAffinity(pod),
		PodTolerations:     pod.Spec.Tolerations,
	}
	if constraint.NodeAffinityPolicy != nil {
		tsc.NodeAffinityPolicy = *constraint.NodeAffinityPolicy
	}
	if constraint.NodeTaintsPolicy != nil {
		tsc.NodeTaintsPolicy = *constraint.NodeTaintsPolicy
	}

	return tsc, nil
}

// Scheduler: https://github.com/kubernetes/kubernetes/blob/release-1.28/pkg/scheduler/framework/plugins/podtopologyspread/common.go#L136
func mergeLabelSetWithSelector(matchLabels labels.Set, s labels.Selector) labels.Selector {
	mergedSelector := labels.SelectorFromSet(matchLabels)

	requirements, ok := s.Requirements()
	if !ok {
		return s
	}

	for _, r := range requirements {
		mergedSelector = mergedSelector.Add(r)
	}

	return mergedSelector
}
