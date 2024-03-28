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

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	v1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/events"
	"k8s.io/klog/v2"
	"sigs.k8s.io/descheduler/metrics"

	eutils "sigs.k8s.io/descheduler/pkg/descheduler/evictions/utils"
	"sigs.k8s.io/descheduler/pkg/tracing"
)

// nodePodEvictedCount keeps count of pods evicted on node
type (
	nodePodEvictedCount    map[string]uint
	namespacePodEvictCount map[string]uint
)

type PodEvictor struct {
	client                     clientset.Interface
	nodes                      []*v1.Node
	policyGroupVersion         string
	dryRun                     bool
	maxPodsToEvictPerNode      *uint
	maxPodsToEvictPerNamespace *uint
	nodepodCount               nodePodEvictedCount
	namespacePodCount          namespacePodEvictCount
	metricsEnabled             bool
	eventRecorder              events.EventRecorder
}

func NewPodEvictor(
	client clientset.Interface,
	policyGroupVersion string,
	dryRun bool,
	maxPodsToEvictPerNode *uint,
	maxPodsToEvictPerNamespace *uint,
	nodes []*v1.Node,
	metricsEnabled bool,
	eventRecorder events.EventRecorder,
) *PodEvictor {
	nodePodCount := make(nodePodEvictedCount)
	namespacePodCount := make(namespacePodEvictCount)
	for _, node := range nodes {
		// Initialize podsEvicted till now with 0.
		nodePodCount[node.Name] = 0
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
		metricsEnabled:             metricsEnabled,
		eventRecorder:              eventRecorder,
	}
}

// NodeEvicted gives a number of pods evicted for node
func (pe *PodEvictor) NodeEvicted(node *v1.Node) uint {
	return pe.nodepodCount[node.Name]
}

// TotalEvicted gives a number of pods evicted through all nodes
func (pe *PodEvictor) TotalEvicted() uint {
	var total uint
	for _, count := range pe.nodepodCount {
		total += count
	}
	return total
}

// NodeLimitExceeded checks if the number of evictions for a node was exceeded
func (pe *PodEvictor) NodeLimitExceeded(node *v1.Node) bool {
	if pe.maxPodsToEvictPerNode != nil {
		return pe.nodepodCount[node.Name] == *pe.maxPodsToEvictPerNode
	}
	return false
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
func (pe *PodEvictor) EvictPod(ctx context.Context, pod *v1.Pod, opts EvictOptions) bool {
	var span trace.Span
	ctx, span = tracing.Tracer().Start(ctx, "EvictPod", trace.WithAttributes(attribute.String("podName", pod.Name), attribute.String("podNamespace", pod.Namespace), attribute.String("reason", opts.Reason), attribute.String("operation", tracing.EvictOperation)))
	defer span.End()

	if pod.Spec.NodeName != "" {
		if pe.maxPodsToEvictPerNode != nil {
			if pe.nodepodCount[pod.Spec.NodeName]+1 == *pe.maxPodsToEvictPerNode {
				if pe.metricsEnabled {
					metrics.PodsEvicted.With(map[string]string{"result": "maximum number of pods per node reached", "strategy": opts.StrategyName, "namespace": pod.Namespace, "node": pod.Spec.NodeName, "profile": opts.ProfileName}).Inc()
					klog.Warningf("Warning maximum number of evicted pods per node will reach, limit %d, node %s", *pe.maxPodsToEvictPerNode, pod.Spec.NodeName)
				}
			} else if pe.nodepodCount[pod.Spec.NodeName]+1 > *pe.maxPodsToEvictPerNode {
				if pe.metricsEnabled {
					metrics.PodsEvicted.With(map[string]string{"result": "maximum number of pods per node reached", "strategy": opts.StrategyName, "namespace": pod.Namespace, "node": pod.Spec.NodeName, "profile": opts.ProfileName}).Inc()
				}
				span.AddEvent("Eviction Failed", trace.WithAttributes(attribute.String("node", pod.Spec.NodeName), attribute.String("err", "Maximum number of evicted pods per node reached")))
				klog.ErrorS(fmt.Errorf("maximum number of evicted pods per node reached"), "Error evicting pod", "limit", *pe.maxPodsToEvictPerNode, "node", pod.Spec.NodeName)
				return false
			}
		}
	}

	if pe.maxPodsToEvictPerNamespace != nil && pe.namespacePodCount[pod.Namespace]+1 > *pe.maxPodsToEvictPerNamespace {
		if pe.metricsEnabled {
			metrics.PodsEvicted.With(map[string]string{"result": "maximum number of pods per namespace reached", "strategy": opts.StrategyName, "namespace": pod.Namespace, "node": pod.Spec.NodeName, "profile": opts.ProfileName}).Inc()
		}
		span.AddEvent("Eviction Failed", trace.WithAttributes(attribute.String("node", pod.Spec.NodeName), attribute.String("err", "Maximum number of evicted pods per namespace reached")))
		klog.ErrorS(fmt.Errorf("maximum number of evicted pods per namespace reached"), "Error evicting pod", "limit", *pe.maxPodsToEvictPerNamespace, "namespace", pod.Namespace)
		return false
	}

	err := evictPod(ctx, pe.client, pod, pe.policyGroupVersion)
	if err != nil {
		// err is used only for logging purposes
		span.AddEvent("Eviction Failed", trace.WithAttributes(attribute.String("node", pod.Spec.NodeName), attribute.String("err", err.Error())))
		klog.ErrorS(err, "Error evicting pod", "pod", klog.KObj(pod), "reason", opts.Reason)
		if pe.metricsEnabled {
			metrics.PodsEvicted.With(map[string]string{"result": "error", "strategy": opts.StrategyName, "namespace": pod.Namespace, "node": pod.Spec.NodeName, "profile": opts.ProfileName}).Inc()
		}
		return false
	}

	if pod.Spec.NodeName != "" {
		pe.nodepodCount[pod.Spec.NodeName]++
	}
	pe.namespacePodCount[pod.Namespace]++

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
		pe.eventRecorder.Eventf(pod, nil, v1.EventTypeNormal, reason, "Descheduled", "pod evicted from %v node by sigs.k8s.io/descheduler", pod.Spec.NodeName)
	}
	return true
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
	err := client.PolicyV1().Evictions(eviction.Namespace).Evict(ctx, eviction)

	if apierrors.IsTooManyRequests(err) {
		return fmt.Errorf("error when evicting pod (ignoring) %q: %v", pod.Name, err)
	}
	if apierrors.IsNotFound(err) {
		return fmt.Errorf("pod not found when evicting %q: %v", pod.Name, err)
	}
	return err
}
