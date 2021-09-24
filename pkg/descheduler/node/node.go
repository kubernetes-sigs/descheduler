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
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/validation"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
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
	/*if node.Spec.Unschedulable {
		klog.V(4).InfoS("Ignoring node since it is unschedulable", "node", klog.KObj(node.Name))
		return false
	}*/
	return true
}

// NodeFit returns true if the provided pod can probably be scheduled onto the provided node.
// This function is used when the NodeFit pod filtering feature of the Descheduler is enabled.
// This function currently considers a subset of the Kubernetes Scheduler's predicates when
// deciding if a pod would fit on a node, but more predicates may be added in the future.
func NodeFit(ctx context.Context, client clientset.Interface, pod *v1.Pod, node *v1.Node) bool {
	// Check node selector and required affinity
	ok, err := utils.PodMatchNodeSelector(pod, node)
	if err != nil || !ok {
		return false
	}
	// Check taints (we only care about NoSchedule and NoExecute taints)
	ok = utils.TolerationsTolerateTaintsWithFilter(pod.Spec.Tolerations, node.Spec.Taints, func(taint *v1.Taint) bool {
		return taint.Effect == v1.TaintEffectNoSchedule || taint.Effect == v1.TaintEffectNoExecute
	})
	if !ok {
		return false
	}
	// Check if the pod can fit on a node based off it's requests
	if !FitsRequest(ctx, client, pod, node) {
		return false
	}
	// Check if node is schedulable
	if IsNodeUnschedulable(node) {
		return false
	}

	klog.V(2).InfoS("Pod can probably be scheduled on a different node once evicted", "pod", klog.KObj(pod), "node", klog.KObj(node))
	return true
}

// PodFitsAnyOtherNode checks if the given pod will probably fit any of the given nodes, besides the node
// the pod is already running on. The predicates used to determine if the pod will fit can be found in the NodeFit function.
func PodFitsAnyOtherNode(ctx context.Context, client clientset.Interface, pod *v1.Pod, nodes []*v1.Node) bool {
	for _, node := range nodes {
		// Skip node pod is already on
		if node.Name == pod.Spec.NodeName {
			continue
		}

		if NodeFit(ctx, client, pod, node) {
			klog.V(2).InfoS("Pod fits on node", "pod", klog.KObj(pod), "node", klog.KObj(node))
			return true
		}
	}
	return false
}

// PodFitsAnyNode checks if the given pod will probably fit any of the given nodes. The predicates used
// to determine if the pod will fit can be found in the NodeFit function.
func PodFitsAnyNode(ctx context.Context, client clientset.Interface, pod *v1.Pod, nodes []*v1.Node) bool {
	for _, node := range nodes {
		if NodeFit(ctx, client, pod, node) {
			klog.V(2).InfoS("Pod fits on node", "pod", klog.KObj(pod), "node", klog.KObj(node))
			return true
		}
	}
	return false
}

// PodFitsCurrentNode checks if the given pod will probably fit onto the given node. The predicates used
// to determine if the pod will fit can be found in the NodeFit function.
func PodFitsCurrentNode(ctx context.Context, client clientset.Interface, pod *v1.Pod, node *v1.Node) bool {
	if NodeFit(ctx, client, pod, node) {
		klog.V(2).InfoS("Pod fits on node", "pod", klog.KObj(pod), "node", klog.KObj(node))
		return true
	}
	return false
}

// IsNodeUnschedulable checks if the node is unschedulable. This is a helper function to check only in case of
// underutilized node so that they won't be accounted for.
func IsNodeUnschedulable(node *v1.Node) bool {
	return node.Spec.Unschedulable
}

// The functions below were lifted from the following link and retrofitted onto the Descheduler.
// https://github.com/kubernetes/kubernetes/blob/master/pkg/scheduler/framework/plugins/noderesources/fit.go

// InsufficientResource describes what kind of resource limit is hit and caused the pod to not fit the node.
type InsufficientResource struct {
	ResourceName v1.ResourceName
	Reason       string
	Requested    int64
	Used         int64
	Capacity     int64
}

// FitsRequest determines if a pod can fit on a node based on that pod's requests. It returns true if
// the pod will fit.
func FitsRequest(ctx context.Context, client clientset.Interface, pod *v1.Pod, node *v1.Node) bool {
	insufficientResources, err := fitsRequest(ctx, client, pod, node)
	if err != nil {
		klog.ErrorS(err, "Error listing evictable pods on node", "node", klog.KObj(node))
		return false
	}

	if len(insufficientResources) != 0 {
		// Log all failure reasons.
		for _, r := range insufficientResources {
			klog.V(2).InfoS("Pod does not fit on node", "pod", klog.KObj(pod), "node", klog.KObj(node), "reason", r.Reason)
		}
		return false
	}

	return true
}

// fitsRequest determines if a pod can fit on a node based on that pod's requests. It returns a slice of []InsufficientResource
// listing all the reasons the pod would have been denied, if any.
func fitsRequest(ctx context.Context, client clientset.Interface, pod *v1.Pod, node *v1.Node) ([]InsufficientResource, error) {
	requests, _ := utils.PodRequestsAndLimits(pod)
	podRequest := NewResource(requests)

	insufficientResources := make([]InsufficientResource, 0, 4)

	podsOnNode, err := podutil.ListPodsOnANode(ctx, client, node)
	if err != nil {
		return nil, err
	}

	nodeInfo := GetNodeInfo(podsOnNode, node)
	allowedPodNumber := nodeInfo.Allocatable.AllowedPodNumber

	if len(nodeInfo.Pods)+1 > int(allowedPodNumber) {
		insufficientResources = append(insufficientResources, InsufficientResource{
			v1.ResourcePods,
			"Too many pods",
			1,
			int64(len(nodeInfo.Pods)),
			int64(allowedPodNumber),
		})
	}

	if podRequest.MilliCPU == 0 &&
		podRequest.Memory == 0 &&
		podRequest.EphemeralStorage == 0 &&
		len(podRequest.ScalarResources) == 0 {
		return insufficientResources, nil
	}

	if podRequest.MilliCPU > (nodeInfo.Allocatable.MilliCPU - nodeInfo.Requested.MilliCPU) {
		insufficientResources = append(insufficientResources, InsufficientResource{
			v1.ResourceCPU,
			"Insufficient cpu",
			podRequest.MilliCPU,
			nodeInfo.Requested.MilliCPU,
			nodeInfo.Allocatable.MilliCPU,
		})
	}
	if podRequest.Memory > (nodeInfo.Allocatable.Memory - nodeInfo.Requested.Memory) {
		insufficientResources = append(insufficientResources, InsufficientResource{
			v1.ResourceMemory,
			"Insufficient memory",
			podRequest.Memory,
			nodeInfo.Requested.Memory,
			nodeInfo.Allocatable.Memory,
		})
	}
	if podRequest.EphemeralStorage > (nodeInfo.Allocatable.EphemeralStorage - nodeInfo.Requested.EphemeralStorage) {
		insufficientResources = append(insufficientResources, InsufficientResource{
			v1.ResourceEphemeralStorage,
			"Insufficient ephemeral-storage",
			podRequest.EphemeralStorage,
			nodeInfo.Requested.EphemeralStorage,
			nodeInfo.Allocatable.EphemeralStorage,
		})
	}

	for rName, rQuant := range podRequest.ScalarResources {
		// Here, a check for ignoring extended resources was removed because it's not currently a
		// feature the Descheduler will utilize. If this code is need in the future, it resides here:
		// https://github.com/kubernetes/kubernetes/blob/deb14b995acf9acd3d7efc1e3fcdb640f09ebdb9/pkg/scheduler/framework/plugins/noderesources/fit.go#L306
		if rQuant > (nodeInfo.Allocatable.ScalarResources[rName] - nodeInfo.Requested.ScalarResources[rName]) {
			insufficientResources = append(insufficientResources, InsufficientResource{
				rName,
				fmt.Sprintf("Insufficient %v", rName),
				podRequest.ScalarResources[rName],
				nodeInfo.Requested.ScalarResources[rName],
				nodeInfo.Allocatable.ScalarResources[rName],
			})
		}
	}

	return insufficientResources, nil
}

// The functions below were lifted from the following link and retrofitted onto the Descheduler.
// https://github.com/kubernetes/kubernetes/blob/master/pkg/scheduler/framework/types.go

// NodeInfo is node level aggregated information.
type NodeInfo struct {
	// Overall node information.
	Node *v1.Node

	// Pods running on the node.
	Pods []*v1.Pod

	// Total requested resources of all pods on this node.
	Requested *Resource

	// We store allocatedResources (which is Node.Status.Allocatable.*) explicitly
	// as int64, to avoid conversions and accessing map.
	Allocatable *Resource
}

// GetNodeInfo intakes a node and all pods running on the node. It returns a
// NodeInfo object representing the state of a node's resource consumption.
func GetNodeInfo(pods []*v1.Pod, node *v1.Node) *NodeInfo {
	nodeInfo := &NodeInfo{
		Node:        node,
		Requested:   &Resource{},
		Allocatable: NewResource(node.Status.Allocatable),
	}

	for _, pod := range pods {
		nodeInfo.AddPod(pod)
	}
	return nodeInfo
}

// AddPod adds pod resource usage information to this NodeInfo.
func (n *NodeInfo) AddPod(pod *v1.Pod) {
	res := calculateResource(pod)
	n.Requested.MilliCPU += res.MilliCPU
	n.Requested.Memory += res.Memory
	n.Requested.EphemeralStorage += res.EphemeralStorage
	if n.Requested.ScalarResources == nil && len(res.ScalarResources) > 0 {
		n.Requested.ScalarResources = map[v1.ResourceName]int64{}
	}
	for rName, rQuant := range res.ScalarResources {
		n.Requested.ScalarResources[rName] += rQuant
	}

	n.Pods = append(n.Pods, pod)
}

// calculateResource uses the below equation to determine the quanities of resources
// a given pod is requesting. It returns a *Resource object representing this.
// resourceRequest = max(sum(podSpec.Containers), podSpec.InitContainers) + overHead
func calculateResource(pod *v1.Pod) *Resource {
	res := &Resource{}
	for _, c := range pod.Spec.Containers {
		res.Add(c.Resources.Requests)
	}

	for _, ic := range pod.Spec.InitContainers {
		res.SetMaxResource(ic.Resources.Requests)
	}

	// If Overhead is being utilized, add to the total requests for the pod
	if pod.Spec.Overhead != nil && utilfeature.DefaultFeatureGate.Enabled(utils.PodOverhead) {
		res.Add(pod.Spec.Overhead)
	}
	return res
}

// Resource is a collection of compute resource.
type Resource struct {
	MilliCPU         int64
	Memory           int64
	EphemeralStorage int64

	// This is Node.Status.Allocatable.Pods().Value()
	AllowedPodNumber int64
	// ScalarResources
	ScalarResources map[v1.ResourceName]int64
}

// NewResource creates a Resource from ResourceList
func NewResource(rl v1.ResourceList) *Resource {
	r := &Resource{}
	r.Add(rl)
	return r
}

// Add adds ResourceList into Resource.
func (r *Resource) Add(rl v1.ResourceList) {
	if r == nil {
		return
	}

	for rName, rQuant := range rl {
		switch rName {
		case v1.ResourceCPU:
			r.MilliCPU += rQuant.MilliValue()
		case v1.ResourceMemory:
			r.Memory += rQuant.Value()
		case v1.ResourcePods:
			r.AllowedPodNumber += rQuant.Value()
		case v1.ResourceEphemeralStorage:
			if utilfeature.DefaultFeatureGate.Enabled(utils.LocalStorageCapacityIsolation) {
				// if the local storage capacity isolation feature gate is disabled, pods request 0 disk.
				r.EphemeralStorage += rQuant.Value()
			}
		default:
			if IsScalarResourceName(rName) {
				r.AddScalar(rName, rQuant.Value())
			}
		}
	}
}

// AddScalar adds a resource by a scalar value of this resource.
func (r *Resource) AddScalar(name v1.ResourceName, quantity int64) {
	r.SetScalar(name, r.ScalarResources[name]+quantity)
}

// SetScalar sets a resource by a scalar value of this resource.
func (r *Resource) SetScalar(name v1.ResourceName, quantity int64) {
	// Lazily allocate scalar resource map.
	if r.ScalarResources == nil {
		r.ScalarResources = map[v1.ResourceName]int64{}
	}
	r.ScalarResources[name] = quantity
}

// SetMaxResource compares with ResourceList and takes max value for each Resource.
func (r *Resource) SetMaxResource(rl v1.ResourceList) {
	if r == nil {
		return
	}

	for rName, rQuantity := range rl {
		switch rName {
		case v1.ResourceMemory:
			r.Memory = max(r.Memory, rQuantity.Value())
		case v1.ResourceCPU:
			r.MilliCPU = max(r.MilliCPU, rQuantity.MilliValue())
		case v1.ResourceEphemeralStorage:
			if utilfeature.DefaultFeatureGate.Enabled(utils.LocalStorageCapacityIsolation) {
				r.EphemeralStorage = max(r.EphemeralStorage, rQuantity.Value())
			}
		default:
			if IsScalarResourceName(rName) {
				r.SetScalar(rName, max(r.ScalarResources[rName], rQuantity.Value()))
			}
		}
	}
}

// IsScalarResourceName validates the resource for Extended, Hugepages, Native and AttachableVolume resources
func IsScalarResourceName(name v1.ResourceName) bool {
	return IsExtendedResourceName(name) || IsHugePageResourceName(name) ||
		IsPrefixedNativeResource(name) || IsAttachableVolumeResourceName(name)
}

func max(a, b int64) int64 {
	if a >= b {
		return a
	}
	return b
}

// The functions below were lifted from the following link and retrofitted onto the Descheduler.
// https://github.com/kubernetes/kubernetes/blob/master/pkg/apis/core/helper/helpers.go

// IsExtendedResourceName returns true if:
// 1. the resource name is not in the default namespace;
// 2. resource name does not have "requests." prefix,
// to avoid confusion with the convention in quota
// 3. it satisfies the rules in IsQualifiedName() after converted into quota resource name
func IsExtendedResourceName(name v1.ResourceName) bool {
	if IsNativeResource(name) || strings.HasPrefix(string(name), v1.DefaultResourceRequestsPrefix) {
		return false
	}
	// Ensure it satisfies the rules in IsQualifiedName() after converted into quota resource name
	nameForQuota := fmt.Sprintf("%s%s", v1.DefaultResourceRequestsPrefix, string(name))
	if errs := validation.IsQualifiedName(string(nameForQuota)); len(errs) != 0 {
		return false
	}
	return true
}

// IsNativeResource returns true if the resource name is in the
// *kubernetes.io/ namespace. Partially-qualified (unprefixed) names are
// implicitly in the kubernetes.io/ namespace.
func IsNativeResource(name v1.ResourceName) bool {
	return !strings.Contains(string(name), "/") ||
		IsPrefixedNativeResource(name)
}

// IsPrefixedNativeResource returns true if the resource name is in the
// *kubernetes.io/ namespace.
func IsPrefixedNativeResource(name v1.ResourceName) bool {
	return strings.Contains(string(name), v1.ResourceDefaultNamespacePrefix)
}

// IsHugePageResourceName returns true if the resource name has the huge page
// resource prefix.
func IsHugePageResourceName(name v1.ResourceName) bool {
	return strings.HasPrefix(string(name), v1.ResourceHugePagesPrefix)
}

// IsAttachableVolumeResourceName returns true when the resource name is prefixed in attachable volume
func IsAttachableVolumeResourceName(name v1.ResourceName) bool {
	return strings.HasPrefix(string(name), v1.ResourceAttachableVolumesPrefix)
}
