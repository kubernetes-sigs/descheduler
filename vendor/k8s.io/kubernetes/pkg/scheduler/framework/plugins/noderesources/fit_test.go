/*
Copyright 2019 The Kubernetes Authors.

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

package noderesources

import (
	"context"
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	v1helper "k8s.io/kubernetes/pkg/apis/core/v1/helper"
	"k8s.io/kubernetes/pkg/features"
	"k8s.io/kubernetes/pkg/scheduler/algorithm/predicates"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/migration"
	framework "k8s.io/kubernetes/pkg/scheduler/framework/v1alpha1"
	schedulernodeinfo "k8s.io/kubernetes/pkg/scheduler/nodeinfo"
)

var (
	extendedResourceA     = v1.ResourceName("example.com/aaa")
	extendedResourceB     = v1.ResourceName("example.com/bbb")
	kubernetesIOResourceA = v1.ResourceName("kubernetes.io/something")
	kubernetesIOResourceB = v1.ResourceName("subdomain.kubernetes.io/something")
	hugePageResourceA     = v1helper.HugePageResourceName(resource.MustParse("2Mi"))
)

func makeResources(milliCPU, memory, pods, extendedA, storage, hugePageA int64) v1.NodeResources {
	return v1.NodeResources{
		Capacity: v1.ResourceList{
			v1.ResourceCPU:              *resource.NewMilliQuantity(milliCPU, resource.DecimalSI),
			v1.ResourceMemory:           *resource.NewQuantity(memory, resource.BinarySI),
			v1.ResourcePods:             *resource.NewQuantity(pods, resource.DecimalSI),
			extendedResourceA:           *resource.NewQuantity(extendedA, resource.DecimalSI),
			v1.ResourceEphemeralStorage: *resource.NewQuantity(storage, resource.BinarySI),
			hugePageResourceA:           *resource.NewQuantity(hugePageA, resource.BinarySI),
		},
	}
}

func makeAllocatableResources(milliCPU, memory, pods, extendedA, storage, hugePageA int64) v1.ResourceList {
	return v1.ResourceList{
		v1.ResourceCPU:              *resource.NewMilliQuantity(milliCPU, resource.DecimalSI),
		v1.ResourceMemory:           *resource.NewQuantity(memory, resource.BinarySI),
		v1.ResourcePods:             *resource.NewQuantity(pods, resource.DecimalSI),
		extendedResourceA:           *resource.NewQuantity(extendedA, resource.DecimalSI),
		v1.ResourceEphemeralStorage: *resource.NewQuantity(storage, resource.BinarySI),
		hugePageResourceA:           *resource.NewQuantity(hugePageA, resource.BinarySI),
	}
}

func newResourcePod(usage ...schedulernodeinfo.Resource) *v1.Pod {
	containers := []v1.Container{}
	for _, req := range usage {
		containers = append(containers, v1.Container{
			Resources: v1.ResourceRequirements{Requests: req.ResourceList()},
		})
	}
	return &v1.Pod{
		Spec: v1.PodSpec{
			Containers: containers,
		},
	}
}

func newResourceInitPod(pod *v1.Pod, usage ...schedulernodeinfo.Resource) *v1.Pod {
	pod.Spec.InitContainers = newResourcePod(usage...).Spec.Containers
	return pod
}

func newResourceOverheadPod(pod *v1.Pod, overhead v1.ResourceList) *v1.Pod {
	pod.Spec.Overhead = overhead
	return pod
}
func TestNodeResourcesFit(t *testing.T) {
	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.PodOverhead, true)()

	enoughPodsTests := []struct {
		pod                      *v1.Pod
		nodeInfo                 *schedulernodeinfo.NodeInfo
		name                     string
		ignoredExtendedResources sets.String
		wantStatus               *framework.Status
	}{
		{
			pod: &v1.Pod{},
			nodeInfo: schedulernodeinfo.NewNodeInfo(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 10, Memory: 20})),
			name: "no resources requested always fits",
		},
		{
			pod: newResourcePod(schedulernodeinfo.Resource{MilliCPU: 1, Memory: 1}),
			nodeInfo: schedulernodeinfo.NewNodeInfo(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 10, Memory: 20})),
			name:       "too many resources fails",
			wantStatus: framework.NewStatus(framework.Unschedulable, predicates.NewInsufficientResourceError(v1.ResourceCPU, 2, 10, 10).GetReason()),
		},
		{
			pod: newResourceInitPod(newResourcePod(schedulernodeinfo.Resource{MilliCPU: 1, Memory: 1}), schedulernodeinfo.Resource{MilliCPU: 3, Memory: 1}),
			nodeInfo: schedulernodeinfo.NewNodeInfo(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 8, Memory: 19})),
			name:       "too many resources fails due to init container cpu",
			wantStatus: framework.NewStatus(framework.Unschedulable, predicates.NewInsufficientResourceError(v1.ResourceCPU, 3, 8, 10).GetReason()),
		},
		{
			pod: newResourceInitPod(newResourcePod(schedulernodeinfo.Resource{MilliCPU: 1, Memory: 1}), schedulernodeinfo.Resource{MilliCPU: 3, Memory: 1}, schedulernodeinfo.Resource{MilliCPU: 2, Memory: 1}),
			nodeInfo: schedulernodeinfo.NewNodeInfo(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 8, Memory: 19})),
			name:       "too many resources fails due to highest init container cpu",
			wantStatus: framework.NewStatus(framework.Unschedulable, predicates.NewInsufficientResourceError(v1.ResourceCPU, 3, 8, 10).GetReason()),
		},
		{
			pod: newResourceInitPod(newResourcePod(schedulernodeinfo.Resource{MilliCPU: 1, Memory: 1}), schedulernodeinfo.Resource{MilliCPU: 1, Memory: 3}),
			nodeInfo: schedulernodeinfo.NewNodeInfo(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 9, Memory: 19})),
			name:       "too many resources fails due to init container memory",
			wantStatus: framework.NewStatus(framework.Unschedulable, predicates.NewInsufficientResourceError(v1.ResourceMemory, 3, 19, 20).GetReason()),
		},
		{
			pod: newResourceInitPod(newResourcePod(schedulernodeinfo.Resource{MilliCPU: 1, Memory: 1}), schedulernodeinfo.Resource{MilliCPU: 1, Memory: 3}, schedulernodeinfo.Resource{MilliCPU: 1, Memory: 2}),
			nodeInfo: schedulernodeinfo.NewNodeInfo(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 9, Memory: 19})),
			name:       "too many resources fails due to highest init container memory",
			wantStatus: framework.NewStatus(framework.Unschedulable, predicates.NewInsufficientResourceError(v1.ResourceMemory, 3, 19, 20).GetReason()),
		},
		{
			pod: newResourceInitPod(newResourcePod(schedulernodeinfo.Resource{MilliCPU: 1, Memory: 1}), schedulernodeinfo.Resource{MilliCPU: 1, Memory: 1}),
			nodeInfo: schedulernodeinfo.NewNodeInfo(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 9, Memory: 19})),
			name: "init container fits because it's the max, not sum, of containers and init containers",
		},
		{
			pod: newResourceInitPod(newResourcePod(schedulernodeinfo.Resource{MilliCPU: 1, Memory: 1}), schedulernodeinfo.Resource{MilliCPU: 1, Memory: 1}, schedulernodeinfo.Resource{MilliCPU: 1, Memory: 1}),
			nodeInfo: schedulernodeinfo.NewNodeInfo(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 9, Memory: 19})),
			name: "multiple init containers fit because it's the max, not sum, of containers and init containers",
		},
		{
			pod: newResourcePod(schedulernodeinfo.Resource{MilliCPU: 1, Memory: 1}),
			nodeInfo: schedulernodeinfo.NewNodeInfo(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 5, Memory: 5})),
			name: "both resources fit",
		},
		{
			pod: newResourcePod(schedulernodeinfo.Resource{MilliCPU: 2, Memory: 1}),
			nodeInfo: schedulernodeinfo.NewNodeInfo(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 9, Memory: 5})),
			name:       "one resource memory fits",
			wantStatus: framework.NewStatus(framework.Unschedulable, predicates.NewInsufficientResourceError(v1.ResourceCPU, 2, 9, 10).GetReason()),
		},
		{
			pod: newResourcePod(schedulernodeinfo.Resource{MilliCPU: 1, Memory: 2}),
			nodeInfo: schedulernodeinfo.NewNodeInfo(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 5, Memory: 19})),
			name:       "one resource cpu fits",
			wantStatus: framework.NewStatus(framework.Unschedulable, predicates.NewInsufficientResourceError(v1.ResourceMemory, 2, 19, 20).GetReason()),
		},
		{
			pod: newResourcePod(schedulernodeinfo.Resource{MilliCPU: 5, Memory: 1}),
			nodeInfo: schedulernodeinfo.NewNodeInfo(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 5, Memory: 19})),
			name: "equal edge case",
		},
		{
			pod: newResourceInitPod(newResourcePod(schedulernodeinfo.Resource{MilliCPU: 4, Memory: 1}), schedulernodeinfo.Resource{MilliCPU: 5, Memory: 1}),
			nodeInfo: schedulernodeinfo.NewNodeInfo(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 5, Memory: 19})),
			name: "equal edge case for init container",
		},
		{
			pod:      newResourcePod(schedulernodeinfo.Resource{ScalarResources: map[v1.ResourceName]int64{extendedResourceA: 1}}),
			nodeInfo: schedulernodeinfo.NewNodeInfo(newResourcePod(schedulernodeinfo.Resource{})),
			name:     "extended resource fits",
		},
		{
			pod:      newResourceInitPod(newResourcePod(schedulernodeinfo.Resource{}), schedulernodeinfo.Resource{ScalarResources: map[v1.ResourceName]int64{extendedResourceA: 1}}),
			nodeInfo: schedulernodeinfo.NewNodeInfo(newResourcePod(schedulernodeinfo.Resource{})),
			name:     "extended resource fits for init container",
		},
		{
			pod: newResourcePod(
				schedulernodeinfo.Resource{MilliCPU: 1, Memory: 1, ScalarResources: map[v1.ResourceName]int64{extendedResourceA: 10}}),
			nodeInfo: schedulernodeinfo.NewNodeInfo(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 0, Memory: 0, ScalarResources: map[v1.ResourceName]int64{extendedResourceA: 0}})),
			name:       "extended resource capacity enforced",
			wantStatus: framework.NewStatus(framework.Unschedulable, predicates.NewInsufficientResourceError(extendedResourceA, 10, 0, 5).GetReason()),
		},
		{
			pod: newResourceInitPod(newResourcePod(schedulernodeinfo.Resource{}),
				schedulernodeinfo.Resource{MilliCPU: 1, Memory: 1, ScalarResources: map[v1.ResourceName]int64{extendedResourceA: 10}}),
			nodeInfo: schedulernodeinfo.NewNodeInfo(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 0, Memory: 0, ScalarResources: map[v1.ResourceName]int64{extendedResourceA: 0}})),
			name:       "extended resource capacity enforced for init container",
			wantStatus: framework.NewStatus(framework.Unschedulable, predicates.NewInsufficientResourceError(extendedResourceA, 10, 0, 5).GetReason()),
		},
		{
			pod: newResourcePod(
				schedulernodeinfo.Resource{MilliCPU: 1, Memory: 1, ScalarResources: map[v1.ResourceName]int64{extendedResourceA: 1}}),
			nodeInfo: schedulernodeinfo.NewNodeInfo(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 0, Memory: 0, ScalarResources: map[v1.ResourceName]int64{extendedResourceA: 5}})),
			name:       "extended resource allocatable enforced",
			wantStatus: framework.NewStatus(framework.Unschedulable, predicates.NewInsufficientResourceError(extendedResourceA, 1, 5, 5).GetReason()),
		},
		{
			pod: newResourceInitPod(newResourcePod(schedulernodeinfo.Resource{}),
				schedulernodeinfo.Resource{MilliCPU: 1, Memory: 1, ScalarResources: map[v1.ResourceName]int64{extendedResourceA: 1}}),
			nodeInfo: schedulernodeinfo.NewNodeInfo(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 0, Memory: 0, ScalarResources: map[v1.ResourceName]int64{extendedResourceA: 5}})),
			name:       "extended resource allocatable enforced for init container",
			wantStatus: framework.NewStatus(framework.Unschedulable, predicates.NewInsufficientResourceError(extendedResourceA, 1, 5, 5).GetReason()),
		},
		{
			pod: newResourcePod(
				schedulernodeinfo.Resource{MilliCPU: 1, Memory: 1, ScalarResources: map[v1.ResourceName]int64{extendedResourceA: 3}},
				schedulernodeinfo.Resource{MilliCPU: 1, Memory: 1, ScalarResources: map[v1.ResourceName]int64{extendedResourceA: 3}}),
			nodeInfo: schedulernodeinfo.NewNodeInfo(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 0, Memory: 0, ScalarResources: map[v1.ResourceName]int64{extendedResourceA: 2}})),
			name:       "extended resource allocatable enforced for multiple containers",
			wantStatus: framework.NewStatus(framework.Unschedulable, predicates.NewInsufficientResourceError(extendedResourceA, 6, 2, 5).GetReason()),
		},
		{
			pod: newResourceInitPod(newResourcePod(schedulernodeinfo.Resource{}),
				schedulernodeinfo.Resource{MilliCPU: 1, Memory: 1, ScalarResources: map[v1.ResourceName]int64{extendedResourceA: 3}},
				schedulernodeinfo.Resource{MilliCPU: 1, Memory: 1, ScalarResources: map[v1.ResourceName]int64{extendedResourceA: 3}}),
			nodeInfo: schedulernodeinfo.NewNodeInfo(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 0, Memory: 0, ScalarResources: map[v1.ResourceName]int64{extendedResourceA: 2}})),
			name: "extended resource allocatable admits multiple init containers",
		},
		{
			pod: newResourceInitPod(newResourcePod(schedulernodeinfo.Resource{}),
				schedulernodeinfo.Resource{MilliCPU: 1, Memory: 1, ScalarResources: map[v1.ResourceName]int64{extendedResourceA: 6}},
				schedulernodeinfo.Resource{MilliCPU: 1, Memory: 1, ScalarResources: map[v1.ResourceName]int64{extendedResourceA: 3}}),
			nodeInfo: schedulernodeinfo.NewNodeInfo(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 0, Memory: 0, ScalarResources: map[v1.ResourceName]int64{extendedResourceA: 2}})),
			name:       "extended resource allocatable enforced for multiple init containers",
			wantStatus: framework.NewStatus(framework.Unschedulable, predicates.NewInsufficientResourceError(extendedResourceA, 6, 2, 5).GetReason()),
		},
		{
			pod: newResourcePod(
				schedulernodeinfo.Resource{MilliCPU: 1, Memory: 1, ScalarResources: map[v1.ResourceName]int64{extendedResourceB: 1}}),
			nodeInfo: schedulernodeinfo.NewNodeInfo(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 0, Memory: 0})),
			name:       "extended resource allocatable enforced for unknown resource",
			wantStatus: framework.NewStatus(framework.Unschedulable, predicates.NewInsufficientResourceError(extendedResourceB, 1, 0, 0).GetReason()),
		},
		{
			pod: newResourceInitPod(newResourcePod(schedulernodeinfo.Resource{}),
				schedulernodeinfo.Resource{MilliCPU: 1, Memory: 1, ScalarResources: map[v1.ResourceName]int64{extendedResourceB: 1}}),
			nodeInfo: schedulernodeinfo.NewNodeInfo(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 0, Memory: 0})),
			name:       "extended resource allocatable enforced for unknown resource for init container",
			wantStatus: framework.NewStatus(framework.Unschedulable, predicates.NewInsufficientResourceError(extendedResourceB, 1, 0, 0).GetReason()),
		},
		{
			pod: newResourcePod(
				schedulernodeinfo.Resource{MilliCPU: 1, Memory: 1, ScalarResources: map[v1.ResourceName]int64{kubernetesIOResourceA: 10}}),
			nodeInfo: schedulernodeinfo.NewNodeInfo(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 0, Memory: 0})),
			name:       "kubernetes.io resource capacity enforced",
			wantStatus: framework.NewStatus(framework.Unschedulable, predicates.NewInsufficientResourceError(kubernetesIOResourceA, 10, 0, 0).GetReason()),
		},
		{
			pod: newResourceInitPod(newResourcePod(schedulernodeinfo.Resource{}),
				schedulernodeinfo.Resource{MilliCPU: 1, Memory: 1, ScalarResources: map[v1.ResourceName]int64{kubernetesIOResourceB: 10}}),
			nodeInfo: schedulernodeinfo.NewNodeInfo(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 0, Memory: 0})),
			name:       "kubernetes.io resource capacity enforced for init container",
			wantStatus: framework.NewStatus(framework.Unschedulable, predicates.NewInsufficientResourceError(kubernetesIOResourceB, 10, 0, 0).GetReason()),
		},
		{
			pod: newResourcePod(
				schedulernodeinfo.Resource{MilliCPU: 1, Memory: 1, ScalarResources: map[v1.ResourceName]int64{hugePageResourceA: 10}}),
			nodeInfo: schedulernodeinfo.NewNodeInfo(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 0, Memory: 0, ScalarResources: map[v1.ResourceName]int64{hugePageResourceA: 0}})),
			name:       "hugepages resource capacity enforced",
			wantStatus: framework.NewStatus(framework.Unschedulable, predicates.NewInsufficientResourceError(hugePageResourceA, 10, 0, 5).GetReason()),
		},
		{
			pod: newResourceInitPod(newResourcePod(schedulernodeinfo.Resource{}),
				schedulernodeinfo.Resource{MilliCPU: 1, Memory: 1, ScalarResources: map[v1.ResourceName]int64{hugePageResourceA: 10}}),
			nodeInfo: schedulernodeinfo.NewNodeInfo(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 0, Memory: 0, ScalarResources: map[v1.ResourceName]int64{hugePageResourceA: 0}})),
			name:       "hugepages resource capacity enforced for init container",
			wantStatus: framework.NewStatus(framework.Unschedulable, predicates.NewInsufficientResourceError(hugePageResourceA, 10, 0, 5).GetReason()),
		},
		{
			pod: newResourcePod(
				schedulernodeinfo.Resource{MilliCPU: 1, Memory: 1, ScalarResources: map[v1.ResourceName]int64{hugePageResourceA: 3}},
				schedulernodeinfo.Resource{MilliCPU: 1, Memory: 1, ScalarResources: map[v1.ResourceName]int64{hugePageResourceA: 3}}),
			nodeInfo: schedulernodeinfo.NewNodeInfo(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 0, Memory: 0, ScalarResources: map[v1.ResourceName]int64{hugePageResourceA: 2}})),
			name:       "hugepages resource allocatable enforced for multiple containers",
			wantStatus: framework.NewStatus(framework.Unschedulable, predicates.NewInsufficientResourceError(hugePageResourceA, 6, 2, 5).GetReason()),
		},
		{
			pod: newResourcePod(
				schedulernodeinfo.Resource{MilliCPU: 1, Memory: 1, ScalarResources: map[v1.ResourceName]int64{extendedResourceB: 1}}),
			nodeInfo: schedulernodeinfo.NewNodeInfo(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 0, Memory: 0})),
			ignoredExtendedResources: sets.NewString(string(extendedResourceB)),
			name:                     "skip checking ignored extended resource",
			wantStatus:               framework.NewStatus(framework.Unschedulable, predicates.NewInsufficientResourceError(extendedResourceB, 2, 10, 10).GetReason()),
		},
		{
			pod: newResourceOverheadPod(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 1, Memory: 1}),
				v1.ResourceList{v1.ResourceCPU: resource.MustParse("3m"), v1.ResourceMemory: resource.MustParse("13")},
			),
			nodeInfo: schedulernodeinfo.NewNodeInfo(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 5, Memory: 5})),
			ignoredExtendedResources: sets.NewString(string(extendedResourceB)),
			name:                     "resources + pod overhead fits",
		},
		{
			pod: newResourceOverheadPod(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 1, Memory: 1}),
				v1.ResourceList{v1.ResourceCPU: resource.MustParse("1m"), v1.ResourceMemory: resource.MustParse("15")},
			),
			nodeInfo: schedulernodeinfo.NewNodeInfo(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 5, Memory: 5})),
			ignoredExtendedResources: sets.NewString(string(extendedResourceB)),
			name:                     "requests + overhead does not fit for memory",
			wantStatus:               framework.NewStatus(framework.Unschedulable, predicates.NewInsufficientResourceError(v1.ResourceMemory, 16, 5, 20).GetReason()),
		},
	}

	for _, test := range enoughPodsTests {
		t.Run(test.name, func(t *testing.T) {
			factory := &predicates.MetadataProducerFactory{}
			meta := factory.GetPredicateMetadata(test.pod, nil)
			state := framework.NewCycleState()
			state.Write(migration.PredicatesStateKey, &migration.PredicatesStateData{Reference: meta})

			node := v1.Node{Status: v1.NodeStatus{Capacity: makeResources(10, 20, 32, 5, 20, 5).Capacity, Allocatable: makeAllocatableResources(10, 20, 32, 5, 20, 5)}}
			test.nodeInfo.SetNode(&node)

			p, _ := NewFit(nil, nil)
			gotStatus := p.(framework.FilterPlugin).Filter(context.Background(), state, test.pod, test.nodeInfo)
			if !reflect.DeepEqual(gotStatus, test.wantStatus) {
				t.Errorf("status does not match: %v, want: %v", gotStatus, test.wantStatus)
			}
		})
	}

	notEnoughPodsTests := []struct {
		pod        *v1.Pod
		nodeInfo   *schedulernodeinfo.NodeInfo
		fits       bool
		name       string
		wantStatus *framework.Status
	}{
		{
			pod: &v1.Pod{},
			nodeInfo: schedulernodeinfo.NewNodeInfo(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 10, Memory: 20})),
			name:       "even without specified resources predicate fails when there's no space for additional pod",
			wantStatus: framework.NewStatus(framework.Unschedulable, predicates.NewInsufficientResourceError(v1.ResourcePods, 1, 1, 1).GetReason()),
		},
		{
			pod: newResourcePod(schedulernodeinfo.Resource{MilliCPU: 1, Memory: 1}),
			nodeInfo: schedulernodeinfo.NewNodeInfo(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 5, Memory: 5})),
			name:       "even if both resources fit predicate fails when there's no space for additional pod",
			wantStatus: framework.NewStatus(framework.Unschedulable, predicates.NewInsufficientResourceError(v1.ResourcePods, 1, 1, 1).GetReason()),
		},
		{
			pod: newResourcePod(schedulernodeinfo.Resource{MilliCPU: 5, Memory: 1}),
			nodeInfo: schedulernodeinfo.NewNodeInfo(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 5, Memory: 19})),
			name:       "even for equal edge case predicate fails when there's no space for additional pod",
			wantStatus: framework.NewStatus(framework.Unschedulable, predicates.NewInsufficientResourceError(v1.ResourcePods, 1, 1, 1).GetReason()),
		},
		{
			pod: newResourceInitPod(newResourcePod(schedulernodeinfo.Resource{MilliCPU: 5, Memory: 1}), schedulernodeinfo.Resource{MilliCPU: 5, Memory: 1}),
			nodeInfo: schedulernodeinfo.NewNodeInfo(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 5, Memory: 19})),
			name:       "even for equal edge case predicate fails when there's no space for additional pod due to init container",
			wantStatus: framework.NewStatus(framework.Unschedulable, predicates.NewInsufficientResourceError(v1.ResourcePods, 1, 1, 1).GetReason()),
		},
	}
	for _, test := range notEnoughPodsTests {
		t.Run(test.name, func(t *testing.T) {
			factory := &predicates.MetadataProducerFactory{}
			meta := factory.GetPredicateMetadata(test.pod, nil)
			state := framework.NewCycleState()
			state.Write(migration.PredicatesStateKey, &migration.PredicatesStateData{Reference: meta})

			node := v1.Node{Status: v1.NodeStatus{Capacity: v1.ResourceList{}, Allocatable: makeAllocatableResources(10, 20, 1, 0, 0, 0)}}
			test.nodeInfo.SetNode(&node)

			p, _ := NewFit(nil, nil)
			gotStatus := p.(framework.FilterPlugin).Filter(context.Background(), state, test.pod, test.nodeInfo)
			if !reflect.DeepEqual(gotStatus, test.wantStatus) {
				t.Errorf("status does not match: %v, want: %v", gotStatus, test.wantStatus)
			}
		})
	}

	storagePodsTests := []struct {
		pod        *v1.Pod
		nodeInfo   *schedulernodeinfo.NodeInfo
		name       string
		wantStatus *framework.Status
	}{
		{
			pod: newResourcePod(schedulernodeinfo.Resource{MilliCPU: 1, Memory: 1}),
			nodeInfo: schedulernodeinfo.NewNodeInfo(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 10, Memory: 10})),
			name:       "due to container scratch disk",
			wantStatus: framework.NewStatus(framework.Unschedulable, predicates.NewInsufficientResourceError(v1.ResourceCPU, 1, 10, 10).GetReason()),
		},
		{
			pod: newResourcePod(schedulernodeinfo.Resource{MilliCPU: 1, Memory: 1}),
			nodeInfo: schedulernodeinfo.NewNodeInfo(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 2, Memory: 10})),
			name: "pod fit",
		},
		{
			pod: newResourcePod(schedulernodeinfo.Resource{EphemeralStorage: 25}),
			nodeInfo: schedulernodeinfo.NewNodeInfo(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 2, Memory: 2})),
			name:       "storage ephemeral local storage request exceeds allocatable",
			wantStatus: framework.NewStatus(framework.Unschedulable, predicates.NewInsufficientResourceError(v1.ResourceEphemeralStorage, 25, 0, 20).GetReason()),
		},
		{
			pod: newResourcePod(schedulernodeinfo.Resource{EphemeralStorage: 10}),
			nodeInfo: schedulernodeinfo.NewNodeInfo(
				newResourcePod(schedulernodeinfo.Resource{MilliCPU: 2, Memory: 2})),
			name: "pod fits",
		},
	}

	for _, test := range storagePodsTests {
		t.Run(test.name, func(t *testing.T) {
			factory := &predicates.MetadataProducerFactory{}
			meta := factory.GetPredicateMetadata(test.pod, nil)
			state := framework.NewCycleState()
			state.Write(migration.PredicatesStateKey, &migration.PredicatesStateData{Reference: meta})

			node := v1.Node{Status: v1.NodeStatus{Capacity: makeResources(10, 20, 32, 5, 20, 5).Capacity, Allocatable: makeAllocatableResources(10, 20, 32, 5, 20, 5)}}
			test.nodeInfo.SetNode(&node)

			p, _ := NewFit(nil, nil)
			gotStatus := p.(framework.FilterPlugin).Filter(context.Background(), state, test.pod, test.nodeInfo)
			if !reflect.DeepEqual(gotStatus, test.wantStatus) {
				t.Errorf("status does not match: %v, want: %v", gotStatus, test.wantStatus)
			}
		})
	}

}
