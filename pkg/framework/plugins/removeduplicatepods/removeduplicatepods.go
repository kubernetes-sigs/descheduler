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

package removeduplicatepods

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/framework"
	"sigs.k8s.io/descheduler/pkg/utils"
)

const PluginName = "RemoveDuplicatePods"

// RemoveDuplicatePods removes the duplicate pods on node. This strategy evicts all duplicate pods on node.
// A pod is said to be a duplicate of other if both of them are from same creator, kind and are within the same
// namespace, and have at least one container with the same image.
// As of now, this strategy won't evict daemonsets, mirror pods, critical pods and pods with local storages.
type RemoveDuplicatePods struct {
	handle            framework.Handle
	args              *framework.RemoveDuplicatePodsArg
	excludeOwnerKinds []string
	podFilter         podutil.FilterFunc
}

var _ framework.Plugin = &RemoveDuplicatePods{}
var _ framework.DeschedulePlugin = &RemoveDuplicatePods{}

func New(args runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	duplicatesArg, ok := args.(*framework.RemoveDuplicatePodsArg)
	if !ok {
		return nil, fmt.Errorf("want args to be of type RemoveDuplicatePodsArg, got %T", args)
	}

	// At most one of include/exclude can be set
	if duplicatesArg.Namespaces != nil && len(duplicatesArg.Namespaces.Include) > 0 && len(duplicatesArg.Namespaces.Exclude) > 0 {
		return nil, fmt.Errorf("only one of Include/Exclude namespaces can be set")
	}
	if duplicatesArg.PriorityThreshold != nil && duplicatesArg.PriorityThreshold.Value != nil && duplicatesArg.PriorityThreshold.Name != "" {
		return nil, fmt.Errorf("only one of priorityThreshold fields can be set")
	}

	thresholdPriority, err := utils.GetPriorityValueFromPriorityThreshold(context.TODO(), handle.ClientSet(), duplicatesArg.PriorityThreshold)
	if err != nil {
		return nil, fmt.Errorf("failed to get priority threshold: %v", err)
	}

	evictable := handle.PodEvictor().Evictable(
		evictions.WithPriorityThreshold(thresholdPriority),
		evictions.WithNodeFit(duplicatesArg.NodeFit),
	)

	var includedNamespaces, excludedNamespaces sets.String
	if duplicatesArg.Namespaces != nil {
		includedNamespaces = sets.NewString(duplicatesArg.Namespaces.Include...)
		excludedNamespaces = sets.NewString(duplicatesArg.Namespaces.Exclude...)
	}

	podFilter, err := podutil.NewOptions().
		WithFilter(evictable.IsEvictable).
		WithNamespaces(includedNamespaces).
		WithoutNamespaces(excludedNamespaces).
		BuildFilterFunc()
	if err != nil {
		return nil, fmt.Errorf("error initializing pod filter function: %v", err)
	}

	return &RemoveDuplicatePods{
		handle:            handle,
		excludeOwnerKinds: duplicatesArg.ExcludeOwnerKinds,
		podFilter:         podFilter,
	}, nil
}

func (d *RemoveDuplicatePods) Name() string {
	return PluginName
}

type podOwner struct {
	namespace, kind, name string
	imagesHash            string
}

func (d *RemoveDuplicatePods) Deschedule(ctx context.Context, nodes []*v1.Node) *framework.Status {
	duplicatePods := make(map[podOwner]map[string][]*v1.Pod)
	ownerKeyOccurence := make(map[podOwner]int32)
	nodeCount := 0
	nodeMap := make(map[string]*v1.Node)

	for _, node := range nodes {
		klog.V(1).InfoS("Processing node", "node", klog.KObj(node))
		pods, err := podutil.ListPodsOnANode(node.Name, d.handle.GetPodsAssignedToNodeFunc(), d.podFilter)
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
			if len(ownerRefList) == 0 || hasExcludedOwnerRefKind(ownerRefList, d.excludeOwnerKinds) {
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
	for ownerKey, podNodes := range duplicatePods {
		targetNodes := getTargetNodes(podNodes, nodes)

		klog.V(2).InfoS("Adjusting feasible nodes", "owner", ownerKey, "from", nodeCount, "to", len(targetNodes))
		if len(targetNodes) < 2 {
			klog.V(1).InfoS("Less than two feasible nodes for duplicates to land, skipping eviction", "owner", ownerKey)
			continue
		}

		upperAvg := int(math.Ceil(float64(ownerKeyOccurence[ownerKey]) / float64(len(targetNodes))))
		for nodeName, pods := range podNodes {
			klog.V(2).InfoS("Average occurrence per node", "node", klog.KObj(nodeMap[nodeName]), "ownerKey", ownerKey, "avg", upperAvg)
			// list of duplicated pods does not contain the original referential pod
			if len(pods)+1 > upperAvg {
				// It's assumed all duplicated pods are in the same priority class
				// TODO(jchaloup): check if the pod has a different node to lend to
				for _, pod := range pods[upperAvg-1:] {
					if _, err := d.handle.PodEvictor().EvictPod(ctx, pod, nodeMap[nodeName], "RemoveDuplicatePods"); err != nil {
						klog.ErrorS(err, "Error evicting pod", "pod", klog.KObj(pod))
						break
					}
				}
			}
		}
	}

	return nil
}

func getNodeAffinityNodeSelector(pod *v1.Pod) *v1.NodeSelector {
	if pod.Spec.Affinity == nil {
		return nil
	}
	if pod.Spec.Affinity.NodeAffinity == nil {
		return nil
	}
	return pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
}

func getTargetNodes(podNodes map[string][]*v1.Pod, nodes []*v1.Node) []*v1.Node {
	// In order to reduce the number of pods processed, identify pods which have
	// equal (tolerations, nodeselectors, node affinity) terms and considered them
	// as identical. Identical pods wrt. (tolerations, nodeselectors, node affinity) terms
	// will produce the same result when checking if a pod is feasible for a node.
	// Thus, improving efficiency of processing pods marked for eviction.

	// Collect all distinct pods which differ in at least taints, node affinity or node selector terms
	distinctPods := map[*v1.Pod]struct{}{}
	for _, pods := range podNodes {
		for _, pod := range pods {
			duplicated := false
			for dp := range distinctPods {
				if utils.TolerationsEqual(pod.Spec.Tolerations, dp.Spec.Tolerations) &&
					utils.NodeSelectorsEqual(getNodeAffinityNodeSelector(pod), getNodeAffinityNodeSelector(dp)) &&
					reflect.DeepEqual(pod.Spec.NodeSelector, dp.Spec.NodeSelector) {
					duplicated = true
					continue
				}
			}
			if duplicated {
				continue
			}
			distinctPods[pod] = struct{}{}
		}
	}

	// For each distinct pod get a list of nodes where it can land
	targetNodesMap := map[string]*v1.Node{}
	for pod := range distinctPods {
		matchingNodes := map[string]*v1.Node{}
		for _, node := range nodes {
			if !utils.TolerationsTolerateTaintsWithFilter(pod.Spec.Tolerations, node.Spec.Taints, func(taint *v1.Taint) bool {
				return taint.Effect == v1.TaintEffectNoSchedule || taint.Effect == v1.TaintEffectNoExecute
			}) {
				continue
			}
			if match, err := utils.PodMatchNodeSelector(pod, node); err == nil && !match {
				continue
			}
			matchingNodes[node.Name] = node
		}
		if len(matchingNodes) > 1 {
			for nodeName := range matchingNodes {
				targetNodesMap[nodeName] = matchingNodes[nodeName]
			}
		}
	}

	targetNodes := []*v1.Node{}
	for _, node := range targetNodesMap {
		targetNodes = append(targetNodes, node)
	}

	return targetNodes
}

func hasExcludedOwnerRefKind(ownerRefs []metav1.OwnerReference, ExcludeOwnerKinds []string) bool {
	if len(ExcludeOwnerKinds) == 0 {
		return false
	}

	exclude := sets.NewString(ExcludeOwnerKinds...)
	for _, owner := range ownerRefs {
		if exclude.Has(owner.Kind) {
			return true
		}
	}
	return false
}
