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

package predicates

import (
	"fmt"
	"reflect"
	"sort"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	fakelisters "k8s.io/kubernetes/pkg/scheduler/listers/fake"
	schedulernodeinfo "k8s.io/kubernetes/pkg/scheduler/nodeinfo"
	nodeinfosnapshot "k8s.io/kubernetes/pkg/scheduler/nodeinfo/snapshot"
	st "k8s.io/kubernetes/pkg/scheduler/testing"
)

// sortablePods lets us to sort pods.
type sortablePods []*v1.Pod

func (s sortablePods) Less(i, j int) bool {
	return s[i].Namespace < s[j].Namespace ||
		(s[i].Namespace == s[j].Namespace && s[i].Name < s[j].Name)
}
func (s sortablePods) Len() int      { return len(s) }
func (s sortablePods) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

var _ sort.Interface = &sortablePods{}

// sortableServices allows us to sort services.
type sortableServices []*v1.Service

func (s sortableServices) Less(i, j int) bool {
	return s[i].Namespace < s[j].Namespace ||
		(s[i].Namespace == s[j].Namespace && s[i].Name < s[j].Name)
}
func (s sortableServices) Len() int      { return len(s) }
func (s sortableServices) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

var _ sort.Interface = &sortableServices{}

// predicateMetadataEquivalent returns true if the two metadata are equivalent.
// Note: this function does not compare podRequest.
func predicateMetadataEquivalent(meta1, meta2 *predicateMetadata) error {
	if !reflect.DeepEqual(meta1.pod, meta2.pod) {
		return fmt.Errorf("pods are not the same")
	}
	if meta1.podBestEffort != meta2.podBestEffort {
		return fmt.Errorf("podBestEfforts are not equal")
	}
	if len(meta1.podFitsHostPortsMetadata.podPorts) != len(meta2.podFitsHostPortsMetadata.podPorts) {
		return fmt.Errorf("podPorts are not equal")
	}
	for !reflect.DeepEqual(meta1.podFitsHostPortsMetadata.podPorts, meta2.podFitsHostPortsMetadata.podPorts) {
		return fmt.Errorf("podPorts are not equal")
	}
	if !reflect.DeepEqual(meta1.podAffinityMetadata.topologyPairsPotentialAffinityPods, meta2.podAffinityMetadata.topologyPairsPotentialAffinityPods) {
		return fmt.Errorf("topologyPairsPotentialAffinityPods are not equal")
	}
	if !reflect.DeepEqual(meta1.podAffinityMetadata.topologyPairsPotentialAntiAffinityPods, meta2.podAffinityMetadata.topologyPairsPotentialAntiAffinityPods) {
		return fmt.Errorf("topologyPairsPotentialAntiAffinityPods are not equal")
	}
	if !reflect.DeepEqual(meta1.podAffinityMetadata.topologyPairsAntiAffinityPodsMap.podToTopologyPairs,
		meta2.podAffinityMetadata.topologyPairsAntiAffinityPodsMap.podToTopologyPairs) {
		return fmt.Errorf("topologyPairsAntiAffinityPodsMap.podToTopologyPairs are not equal")
	}
	if !reflect.DeepEqual(meta1.podAffinityMetadata.topologyPairsAntiAffinityPodsMap.topologyPairToPods,
		meta2.podAffinityMetadata.topologyPairsAntiAffinityPodsMap.topologyPairToPods) {
		return fmt.Errorf("topologyPairsAntiAffinityPodsMap.topologyPairToPods are not equal")
	}
	if meta1.serviceAffinityMetadata != nil {
		sortablePods1 := sortablePods(meta1.serviceAffinityMetadata.matchingPodList)
		sort.Sort(sortablePods1)
		sortablePods2 := sortablePods(meta2.serviceAffinityMetadata.matchingPodList)
		sort.Sort(sortablePods2)
		if !reflect.DeepEqual(sortablePods1, sortablePods2) {
			return fmt.Errorf("serviceAffinityMatchingPodLists are not euqal")
		}

		sortableServices1 := sortableServices(meta1.serviceAffinityMetadata.matchingPodServices)
		sort.Sort(sortableServices1)
		sortableServices2 := sortableServices(meta2.serviceAffinityMetadata.matchingPodServices)
		sort.Sort(sortableServices2)
		if !reflect.DeepEqual(sortableServices1, sortableServices2) {
			return fmt.Errorf("serviceAffinityMatchingPodServices are not euqal")
		}
	}
	return nil
}

func TestPredicateMetadata_AddRemovePod(t *testing.T) {
	var label1 = map[string]string{
		"region": "r1",
		"zone":   "z11",
	}
	var label2 = map[string]string{
		"region": "r1",
		"zone":   "z12",
	}
	var label3 = map[string]string{
		"region": "r2",
		"zone":   "z21",
	}
	selector1 := map[string]string{"foo": "bar"}
	antiAffinityFooBar := &v1.PodAntiAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
			{
				LabelSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "foo",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"bar"},
						},
					},
				},
				TopologyKey: "region",
			},
		},
	}
	antiAffinityComplex := &v1.PodAntiAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
			{
				LabelSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "foo",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"bar", "buzz"},
						},
					},
				},
				TopologyKey: "region",
			},
			{
				LabelSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "service",
							Operator: metav1.LabelSelectorOpNotIn,
							Values:   []string{"bar", "security", "test"},
						},
					},
				},
				TopologyKey: "zone",
			},
		},
	}
	affinityComplex := &v1.PodAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
			{
				LabelSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "foo",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"bar", "buzz"},
						},
					},
				},
				TopologyKey: "region",
			},
			{
				LabelSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "service",
							Operator: metav1.LabelSelectorOpNotIn,
							Values:   []string{"bar", "security", "test"},
						},
					},
				},
				TopologyKey: "zone",
			},
		},
	}

	tests := []struct {
		name         string
		pendingPod   *v1.Pod
		addedPod     *v1.Pod
		existingPods []*v1.Pod
		nodes        []*v1.Node
		services     []*v1.Service
	}{
		{
			name: "no anti-affinity or service affinity exist",
			pendingPod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "pending", Labels: selector1},
			},
			existingPods: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "p1", Labels: selector1},
					Spec: v1.PodSpec{NodeName: "nodeA"},
				},
				{ObjectMeta: metav1.ObjectMeta{Name: "p2"},
					Spec: v1.PodSpec{NodeName: "nodeC"},
				},
			},
			addedPod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "addedPod", Labels: selector1},
				Spec:       v1.PodSpec{NodeName: "nodeB"},
			},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeA", Labels: label1}},
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeB", Labels: label2}},
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeC", Labels: label3}},
			},
		},
		{
			name: "metadata anti-affinity terms are updated correctly after adding and removing a pod",
			pendingPod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "pending", Labels: selector1},
			},
			existingPods: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "p1", Labels: selector1},
					Spec: v1.PodSpec{NodeName: "nodeA"},
				},
				{ObjectMeta: metav1.ObjectMeta{Name: "p2"},
					Spec: v1.PodSpec{
						NodeName: "nodeC",
						Affinity: &v1.Affinity{
							PodAntiAffinity: antiAffinityFooBar,
						},
					},
				},
			},
			addedPod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "addedPod", Labels: selector1},
				Spec: v1.PodSpec{
					NodeName: "nodeB",
					Affinity: &v1.Affinity{
						PodAntiAffinity: antiAffinityFooBar,
					},
				},
			},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeA", Labels: label1}},
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeB", Labels: label2}},
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeC", Labels: label3}},
			},
		},
		{
			name: "metadata service-affinity data are updated correctly after adding and removing a pod",
			pendingPod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "pending", Labels: selector1},
			},
			existingPods: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "p1", Labels: selector1},
					Spec: v1.PodSpec{NodeName: "nodeA"},
				},
				{ObjectMeta: metav1.ObjectMeta{Name: "p2"},
					Spec: v1.PodSpec{NodeName: "nodeC"},
				},
			},
			addedPod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "addedPod", Labels: selector1},
				Spec:       v1.PodSpec{NodeName: "nodeB"},
			},
			services: []*v1.Service{{Spec: v1.ServiceSpec{Selector: selector1}}},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeA", Labels: label1}},
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeB", Labels: label2}},
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeC", Labels: label3}},
			},
		},
		{
			name: "metadata anti-affinity terms and service affinity data are updated correctly after adding and removing a pod",
			pendingPod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "pending", Labels: selector1},
			},
			existingPods: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "p1", Labels: selector1},
					Spec: v1.PodSpec{NodeName: "nodeA"},
				},
				{ObjectMeta: metav1.ObjectMeta{Name: "p2"},
					Spec: v1.PodSpec{
						NodeName: "nodeC",
						Affinity: &v1.Affinity{
							PodAntiAffinity: antiAffinityFooBar,
						},
					},
				},
			},
			addedPod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "addedPod", Labels: selector1},
				Spec: v1.PodSpec{
					NodeName: "nodeA",
					Affinity: &v1.Affinity{
						PodAntiAffinity: antiAffinityComplex,
					},
				},
			},
			services: []*v1.Service{{Spec: v1.ServiceSpec{Selector: selector1}}},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeA", Labels: label1}},
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeB", Labels: label2}},
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeC", Labels: label3}},
			},
		},
		{
			name: "metadata matching pod affinity and anti-affinity are updated correctly after adding and removing a pod",
			pendingPod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "pending", Labels: selector1},
			},
			existingPods: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "p1", Labels: selector1},
					Spec: v1.PodSpec{NodeName: "nodeA"},
				},
				{ObjectMeta: metav1.ObjectMeta{Name: "p2"},
					Spec: v1.PodSpec{
						NodeName: "nodeC",
						Affinity: &v1.Affinity{
							PodAntiAffinity: antiAffinityFooBar,
							PodAffinity:     affinityComplex,
						},
					},
				},
			},
			addedPod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "addedPod", Labels: selector1},
				Spec: v1.PodSpec{
					NodeName: "nodeA",
					Affinity: &v1.Affinity{
						PodAntiAffinity: antiAffinityComplex,
					},
				},
			},
			services: []*v1.Service{{Spec: v1.ServiceSpec{Selector: selector1}}},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeA", Labels: label1}},
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeB", Labels: label2}},
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeC", Labels: label3}},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			allPodLister := fakelisters.PodLister(append(test.existingPods, test.addedPod))
			// getMeta creates predicate meta data given the list of pods.
			getMeta := func(pods []*v1.Pod) (*predicateMetadata, map[string]*schedulernodeinfo.NodeInfo) {
				s := nodeinfosnapshot.NewSnapshot(nodeinfosnapshot.CreateNodeInfoMap(pods, test.nodes))
				_, precompute := NewServiceAffinityPredicate(s.NodeInfos(), s.Pods(), fakelisters.ServiceLister(test.services), nil)
				RegisterPredicateMetadataProducer("ServiceAffinityMetaProducer", precompute)
				factory := &MetadataProducerFactory{}
				meta := factory.GetPredicateMetadata(test.pendingPod, s)
				return meta.(*predicateMetadata), s.NodeInfoMap
			}

			// allPodsMeta is meta data produced when all pods, including test.addedPod
			// are given to the metadata producer.
			allPodsMeta, _ := getMeta(allPodLister)
			// existingPodsMeta1 is meta data produced for test.existingPods (without test.addedPod).
			existingPodsMeta1, nodeInfoMap := getMeta(test.existingPods)
			// Add test.addedPod to existingPodsMeta1 and make sure meta is equal to allPodsMeta
			nodeInfo := nodeInfoMap[test.addedPod.Spec.NodeName]
			if err := existingPodsMeta1.AddPod(test.addedPod, nodeInfo.Node()); err != nil {
				t.Errorf("error adding pod to meta: %v", err)
			}
			if err := predicateMetadataEquivalent(allPodsMeta, existingPodsMeta1); err != nil {
				t.Errorf("meta data are not equivalent: %v", err)
			}
			// Remove the added pod and from existingPodsMeta1 an make sure it is equal
			// to meta generated for existing pods.
			existingPodsMeta2, _ := getMeta(fakelisters.PodLister(test.existingPods))
			if err := existingPodsMeta1.RemovePod(test.addedPod, nil); err != nil {
				t.Errorf("error removing pod from meta: %v", err)
			}
			if err := predicateMetadataEquivalent(existingPodsMeta1, existingPodsMeta2); err != nil {
				t.Errorf("meta data are not equivalent: %v", err)
			}
		})
	}
}

// TestPredicateMetadata_ShallowCopy tests the ShallowCopy function. It is based
// on the idea that shallow-copy should produce an object that is deep-equal to the original
// object.
func TestPredicateMetadata_ShallowCopy(t *testing.T) {
	selector1 := map[string]string{"foo": "bar"}
	source := predicateMetadata{
		pod: &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "testns",
			},
		},
		podBestEffort: true,
		podFitsResourcesMetadata: &podFitsResourcesMetadata{
			podRequest: &schedulernodeinfo.Resource{
				MilliCPU:         1000,
				Memory:           300,
				AllowedPodNumber: 4,
			},
		},
		podFitsHostPortsMetadata: &podFitsHostPortsMetadata{
			podPorts: []*v1.ContainerPort{
				{
					Name:          "name",
					HostPort:      10,
					ContainerPort: 20,
					Protocol:      "TCP",
					HostIP:        "1.2.3.4",
				},
			},
		},
		podAffinityMetadata: &podAffinityMetadata{
			topologyPairsAntiAffinityPodsMap: &topologyPairsMaps{
				topologyPairToPods: map[topologyPair]podSet{
					{key: "name", value: "machine1"}: {
						&v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p2", Labels: selector1},
							Spec: v1.PodSpec{NodeName: "nodeC"},
						}: struct{}{},
					},
					{key: "name", value: "machine2"}: {
						&v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p1", Labels: selector1},
							Spec: v1.PodSpec{NodeName: "nodeA"},
						}: struct{}{},
					},
				},
				podToTopologyPairs: map[string]topologyPairSet{
					"p2_": {
						topologyPair{key: "name", value: "machine1"}: struct{}{},
					},
					"p1_": {
						topologyPair{key: "name", value: "machine2"}: struct{}{},
					},
				},
			},
			topologyPairsPotentialAffinityPods: &topologyPairsMaps{
				topologyPairToPods: map[topologyPair]podSet{
					{key: "name", value: "nodeA"}: {
						&v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p1", Labels: selector1},
							Spec: v1.PodSpec{NodeName: "nodeA"},
						}: struct{}{},
					},
					{key: "name", value: "nodeC"}: {
						&v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p2"},
							Spec: v1.PodSpec{
								NodeName: "nodeC",
							},
						}: struct{}{},
						&v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p6", Labels: selector1},
							Spec: v1.PodSpec{NodeName: "nodeC"},
						}: struct{}{},
					},
				},
				podToTopologyPairs: map[string]topologyPairSet{
					"p1_": {
						topologyPair{key: "name", value: "nodeA"}: struct{}{},
					},
					"p2_": {
						topologyPair{key: "name", value: "nodeC"}: struct{}{},
					},
					"p6_": {
						topologyPair{key: "name", value: "nodeC"}: struct{}{},
					},
				},
			},
			topologyPairsPotentialAntiAffinityPods: &topologyPairsMaps{
				topologyPairToPods: map[topologyPair]podSet{
					{key: "name", value: "nodeN"}: {
						&v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p1", Labels: selector1},
							Spec: v1.PodSpec{NodeName: "nodeN"},
						}: struct{}{},
						&v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p2"},
							Spec: v1.PodSpec{
								NodeName: "nodeM",
							},
						}: struct{}{},
						&v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p3"},
							Spec: v1.PodSpec{
								NodeName: "nodeM",
							},
						}: struct{}{},
					},
					{key: "name", value: "nodeM"}: {
						&v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p6", Labels: selector1},
							Spec: v1.PodSpec{NodeName: "nodeM"},
						}: struct{}{},
					},
				},
				podToTopologyPairs: map[string]topologyPairSet{
					"p1_": {
						topologyPair{key: "name", value: "nodeN"}: struct{}{},
					},
					"p2_": {
						topologyPair{key: "name", value: "nodeN"}: struct{}{},
					},
					"p3_": {
						topologyPair{key: "name", value: "nodeN"}: struct{}{},
					},
					"p6_": {
						topologyPair{key: "name", value: "nodeM"}: struct{}{},
					},
				},
			},
		},
		evenPodsSpreadMetadata: &evenPodsSpreadMetadata{
			tpKeyToCriticalPaths: map[string]*criticalPaths{
				"name": {{"nodeA", 1}, {"nodeC", 2}},
			},
			tpPairToMatchNum: map[topologyPair]int32{
				{key: "name", value: "nodeA"}: 1,
				{key: "name", value: "nodeC"}: 2,
			},
		},
		serviceAffinityMetadata: &serviceAffinityMetadata{
			matchingPodList: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "pod1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pod2"}},
			},
			matchingPodServices: []*v1.Service{
				{ObjectMeta: metav1.ObjectMeta{Name: "service1"}},
			},
		},
	}

	if !reflect.DeepEqual(source.ShallowCopy().(*predicateMetadata), &source) {
		t.Errorf("Copy is not equal to source!")
	}
}

// TestGetTPMapMatchingIncomingAffinityAntiAffinity tests against method getTPMapMatchingIncomingAffinityAntiAffinity
// on Anti Affinity cases
func TestGetTPMapMatchingIncomingAffinityAntiAffinity(t *testing.T) {
	newPodAffinityTerms := func(keys ...string) []v1.PodAffinityTerm {
		var terms []v1.PodAffinityTerm
		for _, key := range keys {
			terms = append(terms, v1.PodAffinityTerm{
				LabelSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      key,
							Operator: metav1.LabelSelectorOpExists,
						},
					},
				},
				TopologyKey: "hostname",
			})
		}
		return terms
	}
	newPod := func(labels ...string) *v1.Pod {
		labelMap := make(map[string]string)
		for _, l := range labels {
			labelMap[l] = ""
		}
		return &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "normal", Labels: labelMap},
			Spec:       v1.PodSpec{NodeName: "nodeA"},
		}
	}
	normalPodA := newPod("aaa")
	normalPodB := newPod("bbb")
	normalPodAB := newPod("aaa", "bbb")
	nodeA := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "nodeA", Labels: map[string]string{"hostname": "nodeA"}}}

	tests := []struct {
		name                     string
		existingPods             []*v1.Pod
		nodes                    []*v1.Node
		pod                      *v1.Pod
		wantAffinityPodsMaps     *topologyPairsMaps
		wantAntiAffinityPodsMaps *topologyPairsMaps
		wantErr                  bool
	}{
		{
			name:  "nil test",
			nodes: []*v1.Node{nodeA},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "aaa-normal"},
			},
			wantAffinityPodsMaps:     newTopologyPairsMaps(),
			wantAntiAffinityPodsMaps: newTopologyPairsMaps(),
		},
		{
			name:         "incoming pod without affinity/anti-affinity causes a no-op",
			existingPods: []*v1.Pod{normalPodA},
			nodes:        []*v1.Node{nodeA},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "aaa-normal"},
			},
			wantAffinityPodsMaps:     newTopologyPairsMaps(),
			wantAntiAffinityPodsMaps: newTopologyPairsMaps(),
		},
		{
			name:         "no pod has label that violates incoming pod's affinity and anti-affinity",
			existingPods: []*v1.Pod{normalPodB},
			nodes:        []*v1.Node{nodeA},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "aaa-anti"},
				Spec: v1.PodSpec{
					Affinity: &v1.Affinity{
						PodAffinity: &v1.PodAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: newPodAffinityTerms("aaa"),
						},
						PodAntiAffinity: &v1.PodAntiAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: newPodAffinityTerms("aaa"),
						},
					},
				},
			},
			wantAffinityPodsMaps:     newTopologyPairsMaps(),
			wantAntiAffinityPodsMaps: newTopologyPairsMaps(),
		},
		{
			name:         "existing pod matches incoming pod's affinity and anti-affinity - single term case",
			existingPods: []*v1.Pod{normalPodA},
			nodes:        []*v1.Node{nodeA},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "affi-antiaffi"},
				Spec: v1.PodSpec{
					Affinity: &v1.Affinity{
						PodAffinity: &v1.PodAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: newPodAffinityTerms("aaa"),
						},
						PodAntiAffinity: &v1.PodAntiAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: newPodAffinityTerms("aaa"),
						},
					},
				},
			},
			wantAffinityPodsMaps: &topologyPairsMaps{
				topologyPairToPods: map[topologyPair]podSet{
					{key: "hostname", value: "nodeA"}: {normalPodA: struct{}{}},
				},
				podToTopologyPairs: map[string]topologyPairSet{
					"normal_": {
						topologyPair{key: "hostname", value: "nodeA"}: struct{}{},
					},
				},
			},
			wantAntiAffinityPodsMaps: &topologyPairsMaps{
				topologyPairToPods: map[topologyPair]podSet{
					{key: "hostname", value: "nodeA"}: {normalPodA: struct{}{}},
				},
				podToTopologyPairs: map[string]topologyPairSet{
					"normal_": {
						topologyPair{key: "hostname", value: "nodeA"}: struct{}{},
					},
				},
			},
		},
		{
			name:         "existing pod matches incoming pod's affinity and anti-affinity - mutiple terms case",
			existingPods: []*v1.Pod{normalPodAB},
			nodes:        []*v1.Node{nodeA},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "affi-antiaffi"},
				Spec: v1.PodSpec{
					Affinity: &v1.Affinity{
						PodAffinity: &v1.PodAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: newPodAffinityTerms("aaa", "bbb"),
						},
						PodAntiAffinity: &v1.PodAntiAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: newPodAffinityTerms("aaa"),
						},
					},
				},
			},
			wantAffinityPodsMaps: &topologyPairsMaps{
				topologyPairToPods: map[topologyPair]podSet{
					{key: "hostname", value: "nodeA"}: {normalPodAB: struct{}{}},
				},
				podToTopologyPairs: map[string]topologyPairSet{
					"normal_": {
						topologyPair{key: "hostname", value: "nodeA"}: struct{}{},
					},
				},
			},
			wantAntiAffinityPodsMaps: &topologyPairsMaps{
				topologyPairToPods: map[topologyPair]podSet{
					{key: "hostname", value: "nodeA"}: {normalPodAB: struct{}{}},
				},
				podToTopologyPairs: map[string]topologyPairSet{
					"normal_": {
						topologyPair{key: "hostname", value: "nodeA"}: struct{}{},
					},
				},
			},
		},
		{
			name:         "existing pod not match incoming pod's affinity but matches anti-affinity",
			existingPods: []*v1.Pod{normalPodA},
			nodes:        []*v1.Node{nodeA},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "affi-antiaffi"},
				Spec: v1.PodSpec{
					Affinity: &v1.Affinity{
						PodAffinity: &v1.PodAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: newPodAffinityTerms("aaa", "bbb"),
						},
						PodAntiAffinity: &v1.PodAntiAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: newPodAffinityTerms("aaa", "bbb"),
						},
					},
				},
			},
			wantAffinityPodsMaps: newTopologyPairsMaps(),
			wantAntiAffinityPodsMaps: &topologyPairsMaps{
				topologyPairToPods: map[topologyPair]podSet{
					{key: "hostname", value: "nodeA"}: {normalPodA: struct{}{}},
				},
				podToTopologyPairs: map[string]topologyPairSet{
					"normal_": {
						topologyPair{key: "hostname", value: "nodeA"}: struct{}{},
					},
				},
			},
		},
		{
			name:         "incoming pod's anti-affinity has more than one term - existing pod violates partial term - case 1",
			existingPods: []*v1.Pod{normalPodAB},
			nodes:        []*v1.Node{nodeA},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "anaffi-antiaffiti"},
				Spec: v1.PodSpec{
					Affinity: &v1.Affinity{
						PodAffinity: &v1.PodAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: newPodAffinityTerms("aaa", "ccc"),
						},
						PodAntiAffinity: &v1.PodAntiAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: newPodAffinityTerms("aaa", "ccc"),
						},
					},
				},
			},
			wantAffinityPodsMaps: newTopologyPairsMaps(),
			wantAntiAffinityPodsMaps: &topologyPairsMaps{
				topologyPairToPods: map[topologyPair]podSet{
					{key: "hostname", value: "nodeA"}: {normalPodAB: struct{}{}},
				},
				podToTopologyPairs: map[string]topologyPairSet{
					"normal_": {
						topologyPair{key: "hostname", value: "nodeA"}: struct{}{},
					},
				},
			},
		},
		{
			name:         "incoming pod's anti-affinity has more than one term - existing pod violates partial term - case 2",
			existingPods: []*v1.Pod{normalPodB},
			nodes:        []*v1.Node{nodeA},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "affi-antiaffi"},
				Spec: v1.PodSpec{
					Affinity: &v1.Affinity{
						PodAffinity: &v1.PodAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: newPodAffinityTerms("aaa", "bbb"),
						},
						PodAntiAffinity: &v1.PodAntiAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: newPodAffinityTerms("aaa", "bbb"),
						},
					},
				},
			},
			wantAffinityPodsMaps: newTopologyPairsMaps(),
			wantAntiAffinityPodsMaps: &topologyPairsMaps{
				topologyPairToPods: map[topologyPair]podSet{
					{key: "hostname", value: "nodeA"}: {normalPodB: struct{}{}},
				},
				podToTopologyPairs: map[string]topologyPairSet{
					"normal_": {
						topologyPair{key: "hostname", value: "nodeA"}: struct{}{},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := nodeinfosnapshot.NewSnapshot(nodeinfosnapshot.CreateNodeInfoMap(tt.existingPods, tt.nodes))
			l, _ := s.NodeInfos().List()
			gotAffinityPodsMaps, gotAntiAffinityPodsMaps, err := getTPMapMatchingIncomingAffinityAntiAffinity(tt.pod, l)
			if (err != nil) != tt.wantErr {
				t.Errorf("getTPMapMatchingIncomingAffinityAntiAffinity() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotAffinityPodsMaps, tt.wantAffinityPodsMaps) {
				t.Errorf("getTPMapMatchingIncomingAffinityAntiAffinity() gotAffinityPodsMaps = %#v, want %#v", gotAffinityPodsMaps, tt.wantAffinityPodsMaps)
			}
			if !reflect.DeepEqual(gotAntiAffinityPodsMaps, tt.wantAntiAffinityPodsMaps) {
				t.Errorf("getTPMapMatchingIncomingAffinityAntiAffinity() gotAntiAffinityPodsMaps = %#v, want %#v", gotAntiAffinityPodsMaps, tt.wantAntiAffinityPodsMaps)
			}
		})
	}
}

func TestGetTPMapMatchingSpreadConstraints(t *testing.T) {
	fooSelector := st.MakeLabelSelector().Exists("foo").Obj()
	barSelector := st.MakeLabelSelector().Exists("bar").Obj()
	tests := []struct {
		name         string
		pod          *v1.Pod
		nodes        []*v1.Node
		existingPods []*v1.Pod
		want         *evenPodsSpreadMetadata
	}{
		{
			name: "clean cluster with one spreadConstraint",
			pod: st.MakePod().Name("p").Label("foo", "").SpreadConstraint(
				5, "zone", hardSpread, st.MakeLabelSelector().Label("foo", "bar").Obj(),
			).Obj(),
			nodes: []*v1.Node{
				st.MakeNode().Name("node-a").Label("zone", "zone1").Label("node", "node-a").Obj(),
				st.MakeNode().Name("node-b").Label("zone", "zone1").Label("node", "node-b").Obj(),
				st.MakeNode().Name("node-x").Label("zone", "zone2").Label("node", "node-x").Obj(),
				st.MakeNode().Name("node-y").Label("zone", "zone2").Label("node", "node-y").Obj(),
			},
			want: &evenPodsSpreadMetadata{
				constraints: []topologySpreadConstraint{
					{
						maxSkew:     5,
						topologyKey: "zone",
						selector:    mustConvertLabelSelectorAsSelector(t, st.MakeLabelSelector().Label("foo", "bar").Obj()),
					},
				},
				tpKeyToCriticalPaths: map[string]*criticalPaths{
					"zone": {{"zone1", 0}, {"zone2", 0}},
				},
				tpPairToMatchNum: map[topologyPair]int32{
					{key: "zone", value: "zone1"}: 0,
					{key: "zone", value: "zone2"}: 0,
				},
			},
		},
		{
			name: "normal case with one spreadConstraint",
			pod: st.MakePod().Name("p").Label("foo", "").SpreadConstraint(
				1, "zone", hardSpread, fooSelector,
			).Obj(),
			nodes: []*v1.Node{
				st.MakeNode().Name("node-a").Label("zone", "zone1").Label("node", "node-a").Obj(),
				st.MakeNode().Name("node-b").Label("zone", "zone1").Label("node", "node-b").Obj(),
				st.MakeNode().Name("node-x").Label("zone", "zone2").Label("node", "node-x").Obj(),
				st.MakeNode().Name("node-y").Label("zone", "zone2").Label("node", "node-y").Obj(),
			},
			existingPods: []*v1.Pod{
				st.MakePod().Name("p-a1").Node("node-a").Label("foo", "").Obj(),
				st.MakePod().Name("p-a2").Node("node-a").Label("foo", "").Obj(),
				st.MakePod().Name("p-b1").Node("node-b").Label("foo", "").Obj(),
				st.MakePod().Name("p-y1").Node("node-y").Label("foo", "").Obj(),
				st.MakePod().Name("p-y2").Node("node-y").Label("foo", "").Obj(),
			},
			want: &evenPodsSpreadMetadata{
				constraints: []topologySpreadConstraint{
					{
						maxSkew:     1,
						topologyKey: "zone",
						selector:    mustConvertLabelSelectorAsSelector(t, fooSelector),
					},
				},
				tpKeyToCriticalPaths: map[string]*criticalPaths{
					"zone": {{"zone2", 2}, {"zone1", 3}},
				},
				tpPairToMatchNum: map[topologyPair]int32{
					{key: "zone", value: "zone1"}: 3,
					{key: "zone", value: "zone2"}: 2,
				},
			},
		},
		{
			name: "normal case with one spreadConstraint, on a 3-zone cluster",
			pod: st.MakePod().Name("p").Label("foo", "").SpreadConstraint(
				1, "zone", hardSpread, st.MakeLabelSelector().Exists("foo").Obj(),
			).Obj(),
			nodes: []*v1.Node{
				st.MakeNode().Name("node-a").Label("zone", "zone1").Label("node", "node-a").Obj(),
				st.MakeNode().Name("node-b").Label("zone", "zone1").Label("node", "node-b").Obj(),
				st.MakeNode().Name("node-x").Label("zone", "zone2").Label("node", "node-x").Obj(),
				st.MakeNode().Name("node-y").Label("zone", "zone2").Label("node", "node-y").Obj(),
				st.MakeNode().Name("node-o").Label("zone", "zone3").Label("node", "node-o").Obj(),
				st.MakeNode().Name("node-p").Label("zone", "zone3").Label("node", "node-p").Obj(),
			},
			existingPods: []*v1.Pod{
				st.MakePod().Name("p-a1").Node("node-a").Label("foo", "").Obj(),
				st.MakePod().Name("p-a2").Node("node-a").Label("foo", "").Obj(),
				st.MakePod().Name("p-b1").Node("node-b").Label("foo", "").Obj(),
				st.MakePod().Name("p-y1").Node("node-y").Label("foo", "").Obj(),
				st.MakePod().Name("p-y2").Node("node-y").Label("foo", "").Obj(),
			},
			want: &evenPodsSpreadMetadata{
				constraints: []topologySpreadConstraint{
					{
						maxSkew:     1,
						topologyKey: "zone",
						selector:    mustConvertLabelSelectorAsSelector(t, fooSelector),
					},
				},
				tpKeyToCriticalPaths: map[string]*criticalPaths{
					"zone": {{"zone3", 0}, {"zone2", 2}},
				},
				tpPairToMatchNum: map[topologyPair]int32{
					{key: "zone", value: "zone1"}: 3,
					{key: "zone", value: "zone2"}: 2,
					{key: "zone", value: "zone3"}: 0,
				},
			},
		},
		{
			name: "namespace mismatch doesn't count",
			pod: st.MakePod().Name("p").Label("foo", "").SpreadConstraint(
				1, "zone", hardSpread, fooSelector,
			).Obj(),
			nodes: []*v1.Node{
				st.MakeNode().Name("node-a").Label("zone", "zone1").Label("node", "node-a").Obj(),
				st.MakeNode().Name("node-b").Label("zone", "zone1").Label("node", "node-b").Obj(),
				st.MakeNode().Name("node-x").Label("zone", "zone2").Label("node", "node-x").Obj(),
				st.MakeNode().Name("node-y").Label("zone", "zone2").Label("node", "node-y").Obj(),
			},
			existingPods: []*v1.Pod{
				st.MakePod().Name("p-a1").Node("node-a").Label("foo", "").Obj(),
				st.MakePod().Name("p-a2").Namespace("ns1").Node("node-a").Label("foo", "").Obj(),
				st.MakePod().Name("p-b1").Node("node-b").Label("foo", "").Obj(),
				st.MakePod().Name("p-y1").Namespace("ns2").Node("node-y").Label("foo", "").Obj(),
				st.MakePod().Name("p-y2").Node("node-y").Label("foo", "").Obj(),
			},
			want: &evenPodsSpreadMetadata{
				constraints: []topologySpreadConstraint{
					{
						maxSkew:     1,
						topologyKey: "zone",
						selector:    mustConvertLabelSelectorAsSelector(t, fooSelector),
					},
				},
				tpKeyToCriticalPaths: map[string]*criticalPaths{
					"zone": {{"zone2", 1}, {"zone1", 2}},
				},
				tpPairToMatchNum: map[topologyPair]int32{
					{key: "zone", value: "zone1"}: 2,
					{key: "zone", value: "zone2"}: 1,
				},
			},
		},
		{
			name: "normal case with two spreadConstraints",
			pod: st.MakePod().Name("p").Label("foo", "").
				SpreadConstraint(1, "zone", hardSpread, fooSelector).
				SpreadConstraint(1, "node", hardSpread, fooSelector).
				Obj(),
			nodes: []*v1.Node{
				st.MakeNode().Name("node-a").Label("zone", "zone1").Label("node", "node-a").Obj(),
				st.MakeNode().Name("node-b").Label("zone", "zone1").Label("node", "node-b").Obj(),
				st.MakeNode().Name("node-x").Label("zone", "zone2").Label("node", "node-x").Obj(),
				st.MakeNode().Name("node-y").Label("zone", "zone2").Label("node", "node-y").Obj(),
			},
			existingPods: []*v1.Pod{
				st.MakePod().Name("p-a1").Node("node-a").Label("foo", "").Obj(),
				st.MakePod().Name("p-a2").Node("node-a").Label("foo", "").Obj(),
				st.MakePod().Name("p-b1").Node("node-b").Label("foo", "").Obj(),
				st.MakePod().Name("p-y1").Node("node-y").Label("foo", "").Obj(),
				st.MakePod().Name("p-y2").Node("node-y").Label("foo", "").Obj(),
				st.MakePod().Name("p-y3").Node("node-y").Label("foo", "").Obj(),
				st.MakePod().Name("p-y4").Node("node-y").Label("foo", "").Obj(),
			},
			want: &evenPodsSpreadMetadata{
				constraints: []topologySpreadConstraint{
					{
						maxSkew:     1,
						topologyKey: "zone",
						selector:    mustConvertLabelSelectorAsSelector(t, fooSelector),
					},
					{
						maxSkew:     1,
						topologyKey: "node",
						selector:    mustConvertLabelSelectorAsSelector(t, fooSelector),
					},
				},
				tpKeyToCriticalPaths: map[string]*criticalPaths{
					"zone": {{"zone1", 3}, {"zone2", 4}},
					"node": {{"node-x", 0}, {"node-b", 1}},
				},
				tpPairToMatchNum: map[topologyPair]int32{
					{key: "zone", value: "zone1"}:  3,
					{key: "zone", value: "zone2"}:  4,
					{key: "node", value: "node-a"}: 2,
					{key: "node", value: "node-b"}: 1,
					{key: "node", value: "node-x"}: 0,
					{key: "node", value: "node-y"}: 4,
				},
			},
		},
		{
			name: "soft spreadConstraints should be bypassed",
			pod: st.MakePod().Name("p").Label("foo", "").
				SpreadConstraint(1, "zone", softSpread, fooSelector).
				SpreadConstraint(1, "zone", hardSpread, fooSelector).
				SpreadConstraint(1, "node", softSpread, fooSelector).
				SpreadConstraint(1, "node", hardSpread, fooSelector).
				Obj(),
			nodes: []*v1.Node{
				st.MakeNode().Name("node-a").Label("zone", "zone1").Label("node", "node-a").Obj(),
				st.MakeNode().Name("node-b").Label("zone", "zone1").Label("node", "node-b").Obj(),
				st.MakeNode().Name("node-y").Label("zone", "zone2").Label("node", "node-y").Obj(),
			},
			existingPods: []*v1.Pod{
				st.MakePod().Name("p-a1").Node("node-a").Label("foo", "").Obj(),
				st.MakePod().Name("p-a2").Node("node-a").Label("foo", "").Obj(),
				st.MakePod().Name("p-b1").Node("node-b").Label("foo", "").Obj(),
				st.MakePod().Name("p-y1").Node("node-y").Label("foo", "").Obj(),
				st.MakePod().Name("p-y2").Node("node-y").Label("foo", "").Obj(),
				st.MakePod().Name("p-y3").Node("node-y").Label("foo", "").Obj(),
				st.MakePod().Name("p-y4").Node("node-y").Label("foo", "").Obj(),
			},
			want: &evenPodsSpreadMetadata{
				constraints: []topologySpreadConstraint{
					{
						maxSkew:     1,
						topologyKey: "zone",
						selector:    mustConvertLabelSelectorAsSelector(t, fooSelector),
					},
					{
						maxSkew:     1,
						topologyKey: "node",
						selector:    mustConvertLabelSelectorAsSelector(t, fooSelector),
					},
				},
				tpKeyToCriticalPaths: map[string]*criticalPaths{
					"zone": {{"zone1", 3}, {"zone2", 4}},
					"node": {{"node-b", 1}, {"node-a", 2}},
				},
				tpPairToMatchNum: map[topologyPair]int32{
					{key: "zone", value: "zone1"}:  3,
					{key: "zone", value: "zone2"}:  4,
					{key: "node", value: "node-a"}: 2,
					{key: "node", value: "node-b"}: 1,
					{key: "node", value: "node-y"}: 4,
				},
			},
		},
		{
			name: "different labelSelectors - simple version",
			pod: st.MakePod().Name("p").Label("foo", "").Label("bar", "").
				SpreadConstraint(1, "zone", hardSpread, fooSelector).
				SpreadConstraint(1, "node", hardSpread, barSelector).
				Obj(),
			nodes: []*v1.Node{
				st.MakeNode().Name("node-a").Label("zone", "zone1").Label("node", "node-a").Obj(),
				st.MakeNode().Name("node-b").Label("zone", "zone1").Label("node", "node-b").Obj(),
				st.MakeNode().Name("node-y").Label("zone", "zone2").Label("node", "node-y").Obj(),
			},
			existingPods: []*v1.Pod{
				st.MakePod().Name("p-a").Node("node-a").Label("foo", "").Obj(),
				st.MakePod().Name("p-b").Node("node-b").Label("bar", "").Obj(),
			},
			want: &evenPodsSpreadMetadata{
				constraints: []topologySpreadConstraint{
					{
						maxSkew:     1,
						topologyKey: "zone",
						selector:    mustConvertLabelSelectorAsSelector(t, fooSelector),
					},
					{
						maxSkew:     1,
						topologyKey: "node",
						selector:    mustConvertLabelSelectorAsSelector(t, barSelector),
					},
				},
				tpKeyToCriticalPaths: map[string]*criticalPaths{
					"zone": {{"zone2", 0}, {"zone1", 1}},
					"node": {{"node-a", 0}, {"node-y", 0}},
				},
				tpPairToMatchNum: map[topologyPair]int32{
					{key: "zone", value: "zone1"}:  1,
					{key: "zone", value: "zone2"}:  0,
					{key: "node", value: "node-a"}: 0,
					{key: "node", value: "node-b"}: 1,
					{key: "node", value: "node-y"}: 0,
				},
			},
		},
		{
			name: "different labelSelectors - complex pods",
			pod: st.MakePod().Name("p").Label("foo", "").Label("bar", "").
				SpreadConstraint(1, "zone", hardSpread, fooSelector).
				SpreadConstraint(1, "node", hardSpread, barSelector).
				Obj(),
			nodes: []*v1.Node{
				st.MakeNode().Name("node-a").Label("zone", "zone1").Label("node", "node-a").Obj(),
				st.MakeNode().Name("node-b").Label("zone", "zone1").Label("node", "node-b").Obj(),
				st.MakeNode().Name("node-y").Label("zone", "zone2").Label("node", "node-y").Obj(),
			},
			existingPods: []*v1.Pod{
				st.MakePod().Name("p-a1").Node("node-a").Label("foo", "").Obj(),
				st.MakePod().Name("p-a2").Node("node-a").Label("foo", "").Label("bar", "").Obj(),
				st.MakePod().Name("p-b1").Node("node-b").Label("foo", "").Obj(),
				st.MakePod().Name("p-y1").Node("node-y").Label("foo", "").Obj(),
				st.MakePod().Name("p-y2").Node("node-y").Label("foo", "").Label("bar", "").Obj(),
				st.MakePod().Name("p-y3").Node("node-y").Label("foo", "").Obj(),
				st.MakePod().Name("p-y4").Node("node-y").Label("foo", "").Label("bar", "").Obj(),
			},
			want: &evenPodsSpreadMetadata{
				constraints: []topologySpreadConstraint{
					{
						maxSkew:     1,
						topologyKey: "zone",
						selector:    mustConvertLabelSelectorAsSelector(t, fooSelector),
					},
					{
						maxSkew:     1,
						topologyKey: "node",
						selector:    mustConvertLabelSelectorAsSelector(t, barSelector),
					},
				},
				tpKeyToCriticalPaths: map[string]*criticalPaths{
					"zone": {{"zone1", 3}, {"zone2", 4}},
					"node": {{"node-b", 0}, {"node-a", 1}},
				},
				tpPairToMatchNum: map[topologyPair]int32{
					{key: "zone", value: "zone1"}:  3,
					{key: "zone", value: "zone2"}:  4,
					{key: "node", value: "node-a"}: 1,
					{key: "node", value: "node-b"}: 0,
					{key: "node", value: "node-y"}: 2,
				},
			},
		},
		{
			name: "two spreadConstraints, and with podAffinity",
			pod: st.MakePod().Name("p").Label("foo", "").
				NodeAffinityNotIn("node", []string{"node-x"}). // exclude node-x
				SpreadConstraint(1, "zone", hardSpread, fooSelector).
				SpreadConstraint(1, "node", hardSpread, fooSelector).
				Obj(),
			nodes: []*v1.Node{
				st.MakeNode().Name("node-a").Label("zone", "zone1").Label("node", "node-a").Obj(),
				st.MakeNode().Name("node-b").Label("zone", "zone1").Label("node", "node-b").Obj(),
				st.MakeNode().Name("node-x").Label("zone", "zone2").Label("node", "node-x").Obj(),
				st.MakeNode().Name("node-y").Label("zone", "zone2").Label("node", "node-y").Obj(),
			},
			existingPods: []*v1.Pod{
				st.MakePod().Name("p-a1").Node("node-a").Label("foo", "").Obj(),
				st.MakePod().Name("p-a2").Node("node-a").Label("foo", "").Obj(),
				st.MakePod().Name("p-b1").Node("node-b").Label("foo", "").Obj(),
				st.MakePod().Name("p-y1").Node("node-y").Label("foo", "").Obj(),
				st.MakePod().Name("p-y2").Node("node-y").Label("foo", "").Obj(),
				st.MakePod().Name("p-y3").Node("node-y").Label("foo", "").Obj(),
				st.MakePod().Name("p-y4").Node("node-y").Label("foo", "").Obj(),
			},
			want: &evenPodsSpreadMetadata{
				constraints: []topologySpreadConstraint{
					{
						maxSkew:     1,
						topologyKey: "zone",
						selector:    mustConvertLabelSelectorAsSelector(t, fooSelector),
					},
					{
						maxSkew:     1,
						topologyKey: "node",
						selector:    mustConvertLabelSelectorAsSelector(t, fooSelector),
					},
				},
				tpKeyToCriticalPaths: map[string]*criticalPaths{
					"zone": {{"zone1", 3}, {"zone2", 4}},
					"node": {{"node-b", 1}, {"node-a", 2}},
				},
				tpPairToMatchNum: map[topologyPair]int32{
					{key: "zone", value: "zone1"}:  3,
					{key: "zone", value: "zone2"}:  4,
					{key: "node", value: "node-a"}: 2,
					{key: "node", value: "node-b"}: 1,
					{key: "node", value: "node-y"}: 4,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := nodeinfosnapshot.NewSnapshot(nodeinfosnapshot.CreateNodeInfoMap(tt.existingPods, tt.nodes))
			l, _ := s.NodeInfos().List()
			got, _ := getEvenPodsSpreadMetadata(tt.pod, l)
			got.sortCriticalPaths()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getEvenPodsSpreadMetadata() = %#v, want %#v", *got, *tt.want)
			}
		})
	}
}

func TestPodSpreadCache_addPod(t *testing.T) {
	nodeConstraint := topologySpreadConstraint{
		maxSkew:     1,
		topologyKey: "node",
		selector:    mustConvertLabelSelectorAsSelector(t, st.MakeLabelSelector().Exists("foo").Obj()),
	}
	zoneConstraint := nodeConstraint
	zoneConstraint.topologyKey = "zone"
	tests := []struct {
		name         string
		preemptor    *v1.Pod
		addedPod     *v1.Pod
		existingPods []*v1.Pod
		nodeIdx      int // denotes which node 'addedPod' belongs to
		nodes        []*v1.Node
		want         *evenPodsSpreadMetadata
	}{
		{
			name: "node a and b both impact current min match",
			preemptor: st.MakePod().Name("p").Label("foo", "").
				SpreadConstraint(1, "node", hardSpread, st.MakeLabelSelector().Exists("foo").Obj()).
				Obj(),
			addedPod:     st.MakePod().Name("p-a1").Node("node-a").Label("foo", "").Obj(),
			existingPods: nil, // it's an empty cluster
			nodeIdx:      0,
			nodes: []*v1.Node{
				st.MakeNode().Name("node-a").Label("zone", "zone1").Label("node", "node-a").Obj(),
				st.MakeNode().Name("node-b").Label("zone", "zone1").Label("node", "node-b").Obj(),
			},
			want: &evenPodsSpreadMetadata{
				constraints: []topologySpreadConstraint{nodeConstraint},
				tpKeyToCriticalPaths: map[string]*criticalPaths{
					"node": {{"node-b", 0}, {"node-a", 1}},
				},
				tpPairToMatchNum: map[topologyPair]int32{
					{key: "node", value: "node-a"}: 1,
					{key: "node", value: "node-b"}: 0,
				},
			},
		},
		{
			name: "only node a impacts current min match",
			preemptor: st.MakePod().Name("p").Label("foo", "").
				SpreadConstraint(1, "node", hardSpread, st.MakeLabelSelector().Exists("foo").Obj()).
				Obj(),
			addedPod: st.MakePod().Name("p-a1").Node("node-a").Label("foo", "").Obj(),
			existingPods: []*v1.Pod{
				st.MakePod().Name("p-b1").Node("node-b").Label("foo", "").Obj(),
			},
			nodeIdx: 0,
			nodes: []*v1.Node{
				st.MakeNode().Name("node-a").Label("zone", "zone1").Label("node", "node-a").Obj(),
				st.MakeNode().Name("node-b").Label("zone", "zone1").Label("node", "node-b").Obj(),
			},
			want: &evenPodsSpreadMetadata{
				constraints: []topologySpreadConstraint{nodeConstraint},
				tpKeyToCriticalPaths: map[string]*criticalPaths{
					"node": {{"node-a", 1}, {"node-b", 1}},
				},
				tpPairToMatchNum: map[topologyPair]int32{
					{key: "node", value: "node-a"}: 1,
					{key: "node", value: "node-b"}: 1,
				},
			},
		},
		{
			name: "add a pod with mis-matched namespace doesn't change topologyKeyToMinPodsMap",
			preemptor: st.MakePod().Name("p").Label("foo", "").
				SpreadConstraint(1, "node", hardSpread, st.MakeLabelSelector().Exists("foo").Obj()).
				Obj(),
			addedPod: st.MakePod().Name("p-a1").Namespace("ns1").Node("node-a").Label("foo", "").Obj(),
			existingPods: []*v1.Pod{
				st.MakePod().Name("p-b1").Node("node-b").Label("foo", "").Obj(),
			},
			nodeIdx: 0,
			nodes: []*v1.Node{
				st.MakeNode().Name("node-a").Label("zone", "zone1").Label("node", "node-a").Obj(),
				st.MakeNode().Name("node-b").Label("zone", "zone1").Label("node", "node-b").Obj(),
			},
			want: &evenPodsSpreadMetadata{
				constraints: []topologySpreadConstraint{nodeConstraint},
				tpKeyToCriticalPaths: map[string]*criticalPaths{
					"node": {{"node-a", 0}, {"node-b", 1}},
				},
				tpPairToMatchNum: map[topologyPair]int32{
					{key: "node", value: "node-a"}: 0,
					{key: "node", value: "node-b"}: 1,
				},
			},
		},
		{
			name: "add pod on non-critical node won't trigger re-calculation",
			preemptor: st.MakePod().Name("p").Label("foo", "").
				SpreadConstraint(1, "node", hardSpread, st.MakeLabelSelector().Exists("foo").Obj()).
				Obj(),
			addedPod: st.MakePod().Name("p-b2").Node("node-b").Label("foo", "").Obj(),
			existingPods: []*v1.Pod{
				st.MakePod().Name("p-b1").Node("node-b").Label("foo", "").Obj(),
			},
			nodeIdx: 1,
			nodes: []*v1.Node{
				st.MakeNode().Name("node-a").Label("zone", "zone1").Label("node", "node-a").Obj(),
				st.MakeNode().Name("node-b").Label("zone", "zone1").Label("node", "node-b").Obj(),
			},
			want: &evenPodsSpreadMetadata{
				constraints: []topologySpreadConstraint{nodeConstraint},
				tpKeyToCriticalPaths: map[string]*criticalPaths{
					"node": {{"node-a", 0}, {"node-b", 2}},
				},
				tpPairToMatchNum: map[topologyPair]int32{
					{key: "node", value: "node-a"}: 0,
					{key: "node", value: "node-b"}: 2,
				},
			},
		},
		{
			name: "node a and x both impact topologyKeyToMinPodsMap on zone and node",
			preemptor: st.MakePod().Name("p").Label("foo", "").
				SpreadConstraint(1, "zone", hardSpread, st.MakeLabelSelector().Exists("foo").Obj()).
				SpreadConstraint(1, "node", hardSpread, st.MakeLabelSelector().Exists("foo").Obj()).
				Obj(),
			addedPod:     st.MakePod().Name("p-a1").Node("node-a").Label("foo", "").Obj(),
			existingPods: nil, // it's an empty cluster
			nodeIdx:      0,
			nodes: []*v1.Node{
				st.MakeNode().Name("node-a").Label("zone", "zone1").Label("node", "node-a").Obj(),
				st.MakeNode().Name("node-x").Label("zone", "zone2").Label("node", "node-x").Obj(),
			},
			want: &evenPodsSpreadMetadata{
				constraints: []topologySpreadConstraint{zoneConstraint, nodeConstraint},
				tpKeyToCriticalPaths: map[string]*criticalPaths{
					"zone": {{"zone2", 0}, {"zone1", 1}},
					"node": {{"node-x", 0}, {"node-a", 1}},
				},
				tpPairToMatchNum: map[topologyPair]int32{
					{key: "zone", value: "zone1"}:  1,
					{key: "zone", value: "zone2"}:  0,
					{key: "node", value: "node-a"}: 1,
					{key: "node", value: "node-x"}: 0,
				},
			},
		},
		{
			name: "only node a impacts topologyKeyToMinPodsMap on zone and node",
			preemptor: st.MakePod().Name("p").Label("foo", "").
				SpreadConstraint(1, "zone", hardSpread, st.MakeLabelSelector().Exists("foo").Obj()).
				SpreadConstraint(1, "node", hardSpread, st.MakeLabelSelector().Exists("foo").Obj()).
				Obj(),
			addedPod: st.MakePod().Name("p-a1").Node("node-a").Label("foo", "").Obj(),
			existingPods: []*v1.Pod{
				st.MakePod().Name("p-x1").Node("node-x").Label("foo", "").Obj(),
			},
			nodeIdx: 0,
			nodes: []*v1.Node{
				st.MakeNode().Name("node-a").Label("zone", "zone1").Label("node", "node-a").Obj(),
				st.MakeNode().Name("node-x").Label("zone", "zone2").Label("node", "node-x").Obj(),
			},
			want: &evenPodsSpreadMetadata{
				constraints: []topologySpreadConstraint{zoneConstraint, nodeConstraint},
				tpKeyToCriticalPaths: map[string]*criticalPaths{
					"zone": {{"zone1", 1}, {"zone2", 1}},
					"node": {{"node-a", 1}, {"node-x", 1}},
				},
				tpPairToMatchNum: map[topologyPair]int32{
					{key: "zone", value: "zone1"}:  1,
					{key: "zone", value: "zone2"}:  1,
					{key: "node", value: "node-a"}: 1,
					{key: "node", value: "node-x"}: 1,
				},
			},
		},
		{
			name: "node a impacts topologyKeyToMinPodsMap on node, node x impacts topologyKeyToMinPodsMap on zone",
			preemptor: st.MakePod().Name("p").Label("foo", "").
				SpreadConstraint(1, "zone", hardSpread, st.MakeLabelSelector().Exists("foo").Obj()).
				SpreadConstraint(1, "node", hardSpread, st.MakeLabelSelector().Exists("foo").Obj()).
				Obj(),
			addedPod: st.MakePod().Name("p-a1").Node("node-a").Label("foo", "").Obj(),
			existingPods: []*v1.Pod{
				st.MakePod().Name("p-b1").Node("node-b").Label("foo", "").Obj(),
				st.MakePod().Name("p-b2").Node("node-b").Label("foo", "").Obj(),
				st.MakePod().Name("p-x1").Node("node-x").Label("foo", "").Obj(),
			},
			nodeIdx: 0,
			nodes: []*v1.Node{
				st.MakeNode().Name("node-a").Label("zone", "zone1").Label("node", "node-a").Obj(),
				st.MakeNode().Name("node-b").Label("zone", "zone1").Label("node", "node-b").Obj(),
				st.MakeNode().Name("node-x").Label("zone", "zone2").Label("node", "node-x").Obj(),
			},
			want: &evenPodsSpreadMetadata{
				constraints: []topologySpreadConstraint{zoneConstraint, nodeConstraint},
				tpKeyToCriticalPaths: map[string]*criticalPaths{
					"zone": {{"zone2", 1}, {"zone1", 3}},
					"node": {{"node-a", 1}, {"node-x", 1}},
				},
				tpPairToMatchNum: map[topologyPair]int32{
					{key: "zone", value: "zone1"}:  3,
					{key: "zone", value: "zone2"}:  1,
					{key: "node", value: "node-a"}: 1,
					{key: "node", value: "node-b"}: 2,
					{key: "node", value: "node-x"}: 1,
				},
			},
		},
		{
			name: "constraints hold different labelSelectors, node a impacts topologyKeyToMinPodsMap on zone",
			preemptor: st.MakePod().Name("p").Label("foo", "").Label("bar", "").
				SpreadConstraint(1, "zone", hardSpread, st.MakeLabelSelector().Exists("foo").Obj()).
				SpreadConstraint(1, "node", hardSpread, st.MakeLabelSelector().Exists("bar").Obj()).
				Obj(),
			addedPod: st.MakePod().Name("p-a1").Node("node-a").Label("foo", "").Obj(),
			existingPods: []*v1.Pod{
				st.MakePod().Name("p-b1").Node("node-b").Label("foo", "").Label("bar", "").Obj(),
				st.MakePod().Name("p-x1").Node("node-x").Label("foo", "").Label("bar", "").Obj(),
				st.MakePod().Name("p-x2").Node("node-x").Label("bar", "").Obj(),
			},
			nodeIdx: 0,
			nodes: []*v1.Node{
				st.MakeNode().Name("node-a").Label("zone", "zone1").Label("node", "node-a").Obj(),
				st.MakeNode().Name("node-b").Label("zone", "zone1").Label("node", "node-b").Obj(),
				st.MakeNode().Name("node-x").Label("zone", "zone2").Label("node", "node-x").Obj(),
			},
			want: &evenPodsSpreadMetadata{
				constraints: []topologySpreadConstraint{
					zoneConstraint,
					{
						maxSkew:     1,
						topologyKey: "node",
						selector:    mustConvertLabelSelectorAsSelector(t, st.MakeLabelSelector().Exists("bar").Obj()),
					},
				},
				tpKeyToCriticalPaths: map[string]*criticalPaths{
					"zone": {{"zone2", 1}, {"zone1", 2}},
					"node": {{"node-a", 0}, {"node-b", 1}},
				},
				tpPairToMatchNum: map[topologyPair]int32{
					{key: "zone", value: "zone1"}:  2,
					{key: "zone", value: "zone2"}:  1,
					{key: "node", value: "node-a"}: 0,
					{key: "node", value: "node-b"}: 1,
					{key: "node", value: "node-x"}: 2,
				},
			},
		},
		{
			name: "constraints hold different labelSelectors, node a impacts topologyKeyToMinPodsMap on both zone and node",
			preemptor: st.MakePod().Name("p").Label("foo", "").Label("bar", "").
				SpreadConstraint(1, "zone", hardSpread, st.MakeLabelSelector().Exists("foo").Obj()).
				SpreadConstraint(1, "node", hardSpread, st.MakeLabelSelector().Exists("bar").Obj()).
				Obj(),
			addedPod: st.MakePod().Name("p-a1").Node("node-a").Label("foo", "").Label("bar", "").Obj(),
			existingPods: []*v1.Pod{
				st.MakePod().Name("p-b1").Node("node-b").Label("bar", "").Obj(),
				st.MakePod().Name("p-x1").Node("node-x").Label("foo", "").Label("bar", "").Obj(),
				st.MakePod().Name("p-x2").Node("node-x").Label("bar", "").Obj(),
			},
			nodeIdx: 0,
			nodes: []*v1.Node{
				st.MakeNode().Name("node-a").Label("zone", "zone1").Label("node", "node-a").Obj(),
				st.MakeNode().Name("node-b").Label("zone", "zone1").Label("node", "node-b").Obj(),
				st.MakeNode().Name("node-x").Label("zone", "zone2").Label("node", "node-x").Obj(),
			},
			want: &evenPodsSpreadMetadata{
				constraints: []topologySpreadConstraint{
					zoneConstraint,
					{
						maxSkew:     1,
						topologyKey: "node",
						selector:    mustConvertLabelSelectorAsSelector(t, st.MakeLabelSelector().Exists("bar").Obj()),
					},
				},
				tpKeyToCriticalPaths: map[string]*criticalPaths{
					"zone": {{"zone1", 1}, {"zone2", 1}},
					"node": {{"node-a", 1}, {"node-b", 1}},
				},
				tpPairToMatchNum: map[topologyPair]int32{
					{key: "zone", value: "zone1"}:  1,
					{key: "zone", value: "zone2"}:  1,
					{key: "node", value: "node-a"}: 1,
					{key: "node", value: "node-b"}: 1,
					{key: "node", value: "node-x"}: 2,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := nodeinfosnapshot.NewSnapshot(nodeinfosnapshot.CreateNodeInfoMap(tt.existingPods, tt.nodes))
			l, _ := s.NodeInfos().List()
			evenPodsSpreadMetadata, _ := getEvenPodsSpreadMetadata(tt.preemptor, l)
			evenPodsSpreadMetadata.addPod(tt.addedPod, tt.preemptor, tt.nodes[tt.nodeIdx])
			evenPodsSpreadMetadata.sortCriticalPaths()
			if !reflect.DeepEqual(evenPodsSpreadMetadata, tt.want) {
				t.Errorf("evenPodsSpreadMetadata#addPod() = %v, want %v", evenPodsSpreadMetadata, tt.want)
			}
		})
	}
}

func TestPodSpreadCache_removePod(t *testing.T) {
	nodeConstraint := topologySpreadConstraint{
		maxSkew:     1,
		topologyKey: "node",
		selector:    mustConvertLabelSelectorAsSelector(t, st.MakeLabelSelector().Exists("foo").Obj()),
	}
	zoneConstraint := nodeConstraint
	zoneConstraint.topologyKey = "zone"
	tests := []struct {
		name          string
		preemptor     *v1.Pod // preemptor pod
		nodes         []*v1.Node
		existingPods  []*v1.Pod
		deletedPodIdx int     // need to reuse *Pod of existingPods[i]
		deletedPod    *v1.Pod // this field is used only when deletedPodIdx is -1
		nodeIdx       int     // denotes which node "deletedPod" belongs to
		want          *evenPodsSpreadMetadata
	}{
		{
			// A high priority pod may not be scheduled due to node taints or resource shortage.
			// So preemption is triggered.
			name: "one spreadConstraint on zone, topologyKeyToMinPodsMap unchanged",
			preemptor: st.MakePod().Name("p").Label("foo", "").
				SpreadConstraint(1, "zone", hardSpread, st.MakeLabelSelector().Exists("foo").Obj()).
				Obj(),
			nodes: []*v1.Node{
				st.MakeNode().Name("node-a").Label("zone", "zone1").Label("node", "node-a").Obj(),
				st.MakeNode().Name("node-b").Label("zone", "zone1").Label("node", "node-b").Obj(),
				st.MakeNode().Name("node-x").Label("zone", "zone2").Label("node", "node-x").Obj(),
			},
			existingPods: []*v1.Pod{
				st.MakePod().Name("p-a1").Node("node-a").Label("foo", "").Obj(),
				st.MakePod().Name("p-b1").Node("node-b").Label("foo", "").Obj(),
				st.MakePod().Name("p-x1").Node("node-x").Label("foo", "").Obj(),
			},
			deletedPodIdx: 0, // remove pod "p-a1"
			nodeIdx:       0, // node-a
			want: &evenPodsSpreadMetadata{
				constraints: []topologySpreadConstraint{zoneConstraint},
				tpKeyToCriticalPaths: map[string]*criticalPaths{
					"zone": {{"zone1", 1}, {"zone2", 1}},
				},
				tpPairToMatchNum: map[topologyPair]int32{
					{key: "zone", value: "zone1"}: 1,
					{key: "zone", value: "zone2"}: 1,
				},
			},
		},
		{
			name: "one spreadConstraint on node, topologyKeyToMinPodsMap changed",
			preemptor: st.MakePod().Name("p").Label("foo", "").
				SpreadConstraint(1, "zone", hardSpread, st.MakeLabelSelector().Exists("foo").Obj()).
				Obj(),
			nodes: []*v1.Node{
				st.MakeNode().Name("node-a").Label("zone", "zone1").Label("node", "node-a").Obj(),
				st.MakeNode().Name("node-b").Label("zone", "zone1").Label("node", "node-b").Obj(),
				st.MakeNode().Name("node-x").Label("zone", "zone2").Label("node", "node-x").Obj(),
				st.MakeNode().Name("node-y").Label("zone", "zone2").Label("node", "node-y").Obj(),
			},
			existingPods: []*v1.Pod{
				st.MakePod().Name("p-a1").Node("node-a").Label("foo", "").Obj(),
				st.MakePod().Name("p-b1").Node("node-b").Label("foo", "").Obj(),
				st.MakePod().Name("p-x1").Node("node-x").Label("foo", "").Obj(),
				st.MakePod().Name("p-y1").Node("node-y").Label("foo", "").Obj(),
			},
			deletedPodIdx: 0, // remove pod "p-a1"
			nodeIdx:       0, // node-a
			want: &evenPodsSpreadMetadata{
				constraints: []topologySpreadConstraint{zoneConstraint},
				tpKeyToCriticalPaths: map[string]*criticalPaths{
					"zone": {{"zone1", 1}, {"zone2", 2}},
				},
				tpPairToMatchNum: map[topologyPair]int32{
					{key: "zone", value: "zone1"}: 1,
					{key: "zone", value: "zone2"}: 2,
				},
			},
		},
		{
			name: "delete an irrelevant pod won't help",
			preemptor: st.MakePod().Name("p").Label("foo", "").
				SpreadConstraint(1, "zone", hardSpread, st.MakeLabelSelector().Exists("foo").Obj()).
				Obj(),
			nodes: []*v1.Node{
				st.MakeNode().Name("node-a").Label("zone", "zone1").Label("node", "node-a").Obj(),
				st.MakeNode().Name("node-b").Label("zone", "zone1").Label("node", "node-b").Obj(),
				st.MakeNode().Name("node-x").Label("zone", "zone2").Label("node", "node-x").Obj(),
				st.MakeNode().Name("node-y").Label("zone", "zone2").Label("node", "node-y").Obj(),
			},
			existingPods: []*v1.Pod{
				st.MakePod().Name("p-a0").Node("node-a").Label("bar", "").Obj(),
				st.MakePod().Name("p-a1").Node("node-a").Label("foo", "").Obj(),
				st.MakePod().Name("p-b1").Node("node-b").Label("foo", "").Obj(),
				st.MakePod().Name("p-x1").Node("node-x").Label("foo", "").Obj(),
				st.MakePod().Name("p-y1").Node("node-y").Label("foo", "").Obj(),
			},
			deletedPodIdx: 0, // remove pod "p-a0"
			nodeIdx:       0, // node-a
			want: &evenPodsSpreadMetadata{
				constraints: []topologySpreadConstraint{zoneConstraint},
				tpKeyToCriticalPaths: map[string]*criticalPaths{
					"zone": {{"zone1", 2}, {"zone2", 2}},
				},
				tpPairToMatchNum: map[topologyPair]int32{
					{key: "zone", value: "zone1"}: 2,
					{key: "zone", value: "zone2"}: 2,
				},
			},
		},
		{
			name: "delete a non-existing pod won't help",
			preemptor: st.MakePod().Name("p").Label("foo", "").
				SpreadConstraint(1, "zone", hardSpread, st.MakeLabelSelector().Exists("foo").Obj()).
				Obj(),
			nodes: []*v1.Node{
				st.MakeNode().Name("node-a").Label("zone", "zone1").Label("node", "node-a").Obj(),
				st.MakeNode().Name("node-b").Label("zone", "zone1").Label("node", "node-b").Obj(),
				st.MakeNode().Name("node-x").Label("zone", "zone2").Label("node", "node-x").Obj(),
				st.MakeNode().Name("node-y").Label("zone", "zone2").Label("node", "node-y").Obj(),
			},
			existingPods: []*v1.Pod{
				st.MakePod().Name("p-a1").Node("node-a").Label("foo", "").Obj(),
				st.MakePod().Name("p-b1").Node("node-b").Label("foo", "").Obj(),
				st.MakePod().Name("p-x1").Node("node-x").Label("foo", "").Obj(),
				st.MakePod().Name("p-y1").Node("node-y").Label("foo", "").Obj(),
			},
			deletedPodIdx: -1,
			deletedPod:    st.MakePod().Name("p-a0").Node("node-a").Label("bar", "").Obj(),
			nodeIdx:       0, // node-a
			want: &evenPodsSpreadMetadata{
				constraints: []topologySpreadConstraint{zoneConstraint},
				tpKeyToCriticalPaths: map[string]*criticalPaths{
					"zone": {{"zone1", 2}, {"zone2", 2}},
				},
				tpPairToMatchNum: map[topologyPair]int32{
					{key: "zone", value: "zone1"}: 2,
					{key: "zone", value: "zone2"}: 2,
				},
			},
		},
		{
			name: "two spreadConstraints",
			preemptor: st.MakePod().Name("p").Label("foo", "").
				SpreadConstraint(1, "zone", hardSpread, st.MakeLabelSelector().Exists("foo").Obj()).
				SpreadConstraint(1, "node", hardSpread, st.MakeLabelSelector().Exists("foo").Obj()).
				Obj(),
			nodes: []*v1.Node{
				st.MakeNode().Name("node-a").Label("zone", "zone1").Label("node", "node-a").Obj(),
				st.MakeNode().Name("node-b").Label("zone", "zone1").Label("node", "node-b").Obj(),
				st.MakeNode().Name("node-x").Label("zone", "zone2").Label("node", "node-x").Obj(),
			},
			existingPods: []*v1.Pod{
				st.MakePod().Name("p-a1").Node("node-a").Label("foo", "").Obj(),
				st.MakePod().Name("p-a2").Node("node-a").Label("foo", "").Obj(),
				st.MakePod().Name("p-b1").Node("node-b").Label("foo", "").Obj(),
				st.MakePod().Name("p-x1").Node("node-x").Label("foo", "").Obj(),
				st.MakePod().Name("p-x2").Node("node-x").Label("foo", "").Obj(),
			},
			deletedPodIdx: 3, // remove pod "p-x1"
			nodeIdx:       2, // node-x
			want: &evenPodsSpreadMetadata{
				constraints: []topologySpreadConstraint{zoneConstraint, nodeConstraint},
				tpKeyToCriticalPaths: map[string]*criticalPaths{
					"zone": {{"zone2", 1}, {"zone1", 3}},
					"node": {{"node-b", 1}, {"node-x", 1}},
				},
				tpPairToMatchNum: map[topologyPair]int32{
					{key: "zone", value: "zone1"}:  3,
					{key: "zone", value: "zone2"}:  1,
					{key: "node", value: "node-a"}: 2,
					{key: "node", value: "node-b"}: 1,
					{key: "node", value: "node-x"}: 1,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := nodeinfosnapshot.NewSnapshot(nodeinfosnapshot.CreateNodeInfoMap(tt.existingPods, tt.nodes))
			l, _ := s.NodeInfos().List()
			evenPodsSpreadMetadata, _ := getEvenPodsSpreadMetadata(tt.preemptor, l)

			var deletedPod *v1.Pod
			if tt.deletedPodIdx < len(tt.existingPods) && tt.deletedPodIdx >= 0 {
				deletedPod = tt.existingPods[tt.deletedPodIdx]
			} else {
				deletedPod = tt.deletedPod
			}
			evenPodsSpreadMetadata.removePod(deletedPod, tt.preemptor, tt.nodes[tt.nodeIdx])
			evenPodsSpreadMetadata.sortCriticalPaths()
			if !reflect.DeepEqual(evenPodsSpreadMetadata, tt.want) {
				t.Errorf("evenPodsSpreadMetadata#removePod() = %v, want %v", evenPodsSpreadMetadata, tt.want)
			}
		})
	}
}

func BenchmarkTestGetTPMapMatchingSpreadConstraints(b *testing.B) {
	tests := []struct {
		name             string
		pod              *v1.Pod
		existingPodsNum  int
		allNodesNum      int
		filteredNodesNum int
	}{
		{
			name: "1000nodes/single-constraint-zone",
			pod: st.MakePod().Name("p").Label("foo", "").
				SpreadConstraint(1, v1.LabelZoneFailureDomain, hardSpread, st.MakeLabelSelector().Exists("foo").Obj()).
				Obj(),
			existingPodsNum:  10000,
			allNodesNum:      1000,
			filteredNodesNum: 500,
		},
		{
			name: "1000nodes/single-constraint-node",
			pod: st.MakePod().Name("p").Label("foo", "").
				SpreadConstraint(1, v1.LabelHostname, hardSpread, st.MakeLabelSelector().Exists("foo").Obj()).
				Obj(),
			existingPodsNum:  10000,
			allNodesNum:      1000,
			filteredNodesNum: 500,
		},
		{
			name: "1000nodes/two-constraints-zone-node",
			pod: st.MakePod().Name("p").Label("foo", "").Label("bar", "").
				SpreadConstraint(1, v1.LabelZoneFailureDomain, hardSpread, st.MakeLabelSelector().Exists("foo").Obj()).
				SpreadConstraint(1, v1.LabelHostname, hardSpread, st.MakeLabelSelector().Exists("bar").Obj()).
				Obj(),
			existingPodsNum:  10000,
			allNodesNum:      1000,
			filteredNodesNum: 500,
		},
	}
	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			existingPods, allNodes, _ := st.MakeNodesAndPodsForEvenPodsSpread(tt.pod.Labels, tt.existingPodsNum, tt.allNodesNum, tt.filteredNodesNum)
			s := nodeinfosnapshot.NewSnapshot(nodeinfosnapshot.CreateNodeInfoMap(existingPods, allNodes))
			l, _ := s.NodeInfos().List()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				getEvenPodsSpreadMetadata(tt.pod, l)
			}
		})
	}
}

var (
	hardSpread = v1.DoNotSchedule
	softSpread = v1.ScheduleAnyway
)

// sortCriticalPaths is only served for testing purpose.
func (m *evenPodsSpreadMetadata) sortCriticalPaths() {
	for _, paths := range m.tpKeyToCriticalPaths {
		// If two paths both hold minimum matching number, and topologyValue is unordered.
		if paths[0].matchNum == paths[1].matchNum && paths[0].topologyValue > paths[1].topologyValue {
			// Swap topologyValue to make them sorted alphabetically.
			paths[0].topologyValue, paths[1].topologyValue = paths[1].topologyValue, paths[0].topologyValue
		}
	}
}

func mustConvertLabelSelectorAsSelector(t *testing.T, ls *metav1.LabelSelector) labels.Selector {
	t.Helper()
	s, err := metav1.LabelSelectorAsSelector(ls)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
