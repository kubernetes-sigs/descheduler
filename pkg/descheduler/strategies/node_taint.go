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

	v1 "k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog"
)

const (
	TolerationOpExists v1.TolerationOperator = "Exists"
	TolerationOpEqual  v1.TolerationOperator = "Equal"
)

// RemovePodsViolatingNodeTaints with elimination strategy
func RemovePodsViolatingNodeTaints(ds *options.DeschedulerServer, strategy api.DeschedulerStrategy, policyGroupVersion string, nodes []*v1.Node, nodePodCount nodePodEvictedCount) {
	if !strategy.Enabled {
		return
	}
	deletePodsViolatingNodeTaints(ds.Client, policyGroupVersion, nodes, ds.DryRun, nodePodCount, ds.PodSelector, ds.MaxNoOfPodsToEvictPerNode, ds.EvictLocalStoragePods)
}

// deletePodsViolatingNodeTaints evicts pods on the node which violate NoSchedule Taints on nodes
func deletePodsViolatingNodeTaints(client clientset.Interface, policyGroupVersion string, nodes []*v1.Node, dryRun bool, nodePodCount nodePodEvictedCount, podSelector string, maxPodsToEvict int, evictLocalStoragePods bool) int {
	podsEvicted := 0
	for _, node := range nodes {
		klog.V(1).Infof("Processing node: %#v\n", node.Name)
		pods, err := podutil.ListEvictablePodsOnNode(client, podSelector, node, evictLocalStoragePods)
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
	tolerations := pod.Spec.Tolerations
	taints := node.Spec.Taints
	if len(taints) == 0 {
		return true
	}
	noScheduleTaints := getNoScheduleTaints(taints)
	if !allTaintsTolerated(noScheduleTaints, tolerations) {
		klog.V(2).Infof("Not all taints are tolerated after update for Pod %v on node %v", pod.Name, node.Name)
		return false
	}
	return true
}

// getNoScheduleTaints return a slice of NoSchedule taints from the a slice of taints that it receives.
func getNoScheduleTaints(taints []v1.Taint) []v1.Taint {
	result := []v1.Taint{}
	for i := range taints {
		if taints[i].Effect == v1.TaintEffectNoSchedule {
			result = append(result, taints[i])
		}
	}
	return result
}

//toleratesTaint returns true if a toleration tolerates a taint, or false otherwise
func toleratesTaint(toleration *v1.Toleration, taint *v1.Taint) bool {

	if (len(toleration.Key) > 0 && toleration.Key != taint.Key) ||
		(len(toleration.Effect) > 0 && toleration.Effect != taint.Effect) {
		return false
	}
	switch toleration.Operator {
	// empty operator means Equal
	case "", TolerationOpEqual:
		return toleration.Value == taint.Value
	case TolerationOpExists:
		return true
	default:
		return false
	}
}

// allTaintsTolerated returns true if all are tolerated, or false otherwise.
func allTaintsTolerated(taints []v1.Taint, tolerations []v1.Toleration) bool {
	if len(taints) == 0 {
		return true
	}
	if len(tolerations) == 0 && len(taints) > 0 {
		return false
	}
	for i := range taints {
		tolerated := false
		for j := range tolerations {
			if toleratesTaint(&tolerations[j], &taints[i]) {
				tolerated = true
				break
			}
		}
		if !tolerated {
			return false
		}
	}
	return true
}
