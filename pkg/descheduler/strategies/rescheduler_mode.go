/*
Copyright 2018 The Kubernetes Authors.

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
	podutil "github.com/kubernetes-incubator/descheduler/pkg/descheduler/pod"
	"k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	priorityutil "k8s.io/kubernetes/plugin/pkg/scheduler/algorithm/priorities/util"
)

const (
	criticalAddonsOnlyTaintKey = "CriticalAddonsOnly"
)

func RunInReschedulerMode(ds *options.DeschedulerServer, strategy api.DeschedulerStrategy, policyGroupVersion string, nodes []*v1.Node, nodepodCount nodePodEvictedCount) {
	if !strategy.Enabled {
		return
	}
	evictPodsOnlyInReschedulerMode(ds.Client, policyGroupVersion, nodes, ds.DryRun, nodepodCount, ds.MaxNoOfPodsToEvictPerNode)
}

func evictPodsOnlyInReschedulerMode(client clientset.Interface, policyGroupVersion string, nodes []*v1.Node, dryRun bool, nodepodCount nodePodEvictedCount, maxPodsToEvict int) int {
	for _, node := range nodes {
		criticalDSPods := getCriticalDaemonSetPodsOnNode(client, node)
		if len(criticalDSPods) < 0 || !IsNodeUnderResourcePressure(client, node) {
			glog.Infof("Node %v doesn't have critical pods or this node is not under resource pressure")
			continue
		}
		// Add critical taint to node, so that default scheduler won't schedule pods onto this node, while we
		// are making room for criticalDaemonSetPods.
		if err := addTaintsToNode(client, node); err != nil {
			glog.Fatalf("Error while applying taints. Let's skip this node.")
			continue
		}
		for _, criticalDSPod := range criticalDSPods {
			if err := makeRoomForCriticalDaemonSetPod(client, criticalDSPod, node, policyGroupVersion, dryRun); err != nil {
				glog.Fatal("Error while making room for critical DS pods")
				continue
			}
		}
		// Release critical taint from node.
		releaseTaintsFromNode(client, node)
	}
	return 0
}

// addTaintsToNode adds criticalAddonsOnlyTaint to the node.
func addTaintsToNode(client clientset.Interface, node *v1.Node) error {
	node.Spec.Taints = append(node.Spec.Taints, v1.Taint{
		Key:    criticalAddonsOnlyTaintKey,
		Value:  "CriticalTaint",
		Effect: v1.TaintEffectNoSchedule,
	})

	if _, err := client.CoreV1().Nodes().Update(node); err != nil {
		return err
	}
	return nil
}

// releaseTaintsFromNode releases criticalAddonsOnlyTaint from node.
func releaseTaintsFromNode(client clientset.Interface, node *v1.Node) {
	// Get the updated node object since, we have applied taint in addTaints function. Without this we'd get
	// error related to node object being old.
	node, err := client.CoreV1().Nodes().Get(node.Name, metav1.GetOptions{})
	newTaints := make([]v1.Taint, 0)
	for _, taint := range node.Spec.Taints {
		if taint.Key == criticalAddonsOnlyTaintKey {
			glog.Infof("Releasing taint %+v on node %v", taint, node.Name)
		} else {
			newTaints = append(newTaints, taint)
		}
	}

	if err != nil {
		glog.Fatal("Error while getting updated node object")
	}
	if len(newTaints) != len(node.Spec.Taints) {
		node.Spec.Taints = newTaints
		_, err := client.CoreV1().Nodes().Update(node)
		if err != nil {
			glog.Warningf("Error while releasing taints on node %v: %v", node.Name, err)
		} else {
			glog.Infof("Successfully released all taints on node %v", node.Name)
		}
	}
}

// getResourceUsageOfPod get the cpu and memory usage of given pod.
func getResourceUsageOfPod(pod *v1.Pod) (int64, int64) {
	var cpuUsed, memoryUsed int64
	for _, container := range pod.Spec.Containers {
		nonZeroMilliCPU, nonZeroMemory := priorityutil.GetNonzeroRequests(&container.Resources.Requests)
		cpuUsed += nonZeroMilliCPU
		memoryUsed += nonZeroMemory
	}
	for _, container := range pod.Spec.InitContainers {
		for rName, rQuantity := range container.Resources.Requests {
			switch rName {
			case v1.ResourceCPU:
				if cpu := rQuantity.MilliValue(); cpu > cpuUsed {
					cpuUsed = cpu
				}
			case v1.ResourceMemory:
				if memory := rQuantity.Value(); memory > memoryUsed {
					memoryUsed = memory
				}
			}
		}
	}
	return cpuUsed, memoryUsed
}

// makeRoomForCriticalDaemonSetPod ensures that criticalDaemonSet pods have place on the node.
func makeRoomForCriticalDaemonSetPod(client clientset.Interface, criticalDSPod *v1.Pod, node *v1.Node, policyGroupVersion string, dryRun bool) error {
	evictablePods, err := podutil.ListEvictablePodsOnNode(client, node)
	if err != nil {
		return err
	}
	neededCPU, neededMemory := getResourceUsageOfPod(criticalDSPod)
	var totalMemoryReclaimed, totalCPUReclaimed int64
	// TODO: @ravig - As of now, there is no order in which pods are getting evicted. We can add logic of
	// qos here to make this algo better.
	for _, evictablePod := range evictablePods {
		if totalMemoryReclaimed >= neededMemory && totalCPUReclaimed >= neededCPU {
			// We shouldn't enter this in the first iteration.
			glog.Infof("We don't need to evict any pod futher as critical Pod has enough resources to run on this node")
			return nil
		}
		// As of know, priority is not taken into consideration while evicting.
		success, err := evictions.EvictPod(client, evictablePod, policyGroupVersion, dryRun)
		if !success {
			glog.Infof("Error when evicting pod: %#v (%#v)", evictablePod.Name, err)
		} else {
			cpuUsed, memoryUsed := getResourceUsageOfPod(evictablePod)
			totalCPUReclaimed += totalCPUReclaimed + cpuUsed
			totalMemoryReclaimed += totalMemoryReclaimed + memoryUsed
		}

	}
	return nil
}

// getCriticalDaemonSetPodsOnNode returns list of critical Daemon Set pods on the node.
func getCriticalDaemonSetPodsOnNode(client clientset.Interface, node *v1.Node) []*v1.Pod {
	pods, err := podutil.ListPodsOnANode(client, node)
	var criticalDaemonSetPods []*v1.Pod
	if err != nil {
		glog.Fatalf("Error while listing pods on node %v", node.Name)
		return criticalDaemonSetPods
	}
	for _, pod := range pods {
		ownerRefs := podutil.OwnerRef(pod)
		if podutil.IsCriticalPod(pod) && podutil.IsDaemonsetPod(ownerRefs) {
			criticalDaemonSetPods = append(criticalDaemonSetPods, pod)
		}
	}
	return criticalDaemonSetPods
}

// IsnodeUnderResourcePressure returns true if sum of all requests of all containers in all pods is greater than node
// allocatable.
func IsNodeUnderResourcePressure(client clientset.Interface, node *v1.Node) bool {
	pods, err := podutil.ListPodsOnANode(client, node)
	if err != nil {
		glog.Fatalf("Error while listing pods on node %v", node.Name)
	}
	var totalCPU, totalMemory int64
	for _, pod := range pods {
		cpuUsed, memoryUsed := getResourceUsageOfPod(pod)
		totalCPU += cpuUsed
		totalMemory += memoryUsed
	}
	// cpu usage or memory usage is greater than allocatable then there is very good chance of pod over subscribing
	// resources because of
	glog.Infof("total CPU used %v, node cpu allocatable %v, total memory used %v, total memory allocatable %v", totalCPU, node.Status.Allocatable.Cpu().MilliValue(), totalMemory, node.Status.Allocatable.Memory().Value())
	return totalCPU >= node.Status.Allocatable.Cpu().MilliValue() || totalMemory >= node.Status.Allocatable.Memory().Value()
}
