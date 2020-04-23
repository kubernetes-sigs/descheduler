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
	"sigs.k8s.io/descheduler/cmd/descheduler/app/options"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/utils"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog"
)

// RemovePodsViolatingInterPodAntiAffinity with elimination strategy
func RemovePodsViolatingInterPodAntiAffinity(ds *options.DeschedulerServer, strategy api.DeschedulerStrategy, nodes []*v1.Node, podEvictor *evictions.PodEvictor) {
	if !strategy.Enabled {
		return
	}
	removePodsWithAffinityRules(ds.Client, nodes, ds.EvictLocalStoragePods, podEvictor)
}

// removePodsWithAffinityRules evicts pods on the node which are having a pod affinity rules.
func removePodsWithAffinityRules(client clientset.Interface, nodes []*v1.Node, evictLocalStoragePods bool, podEvictor *evictions.PodEvictor) {
	for _, node := range nodes {
		klog.V(1).Infof("Processing node: %#v\n", node.Name)
		pods, err := podutil.ListEvictablePodsOnNode(client, node, evictLocalStoragePods)
		if err != nil {
			return
		}
		totalPods := len(pods)
		for i := 0; i < totalPods; i++ {
			if checkPodsWithAntiAffinityExist(pods[i], pods) {
				success, err := podEvictor.EvictPod(pods[i], node)
				if err != nil {
					break
				}

				if success {
					klog.V(1).Infof("Evicted pod: %#v\n because of existing anti-affinity", pods[i].Name)
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
