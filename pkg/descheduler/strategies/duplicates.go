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

	"github.com/golang/glog"

	"k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"

	"github.com/kubernetes-incubator/descheduler/cmd/descheduler/app/options"
	"github.com/kubernetes-incubator/descheduler/pkg/api"
	"github.com/kubernetes-incubator/descheduler/pkg/descheduler/evictions"
	podutil "github.com/kubernetes-incubator/descheduler/pkg/descheduler/pod"
)

//type creator string
type DuplicatePodsMap map[string][]*v1.Pod

// RemoveDuplicatePods removes the duplicate pods on node. This strategy evicts all duplicate pods on node.
// A pod is said to be a duplicate of other if both of them are from same creator, kind and are within the same
// namespace. As of now, this strategy won't evict daemonsets, mirror pods, critical pods and pods with local storages.
func RemoveDuplicatePods(ds *options.DeschedulerServer, strategy api.DeschedulerStrategy, policyGroupVersion string, nodes []*v1.Node, nodepodCount nodePodEvictedCount) {
	if !strategy.Enabled {
		return
	}
	deleteDuplicatePods(ds.Client, policyGroupVersion, nodes, ds.DryRun, nodepodCount, ds.MaxNoOfPodsToEvictPerNode, ds.EvictLocalStoragePods)
}

// deleteDuplicatePods evicts the pod from node and returns the count of evicted pods.
func deleteDuplicatePods(client clientset.Interface, policyGroupVersion string, nodes []*v1.Node, dryRun bool, nodepodCount nodePodEvictedCount, maxPodsToEvict int, evictLocalStoragePods bool) int {
	podsEvicted := 0
	for _, node := range nodes {
		glog.V(1).Infof("Processing node: %#v", node.Name)
		dpm := ListDuplicatePodsOnANode(client, node, evictLocalStoragePods)
		for creator, pods := range dpm {
			if len(pods) > 1 {
				glog.V(1).Infof("%#v", creator)
				// i = 0 does not evict the first pod
				for i := 1; i < len(pods); i++ {
					if maxPodsToEvict > 0 && nodepodCount[node]+1 > maxPodsToEvict {
						break
					}
					success, err := evictions.EvictPod(client, pods[i], policyGroupVersion, dryRun)
					if !success {
						glog.Infof("Error when evicting pod: %#v (%#v)", pods[i].Name, err)
					} else {
						nodepodCount[node]++
						glog.V(1).Infof("Evicted pod: %#v (%#v)", pods[i].Name, err)
					}
				}
			}
		}
		podsEvicted += nodepodCount[node]
	}
	return podsEvicted
}

// ListDuplicatePodsOnANode lists duplicate pods on a given node.
func ListDuplicatePodsOnANode(client clientset.Interface, node *v1.Node, evictLocalStoragePods bool) DuplicatePodsMap {
	pods, err := podutil.ListEvictablePodsOnNode(client, node, evictLocalStoragePods)
	if err != nil {
		return nil
	}
	return FindDuplicatePods(pods)
}

// FindDuplicatePods takes a list of pods and returns a duplicatePodsMap.
func FindDuplicatePods(pods []*v1.Pod) DuplicatePodsMap {
	dpm := DuplicatePodsMap{}
	for _, pod := range pods {
		// Ignoring the error here as in the ListDuplicatePodsOnNode function we call ListEvictablePodsOnNode
		// which checks for error.
		ownerRefList := podutil.OwnerRef(pod)
		for _, ownerRef := range ownerRefList {
			// ownerRef doesn't need namespace since the assumption is owner needs to be in the same namespace.
			s := strings.Join([]string{ownerRef.Kind, ownerRef.Name}, "/")
			dpm[s] = append(dpm[s], pod)
		}
	}
	return dpm
}
