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

package priorities

import (
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	framework "k8s.io/kubernetes/pkg/scheduler/framework/v1alpha1"
	nodeinfosnapshot "k8s.io/kubernetes/pkg/scheduler/nodeinfo/snapshot"
)

func TestResourceLimitsPriority(t *testing.T) {
	noResources := v1.PodSpec{
		Containers: []v1.Container{},
	}

	cpuOnly := v1.PodSpec{
		NodeName: "machine1",
		Containers: []v1.Container{
			{
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceCPU:    resource.MustParse("1000m"),
						v1.ResourceMemory: resource.MustParse("0"),
					},
				},
			},
			{
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceCPU:    resource.MustParse("2000m"),
						v1.ResourceMemory: resource.MustParse("0"),
					},
				},
			},
		},
	}

	memOnly := v1.PodSpec{
		NodeName: "machine2",
		Containers: []v1.Container{
			{
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceCPU:    resource.MustParse("0"),
						v1.ResourceMemory: resource.MustParse("2000"),
					},
				},
			},
			{
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceCPU:    resource.MustParse("0"),
						v1.ResourceMemory: resource.MustParse("3000"),
					},
				},
			},
		},
	}

	cpuAndMemory := v1.PodSpec{
		NodeName: "machine2",
		Containers: []v1.Container{
			{
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceCPU:    resource.MustParse("1000m"),
						v1.ResourceMemory: resource.MustParse("2000"),
					},
				},
			},
			{
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceCPU:    resource.MustParse("2000m"),
						v1.ResourceMemory: resource.MustParse("3000"),
					},
				},
			},
		},
	}

	tests := []struct {
		// input pod
		pod          *v1.Pod
		nodes        []*v1.Node
		expectedList framework.NodeScoreList
		name         string
	}{
		{
			pod:          &v1.Pod{Spec: noResources},
			nodes:        []*v1.Node{makeNode("machine1", 4000, 10000), makeNode("machine2", 4000, 0), makeNode("machine3", 0, 10000), makeNode("machine4", 0, 0)},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: 0}, {Name: "machine2", Score: 0}, {Name: "machine3", Score: 0}, {Name: "machine4", Score: 0}},
			name:         "pod does not specify its resource limits",
		},
		{
			pod:          &v1.Pod{Spec: cpuOnly},
			nodes:        []*v1.Node{makeNode("machine1", 3000, 10000), makeNode("machine2", 2000, 10000)},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: 1}, {Name: "machine2", Score: 0}},
			name:         "pod only specifies  cpu limits",
		},
		{
			pod:          &v1.Pod{Spec: memOnly},
			nodes:        []*v1.Node{makeNode("machine1", 4000, 4000), makeNode("machine2", 5000, 10000)},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: 0}, {Name: "machine2", Score: 1}},
			name:         "pod only specifies  mem limits",
		},
		{
			pod:          &v1.Pod{Spec: cpuAndMemory},
			nodes:        []*v1.Node{makeNode("machine1", 4000, 4000), makeNode("machine2", 5000, 10000)},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: 1}, {Name: "machine2", Score: 1}},
			name:         "pod specifies both cpu and  mem limits",
		},
		{
			pod:          &v1.Pod{Spec: cpuAndMemory},
			nodes:        []*v1.Node{makeNode("machine1", 0, 0)},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: 0}},
			name:         "node does not advertise its allocatables",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := nodeinfosnapshot.NewSnapshot(nodeinfosnapshot.CreateNodeInfoMap(nil, test.nodes))
			metadata := &priorityMetadata{
				podLimits: getResourceLimits(test.pod),
			}

			for _, hasMeta := range []bool{true, false} {
				meta := metadata
				if !hasMeta {
					meta = nil
				}

				list, err := runMapReducePriority(ResourceLimitsPriorityMap, nil, meta, test.pod, snapshot, test.nodes)

				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if !reflect.DeepEqual(test.expectedList, list) {
					t.Errorf("hasMeta %#v expected %#v, got %#v", hasMeta, test.expectedList, list)
				}
			}
		})
	}
}
