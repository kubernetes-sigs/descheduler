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

func RemovePodsViolatingNodeAffinity(ds *options.DeschedulerServer, strategy api.DeschedulerStrategy, evictionPolicyGroupVersion string, nodes []*v1.Node, nodePodCount NodePodEvictedCount) {
	removePodsViolatingNodeAffinityCount(ds, strategy, evictionPolicyGroupVersion, nodes, nodePodCount, ds.MaxNoOfPodsToEvictPerNode, ds.EvictLocalStoragePods)
}

func removePodsViolatingNodeAffinityCount(ds *options.DeschedulerServer, strategy api.DeschedulerStrategy, evictionPolicyGroupVersion string, nodes []*v1.Node, nodepodCount NodePodEvictedCount, maxPodsToEvict int, evictLocalStoragePods bool) int {
	evictedPodCount := 0
	if !strategy.Enabled {
		return evictedPodCount
	}

	for _, nodeAffinity := range strategy.Params.NodeAffinityType {
		glog.V(2).Infof("Executing for nodeAffinityType: %v", nodeAffinity)

		switch nodeAffinity {
		case "requiredDuringSchedulingIgnoredDuringExecution":
			for _, node := range nodes {
				glog.V(1).Infof("Processing node: %#v\n", node.Name)

				pods, err := podutil.ListEvictablePodsOnNode(ds.Client, node, evictLocalStoragePods)
				if err != nil {
					glog.Errorf("failed to get pods from %v: %v", node.Name, err)
				}

				for _, pod := range pods {
					if maxPodsToEvict > 0 && nodepodCount[node]+1 > maxPodsToEvict {
						break
					}
					if pod.Spec.Affinity != nil && pod.Spec.Affinity.NodeAffinity != nil && pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {

						if !nodeutil.PodFitsCurrentNode(pod, node) && nodeutil.PodFitsAnyNode(pod, nodes) {
							glog.V(1).Infof("Evicting pod: %v", pod.Name)
							evictions.EvictPod(ds.Client, pod, evictionPolicyGroupVersion, ds.DryRun)
							nodepodCount[node]++
						}
					}
				}
				evictedPodCount += nodepodCount[node]
			}
		default:
			glog.Errorf("invalid nodeAffinityType: %v", nodeAffinity)
			return evictedPodCount
		}
	}
	glog.V(1).Infof("Evicted %v pods", evictedPodCount)
	return evictedPodCount
}
