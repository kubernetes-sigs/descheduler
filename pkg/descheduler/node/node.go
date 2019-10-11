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
	"time"

	"github.com/golang/glog"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	clientset "k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/descheduler/pkg/utils"
)

// ReadyNodes returns ready nodes irrespective of whether they are
// schedulable or not.
func ReadyNodes(client clientset.Interface, nodeSelector string, stopChannel <-chan struct{}) ([]*v1.Node, error) {
	ns, err := labels.Parse(nodeSelector)
	if err != nil {
		return []*v1.Node{}, err
	}

	var nodes []*v1.Node
	nl := GetNodeLister(client, stopChannel)
	if nl != nil {
		// err is defined above
		if nodes, err = nl.List(ns); err != nil {
			return []*v1.Node{}, err
		}
	}

	if len(nodes) == 0 {
		glog.V(2).Infof("node lister returned empty list, now fetch directly")

		nItems, err := client.CoreV1().Nodes().List(metav1.ListOptions{LabelSelector: nodeSelector})
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

func GetNodeLister(client clientset.Interface, stopChannel <-chan struct{}) corelisters.NodeLister {
	if stopChannel == nil {
		return nil
	}
	listWatcher := cache.NewListWatchFromClient(client.CoreV1().RESTClient(), "nodes", v1.NamespaceAll, fields.Everything())
	store := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	nodeLister := corelisters.NewNodeLister(store)
	reflector := cache.NewReflector(listWatcher, &v1.Node{}, store, time.Hour)
	go reflector.Run(stopChannel)

	// To give some time so that listing works, chosen randomly
	time.Sleep(100 * time.Millisecond)

	return nodeLister
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
			glog.V(1).Infof("Ignoring node %v with %v condition status %v", node.Name, cond.Type, cond.Status)
			return false
		} /*else if cond.Type == v1.NodeOutOfDisk && cond.Status != v1.ConditionFalse {
			glog.V(4).Infof("Ignoring node %v with %v condition status %v", node.Name, cond.Type, cond.Status)
			return false
		} else if cond.Type == v1.NodeNetworkUnavailable && cond.Status != v1.ConditionFalse {
			glog.V(4).Infof("Ignoring node %v with %v condition status %v", node.Name, cond.Type, cond.Status)
			return false
		}*/
	}
	// Ignore nodes that are marked unschedulable
	/*if node.Spec.Unschedulable {
		glog.V(4).Infof("Ignoring node %v since it is unschedulable", node.Name)
		return false
	}*/
	return true
}

// IsNodeUschedulable checks if the node is unschedulable. This is helper function to check only in case of
// underutilized node so that they won't be accounted for.
func IsNodeUschedulable(node *v1.Node) bool {
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
		if ok {
			if !IsNodeUschedulable(node) {
				glog.V(2).Infof("Pod %v can possibly be scheduled on %v", pod.Name, node.Name)
				return true
			}
			return false
		}
	}
	return false
}

// PodFitsCurrentNode checks if the given pod fits on the given node if the pod
// node selector matches the node label.
func PodFitsCurrentNode(pod *v1.Pod, node *v1.Node) bool {
	ok, err := utils.PodMatchNodeSelector(pod, node)

	if err != nil {
		glog.Error(err)
		return false
	}

	if !ok {
		glog.V(1).Infof("Pod %v does not fit on node %v", pod.Name, node.Name)
		return false
	}

	glog.V(3).Infof("Pod %v fits on node %v", pod.Name, node.Name)
	return true
}
