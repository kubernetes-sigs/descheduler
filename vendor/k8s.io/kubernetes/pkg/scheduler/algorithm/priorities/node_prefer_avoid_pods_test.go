/*
Copyright 2016 The Kubernetes Authors.

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

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	framework "k8s.io/kubernetes/pkg/scheduler/framework/v1alpha1"
	nodeinfosnapshot "k8s.io/kubernetes/pkg/scheduler/nodeinfo/snapshot"
)

func TestNodePreferAvoidPriority(t *testing.T) {
	annotations1 := map[string]string{
		v1.PreferAvoidPodsAnnotationKey: `
							{
							    "preferAvoidPods": [
							        {
							            "podSignature": {
							                "podController": {
							                    "apiVersion": "v1",
							                    "kind": "ReplicationController",
							                    "name": "foo",
							                    "uid": "abcdef123456",
							                    "controller": true
							                }
							            },
							            "reason": "some reason",
							            "message": "some message"
							        }
							    ]
							}`,
	}
	annotations2 := map[string]string{
		v1.PreferAvoidPodsAnnotationKey: `
							{
							    "preferAvoidPods": [
							        {
							            "podSignature": {
							                "podController": {
							                    "apiVersion": "v1",
							                    "kind": "ReplicaSet",
							                    "name": "foo",
							                    "uid": "qwert12345",
							                    "controller": true
							                }
							            },
							            "reason": "some reason",
							            "message": "some message"
							        }
							    ]
							}`,
	}
	testNodes := []*v1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "machine1", Annotations: annotations1},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "machine2", Annotations: annotations2},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "machine3"},
		},
	}
	trueVar := true
	tests := []struct {
		pod          *v1.Pod
		nodes        []*v1.Node
		expectedList framework.NodeScoreList
		name         string
	}{
		{
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{Kind: "ReplicationController", Name: "foo", UID: "abcdef123456", Controller: &trueVar},
					},
				},
			},
			nodes:        testNodes,
			expectedList: []framework.NodeScore{{Name: "machine1", Score: 0}, {Name: "machine2", Score: framework.MaxNodeScore}, {Name: "machine3", Score: framework.MaxNodeScore}},
			name:         "pod managed by ReplicationController should avoid a node, this node get lowest priority score",
		},
		{
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{Kind: "RandomController", Name: "foo", UID: "abcdef123456", Controller: &trueVar},
					},
				},
			},
			nodes:        testNodes,
			expectedList: []framework.NodeScore{{Name: "machine1", Score: framework.MaxNodeScore}, {Name: "machine2", Score: framework.MaxNodeScore}, {Name: "machine3", Score: framework.MaxNodeScore}},
			name:         "ownership by random controller should be ignored",
		},
		{
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{Kind: "ReplicationController", Name: "foo", UID: "abcdef123456"},
					},
				},
			},
			nodes:        testNodes,
			expectedList: []framework.NodeScore{{Name: "machine1", Score: framework.MaxNodeScore}, {Name: "machine2", Score: framework.MaxNodeScore}, {Name: "machine3", Score: framework.MaxNodeScore}},
			name:         "owner without Controller field set should be ignored",
		},
		{
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{Kind: "ReplicaSet", Name: "foo", UID: "qwert12345", Controller: &trueVar},
					},
				},
			},
			nodes:        testNodes,
			expectedList: []framework.NodeScore{{Name: "machine1", Score: framework.MaxNodeScore}, {Name: "machine2", Score: 0}, {Name: "machine3", Score: framework.MaxNodeScore}},
			name:         "pod managed by ReplicaSet should avoid a node, this node get lowest priority score",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := nodeinfosnapshot.NewSnapshot(nodeinfosnapshot.CreateNodeInfoMap(nil, test.nodes))
			list, err := runMapReducePriority(CalculateNodePreferAvoidPodsPriorityMap, nil, nil, test.pod, snapshot, test.nodes)
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
