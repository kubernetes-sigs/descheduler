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

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"sigs.k8s.io/descheduler/pkg/utils"
)

// ReadyNodes returns ready nodes irrespective of whether they are
// schedulable or not.
func ReadyNodes(ctx context.Context, client clientset.Interface, nodeInformer coreinformers.NodeInformer, nodeSelector string) ([]*v1.Node, error) {
	ns, err := labels.Parse(nodeSelector)
	if err != nil {
		return []*v1.Node{}, err
	}

	var nodes []*v1.Node
	// err is defined above
	if nodes, err = nodeInformer.Lister().List(ns); err != nil {
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
	if node.Spec.Unschedulable {
		klog.V(4).InfoS("Ignoring node since it is unschedulable", "node", klog.KObj(node))
		return false
	}
	return true
}

// PodFitsAnyOtherNode checks if the given pod fits any of the given nodes, besides the node
// the pod is already running on. The node fit is based on multiple criteria, like, pod node selector
// matching the node label (including affinity), the taints on the node, and the node being schedulable or not.
func PodFitsAnyOtherNode(pod *v1.Pod, nodes []*v1.Node) bool {

	for _, node := range nodes {
		// Skip node pod is already on
		if node.Name == pod.Spec.NodeName {
			continue
		}
		// Check node selector and required affinity
		ok, err := utils.PodMatchNodeSelector(pod, node)
		if err != nil || !ok {
			continue
		}
		// Check taints (we only care about NoSchedule and NoExecute taints)
		ok = utils.TolerationsTolerateTaintsWithFilter(pod.Spec.Tolerations, node.Spec.Taints, func(taint *v1.Taint) bool {
			return taint.Effect == v1.TaintEffectNoSchedule || taint.Effect == v1.TaintEffectNoExecute
		})
		if !ok {
			continue
		}
		// Check if node is schedulable
		if !IsNodeUnschedulable(node) {
			klog.V(2).InfoS("Pod can possibly be scheduled on a different node", "pod", klog.KObj(pod), "node", klog.KObj(node))
			return true
		}
	}
	return false
}

// IsNodeUnschedulable checks if the node is unschedulable. This is a helper function to check only in case of
// underutilized node so that they won't be accounted for.
func IsNodeUnschedulable(node *v1.Node) bool {
	return node.Spec.Unschedulable
}

// PodFitsAnyNode checks if the given pod fits any of the given nodes, based on
// multiple criteria, like, pod node selector matching the node label, node
// being schedulable or not.
func PodFitsAnyNode(pod *v1.Pod, nodes []*v1.Node) bool {
	for _, node := range nodes {

		ok, err := utils.PodMatchNodeSelector(pod, node)
		if err != nil || !ok {
			continue
		}
		if !IsNodeUnschedulable(node) {
			klog.V(2).InfoS("Pod can possibly be scheduled on a different node", "pod", klog.KObj(pod), "node", klog.KObj(node))
			return true
		}
	}
	return false
}

// PodFitsCurrentNode checks if the given pod fits on the given node if the pod
// node selector matches the node label.
func PodFitsCurrentNode(pod *v1.Pod, node *v1.Node) bool {
	ok, err := utils.PodMatchNodeSelector(pod, node)

	if err != nil {
		klog.ErrorS(err, "Failed to match node selector")
		return false
	}

	if !ok {
		klog.V(2).InfoS("Pod does not fit on node", "pod", klog.KObj(pod), "node", klog.KObj(node))
		return false
	}

	klog.V(2).InfoS("Pod fits on node", "pod", klog.KObj(pod), "node", klog.KObj(node))
	return true
}
