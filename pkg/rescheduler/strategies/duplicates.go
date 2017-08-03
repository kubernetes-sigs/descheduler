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

	"github.com/aveshagarwal/rescheduler/pkg/rescheduler/node"
	"github.com/aveshagarwal/rescheduler/pkg/rescheduler/pod"
)

//type creator string
type DuplicatePodsMap map[string][]*v1.Pod

func RemoveDuplicatePods(client clientset.Interface) error {
	stopChannel := make(chan struct{})
	nodes, err := node.ReadyNodes(client, stopChannel)
	if err != nil {
		return err
	}
	for _, node := range nodes {
		fmt.Printf("\nProcessing node: %#v\n", node.Name)
		dpm := RemoveDuplicatePodsOnANode(client, node)
		for i, j := range dpm {
			fmt.Printf("%#v\n", i)
			for _, k := range j {
				fmt.Printf("Duplicate pod %#v\n", k.Name)
			}
		}
	}
}

func RemoveDuplicatePodsOnANode(client clientset.Interface, node *v1.Node) DuplicatePodsMap {
	pods := pod.ListPodsOnANode(client, node)
	return FindDuplicatePods(pods)
}

func FindDuplicatePods(pods []*v1.Pod) DuplicatePodsMap {
	dpm := DuplicatePodsMap{}

	for _, pod := range pods {
		sr, err := pod.CreatorRef(pod)
		if err != nil || sr == nil {
			continue
		}
		if pod.IsMirrorPod(pod) || IsDaemonsetPod(sr) || IsPodWithLocalStorage(pod) {
			continue
		}
		s := strings.Join([]string{sr.Reference.Kind, sr.Reference.Namespace, sr.Reference.Name}, "/")
		append(dpm[s], pod)
	}
	return dpm
}
