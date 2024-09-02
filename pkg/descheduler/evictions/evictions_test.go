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
	"fmt"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/events"
	"k8s.io/component-base/featuregate"
	"k8s.io/klog/v2"
	utilptr "k8s.io/utils/ptr"

	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/features"
	"sigs.k8s.io/descheduler/pkg/utils"
	"sigs.k8s.io/descheduler/test"
)

func initFeatureGates() featuregate.FeatureGate {
	featureGates := featuregate.NewFeatureGate()
	featureGates.Add(map[featuregate.Feature]featuregate.FeatureSpec{
		features.EvictionsInBackground: {Default: true, PreRelease: featuregate.Alpha},
	})
	return featureGates
}

func TestEvictPod(t *testing.T) {
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
		t.Run(test.description, func(t *testing.T) {
			ctx := context.Background()
			fakeClient := &fake.Clientset{}
			fakeClient.Fake.AddReactor("list", "pods", func(action core.Action) (bool, runtime.Object, error) {
				return true, &v1.PodList{Items: test.pods}, nil
			})
			sharedInformerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
			sharedInformerFactory.Start(ctx.Done())
			sharedInformerFactory.WaitForCacheSync(ctx.Done())

			eventRecorder := &events.FakeRecorder{}
			podEvictor := NewPodEvictor(
				ctx,
				fakeClient,
				eventRecorder,
				sharedInformerFactory.Core().V1().Pods().Informer(),
				initFeatureGates(),
				NewOptions(),
			)

			_, got := podEvictor.evictPod(ctx, test.pod)
			if got != test.want {
				t.Errorf("Test error for Desc: %s. Expected %v pod eviction to be %v, got %v", test.description, test.pod.Name, test.want, got)
			}
		})
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
	// p2.Annotations = test.GetDaemonSetAnnotation()
	p2.ObjectMeta.OwnerReferences = test.GetDaemonSetOwnerRefList()
	// A pod with local storage.
	p3.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
	p3.Spec.Volumes = []v1.Volume{
		{
			Name: "sample",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{Path: "somePath"},
				EmptyDir: &v1.EmptyDirVolumeSource{
					SizeLimit: resource.NewQuantity(int64(10), resource.BinarySI),
				},
			},
		},
	}
	// A Mirror Pod.
	p4.Annotations = test.GetMirrorPodAnnotation()
	if !utils.IsMirrorPod(p4) {
		t.Errorf("Expected p4 to be a mirror pod.")
	}
	if !utils.IsPodWithLocalStorage(p3) {
		t.Errorf("Expected p3 to be a pod with local storage.")
	}
	ownerRefList := podutil.OwnerRef(p2)
	if !utils.IsDaemonsetPod(ownerRefList) {
		t.Errorf("Expected p2 to be a daemonset pod.")
	}
	ownerRefList = podutil.OwnerRef(p1)
	if utils.IsDaemonsetPod(ownerRefList) || utils.IsPodWithLocalStorage(p1) || utils.IsCriticalPriorityPod(p1) || utils.IsMirrorPod(p1) || utils.IsStaticPod(p1) {
		t.Errorf("Expected p1 to be a normal pod.")
	}
}

func TestNewPodEvictor(t *testing.T) {
	ctx := context.Background()

	pod1 := test.BuildTestPod("pod", 400, 0, "node", nil)

	fakeClient := fake.NewSimpleClientset(pod1)

	sharedInformerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
	sharedInformerFactory.Start(ctx.Done())
	sharedInformerFactory.WaitForCacheSync(ctx.Done())

	eventRecorder := &events.FakeRecorder{}

	podEvictor := NewPodEvictor(
		ctx,
		fakeClient,
		eventRecorder,
		sharedInformerFactory.Core().V1().Pods().Informer(),
		initFeatureGates(),
		NewOptions().WithMaxPodsToEvictPerNode(utilptr.To[uint](1)),
	)

	stubNode := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node"}}

	// 0 evictions expected
	if evictions := podEvictor.NodeEvicted(stubNode); evictions != 0 {
		t.Errorf("Expected 0 node evictions, got %q instead", evictions)
	}
	// 0 evictions expected
	if evictions := podEvictor.TotalEvicted(); evictions != 0 {
		t.Errorf("Expected 0 total evictions, got %q instead", evictions)
	}

	if err := podEvictor.EvictPod(context.TODO(), pod1, EvictOptions{}); err != nil {
		t.Errorf("Expected a pod eviction, got an eviction error instead: %v", err)
	}

	// 1 node eviction expected
	if evictions := podEvictor.NodeEvicted(stubNode); evictions != 1 {
		t.Errorf("Expected 1 node eviction, got %q instead", evictions)
	}
	// 1 total eviction expected
	if evictions := podEvictor.TotalEvicted(); evictions != 1 {
		t.Errorf("Expected 1 total evictions, got %q instead", evictions)
	}

	err := podEvictor.EvictPod(context.TODO(), pod1, EvictOptions{})
	if err == nil {
		t.Errorf("Expected a pod eviction error, got nil instead")
	}
	switch err.(type) {
	case *EvictionNodeLimitError:
		// all good
	default:
		t.Errorf("Expected a pod eviction EvictionNodeLimitError error, got a different error instead: %v", err)
	}
}

func TestEvictionRequestsCacheCleanup(t *testing.T) {
	ctx := context.Background()
	node1 := test.BuildTestNode("n1", 2000, 3000, 10, nil)

	ownerRef1 := test.GetReplicaSetOwnerRefList()
	updatePod := func(pod *v1.Pod) {
		pod.Namespace = "dev"
		pod.ObjectMeta.OwnerReferences = ownerRef1
	}
	updatePodWithEvictionInBackground := func(pod *v1.Pod) {
		updatePod(pod)
		pod.Annotations = map[string]string{
			EvictionRequestAnnotationKey: "",
		}
	}

	p1 := test.BuildTestPod("p1", 100, 0, node1.Name, updatePodWithEvictionInBackground)
	p2 := test.BuildTestPod("p2", 100, 0, node1.Name, updatePodWithEvictionInBackground)
	p3 := test.BuildTestPod("p3", 100, 0, node1.Name, updatePod)
	p4 := test.BuildTestPod("p4", 100, 0, node1.Name, updatePod)

	client := fakeclientset.NewSimpleClientset(node1, p1, p2, p3, p4)
	sharedInformerFactory := informers.NewSharedInformerFactory(client, 0)
	_, eventRecorder := utils.GetRecorderAndBroadcaster(ctx, client)

	podEvictor := NewPodEvictor(
		ctx,
		client,
		eventRecorder,
		sharedInformerFactory.Core().V1().Pods().Informer(),
		initFeatureGates(),
		nil,
	)

	client.PrependReactor("create", "pods", func(action core.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() == "eviction" {
			createAct, matched := action.(core.CreateActionImpl)
			if !matched {
				return false, nil, fmt.Errorf("unable to convert action to core.CreateActionImpl")
			}
			if eviction, matched := createAct.Object.(*policy.Eviction); matched {
				podName := eviction.GetName()
				if podName == "p1" || podName == "p2" {
					return true, nil, &apierrors.StatusError{
						ErrStatus: metav1.Status{
							Reason:  metav1.StatusReasonTooManyRequests,
							Message: "Eviction triggered evacuation",
						},
					}
				}
				return true, nil, nil
			}
		}
		return false, nil, nil
	})

	sharedInformerFactory.Start(ctx.Done())
	sharedInformerFactory.WaitForCacheSync(ctx.Done())

	podEvictor.EvictPod(ctx, p1, EvictOptions{})
	podEvictor.EvictPod(ctx, p2, EvictOptions{})
	podEvictor.EvictPod(ctx, p3, EvictOptions{})
	podEvictor.EvictPod(ctx, p4, EvictOptions{})

	klog.Infof("2 evictions in background expected, 2 normal evictions")
	if total := podEvictor.TotalEvictionRequests(); total != 2 {
		t.Fatalf("Expected %v total eviction requests, got %v instead", 2, total)
	}
	if total := podEvictor.TotalEvicted(); total != 2 {
		t.Fatalf("Expected %v total evictions, got %v instead", 2, total)
	}

	klog.Infof("2 evictions in background assumed. Wait for few seconds and check the assumed requests timed out")
	time.Sleep(2 * time.Second)
	klog.Infof("Checking the assumed requests timed out and were deleted")
	// Set the timeout to 1s so the cleaning can be tested
	podEvictor.erCache.assumedRequestTimeout = 1
	podEvictor.erCache.cleanCache(ctx)
	if totalERs := podEvictor.TotalEvictionRequests(); totalERs > 0 {
		t.Fatalf("Expected 0 eviction requests, got %v instead", totalERs)
	}
}
