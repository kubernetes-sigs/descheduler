/*
Copyright 2014 The Kubernetes Authors.

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

package priorities

import (
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	framework "k8s.io/kubernetes/pkg/scheduler/framework/v1alpha1"
	fakelisters "k8s.io/kubernetes/pkg/scheduler/listers/fake"
	nodeinfosnapshot "k8s.io/kubernetes/pkg/scheduler/nodeinfo/snapshot"
)

func controllerRef(kind, name, uid string) []metav1.OwnerReference {
	// TODO: When ControllerRef will be implemented uncomment code below.
	return nil
	//trueVar := true
	//return []metav1.OwnerReference{
	//	{Kind: kind, Name: name, UID: types.UID(uid), Controller: &trueVar},
	//}
}

func TestSelectorSpreadPriority(t *testing.T) {
	labels1 := map[string]string{
		"foo": "bar",
		"baz": "blah",
	}
	labels2 := map[string]string{
		"bar": "foo",
		"baz": "blah",
	}
	zone1Spec := v1.PodSpec{
		NodeName: "machine1",
	}
	zone2Spec := v1.PodSpec{
		NodeName: "machine2",
	}
	tests := []struct {
		pod          *v1.Pod
		pods         []*v1.Pod
		nodes        []string
		rcs          []*v1.ReplicationController
		rss          []*apps.ReplicaSet
		services     []*v1.Service
		sss          []*apps.StatefulSet
		expectedList framework.NodeScoreList
		name         string
	}{
		{
			pod:          new(v1.Pod),
			nodes:        []string{"machine1", "machine2"},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: framework.MaxNodeScore}, {Name: "machine2", Score: framework.MaxNodeScore}},
			name:         "nothing scheduled",
		},
		{
			pod:          &v1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
			pods:         []*v1.Pod{{Spec: zone1Spec}},
			nodes:        []string{"machine1", "machine2"},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: framework.MaxNodeScore}, {Name: "machine2", Score: framework.MaxNodeScore}},
			name:         "no services",
		},
		{
			pod:          &v1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
			pods:         []*v1.Pod{{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels2}}},
			nodes:        []string{"machine1", "machine2"},
			services:     []*v1.Service{{Spec: v1.ServiceSpec{Selector: map[string]string{"key": "value"}}}},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: framework.MaxNodeScore}, {Name: "machine2", Score: framework.MaxNodeScore}},
			name:         "different services",
		},
		{
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
			pods: []*v1.Pod{
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels2}},
				{Spec: zone2Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
			},
			nodes:        []string{"machine1", "machine2"},
			services:     []*v1.Service{{Spec: v1.ServiceSpec{Selector: labels1}}},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: framework.MaxNodeScore}, {Name: "machine2", Score: 0}},
			name:         "two pods, one service pod",
		},
		{
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
			pods: []*v1.Pod{
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels2}},
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1, Namespace: metav1.NamespaceDefault}},
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1, Namespace: "ns1"}},
				{Spec: zone2Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
				{Spec: zone2Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels2}},
			},
			nodes:        []string{"machine1", "machine2"},
			services:     []*v1.Service{{Spec: v1.ServiceSpec{Selector: labels1}}},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: framework.MaxNodeScore}, {Name: "machine2", Score: 0}},
			name:         "five pods, one service pod in no namespace",
		},
		{
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: labels1, Namespace: metav1.NamespaceDefault}},
			pods: []*v1.Pod{
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1, Namespace: "ns1"}},
				{Spec: zone2Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1, Namespace: metav1.NamespaceDefault}},
				{Spec: zone2Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels2}},
			},
			nodes:        []string{"machine1", "machine2"},
			services:     []*v1.Service{{Spec: v1.ServiceSpec{Selector: labels1}, ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceDefault}}},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: framework.MaxNodeScore}, {Name: "machine2", Score: 0}},
			name:         "four pods, one service pod in default namespace",
		},
		{
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: labels1, Namespace: "ns1"}},
			pods: []*v1.Pod{
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1, Namespace: metav1.NamespaceDefault}},
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1, Namespace: "ns2"}},
				{Spec: zone2Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1, Namespace: "ns1"}},
				{Spec: zone2Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels2}},
			},
			nodes:        []string{"machine1", "machine2"},
			services:     []*v1.Service{{Spec: v1.ServiceSpec{Selector: labels1}, ObjectMeta: metav1.ObjectMeta{Namespace: "ns1"}}},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: framework.MaxNodeScore}, {Name: "machine2", Score: 0}},
			name:         "five pods, one service pod in specific namespace",
		},
		{
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
			pods: []*v1.Pod{
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels2}},
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
				{Spec: zone2Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
			},
			nodes:        []string{"machine1", "machine2"},
			services:     []*v1.Service{{Spec: v1.ServiceSpec{Selector: labels1}}},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: 0}, {Name: "machine2", Score: 0}},
			name:         "three pods, two service pods on different machines",
		},
		{
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
			pods: []*v1.Pod{
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels2}},
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
				{Spec: zone2Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
				{Spec: zone2Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
			},
			nodes:        []string{"machine1", "machine2"},
			services:     []*v1.Service{{Spec: v1.ServiceSpec{Selector: labels1}}},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: 50}, {Name: "machine2", Score: 0}},
			name:         "four pods, three service pods",
		},
		{
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
			pods: []*v1.Pod{
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels2}},
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
				{Spec: zone2Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
			},
			nodes:        []string{"machine1", "machine2"},
			services:     []*v1.Service{{Spec: v1.ServiceSpec{Selector: map[string]string{"baz": "blah"}}}},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: 0}, {Name: "machine2", Score: 50}},
			name:         "service with partial pod label matches",
		},
		{
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: labels1, OwnerReferences: controllerRef("ReplicationController", "name", "abc123")}},
			pods: []*v1.Pod{
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels2}},
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1, OwnerReferences: controllerRef("ReplicationController", "name", "abc123")}},
				{Spec: zone2Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1, OwnerReferences: controllerRef("ReplicationController", "name", "abc123")}},
			},
			nodes:    []string{"machine1", "machine2"},
			rcs:      []*v1.ReplicationController{{Spec: v1.ReplicationControllerSpec{Selector: map[string]string{"foo": "bar"}}}},
			services: []*v1.Service{{Spec: v1.ServiceSpec{Selector: map[string]string{"baz": "blah"}}}},
			// "baz=blah" matches both labels1 and labels2, and "foo=bar" matches only labels 1. This means that we assume that we want to
			// do spreading pod2 and pod3 and not pod1.
			expectedList: []framework.NodeScore{{Name: "machine1", Score: 0}, {Name: "machine2", Score: 0}},
			name:         "service with partial pod label matches with service and replication controller",
		},
		{
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: labels1, OwnerReferences: controllerRef("ReplicaSet", "name", "abc123")}},
			pods: []*v1.Pod{
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels2}},
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1, OwnerReferences: controllerRef("ReplicaSet", "name", "abc123")}},
				{Spec: zone2Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1, OwnerReferences: controllerRef("ReplicaSet", "name", "abc123")}},
			},
			nodes:    []string{"machine1", "machine2"},
			services: []*v1.Service{{Spec: v1.ServiceSpec{Selector: map[string]string{"baz": "blah"}}}},
			rss:      []*apps.ReplicaSet{{Spec: apps.ReplicaSetSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}}}}},
			// We use ReplicaSet, instead of ReplicationController. The result should be exactly as above.
			expectedList: []framework.NodeScore{{Name: "machine1", Score: 0}, {Name: "machine2", Score: 0}},
			name:         "service with partial pod label matches with service and replica set",
		},
		{
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: labels1, OwnerReferences: controllerRef("StatefulSet", "name", "abc123")}},
			pods: []*v1.Pod{
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels2}},
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1, OwnerReferences: controllerRef("StatefulSet", "name", "abc123")}},
				{Spec: zone2Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1, OwnerReferences: controllerRef("StatefulSet", "name", "abc123")}},
			},
			nodes:        []string{"machine1", "machine2"},
			services:     []*v1.Service{{Spec: v1.ServiceSpec{Selector: map[string]string{"baz": "blah"}}}},
			sss:          []*apps.StatefulSet{{Spec: apps.StatefulSetSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}}}}},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: 0}, {Name: "machine2", Score: 0}},
			name:         "service with partial pod label matches with service and stateful set",
		},
		{
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"foo": "bar", "bar": "foo"}, OwnerReferences: controllerRef("ReplicationController", "name", "abc123")}},
			pods: []*v1.Pod{
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels2}},
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1, OwnerReferences: controllerRef("ReplicationController", "name", "abc123")}},
				{Spec: zone2Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1, OwnerReferences: controllerRef("ReplicationController", "name", "abc123")}},
			},
			nodes:    []string{"machine1", "machine2"},
			rcs:      []*v1.ReplicationController{{Spec: v1.ReplicationControllerSpec{Selector: map[string]string{"foo": "bar"}}}},
			services: []*v1.Service{{Spec: v1.ServiceSpec{Selector: map[string]string{"bar": "foo"}}}},
			// Taken together Service and Replication Controller should match no pods.
			expectedList: []framework.NodeScore{{Name: "machine1", Score: framework.MaxNodeScore}, {Name: "machine2", Score: framework.MaxNodeScore}},
			name:         "disjoined service and replication controller matches no pods",
		},
		{
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"foo": "bar", "bar": "foo"}, OwnerReferences: controllerRef("ReplicaSet", "name", "abc123")}},
			pods: []*v1.Pod{
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels2}},
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1, OwnerReferences: controllerRef("ReplicaSet", "name", "abc123")}},
				{Spec: zone2Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1, OwnerReferences: controllerRef("ReplicaSet", "name", "abc123")}},
			},
			nodes:    []string{"machine1", "machine2"},
			services: []*v1.Service{{Spec: v1.ServiceSpec{Selector: map[string]string{"bar": "foo"}}}},
			rss:      []*apps.ReplicaSet{{Spec: apps.ReplicaSetSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}}}}},
			// We use ReplicaSet, instead of ReplicationController. The result should be exactly as above.
			expectedList: []framework.NodeScore{{Name: "machine1", Score: framework.MaxNodeScore}, {Name: "machine2", Score: framework.MaxNodeScore}},
			name:         "disjoined service and replica set matches no pods",
		},
		{
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"foo": "bar", "bar": "foo"}, OwnerReferences: controllerRef("StatefulSet", "name", "abc123")}},
			pods: []*v1.Pod{
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels2}},
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1, OwnerReferences: controllerRef("StatefulSet", "name", "abc123")}},
				{Spec: zone2Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1, OwnerReferences: controllerRef("StatefulSet", "name", "abc123")}},
			},
			nodes:        []string{"machine1", "machine2"},
			services:     []*v1.Service{{Spec: v1.ServiceSpec{Selector: map[string]string{"bar": "foo"}}}},
			sss:          []*apps.StatefulSet{{Spec: apps.StatefulSetSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}}}}},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: framework.MaxNodeScore}, {Name: "machine2", Score: framework.MaxNodeScore}},
			name:         "disjoined service and stateful set matches no pods",
		},
		{
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: labels1, OwnerReferences: controllerRef("ReplicationController", "name", "abc123")}},
			pods: []*v1.Pod{
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels2}},
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1, OwnerReferences: controllerRef("ReplicationController", "name", "abc123")}},
				{Spec: zone2Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1, OwnerReferences: controllerRef("ReplicationController", "name", "abc123")}},
			},
			nodes: []string{"machine1", "machine2"},
			rcs:   []*v1.ReplicationController{{Spec: v1.ReplicationControllerSpec{Selector: map[string]string{"foo": "bar"}}}},
			// Both Nodes have one pod from the given RC, hence both get 0 score.
			expectedList: []framework.NodeScore{{Name: "machine1", Score: 0}, {Name: "machine2", Score: 0}},
			name:         "Replication controller with partial pod label matches",
		},
		{
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: labels1, OwnerReferences: controllerRef("ReplicaSet", "name", "abc123")}},
			pods: []*v1.Pod{
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels2}},
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1, OwnerReferences: controllerRef("ReplicaSet", "name", "abc123")}},
				{Spec: zone2Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1, OwnerReferences: controllerRef("ReplicaSet", "name", "abc123")}},
			},
			nodes: []string{"machine1", "machine2"},
			rss:   []*apps.ReplicaSet{{Spec: apps.ReplicaSetSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}}}}},
			// We use ReplicaSet, instead of ReplicationController. The result should be exactly as above.
			expectedList: []framework.NodeScore{{Name: "machine1", Score: 0}, {Name: "machine2", Score: 0}},
			name:         "Replica set with partial pod label matches",
		},
		{
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: labels1, OwnerReferences: controllerRef("StatefulSet", "name", "abc123")}},
			pods: []*v1.Pod{
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels2}},
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1, OwnerReferences: controllerRef("StatefulSet", "name", "abc123")}},
				{Spec: zone2Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1, OwnerReferences: controllerRef("StatefulSet", "name", "abc123")}},
			},
			nodes: []string{"machine1", "machine2"},
			sss:   []*apps.StatefulSet{{Spec: apps.StatefulSetSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}}}}},
			// We use StatefulSet, instead of ReplicationController. The result should be exactly as above.
			expectedList: []framework.NodeScore{{Name: "machine1", Score: 0}, {Name: "machine2", Score: 0}},
			name:         "StatefulSet with partial pod label matches",
		},
		{
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: labels1, OwnerReferences: controllerRef("ReplicationController", "name", "abc123")}},
			pods: []*v1.Pod{
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels2, OwnerReferences: controllerRef("ReplicationController", "name", "abc123")}},
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1, OwnerReferences: controllerRef("ReplicationController", "name", "abc123")}},
				{Spec: zone2Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1, OwnerReferences: controllerRef("ReplicationController", "name", "abc123")}},
			},
			nodes:        []string{"machine1", "machine2"},
			rcs:          []*v1.ReplicationController{{Spec: v1.ReplicationControllerSpec{Selector: map[string]string{"baz": "blah"}}}},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: 0}, {Name: "machine2", Score: 50}},
			name:         "Another replication controller with partial pod label matches",
		},
		{
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: labels1, OwnerReferences: controllerRef("ReplicaSet", "name", "abc123")}},
			pods: []*v1.Pod{
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels2, OwnerReferences: controllerRef("ReplicaSet", "name", "abc123")}},
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1, OwnerReferences: controllerRef("ReplicaSet", "name", "abc123")}},
				{Spec: zone2Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1, OwnerReferences: controllerRef("ReplicaSet", "name", "abc123")}},
			},
			nodes: []string{"machine1", "machine2"},
			rss:   []*apps.ReplicaSet{{Spec: apps.ReplicaSetSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"baz": "blah"}}}}},
			// We use ReplicaSet, instead of ReplicationController. The result should be exactly as above.
			expectedList: []framework.NodeScore{{Name: "machine1", Score: 0}, {Name: "machine2", Score: 50}},
			name:         "Another replication set with partial pod label matches",
		},
		{
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: labels1, OwnerReferences: controllerRef("StatefulSet", "name", "abc123")}},
			pods: []*v1.Pod{
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels2, OwnerReferences: controllerRef("StatefulSet", "name", "abc123")}},
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1, OwnerReferences: controllerRef("StatefulSet", "name", "abc123")}},
				{Spec: zone2Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1, OwnerReferences: controllerRef("StatefulSet", "name", "abc123")}},
			},
			nodes: []string{"machine1", "machine2"},
			sss:   []*apps.StatefulSet{{Spec: apps.StatefulSetSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"baz": "blah"}}}}},
			// We use StatefulSet, instead of ReplicationController. The result should be exactly as above.
			expectedList: []framework.NodeScore{{Name: "machine1", Score: 0}, {Name: "machine2", Score: 50}},
			name:         "Another stateful set with partial pod label matches",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			nodes := makeNodeList(test.nodes)
			snapshot := nodeinfosnapshot.NewSnapshot(nodeinfosnapshot.CreateNodeInfoMap(test.pods, nodes))
			selectorSpread := SelectorSpread{
				serviceLister:     fakelisters.ServiceLister(test.services),
				controllerLister:  fakelisters.ControllerLister(test.rcs),
				replicaSetLister:  fakelisters.ReplicaSetLister(test.rss),
				statefulSetLister: fakelisters.StatefulSetLister(test.sss),
			}

			metaDataProducer := NewMetadataFactory(
				fakelisters.ServiceLister(test.services),
				fakelisters.ControllerLister(test.rcs),
				fakelisters.ReplicaSetLister(test.rss),
				fakelisters.StatefulSetLister(test.sss),
				1,
			)
			metaData := metaDataProducer(test.pod, nodes, snapshot)

			list, err := runMapReducePriority(selectorSpread.CalculateSpreadPriorityMap, selectorSpread.CalculateSpreadPriorityReduce, metaData, test.pod, snapshot, makeNodeList(test.nodes))
			if err != nil {
				t.Errorf("unexpected error: %v \n", err)
			}
			if diff := cmp.Diff(test.expectedList, list); diff != "" {
				t.Errorf("wrong priorities produced (-want, +got): %s", diff)
			}
		})
	}
}

func buildPod(nodeName string, labels map[string]string, ownerRefs []metav1.OwnerReference) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Labels: labels, OwnerReferences: ownerRefs},
		Spec:       v1.PodSpec{NodeName: nodeName},
	}
}

func TestZoneSelectorSpreadPriority(t *testing.T) {
	labels1 := map[string]string{
		"label1": "l1",
		"baz":    "blah",
	}
	labels2 := map[string]string{
		"label2": "l2",
		"baz":    "blah",
	}

	const nodeMachine1Zone1 = "machine1.zone1"
	const nodeMachine1Zone2 = "machine1.zone2"
	const nodeMachine2Zone2 = "machine2.zone2"
	const nodeMachine1Zone3 = "machine1.zone3"
	const nodeMachine2Zone3 = "machine2.zone3"
	const nodeMachine3Zone3 = "machine3.zone3"

	buildNodeLabels := func(failureDomain string) map[string]string {
		labels := map[string]string{
			v1.LabelZoneFailureDomain: failureDomain,
		}
		return labels
	}
	labeledNodes := map[string]map[string]string{
		nodeMachine1Zone1: buildNodeLabels("zone1"),
		nodeMachine1Zone2: buildNodeLabels("zone2"),
		nodeMachine2Zone2: buildNodeLabels("zone2"),
		nodeMachine1Zone3: buildNodeLabels("zone3"),
		nodeMachine2Zone3: buildNodeLabels("zone3"),
		nodeMachine3Zone3: buildNodeLabels("zone3"),
	}

	tests := []struct {
		pod          *v1.Pod
		pods         []*v1.Pod
		rcs          []*v1.ReplicationController
		rss          []*apps.ReplicaSet
		services     []*v1.Service
		sss          []*apps.StatefulSet
		expectedList framework.NodeScoreList
		name         string
	}{
		{
			pod: new(v1.Pod),
			expectedList: []framework.NodeScore{
				{Name: nodeMachine1Zone1, Score: framework.MaxNodeScore},
				{Name: nodeMachine1Zone2, Score: framework.MaxNodeScore},
				{Name: nodeMachine2Zone2, Score: framework.MaxNodeScore},
				{Name: nodeMachine1Zone3, Score: framework.MaxNodeScore},
				{Name: nodeMachine2Zone3, Score: framework.MaxNodeScore},
				{Name: nodeMachine3Zone3, Score: framework.MaxNodeScore},
			},
			name: "nothing scheduled",
		},
		{
			pod:  buildPod("", labels1, nil),
			pods: []*v1.Pod{buildPod(nodeMachine1Zone1, nil, nil)},
			expectedList: []framework.NodeScore{
				{Name: nodeMachine1Zone1, Score: framework.MaxNodeScore},
				{Name: nodeMachine1Zone2, Score: framework.MaxNodeScore},
				{Name: nodeMachine2Zone2, Score: framework.MaxNodeScore},
				{Name: nodeMachine1Zone3, Score: framework.MaxNodeScore},
				{Name: nodeMachine2Zone3, Score: framework.MaxNodeScore},
				{Name: nodeMachine3Zone3, Score: framework.MaxNodeScore},
			},
			name: "no services",
		},
		{
			pod:      buildPod("", labels1, nil),
			pods:     []*v1.Pod{buildPod(nodeMachine1Zone1, labels2, nil)},
			services: []*v1.Service{{Spec: v1.ServiceSpec{Selector: map[string]string{"key": "value"}}}},
			expectedList: []framework.NodeScore{
				{Name: nodeMachine1Zone1, Score: framework.MaxNodeScore},
				{Name: nodeMachine1Zone2, Score: framework.MaxNodeScore},
				{Name: nodeMachine2Zone2, Score: framework.MaxNodeScore},
				{Name: nodeMachine1Zone3, Score: framework.MaxNodeScore},
				{Name: nodeMachine2Zone3, Score: framework.MaxNodeScore},
				{Name: nodeMachine3Zone3, Score: framework.MaxNodeScore},
			},
			name: "different services",
		},
		{
			pod: buildPod("", labels1, nil),
			pods: []*v1.Pod{
				buildPod(nodeMachine1Zone1, labels2, nil),
				buildPod(nodeMachine1Zone2, labels2, nil),
			},
			services: []*v1.Service{{Spec: v1.ServiceSpec{Selector: labels1}}},
			expectedList: []framework.NodeScore{
				{Name: nodeMachine1Zone1, Score: framework.MaxNodeScore},
				{Name: nodeMachine1Zone2, Score: framework.MaxNodeScore},
				{Name: nodeMachine2Zone2, Score: framework.MaxNodeScore},
				{Name: nodeMachine1Zone3, Score: framework.MaxNodeScore},
				{Name: nodeMachine2Zone3, Score: framework.MaxNodeScore},
				{Name: nodeMachine3Zone3, Score: framework.MaxNodeScore},
			},
			name: "two pods, 0 matching",
		},
		{
			pod: buildPod("", labels1, nil),
			pods: []*v1.Pod{
				buildPod(nodeMachine1Zone1, labels2, nil),
				buildPod(nodeMachine1Zone2, labels1, nil),
			},
			services: []*v1.Service{{Spec: v1.ServiceSpec{Selector: labels1}}},
			expectedList: []framework.NodeScore{
				{Name: nodeMachine1Zone1, Score: framework.MaxNodeScore},
				{Name: nodeMachine1Zone2, Score: 0},  // Already have pod on machine
				{Name: nodeMachine2Zone2, Score: 33}, // Already have pod in zone
				{Name: nodeMachine1Zone3, Score: framework.MaxNodeScore},
				{Name: nodeMachine2Zone3, Score: framework.MaxNodeScore},
				{Name: nodeMachine3Zone3, Score: framework.MaxNodeScore},
			},
			name: "two pods, 1 matching (in z2)",
		},
		{
			pod: buildPod("", labels1, nil),
			pods: []*v1.Pod{
				buildPod(nodeMachine1Zone1, labels2, nil),
				buildPod(nodeMachine1Zone2, labels1, nil),
				buildPod(nodeMachine2Zone2, labels1, nil),
				buildPod(nodeMachine1Zone3, labels2, nil),
				buildPod(nodeMachine2Zone3, labels1, nil),
			},
			services: []*v1.Service{{Spec: v1.ServiceSpec{Selector: labels1}}},
			expectedList: []framework.NodeScore{
				{Name: nodeMachine1Zone1, Score: framework.MaxNodeScore},
				{Name: nodeMachine1Zone2, Score: 0},  // Pod on node
				{Name: nodeMachine2Zone2, Score: 0},  // Pod on node
				{Name: nodeMachine1Zone3, Score: 66}, // Pod in zone
				{Name: nodeMachine2Zone3, Score: 33}, // Pod on node
				{Name: nodeMachine3Zone3, Score: 66}, // Pod in zone
			},
			name: "five pods, 3 matching (z2=2, z3=1)",
		},
		{
			pod: buildPod("", labels1, nil),
			pods: []*v1.Pod{
				buildPod(nodeMachine1Zone1, labels1, nil),
				buildPod(nodeMachine1Zone2, labels1, nil),
				buildPod(nodeMachine2Zone2, labels2, nil),
				buildPod(nodeMachine1Zone3, labels1, nil),
			},
			services: []*v1.Service{{Spec: v1.ServiceSpec{Selector: labels1}}},
			expectedList: []framework.NodeScore{
				{Name: nodeMachine1Zone1, Score: 0},  // Pod on node
				{Name: nodeMachine1Zone2, Score: 0},  // Pod on node
				{Name: nodeMachine2Zone2, Score: 33}, // Pod in zone
				{Name: nodeMachine1Zone3, Score: 0},  // Pod on node
				{Name: nodeMachine2Zone3, Score: 33}, // Pod in zone
				{Name: nodeMachine3Zone3, Score: 33}, // Pod in zone
			},
			name: "four pods, 3 matching (z1=1, z2=1, z3=1)",
		},
		{
			pod: buildPod("", labels1, nil),
			pods: []*v1.Pod{
				buildPod(nodeMachine1Zone1, labels1, nil),
				buildPod(nodeMachine1Zone2, labels1, nil),
				buildPod(nodeMachine1Zone3, labels1, nil),
				buildPod(nodeMachine2Zone2, labels2, nil),
			},
			services: []*v1.Service{{Spec: v1.ServiceSpec{Selector: labels1}}},
			expectedList: []framework.NodeScore{
				{Name: nodeMachine1Zone1, Score: 0},  // Pod on node
				{Name: nodeMachine1Zone2, Score: 0},  // Pod on node
				{Name: nodeMachine2Zone2, Score: 33}, // Pod in zone
				{Name: nodeMachine1Zone3, Score: 0},  // Pod on node
				{Name: nodeMachine2Zone3, Score: 33}, // Pod in zone
				{Name: nodeMachine3Zone3, Score: 33}, // Pod in zone
			},
			name: "four pods, 3 matching (z1=1, z2=1, z3=1)",
		},
		{
			pod: buildPod("", labels1, controllerRef("ReplicationController", "name", "abc123")),
			pods: []*v1.Pod{
				buildPod(nodeMachine1Zone3, labels1, controllerRef("ReplicationController", "name", "abc123")),
				buildPod(nodeMachine1Zone2, labels1, controllerRef("ReplicationController", "name", "abc123")),
				buildPod(nodeMachine1Zone3, labels1, controllerRef("ReplicationController", "name", "abc123")),
			},
			rcs: []*v1.ReplicationController{{Spec: v1.ReplicationControllerSpec{Selector: labels1}}},
			expectedList: []framework.NodeScore{
				// Note that because we put two pods on the same node (nodeMachine1Zone3),
				// the values here are questionable for zone2, in particular for nodeMachine1Zone2.
				// However they kind of make sense; zone1 is still most-highly favored.
				// zone3 is in general least favored, and m1.z3 particularly low priority.
				// We would probably prefer to see a bigger gap between putting a second
				// pod on m1.z2 and putting a pod on m2.z2, but the ordering is correct.
				// This is also consistent with what we have already.
				{Name: nodeMachine1Zone1, Score: framework.MaxNodeScore}, // No pods in zone
				{Name: nodeMachine1Zone2, Score: 50},                     // Pod on node
				{Name: nodeMachine2Zone2, Score: 66},                     // Pod in zone
				{Name: nodeMachine1Zone3, Score: 0},                      // Two pods on node
				{Name: nodeMachine2Zone3, Score: 33},                     // Pod in zone
				{Name: nodeMachine3Zone3, Score: 33},                     // Pod in zone
			},
			name: "Replication controller spreading (z1=0, z2=1, z3=2)",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			nodes := makeLabeledNodeList(labeledNodes)
			snapshot := nodeinfosnapshot.NewSnapshot(nodeinfosnapshot.CreateNodeInfoMap(test.pods, nodes))
			selectorSpread := SelectorSpread{
				serviceLister:     fakelisters.ServiceLister(test.services),
				controllerLister:  fakelisters.ControllerLister(test.rcs),
				replicaSetLister:  fakelisters.ReplicaSetLister(test.rss),
				statefulSetLister: fakelisters.StatefulSetLister(test.sss),
			}

			metaDataProducer := NewMetadataFactory(
				fakelisters.ServiceLister(test.services),
				fakelisters.ControllerLister(test.rcs),
				fakelisters.ReplicaSetLister(test.rss),
				fakelisters.StatefulSetLister(test.sss),
				1,
			)
			metaData := metaDataProducer(test.pod, nodes, snapshot)
			list, err := runMapReducePriority(selectorSpread.CalculateSpreadPriorityMap, selectorSpread.CalculateSpreadPriorityReduce, metaData, test.pod, snapshot, makeLabeledNodeList(labeledNodes))
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			// sort the two lists to avoid failures on account of different ordering
			sortNodeScoreList(test.expectedList)
			sortNodeScoreList(list)
			if !reflect.DeepEqual(test.expectedList, list) {
				t.Errorf("expected %#v, got %#v", test.expectedList, list)
			}
		})
	}
}

func TestZoneSpreadPriority(t *testing.T) {
	labels1 := map[string]string{
		"foo": "bar",
		"baz": "blah",
	}
	labels2 := map[string]string{
		"bar": "foo",
		"baz": "blah",
	}
	zone1 := map[string]string{
		"zone": "zone1",
	}
	zone2 := map[string]string{
		"zone": "zone2",
	}
	nozone := map[string]string{
		"name": "value",
	}
	zone0Spec := v1.PodSpec{
		NodeName: "machine01",
	}
	zone1Spec := v1.PodSpec{
		NodeName: "machine11",
	}
	zone2Spec := v1.PodSpec{
		NodeName: "machine21",
	}
	labeledNodes := map[string]map[string]string{
		"machine01": nozone, "machine02": nozone,
		"machine11": zone1, "machine12": zone1,
		"machine21": zone2, "machine22": zone2,
	}
	tests := []struct {
		pod          *v1.Pod
		pods         []*v1.Pod
		nodes        map[string]map[string]string
		services     []*v1.Service
		expectedList framework.NodeScoreList
		name         string
	}{
		{
			pod:   new(v1.Pod),
			nodes: labeledNodes,
			expectedList: []framework.NodeScore{{Name: "machine11", Score: framework.MaxNodeScore}, {Name: "machine12", Score: framework.MaxNodeScore},
				{Name: "machine21", Score: framework.MaxNodeScore}, {Name: "machine22", Score: framework.MaxNodeScore},
				{Name: "machine01", Score: 0}, {Name: "machine02", Score: 0}},
			name: "nothing scheduled",
		},
		{
			pod:   &v1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
			pods:  []*v1.Pod{{Spec: zone1Spec}},
			nodes: labeledNodes,
			expectedList: []framework.NodeScore{{Name: "machine11", Score: framework.MaxNodeScore}, {Name: "machine12", Score: framework.MaxNodeScore},
				{Name: "machine21", Score: framework.MaxNodeScore}, {Name: "machine22", Score: framework.MaxNodeScore},
				{Name: "machine01", Score: 0}, {Name: "machine02", Score: 0}},
			name: "no services",
		},
		{
			pod:      &v1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
			pods:     []*v1.Pod{{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels2}}},
			nodes:    labeledNodes,
			services: []*v1.Service{{Spec: v1.ServiceSpec{Selector: map[string]string{"key": "value"}}}},
			expectedList: []framework.NodeScore{{Name: "machine11", Score: framework.MaxNodeScore}, {Name: "machine12", Score: framework.MaxNodeScore},
				{Name: "machine21", Score: framework.MaxNodeScore}, {Name: "machine22", Score: framework.MaxNodeScore},
				{Name: "machine01", Score: 0}, {Name: "machine02", Score: 0}},
			name: "different services",
		},
		{
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
			pods: []*v1.Pod{
				{Spec: zone0Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels2}},
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels2}},
				{Spec: zone2Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
			},
			nodes:    labeledNodes,
			services: []*v1.Service{{Spec: v1.ServiceSpec{Selector: labels1}}},
			expectedList: []framework.NodeScore{{Name: "machine11", Score: framework.MaxNodeScore}, {Name: "machine12", Score: framework.MaxNodeScore},
				{Name: "machine21", Score: 0}, {Name: "machine22", Score: 0},
				{Name: "machine01", Score: 0}, {Name: "machine02", Score: 0}},
			name: "three pods, one service pod",
		},
		{
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
			pods: []*v1.Pod{
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels2}},
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
				{Spec: zone2Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
			},
			nodes:    labeledNodes,
			services: []*v1.Service{{Spec: v1.ServiceSpec{Selector: labels1}}},
			expectedList: []framework.NodeScore{{Name: "machine11", Score: 50}, {Name: "machine12", Score: 50},
				{Name: "machine21", Score: 50}, {Name: "machine22", Score: 50},
				{Name: "machine01", Score: 0}, {Name: "machine02", Score: 0}},
			name: "three pods, two service pods on different machines",
		},
		{
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: labels1, Namespace: metav1.NamespaceDefault}},
			pods: []*v1.Pod{
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1, Namespace: metav1.NamespaceDefault}},
				{Spec: zone2Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
				{Spec: zone2Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1, Namespace: "ns1"}},
			},
			nodes:    labeledNodes,
			services: []*v1.Service{{Spec: v1.ServiceSpec{Selector: labels1}, ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceDefault}}},
			expectedList: []framework.NodeScore{{Name: "machine11", Score: 0}, {Name: "machine12", Score: 0},
				{Name: "machine21", Score: framework.MaxNodeScore}, {Name: "machine22", Score: framework.MaxNodeScore},
				{Name: "machine01", Score: 0}, {Name: "machine02", Score: 0}},
			name: "three service label match pods in different namespaces",
		},
		{
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
			pods: []*v1.Pod{
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels2}},
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
				{Spec: zone2Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
				{Spec: zone2Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
			},
			nodes:    labeledNodes,
			services: []*v1.Service{{Spec: v1.ServiceSpec{Selector: labels1}}},
			expectedList: []framework.NodeScore{{Name: "machine11", Score: 66}, {Name: "machine12", Score: 66},
				{Name: "machine21", Score: 33}, {Name: "machine22", Score: 33},
				{Name: "machine01", Score: 0}, {Name: "machine02", Score: 0}},
			name: "four pods, three service pods",
		},
		{
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
			pods: []*v1.Pod{
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels2}},
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
				{Spec: zone2Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
			},
			nodes:    labeledNodes,
			services: []*v1.Service{{Spec: v1.ServiceSpec{Selector: map[string]string{"baz": "blah"}}}},
			expectedList: []framework.NodeScore{{Name: "machine11", Score: 33}, {Name: "machine12", Score: 33},
				{Name: "machine21", Score: 66}, {Name: "machine22", Score: 66},
				{Name: "machine01", Score: 0}, {Name: "machine02", Score: 0}},
			name: "service with partial pod label matches",
		},
		{
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
			pods: []*v1.Pod{
				{Spec: zone0Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
				{Spec: zone1Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
				{Spec: zone2Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
				{Spec: zone2Spec, ObjectMeta: metav1.ObjectMeta{Labels: labels1}},
			},
			nodes:    labeledNodes,
			services: []*v1.Service{{Spec: v1.ServiceSpec{Selector: labels1}}},
			expectedList: []framework.NodeScore{{Name: "machine11", Score: 75}, {Name: "machine12", Score: 75},
				{Name: "machine21", Score: 50}, {Name: "machine22", Score: 50},
				{Name: "machine01", Score: 0}, {Name: "machine02", Score: 0}},
			name: "service pod on non-zoned node",
		},
	}
	// these local variables just make sure controllerLister\replicaSetLister\statefulSetLister not nil
	// when construct metaDataProducer
	sss := []*apps.StatefulSet{{Spec: apps.StatefulSetSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}}}}}
	rcs := []*v1.ReplicationController{{Spec: v1.ReplicationControllerSpec{Selector: map[string]string{"foo": "bar"}}}}
	rss := []*apps.ReplicaSet{{Spec: apps.ReplicaSetSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}}}}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			nodes := makeLabeledNodeList(labeledNodes)
			snapshot := nodeinfosnapshot.NewSnapshot(nodeinfosnapshot.CreateNodeInfoMap(test.pods, nodes))
			zoneSpread := ServiceAntiAffinity{podLister: snapshot.Pods(), serviceLister: fakelisters.ServiceLister(test.services), labels: []string{"zone"}}

			metaDataProducer := NewMetadataFactory(
				fakelisters.ServiceLister(test.services),
				fakelisters.ControllerLister(rcs),
				fakelisters.ReplicaSetLister(rss),
				fakelisters.StatefulSetLister(sss),
				1,
			)
			metaData := metaDataProducer(test.pod, nodes, snapshot)
			list, err := runMapReducePriority(zoneSpread.CalculateAntiAffinityPriorityMap, zoneSpread.CalculateAntiAffinityPriorityReduce, metaData, test.pod, snapshot, makeLabeledNodeList(test.nodes))
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			// sort the two lists to avoid failures on account of different ordering
			sortNodeScoreList(test.expectedList)
			sortNodeScoreList(list)
			if !reflect.DeepEqual(test.expectedList, list) {
				t.Errorf("expected %#v, got %#v", test.expectedList, list)
			}
		})
	}
}

func makeLabeledNodeList(nodeMap map[string]map[string]string) []*v1.Node {
	nodes := make([]*v1.Node, 0, len(nodeMap))
	for nodeName, labels := range nodeMap {
		nodes = append(nodes, &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: nodeName, Labels: labels}})
	}
	return nodes
}

func makeNodeList(nodeNames []string) []*v1.Node {
	nodes := make([]*v1.Node, 0, len(nodeNames))
	for _, nodeName := range nodeNames {
		nodes = append(nodes, &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: nodeName}})
	}
	return nodes
}
