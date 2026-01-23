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

package descheduler

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	promapi "github.com/prometheus/client_golang/api"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	v1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1"
	policyv1 "k8s.io/api/policy/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	utilversion "k8s.io/apimachinery/pkg/util/version"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/events"
	"k8s.io/client-go/util/workqueue"
	componentbaseconfig "k8s.io/component-base/config"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/cmd/descheduler/app/options"
	"sigs.k8s.io/descheduler/metrics"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/client"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	eutils "sigs.k8s.io/descheduler/pkg/descheduler/evictions/utils"
	"sigs.k8s.io/descheduler/pkg/descheduler/metricscollector"
	nodeutil "sigs.k8s.io/descheduler/pkg/descheduler/node"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/framework/pluginregistry"
	frameworkprofile "sigs.k8s.io/descheduler/pkg/framework/profile"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
	"sigs.k8s.io/descheduler/pkg/tracing"
	"sigs.k8s.io/descheduler/pkg/utils"
	"sigs.k8s.io/descheduler/pkg/version"
)

const (
	prometheusAuthTokenSecretKey = "prometheusAuthToken"
	workQueueKey                 = "key"
	indexerNodeSelectorGlobal    = "indexer_node_selector_global"
)

type eprunner func(ctx context.Context, nodes []*v1.Node) *frameworktypes.Status

type profileRunner struct {
	name                      string
	descheduleEPs, balanceEPs eprunner
}

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

type descheduler struct {
	rs                                *options.DeschedulerServer
	client                            clientset.Interface
	kubeClientSandbox                 *kubeClientSandbox
	getPodsAssignedToNode             podutil.GetPodsAssignedToNodeFunc
	sharedInformerFactory             informers.SharedInformerFactory
	namespacedSecretsLister           corev1listers.SecretNamespaceLister
	deschedulerPolicy                 *api.DeschedulerPolicy
	eventRecorder                     events.EventRecorder
	podEvictor                        *evictions.PodEvictor
	metricsCollector                  *metricscollector.MetricsCollector
	prometheusClient                  promapi.Client
	previousPrometheusClientTransport *http.Transport
	queue                             workqueue.RateLimitingInterface
	currentPrometheusAuthToken        string
	metricsProviders                  map[api.MetricsSource]*api.MetricsProvider
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

func nodeSelectorFromPolicy(deschedulerPolicy *api.DeschedulerPolicy) (labels.Selector, error) {
	nodeSelector := labels.Everything()
	if deschedulerPolicy.NodeSelector != nil {
		sel, err := labels.Parse(*deschedulerPolicy.NodeSelector)
		if err != nil {
			return nil, err
		}
		nodeSelector = sel
	}
	return nodeSelector, nil
}

func addNodeSelectorIndexer(sharedInformerFactory informers.SharedInformerFactory, nodeSelector labels.Selector) error {
	return nodeutil.AddNodeSelectorIndexer(sharedInformerFactory.Core().V1().Nodes().Informer(), indexerNodeSelectorGlobal, nodeSelector)
}

func metricsProviderListToMap(providersList []api.MetricsProvider) map[api.MetricsSource]*api.MetricsProvider {
	providersMap := make(map[api.MetricsSource]*api.MetricsProvider)
	for _, provider := range providersList {
		providersMap[provider.Source] = &provider
	}
	return providersMap
}

// setupPrometheusProvider sets up the prometheus provider on the descheduler if configured
func setupPrometheusProvider(d *descheduler, namespacedSharedInformerFactory informers.SharedInformerFactory) error {
	prometheusProvider := d.metricsProviders[api.PrometheusMetrics]
	if prometheusProvider != nil && prometheusProvider.Prometheus != nil && prometheusProvider.Prometheus.AuthToken != nil {
		authTokenSecret := prometheusProvider.Prometheus.AuthToken.SecretReference
		if authTokenSecret == nil || authTokenSecret.Namespace == "" {
			return fmt.Errorf("prometheus metrics source configuration is missing authentication token secret")
		}
		if namespacedSharedInformerFactory == nil {
			return fmt.Errorf("namespacedSharedInformerFactory not configured")
		}
		namespacedSharedInformerFactory.Core().V1().Secrets().Informer().AddEventHandler(d.eventHandler())
		d.namespacedSecretsLister = namespacedSharedInformerFactory.Core().V1().Secrets().Lister().Secrets(authTokenSecret.Namespace)
	}
	return nil
}

func newDescheduler(ctx context.Context, rs *options.DeschedulerServer, deschedulerPolicy *api.DeschedulerPolicy, evictionPolicyGroupVersion string, eventRecorder events.EventRecorder, client clientset.Interface, sharedInformerFactory informers.SharedInformerFactory, kubeClientSandbox *kubeClientSandbox) (*descheduler, error) {
	podInformer := sharedInformerFactory.Core().V1().Pods().Informer()
	// Temporarily register the PVC because it is used by the DefaultEvictor plugin during
	// the descheduling cycle, where informer registration is ignored.
	_ = sharedInformerFactory.Core().V1().PersistentVolumeClaims().Informer()

	getPodsAssignedToNode, err := podutil.BuildGetPodsAssignedToNodeFunc(podInformer)
	if err != nil {
		return nil, fmt.Errorf("build get pods assigned to node function error: %v", err)
	}

	podEvictor, err := evictions.NewPodEvictor(
		ctx,
		client,
		eventRecorder,
		podInformer,
		rs.DefaultFeatureGates,
		evictions.NewOptions().
			WithPolicyGroupVersion(evictionPolicyGroupVersion).
			WithMaxPodsToEvictPerNode(deschedulerPolicy.MaxNoOfPodsToEvictPerNode).
			WithMaxPodsToEvictPerNamespace(deschedulerPolicy.MaxNoOfPodsToEvictPerNamespace).
			WithMaxPodsToEvictTotal(deschedulerPolicy.MaxNoOfPodsToEvictTotal).
			WithEvictionFailureEventNotification(deschedulerPolicy.EvictionFailureEventNotification).
			WithGracePeriodSeconds(deschedulerPolicy.GracePeriodSeconds).
			WithDryRun(rs.DryRun).
			WithMetricsEnabled(!rs.DisableMetrics),
	)
	if err != nil {
		return nil, err
	}

	desch := &descheduler{
		rs:                    rs,
		client:                client,
		kubeClientSandbox:     kubeClientSandbox,
		getPodsAssignedToNode: getPodsAssignedToNode,
		sharedInformerFactory: sharedInformerFactory,
		deschedulerPolicy:     deschedulerPolicy,
		eventRecorder:         eventRecorder,
		podEvictor:            podEvictor,
		prometheusClient:      rs.PrometheusClient,
		queue:                 workqueue.NewRateLimitingQueueWithConfig(workqueue.DefaultControllerRateLimiter(), workqueue.RateLimitingQueueConfig{Name: "descheduler"}),
		metricsProviders:      metricsProviderListToMap(deschedulerPolicy.MetricsProviders),
	}

	nodeSelector, err := nodeSelectorFromPolicy(deschedulerPolicy)
	if err != nil {
		return nil, err
	}

	if err := addNodeSelectorIndexer(sharedInformerFactory, nodeSelector); err != nil {
		return nil, err
	}

	if rs.MetricsClient != nil {
		desch.metricsCollector = metricscollector.NewMetricsCollector(sharedInformerFactory.Core().V1().Nodes().Lister(), rs.MetricsClient, nodeSelector)
	}

	return desch, nil
}

func (d *descheduler) reconcileInClusterSAToken() error {
	// Read the sa token and assume it has the sufficient permissions to authenticate
	cfg, err := rest.InClusterConfig()
	if err == nil {
		if d.currentPrometheusAuthToken != cfg.BearerToken {
			klog.V(2).Infof("Creating Prometheus client (with SA token)")
			prometheusClient, transport, err := client.CreatePrometheusClient(d.metricsProviders[api.PrometheusMetrics].Prometheus.URL, cfg.BearerToken)
			if err != nil {
				return fmt.Errorf("unable to create a prometheus client: %v", err)
			}
			d.prometheusClient = prometheusClient
			if d.previousPrometheusClientTransport != nil {
				d.previousPrometheusClientTransport.CloseIdleConnections()
			}
			d.previousPrometheusClientTransport = transport
			d.currentPrometheusAuthToken = cfg.BearerToken
		}
		return nil
	}
	if err == rest.ErrNotInCluster {
		return nil
	}
	return fmt.Errorf("unexpected error when reading in cluster config: %v", err)
}

func (d *descheduler) runAuthenticationSecretReconciler(ctx context.Context) {
	defer utilruntime.HandleCrash()
	defer d.queue.ShutDown()

	klog.Infof("Starting authentication secret reconciler")
	defer klog.Infof("Shutting down authentication secret reconciler")

	go wait.UntilWithContext(ctx, d.runAuthenticationSecretReconcilerWorker, time.Second)

	<-ctx.Done()
}

func (d *descheduler) runAuthenticationSecretReconcilerWorker(ctx context.Context) {
	for d.processNextWorkItem(ctx) {
	}
}

func (d *descheduler) processNextWorkItem(ctx context.Context) bool {
	dsKey, quit := d.queue.Get()
	if quit {
		return false
	}
	defer d.queue.Done(dsKey)

	err := d.sync()
	if err == nil {
		d.queue.Forget(dsKey)
		return true
	}

	utilruntime.HandleError(fmt.Errorf("%v failed with : %v", dsKey, err))
	d.queue.AddRateLimited(dsKey)

	return true
}

func (d *descheduler) sync() error {
	prometheusConfig := d.metricsProviders[api.PrometheusMetrics].Prometheus
	if prometheusConfig == nil || prometheusConfig.AuthToken == nil || prometheusConfig.AuthToken.SecretReference == nil {
		return fmt.Errorf("prometheus metrics source configuration is missing authentication token secret")
	}
	ns := prometheusConfig.AuthToken.SecretReference.Namespace
	name := prometheusConfig.AuthToken.SecretReference.Name
	secretObj, err := d.namespacedSecretsLister.Get(name)
	if err != nil {
		// clear the token if the secret is not found
		if apierrors.IsNotFound(err) {
			d.currentPrometheusAuthToken = ""
			if d.previousPrometheusClientTransport != nil {
				d.previousPrometheusClientTransport.CloseIdleConnections()
			}
			d.previousPrometheusClientTransport = nil
			d.prometheusClient = nil
		}
		return fmt.Errorf("unable to get %v/%v secret", ns, name)
	}
	authToken := string(secretObj.Data[prometheusAuthTokenSecretKey])
	if authToken == "" {
		return fmt.Errorf("prometheus authentication token secret missing %q data or empty", prometheusAuthTokenSecretKey)
	}
	if d.currentPrometheusAuthToken == authToken {
		return nil
	}

	klog.V(2).Infof("authentication secret token updated, recreating prometheus client")
	prometheusClient, transport, err := client.CreatePrometheusClient(prometheusConfig.URL, authToken)
	if err != nil {
		return fmt.Errorf("unable to create a prometheus client: %v", err)
	}
	d.prometheusClient = prometheusClient
	if d.previousPrometheusClientTransport != nil {
		d.previousPrometheusClientTransport.CloseIdleConnections()
	}
	d.previousPrometheusClientTransport = transport
	d.currentPrometheusAuthToken = authToken
	return nil
}

func (d *descheduler) eventHandler() cache.ResourceEventHandler {
	return cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { d.queue.Add(workQueueKey) },
		UpdateFunc: func(old, new interface{}) { d.queue.Add(workQueueKey) },
		DeleteFunc: func(obj interface{}) { d.queue.Add(workQueueKey) },
	}
}

func (d *descheduler) runDeschedulerLoop(ctx context.Context) error {
	var span trace.Span
	ctx, span = tracing.Tracer().Start(ctx, "runDeschedulerLoop")
	defer span.End()
	defer func(loopStartDuration time.Time) {
		metrics.DeschedulerLoopDuration.With(map[string]string{}).Observe(time.Since(loopStartDuration).Seconds())
		metrics.LoopDuration.With(map[string]string{}).Observe(time.Since(loopStartDuration).Seconds())
	}(time.Now())

	klog.V(3).Infof("Resetting pod evictor counters")
	d.podEvictor.ResetCounters()

	d.runProfiles(ctx)

	if d.rs.DryRun {
		if d.kubeClientSandbox == nil {
			return fmt.Errorf("kubeClientSandbox is nil in DryRun mode")
		}
		klog.V(3).Infof("Restoring evicted pods from cache")
		if err := d.kubeClientSandbox.restoreEvictedPods(ctx); err != nil {
			klog.ErrorS(err, "Failed to restore evicted pods")
			return fmt.Errorf("failed to restore evicted pods: %w", err)
		}
		d.kubeClientSandbox.reset()
	}

	klog.V(1).InfoS("Number of evictions/requests", "totalEvicted", d.podEvictor.TotalEvicted(), "evictionRequests", d.podEvictor.TotalEvictionRequests())

	return nil
}

// runProfiles runs all the deschedule plugins of all profiles and
// later runs through all balance plugins of all profiles. (All Balance plugins should come after all Deschedule plugins)
// see https://github.com/kubernetes-sigs/descheduler/issues/979
func (d *descheduler) runProfiles(ctx context.Context) {
	var span trace.Span
	ctx, span = tracing.Tracer().Start(ctx, "runProfiles")
	defer span.End()

	nodesAsInterface, err := d.sharedInformerFactory.Core().V1().Nodes().Informer().GetIndexer().ByIndex(indexerNodeSelectorGlobal, indexerNodeSelectorGlobal)
	if err != nil {
		span.AddEvent("Failed to list nodes with global node selector", trace.WithAttributes(attribute.String("err", err.Error())))
		klog.Error(err)
		return
	}

	nodes, err := nodeutil.ReadyNodesFromInterfaces(nodesAsInterface)
	if err != nil {
		span.AddEvent("Failed to convert node as interfaces into ready nodes", trace.WithAttributes(attribute.String("err", err.Error())))
		klog.Error(err)
		return
	}

	// if len is still <= 1 error out
	if len(nodes) <= 1 {
		klog.InfoS("Skipping descheduling cycle: requires >=2 nodes", "found", len(nodes))
		return // gracefully skip this cycle instead of aborting
	}

	var profileRunners []profileRunner
	for idx, profile := range d.deschedulerPolicy.Profiles {
		currProfile, err := frameworkprofile.NewProfile(
			ctx,
			profile,
			pluginregistry.PluginRegistry,
			frameworkprofile.WithClientSet(d.client),
			frameworkprofile.WithSharedInformerFactory(d.sharedInformerFactory),
			frameworkprofile.WithPodEvictor(d.podEvictor),
			frameworkprofile.WithGetPodsAssignedToNodeFnc(d.getPodsAssignedToNode),
			frameworkprofile.WithMetricsCollector(d.metricsCollector),
			frameworkprofile.WithPrometheusClient(d.prometheusClient),
			// Generate a unique instance ID using just the index to avoid long IDs
			// when profile names are very long
			frameworkprofile.WithProfileInstanceID(fmt.Sprintf("%d", idx)),
		)
		if err != nil {
			klog.ErrorS(err, "unable to create a profile", "profile", profile.Name)
			continue
		}
		profileRunners = append(profileRunners, profileRunner{profile.Name, currProfile.RunDeschedulePlugins, currProfile.RunBalancePlugins})
	}

	for _, profileR := range profileRunners {
		// First deschedule
		status := profileR.descheduleEPs(ctx, nodes)
		if status != nil && status.Err != nil {
			span.AddEvent("failed to perform deschedule operations", trace.WithAttributes(attribute.String("err", status.Err.Error()), attribute.String("profile", profileR.name), attribute.String("operation", tracing.DescheduleOperation)))
			klog.ErrorS(status.Err, "running deschedule extension point failed with error", "profile", profileR.name)
			continue
		}
	}

	for _, profileR := range profileRunners {
		// Balance Later
		status := profileR.balanceEPs(ctx, nodes)
		if status != nil && status.Err != nil {
			span.AddEvent("failed to perform balance operations", trace.WithAttributes(attribute.String("err", status.Err.Error()), attribute.String("profile", profileR.name), attribute.String("operation", tracing.BalanceOperation)))
			klog.ErrorS(status.Err, "running balance extension point failed with error", "profile", profileR.name)
			continue
		}
	}
}

func Run(ctx context.Context, rs *options.DeschedulerServer) error {
	var span trace.Span
	ctx, span = tracing.Tracer().Start(ctx, "Run")
	defer span.End()
	metrics.Register()

	clientConnection := rs.ClientConnection
	if rs.KubeconfigFile != "" && clientConnection.Kubeconfig == "" {
		clientConnection.Kubeconfig = rs.KubeconfigFile
	}
	rsclient, eventClient, err := createClients(clientConnection)
	if err != nil {
		return err
	}
	rs.Client = rsclient
	rs.EventClient = eventClient

	deschedulerPolicy, err := LoadPolicyConfig(rs.PolicyConfigFile, rs.Client, pluginregistry.PluginRegistry)
	if err != nil {
		return err
	}
	if deschedulerPolicy == nil {
		return fmt.Errorf("deschedulerPolicy is nil")
	}

	// Add k8s compatibility warnings to logs
	if err := validateVersionCompatibility(rs.Client.Discovery(), version.Get()); err != nil {
		klog.Warning(err.Error())
	}

	evictionPolicyGroupVersion, err := eutils.SupportEviction(rs.Client)
	if err != nil || len(evictionPolicyGroupVersion) == 0 {
		return err
	}

	if (deschedulerPolicy.MetricsCollector != nil && deschedulerPolicy.MetricsCollector.Enabled) || metricsProviderListToMap(deschedulerPolicy.MetricsProviders)[api.KubernetesMetrics] != nil {
		metricsClient, err := client.CreateMetricsClient(clientConnection, "descheduler")
		if err != nil {
			return err
		}
		rs.MetricsClient = metricsClient
	}

	runFn := func() error {
		return RunDeschedulerStrategies(ctx, rs, deschedulerPolicy, evictionPolicyGroupVersion)
	}

	if rs.LeaderElection.LeaderElect && rs.DeschedulingInterval.Seconds() == 0 {
		span.AddEvent("Validation Failure", trace.WithAttributes(attribute.String("err", "leaderElection must be used with deschedulingInterval")))
		return fmt.Errorf("leaderElection must be used with deschedulingInterval")
	}

	if rs.LeaderElection.LeaderElect && rs.DryRun {
		klog.V(1).Info("Warning: DryRun is set to True. You need to disable it to use Leader Election.")
	}

	if rs.LeaderElection.LeaderElect && !rs.DryRun {
		if err := NewLeaderElection(runFn, rsclient, &rs.LeaderElection, ctx); err != nil {
			span.AddEvent("Leader Election Failure", trace.WithAttributes(attribute.String("err", err.Error())))
			return fmt.Errorf("leaderElection: %w", err)
		}
		return nil
	}

	return runFn()
}

func validateVersionCompatibility(discovery discovery.DiscoveryInterface, deschedulerVersionInfo version.Info) error {
	kubeServerVersionInfo, err := discovery.ServerVersion()
	if err != nil {
		return fmt.Errorf("failed to discover Kubernetes server version: %v", err)
	}

	kubeServerVersion, err := utilversion.ParseSemantic(kubeServerVersionInfo.String())
	if err != nil {
		return fmt.Errorf("failed to parse Kubernetes server version '%s': %v", kubeServerVersionInfo.String(), err)
	}

	deschedulerMinor, err := strconv.ParseFloat(deschedulerVersionInfo.Minor, 64)
	if err != nil {
		return fmt.Errorf("failed to convert Descheduler minor version '%s' to float: %v", deschedulerVersionInfo.Minor, err)
	}

	kubeServerMinor := float64(kubeServerVersion.Minor())
	if math.Abs(deschedulerMinor-kubeServerMinor) > 3 {
		return fmt.Errorf(
			"descheduler version %s.%s may not be supported on your version of Kubernetes %v."+
				"See compatibility docs for more info: https://github.com/kubernetes-sigs/descheduler#compatibility-matrix",
			deschedulerVersionInfo.Major,
			deschedulerVersionInfo.Minor,
			kubeServerVersionInfo.String(),
		)
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

type tokenReconciliation int

const (
	noReconciliation tokenReconciliation = iota
	inClusterReconciliation
	secretReconciliation
)

func RunDeschedulerStrategies(ctx context.Context, rs *options.DeschedulerServer, deschedulerPolicy *api.DeschedulerPolicy, evictionPolicyGroupVersion string) error {
	var span trace.Span
	ctx, span = tracing.Tracer().Start(ctx, "RunDeschedulerStrategies")
	defer span.End()

	sharedInformerFactory := informers.NewSharedInformerFactoryWithOptions(rs.Client, 0, informers.WithTransform(trimManagedFields))

	var eventClient clientset.Interface
	if rs.DryRun {
		eventClient = fakeclientset.NewSimpleClientset()
	} else {
		eventClient = rs.Client
	}
	eventBroadcaster, eventRecorder := utils.GetRecorderAndBroadcaster(ctx, eventClient)
	defer eventBroadcaster.Shutdown()

	var namespacedSharedInformerFactory informers.SharedInformerFactory
	metricProviderTokenReconciliation := noReconciliation

	prometheusProvider := metricsProviderListToMap(deschedulerPolicy.MetricsProviders)[api.PrometheusMetrics]
	if prometheusProvider != nil && prometheusProvider.Prometheus != nil && prometheusProvider.Prometheus.URL != "" {
		if prometheusProvider.Prometheus.AuthToken != nil {
			// Will get reconciled
			namespacedSharedInformerFactory = informers.NewSharedInformerFactoryWithOptions(rs.Client, 0, informers.WithTransform(trimManagedFields), informers.WithNamespace(prometheusProvider.Prometheus.AuthToken.SecretReference.Namespace))
			metricProviderTokenReconciliation = secretReconciliation
		} else {
			// Use the sa token and assume it has the sufficient permissions to authenticate
			metricProviderTokenReconciliation = inClusterReconciliation
		}
	}

	// Always create descheduler with real client/factory first to register all informers
	descheduler, err := newDescheduler(ctx, rs, deschedulerPolicy, evictionPolicyGroupVersion, eventRecorder, rs.Client, sharedInformerFactory, nil)
	if err != nil {
		span.AddEvent("Failed to create new descheduler", trace.WithAttributes(attribute.String("err", err.Error())))
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Setup Prometheus provider (only for real client case, not for dry run)
	if err := setupPrometheusProvider(descheduler, namespacedSharedInformerFactory); err != nil {
		span.AddEvent("Failed to setup Prometheus provider", trace.WithAttributes(attribute.String("err", err.Error())))
		return err
	}

	// If in dry run mode, replace the descheduler with one using fake client/factory
	if rs.DryRun {
		// Create sandbox with resources to mirror from real client
		kubeClientSandbox, err := newDefaultKubeClientSandbox(rs.Client, sharedInformerFactory)
		if err != nil {
			span.AddEvent("Failed to create kube client sandbox", trace.WithAttributes(attribute.String("err", err.Error())))
			return fmt.Errorf("failed to create kube client sandbox: %v", err)
		}

		klog.V(3).Infof("Building a cached client from the cluster for the dry run")

		// TODO(ingvagabund): drop the previous queue
		// TODO(ingvagabund): stop the previous pod evictor
		// Replace descheduler with one using fake client/factory
		descheduler, err = newDescheduler(ctx, rs, deschedulerPolicy, evictionPolicyGroupVersion, eventRecorder, kubeClientSandbox.fakeClient(), kubeClientSandbox.fakeSharedInformerFactory(), kubeClientSandbox)
		if err != nil {
			span.AddEvent("Failed to create dry run descheduler", trace.WithAttributes(attribute.String("err", err.Error())))
			return err
		}
	}

	// In dry run mode, start and sync the fake shared informer factory so it can mirror
	// events from the real factory. Reliable propagation depends on both factories being
	// fully synced (see WaitForCacheSync calls below), not solely on startup order.
	if rs.DryRun {
		descheduler.kubeClientSandbox.fakeSharedInformerFactory().Start(ctx.Done())
		descheduler.kubeClientSandbox.fakeSharedInformerFactory().WaitForCacheSync(ctx.Done())
	}
	sharedInformerFactory.Start(ctx.Done())
	if metricProviderTokenReconciliation == secretReconciliation {
		namespacedSharedInformerFactory.Start(ctx.Done())
	}

	sharedInformerFactory.WaitForCacheSync(ctx.Done())
	if metricProviderTokenReconciliation == secretReconciliation {
		namespacedSharedInformerFactory.WaitForCacheSync(ctx.Done())
	}

	descheduler.podEvictor.WaitForEventHandlersSync(ctx)

	if descheduler.metricsCollector != nil {
		go func() {
			klog.V(2).Infof("Starting metrics collector")
			descheduler.metricsCollector.Run(ctx)
			klog.V(2).Infof("Stopped metrics collector")
		}()
		klog.V(2).Infof("Waiting for metrics collector to sync")
		if err := wait.PollWithContext(ctx, time.Second, time.Minute, func(context.Context) (done bool, err error) {
			return descheduler.metricsCollector.HasSynced(), nil
		}); err != nil {
			return fmt.Errorf("unable to wait for metrics collector to sync: %v", err)
		}
	}

	if metricProviderTokenReconciliation == secretReconciliation {
		go descheduler.runAuthenticationSecretReconciler(ctx)
	}

	wait.NonSlidingUntil(func() {
		if metricProviderTokenReconciliation == inClusterReconciliation {
			// Read the sa token and assume it has the sufficient permissions to authenticate
			if err := descheduler.reconcileInClusterSAToken(); err != nil {
				klog.ErrorS(err, "unable to reconcile an in cluster SA token")
				return
			}
		}

		// A next context is created here intentionally to avoid nesting the spans via context.
		sCtx, sSpan := tracing.Tracer().Start(ctx, "NonSlidingUntil")
		defer sSpan.End()

		err = descheduler.runDeschedulerLoop(sCtx)
		if err != nil {
			sSpan.AddEvent("Failed to run descheduler loop", trace.WithAttributes(attribute.String("err", err.Error())))
			klog.Error(err)
			cancel()
			return
		}
		// If there was no interval specified, send a signal to the stopChannel to end the wait.Until loop after 1 iteration
		if rs.DeschedulingInterval.Seconds() == 0 {
			cancel()
		}
	}, rs.DeschedulingInterval, ctx.Done())

	return nil
}

func GetPluginConfig(pluginName string, pluginConfigs []api.PluginConfig) (*api.PluginConfig, int) {
	for idx, pluginConfig := range pluginConfigs {
		if pluginConfig.Name == pluginName {
			return &pluginConfig, idx
		}
	}
	return nil, 0
}

func createClients(clientConnection componentbaseconfig.ClientConnectionConfiguration) (clientset.Interface, clientset.Interface, error) {
	kClient, err := client.CreateClient(clientConnection, "descheduler")
	if err != nil {
		return nil, nil, err
	}

	eventClient, err := client.CreateClient(clientConnection, "")
	if err != nil {
		return nil, nil, err
	}

	return kClient, eventClient, nil
}

func trimManagedFields(obj interface{}) (interface{}, error) {
	if accessor, err := meta.Accessor(obj); err == nil {
		accessor.SetManagedFields(nil)
	}
	return obj, nil
}
