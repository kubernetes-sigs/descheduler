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

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"

	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/framework"
	"sigs.k8s.io/descheduler/pkg/utils"
)

const PluginName = "RemovePodsViolatingInterPodAntiAffinity"

// RemovePodsViolatingInterPodAntiAffinity evicts pods on the node which are having a pod affinity rules.
type RemovePodsViolatingInterPodAntiAffinity struct {
	handle    framework.Handle
	args      *framework.RemovePodsViolatingInterPodAntiAffinityArgs
	podFilter podutil.FilterFunc
}

var _ framework.Plugin = &RemovePodsViolatingInterPodAntiAffinity{}
var _ framework.DeschedulePlugin = &RemovePodsViolatingInterPodAntiAffinity{}

func New(args runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	podAffinityArgs, ok := args.(*framework.RemovePodsViolatingInterPodAntiAffinityArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type RemovePodsViolatingInterPodAntiAffinityArgs, got %T", args)
	}

	if err := framework.ValidateCommonArgs(podAffinityArgs.CommonArgs); err != nil {
		return nil, err
	}

	var includedNamespaces, excludedNamespaces sets.String
	if podAffinityArgs.Namespaces != nil {
		includedNamespaces = sets.NewString(podAffinityArgs.Namespaces.Include...)
		excludedNamespaces = sets.NewString(podAffinityArgs.Namespaces.Exclude...)
	}

	podFilter, err := podutil.NewOptions().
		WithNamespaces(includedNamespaces).
		WithoutNamespaces(excludedNamespaces).
		WithLabelSelector(podAffinityArgs.LabelSelector).
		BuildFilterFunc()
	if err != nil {
		return nil, fmt.Errorf("error initializing pod filter function: %v", err)
	}

	return &RemovePodsViolatingInterPodAntiAffinity{
		handle:    handle,
		args:      podAffinityArgs,
		podFilter: podFilter,
	}, nil
}

func (d *RemovePodsViolatingInterPodAntiAffinity) Name() string {
	return PluginName
}

func (d *RemovePodsViolatingInterPodAntiAffinity) Deschedule(ctx context.Context, nodes []*v1.Node) *framework.Status {
	for _, node := range nodes {
		klog.V(1).InfoS("Processing node", "node", klog.KObj(node))
		pods, err := podutil.ListPodsOnANode(node.Name, d.handle.GetPodsAssignedToNodeFunc(), d.podFilter)
		if err != nil {
			// no pods evicted as error encountered retrieving evictable Pods
			return &framework.Status{
				Err: fmt.Errorf("error listing pods on a node: %v", err),
			}
		}
		// sort the evictable Pods based on priority, if there are multiple pods with same priority, they are sorted based on QoS tiers.
		podutil.SortPodsBasedOnPriorityLowToHigh(pods)
		totalPods := len(pods)
		for i := 0; i < totalPods; i++ {
			if checkPodsWithAntiAffinityExist(pods[i], pods) && d.handle.Evictor().Filter(pods[i]) {
				if d.handle.Evictor().Evict(ctx, pods[i]) {
					// Since the current pod is evicted all other pods which have anti-affinity with this
					// pod need not be evicted.
					// Update pods.
					pods = append(pods[:i], pods[i+1:]...)
					i--
					totalPods--
				}
			}
		}
	}
	return nil
}

// checkPodsWithAntiAffinityExist checks if there are other pods on the node that the current pod cannot tolerate.
func checkPodsWithAntiAffinityExist(pod *v1.Pod, pods []*v1.Pod) bool {
	affinity := pod.Spec.Affinity
	if affinity != nil && affinity.PodAntiAffinity != nil {
		for _, term := range getPodAntiAffinityTerms(affinity.PodAntiAffinity) {
			namespaces := utils.GetNamespacesFromPodAffinityTerm(pod, &term)
			selector, err := metav1.LabelSelectorAsSelector(term.LabelSelector)
			if err != nil {
				klog.ErrorS(err, "Unable to convert LabelSelector into Selector")
				return false
			}
			for _, existingPod := range pods {
				if existingPod.Name != pod.Name && utils.PodMatchesTermsNamespaceAndSelector(existingPod, namespaces, selector) {
					return true
				}
			}
		}
	}
	return false
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
