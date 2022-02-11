/*
Copyright 2020 The Kubernetes Authors.

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
	"context"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/test"
)

func TestPodLifeTime(t *testing.T) {
	node1 := test.BuildTestNode("n1", 2000, 3000, 10, nil)
	olderPodCreationTime := metav1.NewTime(time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC))
	newerPodCreationTime := metav1.NewTime(time.Now())

	// Setup pods, one should be evicted
	p1 := test.BuildTestPod("p1", 100, 0, node1.Name, nil)
	p1.Namespace = "dev"
	p1.ObjectMeta.CreationTimestamp = newerPodCreationTime
	p2 := test.BuildTestPod("p2", 100, 0, node1.Name, nil)
	p2.Namespace = "dev"
	p2.ObjectMeta.CreationTimestamp = olderPodCreationTime

	ownerRef1 := test.GetReplicaSetOwnerRefList()
	p1.ObjectMeta.OwnerReferences = ownerRef1
	p2.ObjectMeta.OwnerReferences = ownerRef1

	// Setup pods, zero should be evicted
	p3 := test.BuildTestPod("p3", 100, 0, node1.Name, nil)
	p3.Namespace = "dev"
	p3.ObjectMeta.CreationTimestamp = newerPodCreationTime
	p4 := test.BuildTestPod("p4", 100, 0, node1.Name, nil)
	p4.Namespace = "dev"
	p4.ObjectMeta.CreationTimestamp = newerPodCreationTime

	ownerRef2 := test.GetReplicaSetOwnerRefList()
	p3.ObjectMeta.OwnerReferences = ownerRef2
	p4.ObjectMeta.OwnerReferences = ownerRef2

	// Setup pods, one should be evicted
	p5 := test.BuildTestPod("p5", 100, 0, node1.Name, nil)
	p5.Namespace = "dev"
	p5.ObjectMeta.CreationTimestamp = newerPodCreationTime
	p6 := test.BuildTestPod("p6", 100, 0, node1.Name, nil)
	p6.Namespace = "dev"
	p6.ObjectMeta.CreationTimestamp = metav1.NewTime(time.Now().Add(time.Second * 605))

	ownerRef3 := test.GetReplicaSetOwnerRefList()
	p5.ObjectMeta.OwnerReferences = ownerRef3
	p6.ObjectMeta.OwnerReferences = ownerRef3

	// Setup pods, zero should be evicted
	p7 := test.BuildTestPod("p7", 100, 0, node1.Name, nil)
	p7.Namespace = "dev"
	p7.ObjectMeta.CreationTimestamp = newerPodCreationTime
	p8 := test.BuildTestPod("p8", 100, 0, node1.Name, nil)
	p8.Namespace = "dev"
	p8.ObjectMeta.CreationTimestamp = metav1.NewTime(time.Now().Add(time.Second * 595))

	ownerRef4 := test.GetReplicaSetOwnerRefList()
	p5.ObjectMeta.OwnerReferences = ownerRef4
	p6.ObjectMeta.OwnerReferences = ownerRef4

	// Setup two old pods with different status phases
	p9 := test.BuildTestPod("p9", 100, 0, node1.Name, nil)
	p9.Namespace = "dev"
	p9.ObjectMeta.CreationTimestamp = olderPodCreationTime
	p10 := test.BuildTestPod("p10", 100, 0, node1.Name, nil)
	p10.Namespace = "dev"
	p10.ObjectMeta.CreationTimestamp = olderPodCreationTime

	p9.Status.Phase = "Pending"
	p10.Status.Phase = "Running"
	p9.ObjectMeta.OwnerReferences = ownerRef1
	p10.ObjectMeta.OwnerReferences = ownerRef1

	p11 := test.BuildTestPod("p11", 100, 0, node1.Name, func(pod *v1.Pod) {
		pod.Spec.Volumes = []v1.Volume{
			{
				Name: "pvc", VolumeSource: v1.VolumeSource{
					PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{ClaimName: "foo"},
				},
			},
		}
		pod.Namespace = "dev"
		pod.ObjectMeta.CreationTimestamp = olderPodCreationTime
		pod.ObjectMeta.OwnerReferences = ownerRef1
	})

	// Setup two old pods with different labels
	p12 := test.BuildTestPod("p12", 100, 0, node1.Name, nil)
	p12.Namespace = "dev"
	p12.ObjectMeta.CreationTimestamp = olderPodCreationTime
	p13 := test.BuildTestPod("p13", 100, 0, node1.Name, nil)
	p13.Namespace = "dev"
	p13.ObjectMeta.CreationTimestamp = olderPodCreationTime

	p12.ObjectMeta.Labels = map[string]string{"foo": "bar"}
	p13.ObjectMeta.Labels = map[string]string{"foo": "bar1"}
	p12.ObjectMeta.OwnerReferences = ownerRef1
	p13.ObjectMeta.OwnerReferences = ownerRef1

	p14 := test.BuildTestPod("p14", 100, 0, node1.Name, nil)
	p15 := test.BuildTestPod("p15", 100, 0, node1.Name, nil)
	p14.Namespace = "dev"
	p15.Namespace = "dev"
	p14.ObjectMeta.CreationTimestamp = olderPodCreationTime
	p15.ObjectMeta.CreationTimestamp = olderPodCreationTime
	p14.ObjectMeta.OwnerReferences = ownerRef1
	p15.ObjectMeta.OwnerReferences = ownerRef1
	p14.DeletionTimestamp = &metav1.Time{}
	p15.DeletionTimestamp = &metav1.Time{}
	pv1 := test.BuildTestPV("pv1", "localFastSSD", nil)
	pv2 := test.BuildTestPV("pv2", "localFastSSD", nil)
	pvc1 := test.BuildTestPVC("pvc1", "testpvc", "pv1", "localFastSSD", nil)
	pvc2 := test.BuildTestPVC("pvc2", "testpvc", "pv2", "localFastSSD", nil)
	p16 := test.BuildTestPod("p16", 100, 0, node1.Name, func(pod *v1.Pod) {
		test.SetPodPVCVolume(pod, "test-vol", pvc1.Name)
		pod.Namespace = "testpvc"
		pod.ObjectMeta.CreationTimestamp = olderPodCreationTime
		pod.ObjectMeta.OwnerReferences = ownerRef1
	})
	p17 := test.BuildTestPod("p17", 100, 0, node1.Name, func(pod *v1.Pod) {
		test.SetPodPVCVolume(pod, "test-vol", pvc1.Name)
		test.SetPodPVCVolume(pod, "test-vol2", pvc2.Name)
		pod.Namespace = "testpvc"
		pod.ObjectMeta.CreationTimestamp = olderPodCreationTime
		pod.ObjectMeta.OwnerReferences = ownerRef1
	})

	var maxLifeTime uint = 600
	testCases := []struct {
		description             string
		strategy                api.DeschedulerStrategy
		pods                    []*v1.Pod
		nodes                   []*v1.Node
		pvs                     []*v1.PersistentVolume
		pvcs                    []*v1.PersistentVolumeClaim
		expectedEvictedPodCount uint
		ignorePvcPods           bool
		ignoreLocalPvcPods      bool
		evictLocalStoragePods   bool
	}{
		{
			description: "Two pods in the `dev` Namespace, 1 is new and 1 very is old. 1 should be evicted.",
			strategy: api.DeschedulerStrategy{
				Enabled: true,
				Params: &api.StrategyParameters{
					PodLifeTime: &api.PodLifeTime{MaxPodLifeTimeSeconds: &maxLifeTime},
				},
			},
			pods:                    []*v1.Pod{p1, p2},
			nodes:                   []*v1.Node{node1},
			expectedEvictedPodCount: 1,
		},
		{
			description: "Two pods in the `dev` Namespace, 2 are new and 0 are old. 0 should be evicted.",
			strategy: api.DeschedulerStrategy{
				Enabled: true,
				Params: &api.StrategyParameters{
					PodLifeTime: &api.PodLifeTime{MaxPodLifeTimeSeconds: &maxLifeTime},
				},
			},
			pods:                    []*v1.Pod{p3, p4},
			nodes:                   []*v1.Node{node1},
			expectedEvictedPodCount: 0,
		},
		{
			description: "Two pods in the `dev` Namespace, 1 created 605 seconds ago. 1 should be evicted.",
			strategy: api.DeschedulerStrategy{
				Enabled: true,
				Params: &api.StrategyParameters{
					PodLifeTime: &api.PodLifeTime{MaxPodLifeTimeSeconds: &maxLifeTime},
				},
			},
			pods:                    []*v1.Pod{p5, p6},
			nodes:                   []*v1.Node{node1},
			expectedEvictedPodCount: 1,
		},
		{
			description: "Two pods in the `dev` Namespace, 1 created 595 seconds ago. 0 should be evicted.",
			strategy: api.DeschedulerStrategy{
				Enabled: true,
				Params: &api.StrategyParameters{
					PodLifeTime: &api.PodLifeTime{MaxPodLifeTimeSeconds: &maxLifeTime},
				},
			},
			pods:                    []*v1.Pod{p7, p8},
			nodes:                   []*v1.Node{node1},
			expectedEvictedPodCount: 0,
		},
		{
			description: "Two old pods with different status phases. 1 should be evicted.",
			strategy: api.DeschedulerStrategy{
				Enabled: true,
				Params: &api.StrategyParameters{
					PodLifeTime: &api.PodLifeTime{
						MaxPodLifeTimeSeconds: &maxLifeTime,
						PodStatusPhases:       []string{"Pending"},
					},
				},
			},
			pods:                    []*v1.Pod{p9, p10},
			nodes:                   []*v1.Node{node1},
			expectedEvictedPodCount: 1,
		},
		{
			description: "does not evict pvc pods with ignorePvcPods set to true",
			strategy: api.DeschedulerStrategy{
				Enabled: true,
				Params: &api.StrategyParameters{
					PodLifeTime: &api.PodLifeTime{MaxPodLifeTimeSeconds: &maxLifeTime},
				},
			},
			pods:                    []*v1.Pod{p11},
			nodes:                   []*v1.Node{node1},
			expectedEvictedPodCount: 0,
			ignorePvcPods:           true,
		},
		{
			description: "evicts pvc pods with ignorePvcPods set to false (or unset)",
			strategy: api.DeschedulerStrategy{
				Enabled: true,
				Params: &api.StrategyParameters{
					PodLifeTime: &api.PodLifeTime{MaxPodLifeTimeSeconds: &maxLifeTime},
				},
			},
			pods:                    []*v1.Pod{p11},
			nodes:                   []*v1.Node{node1},
			expectedEvictedPodCount: 1,
		},
		{
			description: "No pod to evicted since all pod terminating",
			strategy: api.DeschedulerStrategy{
				Enabled: true,
				Params: &api.StrategyParameters{
					PodLifeTime: &api.PodLifeTime{MaxPodLifeTimeSeconds: &maxLifeTime},
					LabelSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"foo": "bar"},
					},
				},
			},
			pods:                    []*v1.Pod{p12, p13},
			nodes:                   []*v1.Node{node1},
			expectedEvictedPodCount: 1,
		},
		{
			description: "No pod should be evicted since pod terminating",
			strategy: api.DeschedulerStrategy{
				Enabled: true,
				Params: &api.StrategyParameters{
					PodLifeTime: &api.PodLifeTime{MaxPodLifeTimeSeconds: &maxLifeTime},
					LabelSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"foo": "bar"},
					},
				},
			},
			pods:                    []*v1.Pod{p14, p15},
			nodes:                   []*v1.Node{node1},
			expectedEvictedPodCount: 0,
		},
		{
			description: "No pod should be evicted since pod has local PVC Storage and ignoreLocalPvcPods is true",
			strategy: api.DeschedulerStrategy{
				Enabled: true,
				Params: &api.StrategyParameters{
					PodLifeTime: &api.PodLifeTime{MaxPodLifeTimeSeconds: &maxLifeTime},
				},
			},
			pods:                    []*v1.Pod{p16},
			nodes:                   []*v1.Node{node1},
			pvs:                     []*v1.PersistentVolume{pv1},
			pvcs:                    []*v1.PersistentVolumeClaim{pvc1},
			ignorePvcPods:           false,
			evictLocalStoragePods:   true,
			ignoreLocalPvcPods:      true,
			expectedEvictedPodCount: 0,
		},
		{
			description: "Pod should be evicted since pod has local PVC Storage and ignoreLocalPvcPods is false",
			strategy: api.DeschedulerStrategy{
				Enabled: true,
				Params: &api.StrategyParameters{
					PodLifeTime: &api.PodLifeTime{MaxPodLifeTimeSeconds: &maxLifeTime},
				},
			},
			pods:                    []*v1.Pod{p16},
			nodes:                   []*v1.Node{node1},
			pvs:                     []*v1.PersistentVolume{pv1},
			pvcs:                    []*v1.PersistentVolumeClaim{pvc1},
			ignorePvcPods:           false,
			ignoreLocalPvcPods:      false,
			evictLocalStoragePods:   true,
			expectedEvictedPodCount: 1,
		},
		{
			description: "Pod should be evicted since pod has local PVC Storage (2) and ignoreLocalPvcPods is false",
			strategy: api.DeschedulerStrategy{
				Enabled: true,
				Params: &api.StrategyParameters{
					PodLifeTime: &api.PodLifeTime{MaxPodLifeTimeSeconds: &maxLifeTime},
				},
			},
			pods:                    []*v1.Pod{p17},
			nodes:                   []*v1.Node{node1},
			pvs:                     []*v1.PersistentVolume{pv1, pv2},
			pvcs:                    []*v1.PersistentVolumeClaim{pvc1, pvc2},
			ignorePvcPods:           false,
			ignoreLocalPvcPods:      false,
			evictLocalStoragePods:   true,
			expectedEvictedPodCount: 1,
		},
		{
			description: "Pod should not be evicted since pod has local PVC Storage and evictLocalStoragePods is false",
			strategy: api.DeschedulerStrategy{
				Enabled: true,
				Params: &api.StrategyParameters{
					PodLifeTime: &api.PodLifeTime{MaxPodLifeTimeSeconds: &maxLifeTime},
				},
			},
			pods:                    []*v1.Pod{p16},
			nodes:                   []*v1.Node{node1},
			pvs:                     []*v1.PersistentVolume{pv1},
			pvcs:                    []*v1.PersistentVolumeClaim{pvc1},
			ignorePvcPods:           false,
			ignoreLocalPvcPods:      false,
			evictLocalStoragePods:   false,
			expectedEvictedPodCount: 0,
		},
		{
			description: "Pod should not be not evicted since pod has local PVC Storage and ignorePvcPods is true",
			strategy: api.DeschedulerStrategy{
				Enabled: true,
				Params: &api.StrategyParameters{
					PodLifeTime: &api.PodLifeTime{MaxPodLifeTimeSeconds: &maxLifeTime},
				},
			},
			pods:                    []*v1.Pod{p16},
			nodes:                   []*v1.Node{node1},
			pvs:                     []*v1.PersistentVolume{pv1},
			pvcs:                    []*v1.PersistentVolumeClaim{pvc1},
			ignorePvcPods:           true,
			ignoreLocalPvcPods:      false,
			evictLocalStoragePods:   false,
			expectedEvictedPodCount: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			var objs []runtime.Object
			for _, node := range tc.nodes {
				objs = append(objs, node)
			}
			for _, pod := range tc.pods {
				objs = append(objs, pod)
			}
			for _, pv := range tc.pvs {
				objs = append(objs, pv)
			}
			for _, pvc := range tc.pvcs {
				objs = append(objs, pvc)
			}
			klog.Info("Loading FakeClient with objects")
			fakeClient := fake.NewSimpleClientset(objs...)

			sharedInformerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
			podInformer := sharedInformerFactory.Core().V1().Pods()

			getPodsAssignedToNode, err := podutil.BuildGetPodsAssignedToNodeFunc(podInformer)
			if err != nil {
				t.Errorf("Build get pods assigned to node function error: %v", err)
			}

			sharedInformerFactory.Start(ctx.Done())
			sharedInformerFactory.WaitForCacheSync(ctx.Done())

			podEvictor := evictions.NewPodEvictor(
				fakeClient,
				policyv1.SchemeGroupVersion.String(),
				false,
				nil,
				nil,
				tc.nodes,
				tc.evictLocalStoragePods,
				false,
				tc.ignorePvcPods,
				true,
				false,
				tc.ignoreLocalPvcPods,
			)

			PodLifeTime(ctx, fakeClient, tc.strategy, tc.nodes, podEvictor, getPodsAssignedToNode)
			podsEvicted := podEvictor.TotalEvicted()
			if podsEvicted != tc.expectedEvictedPodCount {
				t.Errorf("Test error for description: %s. Expected evicted pods count %v, got %v", tc.description, tc.expectedEvictedPodCount, podsEvicted)
			}
		})
	}
}
