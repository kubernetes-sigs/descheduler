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
	"strings"

	"k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
)

// RemoveDuplicatePods removes the duplicate pods on node. This strategy evicts all duplicate pods on node.
// A pod is said to be a duplicate of other if both of them are from same creator, kind and are within the same
// namespace. As of now, this strategy won't evict daemonsets, mirror pods, critical pods and pods with local storages.
func RemoveDuplicatePods(
	client clientset.Interface,
	strategy api.DeschedulerStrategy,
	nodes []*v1.Node,
	evictLocalStoragePods bool,
	podEvictor *evictions.PodEvictor,
) {
	for _, node := range nodes {
		klog.V(1).Infof("Processing node: %#v", node.Name)
		dpm := listDuplicatePodsOnANode(client, node, evictLocalStoragePods)
		for creator, pods := range dpm {
			if len(pods) > 1 {
				klog.V(1).Infof("%#v", creator)
				// i = 0 does not evict the first pod
				for i := 1; i < len(pods); i++ {
					if _, err := podEvictor.EvictPod(pods[i], node); err != nil {
						break
					}
				}
			}
		}
	}
}

//type creator string
type duplicatePodsMap map[string][]*v1.Pod

// listDuplicatePodsOnANode lists duplicate pods on a given node.
func listDuplicatePodsOnANode(client clientset.Interface, node *v1.Node, evictLocalStoragePods bool) duplicatePodsMap {
	pods, err := podutil.ListEvictablePodsOnNode(client, node, evictLocalStoragePods)
	if err != nil {
		return nil
	}

	dpm := duplicatePodsMap{}
	// Ignoring the error here as in the ListDuplicatePodsOnNode function we call ListEvictablePodsOnNode which checks for error.
	for _, pod := range pods {
		ownerRefList := podutil.OwnerRef(pod)
		for _, ownerRef := range ownerRefList {
			// Namespace/Kind/Name should be unique for the cluster.
			s := strings.Join([]string{pod.ObjectMeta.Namespace, ownerRef.Kind, ownerRef.Name}, "/")
			dpm[s] = append(dpm[s], pod)
		}
	}
	return dpm
}
