/*
Copyright 2026 The Kubernetes Authors.

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

package descheduler

import (
	"context"
	"testing"

	v1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	fakeclientset "k8s.io/client-go/kubernetes/fake"

	"sigs.k8s.io/descheduler/test"
)

func TestKubeClientSandboxReset(t *testing.T) {
	ctx := context.Background()
	node1 := test.BuildTestNode("n1", 2000, 3000, 10, nil)
	p1 := test.BuildTestPod("p1", 100, 0, node1.Name, test.SetRSOwnerRef)
	p2 := test.BuildTestPod("p2", 100, 0, node1.Name, test.SetRSOwnerRef)

	client := fakeclientset.NewSimpleClientset(node1, p1, p2)
	sharedInformerFactory := informers.NewSharedInformerFactoryWithOptions(client, 0)

	// Explicitly get the informers to ensure they're registered
	_ = sharedInformerFactory.Core().V1().Pods().Informer()
	_ = sharedInformerFactory.Core().V1().Nodes().Informer()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	sharedInformerFactory.Start(ctx.Done())
	sharedInformerFactory.WaitForCacheSync(ctx.Done())

	sandbox, err := newKubeClientSandbox(client, sharedInformerFactory,
		v1.SchemeGroupVersion.WithResource("pods"),
		v1.SchemeGroupVersion.WithResource("nodes"),
	)
	if err != nil {
		t.Fatalf("Failed to create kubeClientSandbox: %v", err)
	}

	eviction1 := &policy.Eviction{
		ObjectMeta: metav1.ObjectMeta{
			Name:      p1.Name,
			Namespace: p1.Namespace,
		},
	}
	eviction2 := &policy.Eviction{
		ObjectMeta: metav1.ObjectMeta{
			Name:      p2.Name,
			Namespace: p2.Namespace,
		},
	}

	sandbox.fakeSharedInformerFactory().Start(ctx.Done())
	sandbox.fakeSharedInformerFactory().WaitForCacheSync(ctx.Done())

	if err := sandbox.fakeClient().CoreV1().Pods(p1.Namespace).EvictV1(context.TODO(), eviction1); err != nil {
		t.Fatalf("Error evicting p1: %v", err)
	}
	if err := sandbox.fakeClient().CoreV1().Pods(p2.Namespace).EvictV1(context.TODO(), eviction2); err != nil {
		t.Fatalf("Error evicting p2: %v", err)
	}

	evictedPods := sandbox.evictedPodsCache.list()
	if len(evictedPods) != 2 {
		t.Fatalf("Expected 2 evicted pods in cache, but got %d", len(evictedPods))
	}
	t.Logf("Evicted pods in cache before reset: %d", len(evictedPods))

	for _, evictedPod := range evictedPods {
		if evictedPod.Namespace == "" || evictedPod.Name == "" || evictedPod.UID == "" {
			t.Errorf("Evicted pod has empty fields: namespace=%s, name=%s, uid=%s", evictedPod.Namespace, evictedPod.Name, evictedPod.UID)
		}
		t.Logf("Evicted pod: %s/%s (UID: %s)", evictedPod.Namespace, evictedPod.Name, evictedPod.UID)
	}

	sandbox.reset()

	evictedPodsAfterReset := sandbox.evictedPodsCache.list()
	if len(evictedPodsAfterReset) != 0 {
		t.Fatalf("Expected cache to be empty after reset, but found %d pods", len(evictedPodsAfterReset))
	}
	t.Logf("Successfully verified cache is empty after reset")
}

func TestEvictedPodsCache(t *testing.T) {
	t.Run("add single pod", func(t *testing.T) {
		const (
			podName      = "pod1"
			podNamespace = "default"
			podUID       = "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
		)
		cache := newEvictedPodsCache()
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      podName,
				Namespace: podNamespace,
				UID:       podUID,
			},
		}

		cache.add(pod)

		pods := cache.list()
		if len(pods) != 1 {
			t.Fatalf("Expected 1 pod in cache, got %d", len(pods))
		}
		if pods[0].Name != podName || pods[0].Namespace != podNamespace || pods[0].UID != podUID {
			t.Errorf("Pod data mismatch: got name=%s, namespace=%s, uid=%s", pods[0].Name, pods[0].Namespace, pods[0].UID)
		}
	})

	t.Run("add multiple pods", func(t *testing.T) {
		cache := newEvictedPodsCache()
		pods := []*v1.Pod{
			{ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "default", UID: "11111111-1111-1111-1111-111111111111"}},
			{ObjectMeta: metav1.ObjectMeta{Name: "pod2", Namespace: "kube-system", UID: "22222222-2222-2222-2222-222222222222"}},
			{ObjectMeta: metav1.ObjectMeta{Name: "pod3", Namespace: "default", UID: "33333333-3333-3333-3333-333333333333"}},
		}

		for _, pod := range pods {
			cache.add(pod)
		}

		cachedPods := cache.list()
		if len(cachedPods) != 3 {
			t.Fatalf("Expected 3 pods in cache, got %d", len(cachedPods))
		}

		podMap := make(map[string]*evictedPodInfo)
		for _, cachedPod := range cachedPods {
			podMap[cachedPod.UID] = cachedPod
		}

		for _, pod := range pods {
			cached, ok := podMap[string(pod.UID)]
			if !ok {
				t.Errorf("Pod with UID %s not found in cache", pod.UID)
				continue
			}
			if cached.Name != pod.Name || cached.Namespace != pod.Namespace {
				t.Errorf("Pod data mismatch for UID %s: got name=%s, namespace=%s", pod.UID, cached.Name, cached.Namespace)
			}
		}
	})

	t.Run("add duplicate pod updates entry", func(t *testing.T) {
		const (
			duplicateUID   = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
			updatedPodName = "pod1-new"
			updatedPodNS   = "kube-system"
		)
		cache := newEvictedPodsCache()
		pod1 := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod1",
				Namespace: "default",
				UID:       duplicateUID,
			},
		}
		pod2 := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      updatedPodName,
				Namespace: updatedPodNS,
				UID:       duplicateUID,
			},
		}

		cache.add(pod1)
		cache.add(pod2)

		pods := cache.list()
		if len(pods) != 1 {
			t.Fatalf("Expected 1 pod in cache (duplicates should overwrite), got %d", len(pods))
		}
		if pods[0].Name != updatedPodName || pods[0].Namespace != updatedPodNS {
			t.Errorf("Expected pod2 data, got name=%s, namespace=%s", pods[0].Name, pods[0].Namespace)
		}
	})

	t.Run("list returns empty array for empty cache", func(t *testing.T) {
		cache := newEvictedPodsCache()
		pods := cache.list()
		if pods == nil {
			t.Fatal("Expected non-nil slice from list()")
		}
		if len(pods) != 0 {
			t.Fatalf("Expected empty list, got %d pods", len(pods))
		}
	})

	t.Run("list returns copies not references", func(t *testing.T) {
		const originalPodName = "pod1"
		cache := newEvictedPodsCache()
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      originalPodName,
				Namespace: "default",
				UID:       "12345678-1234-1234-1234-123456789abc",
			},
		}
		cache.add(pod)

		pods1 := cache.list()
		pods2 := cache.list()

		if len(pods1) != 1 || len(pods2) != 1 {
			t.Fatalf("Expected 1 pod in both lists")
		}

		pods1[0].Name = "modified"

		if pods2[0].Name == "modified" {
			t.Error("Modifying list result should not affect other list results (should be copies)")
		}

		pods3 := cache.list()
		if pods3[0].Name != originalPodName {
			t.Error("Cache data was modified, list() should return copies")
		}
	})

	t.Run("clear empties the cache", func(t *testing.T) {
		cache := newEvictedPodsCache()
		cache.add(&v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "default", UID: "aaaa0000-0000-0000-0000-000000000001"}})
		cache.add(&v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod2", Namespace: "kube-system", UID: "bbbb0000-0000-0000-0000-000000000002"}})

		if len(cache.list()) != 2 {
			t.Fatal("Expected 2 pods before clear")
		}

		cache.clear()

		pods := cache.list()
		if len(pods) != 0 {
			t.Fatalf("Expected empty cache after clear, got %d pods", len(pods))
		}
	})

	t.Run("clear on empty cache is safe", func(t *testing.T) {
		cache := newEvictedPodsCache()
		cache.clear()

		pods := cache.list()
		if len(pods) != 0 {
			t.Fatalf("Expected empty cache, got %d pods", len(pods))
		}
	})

	t.Run("add after clear works correctly", func(t *testing.T) {
		cache := newEvictedPodsCache()
		cache.add(&v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "default", UID: "00000001-0001-0001-0001-000000000001"}})
		cache.clear()
		cache.add(&v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod2", Namespace: "kube-system", UID: "00000002-0002-0002-0002-000000000002"}})

		pods := cache.list()
		if len(pods) != 1 {
			t.Fatalf("Expected 1 pod after clear and add, got %d", len(pods))
		}
		if pods[0].Name != "pod2" {
			t.Errorf("Expected pod2, got %s", pods[0].Name)
		}
	})
}
