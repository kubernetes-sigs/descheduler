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

package evictions

import (
	"context"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/utils"
	"sigs.k8s.io/descheduler/test"
)

func TestEvictPod(t *testing.T) {
	ctx := context.Background()
	node1 := test.BuildTestNode("node1", 1000, 2000, 9, nil)
	pod1 := test.BuildTestPod("p1", 400, 0, "node1", nil)
	tests := []struct {
		description string
		node        *v1.Node
		pod         *v1.Pod
		pods        []v1.Pod
		want        error
	}{
		{
			description: "test pod eviction - pod present",
			node:        node1,
			pod:         pod1,
			pods:        []v1.Pod{*pod1},
			want:        nil,
		},
		{
			description: "test pod eviction - pod absent",
			node:        node1,
			pod:         pod1,
			pods:        []v1.Pod{*test.BuildTestPod("p2", 400, 0, "node1", nil), *test.BuildTestPod("p3", 450, 0, "node1", nil)},
			want:        nil,
		},
	}

	for _, test := range tests {
		fakeClient := &fake.Clientset{}
		fakeClient.Fake.AddReactor("list", "pods", func(action core.Action) (bool, runtime.Object, error) {
			return true, &v1.PodList{Items: test.pods}, nil
		})
		got := evictPod(ctx, fakeClient, test.pod, "v1", false)
		if got != test.want {
			t.Errorf("Test error for Desc: %s. Expected %v pod eviction to be %v, got %v", test.description, test.pod.Name, test.want, got)
		}
	}
}

func TestIsEvictable(t *testing.T) {
	n1 := test.BuildTestNode("node1", 1000, 2000, 13, nil)
	lowPriority := int32(800)
	highPriority := int32(900)
	type testCase struct {
		pod                   *v1.Pod
		runBefore             func(*v1.Pod)
		evictLocalStoragePods bool
		priorityThreshold     *int32
		result                bool
	}

	testCases := []testCase{
		{
			pod: test.BuildTestPod("p1", 400, 0, n1.Name, nil),
			runBefore: func(pod *v1.Pod) {
				pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
			},
			evictLocalStoragePods: false,
			result:                true,
		}, {
			pod: test.BuildTestPod("p2", 400, 0, n1.Name, nil),
			runBefore: func(pod *v1.Pod) {
				pod.Annotations = map[string]string{"descheduler.alpha.kubernetes.io/evict": "true"}
				pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
			},
			evictLocalStoragePods: false,
			result:                true,
		}, {
			pod: test.BuildTestPod("p3", 400, 0, n1.Name, nil),
			runBefore: func(pod *v1.Pod) {
				pod.ObjectMeta.OwnerReferences = test.GetReplicaSetOwnerRefList()
			},
			evictLocalStoragePods: false,
			result:                true,
		}, {
			pod: test.BuildTestPod("p4", 400, 0, n1.Name, nil),
			runBefore: func(pod *v1.Pod) {
				pod.Annotations = map[string]string{"descheduler.alpha.kubernetes.io/evict": "true"}
				pod.ObjectMeta.OwnerReferences = test.GetReplicaSetOwnerRefList()
			},
			evictLocalStoragePods: false,
			result:                true,
		}, {
			pod: test.BuildTestPod("p5", 400, 0, n1.Name, nil),
			runBefore: func(pod *v1.Pod) {
				pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
				pod.Spec.Volumes = []v1.Volume{
					{
						Name: "sample",
						VolumeSource: v1.VolumeSource{
							HostPath: &v1.HostPathVolumeSource{Path: "somePath"},
							EmptyDir: &v1.EmptyDirVolumeSource{
								SizeLimit: resource.NewQuantity(int64(10), resource.BinarySI)},
						},
					},
				}
			},
			evictLocalStoragePods: false,
			result:                false,
		}, {
			pod: test.BuildTestPod("p6", 400, 0, n1.Name, nil),
			runBefore: func(pod *v1.Pod) {
				pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
				pod.Spec.Volumes = []v1.Volume{
					{
						Name: "sample",
						VolumeSource: v1.VolumeSource{
							HostPath: &v1.HostPathVolumeSource{Path: "somePath"},
							EmptyDir: &v1.EmptyDirVolumeSource{
								SizeLimit: resource.NewQuantity(int64(10), resource.BinarySI)},
						},
					},
				}
			},
			evictLocalStoragePods: true,
			result:                true,
		}, {
			pod: test.BuildTestPod("p7", 400, 0, n1.Name, nil),
			runBefore: func(pod *v1.Pod) {
				pod.Annotations = map[string]string{"descheduler.alpha.kubernetes.io/evict": "true"}
				pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
				pod.Spec.Volumes = []v1.Volume{
					{
						Name: "sample",
						VolumeSource: v1.VolumeSource{
							HostPath: &v1.HostPathVolumeSource{Path: "somePath"},
							EmptyDir: &v1.EmptyDirVolumeSource{
								SizeLimit: resource.NewQuantity(int64(10), resource.BinarySI)},
						},
					},
				}
			},
			evictLocalStoragePods: false,
			result:                true,
		}, {
			pod: test.BuildTestPod("p8", 400, 0, n1.Name, nil),
			runBefore: func(pod *v1.Pod) {
				pod.ObjectMeta.OwnerReferences = test.GetDaemonSetOwnerRefList()
			},
			evictLocalStoragePods: false,
			result:                false,
		}, {
			pod: test.BuildTestPod("p9", 400, 0, n1.Name, nil),
			runBefore: func(pod *v1.Pod) {
				pod.Annotations = map[string]string{"descheduler.alpha.kubernetes.io/evict": "true"}
				pod.ObjectMeta.OwnerReferences = test.GetDaemonSetOwnerRefList()
			},
			evictLocalStoragePods: false,
			result:                true,
		}, {
			pod: test.BuildTestPod("p10", 400, 0, n1.Name, nil),
			runBefore: func(pod *v1.Pod) {
				pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
				pod.Annotations = test.GetMirrorPodAnnotation()
			},
			evictLocalStoragePods: false,
			result:                false,
		}, {
			pod: test.BuildTestPod("p11", 400, 0, n1.Name, nil),
			runBefore: func(pod *v1.Pod) {
				pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
				pod.Annotations = test.GetMirrorPodAnnotation()
				pod.Annotations["descheduler.alpha.kubernetes.io/evict"] = "true"
			},
			evictLocalStoragePods: false,
			result:                true,
		}, {
			pod: test.BuildTestPod("p12", 400, 0, n1.Name, nil),
			runBefore: func(pod *v1.Pod) {
				pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
				priority := utils.SystemCriticalPriority
				pod.Spec.Priority = &priority
			},
			evictLocalStoragePods: false,
			result:                false,
		}, {
			pod: test.BuildTestPod("p13", 400, 0, n1.Name, nil),
			runBefore: func(pod *v1.Pod) {
				pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
				priority := utils.SystemCriticalPriority
				pod.Spec.Priority = &priority
				pod.Annotations = map[string]string{
					"descheduler.alpha.kubernetes.io/evict": "true",
				}
			},
			evictLocalStoragePods: false,
			result:                true,
		}, {
			pod: test.BuildTestPod("p14", 400, 0, n1.Name, nil),
			runBefore: func(pod *v1.Pod) {
				pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
				pod.Spec.Priority = &highPriority
			},
			evictLocalStoragePods: false,
			priorityThreshold:     &lowPriority,
			result:                false,
		}, {
			pod: test.BuildTestPod("p15", 400, 0, n1.Name, nil),
			runBefore: func(pod *v1.Pod) {
				pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
				pod.Annotations = map[string]string{"descheduler.alpha.kubernetes.io/evict": "true"}
				pod.Spec.Priority = &highPriority
			},
			evictLocalStoragePods: false,
			priorityThreshold:     &lowPriority,
			result:                true,
		}, {
			pod: test.BuildTestPod("p16", 400, 0, n1.Name, nil),
			runBefore: func(pod *v1.Pod) {
				pod.ObjectMeta.OwnerReferences = test.GetStatefulSetOwnerRefList()
			},
			evictLocalStoragePods: false,
			result:                true,
		}, {
			pod: test.BuildTestPod("p17", 400, 0, n1.Name, nil),
			runBefore: func(pod *v1.Pod) {
				pod.Annotations = map[string]string{"descheduler.alpha.kubernetes.io/evict": "true"}
				pod.ObjectMeta.OwnerReferences = test.GetStatefulSetOwnerRefList()
			},
			evictLocalStoragePods: false,
			result:                true,
		},
	}

	for _, test := range testCases {
		test.runBefore(test.pod)

		podEvictor := &PodEvictor{
			evictLocalStoragePods: test.evictLocalStoragePods,
		}

		evictable := podEvictor.Evictable()
		if test.priorityThreshold != nil {
			evictable = podEvictor.Evictable(WithPriorityThreshold(*test.priorityThreshold))
		}

		result := evictable.IsEvictable(test.pod)
		if result != test.result {
			t.Errorf("IsEvictable should return for pod %s %t, but it returns %t", test.pod.Name, test.result, result)
		}

	}
}
func TestPodTypes(t *testing.T) {
	n1 := test.BuildTestNode("node1", 1000, 2000, 9, nil)
	p1 := test.BuildTestPod("p1", 400, 0, n1.Name, nil)

	// These won't be evicted.
	p2 := test.BuildTestPod("p2", 400, 0, n1.Name, nil)
	p3 := test.BuildTestPod("p3", 400, 0, n1.Name, nil)
	p4 := test.BuildTestPod("p4", 400, 0, n1.Name, nil)

	p1.ObjectMeta.OwnerReferences = test.GetReplicaSetOwnerRefList()
	// The following 4 pods won't get evicted.
	// A daemonset.
	//p2.Annotations = test.GetDaemonSetAnnotation()
	p2.ObjectMeta.OwnerReferences = test.GetDaemonSetOwnerRefList()
	// A pod with local storage.
	p3.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
	p3.Spec.Volumes = []v1.Volume{
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
	p4.Annotations = test.GetMirrorPodAnnotation()
	if !IsMirrorPod(p4) {
		t.Errorf("Expected p4 to be a mirror pod.")
	}
	if !IsPodWithLocalStorage(p3) {
		t.Errorf("Expected p3 to be a pod with local storage.")
	}
	ownerRefList := podutil.OwnerRef(p2)
	if !IsDaemonsetPod(ownerRefList) {
		t.Errorf("Expected p2 to be a daemonset pod.")
	}
	ownerRefList = podutil.OwnerRef(p1)
	if IsDaemonsetPod(ownerRefList) || IsPodWithLocalStorage(p1) || IsCriticalPod(p1) || IsMirrorPod(p1) {
		t.Errorf("Expected p1 to be a normal pod.")
	}

}
