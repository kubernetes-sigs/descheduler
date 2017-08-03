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
	"k8s.io/kubernetes/pkg/api/v1"
	"k8s.io/kubernetes/pkg/client/clientset_generated/clientset"

	"github.com/aveshagarwal/rescheduler/pkg/rescheduler/node"
	"github.com/aveshagarwal/rescheduler/pkg/rescheduler/pod"
)

type creator string
type DuplicatePodsMap [creator][]*v1.Pod

func RemoveDuplicatePods(client clientset.Interface) {
}

func RemoveDuplicatePodsOnANode(client clientset.Interface, node *v1.Node) {
	pods := pod.ListPodsOnANode(client, node)
}

func FindDuplicatePods(pods []*v1.Pod) {
	for i, pod := range pods {
		sr, err := pod.CreatorRef(pod[i])
		if err != nil || sr == nil {
			continue
		}
		if pod.IsMirrorPod(pod) || IsDaemonsetPod(sr) || IsPodWithLocalStorage(pod) {
			continue
		}

	}
}
