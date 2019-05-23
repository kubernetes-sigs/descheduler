/*
Copyright 2017 The Kubernetes Authors.

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
	"fmt"
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

	fmt.Printf("Found following parameters for TopologySpreadConstraint %v\n", strategy)
	for _, topoConstraints := range strategy.Params.NamespacedTopologySpreadConstraints {
		glog.Infof("Found Topo Constraints %v\n", topoConstraints)
		evictPodsViolatingSpreadConstraints(ds.Client, policyGroupVersion, nodes, ds.DryRun, nodePodCount, topoConstraints)
	}

}

// AntiAffinityTerm's topology key value used in predicate metadata
type topologyPair struct {
	key   string
	value string
}

type podSet map[*v1.Pod]struct{}

type topologyPairSet map[topologyPair]struct{}

// finnd all nodes
// find all pods on the nodes
// find the pods that match the namespace
// find the constraints topokey , use this topokey to finds the node topokey value
// add the pod with key as the combination of topokey and its value
// find the min value of pods with any key
// iterate through all keys and diff currentPods -minPods <=maxSkew
// if diff > maxSkew, select a pod in the current bucket for eviction
//
func evictPodsViolatingSpreadConstraints(client clientset.Interface, policyGroupVersion string, nodes []*v1.Node, dryRun bool, nodePodCount nodePodEvictedCount,
	constraint api.NamespacedTopologySpreadConstraint) {

	if len(constraint.TopologySpreadConstraints) != 1 {
		glog.V(1).Infof("We currently only support 1 topology spread constraint per namespace")
		return
	}
	topologyPairToPods := make(map[topologyPair]podSet)
	for _, node := range nodes {
		glog.V(1).Infof("Processing node: %#v\n", node.Name)
		pods, err := podutil.ListEvictablePodsOnNode(client, node, false)
		if err != nil {
		}
		for _, pod := range pods {
			if pod.Namespace != constraint.Namespace {
				continue
			}
			// does this pod labels match the constraint label selector
			// TODO: This is intentional that it only looks at the first constraint
			selector, err := metav1.LabelSelectorAsSelector(constraint.TopologySpreadConstraints[0].LabelSelector)
			if err != nil {
				continue
			}
			if !selector.Matches(labels.Set(pod.Labels)) {
				continue
			}
			// TODO: Need to determine if the topokey already present in the node or not
			pair := topologyPair{key: constraint.TopologySpreadConstraints[0].TopologyKey, value: node.Labels[constraint.TopologySpreadConstraints[0].TopologyKey]}
			addTopologyPair(topologyPairToPods, pair, pod)
		}
	}

	minPodsForGivenTopo := math.MaxInt32
	for _, v := range topologyPairToPods {
		if len(v) < minPodsForGivenTopo {
			minPodsForGivenTopo = len(v)
		}
	}

	for _, v := range topologyPairToPods {
		podsInTopo := len(v)
		if int32(podsInTopo-minPodsForGivenTopo) >= constraint.TopologySpreadConstraints[0].MaxSkew {
			//we need to evict maxSkew-(podsInTopo-minPodsForGivenTopo))
			countToEvict := constraint.TopologySpreadConstraints[0].MaxSkew - int32(podsInTopo-minPodsForGivenTopo)
			podsListToEvict := GetPodsToEvict(countToEvict, v)
			for _, podToEvict := range podsListToEvict {
				success, err := evictions.EvictPod(client, podToEvict, policyGroupVersion, dryRun)
				if !success {
					glog.Infof("Error when evicting pod: %#v (%#v)\n", podToEvict.Name, err)
				} else {

				}
			}
		}
	}
}

func GetPodsToEvict(countToEvict int32, podMap map[*v1.Pod]struct{}) []*v1.Pod {
	count := int32(0)
	podList := []*v1.Pod{}
	for k := range podMap {
		podList = append(podList, k)
		count++
		if count == countToEvict {
			break
		}
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
