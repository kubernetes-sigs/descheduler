/*
Copyright 2020 The Kubernetes Authors.

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

	v1 "k8s.io/api/core/v1"
	v1meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
)

// PodLifeTime evicts pods on nodes that were created more than strategy.Params.MaxPodLifeTimeSeconds seconds ago.
func PodLifeTime(ctx context.Context, client clientset.Interface, strategy api.DeschedulerStrategy, nodes []*v1.Node, podEvictor *evictions.PodEvictor) {
	if strategy.Params == nil || strategy.Params.MaxPodLifeTimeSeconds == nil {
		klog.V(1).Infof("MaxPodLifeTimeSeconds not set")
		return
	}

	for _, node := range nodes {
		klog.V(1).Infof("Processing node: %#v", node.Name)
		pods := listOldPodsOnNode(ctx, client, node, *strategy.Params.MaxPodLifeTimeSeconds, podEvictor)
		for _, pod := range pods {
			success, err := podEvictor.EvictPod(ctx, pod, node)
			if success {
				klog.V(1).Infof("Evicted pod: %#v because it was created more than %v seconds ago", pod.Name, *strategy.Params.MaxPodLifeTimeSeconds)
			}

			if err != nil {
				klog.Errorf("Error evicting pod: (%#v)", err)
				break
			}
		}
	}
}

func listOldPodsOnNode(ctx context.Context, client clientset.Interface, node *v1.Node, maxAge uint, evictor *evictions.PodEvictor) []*v1.Pod {
	pods, err := podutil.ListPodsOnANode(ctx, client, node, evictor.IsEvictable)
	if err != nil {
		return nil
	}

	var oldPods []*v1.Pod
	for _, pod := range pods {
		podAgeSeconds := uint(v1meta.Now().Sub(pod.GetCreationTimestamp().Local()).Seconds())
		if podAgeSeconds > maxAge {
			oldPods = append(oldPods, pod)
		}
	}

	return oldPods
}
