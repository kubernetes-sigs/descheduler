/*
Copyright 2019 The Kubernetes Authors.

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
	"math"

	"github.com/golang/glog"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	clientset "k8s.io/client-go/kubernetes"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
)

func TopologySpreadConstraint(ctx context.Context, client clientset.Interface, strategy api.DeschedulerStrategy, nodes []*v1.Node, evictLocalStoragePods bool, podEvictor *evictions.PodEvictor) {
	if !strategy.Enabled {
		return
	}

	glog.Infof("Found following parameters for TopologySpreadConstraint %v", strategy)
	//evictPodsViolatingSpreadConstraints(ds.Client, policyGroupVersion, nodes, ds.DryRun, nodePodCount, strategy.Params.NamespacedTopologySpreadConstraints)

}

// AntiAffinityTerm's topology key value used in predicate metadata
type topologyPair struct {
	key   string
	value string
}

type podSet map[*v1.Pod]struct{}

// for each topology pair, what is the set of pods
type topologyPairToPodSetMap map[topologyPair]podSet

// for each topologyKey, what is the map of topologyKey pairs to pods
type topologyKeyToTopologyPairSetMap map[string]topologyPairToPodSetMap

// Create a map of Node Name to v1.Node
// for each namespace for which there is Topology Constraint
// for each TopologySpreadyConstraint in that namespace
// find all evictable pods in that namespace
// for each evictable pod in that namespace
// If the pod matches this TopologySpreadConstraint LabelSelector
// If the pod nodeName is present in the nodeMap
// create a topoPair with key as this TopologySpreadConstraint.TopologyKey and value as this pod's Node Label Value for this TopologyKey
// add the pod with key as this topoPair
// find the min number of pods in any topoPair for this topologyKey
// iterate through all topoPairs for this topologyKey and diff currentPods -minPods <=maxSkew
// if diff > maxSkew, add this pod in the current bucket for eviction

// We get N podLists , one for each TopologyKey in a given Namespace
// Find the pods which are common to each of these podLists
// Evict these Pods
// TODO: Break this down into UT'able functions
func evictPodsViolatingSpreadConstraints(
	client clientset.Interface,
	policyGroupVersion string,
	nodes []*v1.Node, dryRun bool,
	nodePodCount nodePodEvictedCount,
	namespacedTopologySpreadConstraints []api.NamespacedTopologySpreadConstraint,
) {

	namespaceToTopologyKeySet := make(map[string]topologyKeyToTopologyPairSetMap)

	// create a node map matching nodeName to v1.Node
	nodeMap := make(map[string]*v1.Node)
	for _, node := range nodes {
		nodeMap[node.Name] = node
	}
	for _, namespacedConstraint := range namespacedTopologySpreadConstraints {
		if namespaceToTopologyKeySet[namespacedConstraint.Namespace] == nil {
			namespaceToTopologyKeySet[namespacedConstraint.Namespace] = make(topologyKeyToTopologyPairSetMap)
		}
		for _, topoConstraint := range namespacedConstraint.TopologySpreadConstraints {
			if namespaceToTopologyKeySet[namespacedConstraint.Namespace][topoConstraint.TopologyKey] == nil {
				namespaceToTopologyKeySet[namespacedConstraint.Namespace][topoConstraint.TopologyKey] = make(topologyPairToPodSetMap)
			}
			for _, node := range nodes {
				if node.Labels[topoConstraint.TopologyKey] == "" {
					continue
				}
				pair := topologyPair{key: topoConstraint.TopologyKey, value: node.Labels[topoConstraint.TopologyKey]}
				if namespaceToTopologyKeySet[namespacedConstraint.Namespace][topoConstraint.TopologyKey][pair] == nil {
					// this ensures that nodes which match topokey but no pods are accounted for
					namespaceToTopologyKeySet[namespacedConstraint.Namespace][topoConstraint.TopologyKey][pair] = make(podSet)
				}
			}
			pods, err := podutil.ListEvictablePodsByNamespace(client, false, namespacedConstraint.Namespace)
			if err != nil || len(pods) == 0 {
				glog.V(1).Infof("No Evictable pods found for Namespace %v", namespacedConstraint.Namespace)
				continue
			}

			for _, pod := range pods {
				glog.V(2).Infof("Processing pod %v", pod.Name)
				// does this pod labels match the constraint label selector
				selector, err := metav1.LabelSelectorAsSelector(topoConstraint.LabelSelector)
				if err != nil {
					glog.V(2).Infof("Pod Labels dont match for %v", pod.Name)
					continue
				}
				if !selector.Matches(labels.Set(pod.Labels)) {
					glog.V(2).Infof("Pod Labels dont match for %v", pod.Name)
					continue
				}
				glog.V(1).Infof("Pod %v matched labels", pod.Name)
				// TODO: Need to determine if the topokey already present in the node or not
				if pod.Spec.NodeName == "" {
					continue
				}
				// see of this pods NodeName exists in the candidates nodes, else ignore
				_, ok := nodeMap[pod.Spec.NodeName]
				if !ok {
					glog.V(2).Infof("Found a node %v in pod %v, which is not present in our map, ignoring it...", pod.Spec.NodeName, pod.Name)
					continue
				}
				pair := topologyPair{key: topoConstraint.TopologyKey, value: nodeMap[pod.Spec.NodeName].Labels[topoConstraint.TopologyKey]}
				if namespaceToTopologyKeySet[namespacedConstraint.Namespace][topoConstraint.TopologyKey][pair] == nil {
					// this ensures that nodes which match topokey but no pods are accounted for
					namespaceToTopologyKeySet[namespacedConstraint.Namespace][topoConstraint.TopologyKey][pair] = make(podSet)
				}
				namespaceToTopologyKeySet[namespacedConstraint.Namespace][topoConstraint.TopologyKey][pair][pod] = struct{}{}
				glog.V(2).Infof("Topo Pair %v, Count %v", pair, len(namespaceToTopologyKeySet[namespacedConstraint.Namespace][topoConstraint.TopologyKey][pair]))

			}
		}
	}

	// finalPodsToEvict := []*v1.Pod{}
	for _, namespacedConstraint := range namespacedTopologySpreadConstraints {
		allPodsToEvictPerTopoKey := make(map[string][]*v1.Pod)
		for _, topoConstraint := range namespacedConstraint.TopologySpreadConstraints {
			minPodsForGivenTopo := math.MaxInt32
			for _, v := range namespaceToTopologyKeySet[namespacedConstraint.Namespace][topoConstraint.TopologyKey] {
				if len(v) < minPodsForGivenTopo {
					minPodsForGivenTopo = len(v)
				}
			}

			topologyPairToPods := namespaceToTopologyKeySet[namespacedConstraint.Namespace][topoConstraint.TopologyKey]
			for pair, v := range topologyPairToPods {
				podsInTopo := len(v)
				glog.V(1).Infof("Min Pods in Any Pair %v, pair %v, PodCount %v", minPodsForGivenTopo, pair, podsInTopo)

				if int32(podsInTopo-minPodsForGivenTopo) > topoConstraint.MaxSkew {
					countToEvict := int32(podsInTopo-minPodsForGivenTopo) - topoConstraint.MaxSkew
					glog.V(1).Infof("pair %v, Count to evict %v", pair, countToEvict)
					podsListToEvict := getPodsToEvict(countToEvict, v)
					allPodsToEvictPerTopoKey[topoConstraint.TopologyKey] = append(allPodsToEvictPerTopoKey[topoConstraint.TopologyKey], podsListToEvict...)

				}
			}

		}

		// TODO: Sometimes we will have hierarchical TopoKeys, like a Building has Rooms and Rooms have Racks
		// Our Current Definition of TopologySpreadConstraint Doesnt allow you to capture that Constraint
		// If we could capture that Hierarchy, I would do the following:-
		// - Create a List of Pods to Evict per TopologyKey
		// - Take intersection of all lists to produce a list of pods to evict
		// This is because in hierarchical topologyKeys, if we make an indepdent decision of evicting only by
		// Rack, but didnt consider the Room spreading at all,we might mess up the Room Spreading. This is too
		// constrained though since, if we consider an intersection of all hierarchies, we would not even balance
		// properly. So we would need to define some sorta importance of which topologyKey has what weight, etc
		// finalPodsToEvict = intersectAllPodsList(allPodsToEvictPerTopoKey)

		// defer the decision as late as possible to cause less schedulings
		for topoKey, podList := range allPodsToEvictPerTopoKey {
			glog.V(1).Infof("Total pods to evict in TopoKey %v is %v", topoKey, len(podList))
			evictPodsSimple(client, podList, policyGroupVersion, dryRun)
		}
	}

}

func evictPodsSimple(client clientset.Interface, podsListToEvict []*v1.Pod, policyGroupVersion string, dryRun bool) {
	for _, podToEvict := range podsListToEvict {
		glog.V(1).Infof("Evicting pods %v", podToEvict.Name)
		success, err := evictions.EvictPod(client, podToEvict, policyGroupVersion, dryRun)
		if !success {
			glog.Infof("Error when evicting pod: %#v (%#v)\n", podToEvict.Name, err)
		} else {

		}
	}
}

func intersectAllPodsList(allPodsToEvictPerTopoKey map[string][]*v1.Pod) []*v1.Pod {
	// increment each pod's count by 1
	// if the pod count reaches the number of topoKeys, it should be evicted
	perPodCount := make(map[string]int)

	finalList := []*v1.Pod{}
	totalTopoKeys := len(allPodsToEvictPerTopoKey)
	glog.V(1).Infof("Total topokeys found %v", totalTopoKeys)
	for _, podList := range allPodsToEvictPerTopoKey {
		for _, pod := range podList {
			key := pod.Name + "-" + pod.Namespace
			perPodCount[key] = perPodCount[key] + 1
			if perPodCount[key] == len(allPodsToEvictPerTopoKey) {
				finalList = append(finalList, pod)
			}
		}
	}
	return finalList
}

func getPodsToEvict(countToEvict int32, podMap map[*v1.Pod]struct{}) []*v1.Pod {
	count := int32(0)
	podList := []*v1.Pod{}
	for k := range podMap {
		if count == countToEvict {
			break
		}
		podList = append(podList, k)
		count++
	}

	return podList
}

func addTopologyPair(topoMap map[topologyPair]podSet, pair topologyPair, pod *v1.Pod) {
	if topoMap[pair] == nil {
		topoMap[pair] = make(map[*v1.Pod]struct{})
	}
	topoMap[pair][pod] = struct{}{}
}
