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

package pod

import (
	"math/rand"
	"sort"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/tools/cache"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/utils"
)

const (
	nodeNameKeyIndex = "spec.nodeName"
)

var (
	EvictionStrategy = api.EvictionStrategy{}
)

// FilterFunc is a filter for a pod.
type FilterFunc func(*v1.Pod) bool

// GetPodsAssignedToNodeFunc is a function which accept a node name and a pod filter function
// as input and returns the pods that assigned to the node.
type GetPodsAssignedToNodeFunc func(string, FilterFunc) ([]*v1.Pod, error)

// WrapFilterFuncs wraps a set of FilterFunc in one.
func WrapFilterFuncs(filters ...FilterFunc) FilterFunc {
	return func(pod *v1.Pod) bool {
		for _, filter := range filters {
			if filter != nil && !filter(pod) {
				return false
			}
		}
		return true
	}
}

type Options struct {
	filter             FilterFunc
	includedNamespaces sets.String
	excludedNamespaces sets.String
	labelSelector      *metav1.LabelSelector
}

// NewOptions returns an empty Options.
func NewOptions() *Options {
	return &Options{}
}

// WithFilter sets a pod filter.
// The filter function should return true if the pod should be returned from ListPodsOnANode
func (o *Options) WithFilter(filter FilterFunc) *Options {
	o.filter = filter
	return o
}

// WithNamespaces sets included namespaces
func (o *Options) WithNamespaces(namespaces sets.String) *Options {
	o.includedNamespaces = namespaces
	return o
}

// WithoutNamespaces sets excluded namespaces
func (o *Options) WithoutNamespaces(namespaces sets.String) *Options {
	o.excludedNamespaces = namespaces
	return o
}

// WithLabelSelector sets a pod label selector
func (o *Options) WithLabelSelector(labelSelector *metav1.LabelSelector) *Options {
	o.labelSelector = labelSelector
	return o
}

// BuildFilterFunc builds a final FilterFunc based on Options.
func (o *Options) BuildFilterFunc() (FilterFunc, error) {
	var s labels.Selector
	var err error
	if o.labelSelector != nil {
		s, err = metav1.LabelSelectorAsSelector(o.labelSelector)
		if err != nil {
			return nil, err
		}
	}
	return func(pod *v1.Pod) bool {
		if o.filter != nil && !o.filter(pod) {
			return false
		}
		if len(o.includedNamespaces) > 0 && !o.includedNamespaces.Has(pod.Namespace) {
			return false
		}
		if len(o.excludedNamespaces) > 0 && o.excludedNamespaces.Has(pod.Namespace) {
			return false
		}
		if s != nil && !s.Matches(labels.Set(pod.GetLabels())) {
			return false
		}
		return true
	}, nil
}

// BuildGetPodsAssignedToNodeFunc establishes an indexer to map the pods and their assigned nodes.
// It returns a function to help us get all the pods that assigned to a node based on the indexer.
func BuildGetPodsAssignedToNodeFunc(podInformer coreinformers.PodInformer) (GetPodsAssignedToNodeFunc, error) {
	// Establish an indexer to map the pods and their assigned nodes.
	err := podInformer.Informer().AddIndexers(cache.Indexers{
		nodeNameKeyIndex: func(obj interface{}) ([]string, error) {
			pod, ok := obj.(*v1.Pod)
			if !ok {
				return []string{}, nil
			}
			if len(pod.Spec.NodeName) == 0 {
				return []string{}, nil
			}
			return []string{pod.Spec.NodeName}, nil
		},
	})
	if err != nil {
		return nil, err
	}

	// The indexer helps us get all the pods that assigned to a node.
	podIndexer := podInformer.Informer().GetIndexer()
	getPodsAssignedToNode := func(nodeName string, filter FilterFunc) ([]*v1.Pod, error) {
		objs, err := podIndexer.ByIndex(nodeNameKeyIndex, nodeName)
		if err != nil {
			return nil, err
		}
		pods := make([]*v1.Pod, 0, len(objs))
		for _, obj := range objs {
			pod, ok := obj.(*v1.Pod)
			if !ok {
				continue
			}
			if filter(pod) {
				pods = append(pods, pod)
			}
		}
		return pods, nil
	}
	return getPodsAssignedToNode, nil
}

// ListPodsOnANode lists all pods on a node.
// It also accepts a "filter" function which can be used to further limit the pods that are returned.
// (Usually this is podEvictor.Evictable().IsEvictable, in order to only list the evictable pods on a node, but can
// be used by strategies to extend it if there are further restrictions, such as with NodeAffinity).
func ListPodsOnANode(
	nodeName string,
	getPodsAssignedToNode GetPodsAssignedToNodeFunc,
	filter FilterFunc,
) ([]*v1.Pod, error) {
	// Succeeded and failed pods are not considered because they don't occupy any resource.
	f := func(pod *v1.Pod) bool {
		return pod.Status.Phase != v1.PodSucceeded && pod.Status.Phase != v1.PodFailed
	}
	return ListAllPodsOnANode(nodeName, getPodsAssignedToNode, WrapFilterFuncs(f, filter))
}

// ListAllPodsOnANode lists all the pods on a node no matter what the phase of the pod is.
func ListAllPodsOnANode(
	nodeName string,
	getPodsAssignedToNode GetPodsAssignedToNodeFunc,
	filter FilterFunc,
) ([]*v1.Pod, error) {
	pods, err := getPodsAssignedToNode(nodeName, filter)
	if err != nil {
		return []*v1.Pod{}, err
	}

	return pods, nil
}

// OwnerRef returns the ownerRefList for the pod.
func OwnerRef(pod *v1.Pod) []metav1.OwnerReference {
	return pod.ObjectMeta.GetOwnerReferences()
}

func isBestEffortPod(pod *v1.Pod) bool {
	return utils.GetPodQOS(pod) == v1.PodQOSBestEffort
}

func isBurstablePod(pod *v1.Pod) bool {
	return utils.GetPodQOS(pod) == v1.PodQOSBurstable
}

func isGuaranteedPod(pod *v1.Pod) bool {
	return utils.GetPodQOS(pod) == v1.PodQOSGuaranteed
}

func isQOSEqual(pa, pb *v1.Pod) bool {
	return utils.GetPodQOS(pa) == utils.GetPodQOS(pb)
}

// sortPodsBasedOnEvictionSortStrategy sorts pods based on the evictionSort strategy.
// if the evictionSort strategy is empty, pods will be sorted in alphabetical order.
// if the evictionSort strategy is None, pods will be sorted in random order.
// if the evictionSort strategy is OldestFirst, pods will be sorted in order of age.
func sortPodsBasedOnEvictionSortStrategy(pods []*v1.Pod) {
	if EvictionStrategy.None {
		rand.Shuffle(len(pods), func(i, j int) {
			pods[i], pods[j] = pods[j], pods[i]
		})
	}
	if EvictionStrategy.OldestFirst {
		sort.Slice(pods, func(i, j int) bool {
			return pods[i].CreationTimestamp.Before(&pods[j].CreationTimestamp)
		})
	}
}

// SortPodsBasedOnPriorityLowToHigh sorts pods based on their priorities from low to high.
// If pods have same priorities, they will be sorted by QoS in the following order:
// BestEffort, Burstable, Guaranteed
func SortPodsBasedOnPriorityLowToHigh(pods []*v1.Pod) {
	sortPodsBasedOnEvictionSortStrategy(pods)
	sort.Slice(pods, func(i, j int) bool {
		if pods[i].Spec.Priority == nil && pods[j].Spec.Priority != nil {
			return true
		}
		if pods[j].Spec.Priority == nil && pods[i].Spec.Priority != nil {
			return false
		}
		if (pods[j].Spec.Priority == nil && pods[i].Spec.Priority == nil) || (*pods[i].Spec.Priority == *pods[j].Spec.Priority) {
			// if both pods share the same priority and QoS level, the original order remains.
			if isQOSEqual(pods[i], pods[j]) {
				return i < j
			}
			if isBestEffortPod(pods[i]) {
				return true
			}
			if isBurstablePod(pods[i]) && isGuaranteedPod(pods[j]) {
				return true
			}
			return false
		}
		return *pods[i].Spec.Priority < *pods[j].Spec.Priority
	})
}
