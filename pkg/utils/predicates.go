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

package utils

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/component-helpers/scheduling/corev1"
	"k8s.io/klog/v2"
)

// The following code has been copied from predicates package to avoid the
// huge vendoring issues, mostly copied from
// k8s.io/kubernetes/plugin/pkg/scheduler/algorithm/predicates/
// Some minor changes have been made to ease the imports, but most of the code
// remains untouched

// PodMatchNodeSelector checks if a pod node selector matches the node label.
func PodMatchNodeSelector(pod *v1.Pod, node *v1.Node) (bool, error) {
	if node == nil {
		return false, fmt.Errorf("node not found")
	}
	if podMatchesNodeLabels(pod, node) {
		return true, nil
	}
	return false, nil
}

// The pod can only schedule onto nodes that satisfy requirements in both NodeAffinity and nodeSelector.
func podMatchesNodeLabels(pod *v1.Pod, node *v1.Node) bool {
	// Check if node.Labels match pod.Spec.NodeSelector.
	if len(pod.Spec.NodeSelector) > 0 {
		selector := labels.SelectorFromSet(pod.Spec.NodeSelector)
		if !selector.Matches(labels.Set(node.Labels)) {
			return false
		}
	}

	// 1. nil NodeSelector matches all nodes (i.e. does not filter out any nodes)
	// 2. nil []NodeSelectorTerm (equivalent to non-nil empty NodeSelector) matches no nodes
	// 3. zero-length non-nil []NodeSelectorTerm matches no nodes also, just for simplicity
	// 4. nil []NodeSelectorRequirement (equivalent to non-nil empty NodeSelectorTerm) matches no nodes
	// 5. zero-length non-nil []NodeSelectorRequirement matches no nodes also, just for simplicity
	// 6. non-nil empty NodeSelectorRequirement is not allowed

	affinity := pod.Spec.Affinity
	if affinity != nil && affinity.NodeAffinity != nil {
		nodeAffinity := affinity.NodeAffinity
		// if no required NodeAffinity requirements, will do no-op, means select all nodes.
		if nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
			return true
		}

		// Match node selector for requiredDuringSchedulingIgnoredDuringExecution.
		if nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
			klog.V(10).InfoS("Match for RequiredDuringSchedulingIgnoredDuringExecution node selector", "selector", nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution)
			matches, err := corev1.MatchNodeSelectorTerms(node, nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution)
			if err != nil {
				klog.ErrorS(err, "error parsing node selector", "selector", nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution)
			}
			return matches
		}
	}
	return true
}

// TolerationsTolerateTaint checks if taint is tolerated by any of the tolerations.
func TolerationsTolerateTaint(tolerations []v1.Toleration, taint *v1.Taint) bool {
	for i := range tolerations {
		if tolerations[i].ToleratesTaint(taint) {
			return true
		}
	}
	return false
}

type taintsFilterFunc func(*v1.Taint) bool

// TolerationsTolerateTaintsWithFilter checks if given tolerations tolerates
// all the taints that apply to the filter in given taint list.
func TolerationsTolerateTaintsWithFilter(tolerations []v1.Toleration, taints []v1.Taint, applyFilter taintsFilterFunc) bool {
	if len(taints) == 0 {
		return true
	}

	for i := range taints {
		if applyFilter != nil && !applyFilter(&taints[i]) {
			continue
		}

		if !TolerationsTolerateTaint(tolerations, &taints[i]) {
			return false
		}
	}

	return true
}
