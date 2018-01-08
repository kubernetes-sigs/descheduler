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
	"fmt"
	"strings"
	"testing"

	"github.com/kubernetes-incubator/descheduler/pkg/api"
	"github.com/kubernetes-incubator/descheduler/test"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	core "k8s.io/client-go/testing"
	"k8s.io/kubernetes/pkg/api/v1"
	"k8s.io/kubernetes/pkg/client/clientset_generated/clientset/fake"
)

// TODO: Make this table driven.
func TestLowNodeUtilization(t *testing.T) {
	var thresholds = make(api.ResourceThresholds)
	var targetThresholds = make(api.ResourceThresholds)
	thresholds[v1.ResourceCPU] = 30
	thresholds[v1.ResourcePods] = 30
	targetThresholds[v1.ResourceCPU] = 50
	targetThresholds[v1.ResourcePods] = 50

	n1 := test.BuildTestNode("n1", 4000, 3000, 9)
	n2 := test.BuildTestNode("n2", 4000, 3000, 10)
	n3 := test.BuildTestNode("n3", 4000, 3000, 10)
	// Making n3 node unschedulable so that it won't counted in lowUtilized nodes list.
	n3.Spec.Unschedulable = true
	p1 := test.BuildTestPod("p1", 400, 0, n1.Name)
	p2 := test.BuildTestPod("p2", 400, 0, n1.Name)
	p3 := test.BuildTestPod("p3", 400, 0, n1.Name)
	p4 := test.BuildTestPod("p4", 400, 0, n1.Name)
	p5 := test.BuildTestPod("p5", 400, 0, n1.Name)

	// These won't be evicted.
	p6 := test.BuildTestPod("p6", 400, 0, n1.Name)
	p7 := test.BuildTestPod("p7", 400, 0, n1.Name)
	p8 := test.BuildTestPod("p8", 400, 0, n1.Name)

	p1.ObjectMeta.OwnerReferences = test.GetReplicaSetOwnerRefList()
	p2.ObjectMeta.OwnerReferences = test.GetReplicaSetOwnerRefList()
	p3.ObjectMeta.OwnerReferences = test.GetReplicaSetOwnerRefList()
	p4.ObjectMeta.OwnerReferences = test.GetReplicaSetOwnerRefList()
	p5.ObjectMeta.OwnerReferences = test.GetReplicaSetOwnerRefList()
	// The following 4 pods won't get evicted.
	// A daemonset.
	p6.ObjectMeta.OwnerReferences = test.GetDaemonSetOwnerRefList()
	// A pod with local storage.
	p7.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
	p7.Spec.Volumes = []v1.Volume{
		{
			Name: "sample",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{Path: "somePath"},
				EmptyDir: &v1.EmptyDirVolumeSource{
					SizeLimit: resource.NewQuantity(int64(10), resource.BinarySI)},
			},
		},
	}
	// A Mirror Pod.
	p7.Annotations = test.GetMirrorPodAnnotation()
	// A Critical Pod.
	p8.Namespace = "kube-system"
	p8.Annotations = test.GetCriticalPodAnnotation()
	p9 := test.BuildTestPod("p9", 400, 0, n1.Name)
	p9.ObjectMeta.OwnerReferences = test.GetReplicaSetOwnerRefList()
	fakeClient := &fake.Clientset{}
	fakeClient.Fake.AddReactor("list", "pods", func(action core.Action) (bool, runtime.Object, error) {
		list := action.(core.ListAction)
		fieldString := list.GetListRestrictions().Fields.String()
		if strings.Contains(fieldString, "n1") {
			return true, &v1.PodList{Items: []v1.Pod{*p1, *p2, *p3, *p4, *p5, *p6, *p7, *p8}}, nil
		}
		if strings.Contains(fieldString, "n2") {
			return true, &v1.PodList{Items: []v1.Pod{*p9}}, nil
		}
		if strings.Contains(fieldString, "n3") {
			return true, &v1.PodList{Items: []v1.Pod{}}, nil
		}
		return true, nil, fmt.Errorf("Failed to list: %v", list)
	})
	fakeClient.Fake.AddReactor("get", "nodes", func(action core.Action) (bool, runtime.Object, error) {
		getAction := action.(core.GetAction)
		switch getAction.GetName() {
		case n1.Name:
			return true, n1, nil
		case n2.Name:
			return true, n2, nil
		case n3.Name:
			return true, n3, nil
		}
		return true, nil, fmt.Errorf("Wrong node: %v", getAction.GetName())
	})
	expectedPodsEvicted := 4
	npm := CreateNodePodsMap(fakeClient, []*v1.Node{n1, n2, n3})
	lowNodes, targetNodes := classifyNodes(npm, thresholds, targetThresholds)
	if len(lowNodes) != 1 {
		t.Errorf("After ignoring unschedulable nodes, expected only one node to be under utilized.")
	}
	podsEvicted := evictPodsFromTargetNodes(fakeClient, "v1", targetNodes, lowNodes, targetThresholds, false)
	if expectedPodsEvicted != podsEvicted {
		t.Errorf("Expected %#v pods to be evicted but %#v got evicted", expectedPodsEvicted)
	}

}

func TestValidateThresholds(t *testing.T) {
	tests := []struct {
		name    string
		input   api.ResourceThresholds
		succeed bool
	}{
		{
			name:    "passing nil map for threshold",
			input:   nil,
			succeed: false,
		},
		{
			name:    "passing no threshold",
			input:   api.ResourceThresholds{},
			succeed: false,
		},
		{
			name: "passing unsupported resource name",
			input: api.ResourceThresholds{
				v1.ResourceCPU:     40,
				v1.ResourceStorage: 25.5,
			},
			succeed: false,
		},
		{
			name: "passing invalid resource name",
			input: api.ResourceThresholds{
				v1.ResourceCPU: 40,
				"coolResource": 42.0,
			},
			succeed: false,
		},
		{
			name: "passing a valid threshold with cpu, memory and pods",
			input: api.ResourceThresholds{
				v1.ResourceCPU:    20,
				v1.ResourceMemory: 30,
				v1.ResourcePods:   40,
			},
			succeed: true,
		},
	}

	for _, test := range tests {
		isValid := validateThresholds(test.input)

		if isValid != test.succeed {
			t.Errorf("expected validity of threshold: %#v\nto be %v but got %v instead", test.input, test.succeed, isValid)
		}
	}
}
