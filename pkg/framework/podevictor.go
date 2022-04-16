package framework

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	clientcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"sigs.k8s.io/descheduler/metrics"

	eutils "sigs.k8s.io/descheduler/pkg/descheduler/evictions/utils"
)

// nodePodEvictedCount keeps count of pods evicted on node
type nodePodEvictedCount map[string]uint
type namespacePodEvictCount map[string]uint

type PodEvictor struct {
	client                     clientset.Interface
	policyGroupVersion         string
	dryRun                     bool
	maxPodsToEvictPerNode      *uint
	maxPodsToEvictPerNamespace *uint
	nodepodCount               nodePodEvictedCount
	namespacePodCount          namespacePodEvictCount
	metricsEnabled             bool
}

func NewPodEvictor(
	client clientset.Interface,
	policyGroupVersion string,
	dryRun bool,
	maxPodsToEvictPerNode *uint,
	maxPodsToEvictPerNamespace *uint,
	metricsEnabled bool,
) *PodEvictor {
	return &PodEvictor{
		client:                     client,
		policyGroupVersion:         policyGroupVersion,
		dryRun:                     dryRun,
		maxPodsToEvictPerNode:      maxPodsToEvictPerNode,
		maxPodsToEvictPerNamespace: maxPodsToEvictPerNamespace,
		nodepodCount:               make(nodePodEvictedCount),
		namespacePodCount:          make(namespacePodEvictCount),
		metricsEnabled:             metricsEnabled,
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

// true when the pod is evicted on the server side.
// eviction reason can be set through the ctx's evictionReason:STRING pair
func (pe *PodEvictor) Evict(ctx context.Context, pod *v1.Pod) bool {
	// TODO(jchaloup): change the strategy metric label key to plugin?
	strategy := ""
	if ctx.Value("pluginName") != nil {
		strategy = ctx.Value("pluginName").(string)
	}
	reason := ""
	if ctx.Value("evictionReason") != nil {
		strategy = ctx.Value("evictionReason").(string)
	}
	nodeName := pod.Spec.NodeName
	if pe.maxPodsToEvictPerNode != nil && pe.nodepodCount[nodeName]+1 > *pe.maxPodsToEvictPerNode {
		if pe.metricsEnabled {
			metrics.PodsEvicted.With(map[string]string{"result": "maximum number of pods per node reached", "strategy": strategy, "namespace": pod.Namespace, "node": nodeName}).Inc()
		}
		klog.ErrorS(fmt.Errorf("Maximum number of evicted pods per node reached"), "limit", *pe.maxPodsToEvictPerNode, "node", nodeName)
		return false
	}

	if pe.maxPodsToEvictPerNamespace != nil && pe.namespacePodCount[pod.Namespace]+1 > *pe.maxPodsToEvictPerNamespace {
		if pe.metricsEnabled {
			metrics.PodsEvicted.With(map[string]string{"result": "maximum number of pods per namespace reached", "strategy": strategy, "namespace": pod.Namespace, "node": nodeName}).Inc()
		}
		klog.ErrorS(fmt.Errorf("Maximum number of evicted pods per namespace reached"), "limit", *pe.maxPodsToEvictPerNamespace, "namespace", pod.Namespace)
		return false
	}

	err := evictPod(ctx, pe.client, pod, pe.policyGroupVersion)
	if err != nil {
		// err is used only for logging purposes
		klog.ErrorS(err, "Error evicting pod", "pod", klog.KObj(pod), "reason", reason)
		if pe.metricsEnabled {
			metrics.PodsEvicted.With(map[string]string{"result": "error", "strategy": strategy, "namespace": pod.Namespace, "node": nodeName}).Inc()
		}
		return false
	}

	pe.nodepodCount[nodeName]++
	pe.namespacePodCount[pod.Namespace]++

	if pe.metricsEnabled {
		metrics.PodsEvicted.With(map[string]string{"result": "success", "strategy": strategy, "namespace": pod.Namespace, "node": nodeName}).Inc()
	}

	if pe.dryRun {
		klog.V(1).InfoS("Evicted pod in dry run mode", "pod", klog.KObj(pod), "reason", reason, "strategy", strategy, "node", nodeName)
	} else {
		klog.V(1).InfoS("Evicted pod", "pod", klog.KObj(pod), "reason", reason, "strategy", strategy, "node", nodeName)
		eventBroadcaster := record.NewBroadcaster()
		eventBroadcaster.StartStructuredLogging(3)
		eventBroadcaster.StartRecordingToSink(&clientcorev1.EventSinkImpl{Interface: pe.client.CoreV1().Events(pod.Namespace)})
		r := eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: "sigs.k8s.io.descheduler"})
		r.Event(pod, v1.EventTypeNormal, "Descheduled", fmt.Sprintf("pod evicted by sigs.k8s.io/descheduler%s", reason))
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
	err := client.PolicyV1beta1().Evictions(eviction.Namespace).Evict(ctx, eviction)

	if apierrors.IsTooManyRequests(err) {
		return fmt.Errorf("error when evicting pod (ignoring) %q: %v", pod.Name, err)
	}
	if apierrors.IsNotFound(err) {
		return fmt.Errorf("pod not found when evicting %q: %v", pod.Name, err)
	}
	return err
}
