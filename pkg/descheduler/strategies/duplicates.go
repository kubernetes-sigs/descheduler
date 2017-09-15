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
	"strings"

	"k8s.io/kubernetes/pkg/api/v1"
	//TODO: Change to client-go instead of generated clientset.
	"k8s.io/kubernetes/pkg/client/clientset_generated/clientset"

	"github.com/kubernetes-incubator/descheduler/pkg/api"
	"github.com/kubernetes-incubator/descheduler/pkg/descheduler/evictions"
	podutil "github.com/kubernetes-incubator/descheduler/pkg/descheduler/pod"
)

//type creator string
type DuplicatePodsMap map[string][]*v1.Pod

// RemoveDuplicatePods removes the duplicate pods on node. This strategy evicts all duplicate pods on node.
// A pod is said to be a duplicate of other if both of them are from same creator, kind and are within the same
// namespace. As of now, this strategy won't evict daemonsets, mirror pods, critical pods and pods with local storages.
func RemoveDuplicatePods(client clientset.Interface, strategy api.DeschedulerStrategy, policyGroupVersion string, nodes []*v1.Node) {
	if !strategy.Enabled {
		return
	}
	deleteDuplicatePods(client, policyGroupVersion, nodes)
}

// deleteDuplicatePods evicts the pod from node and returns the count of evicted pods.
func deleteDuplicatePods(client clientset.Interface, policyGroupVersion string, nodes []*v1.Node) int {
	podsEvicted := 0
	for _, node := range nodes {
		fmt.Printf("\nProcessing node: %#v\n", node.Name)
		dpm := ListDuplicatePodsOnANode(client, node)
		for creator, pods := range dpm {
			if len(pods) > 1 {
				fmt.Printf("%#v\n", creator)
				// i = 0 does not evict the first pod
				for i := 1; i < len(pods); i++ {
					//fmt.Printf("Removing duplicate pod %#v\n", k.Name)
					success, err := evictions.EvictPod(client, pods[i], policyGroupVersion)
					if !success {
						//TODO: change fmt.Printf as glogs.
						fmt.Printf("Error when evicting pod: %#v (%#v)\n", pods[i].Name, err)
					} else {
						podsEvicted++
						fmt.Printf("Evicted pod: %#v (%#v)\n", pods[i].Name, err)
					}
				}
			}
		}
	}
	return podsEvicted
}

// ListDuplicatePodsOnANode lists duplicate pods on a given node.
func ListDuplicatePodsOnANode(client clientset.Interface, node *v1.Node) DuplicatePodsMap {
	pods, err := podutil.ListPodsOnANode(client, node)
	if err != nil {
		return nil
	}
	return FindDuplicatePods(pods)
}

// FindDuplicatePods takes a list of pods and returns a duplicatePodsMap.
func FindDuplicatePods(pods []*v1.Pod) DuplicatePodsMap {
	dpm := DuplicatePodsMap{}
	for _, pod := range pods {
		sr, err := podutil.CreatorRef(pod)
		if err != nil || sr == nil {
			continue
		}
		if podutil.IsMirrorPod(pod) || podutil.IsDaemonsetPod(sr) || podutil.IsPodWithLocalStorage(pod) || podutil.IsCriticalPod(pod) {
			continue
		}
		s := strings.Join([]string{sr.Reference.Kind, sr.Reference.Namespace, sr.Reference.Name}, "/")
		dpm[s] = append(dpm[s], pod)
	}
	return dpm
}
