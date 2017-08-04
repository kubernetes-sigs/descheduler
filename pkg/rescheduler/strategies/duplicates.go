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
	"k8s.io/kubernetes/pkg/client/clientset_generated/clientset"

	"github.com/aveshagarwal/rescheduler/pkg/rescheduler/evictions"
	podutil "github.com/aveshagarwal/rescheduler/pkg/rescheduler/pod"
)

//type creator string
type DuplicatePodsMap map[string][]*v1.Pod

func RemoveDuplicatePods(client clientset.Interface, policyGroupVersion string, nodes []*v1.Node) error {
	for _, node := range nodes {
		fmt.Printf("\nProcessing node: %#v\n", node.Name)
		dpm := RemoveDuplicatePodsOnANode(client, node)
		for creator, pods := range dpm {
			if len(pods) > 1 {
				fmt.Printf("%#v\n", creator)
				// i = 0 does not evict the first pod
				for i := 1; i < len(pods); i++ {
					//fmt.Printf("Removing duplicate pod %#v\n", k.Name)
					success, err := evictions.EvictPod(client, pods[i], policyGroupVersion)
					if !success {
						fmt.Printf("Error when evicting pod: %#v (%#v)\n", pods[i].Name, err)
					} else {
						fmt.Printf("Evicted pod: %#v (%#v)\n", pods[i].Name, err)
					}
				}
			}
		}
	}
	return nil
}

func RemoveDuplicatePodsOnANode(client clientset.Interface, node *v1.Node) DuplicatePodsMap {
	pods, err := podutil.ListPodsOnANode(client, node)
	if err != nil {
		return nil
	}
	return FindDuplicatePods(pods)
}

func FindDuplicatePods(pods []*v1.Pod) DuplicatePodsMap {
	dpm := DuplicatePodsMap{}

	for _, pod := range pods {
		sr, err := podutil.CreatorRef(pod)
		if err != nil || sr == nil {
			continue
		}
		if podutil.IsMirrorPod(pod) || podutil.IsDaemonsetPod(sr) || podutil.IsPodWithLocalStorage(pod) {
			continue
		}
		s := strings.Join([]string{sr.Reference.Kind, sr.Reference.Namespace, sr.Reference.Name}, "/")
		dpm[s] = append(dpm[s], pod)
	}
	return dpm
}
