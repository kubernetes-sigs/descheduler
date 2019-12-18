/*
Copyright 2019 The Kubernetes Authors.

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

package snapshot

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	schedulerlisters "k8s.io/kubernetes/pkg/scheduler/listers"
	schedulernodeinfo "k8s.io/kubernetes/pkg/scheduler/nodeinfo"
)

// Snapshot is a snapshot of cache NodeInfo and NodeTree order. The scheduler takes a
// snapshot at the beginning of each scheduling cycle and uses it for its operations in that cycle.
type Snapshot struct {
	// NodeInfoMap a map of node name to a snapshot of its NodeInfo.
	NodeInfoMap map[string]*schedulernodeinfo.NodeInfo
	// NodeInfoList is the list of nodes as ordered in the cache's nodeTree.
	NodeInfoList []*schedulernodeinfo.NodeInfo
	// HavePodsWithAffinityNodeInfoList is the list of nodes with at least one pod declaring affinity terms.
	HavePodsWithAffinityNodeInfoList []*schedulernodeinfo.NodeInfo
	Generation                       int64
}

var _ schedulerlisters.SharedLister = &Snapshot{}

// NewEmptySnapshot initializes a Snapshot struct and returns it.
func NewEmptySnapshot() *Snapshot {
	return &Snapshot{
		NodeInfoMap: make(map[string]*schedulernodeinfo.NodeInfo),
	}
}

// NewSnapshot initializes a Snapshot struct and returns it.
func NewSnapshot(nodeInfoMap map[string]*schedulernodeinfo.NodeInfo) *Snapshot {
	nodeInfoList := make([]*schedulernodeinfo.NodeInfo, 0, len(nodeInfoMap))
	havePodsWithAffinityNodeInfoList := make([]*schedulernodeinfo.NodeInfo, 0, len(nodeInfoMap))
	for _, v := range nodeInfoMap {
		nodeInfoList = append(nodeInfoList, v)
		if len(v.PodsWithAffinity()) > 0 {
			havePodsWithAffinityNodeInfoList = append(havePodsWithAffinityNodeInfoList, v)
		}
	}

	s := NewEmptySnapshot()
	s.NodeInfoMap = nodeInfoMap
	s.NodeInfoList = nodeInfoList
	s.HavePodsWithAffinityNodeInfoList = havePodsWithAffinityNodeInfoList

	return s
}

// CreateNodeInfoMap obtains a list of pods and pivots that list into a map where the keys are node names
// and the values are the aggregated information for that node.
func CreateNodeInfoMap(pods []*v1.Pod, nodes []*v1.Node) map[string]*schedulernodeinfo.NodeInfo {
	nodeNameToInfo := make(map[string]*schedulernodeinfo.NodeInfo)
	for _, pod := range pods {
		nodeName := pod.Spec.NodeName
		if _, ok := nodeNameToInfo[nodeName]; !ok {
			nodeNameToInfo[nodeName] = schedulernodeinfo.NewNodeInfo()
		}
		nodeNameToInfo[nodeName].AddPod(pod)
	}
	imageExistenceMap := createImageExistenceMap(nodes)

	for _, node := range nodes {
		if _, ok := nodeNameToInfo[node.Name]; !ok {
			nodeNameToInfo[node.Name] = schedulernodeinfo.NewNodeInfo()
		}
		nodeInfo := nodeNameToInfo[node.Name]
		nodeInfo.SetNode(node)
		nodeInfo.SetImageStates(getNodeImageStates(node, imageExistenceMap))
	}
	return nodeNameToInfo
}

// getNodeImageStates returns the given node's image states based on the given imageExistence map.
func getNodeImageStates(node *v1.Node, imageExistenceMap map[string]sets.String) map[string]*schedulernodeinfo.ImageStateSummary {
	imageStates := make(map[string]*schedulernodeinfo.ImageStateSummary)

	for _, image := range node.Status.Images {
		for _, name := range image.Names {
			imageStates[name] = &schedulernodeinfo.ImageStateSummary{
				Size:     image.SizeBytes,
				NumNodes: len(imageExistenceMap[name]),
			}
		}
	}
	return imageStates
}

// createImageExistenceMap returns a map recording on which nodes the images exist, keyed by the images' names.
func createImageExistenceMap(nodes []*v1.Node) map[string]sets.String {
	imageExistenceMap := make(map[string]sets.String)
	for _, node := range nodes {
		for _, image := range node.Status.Images {
			for _, name := range image.Names {
				if _, ok := imageExistenceMap[name]; !ok {
					imageExistenceMap[name] = sets.NewString(node.Name)
				} else {
					imageExistenceMap[name].Insert(node.Name)
				}
			}
		}
	}
	return imageExistenceMap
}

// Pods returns a PodLister
func (s *Snapshot) Pods() schedulerlisters.PodLister {
	return &podLister{snapshot: s}
}

// NodeInfos returns a NodeInfoLister.
func (s *Snapshot) NodeInfos() schedulerlisters.NodeInfoLister {
	return &nodeInfoLister{snapshot: s}
}

// ListNodes returns the list of nodes in the snapshot.
func (s *Snapshot) ListNodes() []*v1.Node {
	nodes := make([]*v1.Node, 0, len(s.NodeInfoMap))
	for _, n := range s.NodeInfoList {
		if n.Node() != nil {
			nodes = append(nodes, n.Node())
		}
	}
	return nodes
}

type podLister struct {
	snapshot *Snapshot
}

// List returns the list of pods in the snapshot.
func (p *podLister) List(selector labels.Selector) ([]*v1.Pod, error) {
	alwaysTrue := func(p *v1.Pod) bool { return true }
	return p.FilteredList(alwaysTrue, selector)
}

// FilteredList returns a filtered list of pods in the snapshot.
func (p *podLister) FilteredList(podFilter schedulerlisters.PodFilter, selector labels.Selector) ([]*v1.Pod, error) {
	// podFilter is expected to return true for most or all of the pods. We
	// can avoid expensive array growth without wasting too much memory by
	// pre-allocating capacity.
	maxSize := 0
	for _, n := range p.snapshot.NodeInfoMap {
		maxSize += len(n.Pods())
	}
	pods := make([]*v1.Pod, 0, maxSize)
	for _, n := range p.snapshot.NodeInfoMap {
		for _, pod := range n.Pods() {
			if podFilter(pod) && selector.Matches(labels.Set(pod.Labels)) {
				pods = append(pods, pod)
			}
		}
	}
	return pods, nil
}

type nodeInfoLister struct {
	snapshot *Snapshot
}

// List returns the list of nodes in the snapshot.
func (n *nodeInfoLister) List() ([]*schedulernodeinfo.NodeInfo, error) {
	return n.snapshot.NodeInfoList, nil
}

// HavePodsWithAffinityList returns the list of nodes with at least one pods with inter-pod affinity
func (n *nodeInfoLister) HavePodsWithAffinityList() ([]*schedulernodeinfo.NodeInfo, error) {
	return n.snapshot.HavePodsWithAffinityNodeInfoList, nil
}

// Returns the NodeInfo of the given node name.
func (n *nodeInfoLister) Get(nodeName string) (*schedulernodeinfo.NodeInfo, error) {
	if v, ok := n.snapshot.NodeInfoMap[nodeName]; ok {
		return v, nil
	}
	return nil, fmt.Errorf("nodeinfo not found for node name %q", nodeName)
}
