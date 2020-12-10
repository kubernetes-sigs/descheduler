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
	"context"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/utils"
)

func validateRemoveDuplicatePodsParams(params *api.StrategyParameters) error {
	if params == nil {
		return nil
	}
	// At most one of include/exclude can be set
	if params.Namespaces != nil && len(params.Namespaces.Include) > 0 && len(params.Namespaces.Exclude) > 0 {
		return fmt.Errorf("only one of Include/Exclude namespaces can be set")
	}
	if params.ThresholdPriority != nil && params.ThresholdPriorityClassName != "" {
		return fmt.Errorf("only one of thresholdPriority and thresholdPriorityClassName can be set")
	}

	return nil
}

type podOwner struct {
	namespace, kind, name string
	imagesHash            string
}

// RemoveDuplicatePods removes the duplicate pods on node. This strategy evicts all duplicate pods on node.
// A pod is said to be a duplicate of other if both of them are from same creator, kind and are within the same
// namespace, and have at least one container with the same image.
// As of now, this strategy won't evict daemonsets, mirror pods, critical pods and pods with local storages.
func RemoveDuplicatePods(
	ctx context.Context,
	client clientset.Interface,
	strategy api.DeschedulerStrategy,
	nodes []*v1.Node,
	podEvictor *evictions.PodEvictor,
) {
	if err := validateRemoveDuplicatePodsParams(strategy.Params); err != nil {
		klog.ErrorS(err, "Invalid RemoveDuplicatePods parameters")
		return
	}
	thresholdPriority, err := utils.GetPriorityFromStrategyParams(ctx, client, strategy.Params)
	if err != nil {
		klog.ErrorS(err, "Failed to get threshold priority from strategy's params")
		return
	}

	var includedNamespaces, excludedNamespaces []string
	if strategy.Params != nil && strategy.Params.Namespaces != nil {
		includedNamespaces = strategy.Params.Namespaces.Include
		excludedNamespaces = strategy.Params.Namespaces.Exclude
	}

	evictable := podEvictor.Evictable(evictions.WithPriorityThreshold(thresholdPriority))

	duplicatePods := make(map[podOwner]map[string][]*v1.Pod)
	ownerKeyOccurence := make(map[podOwner]int32)
	nodeCount := 0
	nodeMap := make(map[string]*v1.Node)

	for _, node := range nodes {
		klog.V(1).InfoS("Processing node", "node", klog.KObj(node))
		pods, err := podutil.ListPodsOnANode(ctx,
			client,
			node,
			podutil.WithFilter(evictable.IsEvictable),
			podutil.WithNamespaces(includedNamespaces),
			podutil.WithoutNamespaces(excludedNamespaces),
		)
		if err != nil {
			klog.ErrorS(err, "Error listing evictable pods on node", "node", klog.KObj(node))
			continue
		}
		nodeMap[node.Name] = node
		nodeCount++
		// Each pod has a list of owners and a list of containers, and each container has 1 image spec.
		// For each pod, we go through all the OwnerRef/Image mappings and represent them as a "key" string.
		// All of those mappings together makes a list of "key" strings that essentially represent that pod's uniqueness.
		// This list of keys representing a single pod is then sorted alphabetically.
		// If any other pod has a list that matches that pod's list, those pods are undeniably duplicates for the following reasons:
		//   - The 2 pods have the exact same ownerrefs
		//   - The 2 pods have the exact same container images
		//
		// duplicateKeysMap maps the first Namespace/Kind/Name/Image in a pod's list to a 2D-slice of all the other lists where that is the first key
		// (Since we sort each pod's list, we only need to key the map on the first entry in each list. Any pod that doesn't have
		// the same first entry is clearly not a duplicate. This makes lookup quick and minimizes storage needed).
		// If any of the existing lists for that first key matches the current pod's list, the current pod is a duplicate.
		// If not, then we add this pod's list to the list of lists for that key.
		duplicateKeysMap := map[string][][]string{}
		for _, pod := range pods {
			ownerRefList := podutil.OwnerRef(pod)
			if hasExcludedOwnerRefKind(ownerRefList, strategy) || len(ownerRefList) == 0 {
				continue
			}
			podContainerKeys := make([]string, 0, len(ownerRefList)*len(pod.Spec.Containers))
			imageList := []string{}
			for _, container := range pod.Spec.Containers {
				imageList = append(imageList, container.Image)
			}
			sort.Strings(imageList)
			imagesHash := strings.Join(imageList, "#")
			for _, ownerRef := range ownerRefList {
				ownerKey := podOwner{
					namespace:  pod.ObjectMeta.Namespace,
					kind:       ownerRef.Kind,
					name:       ownerRef.Name,
					imagesHash: imagesHash,
				}
				ownerKeyOccurence[ownerKey] = ownerKeyOccurence[ownerKey] + 1
				for _, image := range imageList {
					// Namespace/Kind/Name should be unique for the cluster.
					// We also consider the image, as 2 pods could have the same owner but serve different purposes
					// So any non-unique Namespace/Kind/Name/Image pattern is a duplicate pod.
					s := strings.Join([]string{pod.ObjectMeta.Namespace, ownerRef.Kind, ownerRef.Name, image}, "/")
					podContainerKeys = append(podContainerKeys, s)
				}
			}
			sort.Strings(podContainerKeys)

			// If there have been any other pods with the same first "key", look through all the lists to see if any match
			if existing, ok := duplicateKeysMap[podContainerKeys[0]]; ok {
				matched := false
				for _, keys := range existing {
					if reflect.DeepEqual(keys, podContainerKeys) {
						matched = true
						klog.V(3).InfoS("Duplicate found", "pod", klog.KObj(pod))
						for _, ownerRef := range ownerRefList {
							ownerKey := podOwner{
								namespace:  pod.ObjectMeta.Namespace,
								kind:       ownerRef.Kind,
								name:       ownerRef.Name,
								imagesHash: imagesHash,
							}
							if _, ok := duplicatePods[ownerKey]; !ok {
								duplicatePods[ownerKey] = make(map[string][]*v1.Pod)
							}
							duplicatePods[ownerKey][node.Name] = append(duplicatePods[ownerKey][node.Name], pod)
						}
						break
					}
				}
				if !matched {
					// Found no matches, add this list of keys to the list of lists that have the same first key
					duplicateKeysMap[podContainerKeys[0]] = append(duplicateKeysMap[podContainerKeys[0]], podContainerKeys)
				}
			} else {
				// This is the first pod we've seen that has this first "key" entry
				duplicateKeysMap[podContainerKeys[0]] = [][]string{podContainerKeys}
			}
		}
	}

	// 1. how many pods can be evicted to respect uniform placement of pods among viable nodes?
	for ownerKey, nodes := range duplicatePods {
		upperAvg := int(math.Ceil(float64(ownerKeyOccurence[ownerKey]) / float64(nodeCount)))
		for nodeName, pods := range nodes {
			klog.V(2).InfoS("Average occurrence per node", "node", klog.KObj(nodeMap[nodeName]), "ownerKey", ownerKey, "avg", upperAvg)
			// list of duplicated pods does not contain the original referential pod
			if len(pods)+1 > upperAvg {
				// It's assumed all duplicated pods are in the same priority class
				// TODO(jchaloup): check if the pod has a different node to lend to
				for _, pod := range pods[upperAvg-1:] {
					if _, err := podEvictor.EvictPod(ctx, pod, nodeMap[nodeName], "RemoveDuplicatePods"); err != nil {
						klog.ErrorS(err, "Error evicting pod", "pod", klog.KObj(pod))
						break
					}
				}
			}
		}
	}
}

func hasExcludedOwnerRefKind(ownerRefs []metav1.OwnerReference, strategy api.DeschedulerStrategy) bool {
	if strategy.Params == nil || strategy.Params.RemoveDuplicates == nil {
		return false
	}
	exclude := sets.NewString(strategy.Params.RemoveDuplicates.ExcludeOwnerKinds...)
	for _, owner := range ownerRefs {
		if exclude.Has(owner.Kind) {
			return true
		}
	}
	return false
}
