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

// RemoveDuplicatePods removes the duplicate pods on node. This strategy evicts duplicate pods on node.
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

	listOfPodsOnNode := map[*v1.Node][]*v1.Pod{}
	ownerKeyCountInCluster := map[string]int32{}
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
		listOfPodsOnNode[node] = pods
		for _, pod := range pods {
			ownerKeyOfPod := getUniqueOwnerKeyOfPod(pod)
			ownerKeyCountInCluster[ownerKeyOfPod] += 1
		}
	}

	for node, pods := range listOfPodsOnNode {
		podsToEvict := make([]*v1.Pod, 0, len(pods))

		// count of pods on node
		ownerKeyCountOnNode := map[string]int32{}
		for _, pod := range pods {
			ownerKeyOfPod := getUniqueOwnerKeyOfPod(pod)
			ownerKeyCountOnNode[ownerKeyOfPod] += 1
		}

		for _, pod := range pods {
			ownerRefList := podutil.OwnerRef(pod)
			if hasExcludedOwnerRefKind(ownerRefList, strategy) || len(ownerRefList) == 0 {
				continue
			}
			ownerKeyOfPod := getUniqueOwnerKeyOfPod(pod)
			countInCluster := ownerKeyCountInCluster[ownerKeyOfPod]
			maxCountOnNode := int32(float64(countInCluster)/float64(len(nodes))) + 1
			if countOnThisNode := ownerKeyCountOnNode[ownerKeyOfPod]; countOnThisNode > 1 &&
				countOnThisNode >= maxCountOnNode {
				podsToEvict = append(podsToEvict, pod)
				ownerKeyCountOnNode[ownerKeyOfPod]--
			}
		}

		for _, pod := range podsToEvict {
			if _, err := podEvictor.EvictPod(ctx, pod, node, "RemoveDuplicatePods"); err != nil {
				klog.ErrorS(err, "Error evicting pod", "pod", klog.KObj(pod))
				break
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

func getUniqueOwnerKeyOfPod(pod *v1.Pod) string {
	ownerRefList := podutil.OwnerRef(pod)
	podContainerKeys := make([]string, 0, len(ownerRefList)*len(pod.Spec.Containers))
	for _, ownerRef := range ownerRefList {
		for _, container := range pod.Spec.Containers {
			// Namespace/Kind/Name should be unique for the cluster.
			// We also consider the image, as 2 pods could have the same owner but serve different purposes
			// So any non-unique Namespace/Kind/Name/Image pattern is a duplicate pod.
			s := strings.Join([]string{pod.ObjectMeta.Namespace, ownerRef.Kind, ownerRef.Name, container.Image}, "/")
			podContainerKeys = append(podContainerKeys, s)
		}
	}
	sort.Strings(podContainerKeys)
	// Each pod has a list of owners and a list of containers, and each container has 1 image spec.
	return podContainerKeys[0]
}
