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
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	v1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/events"
	"k8s.io/component-base/featuregate"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/metrics"
	eutils "sigs.k8s.io/descheduler/pkg/descheduler/evictions/utils"
	"sigs.k8s.io/descheduler/pkg/features"
	"sigs.k8s.io/descheduler/pkg/tracing"
)

var (
	assumedEvictionRequestTimeoutSeconds uint          = 10 * 60 // 10 minutes
	evictionRequestsCacheResyncPeriod    time.Duration = 10 * time.Minute
	// syncedPollPeriod controls how often you look at the status of your sync funcs
	syncedPollPeriod = 100 * time.Millisecond
)

type evictionRequestItem struct {
	podName, podNamespace, podNodeName string
	evictionAssumed                    bool
	assumedTimestamp                   metav1.Time
}

type evictionRequestsCache struct {
	mu                           sync.RWMutex
	requests                     map[string]evictionRequestItem
	requestsPerNode              map[string]uint
	requestsPerNamespace         map[string]uint
	requestsTotal                uint
	assumedRequestTimeoutSeconds uint
}

func newEvictionRequestsCache(assumedRequestTimeoutSeconds uint) *evictionRequestsCache {
	return &evictionRequestsCache{
		requests:                     make(map[string]evictionRequestItem),
		requestsPerNode:              make(map[string]uint),
		requestsPerNamespace:         make(map[string]uint),
		assumedRequestTimeoutSeconds: assumedRequestTimeoutSeconds,
	}
}

func (erc *evictionRequestsCache) run(ctx context.Context) {
	wait.UntilWithContext(ctx, erc.cleanCache, evictionRequestsCacheResyncPeriod)
}

// cleanCache removes all assumed entries that has not been confirmed
// for more than a specified timeout
func (erc *evictionRequestsCache) cleanCache(ctx context.Context) {
	erc.mu.Lock()
	defer erc.mu.Unlock()
	klog.V(4).Infof("Cleaning cache of assumed eviction requests in background")
	for uid, item := range erc.requests {
		if item.evictionAssumed {
			requestAgeSeconds := uint(metav1.Now().Sub(item.assumedTimestamp.Local()).Seconds())
			if requestAgeSeconds > erc.assumedRequestTimeoutSeconds {
				klog.V(4).InfoS("Assumed eviction request in background timed out, deleting", "timeout", erc.assumedRequestTimeoutSeconds, "podNamespace", item.podNamespace, "podName", item.podName)
				erc.deleteItem(uid)
			}
		}
	}
}

func (erc *evictionRequestsCache) evictionRequestsPerNode(nodeName string) uint {
	erc.mu.RLock()
	defer erc.mu.RUnlock()
	return erc.requestsPerNode[nodeName]
}

func (erc *evictionRequestsCache) evictionRequestsPerNamespace(ns string) uint {
	erc.mu.RLock()
	defer erc.mu.RUnlock()
	return erc.requestsPerNamespace[ns]
}

func (erc *evictionRequestsCache) evictionRequestsTotal() uint {
	erc.mu.RLock()
	defer erc.mu.RUnlock()
	return erc.requestsTotal
}

func (erc *evictionRequestsCache) TotalEvictionRequests() uint {
	erc.mu.RLock()
	defer erc.mu.RUnlock()
	return uint(len(erc.requests))
}

// getPodKey returns the string key of a pod.
func getPodKey(pod *v1.Pod) string {
	uid := string(pod.UID)
	// Every pod is expected to have the UID set.
	// When the descheduling framework is used for simulation
	// user created workload may forget to set the UID.
	if len(uid) == 0 {
		panic(fmt.Errorf("cannot get cache key for %v/%v pod with empty UID", pod.Namespace, pod.Name))
	}
	return uid
}

func (erc *evictionRequestsCache) addPod(pod *v1.Pod) {
	erc.mu.Lock()
	defer erc.mu.Unlock()
	uid := getPodKey(pod)
	if _, exists := erc.requests[uid]; exists {
		return
	}
	erc.requests[uid] = evictionRequestItem{podNamespace: pod.Namespace, podName: pod.Name, podNodeName: pod.Spec.NodeName}
	erc.requestsPerNode[pod.Spec.NodeName]++
	erc.requestsPerNamespace[pod.Namespace]++
	erc.requestsTotal++
}

func (erc *evictionRequestsCache) assumePod(pod *v1.Pod) {
	erc.mu.Lock()
	defer erc.mu.Unlock()
	uid := getPodKey(pod)
	if _, exists := erc.requests[uid]; exists {
		return
	}
	erc.requests[uid] = evictionRequestItem{
		podNamespace:     pod.Namespace,
		podName:          pod.Name,
		podNodeName:      pod.Spec.NodeName,
		evictionAssumed:  true,
		assumedTimestamp: metav1.NewTime(time.Now()),
	}
	erc.requestsPerNode[pod.Spec.NodeName]++
	erc.requestsPerNamespace[pod.Namespace]++
	erc.requestsTotal++
}

// no locking, expected to be invoked from protected methods only
func (erc *evictionRequestsCache) deleteItem(uid string) {
	erc.requestsPerNode[erc.requests[uid].podNodeName]--
	if erc.requestsPerNode[erc.requests[uid].podNodeName] == 0 {
		delete(erc.requestsPerNode, erc.requests[uid].podNodeName)
	}
	erc.requestsPerNamespace[erc.requests[uid].podNamespace]--
	if erc.requestsPerNamespace[erc.requests[uid].podNamespace] == 0 {
		delete(erc.requestsPerNamespace, erc.requests[uid].podNamespace)
	}
	erc.requestsTotal--
	delete(erc.requests, uid)
}

func (erc *evictionRequestsCache) deletePod(pod *v1.Pod) {
	erc.mu.Lock()
	defer erc.mu.Unlock()
	uid := getPodKey(pod)
	if _, exists := erc.requests[uid]; exists {
		erc.deleteItem(uid)
	}
}

func (erc *evictionRequestsCache) hasPod(pod *v1.Pod) bool {
	erc.mu.RLock()
	defer erc.mu.RUnlock()
	uid := getPodKey(pod)
	_, exists := erc.requests[uid]
	return exists
}

var (
	EvictionRequestAnnotationKey    = "descheduler.alpha.kubernetes.io/request-evict-only"
	EvictionInProgressAnnotationKey = "descheduler.alpha.kubernetes.io/eviction-in-progress"
	EvictionInBackgroundErrorText   = "Eviction triggered evacuation"
)

// nodePodEvictedCount keeps count of pods evicted on node
type (
	nodePodEvictedCount    map[string]uint
	namespacePodEvictCount map[string]uint
)

type PodEvictor struct {
	mu                               sync.RWMutex
	client                           clientset.Interface
	policyGroupVersion               string
	dryRun                           bool
	evictionFailureEventNotification bool
	maxPodsToEvictPerNode            *uint
	maxPodsToEvictPerNamespace       *uint
	maxPodsToEvictTotal              *uint
	gracePeriodSeconds               *int64
	nodePodCount                     nodePodEvictedCount
	namespacePodCount                namespacePodEvictCount
	totalPodCount                    uint
	metricsEnabled                   bool
	eventRecorder                    events.EventRecorder
	erCache                          *evictionRequestsCache
	featureGates                     featuregate.FeatureGate

	// registeredHandlers contains the registrations of all handlers. It's used to check if all handlers have finished syncing before the scheduling cycles start.
	registeredHandlers []cache.ResourceEventHandlerRegistration
}

func NewPodEvictor(
	ctx context.Context,
	client clientset.Interface,
	eventRecorder events.EventRecorder,
	podInformer cache.SharedIndexInformer,
	featureGates featuregate.FeatureGate,
	options *Options,
) (*PodEvictor, error) {
	if options == nil {
		options = NewOptions()
	}

	podEvictor := &PodEvictor{
		client:                           client,
		eventRecorder:                    eventRecorder,
		policyGroupVersion:               options.policyGroupVersion,
		dryRun:                           options.dryRun,
		evictionFailureEventNotification: options.evictionFailureEventNotification,
		maxPodsToEvictPerNode:            options.maxPodsToEvictPerNode,
		maxPodsToEvictPerNamespace:       options.maxPodsToEvictPerNamespace,
		maxPodsToEvictTotal:              options.maxPodsToEvictTotal,
		gracePeriodSeconds:               options.gracePeriodSeconds,
		metricsEnabled:                   options.metricsEnabled,
		nodePodCount:                     make(nodePodEvictedCount),
		namespacePodCount:                make(namespacePodEvictCount),
		featureGates:                     featureGates,
	}

	if featureGates.Enabled(features.EvictionsInBackground) {
		erCache := newEvictionRequestsCache(assumedEvictionRequestTimeoutSeconds)

		handlerRegistration, err := podInformer.AddEventHandler(
			cache.ResourceEventHandlerFuncs{
				AddFunc: func(obj interface{}) {
					pod, ok := obj.(*v1.Pod)
					if !ok {
						klog.ErrorS(nil, "Cannot convert to *v1.Pod", "obj", obj)
						return
					}
					if _, exists := pod.Annotations[EvictionRequestAnnotationKey]; exists {
						if _, exists := pod.Annotations[EvictionInProgressAnnotationKey]; exists {
							// Ignore completed/suceeeded or failed pods
							if pod.Status.Phase != v1.PodSucceeded && pod.Status.Phase != v1.PodFailed {
								klog.V(3).InfoS("Eviction in background detected. Adding pod to the cache.", "pod", klog.KObj(pod))
								erCache.addPod(pod)
							}
						}
					}
				},
				UpdateFunc: func(oldObj, newObj interface{}) {
					oldPod, ok := oldObj.(*v1.Pod)
					if !ok {
						klog.ErrorS(nil, "Cannot convert oldObj to *v1.Pod", "oldObj", oldObj)
						return
					}
					newPod, ok := newObj.(*v1.Pod)
					if !ok {
						klog.ErrorS(nil, "Cannot convert newObj to *v1.Pod", "newObj", newObj)
						return
					}
					// Ignore pod's that are not subject to an eviction in background
					if _, exists := newPod.Annotations[EvictionRequestAnnotationKey]; !exists {
						if erCache.hasPod(newPod) {
							klog.V(3).InfoS("Pod with eviction in background lost annotation. Removing pod from the cache.", "pod", klog.KObj(newPod))
						}
						erCache.deletePod(newPod)
						return
					}
					// Remove completed/suceeeded or failed pods from the cache
					if newPod.Status.Phase == v1.PodSucceeded || newPod.Status.Phase == v1.PodFailed {
						klog.V(3).InfoS("Pod with eviction in background completed. Removing pod from the cache.", "pod", klog.KObj(newPod))
						erCache.deletePod(newPod)
						return
					}
					// Ignore any pod that does not have eviction in progress
					if _, exists := newPod.Annotations[EvictionInProgressAnnotationKey]; !exists {
						// In case EvictionInProgressAnnotationKey annotation is not present/removed
						// it's unclear whether the eviction was restarted or terminated.
						// If the eviction gets restarted the pod needs to be removed from the cache
						// to allow re-triggering the eviction.
						if _, exists := oldPod.Annotations[EvictionInProgressAnnotationKey]; !exists {
							return
						}
						// the annotation was removed -> remove the pod from the cache to allow to
						// request for eviction again. In case the eviction got restarted requesting
						// the eviction again is expected to be a no-op. In case the eviction
						// got terminated with no-retry, requesting a new eviction is a normal
						// operation.
						klog.V(3).InfoS("Eviction in background canceled (annotation removed). Removing pod from the cache.", "annotation", EvictionInProgressAnnotationKey, "pod", klog.KObj(newPod))
						erCache.deletePod(newPod)
						return
					}
					// Pick up the eviction in progress
					if !erCache.hasPod(newPod) {
						klog.V(3).InfoS("Eviction in background detected. Updating the cache.", "pod", klog.KObj(newPod))
					}
					erCache.addPod(newPod)
				},
				DeleteFunc: func(obj interface{}) {
					var pod *v1.Pod
					switch t := obj.(type) {
					case *v1.Pod:
						pod = t
					case cache.DeletedFinalStateUnknown:
						var ok bool
						pod, ok = t.Obj.(*v1.Pod)
						if !ok {
							klog.ErrorS(nil, "Cannot convert to *v1.Pod", "obj", t.Obj)
							return
						}
					default:
						klog.ErrorS(nil, "Cannot convert to *v1.Pod", "obj", t)
						return
					}
					if erCache.hasPod(pod) {
						klog.V(3).InfoS("Pod with eviction in background deleted/evicted. Removing pod from the cache.", "pod", klog.KObj(pod))
					}
					erCache.deletePod(pod)
				},
			},
		)
		if err != nil {
			return nil, fmt.Errorf("unable to register event handler for pod evictor: %v", err)
		}

		podEvictor.registeredHandlers = append(podEvictor.registeredHandlers, handlerRegistration)

		go erCache.run(ctx)

		podEvictor.erCache = erCache
	}

	return podEvictor, nil
}

// WaitForEventHandlersSync waits for EventHandlers to sync.
// It returns true if it was successful, false if the controller should shut down
func (pe *PodEvictor) WaitForEventHandlersSync(ctx context.Context) error {
	return wait.PollUntilContextCancel(ctx, syncedPollPeriod, true, func(ctx context.Context) (done bool, err error) {
		for _, handler := range pe.registeredHandlers {
			if !handler.HasSynced() {
				return false, nil
			}
		}
		return true, nil
	})
}

// NodeEvicted gives a number of pods evicted for node
func (pe *PodEvictor) NodeEvicted(node *v1.Node) uint {
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	return pe.nodePodCount[node.Name]
}

// TotalEvicted gives a number of pods evicted through all nodes
func (pe *PodEvictor) TotalEvicted() uint {
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	return pe.totalPodCount
}

func (pe *PodEvictor) ResetCounters() {
	pe.mu.Lock()
	defer pe.mu.Unlock()
	pe.nodePodCount = make(nodePodEvictedCount)
	pe.namespacePodCount = make(namespacePodEvictCount)
	pe.totalPodCount = 0
}

func (pe *PodEvictor) SetClient(client clientset.Interface) {
	pe.mu.Lock()
	defer pe.mu.Unlock()
	pe.client = client
}

func (pe *PodEvictor) evictionRequestsTotal() uint {
	if pe.featureGates.Enabled(features.EvictionsInBackground) {
		return pe.erCache.evictionRequestsTotal()
	} else {
		return 0
	}
}

func (pe *PodEvictor) evictionRequestsPerNode(node string) uint {
	if pe.featureGates.Enabled(features.EvictionsInBackground) {
		return pe.erCache.evictionRequestsPerNode(node)
	} else {
		return 0
	}
}

func (pe *PodEvictor) evictionRequestsPerNamespace(ns string) uint {
	if pe.featureGates.Enabled(features.EvictionsInBackground) {
		return pe.erCache.evictionRequestsPerNamespace(ns)
	} else {
		return 0
	}
}

func (pe *PodEvictor) EvictionRequests(node *v1.Node) uint {
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	return pe.evictionRequestsTotal()
}

func (pe *PodEvictor) TotalEvictionRequests() uint {
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	if pe.featureGates.Enabled(features.EvictionsInBackground) {
		return pe.erCache.TotalEvictionRequests()
	} else {
		return 0
	}
}

// EvictOptions provides a handle for passing additional info to EvictPod
type EvictOptions struct {
	// Reason allows for passing details about the specific eviction for logging.
	Reason string
	// ProfileName allows for passing details about profile for observability.
	ProfileName string
	// StrategyName allows for passing details about strategy for observability.
	StrategyName string
}

// EvictPod evicts a pod while exercising eviction limits.
// Returns true when the pod is evicted on the server side.
func (pe *PodEvictor) EvictPod(ctx context.Context, pod *v1.Pod, opts EvictOptions) error {
	if len(pod.UID) == 0 {
		klog.InfoS("Ignoring pod eviction due to missing UID", "pod", pod)
		return fmt.Errorf("Pod %v is missing UID", klog.KObj(pod))
	}

	if pe.featureGates.Enabled(features.EvictionsInBackground) {
		// eviction in background requested
		if _, exists := pod.Annotations[EvictionRequestAnnotationKey]; exists {
			if pe.erCache.hasPod(pod) {
				klog.V(3).InfoS("Eviction in background already requested (ignoring)", "pod", klog.KObj(pod))
				return nil
			}
		}
	}

	pe.mu.Lock()
	defer pe.mu.Unlock()

	var span trace.Span
	ctx, span = tracing.Tracer().Start(ctx, "EvictPod", trace.WithAttributes(attribute.String("podName", pod.Name), attribute.String("podNamespace", pod.Namespace), attribute.String("reason", opts.Reason), attribute.String("operation", tracing.EvictOperation)))
	defer span.End()

	if pe.maxPodsToEvictTotal != nil && pe.totalPodCount+pe.evictionRequestsTotal()+1 > *pe.maxPodsToEvictTotal {
		err := NewEvictionTotalLimitError()
		if pe.metricsEnabled {
			metrics.PodsEvicted.With(map[string]string{"result": err.Error(), "strategy": opts.StrategyName, "namespace": pod.Namespace, "node": pod.Spec.NodeName, "profile": opts.ProfileName}).Inc()
		}
		span.AddEvent("Eviction Failed", trace.WithAttributes(attribute.String("node", pod.Spec.NodeName), attribute.String("err", err.Error())))
		klog.ErrorS(err, "Error evicting pod", "limit", *pe.maxPodsToEvictTotal)
		if pe.evictionFailureEventNotification {
			pe.eventRecorder.Eventf(pod, nil, v1.EventTypeWarning, "EvictionFailed", "Descheduled", "pod eviction from %v node by sigs.k8s.io/descheduler failed: total eviction limit exceeded (%v)", pod.Spec.NodeName, *pe.maxPodsToEvictTotal)
		}
		return err
	}

	if pod.Spec.NodeName != "" {
		if pe.maxPodsToEvictPerNode != nil && pe.nodePodCount[pod.Spec.NodeName]+pe.evictionRequestsPerNode(pod.Spec.NodeName)+1 > *pe.maxPodsToEvictPerNode {
			err := NewEvictionNodeLimitError(pod.Spec.NodeName)
			if pe.metricsEnabled {
				metrics.PodsEvicted.With(map[string]string{"result": err.Error(), "strategy": opts.StrategyName, "namespace": pod.Namespace, "node": pod.Spec.NodeName, "profile": opts.ProfileName}).Inc()
			}
			span.AddEvent("Eviction Failed", trace.WithAttributes(attribute.String("node", pod.Spec.NodeName), attribute.String("err", err.Error())))
			klog.ErrorS(err, "Error evicting pod", "limit", *pe.maxPodsToEvictPerNode, "node", pod.Spec.NodeName)
			if pe.evictionFailureEventNotification {
				pe.eventRecorder.Eventf(pod, nil, v1.EventTypeWarning, "EvictionFailed", "Descheduled", "pod eviction from %v node by sigs.k8s.io/descheduler failed: node eviction limit exceeded (%v)", pod.Spec.NodeName, *pe.maxPodsToEvictPerNode)
			}
			return err
		}
	}

	if pe.maxPodsToEvictPerNamespace != nil && pe.namespacePodCount[pod.Namespace]+pe.evictionRequestsPerNamespace(pod.Namespace)+1 > *pe.maxPodsToEvictPerNamespace {
		err := NewEvictionNamespaceLimitError(pod.Namespace)
		if pe.metricsEnabled {
			metrics.PodsEvicted.With(map[string]string{"result": err.Error(), "strategy": opts.StrategyName, "namespace": pod.Namespace, "node": pod.Spec.NodeName, "profile": opts.ProfileName}).Inc()
		}
		span.AddEvent("Eviction Failed", trace.WithAttributes(attribute.String("node", pod.Spec.NodeName), attribute.String("err", err.Error())))
		klog.ErrorS(err, "Error evicting pod", "limit", *pe.maxPodsToEvictPerNamespace, "namespace", pod.Namespace, "pod", klog.KObj(pod))
		if pe.evictionFailureEventNotification {
			pe.eventRecorder.Eventf(pod, nil, v1.EventTypeWarning, "EvictionFailed", "Descheduled", "pod eviction from %v node by sigs.k8s.io/descheduler failed: namespace eviction limit exceeded (%v)", pod.Spec.NodeName, *pe.maxPodsToEvictPerNamespace)
		}
		return err
	}

	ignore, err := pe.evictPod(ctx, pod)
	if err != nil {
		// err is used only for logging purposes
		span.AddEvent("Eviction Failed", trace.WithAttributes(attribute.String("node", pod.Spec.NodeName), attribute.String("err", err.Error())))
		klog.ErrorS(err, "Error evicting pod", "pod", klog.KObj(pod), "reason", opts.Reason)
		if pe.metricsEnabled {
			metrics.PodsEvicted.With(map[string]string{"result": "error", "strategy": opts.StrategyName, "namespace": pod.Namespace, "node": pod.Spec.NodeName, "profile": opts.ProfileName}).Inc()
		}
		if pe.evictionFailureEventNotification {
			pe.eventRecorder.Eventf(pod, nil, v1.EventTypeWarning, "EvictionFailed", "Descheduled", "pod eviction from %v node by sigs.k8s.io/descheduler failed: %v", pod.Spec.NodeName, err.Error())
		}
		return err
	}

	if ignore {
		return nil
	}

	if pod.Spec.NodeName != "" {
		pe.nodePodCount[pod.Spec.NodeName]++
	}
	pe.namespacePodCount[pod.Namespace]++
	pe.totalPodCount++

	if pe.metricsEnabled {
		metrics.PodsEvicted.With(map[string]string{"result": "success", "strategy": opts.StrategyName, "namespace": pod.Namespace, "node": pod.Spec.NodeName, "profile": opts.ProfileName}).Inc()
	}

	if pe.dryRun {
		klog.V(1).InfoS("Evicted pod in dry run mode", "pod", klog.KObj(pod), "reason", opts.Reason, "strategy", opts.StrategyName, "node", pod.Spec.NodeName, "profile", opts.ProfileName)
	} else {
		klog.V(1).InfoS("Evicted pod", "pod", klog.KObj(pod), "reason", opts.Reason, "strategy", opts.StrategyName, "node", pod.Spec.NodeName, "profile", opts.ProfileName)
		reason := opts.Reason
		if len(reason) == 0 {
			reason = opts.StrategyName
			if len(reason) == 0 {
				reason = "NotSet"
			}
		}
		pe.eventRecorder.Eventf(pod, nil, v1.EventTypeNormal, reason, "Descheduled", "pod eviction from %v node by sigs.k8s.io/descheduler", pod.Spec.NodeName)
	}
	return nil
}

// return (ignore, err)
func (pe *PodEvictor) evictPod(ctx context.Context, pod *v1.Pod) (bool, error) {
	deleteOptions := &metav1.DeleteOptions{}

	// Honor the pod's TerminationGracePeriodSeconds firstly.
	// If the gracePeriodSeconds of pod is not set, then set our value.
	if pod.Spec.TerminationGracePeriodSeconds == nil {
		deleteOptions.GracePeriodSeconds = pe.gracePeriodSeconds
	}

	// GracePeriodSeconds ?
	eviction := &policy.Eviction{
		TypeMeta: metav1.TypeMeta{
			APIVersion: pe.policyGroupVersion,
			Kind:       eutils.EvictionKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		},
		DeleteOptions: deleteOptions,
	}
	err := pe.client.PolicyV1().Evictions(eviction.Namespace).Evict(ctx, eviction)
	if err == nil {
		return false, nil
	}
	if pe.featureGates.Enabled(features.EvictionsInBackground) {
		// eviction in background requested
		if _, exists := pod.Annotations[EvictionRequestAnnotationKey]; exists {
			// Simulating https://github.com/kubevirt/kubevirt/pull/11532/files#diff-059cc1fc09e8b469143348cc3aa80b40de987670e008fa18a6fe010061f973c9R77
			if apierrors.IsTooManyRequests(err) && strings.Contains(err.Error(), EvictionInBackgroundErrorText) {
				// Ignore eviction of any pod that's failed or completed.
				// It can happen an eviction in background ends up with the pod stuck in the completed state.
				// Normally, any request eviction is expected to end with the pod deletion.
				// However, some custom eviction policies may end up with completed pods around.
				// Which leads to all the completed pods to be considered still as unfinished evictions in background.
				if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
					klog.V(3).InfoS("Ignoring eviction of a completed/failed pod", "pod", klog.KObj(pod))
					return true, nil
				}
				klog.V(3).InfoS("Eviction in background assumed", "pod", klog.KObj(pod))
				pe.erCache.assumePod(pod)
				return true, nil
			}
		}
	}

	if apierrors.IsTooManyRequests(err) {
		return false, fmt.Errorf("error when evicting pod (ignoring) %q: %v", pod.Name, err)
	}
	if apierrors.IsNotFound(err) {
		return false, fmt.Errorf("pod not found when evicting %q: %v", pod.Name, err)
	}
	return false, err
}
