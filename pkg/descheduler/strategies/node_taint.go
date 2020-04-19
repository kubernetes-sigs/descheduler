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
	"sigs.k8s.io/descheduler/cmd/descheduler/app/options"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/utils"

	"k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog"
)

const (
	TolerationOpExists v1.TolerationOperator = "Exists"
	TolerationOpEqual  v1.TolerationOperator = "Equal"
)

// RemovePodsViolatingNodeTaints with elimination strategy
func RemovePodsViolatingNodeTaints(ds *options.DeschedulerServer, strategy api.DeschedulerStrategy, policyGroupVersion string, nodes []*v1.Node, nodePodCount utils.NodePodEvictedCount) {
	if !strategy.Enabled {
		return
	}
	deletePodsViolatingNodeTaints(ds.Client, policyGroupVersion, nodes, ds.DryRun, nodePodCount, ds.MaxNoOfPodsToEvictPerNode, ds.EvictLocalStoragePods)
}

// deletePodsViolatingNodeTaints evicts pods on the node which violate NoSchedule Taints on nodes
func deletePodsViolatingNodeTaints(client clientset.Interface, policyGroupVersion string, nodes []*v1.Node, dryRun bool, nodePodCount utils.NodePodEvictedCount, maxPodsToEvict int, evictLocalStoragePods bool) int {
	podsEvicted := 0
	for _, node := range nodes {
		klog.V(1).Infof("Processing node: %#v\n", node.Name)
		pods, err := podutil.ListEvictablePodsOnNode(client, node, evictLocalStoragePods)
		if err != nil {
			//no pods evicted as error encountered retrieving evictable Pods
			return 0
		}
		totalPods := len(pods)
		for i := 0; i < totalPods; i++ {
			if maxPodsToEvict > 0 && nodePodCount[node]+1 > maxPodsToEvict {
				break
			}
			if !checkPodsSatisfyTolerations(pods[i], node) {
				success, err := evictions.EvictPod(client, pods[i], policyGroupVersion, dryRun)
				if !success {
					klog.Errorf("Error when evicting pod: %#v (%#v)\n", pods[i].Name, err)
				} else {
					nodePodCount[node]++
					klog.V(1).Infof("Evicted pod: %#v (%#v)", pods[i].Name, err)
				}
			}
		}
		podsEvicted += nodePodCount[node]
	}
	return podsEvicted
}

// checkPodsSatisfyTolerations checks if the node's taints (NoSchedule) are still satisfied by pods' tolerations.
func checkPodsSatisfyTolerations(pod *v1.Pod, node *v1.Node) bool {
	if !utils.TolerationsTolerateTaintsWithFilter(
		pod.Spec.Tolerations,
		node.Spec.Taints,
		func(taint *v1.Taint) bool { return taint.Effect == v1.TaintEffectNoSchedule },
	) {
		klog.V(2).Infof("Not all taints are tolerated after update for Pod %v on node %v", pod.Name, node.Name)
		return false
	}
	return true
}
