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

package removepodsviolatinginterpodantiaffinity

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
	"sigs.k8s.io/descheduler/pkg/utils"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

const PluginName = "RemovePodsViolatingInterPodAntiAffinity"

// RemovePodsViolatingInterPodAntiAffinity evicts pods on the node which violate inter pod anti affinity
type RemovePodsViolatingInterPodAntiAffinity struct {
	handle    frameworktypes.Handle
	args      *RemovePodsViolatingInterPodAntiAffinityArgs
	podFilter podutil.FilterFunc
}

var _ frameworktypes.DeschedulePlugin = &RemovePodsViolatingInterPodAntiAffinity{}

// New builds plugin from its arguments while passing a handle
func New(args runtime.Object, handle frameworktypes.Handle) (frameworktypes.Plugin, error) {
	interPodAntiAffinityArgs, ok := args.(*RemovePodsViolatingInterPodAntiAffinityArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type RemovePodsViolatingInterPodAntiAffinityArgs, got %T", args)
	}

	var includedNamespaces, excludedNamespaces sets.Set[string]
	if interPodAntiAffinityArgs.Namespaces != nil {
		includedNamespaces = sets.New(interPodAntiAffinityArgs.Namespaces.Include...)
		excludedNamespaces = sets.New(interPodAntiAffinityArgs.Namespaces.Exclude...)
	}

	podFilter, err := podutil.NewOptions().
		WithNamespaces(includedNamespaces).
		WithoutNamespaces(excludedNamespaces).
		WithLabelSelector(interPodAntiAffinityArgs.LabelSelector).
		BuildFilterFunc()
	if err != nil {
		return nil, fmt.Errorf("error initializing pod filter function: %v", err)
	}

	return &RemovePodsViolatingInterPodAntiAffinity{
		handle:    handle,
		podFilter: podFilter,
		args:      interPodAntiAffinityArgs,
	}, nil
}

// Name retrieves the plugin name
func (d *RemovePodsViolatingInterPodAntiAffinity) Name() string {
	return PluginName
}

func (d *RemovePodsViolatingInterPodAntiAffinity) Deschedule(ctx context.Context, nodes []*v1.Node) *frameworktypes.Status {
	pods, err := podutil.ListPodsOnNodes(nodes, d.handle.GetPodsAssignedToNodeFunc(), d.podFilter)
	if err != nil {
		return &frameworktypes.Status{
			Err: fmt.Errorf("error listing all pods: %v", err),
		}
	}

	podsInANamespace := podutil.GroupByNamespace(pods)
	podsOnANode := groupByNodeName(pods)
	nodeMap := createNodeMap(nodes)

loop:
	for _, node := range nodes {
		klog.V(2).InfoS("Processing node", "node", klog.KObj(node))
		pods := podsOnANode[node.Name]
		// sort the evict-able Pods based on priority, if there are multiple pods with same priority, they are sorted based on QoS tiers.
		podutil.SortPodsBasedOnPriorityLowToHigh(pods)
		totalPods := len(pods)
		for i := 0; i < totalPods; i++ {
			if checkPodsWithAntiAffinityExist(pods[i], podsInANamespace, nodeMap) && d.handle.Evictor().Filter(pods[i]) && d.handle.Evictor().PreEvictionFilter(pods[i]) {
				if d.handle.Evictor().Evict(ctx, pods[i], evictions.EvictOptions{StrategyName: PluginName}) {
					// Since the current pod is evicted all other pods which have anti-affinity with this
					// pod need not be evicted.
					// Update allPods.
					podsInANamespace = removePodFromNamespaceMap(pods[i], podsInANamespace)
					pods = append(pods[:i], pods[i+1:]...)
					i--
					totalPods--
				}
			}
			if d.handle.Evictor().NodeLimitExceeded(node) {
				continue loop
			}
		}
	}
	return nil
}

func removePodFromNamespaceMap(podToRemove *v1.Pod, podMap map[string][]*v1.Pod) map[string][]*v1.Pod {
	podList, ok := podMap[podToRemove.Namespace]
	if !ok {
		return podMap
	}
	for i := 0; i < len(podList); i++ {
		podToCheck := podList[i]
		if podToRemove.Name == podToCheck.Name {
			podMap[podToRemove.Namespace] = append(podList[:i], podList[i+1:]...)
			return podMap
		}
	}
	return podMap
}

func groupByNodeName(pods []*v1.Pod) map[string][]*v1.Pod {
	m := make(map[string][]*v1.Pod)
	for i := 0; i < len(pods); i++ {
		pod := pods[i]
		m[pod.Spec.NodeName] = append(m[pod.Spec.NodeName], pod)
	}
	return m
}

func createNodeMap(nodes []*v1.Node) map[string]*v1.Node {
	m := make(map[string]*v1.Node, len(nodes))
	for _, node := range nodes {
		m[node.GetName()] = node
	}
	return m
}

// checkPodsWithAntiAffinityExist checks if there are other pods on the node that the current pod cannot tolerate.
func checkPodsWithAntiAffinityExist(pod *v1.Pod, pods map[string][]*v1.Pod, nodeMap map[string]*v1.Node) bool {
	affinity := pod.Spec.Affinity
	if affinity != nil && affinity.PodAntiAffinity != nil {
		for _, term := range getPodAntiAffinityTerms(affinity.PodAntiAffinity) {
			namespaces := utils.GetNamespacesFromPodAffinityTerm(pod, &term)
			selector, err := metav1.LabelSelectorAsSelector(term.LabelSelector)
			if err != nil {
				klog.ErrorS(err, "Unable to convert LabelSelector into Selector")
				return false
			}
			for namespace := range namespaces {
				for _, existingPod := range pods[namespace] {
					if existingPod.Name != pod.Name && utils.PodMatchesTermsNamespaceAndSelector(existingPod, namespaces, selector) {
						node, ok := nodeMap[pod.Spec.NodeName]
						if !ok {
							continue
						}
						nodeHavingExistingPod, ok := nodeMap[existingPod.Spec.NodeName]
						if !ok {
							continue
						}
						if hasSameLabelValue(node, nodeHavingExistingPod, term.TopologyKey) {
							klog.V(1).InfoS("Found Pods violating PodAntiAffinity", "pod to evicted", klog.KObj(pod))
							return true
						}
					}
				}
			}
		}
	}
	return false
}

// hasSameLabelValue checks if the pods are in the same topology zone.
func hasSameLabelValue(node1, node2 *v1.Node, key string) bool {
	if node1.Name == node2.Name {
		return true
	}

	// no match if node has empty labels
	node1Labels := node1.Labels
	if node1Labels == nil {
		return false
	}
	node2Labels := node2.Labels
	if node2Labels == nil {
		return false
	}

	// no match if node has no topology zone label with given key
	value1, ok := node1Labels[key]
	if !ok {
		return false
	}
	value2, ok := node2Labels[key]
	if !ok {
		return false
	}

	return value1 == value2
}

// getPodAntiAffinityTerms gets the antiaffinity terms for the given pod.
func getPodAntiAffinityTerms(podAntiAffinity *v1.PodAntiAffinity) (terms []v1.PodAffinityTerm) {
	if podAntiAffinity != nil {
		if len(podAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution) != 0 {
			terms = podAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution
		}
	}
	return terms
}
