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

	v1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	clientcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"

	eutils "sigs.k8s.io/descheduler/pkg/descheduler/evictions/utils"
)

// nodePodEvictedCount keeps count of pods evicted on node
type nodePodEvictedCount map[*v1.Node]int

type PodEvictor struct {
	client             clientset.Interface
	policyGroupVersion string
	dryRun             bool
	maxPodsToEvict     int
	nodepodCount       nodePodEvictedCount
}

func NewPodEvictor(
	client clientset.Interface,
	policyGroupVersion string,
	dryRun bool,
	maxPodsToEvict int,
	nodes []*v1.Node,
) *PodEvictor {
	var nodePodCount = make(nodePodEvictedCount)
	for _, node := range nodes {
		// Initialize podsEvicted till now with 0.
		nodePodCount[node] = 0
	}

	return &PodEvictor{
		client:             client,
		policyGroupVersion: policyGroupVersion,
		dryRun:             dryRun,
		maxPodsToEvict:     maxPodsToEvict,
		nodepodCount:       nodePodCount,
	}
}

// NodeEvicted gives a number of pods evicted for node
func (pe *PodEvictor) NodeEvicted(node *v1.Node) int {
	return pe.nodepodCount[node]
}

// TotalEvicted gives a number of pods evicted through all nodes
func (pe *PodEvictor) TotalEvicted() int {
	var total int
	for _, count := range pe.nodepodCount {
		total += count
	}
	return total
}

// EvictPod returns non-nil error only when evicting a pod on a node is not
// possible (due to maxPodsToEvict constraint). Success is true when the pod
// is evicted on the server side.
func (pe *PodEvictor) EvictPod(ctx context.Context, pod *v1.Pod, node *v1.Node) (success bool, err error) {
	if pe.maxPodsToEvict > 0 && pe.nodepodCount[node]+1 > pe.maxPodsToEvict {
		return false, fmt.Errorf("Maximum number %v of evicted pods per %q node reached", pe.maxPodsToEvict, node.Name)
	}

	success, err = EvictPod(ctx, pe.client, pod, pe.policyGroupVersion, pe.dryRun)
	if success {
		pe.nodepodCount[node]++
		klog.V(1).Infof("Evicted pod: %#v in namespace %#v", pod.Name, pod.Namespace)
		return success, nil
	}
	// err is used only for logging purposes
	klog.Errorf("Error evicting pod: %#v in namespace %#v (%#v)", pod.Name, pod.Namespace, err)
	return false, nil
}

func EvictPod(ctx context.Context, client clientset.Interface, pod *v1.Pod, policyGroupVersion string, dryRun bool) (bool, error) {
	if dryRun {
		return true, nil
	}
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

	if err == nil {
		eventBroadcaster := record.NewBroadcaster()
		eventBroadcaster.StartLogging(klog.V(3).Infof)
		eventBroadcaster.StartRecordingToSink(&clientcorev1.EventSinkImpl{Interface: client.CoreV1().Events(pod.Namespace)})
		r := eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: "sigs.k8s.io.descheduler"})
		r.Event(pod, v1.EventTypeNormal, "Descheduled", "pod evicted by sigs.k8s.io/descheduler")
		return true, nil
	}
	if apierrors.IsTooManyRequests(err) {
		return false, fmt.Errorf("error when evicting pod (ignoring) %q: %v", pod.Name, err)
	}
	if apierrors.IsNotFound(err) {
		return false, fmt.Errorf("pod not found when evicting %q: %v", pod.Name, err)
	}
	return false, err
}
