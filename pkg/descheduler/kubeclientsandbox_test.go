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
	"time"

	v1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	fakeclientset "k8s.io/client-go/kubernetes/fake"

	"sigs.k8s.io/descheduler/test"
)

// setupTestSandbox creates and initializes a kubeClientSandbox for testing.
// It creates a fake client, informer factory, registers pod and node informers,
// creates the sandbox, and starts both factories.
func setupTestSandbox(ctx context.Context, t *testing.T, initialObjects ...runtime.Object) (*kubeClientSandbox, informers.SharedInformerFactory, clientset.Interface) {
	// Create a "real" fake client to act as the source of truth
	realClient := fakeclientset.NewSimpleClientset(initialObjects...)
	realFactory := informers.NewSharedInformerFactoryWithOptions(realClient, 0)

	// Register pods and nodes informers BEFORE creating the sandbox
	_ = realFactory.Core().V1().Pods().Informer()
	_ = realFactory.Core().V1().Nodes().Informer()

	// Create the sandbox with only pods and nodes resources
	sandbox, err := newKubeClientSandbox(realClient, realFactory,
		v1.SchemeGroupVersion.WithResource("pods"),
		v1.SchemeGroupVersion.WithResource("nodes"),
	)
	if err != nil {
		t.Fatalf("Failed to create kubeClientSandbox: %v", err)
	}

	// fake factory created by newKubeClientSandbox needs to be started before
	// the "real" fake client factory to have all handlers registered
	// to get complete propagation
	sandbox.fakeSharedInformerFactory().Start(ctx.Done())
	sandbox.fakeSharedInformerFactory().WaitForCacheSync(ctx.Done())

	realFactory.Start(ctx.Done())
	realFactory.WaitForCacheSync(ctx.Done())

	return sandbox, realFactory, realClient
}

func TestKubeClientSandboxEventHandlers(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sandbox, realFactory, realClient := setupTestSandbox(ctx, t)

	// Register a third resource (secrets) in the real factory AFTER sandbox creation
	// This should NOT be synced to the fake client
	_ = realFactory.Core().V1().Secrets().Informer()

	// Create test objects
	testPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       "pod-uid-12345",
		},
		Spec: v1.PodSpec{
			NodeName: "test-node",
		},
	}

	testNode := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			UID:  "node-uid-67890",
		},
	}

	testSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "default",
			UID:       "secret-uid-abcde",
		},
		Data: map[string][]byte{
			"key": []byte("value"),
		},
	}

	// Add objects to the real client
	var err error
	_, err = realClient.CoreV1().Pods(testPod.Namespace).Create(ctx, testPod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create pod in real client: %v", err)
	}

	_, err = realClient.CoreV1().Nodes().Create(ctx, testNode, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create node in real client: %v", err)
	}

	_, err = realClient.CoreV1().Secrets(testSecret.Namespace).Create(ctx, testSecret, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create secret in real client: %v", err)
	}

	// Helper function to wait for a resource to appear in the sandbox's fake client indexer
	waitForResourceInIndexer := func(resourceType, namespace, name, uid, description string) error {
		t.Logf("Waiting for %s to appear in fake client indexer...", description)
		return wait.PollUntilContextTimeout(ctx, 100*time.Millisecond, 5*time.Second, true, func(ctx context.Context) (bool, error) {
			exists, err := sandbox.hasObjectInIndexer(
				v1.SchemeGroupVersion.WithResource(resourceType),
				namespace,
				name,
				uid,
			)
			if err != nil {
				t.Logf("Error checking %s in indexer: %v", description, err)
				return false, nil
			}
			if exists {
				t.Logf("%s appeared in fake client indexer", description)
				return true, nil
			}
			return false, nil
		})
	}

	// Wait for the pod to appear in the sandbox's fake client indexer
	if err := waitForResourceInIndexer("pods", testPod.Namespace, testPod.Name, string(testPod.UID), "pod"); err != nil {
		t.Fatalf("Pod did not appear in fake client indexer within timeout: %v", err)
	}

	// Wait for the node to appear in the sandbox's fake client indexer
	if err := waitForResourceInIndexer("nodes", "", testNode.Name, string(testNode.UID), "node"); err != nil {
		t.Fatalf("Node did not appear in fake client indexer within timeout: %v", err)
	}

	// Verify the pod can be retrieved from the fake client
	retrievedPod, err := sandbox.fakeClient().CoreV1().Pods(testPod.Namespace).Get(ctx, testPod.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to retrieve pod from fake client: %v", err)
	}
	if retrievedPod.Namespace != testPod.Namespace || retrievedPod.Name != testPod.Name || retrievedPod.UID != testPod.UID {
		t.Errorf("Retrieved pod mismatch: got namespace=%s name=%s uid=%s, want namespace=%s name=%s uid=%s",
			retrievedPod.Namespace, retrievedPod.Name, retrievedPod.UID, testPod.Namespace, testPod.Name, testPod.UID)
	}
	t.Logf("Successfully retrieved pod from fake client: %s/%s", retrievedPod.Namespace, retrievedPod.Name)

	// Verify the node can be retrieved from the fake client
	retrievedNode, err := sandbox.fakeClient().CoreV1().Nodes().Get(ctx, testNode.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to retrieve node from fake client: %v", err)
	}
	if retrievedNode.Name != testNode.Name || retrievedNode.UID != testNode.UID {
		t.Errorf("Retrieved node mismatch: got name=%s uid=%s, want name=%s uid=%s",
			retrievedNode.Name, retrievedNode.UID, testNode.Name, testNode.UID)
	}
	t.Logf("Successfully retrieved node from fake client: %s", retrievedNode.Name)

	// Wait a bit longer and verify the secret does NOT appear in the fake client indexer
	// because secrets were registered AFTER the sandbox was created
	t.Log("Verifying secret does NOT appear in fake client indexer...")
	time.Sleep(500 * time.Millisecond) // Give extra time to ensure it's not just a timing issue

	// First, verify we can get the informer for secrets in the fake factory
	secretInformer, err := sandbox.fakeSharedInformerFactory().ForResource(v1.SchemeGroupVersion.WithResource("secrets"))
	if err != nil {
		t.Logf("Expected: Cannot get secret informer from fake factory: %v", err)
	} else {
		// If we can get the informer, check if the secret exists in it
		key := "default/test-secret"
		_, exists, err := secretInformer.Informer().GetIndexer().GetByKey(key)
		if err != nil {
			t.Logf("Error checking secret in fake indexer: %v", err)
		}
		if exists {
			t.Error("Secret should NOT exist in fake client indexer (it was registered after sandbox creation)")
		} else {
			t.Log("Correctly verified: Secret does not exist in fake client indexer")
		}
	}

	// Also verify that attempting to get the secret directly from fake client should fail
	_, err = sandbox.fakeClient().CoreV1().Secrets(testSecret.Namespace).Get(ctx, testSecret.Name, metav1.GetOptions{})
	if err == nil {
		t.Error("Secret should NOT be retrievable from fake client (it was not synced)")
	} else {
		t.Logf("Correctly verified: Secret not retrievable from fake client: %v", err)
	}

	// Verify the secret IS in the real client (sanity check)
	_, err = realClient.CoreV1().Secrets(testSecret.Namespace).Get(ctx, testSecret.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Secret should exist in real client but got error: %v", err)
	}
	t.Log("Sanity check passed: Secret exists in real client")
}

func TestKubeClientSandboxReset(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	node1 := test.BuildTestNode("n1", 2000, 3000, 10, nil)
	p1 := test.BuildTestPod("p1", 100, 0, node1.Name, test.SetRSOwnerRef)
	p2 := test.BuildTestPod("p2", 100, 0, node1.Name, test.SetRSOwnerRef)

	sandbox, _, _ := setupTestSandbox(ctx, t, node1, p1, p2)

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

func TestKubeClientSandboxRestoreEvictedPods(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	node1 := test.BuildTestNode("n1", 2000, 3000, 10, nil)

	// Create test pods
	pod1 := test.BuildTestPod("pod1", 100, 0, node1.Name, test.SetRSOwnerRef)
	pod2 := test.BuildTestPod("pod2", 100, 0, node1.Name, test.SetRSOwnerRef)
	pod3 := test.BuildTestPod("pod3", 100, 0, node1.Name, test.SetRSOwnerRef)
	pod4 := test.BuildTestPod("pod4", 100, 0, node1.Name, test.SetRSOwnerRef)

	sandbox, _, realClient := setupTestSandbox(ctx, t, node1, pod1, pod2, pod3, pod4)

	// Evict all pods
	for _, pod := range []*v1.Pod{pod1, pod2, pod3, pod4} {
		eviction := &policy.Eviction{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pod.Name,
				Namespace: pod.Namespace,
			},
		}
		if err := sandbox.fakeClient().CoreV1().Pods(pod.Namespace).EvictV1(ctx, eviction); err != nil {
			t.Fatalf("Error evicting %s: %v", pod.Name, err)
		}
	}

	// Delete pod2 from real client to simulate it being deleted (should skip restoration)
	if err := realClient.CoreV1().Pods(pod2.Namespace).Delete(ctx, pod2.Name, metav1.DeleteOptions{}); err != nil {
		t.Fatalf("Error deleting pod2 from real client: %v", err)
	}

	// Delete and recreate pod3 with different UID in real client (should skip restoration)
	if err := realClient.CoreV1().Pods(pod3.Namespace).Delete(ctx, pod3.Name, metav1.DeleteOptions{}); err != nil {
		t.Fatalf("Error deleting pod3 from real client: %v", err)
	}
	pod3New := test.BuildTestPod("pod3", 100, 0, node1.Name, test.SetRSOwnerRef)
	// Ensure pod3New has a different UID from pod3
	if pod3New.UID == pod3.UID {
		pod3New.UID = "new-uid-for-pod3"
	}
	if _, err := realClient.CoreV1().Pods(pod3New.Namespace).Create(ctx, pod3New, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Error recreating pod3 in real client: %v", err)
	}

	// Verify evicted pods are in cache
	evictedPods := sandbox.evictedPodsCache.list()
	if len(evictedPods) != 4 {
		t.Fatalf("Expected 4 evicted pods in cache, got %d", len(evictedPods))
	}
	t.Logf("Evicted pods in cache: %d", len(evictedPods))

	// Call restoreEvictedPods
	t.Log("Calling restoreEvictedPods...")
	if err := sandbox.restoreEvictedPods(ctx); err != nil {
		t.Fatalf("restoreEvictedPods failed: %v", err)
	}

	// Verify pod1 and pod4 were restored (exists in fake client with matching UID and accessible via indexer)
	for _, pod := range []*v1.Pod{pod1, pod4} {
		// Check restoration via fake client
		restoredPod, err := sandbox.fakeClient().CoreV1().Pods(pod.Namespace).Get(ctx, pod.Name, metav1.GetOptions{})
		if err != nil {
			t.Errorf("%s should have been restored to fake client: %v", pod.Name, err)
		} else {
			if restoredPod.UID != pod.UID {
				t.Errorf("Restored %s UID mismatch: got %s, want %s", pod.Name, restoredPod.UID, pod.UID)
			}
			t.Logf("Successfully verified %s restoration: %s/%s (UID: %s)", pod.Name, restoredPod.Namespace, restoredPod.Name, restoredPod.UID)
		}

		// Check accessibility via indexer
		exists, err := sandbox.hasObjectInIndexer(v1.SchemeGroupVersion.WithResource("pods"), pod.Namespace, pod.Name, string(pod.UID))
		if err != nil {
			t.Errorf("Error checking %s in indexer: %v", pod.Name, err)
		}
		if !exists {
			t.Errorf("%s should exist in fake indexer after restoration", pod.Name)
		} else {
			t.Logf("Successfully verified %s exists in fake indexer", pod.Name)
		}
	}

	// Verify pod2 was NOT restored (deleted from real client)
	_, err := sandbox.fakeClient().CoreV1().Pods(pod2.Namespace).Get(ctx, pod2.Name, metav1.GetOptions{})
	if err == nil {
		t.Error("pod2 should NOT have been restored (was deleted from real client)")
	} else {
		t.Logf("Correctly verified pod2 was not restored: %v", err)
	}

	// Verify pod3 was NOT restored with old UID (UID mismatch case)
	// Note: pod3 may exist in fake client with NEW UID due to event handler syncing,
	// but it should NOT have been restored with the OLD UID from evicted cache
	pod3InFake, err := sandbox.fakeClient().CoreV1().Pods(pod3.Namespace).Get(ctx, pod3.Name, metav1.GetOptions{})
	if err == nil {
		// Pod3 exists, but it should have the NEW UID, not the old one
		if pod3InFake.UID == pod3.UID {
			t.Error("pod3 should NOT have been restored with old UID (UID mismatch should prevent restoration)")
		} else {
			t.Logf("Correctly verified pod3 has new UID (%s), not old UID (%s) - restoration was skipped", pod3InFake.UID, pod3.UID)
		}
	} else {
		// Pod3 doesn't exist - this is also acceptable (event handlers haven't synced it yet)
		t.Logf("pod3 not found in fake client: %v", err)
	}

	// Verify evicted pods cache is still intact (restoreEvictedPods doesn't clear it)
	evictedPodsAfter := sandbox.evictedPodsCache.list()
	if len(evictedPodsAfter) != 4 {
		t.Errorf("Expected evicted pods cache to still have 4 entries, got %d", len(evictedPodsAfter))
	}
}

func TestKubeClientSandboxRestoreEvictedPodsEmptyCache(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	node1 := test.BuildTestNode("n1", 2000, 3000, 10, nil)
	sandbox, _, _ := setupTestSandbox(ctx, t, node1)

	// Call restoreEvictedPods with empty cache - should be a no-op
	t.Log("Calling restoreEvictedPods with empty cache...")
	if err := sandbox.restoreEvictedPods(ctx); err != nil {
		t.Fatalf("restoreEvictedPods should succeed with empty cache: %v", err)
	}
	t.Log("Successfully verified restoreEvictedPods handles empty cache")
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
