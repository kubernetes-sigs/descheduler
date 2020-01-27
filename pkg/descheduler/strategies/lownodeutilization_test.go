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

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"reflect"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/utils"
	"sigs.k8s.io/descheduler/test"
)

// TODO: Make this table driven.
func TestLowNodeUtilizationWithoutPriority(t *testing.T) {
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
	expectedPodsEvicted := 3
	npm := createNodePodsMap(fakeClient, []*v1.Node{n1, n2, n3})
	lowNodes, targetNodes := classifyNodes(npm, thresholds, targetThresholds, false)
	if len(lowNodes) != 1 {
		t.Errorf("After ignoring unschedulable nodes, expected only one node to be under utilized.")
	}
	npe := utils.NodePodEvictedCount{}
	npe[n1] = 0
	npe[n2] = 0
	npe[n3] = 0
	podsEvicted := evictPodsFromTargetNodes(fakeClient, "v1", targetNodes, lowNodes, targetThresholds, false, 3, npe)
	if expectedPodsEvicted != podsEvicted {
		t.Errorf("Expected %#v pods to be evicted but %#v got evicted", expectedPodsEvicted, podsEvicted)
	}

}

// TODO: Make this table driven.
func TestLowNodeUtilizationWithPriorities(t *testing.T) {
	var thresholds = make(api.ResourceThresholds)
	var targetThresholds = make(api.ResourceThresholds)
	thresholds[v1.ResourceCPU] = 30
	thresholds[v1.ResourcePods] = 30
	targetThresholds[v1.ResourceCPU] = 50
	targetThresholds[v1.ResourcePods] = 50
	lowPriority := int32(0)
	highPriority := int32(10000)
	n1 := test.BuildTestNode("n1", 4000, 3000, 9)
	n2 := test.BuildTestNode("n2", 4000, 3000, 10)
	n3 := test.BuildTestNode("n3", 4000, 3000, 10)
	// Making n3 node unschedulable so that it won't counted in lowUtilized nodes list.
	n3.Spec.Unschedulable = true
	p1 := test.BuildTestPod("p1", 400, 0, n1.Name)
	p1.Spec.Priority = &highPriority
	p2 := test.BuildTestPod("p2", 400, 0, n1.Name)
	p2.Spec.Priority = &highPriority
	p3 := test.BuildTestPod("p3", 400, 0, n1.Name)
	p3.Spec.Priority = &highPriority
	p4 := test.BuildTestPod("p4", 400, 0, n1.Name)
	p4.Spec.Priority = &highPriority
	p5 := test.BuildTestPod("p5", 400, 0, n1.Name)
	p5.Spec.Priority = &lowPriority

	// These won't be evicted.
	p6 := test.BuildTestPod("p6", 400, 0, n1.Name)
	p6.Spec.Priority = &highPriority
	p7 := test.BuildTestPod("p7", 400, 0, n1.Name)
	p7.Spec.Priority = &lowPriority
	p8 := test.BuildTestPod("p8", 400, 0, n1.Name)
	p8.Spec.Priority = &lowPriority

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
	expectedPodsEvicted := 3
	npm := createNodePodsMap(fakeClient, []*v1.Node{n1, n2, n3})
	lowNodes, targetNodes := classifyNodes(npm, thresholds, targetThresholds, false)
	if len(lowNodes) != 1 {
		t.Errorf("After ignoring unschedulable nodes, expected only one node to be under utilized.")
	}
	npe := utils.NodePodEvictedCount{}
	npe[n1] = 0
	npe[n2] = 0
	npe[n3] = 0
	podsEvicted := evictPodsFromTargetNodes(fakeClient, "v1", targetNodes, lowNodes, targetThresholds, false, 3, npe)
	if expectedPodsEvicted != podsEvicted {
		t.Errorf("Expected %#v pods to be evicted but %#v got evicted", expectedPodsEvicted, podsEvicted)
	}

}

func TestSortPodsByPriority(t *testing.T) {
	n1 := test.BuildTestNode("n1", 4000, 3000, 9)
	lowPriority := int32(0)
	highPriority := int32(10000)
	p1 := test.BuildTestPod("p1", 400, 0, n1.Name)
	p1.Spec.Priority = &lowPriority

	// BestEffort
	p2 := test.BuildTestPod("p2", 400, 0, n1.Name)
	p2.Spec.Priority = &highPriority

	p2.Spec.Containers[0].Resources.Requests = nil
	p2.Spec.Containers[0].Resources.Limits = nil

	// Burstable
	p3 := test.BuildTestPod("p3", 400, 0, n1.Name)
	p3.Spec.Priority = &highPriority

	// Guaranteed
	p4 := test.BuildTestPod("p4", 400, 100, n1.Name)
	p4.Spec.Priority = &highPriority
	p4.Spec.Containers[0].Resources.Limits[v1.ResourceCPU] = *resource.NewMilliQuantity(400, resource.DecimalSI)
	p4.Spec.Containers[0].Resources.Limits[v1.ResourceMemory] = *resource.NewQuantity(100, resource.DecimalSI)

	// Best effort with nil priorities.
	p5 := test.BuildTestPod("p5", 400, 100, n1.Name)
	p5.Spec.Priority = nil
	p6 := test.BuildTestPod("p6", 400, 100, n1.Name)
	p6.Spec.Containers[0].Resources.Limits[v1.ResourceCPU] = *resource.NewMilliQuantity(400, resource.DecimalSI)
	p6.Spec.Containers[0].Resources.Limits[v1.ResourceMemory] = *resource.NewQuantity(100, resource.DecimalSI)
	p6.Spec.Priority = nil

	podList := []*v1.Pod{p4, p3, p2, p1, p6, p5}

	sortPodsBasedOnPriority(podList)
	if !reflect.DeepEqual(podList[len(podList)-1], p4) {
		t.Errorf("Expected last pod in sorted list to be %v which of highest priority and guaranteed but got %v", p4, podList[len(podList)-1])
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
