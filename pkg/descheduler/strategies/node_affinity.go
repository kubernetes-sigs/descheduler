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

	v1 "k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	nodeutil "sigs.k8s.io/descheduler/pkg/descheduler/node"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
)

// RemovePodsViolatingNodeAffinity evicts pods on nodes which violate node affinity
func RemovePodsViolatingNodeAffinity(ctx context.Context, client clientset.Interface, strategy api.DeschedulerStrategy, nodes []*v1.Node, podEvictor *evictions.PodEvictor) {
	if strategy.Params == nil {
		klog.V(1).Infof("NodeAffinityType not set")
		return
	}
	for _, nodeAffinity := range strategy.Params.NodeAffinityType {
		klog.V(2).Infof("Executing for nodeAffinityType: %v", nodeAffinity)

		switch nodeAffinity {
		case "requiredDuringSchedulingIgnoredDuringExecution":
			for _, node := range nodes {
				klog.V(1).Infof("Processing node: %#v\n", node.Name)

				pods, err := podutil.ListPodsOnANode(ctx, client, node, func(pod *v1.Pod) bool {
					return podEvictor.IsEvictable(pod) &&
						!nodeutil.PodFitsCurrentNode(pod, node) &&
						nodeutil.PodFitsAnyNode(pod, nodes)
				})
				if err != nil {
					klog.Errorf("failed to get pods from %v: %v", node.Name, err)
				}

				for _, pod := range pods {
					if pod.Spec.Affinity != nil && pod.Spec.Affinity.NodeAffinity != nil && pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
						klog.V(1).Infof("Evicting pod: %v", pod.Name)
						if _, err := podEvictor.EvictPod(ctx, pod, node, "NodeAffinity"); err != nil {
							klog.Errorf("Error evicting pod: (%#v)", err)
							break
						}
					}
				}
			}
		default:
			klog.Errorf("invalid nodeAffinityType: %v", nodeAffinity)
		}
	}
	klog.V(1).Infof("Evicted %v pods", podEvictor.TotalEvicted())
}
