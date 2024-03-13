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

package node

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	clientset "k8s.io/client-go/kubernetes"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/utils"
)

// ReadyNodes returns ready nodes irrespective of whether they are
// schedulable or not.
func ReadyNodes(ctx context.Context, client clientset.Interface, nodeLister listersv1.NodeLister, nodeSelector string) ([]*v1.Node, error) {
	ns, err := labels.Parse(nodeSelector)
	if err != nil {
		return []*v1.Node{}, err
	}

	var nodes []*v1.Node
	// err is defined above
	if nodes, err = nodeLister.List(ns); err != nil {
		return []*v1.Node{}, err
	}

	if len(nodes) == 0 {
		klog.V(2).InfoS("Node lister returned empty list, now fetch directly")

		nItems, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{LabelSelector: nodeSelector})
		if err != nil {
			return []*v1.Node{}, err
		}

		if nItems == nil || len(nItems.Items) == 0 {
			return []*v1.Node{}, nil
		}

		for i := range nItems.Items {
			node := nItems.Items[i]
			nodes = append(nodes, &node)
		}
	}

	readyNodes := make([]*v1.Node, 0, len(nodes))
	for _, node := range nodes {
		if IsReady(node) {
			readyNodes = append(readyNodes, node)
		}
	}
	return readyNodes, nil
}

// IsReady checks if the descheduler could run against given node.
func IsReady(node *v1.Node) bool {
	for i := range node.Status.Conditions {
		cond := &node.Status.Conditions[i]
		// We consider the node for scheduling only when its:
		// - NodeReady condition status is ConditionTrue,
		// - NodeOutOfDisk condition status is ConditionFalse,
		// - NodeNetworkUnavailable condition status is ConditionFalse.
		if cond.Type == v1.NodeReady && cond.Status != v1.ConditionTrue {
			klog.V(1).InfoS("Ignoring node", "node", klog.KObj(node), "condition", cond.Type, "status", cond.Status)
			return false
		} /*else if cond.Type == v1.NodeOutOfDisk && cond.Status != v1.ConditionFalse {
			klog.V(4).InfoS("Ignoring node with condition status", "node", klog.KObj(node.Name), "condition", cond.Type, "status", cond.Status)
			return false
		} else if cond.Type == v1.NodeNetworkUnavailable && cond.Status != v1.ConditionFalse {
			klog.V(4).InfoS("Ignoring node with condition status", "node", klog.KObj(node.Name), "condition", cond.Type, "status", cond.Status)
			return false
		}*/
	}
	// Ignore nodes that are marked unschedulable
	/*if node.Spec.Unschedulable {
		klog.V(4).InfoS("Ignoring node since it is unschedulable", "node", klog.KObj(node.Name))
		return false
	}*/
	return true
}

// NodeFit returns true if the provided pod can be scheduled onto the provided node.
// This function is used when the NodeFit pod filtering feature of the Descheduler is enabled.
// This function currently considers a subset of the Kubernetes Scheduler's predicates when
// deciding if a pod would fit on a node, but more predicates may be added in the future.
func NodeFit(nodeIndexer podutil.GetPodsAssignedToNodeFunc, pod *v1.Pod, node *v1.Node) []error {
	// Check node selector and required affinity
	var errors []error
	if ok, err := utils.PodMatchNodeSelector(pod, node); err != nil {
		errors = append(errors, err)
	} else if !ok {
		errors = append(errors, fmt.Errorf("pod node selector does not match the node label"))
	}
	// Check taints (we only care about NoSchedule and NoExecute taints)
	ok := utils.TolerationsTolerateTaintsWithFilter(pod.Spec.Tolerations, node.Spec.Taints, func(taint *v1.Taint) bool {
		return taint.Effect == v1.TaintEffectNoSchedule || taint.Effect == v1.TaintEffectNoExecute
	})
	if !ok {
		errors = append(errors, fmt.Errorf("pod does not tolerate taints on the node"))
	}
	// Check if the pod can fit on a node based off it's requests
	if pod.Spec.NodeName == "" || pod.Spec.NodeName != node.Name {
		if ok, reqErrors := fitsRequest(nodeIndexer, pod, node); !ok {
			errors = append(errors, reqErrors...)
		}
	}
	// Check if node is schedulable
	if IsNodeUnschedulable(node) {
		errors = append(errors, fmt.Errorf("node is not schedulable"))
	}

	// Check if pod matches inter-pod anti-affinity rule of pod on node
	if match, err := podMatchesInterPodAntiAffinity(nodeIndexer, pod, node); err != nil {
		errors = append(errors, err)
	} else if match {
		errors = append(errors, fmt.Errorf("pod matches inter-pod anti-affinity rule of other pod on node"))
	}

	return errors
}

// PodFitsAnyOtherNode checks if the given pod will fit any of the given nodes, besides the node
// the pod is already running on. The predicates used to determine if the pod will fit can be found in the NodeFit function.
func PodFitsAnyOtherNode(nodeIndexer podutil.GetPodsAssignedToNodeFunc, pod *v1.Pod, nodes []*v1.Node) bool {
	for _, node := range nodes {
		// Skip node pod is already on
		if node.Name == pod.Spec.NodeName {
			continue
		}

		errors := NodeFit(nodeIndexer, pod, node)
		if len(errors) == 0 {
			klog.V(4).InfoS("Pod fits on node", "pod", klog.KObj(pod), "node", klog.KObj(node))
			return true
		}
		klog.V(4).InfoS("Pod does not fit on any other node",
			"pod:", klog.KObj(pod), "node:", klog.KObj(node), "error:", utilerrors.NewAggregate(errors).Error())
	}

	return false
}

// PodFitsAnyNode checks if the given pod will fit any of the given nodes. The predicates used
// to determine if the pod will fit can be found in the NodeFit function.
func PodFitsAnyNode(nodeIndexer podutil.GetPodsAssignedToNodeFunc, pod *v1.Pod, nodes []*v1.Node) bool {
	for _, node := range nodes {
		errors := NodeFit(nodeIndexer, pod, node)
		if len(errors) == 0 {
			klog.V(4).InfoS("Pod fits on node", "pod", klog.KObj(pod), "node", klog.KObj(node))
			return true
		}
		klog.V(4).InfoS("Pod does not fit on any node",
			"pod:", klog.KObj(pod), "node:", klog.KObj(node), "error:", utilerrors.NewAggregate(errors).Error())
	}

	return false
}

// PodFitsCurrentNode checks if the given pod will fit onto the given node. The predicates used
// to determine if the pod will fit can be found in the NodeFit function.
func PodFitsCurrentNode(nodeIndexer podutil.GetPodsAssignedToNodeFunc, pod *v1.Pod, node *v1.Node) bool {
	errors := NodeFit(nodeIndexer, pod, node)
	if len(errors) == 0 {
		klog.V(4).InfoS("Pod fits on node", "pod", klog.KObj(pod), "node", klog.KObj(node))
		return true
	}

	klog.V(4).InfoS("Pod does not fit on current node",
		"pod:", klog.KObj(pod), "node:", klog.KObj(node), "error:", utilerrors.NewAggregate(errors).Error())

	return false
}

// IsNodeUnschedulable checks if the node is unschedulable. This is a helper function to check only in case of
// underutilized node so that they won't be accounted for.
func IsNodeUnschedulable(node *v1.Node) bool {
	return node.Spec.Unschedulable
}

// fitsRequest determines if a pod can fit on a node based on its resource requests. It returns true if
// the pod will fit.
func fitsRequest(nodeIndexer podutil.GetPodsAssignedToNodeFunc, pod *v1.Pod, node *v1.Node) (bool, []error) {
	var insufficientResources []error

	// Get pod requests
	podRequests, _ := utils.PodRequestsAndLimits(pod)
	resourceNames := make([]v1.ResourceName, 0, len(podRequests))
	for name := range podRequests {
		resourceNames = append(resourceNames, name)
	}

	availableResources, err := nodeAvailableResources(nodeIndexer, node, resourceNames)
	if err != nil {
		return false, []error{err}
	}

	podFitsOnNode := true
	for _, resource := range resourceNames {
		podResourceRequest := podRequests[resource]
		availableResource, ok := availableResources[resource]
		if !ok || podResourceRequest.MilliValue() > availableResource.MilliValue() {
			insufficientResources = append(insufficientResources, fmt.Errorf("insufficient %v", resource))
			podFitsOnNode = false
		}
	}
	// check pod num, at least one pod number is avaibalbe
	if availableResources[v1.ResourcePods].MilliValue() <= 0 {
		insufficientResources = append(insufficientResources, fmt.Errorf("insufficient %v", v1.ResourcePods))
		podFitsOnNode = false
	}

	return podFitsOnNode, insufficientResources
}

// nodeAvailableResources returns resources mapped to the quanitity available on the node.
func nodeAvailableResources(nodeIndexer podutil.GetPodsAssignedToNodeFunc, node *v1.Node, resourceNames []v1.ResourceName) (map[v1.ResourceName]*resource.Quantity, error) {
	podsOnNode, err := podutil.ListPodsOnANode(node.Name, nodeIndexer, nil)
	if err != nil {
		return nil, err
	}
	nodeUtilization := NodeUtilization(podsOnNode, resourceNames)
	remainingResources := map[v1.ResourceName]*resource.Quantity{
		v1.ResourceCPU:    resource.NewMilliQuantity(node.Status.Allocatable.Cpu().MilliValue()-nodeUtilization[v1.ResourceCPU].MilliValue(), resource.DecimalSI),
		v1.ResourceMemory: resource.NewQuantity(node.Status.Allocatable.Memory().Value()-nodeUtilization[v1.ResourceMemory].Value(), resource.BinarySI),
		v1.ResourcePods:   resource.NewQuantity(node.Status.Allocatable.Pods().Value()-nodeUtilization[v1.ResourcePods].Value(), resource.DecimalSI),
	}
	for _, name := range resourceNames {
		if !IsBasicResource(name) {
			if _, exists := node.Status.Allocatable[name]; exists {
				allocatableResource := node.Status.Allocatable[name]
				remainingResources[name] = resource.NewQuantity(allocatableResource.Value()-nodeUtilization[name].Value(), resource.DecimalSI)
			} else {
				remainingResources[name] = resource.NewQuantity(0, resource.DecimalSI)
			}
		}
	}

	return remainingResources, nil
}

// NodeUtilization returns the resources requested by the given pods. Only resources supplied in the resourceNames parameter are calculated.
func NodeUtilization(pods []*v1.Pod, resourceNames []v1.ResourceName) map[v1.ResourceName]*resource.Quantity {
	totalReqs := map[v1.ResourceName]*resource.Quantity{
		v1.ResourceCPU:    resource.NewMilliQuantity(0, resource.DecimalSI),
		v1.ResourceMemory: resource.NewQuantity(0, resource.BinarySI),
		v1.ResourcePods:   resource.NewQuantity(int64(len(pods)), resource.DecimalSI),
	}
	for _, name := range resourceNames {
		if !IsBasicResource(name) {
			totalReqs[name] = resource.NewQuantity(0, resource.DecimalSI)
		}
	}

	for _, pod := range pods {
		req, _ := utils.PodRequestsAndLimits(pod)
		for _, name := range resourceNames {
			quantity, ok := req[name]
			if ok && name != v1.ResourcePods {
				// As Quantity.Add says: Add adds the provided y quantity to the current value. If the current value is zero,
				// the format of the quantity will be updated to the format of y.
				totalReqs[name].Add(quantity)
			}
		}
	}

	return totalReqs
}

// IsBasicResource checks if resource is basic native.
func IsBasicResource(name v1.ResourceName) bool {
	switch name {
	case v1.ResourceCPU, v1.ResourceMemory, v1.ResourcePods:
		return true
	default:
		return false
	}
}

// GetNodeWeightGivenPodPreferredAffinity returns the weight
// that the pod gives to a node by analyzing the soft node affinity of that pod
// (nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution)
func GetNodeWeightGivenPodPreferredAffinity(pod *v1.Pod, node *v1.Node) int32 {
	totalWeight, err := utils.GetNodeWeightGivenPodPreferredAffinity(pod, node)
	if err != nil {
		return 0
	}
	return totalWeight
}

// GetBestNodeWeightGivenPodPreferredAffinity returns the best weight
// (maximum one) that the pod gives to the best node by analyzing the soft node affinity
// of that pod (nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution)
func GetBestNodeWeightGivenPodPreferredAffinity(pod *v1.Pod, nodes []*v1.Node) int32 {
	var bestWeight int32 = 0
	for _, node := range nodes {
		weight := GetNodeWeightGivenPodPreferredAffinity(pod, node)
		if weight > bestWeight {
			bestWeight = weight
		}
	}
	return bestWeight
}

// PodMatchNodeSelector checks if a pod node selector matches the node label.
func PodMatchNodeSelector(pod *v1.Pod, node *v1.Node) bool {
	matches, err := utils.PodMatchNodeSelector(pod, node)
	if err != nil {
		return false
	}
	return matches
}

// podMatchesInterPodAntiAffinity checks if the pod matches the anti-affinity rule
// of another pod that is already on the given node.
// If a match is found, it returns true.
func podMatchesInterPodAntiAffinity(nodeIndexer podutil.GetPodsAssignedToNodeFunc, pod *v1.Pod, node *v1.Node) (bool, error) {
	if pod.Spec.Affinity == nil || pod.Spec.Affinity.PodAntiAffinity == nil {
		return false, nil
	}

	podsOnNode, err := podutil.ListPodsOnANode(node.Name, nodeIndexer, nil)
	if err != nil {
		return false, fmt.Errorf("error listing all pods: %v", err)
	}

	podsInANamespace := podutil.GroupByNamespace(podsOnNode)
	nodeMap := utils.CreateNodeMap([]*v1.Node{node})

	return utils.CheckPodsWithAntiAffinityExist(pod, podsInANamespace, nodeMap), nil
}
