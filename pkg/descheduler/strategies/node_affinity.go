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
	"github.com/golang/glog"
	"github.com/kubernetes-incubator/descheduler/cmd/descheduler/app/options"
	"github.com/kubernetes-incubator/descheduler/pkg/api"
	"github.com/kubernetes-incubator/descheduler/pkg/descheduler/evictions"
	nodeutil "github.com/kubernetes-incubator/descheduler/pkg/descheduler/node"
	podutil "github.com/kubernetes-incubator/descheduler/pkg/descheduler/pod"
	"k8s.io/api/core/v1"
)

func RemovePodsViolatingNodeAffinity(ds *options.DeschedulerServer, strategy api.DeschedulerStrategy, evictionPolicyGroupVersion string, nodes []*v1.Node, nodePodCount nodePodEvictedCount) {
	removePodsViolatingNodeAffinityCount(ds, strategy, evictionPolicyGroupVersion, nodes, nodePodCount, ds.MaxNoOfPodsToEvictPerNode)
}

func removePodsViolatingNodeAffinityCount(ds *options.DeschedulerServer, strategy api.DeschedulerStrategy, evictionPolicyGroupVersion string, nodes []*v1.Node, nodepodCount nodePodEvictedCount, maxPodsToEvict int) int {
	evictedPodCount := 0
	if !strategy.Enabled {
		return evictedPodCount
	}

	requiredDuringSchedulingIgnoredDuringExecutionEnabled := false
	preferredDuringSchedulingIgnoredDuringExecutionEnabled := false
	for _, nodeAffinity := range strategy.Params.NodeAffinityType {
		switch nodeAffinity {
		case "requiredDuringSchedulingIgnoredDuringExecution":
			requiredDuringSchedulingIgnoredDuringExecutionEnabled = true
		case "preferredDuringSchedulingIgnoredDuringExecution":
			preferredDuringSchedulingIgnoredDuringExecutionEnabled = true
		default:
			glog.Errorf("invalid nodeAffinityType: %v", nodeAffinity)
			return evictedPodCount
		}
	}

	for _, node := range nodes {
		glog.V(1).Infof("Processing node: %#v\n", node.Name)

		pods, err := podutil.ListEvictablePodsOnNode(ds.Client, node)
		if err != nil {
			glog.Errorf("failed to get pods from %v: %v", node.Name, err)
		}

		for _, pod := range pods {
			if maxPodsToEvict > 0 && nodepodCount[node]+1 > maxPodsToEvict {
				break
			}

			if pod.Spec.Affinity == nil || pod.Spec.Affinity.NodeAffinity == nil {
				continue // go to the next pod
			}
			nodeAffinity := pod.Spec.Affinity.NodeAffinity

			if requiredDuringSchedulingIgnoredDuringExecutionEnabled && nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
				glog.V(2).Infof("Checking for requiredDuringSchedulingIgnoredDuringExecution")

				if !nodeutil.PodFitsCurrentNode(pod, node) && nodeutil.PodFitsAnyNode(pod, nodes) {
					glog.V(1).Infof("Evicting pod: %v", pod.Name)
					evictions.EvictPod(ds.Client, pod, evictionPolicyGroupVersion, ds.DryRun)
					nodepodCount[node]++
					evictedPodCount++
					continue // go to the next pod
				}
			}

			if preferredDuringSchedulingIgnoredDuringExecutionEnabled && nodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution != nil {
				glog.V(2).Infof("Checking for preferredDuringSchedulingIgnoredDuringExecution")

				// Evict pod if there is a more preferred node than current node.
				score, err := nodeutil.CalcPodPriorityScore(pod, node)
				if err != nil {
					glog.Errorf("Failed to calculate priority score for pod %v on node %v: %v", pod.Name, node.Name, err)
				} else {
					foundBetter, scoreBetter, nodeNameBetter := nodeutil.FindBetterPreferredNode(pod, score, nodes)
					if foundBetter {
						glog.V(2).Infof("Pod %v can possibly be scheduled on %v", pod.Name, nodeNameBetter)
						glog.V(1).Infof("Evicting pod(%d < %d): %v", score, scoreBetter, pod.Name)
						evictions.EvictPod(ds.Client, pod, evictionPolicyGroupVersion, ds.DryRun)
						nodepodCount[node]++
						evictedPodCount++
						continue // go to the next pod
					}
				}
			}
		}
	}

	glog.V(1).Infof("Evicted %v pods", evictedPodCount)
	return evictedPodCount
}
