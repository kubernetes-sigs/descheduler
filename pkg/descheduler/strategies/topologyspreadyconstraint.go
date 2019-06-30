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
	"math"

	"github.com/golang/glog"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	clientset "k8s.io/client-go/kubernetes"

	"github.com/kubernetes-incubator/descheduler/cmd/descheduler/app/options"
	"github.com/kubernetes-incubator/descheduler/pkg/api"
	"github.com/kubernetes-incubator/descheduler/pkg/descheduler/evictions"
	podutil "github.com/kubernetes-incubator/descheduler/pkg/descheduler/pod"
)

func TopologySpreadConstraint(ds *options.DeschedulerServer, strategy api.DeschedulerStrategy, policyGroupVersion string, nodes []*v1.Node, nodePodCount nodePodEvictedCount) {
	if !strategy.Enabled {
		return
	}

	glog.Infof("Found following parameters for TopologySpreadConstraint %v\n", strategy)
	evictPodsViolatingSpreadConstraints(ds.Client, policyGroupVersion, nodes, ds.DryRun, nodePodCount, strategy.Params.NamespacedTopologySpreadConstraints)

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

// finnd all nodes
// find all pods on the nodes
// find the pods that match the namespace
// find the constraints topokey , use this topokey to finds the node topokey value
// add the pod with key as the combination of topokey and its value
// find the min value of pods with any key
// iterate through all keys and diff currentPods -minPods <=maxSkew
// if diff > maxSkew, select a pod in the current bucket for eviction
//
func evictPodsViolatingSpreadConstraints(
	client clientset.Interface,
	policyGroupVersion string,
	nodes []*v1.Node, dryRun bool,
	nodePodCount nodePodEvictedCount,
	namespacedTopologySpreadConstraints []api.NamespacedTopologySpreadConstraint,
) {

	namespaceToTopologyKeySet := make(map[string]topologyKeyToTopologyPairSetMap)

	for _, namespacedConstraint := range namespacedTopologySpreadConstraints {
		if namespaceToTopologyKeySet[namespacedConstraint.Namespace] == nil {
			namespaceToTopologyKeySet[namespacedConstraint.Namespace] = make(topologyKeyToTopologyPairSetMap)
		}
		for _, topoConstraint := range namespacedConstraint.TopologySpreadConstraints {
			if namespaceToTopologyKeySet[namespacedConstraint.Namespace][topoConstraint.TopologyKey] == nil {
				namespaceToTopologyKeySet[namespacedConstraint.Namespace][topoConstraint.TopologyKey] = make(topologyPairToPodSetMap)
			}
			for _, node := range nodes {
				pair := topologyPair{key: topoConstraint.TopologyKey, value: node.Labels[topoConstraint.TopologyKey]}
				if namespaceToTopologyKeySet[namespacedConstraint.Namespace][topoConstraint.TopologyKey][pair] == nil {
					// this ensures that nodes which match topokey but no pods are accounted for
					namespaceToTopologyKeySet[namespacedConstraint.Namespace][topoConstraint.TopologyKey][pair] = make(podSet)
				}
				glog.V(1).Infof("Processing node: %#v\n", node.Name)
				pods, err := podutil.ListEvictablePodsOnNodeByNamespace(client, node, false, namespacedConstraint.Namespace)
				if err != nil || len(pods) == 0 {
					glog.V(1).Infof("No Evictable pods found for Node %v", node.Name)
					continue
				}

				for _, pod := range pods {
					glog.V(1).Infof("Processing pod %v", pod.Name)
					// does this pod labels match the constraint label selector
					selector, err := metav1.LabelSelectorAsSelector(topoConstraint.LabelSelector)
					if err != nil {
						glog.V(2).Infof("No Evictable pods found for Node %v matching the Topology Selector", node.Name)
						continue
					}
					if !selector.Matches(labels.Set(pod.Labels)) {
						glog.V(2).Infof("No Evictable pods found for Node %v matching the Topology Selector", node.Name)
						continue
					}
					glog.V(1).Infof("Pod %v matched labels", pod.Name)
					// TODO: Need to determine if the topokey already present in the node or not
					pair := topologyPair{key: topoConstraint.TopologyKey, value: node.Labels[topoConstraint.TopologyKey]}
					namespaceToTopologyKeySet[namespacedConstraint.Namespace][topoConstraint.TopologyKey][pair][pod] = struct{}{}
					glog.V(1).Infof("counting for pair %v", pair)
					glog.V(1).Infof("length %v", len(namespaceToTopologyKeySet[namespacedConstraint.Namespace][topoConstraint.TopologyKey][pair]))

				}
			}
		}
	}
	finalPodsToEvict := []*v1.Pod{}
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
				glog.V(1).Infof("Pods in Topo %v for pair %v", podsInTopo, pair)
				glog.V(1).Infof("Min Pods in Topo %v", minPodsForGivenTopo)

				if int32(podsInTopo-minPodsForGivenTopo) > topoConstraint.MaxSkew {
					countToEvict := int32(podsInTopo-minPodsForGivenTopo) - topoConstraint.MaxSkew
					glog.V(1).Infof("Count to evict %v", countToEvict)
					podsListToEvict := GetPodsToEvict(countToEvict, v)
					allPodsToEvictPerTopoKey[topoConstraint.TopologyKey] = append(allPodsToEvictPerTopoKey[topoConstraint.TopologyKey], podsListToEvict...)
				}
			}

		}

		//we have n lists of pods to evict, where n is same as number of topology constraints per namespace
		//intersect all n lists to produce a list of pods which needs to be evicted
		finalPodsToEvict = intersectAllPodsList(allPodsToEvictPerTopoKey)

	}

	for _, podToEvict := range finalPodsToEvict {
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

func GetPodsToEvict(countToEvict int32, podMap map[*v1.Pod]struct{}) []*v1.Pod {
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

// GetPodFullName returns a name that uniquely identifies a pod.
func GetPodFullName(pod *v1.Pod) string {
	// Use underscore as the delimiter because it is not allowed in pod name
	// (DNS subdomain format).
	return pod.Name + "_" + pod.Namespace
}
