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

	v1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/errors"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	clientcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"sigs.k8s.io/descheduler/metrics"
	nodeutil "sigs.k8s.io/descheduler/pkg/descheduler/node"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/utils"

	eutils "sigs.k8s.io/descheduler/pkg/descheduler/evictions/utils"
)

const (
	evictPodAnnotationKey = "descheduler.alpha.kubernetes.io/evict"
)

// nodePodEvictedCount keeps count of pods evicted on node
type nodePodEvictedCount map[*v1.Node]uint
type namespacePodEvictCount map[string]uint

type PodEvictor struct {
	client                     clientset.Interface
	nodes                      []*v1.Node
	policyGroupVersion         string
	dryRun                     bool
	maxPodsToEvictPerNode      *uint
	maxPodsToEvictPerNamespace *uint
	nodepodCount               nodePodEvictedCount
	namespacePodCount          namespacePodEvictCount
	evictFailedBarePods        bool
	evictLocalStoragePods      bool
	evictSystemCriticalPods    bool
	ignorePvcPods              bool
	metricsEnabled             bool
}

func NewPodEvictor(
	client clientset.Interface,
	policyGroupVersion string,
	dryRun bool,
	maxPodsToEvictPerNode *uint,
	maxPodsToEvictPerNamespace *uint,
	nodes []*v1.Node,
	evictLocalStoragePods bool,
	evictSystemCriticalPods bool,
	ignorePvcPods bool,
	evictFailedBarePods bool,
	metricsEnabled bool,
) *PodEvictor {
	var nodePodCount = make(nodePodEvictedCount)
	var namespacePodCount = make(namespacePodEvictCount)
	for _, node := range nodes {
		// Initialize podsEvicted till now with 0.
		nodePodCount[node] = 0
	}

	return &PodEvictor{
		client:                     client,
		nodes:                      nodes,
		policyGroupVersion:         policyGroupVersion,
		dryRun:                     dryRun,
		maxPodsToEvictPerNode:      maxPodsToEvictPerNode,
		maxPodsToEvictPerNamespace: maxPodsToEvictPerNamespace,
		nodepodCount:               nodePodCount,
		namespacePodCount:          namespacePodCount,
		evictLocalStoragePods:      evictLocalStoragePods,
		evictSystemCriticalPods:    evictSystemCriticalPods,
		evictFailedBarePods:        evictFailedBarePods,
		ignorePvcPods:              ignorePvcPods,
		metricsEnabled:             metricsEnabled,
	}
}

// NodeEvicted gives a number of pods evicted for node
func (pe *PodEvictor) NodeEvicted(node *v1.Node) uint {
	return pe.nodepodCount[node]
}

// TotalEvicted gives a number of pods evicted through all nodes
func (pe *PodEvictor) TotalEvicted() uint {
	var total uint
	for _, count := range pe.nodepodCount {
		total += count
	}
	return total
}

// EvictPod returns non-nil error only when evicting a pod on a node is not
// possible (due to maxPodsToEvictPerNode constraint). Success is true when the pod
// is evicted on the server side.
func (pe *PodEvictor) EvictPod(ctx context.Context, pod *v1.Pod, node *v1.Node, strategy string, reasons ...string) (bool, error) {
	reason := strategy
	if len(reasons) > 0 {
		reason += " (" + strings.Join(reasons, ", ") + ")"
	}
	if pe.maxPodsToEvictPerNode != nil && pe.nodepodCount[node]+1 > *pe.maxPodsToEvictPerNode {
		if pe.metricsEnabled {
			metrics.PodsEvicted.With(map[string]string{"result": "maximum number of pods per node reached", "strategy": strategy, "namespace": pod.Namespace, "node": node.Name}).Inc()
		}
		return false, fmt.Errorf("Maximum number %v of evicted pods per %q node reached", *pe.maxPodsToEvictPerNode, node.Name)
	}

	if pe.maxPodsToEvictPerNamespace != nil && pe.namespacePodCount[pod.Namespace]+1 > *pe.maxPodsToEvictPerNamespace {
		if pe.metricsEnabled {
			metrics.PodsEvicted.With(map[string]string{"result": "maximum number of pods per namespace reached", "strategy": strategy, "namespace": pod.Namespace, "node": node.Name}).Inc()
		}
		return false, fmt.Errorf("Maximum number %v of evicted pods per %q namespace reached", *pe.maxPodsToEvictPerNamespace, pod.Namespace)
	}

	err := evictPod(ctx, pe.client, pod, pe.policyGroupVersion)
	if err != nil {
		// err is used only for logging purposes
		klog.ErrorS(err, "Error evicting pod", "pod", klog.KObj(pod), "reason", reason)
		if pe.metricsEnabled {
			metrics.PodsEvicted.With(map[string]string{"result": "error", "strategy": strategy, "namespace": pod.Namespace, "node": node.Name}).Inc()
		}
		return false, nil
	}

	pe.nodepodCount[node]++
	pe.namespacePodCount[pod.Namespace]++
	if pe.dryRun {
		klog.V(1).InfoS("Evicted pod in dry run mode", "pod", klog.KObj(pod), "reason", reason)
	} else {
		klog.V(1).InfoS("Evicted pod", "pod", klog.KObj(pod), "reason", reason)
		eventBroadcaster := record.NewBroadcaster()
		eventBroadcaster.StartStructuredLogging(3)
		eventBroadcaster.StartRecordingToSink(&clientcorev1.EventSinkImpl{Interface: pe.client.CoreV1().Events(pod.Namespace)})
		r := eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: "sigs.k8s.io.descheduler"})
		r.Event(pod, v1.EventTypeNormal, "Descheduled", fmt.Sprintf("pod evicted by sigs.k8s.io/descheduler%s", reason))
		if pe.metricsEnabled {
			metrics.PodsEvicted.With(map[string]string{"result": "success", "strategy": strategy, "namespace": pod.Namespace, "node": node.Name}).Inc()
		}
	}
	return true, nil
}

func evictPod(ctx context.Context, client clientset.Interface, pod *v1.Pod, policyGroupVersion string) error {
	deleteOptions := &metav1.DeleteOptions{}
	// GracePeriodSeconds ?
	eviction := &policy.Eviction{
		TypeMeta: metav1.TypeMeta{
			APIVersion: policyGroupVersion,
			Kind:       eutils.EvictionKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		},
		DeleteOptions: deleteOptions,
	}
	err := client.PolicyV1beta1().Evictions(eviction.Namespace).Evict(ctx, eviction)

	if apierrors.IsTooManyRequests(err) {
		return fmt.Errorf("error when evicting pod (ignoring) %q: %v", pod.Name, err)
	}
	if apierrors.IsNotFound(err) {
		return fmt.Errorf("pod not found when evicting %q: %v", pod.Name, err)
	}
	return err
}

type Options struct {
	priority      *int32
	nodeFit       bool
	labelSelector labels.Selector
}

// WithPriorityThreshold sets a threshold for pod's priority class.
// Any pod whose priority class is lower is evictable.
func WithPriorityThreshold(priority int32) func(opts *Options) {
	return func(opts *Options) {
		var p int32 = priority
		opts.priority = &p
	}
}

// WithNodeFit sets whether or not to consider taints, node selectors,
// and pod affinity when evicting. A pod whose tolerations, node selectors,
// and affinity match a node other than the one it is currently running on
// is evictable.
func WithNodeFit(nodeFit bool) func(opts *Options) {
	return func(opts *Options) {
		opts.nodeFit = nodeFit
	}
}

// WithLabelSelector sets whether or not to apply label filtering when evicting.
// Any pod matching the label selector is considered evictable.
func WithLabelSelector(labelSelector labels.Selector) func(opts *Options) {
	return func(opts *Options) {
		opts.labelSelector = labelSelector
	}
}

type constraint func(pod *v1.Pod) error

type evictable struct {
	constraints []constraint
}

// Evictable provides an implementation of IsEvictable(IsEvictable(pod *v1.Pod) bool).
// The method accepts a list of options which allow to extend constraints
// which decides when a pod is considered evictable.
func (pe *PodEvictor) Evictable(opts ...func(opts *Options)) *evictable {
	options := &Options{}
	for _, opt := range opts {
		opt(options)
	}

	ev := &evictable{}
	if pe.evictFailedBarePods {
		ev.constraints = append(ev.constraints, func(pod *v1.Pod) error {
			ownerRefList := podutil.OwnerRef(pod)
			// Enable evictFailedBarePods to evict bare pods in failed phase
			if len(ownerRefList) == 0 && pod.Status.Phase != v1.PodFailed {
				return fmt.Errorf("pod does not have any ownerRefs and is not in failed phase")
			}
			return nil
		})
	} else {
		ev.constraints = append(ev.constraints, func(pod *v1.Pod) error {
			ownerRefList := podutil.OwnerRef(pod)
			// Moved from IsEvictable function for backward compatibility
			if len(ownerRefList) == 0 {
				return fmt.Errorf("pod does not have any ownerRefs")
			}
			return nil
		})
	}
	if !pe.evictSystemCriticalPods {
		ev.constraints = append(ev.constraints, func(pod *v1.Pod) error {
			// Moved from IsEvictable function to allow for disabling
			if utils.IsCriticalPriorityPod(pod) {
				return fmt.Errorf("pod has system critical priority")
			}
			return nil
		})

		if options.priority != nil {
			ev.constraints = append(ev.constraints, func(pod *v1.Pod) error {
				if IsPodEvictableBasedOnPriority(pod, *options.priority) {
					return nil
				}
				return fmt.Errorf("pod has higher priority than specified priority class threshold")
			})
		}
	}
	if !pe.evictLocalStoragePods {
		ev.constraints = append(ev.constraints, func(pod *v1.Pod) error {
			if utils.IsPodWithLocalStorage(pod) {
				return fmt.Errorf("pod has local storage and descheduler is not configured with evictLocalStoragePods")
			}
			return nil
		})
	}
	if pe.ignorePvcPods {
		ev.constraints = append(ev.constraints, func(pod *v1.Pod) error {
			if utils.IsPodWithPVC(pod) {
				return fmt.Errorf("pod has a PVC and descheduler is configured to ignore PVC pods")
			}
			return nil
		})
	}
	if options.nodeFit {
		ev.constraints = append(ev.constraints, func(pod *v1.Pod) error {
			if !nodeutil.PodFitsAnyOtherNode(pod, pe.nodes) {
				return fmt.Errorf("pod does not fit on any other node because of nodeSelector(s), Taint(s), or nodes marked as unschedulable")
			}
			return nil
		})
	}
	if options.labelSelector != nil && !options.labelSelector.Empty() {
		ev.constraints = append(ev.constraints, func(pod *v1.Pod) error {
			if !options.labelSelector.Matches(labels.Set(pod.Labels)) {
				return fmt.Errorf("pod labels do not match the labelSelector filter in the policy parameter")
			}
			return nil
		})
	}

	return ev
}

// IsEvictable decides when a pod is evictable
func (ev *evictable) IsEvictable(pod *v1.Pod) bool {
	checkErrs := []error{}

	ownerRefList := podutil.OwnerRef(pod)
	if utils.IsDaemonsetPod(ownerRefList) {
		checkErrs = append(checkErrs, fmt.Errorf("pod is a DaemonSet pod"))
	}

	if utils.IsMirrorPod(pod) {
		checkErrs = append(checkErrs, fmt.Errorf("pod is a mirror pod"))
	}

	if utils.IsStaticPod(pod) {
		checkErrs = append(checkErrs, fmt.Errorf("pod is a static pod"))
	}

	if utils.IsPodTerminating(pod) {
		checkErrs = append(checkErrs, fmt.Errorf("pod is terminating"))
	}

	for _, c := range ev.constraints {
		if err := c(pod); err != nil {
			checkErrs = append(checkErrs, err)
		}
	}

	if len(checkErrs) > 0 && !HaveEvictAnnotation(pod) {
		klog.V(4).InfoS("Pod lacks an eviction annotation and fails the following checks", "pod", klog.KObj(pod), "checks", errors.NewAggregate(checkErrs).Error())
		return false
	}

	return true
}

// HaveEvictAnnotation checks if the pod have evict annotation
func HaveEvictAnnotation(pod *v1.Pod) bool {
	_, found := pod.ObjectMeta.Annotations[evictPodAnnotationKey]
	return found
}

// IsPodEvictableBasedOnPriority checks if the given pod is evictable based on priority resolved from pod Spec.
func IsPodEvictableBasedOnPriority(pod *v1.Pod, priority int32) bool {
	return pod.Spec.Priority == nil || *pod.Spec.Priority < priority
}
