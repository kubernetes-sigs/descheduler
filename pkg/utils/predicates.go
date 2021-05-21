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
	"reflect"
	"sort"

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

func uniqueSortNodeSelectorTerms(srcTerms []v1.NodeSelectorTerm) []v1.NodeSelectorTerm {
	terms := append([]v1.NodeSelectorTerm{}, srcTerms...)
	for i := range terms {
		if terms[i].MatchExpressions != nil {
			terms[i].MatchExpressions = uniqueSortNodeSelectorRequirements(terms[i].MatchExpressions)
		}
		if terms[i].MatchFields != nil {
			terms[i].MatchFields = uniqueSortNodeSelectorRequirements(terms[i].MatchFields)
		}
	}

	if len(terms) < 2 {
		return terms
	}

	lastTerm := terms[0]
	uniqueTerms := append([]v1.NodeSelectorTerm{}, terms[0])

	for _, term := range terms[1:] {
		if reflect.DeepEqual(term, lastTerm) {
			continue
		}
		lastTerm = term
		uniqueTerms = append(uniqueTerms, term)
	}

	return uniqueTerms
}

// sort NodeSelectorRequirement in (key, operator, values) order
func uniqueSortNodeSelectorRequirements(srcReqs []v1.NodeSelectorRequirement) []v1.NodeSelectorRequirement {
	reqs := append([]v1.NodeSelectorRequirement{}, srcReqs...)

	// unique sort Values
	for i := range reqs {
		sort.Strings(reqs[i].Values)
		if len(reqs[i].Values) > 1 {
			lastString := reqs[i].Values[0]
			values := []string{lastString}
			for _, val := range reqs[i].Values {
				if val == lastString {
					continue
				}
				lastString = val
				values = append(values, val)
			}
			reqs[i].Values = values
		}
	}

	if len(reqs) < 2 {
		return reqs
	}

	// unique sort reqs
	sort.Slice(reqs, func(i, j int) bool {
		if reqs[i].Key < reqs[j].Key {
			return true
		}
		if reqs[i].Key > reqs[j].Key {
			return false
		}
		if reqs[i].Operator < reqs[j].Operator {
			return true
		}
		if reqs[i].Operator > reqs[j].Operator {
			return false
		}
		if len(reqs[i].Values) < len(reqs[j].Values) {
			return true
		}
		if len(reqs[i].Values) > len(reqs[j].Values) {
			return false
		}
		for k := range reqs[i].Values {
			if reqs[i].Values[k] < reqs[j].Values[k] {
				return true
			}
			if reqs[i].Values[k] > reqs[j].Values[k] {
				return false
			}
		}
		return true
	})

	lastReq := reqs[0]
	uniqueReqs := append([]v1.NodeSelectorRequirement{}, lastReq)
	for _, req := range reqs[1:] {
		if reflect.DeepEqual(req, lastReq) {
			continue
		}
		lastReq = req
		uniqueReqs = append(uniqueReqs, req)
	}
	return uniqueReqs
}

func NodeSelectorsEqual(n1, n2 *v1.NodeSelector) bool {
	if n1 == nil && n2 == nil {
		return true
	}
	if n1 == nil || n2 == nil {
		return false
	}
	return reflect.DeepEqual(
		uniqueSortNodeSelectorTerms(n1.NodeSelectorTerms),
		uniqueSortNodeSelectorTerms(n2.NodeSelectorTerms),
	)
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

// sort by (key, value, effect, operand)
func uniqueSortTolerations(srcTolerations []v1.Toleration) []v1.Toleration {
	tolerations := append([]v1.Toleration{}, srcTolerations...)

	if len(tolerations) < 2 {
		return tolerations
	}

	sort.Slice(tolerations, func(i, j int) bool {
		if tolerations[i].Key < tolerations[j].Key {
			return true
		}
		if tolerations[i].Key > tolerations[j].Key {
			return false
		}
		if tolerations[i].Value < tolerations[j].Value {
			return true
		}
		if tolerations[i].Value > tolerations[j].Value {
			return false
		}
		if tolerations[i].Effect < tolerations[j].Effect {
			return true
		}
		if tolerations[i].Effect > tolerations[j].Effect {
			return false
		}
		return tolerations[i].Operator < tolerations[j].Operator
	})
	uniqueTolerations := []v1.Toleration{tolerations[0]}
	idx := 0
	for _, t := range tolerations[1:] {
		if t.MatchToleration(&uniqueTolerations[idx]) {
			continue
		}
		idx++
		uniqueTolerations = append(uniqueTolerations, t)
	}
	return uniqueTolerations
}

func TolerationsEqual(t1, t2 []v1.Toleration) bool {
	t1Sorted := uniqueSortTolerations(t1)
	t2Sorted := uniqueSortTolerations(t2)
	l1Len := len(t1Sorted)
	if l1Len != len(t2Sorted) {
		return false
	}
	for i := 0; i < l1Len; i++ {
		if !t1Sorted[i].MatchToleration(&t2Sorted[i]) {
			return false
		}
	}
	return true
}
