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
	"reflect"
	"sigs.k8s.io/descheduler/pkg/descheduler/strategies/options"
	"sort"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
)

// RemoveDuplicatePods removes the duplicate pods on node. This strategy evicts all duplicate pods on node.
// A pod is said to be a duplicate of other if both of them are from same creator, kind and are within the same
// namespace, and have at least one container with the same image.
// As of now, this strategy won't evict daemonsets, mirror pods, critical pods and pods with local storages.
func RemoveDuplicatePods(
	ctx context.Context,
	client clientset.Interface,
	strategy api.DeschedulerStrategy,
	nodes []*v1.Node,
	opts options.Options,
	podEvictor *evictions.PodEvictor,
) {
	for _, node := range nodes {
		klog.V(1).Infof("Processing node: %#v", node.Name)
		duplicatePods := listDuplicatePodsOnANode(ctx, client, node, strategy, opts)
		for _, pod := range duplicatePods {
			if _, err := podEvictor.EvictPod(ctx, pod, node); err != nil {
				klog.Errorf("Error evicting pod: (%#v)", err)
				break
			}
		}
	}
}

// listDuplicatePodsOnANode lists duplicate pods on a given node.
// It checks for pods which have the same owner and have at least 1 container with the same image spec
func listDuplicatePodsOnANode(ctx context.Context, client clientset.Interface, node *v1.Node, strategy api.DeschedulerStrategy, opts options.Options) []*v1.Pod {
	pods, err := podutil.ListEvictablePodsOnNode(ctx, client, node, opts)
	if err != nil {
		return nil
	}

	duplicatePods := make([]*v1.Pod, 0, len(pods))
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
		if hasExcludedOwnerRefKind(ownerRefList, strategy) {
			continue
		}
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

		// If there have been any other pods with the same first "key", look through all the lists to see if any match
		if existing, ok := duplicateKeysMap[podContainerKeys[0]]; ok {
			for _, keys := range existing {
				if reflect.DeepEqual(keys, podContainerKeys) {
					duplicatePods = append(duplicatePods, pod)
					break
				}
				// Found no matches, add this list of keys to the list of lists that have the same first key
				duplicateKeysMap[podContainerKeys[0]] = append(duplicateKeysMap[podContainerKeys[0]], podContainerKeys)
			}
		} else {
			// This is the first pod we've seen that has this first "key" entry
			duplicateKeysMap[podContainerKeys[0]] = [][]string{podContainerKeys}
		}
	}
	return duplicatePods
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
