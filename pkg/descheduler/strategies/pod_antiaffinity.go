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
	"context"
	"fmt"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/utils"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

func validateRemovePodsViolatingInterPodAntiAffinityParams(params *api.StrategyParameters) error {
	if params == nil {
		return nil
	}

	// At most one of include/exclude can be set
	if len(params.Namespaces.Include) > 0 && len(params.Namespaces.Exclude) > 0 {
		return fmt.Errorf("only one of Include/Exclude namespaces can be set")
	}

	return nil
}

// RemovePodsViolatingInterPodAntiAffinity evicts pods on the node which are having a pod affinity rules.
func RemovePodsViolatingInterPodAntiAffinity(ctx context.Context, client clientset.Interface, strategy api.DeschedulerStrategy, nodes []*v1.Node, podEvictor *evictions.PodEvictor) {
	var namespaces api.Namespaces
	if strategy.Params != nil {
		namespaces = strategy.Params.Namespaces
	}

	for _, node := range nodes {
		klog.V(1).Infof("Processing node: %#v\n", node.Name)
		pods, err := podutil.ListPodsOnANode(
			ctx,
			client,
			node,
			podutil.WithFilter(func(pod *v1.Pod) bool {
				return podEvictor.IsEvictable(pod, utils.SystemCriticalPriority)
			}),
			podutil.WithNamespaces(namespaces.Include),
			podutil.WithoutNamespaces(namespaces.Exclude),
		)
		if err != nil {
			return
		}
		// sort the evictable Pods based on priority, if there are multiple pods with same priority, they are sorted based on QoS tiers.
		podutil.SortPodsBasedOnPriorityLowToHigh(pods)
		totalPods := len(pods)
		for i := 0; i < totalPods; i++ {
			if checkPodsWithAntiAffinityExist(pods[i], pods) {
				success, err := podEvictor.EvictPod(ctx, pods[i], node, "InterPodAntiAffinity")
				if err != nil {
					klog.Errorf("Error evicting pod: (%#v)", err)
					break
				}

				if success {
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
}

// checkPodsWithAntiAffinityExist checks if there are other pods on the node that the current pod cannot tolerate.
func checkPodsWithAntiAffinityExist(pod *v1.Pod, pods []*v1.Pod) bool {
	affinity := pod.Spec.Affinity
	if affinity != nil && affinity.PodAntiAffinity != nil {
		for _, term := range getPodAntiAffinityTerms(affinity.PodAntiAffinity) {
			namespaces := utils.GetNamespacesFromPodAffinityTerm(pod, &term)
			selector, err := metav1.LabelSelectorAsSelector(term.LabelSelector)
			if err != nil {
				klog.Infof("%v", err)
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
