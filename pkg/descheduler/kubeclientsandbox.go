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
	"fmt"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1"
	policyv1 "k8s.io/api/policy/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

// evictedPodInfo stores identifying information about a pod that was evicted during dry-run mode
type evictedPodInfo struct {
	Namespace string
	Name      string
	UID       string
}

// evictedPodsCache is a thread-safe cache for tracking pods evicted during dry-run mode
type evictedPodsCache struct {
	sync.RWMutex
	pods map[string]*evictedPodInfo
}

func newEvictedPodsCache() *evictedPodsCache {
	return &evictedPodsCache{
		pods: make(map[string]*evictedPodInfo),
	}
}

func (c *evictedPodsCache) add(pod *v1.Pod) {
	c.Lock()
	defer c.Unlock()
	c.pods[string(pod.UID)] = &evictedPodInfo{
		Namespace: pod.Namespace,
		Name:      pod.Name,
		UID:       string(pod.UID),
	}
}

func (c *evictedPodsCache) list() []*evictedPodInfo {
	c.RLock()
	defer c.RUnlock()
	pods := make([]*evictedPodInfo, 0, len(c.pods))
	for _, pod := range c.pods {
		podCopy := *pod
		pods = append(pods, &podCopy)
	}
	return pods
}

func (c *evictedPodsCache) clear() {
	c.Lock()
	defer c.Unlock()
	c.pods = make(map[string]*evictedPodInfo)
}

// kubeClientSandbox creates a sandbox environment with a fake client and informer factory
// that mirrors resources from a real client, useful for dry-run testing scenarios
type kubeClientSandbox struct {
	fakeKubeClient         *fakeclientset.Clientset
	fakeFactory            informers.SharedInformerFactory
	resourceToInformer     map[schema.GroupVersionResource]informers.GenericInformer
	evictedPodsCache       *evictedPodsCache
	podEvictionReactionFnc func(*fakeclientset.Clientset, *evictedPodsCache) func(action core.Action) (bool, runtime.Object, error)
}

func newDefaultKubeClientSandbox(client clientset.Interface, sharedInformerFactory informers.SharedInformerFactory) (*kubeClientSandbox, error) {
	return newKubeClientSandbox(client, sharedInformerFactory,
		v1.SchemeGroupVersion.WithResource("pods"),
		v1.SchemeGroupVersion.WithResource("nodes"),
		v1.SchemeGroupVersion.WithResource("namespaces"),
		schedulingv1.SchemeGroupVersion.WithResource("priorityclasses"),
		policyv1.SchemeGroupVersion.WithResource("poddisruptionbudgets"),
		v1.SchemeGroupVersion.WithResource("persistentvolumeclaims"),
	)
}

func newKubeClientSandbox(client clientset.Interface, sharedInformerFactory informers.SharedInformerFactory, resources ...schema.GroupVersionResource) (*kubeClientSandbox, error) {
	sandbox := &kubeClientSandbox{
		resourceToInformer:     make(map[schema.GroupVersionResource]informers.GenericInformer),
		evictedPodsCache:       newEvictedPodsCache(),
		podEvictionReactionFnc: podEvictionReactionFnc,
	}

	sandbox.fakeKubeClient = fakeclientset.NewSimpleClientset()
	// simulate a pod eviction by deleting a pod
	sandbox.fakeKubeClient.PrependReactor("create", "pods", sandbox.podEvictionReactionFnc(sandbox.fakeKubeClient, sandbox.evictedPodsCache))
	sandbox.fakeFactory = informers.NewSharedInformerFactory(sandbox.fakeKubeClient, 0)

	for _, resource := range resources {
		informer, err := sharedInformerFactory.ForResource(resource)
		if err != nil {
			return nil, err
		}
		sandbox.resourceToInformer[resource] = informer
	}

	// Register event handlers to sync changes from real client to fake client.
	// These handlers will keep the fake client in sync with ongoing changes.
	if err := sandbox.registerEventHandlers(); err != nil {
		return nil, fmt.Errorf("error registering event handlers: %w", err)
	}

	return sandbox, nil
}

func (sandbox *kubeClientSandbox) registerEventHandlers() error {
	for resource, informer := range sandbox.resourceToInformer {
		// Create a local copy to avoid closure capture issue
		resource := resource

		_, err := sandbox.fakeFactory.ForResource(resource)
		if err != nil {
			return fmt.Errorf("error getting resource %s for fake factory: %w", resource, err)
		}

		_, err = informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				runtimeObj, ok := obj.(runtime.Object)
				if !ok {
					klog.ErrorS(nil, "object is not a runtime.Object", "resource", resource)
					return
				}
				if err := sandbox.fakeKubeClient.Tracker().Add(runtimeObj); err != nil {
					if !apierrors.IsAlreadyExists(err) {
						klog.ErrorS(err, "failed to add object to fake client", "resource", resource)
					}
				}
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				runtimeObj, ok := newObj.(runtime.Object)
				if !ok {
					klog.ErrorS(nil, "object is not a runtime.Object", "resource", resource)
					return
				}
				metaObj, err := meta.Accessor(runtimeObj)
				if err != nil {
					klog.ErrorS(err, "failed to get object metadata", "resource", resource)
					return
				}
				if err := sandbox.fakeKubeClient.Tracker().Update(resource, runtimeObj, metaObj.GetNamespace()); err != nil {
					klog.ErrorS(err, "failed to update object in fake client", "resource", resource)
				}
			},
			DeleteFunc: func(obj interface{}) {
				// Handle tombstone case where the object might be wrapped
				if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
					obj = tombstone.Obj
				}

				runtimeObj, ok := obj.(runtime.Object)
				if !ok {
					klog.ErrorS(nil, "object is not a runtime.Object", "resource", resource)
					return
				}
				metaObj, err := meta.Accessor(runtimeObj)
				if err != nil {
					klog.ErrorS(err, "failed to get object metadata", "resource", resource)
					return
				}
				if err := sandbox.fakeKubeClient.Tracker().Delete(resource, metaObj.GetNamespace(), metaObj.GetName()); err != nil {
					klog.ErrorS(err, "failed to delete object from fake client", "resource", resource)
				}
			},
		})
		if err != nil {
			return fmt.Errorf("error adding event handler for resource %s: %w", resource, err)
		}
	}
	return nil
}

func (sandbox *kubeClientSandbox) fakeClient() *fakeclientset.Clientset {
	return sandbox.fakeKubeClient
}

func (sandbox *kubeClientSandbox) fakeSharedInformerFactory() informers.SharedInformerFactory {
	return sandbox.fakeFactory
}

func (sandbox *kubeClientSandbox) reset() {
	sandbox.evictedPodsCache.clear()
}

// hasObjectInIndexer checks if an object exists in the fake indexer for the specified resource
func (sandbox *kubeClientSandbox) hasObjectInIndexer(resource schema.GroupVersionResource, namespace, name string) (bool, error) {
	informer, err := sandbox.fakeFactory.ForResource(resource)
	if err != nil {
		return false, fmt.Errorf("error getting informer for resource %s: %w", resource, err)
	}

	key := cache.MetaObjectToName(&metav1.ObjectMeta{Namespace: namespace, Name: name}).String()
	_, exists, err := informer.Informer().GetIndexer().GetByKey(key)
	if err != nil {
		return false, err
	}

	return exists, nil
}

// hasRuntimeObjectInIndexer checks if a runtime.Object exists in the fake indexer by detecting its resource type
func (sandbox *kubeClientSandbox) hasRuntimeObjectInIndexer(obj runtime.Object) (bool, error) {
	// Get metadata accessor to extract namespace and name
	metaObj, err := meta.Accessor(obj)
	if err != nil {
		return false, fmt.Errorf("failed to get object metadata: %w", err)
	}

	// Get the GVK from the object using TypeMeta
	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk.Empty() {
		return false, fmt.Errorf("no GroupVersionKind found for object")
	}

	// Use the GVK to construct the GVR by pluralizing the kind
	plural, _ := meta.UnsafeGuessKindToResource(gvk)
	gvr := schema.GroupVersionResource{
		Group:    gvk.Group,
		Version:  gvk.Version,
		Resource: plural.Resource,
	}

	return sandbox.hasObjectInIndexer(gvr, metaObj.GetNamespace(), metaObj.GetName())
}

func waitForPodsCondition(ctx context.Context, pods []*evictedPodInfo, checkFn func(*evictedPodInfo) (bool, error), successMsg string) error {
	if len(pods) == 0 {
		return nil
	}

	err := wait.PollUntilContextTimeout(ctx, 100*time.Millisecond, 5*time.Second, true, func(ctx context.Context) (bool, error) {
		for _, pod := range pods {
			satisfied, err := checkFn(pod)
			if err != nil {
				return false, err
			}
			if !satisfied {
				return false, nil
			}
		}
		return true, nil
	})
	if err != nil {
		return err
	}

	klog.V(4).InfoS(successMsg)
	return nil
}

// restoreEvictedPods restores pods from the evicted pods cache back to the fake client
func (sandbox *kubeClientSandbox) restoreEvictedPods(ctx context.Context) error {
	podInformer, ok := sandbox.resourceToInformer[v1.SchemeGroupVersion.WithResource("pods")]
	if !ok {
		return fmt.Errorf("pod informer not found in resourceToInformer")
	}

	evictedPods := sandbox.evictedPodsCache.list()

	// First wait loop: Check that all evicted pods are cleared from the indexers.
	// This ensures the eviction has fully propagated through the fake informer's indexer.
	if err := waitForPodsCondition(ctx, evictedPods, func(pod *evictedPodInfo) (bool, error) {
		exists, err := sandbox.hasObjectInIndexer(v1.SchemeGroupVersion.WithResource("pods"), pod.Namespace, pod.Name)
		if err != nil {
			klog.V(4).InfoS("Error checking indexer for pod", "namespace", pod.Namespace, "name", pod.Name, "error", err)
			return false, nil
		}
		if exists {
			klog.V(4).InfoS("Pod still exists in fake indexer, waiting", "namespace", pod.Namespace, "name", pod.Name)
			return false, nil
		}
		klog.V(4).InfoS("Pod no longer in fake indexer", "namespace", pod.Namespace, "name", pod.Name)
		return true, nil
	}, "All evicted pods removed from fake indexer"); err != nil {
		return fmt.Errorf("timeout waiting for evicted pods to be removed from fake indexer: %w", err)
	}

	var restoredPods []*evictedPodInfo
	for _, evictedPodInfo := range sandbox.evictedPodsCache.list() {
		obj, err := podInformer.Lister().ByNamespace(evictedPodInfo.Namespace).Get(evictedPodInfo.Name)
		if err != nil {
			klog.V(3).InfoS("Pod not found in real client, skipping restoration", "namespace", evictedPodInfo.Namespace, "name", evictedPodInfo.Name, "error", err)
			continue
		}

		pod, ok := obj.(*v1.Pod)
		if !ok {
			klog.ErrorS(nil, "Object is not a pod", "namespace", evictedPodInfo.Namespace, "name", evictedPodInfo.Name)
			continue
		}

		if string(pod.UID) != evictedPodInfo.UID {
			klog.V(3).InfoS("Pod UID mismatch, skipping restoration", "namespace", evictedPodInfo.Namespace, "name", evictedPodInfo.Name, "expectedUID", evictedPodInfo.UID, "actualUID", string(pod.UID))
			continue
		}

		if err := sandbox.fakeKubeClient.Tracker().Add(pod); err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to restore pod %s/%s to fake client: %w", evictedPodInfo.Namespace, evictedPodInfo.Name, err)
		}
		klog.V(4).InfoS("Successfully restored pod to fake client", "namespace", evictedPodInfo.Namespace, "name", evictedPodInfo.Name, "uid", evictedPodInfo.UID)
		restoredPods = append(restoredPods, evictedPodInfo)
	}

	// Second wait loop: Make sure the evicted pods are added back to the fake client.
	// This ensures the restored pods are accessible through the fake informer's lister.
	if err := waitForPodsCondition(ctx, restoredPods, func(pod *evictedPodInfo) (bool, error) {
		podObj, err := sandbox.fakeFactory.Core().V1().Pods().Lister().Pods(pod.Namespace).Get(pod.Name)
		if err != nil {
			klog.V(4).InfoS("Pod not yet accessible in fake informer, waiting", "namespace", pod.Namespace, "name", pod.Name)
			return false, nil
		}
		klog.V(4).InfoS("Pod accessible in fake informer", "namespace", pod.Namespace, "name", pod.Name, "node", podObj.Spec.NodeName)
		return true, nil
	}, "All restored pods are accessible in fake informer"); err != nil {
		return fmt.Errorf("timeout waiting for pods to be accessible in fake informer: %w", err)
	}

	// Third wait loop: Make sure the indexers see the added pods.
	// This is important to ensure each descheduling cycle can see all the restored pods.
	// Without this wait, the next cycle might not see the restored pods in the indexer yet.
	if err := waitForPodsCondition(ctx, restoredPods, func(pod *evictedPodInfo) (bool, error) {
		exists, err := sandbox.hasObjectInIndexer(v1.SchemeGroupVersion.WithResource("pods"), pod.Namespace, pod.Name)
		if err != nil {
			klog.V(4).InfoS("Error checking indexer for restored pod", "namespace", pod.Namespace, "name", pod.Name, "error", err)
			return false, nil
		}
		if !exists {
			klog.V(4).InfoS("Restored pod not yet in fake indexer, waiting", "namespace", pod.Namespace, "name", pod.Name)
			return false, nil
		}
		klog.V(4).InfoS("Restored pod now in fake indexer", "namespace", pod.Namespace, "name", pod.Name)
		return true, nil
	}, "All restored pods are now in fake indexer"); err != nil {
		return fmt.Errorf("timeout waiting for restored pods to appear in fake indexer: %w", err)
	}

	return nil
}

func podEvictionReactionFnc(fakeClient *fakeclientset.Clientset, evictedCache *evictedPodsCache) func(action core.Action) (bool, runtime.Object, error) {
	return func(action core.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() == "eviction" {
			createAct, matched := action.(core.CreateActionImpl)
			if !matched {
				return false, nil, fmt.Errorf("unable to convert action to core.CreateActionImpl")
			}
			eviction, matched := createAct.Object.(*policy.Eviction)
			if !matched {
				return false, nil, fmt.Errorf("unable to convert action object into *policy.Eviction")
			}
			podObj, err := fakeClient.Tracker().Get(action.GetResource(), eviction.GetNamespace(), eviction.GetName())
			if err == nil {
				if pod, ok := podObj.(*v1.Pod); ok {
					evictedCache.add(pod)
				} else {
					return false, nil, fmt.Errorf("unable to convert object to *v1.Pod for %v/%v", eviction.GetNamespace(), eviction.GetName())
				}
			} else if !apierrors.IsNotFound(err) {
				return false, nil, fmt.Errorf("unable to get pod %v/%v: %v", eviction.GetNamespace(), eviction.GetName(), err)
			}
			if err := fakeClient.Tracker().Delete(action.GetResource(), eviction.GetNamespace(), eviction.GetName()); err != nil {
				return false, nil, fmt.Errorf("unable to delete pod %v/%v: %v", eviction.GetNamespace(), eviction.GetName(), err)
			}
			return true, nil, nil
		}
		// fallback to the default reactor
		return false, nil, nil
	}
}
