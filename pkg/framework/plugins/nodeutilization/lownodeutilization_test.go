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

package nodeutilization

import (
	"context"
	"fmt"
	"testing"

	v1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/metrics/pkg/apis/metrics/v1beta1"
	fakemetricsclient "k8s.io/metrics/pkg/client/clientset/versioned/fake"
	"k8s.io/utils/ptr"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	"sigs.k8s.io/descheduler/pkg/descheduler/metricscollector"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	frameworktesting "sigs.k8s.io/descheduler/pkg/framework/testing"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
	"sigs.k8s.io/descheduler/pkg/utils"
	"sigs.k8s.io/descheduler/test"

	"github.com/prometheus/common/model"
)

func TestLowNodeUtilization(t *testing.T) {
	n1NodeName := "n1"
	n2NodeName := "n2"
	n3NodeName := "n3"

	nodeSelectorKey := "datacenter"
	nodeSelectorValue := "west"
	notMatchingNodeSelectorValue := "east"

	testCases := []struct {
		name                           string
		useDeviationThresholds         bool
		thresholds, targetThresholds   api.ResourceThresholds
		nodes                          []*v1.Node
		pods                           []*v1.Pod
		nodemetricses                  []*v1beta1.NodeMetrics
		podmetricses                   []*v1beta1.PodMetrics
		expectedPodsEvicted            uint
		expectedPodsWithMetricsEvicted uint
		evictedPods                    []string
		evictableNamespaces            *api.Namespaces
		evictionLimits                 *api.EvictionLimits
	}{
		{
			name: "no evictable pods",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:  30,
				v1.ResourcePods: 30,
			},
			targetThresholds: api.ResourceThresholds{
				v1.ResourceCPU:  50,
				v1.ResourcePods: 50,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 3000, 9, nil),
				test.BuildTestNode(n2NodeName, 4000, 3000, 10, nil),
				test.BuildTestNode(n3NodeName, 4000, 3000, 10, test.SetNodeUnschedulable),
			},
			pods: []*v1.Pod{
				// These won't be evicted.
				test.BuildTestPod("p1", 400, 0, n1NodeName, test.SetDSOwnerRef),
				test.BuildTestPod("p2", 400, 0, n1NodeName, test.SetDSOwnerRef),
				test.BuildTestPod("p3", 400, 0, n1NodeName, test.SetDSOwnerRef),
				test.BuildTestPod("p4", 400, 0, n1NodeName, test.SetDSOwnerRef),
				test.BuildTestPod("p5", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A pod with local storage.
					test.SetNormalOwnerRef(pod)
					pod.Spec.Volumes = []v1.Volume{
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
					pod.Annotations = test.GetMirrorPodAnnotation()
				}),
				test.BuildTestPod("p6", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A Critical Pod.
					pod.Namespace = "kube-system"
					priority := utils.SystemCriticalPriority
					pod.Spec.Priority = &priority
				}),
				test.BuildTestPod("p9", 400, 0, n2NodeName, test.SetRSOwnerRef),
			},
			nodemetricses: []*v1beta1.NodeMetrics{
				test.BuildNodeMetrics(n1NodeName, 2401, 1714978816),
				test.BuildNodeMetrics(n2NodeName, 401, 1714978816),
				test.BuildNodeMetrics(n3NodeName, 10, 1714978816),
			},
			podmetricses: []*v1beta1.PodMetrics{
				test.BuildPodMetrics("p1", 401, 0),
				test.BuildPodMetrics("p2", 401, 0),
				test.BuildPodMetrics("p3", 401, 0),
				test.BuildPodMetrics("p4", 401, 0),
				test.BuildPodMetrics("p5", 401, 0),
			},
			expectedPodsEvicted:            0,
			expectedPodsWithMetricsEvicted: 0,
		},
		{
			name: "without priorities",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:  30,
				v1.ResourcePods: 30,
			},
			targetThresholds: api.ResourceThresholds{
				v1.ResourceCPU:  50,
				v1.ResourcePods: 50,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 3000, 9, nil),
				test.BuildTestNode(n2NodeName, 4000, 3000, 10, nil),
				test.BuildTestNode(n3NodeName, 4000, 3000, 10, test.SetNodeUnschedulable),
			},
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p2", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p3", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p4", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p5", 400, 0, n1NodeName, test.SetRSOwnerRef),
				// These won't be evicted.
				test.BuildTestPod("p6", 400, 0, n1NodeName, test.SetDSOwnerRef),
				test.BuildTestPod("p7", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A pod with local storage.
					test.SetNormalOwnerRef(pod)
					pod.Spec.Volumes = []v1.Volume{
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
					pod.Annotations = test.GetMirrorPodAnnotation()
				}),
				test.BuildTestPod("p8", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A Critical Pod.
					pod.Namespace = "kube-system"
					priority := utils.SystemCriticalPriority
					pod.Spec.Priority = &priority
				}),
				test.BuildTestPod("p9", 400, 0, n2NodeName, test.SetRSOwnerRef),
			},
			nodemetricses: []*v1beta1.NodeMetrics{
				test.BuildNodeMetrics(n1NodeName, 3201, 0),
				test.BuildNodeMetrics(n2NodeName, 401, 0),
				test.BuildNodeMetrics(n3NodeName, 11, 0),
			},
			podmetricses: []*v1beta1.PodMetrics{
				test.BuildPodMetrics("p1", 401, 0),
				test.BuildPodMetrics("p2", 401, 0),
				test.BuildPodMetrics("p3", 401, 0),
				test.BuildPodMetrics("p4", 401, 0),
				test.BuildPodMetrics("p5", 401, 0),
			},
			expectedPodsEvicted:            4,
			expectedPodsWithMetricsEvicted: 4,
		},
		{
			name: "without priorities, but excluding namespaces",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:  30,
				v1.ResourcePods: 30,
			},
			targetThresholds: api.ResourceThresholds{
				v1.ResourceCPU:  50,
				v1.ResourcePods: 50,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 3000, 9, nil),
				test.BuildTestNode(n2NodeName, 4000, 3000, 10, nil),
				test.BuildTestNode(n3NodeName, 4000, 3000, 10, test.SetNodeUnschedulable),
			},
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetNormalOwnerRef(pod)
					pod.Namespace = "namespace1"
				}),
				test.BuildTestPod("p2", 400, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetNormalOwnerRef(pod)
					pod.Namespace = "namespace1"
				}),
				test.BuildTestPod("p3", 400, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetNormalOwnerRef(pod)
					pod.Namespace = "namespace1"
				}),
				test.BuildTestPod("p4", 400, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetNormalOwnerRef(pod)
					pod.Namespace = "namespace1"
				}),
				test.BuildTestPod("p5", 400, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetNormalOwnerRef(pod)
					pod.Namespace = "namespace1"
				}),
				// These won't be evicted.
				test.BuildTestPod("p6", 400, 0, n1NodeName, test.SetDSOwnerRef),
				test.BuildTestPod("p7", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A pod with local storage.
					test.SetNormalOwnerRef(pod)
					pod.Spec.Volumes = []v1.Volume{
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
					pod.Annotations = test.GetMirrorPodAnnotation()
				}),
				test.BuildTestPod("p8", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A Critical Pod.
					pod.Namespace = "kube-system"
					priority := utils.SystemCriticalPriority
					pod.Spec.Priority = &priority
				}),
				test.BuildTestPod("p9", 400, 0, n2NodeName, test.SetRSOwnerRef),
			},
			nodemetricses: []*v1beta1.NodeMetrics{
				test.BuildNodeMetrics(n1NodeName, 3201, 0),
				test.BuildNodeMetrics(n2NodeName, 401, 0),
				test.BuildNodeMetrics(n3NodeName, 11, 0),
			},
			podmetricses: []*v1beta1.PodMetrics{
				test.BuildPodMetrics("p1", 401, 0),
				test.BuildPodMetrics("p2", 401, 0),
				test.BuildPodMetrics("p3", 401, 0),
				test.BuildPodMetrics("p4", 401, 0),
				test.BuildPodMetrics("p5", 401, 0),
			},
			evictableNamespaces: &api.Namespaces{
				Exclude: []string{
					"namespace1",
				},
			},
			expectedPodsEvicted:            0,
			expectedPodsWithMetricsEvicted: 0,
		},
		{
			name: "without priorities, but include only default namespace",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:  30,
				v1.ResourcePods: 30,
			},
			targetThresholds: api.ResourceThresholds{
				v1.ResourceCPU:  50,
				v1.ResourcePods: 50,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 3000, 9, nil),
				test.BuildTestNode(n2NodeName, 4000, 3000, 10, nil),
				test.BuildTestNode(n3NodeName, 4000, 3000, 10, test.SetNodeUnschedulable),
			},
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p2", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p3", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// TODO(zhifei92): add ownerRef for pod
					pod.Namespace = "namespace3"
				}),
				test.BuildTestPod("p4", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// TODO(zhifei92): add ownerRef for pod
					pod.Namespace = "namespace4"
				}),
				test.BuildTestPod("p5", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// TODO(zhifei92): add ownerRef for pod
					pod.Namespace = "namespace5"
				}),
				// These won't be evicted.
				test.BuildTestPod("p6", 400, 0, n1NodeName, test.SetDSOwnerRef),
				test.BuildTestPod("p7", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A pod with local storage.
					test.SetNormalOwnerRef(pod)
					pod.Spec.Volumes = []v1.Volume{
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
					pod.Annotations = test.GetMirrorPodAnnotation()
				}),
				test.BuildTestPod("p8", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A Critical Pod.
					test.SetNormalOwnerRef(pod)
					pod.Namespace = "kube-system"
					priority := utils.SystemCriticalPriority
					pod.Spec.Priority = &priority
				}),
				test.BuildTestPod("p9", 400, 0, n2NodeName, test.SetRSOwnerRef),
			},
			nodemetricses: []*v1beta1.NodeMetrics{
				test.BuildNodeMetrics(n1NodeName, 3201, 0),
				test.BuildNodeMetrics(n2NodeName, 401, 0),
				test.BuildNodeMetrics(n3NodeName, 11, 0),
			},
			podmetricses: []*v1beta1.PodMetrics{
				test.BuildPodMetrics("p1", 401, 0),
				test.BuildPodMetrics("p2", 401, 0),
				test.BuildPodMetrics("p3", 401, 0),
				test.BuildPodMetrics("p4", 401, 0),
				test.BuildPodMetrics("p5", 401, 0),
			},
			evictableNamespaces: &api.Namespaces{
				Include: []string{
					"default",
				},
			},
			expectedPodsEvicted:            2,
			expectedPodsWithMetricsEvicted: 2,
		},
		{
			name: "without priorities stop when cpu capacity is depleted",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:  30,
				v1.ResourcePods: 30,
			},
			targetThresholds: api.ResourceThresholds{
				v1.ResourceCPU:  50,
				v1.ResourcePods: 50,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 3000, 9, nil),
				test.BuildTestNode(n2NodeName, 4000, 3000, 10, nil),
				test.BuildTestNode(n3NodeName, 4000, 3000, 10, test.SetNodeUnschedulable),
			},
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p2", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p3", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p4", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p5", 400, 0, n1NodeName, test.SetRSOwnerRef),
				// These won't be evicted.
				test.BuildTestPod("p6", 400, 0, n1NodeName, test.SetDSOwnerRef),
				test.BuildTestPod("p7", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A pod with local storage.
					test.SetNormalOwnerRef(pod)
					pod.Spec.Volumes = []v1.Volume{
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
					pod.Annotations = test.GetMirrorPodAnnotation()
				}),
				test.BuildTestPod("p8", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A Critical Pod.
					test.SetNormalOwnerRef(pod)
					pod.Namespace = "kube-system"
					priority := utils.SystemCriticalPriority
					pod.Spec.Priority = &priority
				}),
				test.BuildTestPod("p9", 400, 0, n2NodeName, test.SetRSOwnerRef),
			},
			nodemetricses: []*v1beta1.NodeMetrics{
				test.BuildNodeMetrics(n1NodeName, 3201, 0),
				test.BuildNodeMetrics(n2NodeName, 401, 0),
				test.BuildNodeMetrics(n3NodeName, 0, 0),
			},
			podmetricses: []*v1beta1.PodMetrics{
				test.BuildPodMetrics("p1", 401, 0),
				test.BuildPodMetrics("p2", 401, 0),
				test.BuildPodMetrics("p3", 401, 0),
				test.BuildPodMetrics("p4", 401, 0),
				test.BuildPodMetrics("p5", 401, 0),
			},
			expectedPodsEvicted:            4,
			expectedPodsWithMetricsEvicted: 4,
		},
		{
			name: "with priorities",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:  30,
				v1.ResourcePods: 30,
			},
			targetThresholds: api.ResourceThresholds{
				v1.ResourceCPU:  50,
				v1.ResourcePods: 50,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 3000, 9, nil),
				test.BuildTestNode(n2NodeName, 4000, 3000, 10, nil),
				test.BuildTestNode(n3NodeName, 4000, 3000, 10, test.SetNodeUnschedulable),
			},
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.SetPodPriority(pod, highPriority)
				}),
				test.BuildTestPod("p2", 400, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.SetPodPriority(pod, highPriority)
				}),
				test.BuildTestPod("p3", 400, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.SetPodPriority(pod, highPriority)
				}),
				test.BuildTestPod("p4", 400, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.SetPodPriority(pod, highPriority)
				}),
				test.BuildTestPod("p5", 400, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.SetPodPriority(pod, lowPriority)
				}),
				// These won't be evicted.
				test.BuildTestPod("p6", 400, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetDSOwnerRef(pod)
					test.SetPodPriority(pod, highPriority)
				}),
				test.BuildTestPod("p7", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A pod with local storage.
					test.SetNormalOwnerRef(pod)
					test.SetPodPriority(pod, lowPriority)
					pod.Spec.Volumes = []v1.Volume{
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
					pod.Annotations = test.GetMirrorPodAnnotation()
				}),
				test.BuildTestPod("p8", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A Critical Pod.
					test.SetNormalOwnerRef(pod)
					pod.Namespace = "kube-system"
					priority := utils.SystemCriticalPriority
					pod.Spec.Priority = &priority
				}),
				test.BuildTestPod("p9", 400, 0, n2NodeName, test.SetRSOwnerRef),
			},
			nodemetricses: []*v1beta1.NodeMetrics{
				test.BuildNodeMetrics(n1NodeName, 3201, 0),
				test.BuildNodeMetrics(n2NodeName, 401, 0),
				test.BuildNodeMetrics(n3NodeName, 11, 0),
			},
			podmetricses: []*v1beta1.PodMetrics{
				test.BuildPodMetrics("p1", 401, 0),
				test.BuildPodMetrics("p2", 401, 0),
				test.BuildPodMetrics("p3", 401, 0),
				test.BuildPodMetrics("p4", 401, 0),
				test.BuildPodMetrics("p5", 401, 0),
			},
			expectedPodsEvicted:            4,
			expectedPodsWithMetricsEvicted: 4,
		},
		{
			name: "without priorities evicting best-effort pods only",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:  30,
				v1.ResourcePods: 30,
			},
			targetThresholds: api.ResourceThresholds{
				v1.ResourceCPU:  50,
				v1.ResourcePods: 50,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 3000, 9, nil),
				test.BuildTestNode(n2NodeName, 4000, 3000, 10, nil),
				test.BuildTestNode(n3NodeName, 4000, 3000, 10, test.SetNodeUnschedulable),
			},
			// All pods are assumed to be burstable (tc.BuildTestNode always sets both cpu/memory resource requests to some value)
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.MakeBestEffortPod(pod)
				}),
				test.BuildTestPod("p2", 400, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.MakeBestEffortPod(pod)
				}),
				test.BuildTestPod("p3", 400, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
				}),
				test.BuildTestPod("p4", 400, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.MakeBestEffortPod(pod)
				}),
				test.BuildTestPod("p5", 400, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.MakeBestEffortPod(pod)
				}),
				// These won't be evicted.
				test.BuildTestPod("p6", 400, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetDSOwnerRef(pod)
				}),
				test.BuildTestPod("p7", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A pod with local storage.
					test.SetNormalOwnerRef(pod)
					pod.Spec.Volumes = []v1.Volume{
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
					pod.Annotations = test.GetMirrorPodAnnotation()
				}),
				test.BuildTestPod("p8", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A Critical Pod.
					test.SetNormalOwnerRef(pod)
					pod.Namespace = "kube-system"
					priority := utils.SystemCriticalPriority
					pod.Spec.Priority = &priority
				}),
				test.BuildTestPod("p9", 400, 0, n2NodeName, test.SetRSOwnerRef),
			},
			nodemetricses: []*v1beta1.NodeMetrics{
				test.BuildNodeMetrics(n1NodeName, 3201, 0),
				test.BuildNodeMetrics(n2NodeName, 401, 0),
				test.BuildNodeMetrics(n3NodeName, 11, 0),
			},
			podmetricses: []*v1beta1.PodMetrics{
				test.BuildPodMetrics("p1", 401, 0),
				test.BuildPodMetrics("p2", 401, 0),
				test.BuildPodMetrics("p3", 401, 0),
				test.BuildPodMetrics("p4", 401, 0),
				test.BuildPodMetrics("p5", 401, 0),
			},
			expectedPodsEvicted:            4,
			expectedPodsWithMetricsEvicted: 4,
			evictedPods:                    []string{"p1", "p2", "p4", "p5"},
		},
		{
			name: "with extended resource",
			thresholds: api.ResourceThresholds{
				v1.ResourcePods:  30,
				extendedResource: 30,
			},
			targetThresholds: api.ResourceThresholds{
				v1.ResourcePods:  50,
				extendedResource: 50,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 3000, 9, func(node *v1.Node) {
					test.SetNodeExtendedResource(node, extendedResource, 8)
				}),
				test.BuildTestNode(n2NodeName, 4000, 3000, 10, func(node *v1.Node) {
					test.SetNodeExtendedResource(node, extendedResource, 8)
				}),
				test.BuildTestNode(n3NodeName, 4000, 3000, 10, test.SetNodeUnschedulable),
			},
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 0, 0, n1NodeName, func(pod *v1.Pod) {
					// A pod with extended resource.
					test.SetRSOwnerRef(pod)
					test.SetPodExtendedResourceRequest(pod, extendedResource, 1)
				}),
				test.BuildTestPod("p2", 0, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.SetPodExtendedResourceRequest(pod, extendedResource, 1)
				}),
				test.BuildTestPod("p3", 0, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.SetPodExtendedResourceRequest(pod, extendedResource, 1)
				}),
				test.BuildTestPod("p4", 0, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.SetPodExtendedResourceRequest(pod, extendedResource, 1)
				}),
				test.BuildTestPod("p5", 0, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.SetPodExtendedResourceRequest(pod, extendedResource, 1)
				}),
				test.BuildTestPod("p6", 0, 0, n1NodeName, func(pod *v1.Pod) {
					test.SetNormalOwnerRef(pod)
					test.SetPodExtendedResourceRequest(pod, extendedResource, 1)
				}),

				test.BuildTestPod("p7", 0, 0, n1NodeName, func(pod *v1.Pod) {
					// A pod with local storage.
					test.SetNormalOwnerRef(pod)
					test.SetPodExtendedResourceRequest(pod, extendedResource, 1)
					pod.Spec.Volumes = []v1.Volume{
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
					pod.Annotations = test.GetMirrorPodAnnotation()
				}),
				test.BuildTestPod("p8", 0, 0, n1NodeName, func(pod *v1.Pod) {
					// A Critical Pod.
					test.SetNormalOwnerRef(pod)
					test.SetPodExtendedResourceRequest(pod, extendedResource, 1)
					pod.Namespace = "kube-system"
					priority := utils.SystemCriticalPriority
					pod.Spec.Priority = &priority
				}),
				test.BuildTestPod("p9", 0, 0, n2NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
					test.SetPodExtendedResourceRequest(pod, extendedResource, 1)
				}),
			},
			nodemetricses: []*v1beta1.NodeMetrics{
				test.BuildNodeMetrics(n1NodeName, 3201, 0),
				test.BuildNodeMetrics(n2NodeName, 401, 0),
				test.BuildNodeMetrics(n3NodeName, 11, 0),
			},
			podmetricses: []*v1beta1.PodMetrics{
				test.BuildPodMetrics("p1", 401, 0),
				test.BuildPodMetrics("p2", 401, 0),
				test.BuildPodMetrics("p3", 401, 0),
				test.BuildPodMetrics("p4", 401, 0),
				test.BuildPodMetrics("p5", 401, 0),
			},
			// 4 pods available for eviction based on v1.ResourcePods, only 3 pods can be evicted before extended resource is depleted
			expectedPodsEvicted:            3,
			expectedPodsWithMetricsEvicted: 0,
		},
		{
			name: "with extended resource in some of nodes",
			thresholds: api.ResourceThresholds{
				v1.ResourcePods:  30,
				extendedResource: 30,
			},
			targetThresholds: api.ResourceThresholds{
				v1.ResourcePods:  50,
				extendedResource: 50,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 3000, 9, func(node *v1.Node) {
					test.SetNodeExtendedResource(node, extendedResource, 8)
				}),
				test.BuildTestNode(n2NodeName, 4000, 3000, 10, nil),
				test.BuildTestNode(n3NodeName, 4000, 3000, 10, test.SetNodeUnschedulable),
			},
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 0, 0, n1NodeName, func(pod *v1.Pod) {
					// A pod with extended resource.
					test.SetRSOwnerRef(pod)
					test.SetPodExtendedResourceRequest(pod, extendedResource, 1)
				}),
				test.BuildTestPod("p9", 0, 0, n2NodeName, test.SetRSOwnerRef),
			},
			nodemetricses: []*v1beta1.NodeMetrics{
				test.BuildNodeMetrics(n1NodeName, 3201, 0),
				test.BuildNodeMetrics(n2NodeName, 401, 0),
				test.BuildNodeMetrics(n3NodeName, 11, 0),
			},
			podmetricses: []*v1beta1.PodMetrics{
				test.BuildPodMetrics("p1", 401, 0),
				test.BuildPodMetrics("p2", 401, 0),
				test.BuildPodMetrics("p3", 401, 0),
				test.BuildPodMetrics("p4", 401, 0),
				test.BuildPodMetrics("p5", 401, 0),
			},
			// 0 pods available for eviction because there's no enough extended resource in node2
			expectedPodsEvicted:            0,
			expectedPodsWithMetricsEvicted: 0,
		},
		{
			name: "with extended resource in some of nodes with deviation",
			thresholds: api.ResourceThresholds{
				v1.ResourcePods:  5,
				extendedResource: 10,
			},
			targetThresholds: api.ResourceThresholds{
				v1.ResourcePods:  5,
				extendedResource: 10,
			},
			useDeviationThresholds: true,
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 3000, 10, func(node *v1.Node) {
					test.SetNodeExtendedResource(node, extendedResource, 8)
				}),
				test.BuildTestNode(n2NodeName, 4000, 3000, 10, func(node *v1.Node) {
					test.SetNodeExtendedResource(node, extendedResource, 8)
				}),
				test.BuildTestNode(n3NodeName, 4000, 3000, 10, test.SetNodeUnschedulable),
			},
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 0, 0, n1NodeName, func(pod *v1.Pod) {
					// A pod with extended resource.
					test.SetRSOwnerRef(pod)
					test.SetPodExtendedResourceRequest(pod, extendedResource, 1)
				}),
				test.BuildTestPod("p2", 0, 0, n2NodeName, func(pod *v1.Pod) {
					// A pod with extended resource.
					test.SetRSOwnerRef(pod)
					test.SetPodExtendedResourceRequest(pod, extendedResource, 7)
				}),
				test.BuildTestPod("p3", 0, 0, n2NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
				}),
				test.BuildTestPod("p8", 0, 0, n3NodeName, func(pod *v1.Pod) {
					test.SetRSOwnerRef(pod)
				}),
				test.BuildTestPod("p9", 0, 0, n3NodeName, test.SetRSOwnerRef),
			},
			nodemetricses: []*v1beta1.NodeMetrics{
				test.BuildNodeMetrics(n1NodeName, 3201, 0),
				test.BuildNodeMetrics(n2NodeName, 401, 0),
				test.BuildNodeMetrics(n3NodeName, 11, 0),
			},
			podmetricses: []*v1beta1.PodMetrics{
				test.BuildPodMetrics("p1", 401, 0),
				test.BuildPodMetrics("p2", 401, 0),
				test.BuildPodMetrics("p3", 401, 0),
				test.BuildPodMetrics("p4", 401, 0),
				test.BuildPodMetrics("p5", 401, 0),
			},
			expectedPodsEvicted:            1,
			expectedPodsWithMetricsEvicted: 0,
		},
		{
			name: "without priorities, but only other node is unschedulable",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:  30,
				v1.ResourcePods: 30,
			},
			targetThresholds: api.ResourceThresholds{
				v1.ResourceCPU:  50,
				v1.ResourcePods: 50,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 3000, 9, nil),
				test.BuildTestNode(n2NodeName, 4000, 3000, 10, test.SetNodeUnschedulable),
			},
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p2", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p3", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p4", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p5", 400, 0, n1NodeName, test.SetRSOwnerRef),
				// These won't be evicted.
				test.BuildTestPod("p6", 400, 0, n1NodeName, test.SetDSOwnerRef),
				test.BuildTestPod("p7", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A pod with local storage.
					test.SetNormalOwnerRef(pod)
					pod.Spec.Volumes = []v1.Volume{
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
					pod.Annotations = test.GetMirrorPodAnnotation()
				}),
				test.BuildTestPod("p8", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A Critical Pod.
					test.SetNormalOwnerRef(pod)
					pod.Namespace = "kube-system"
					priority := utils.SystemCriticalPriority
					pod.Spec.Priority = &priority
				}),
			},
			nodemetricses: []*v1beta1.NodeMetrics{
				test.BuildNodeMetrics(n1NodeName, 3201, 0),
				test.BuildNodeMetrics(n2NodeName, 401, 0),
			},
			podmetricses: []*v1beta1.PodMetrics{
				test.BuildPodMetrics("p1", 401, 0),
				test.BuildPodMetrics("p2", 401, 0),
				test.BuildPodMetrics("p3", 401, 0),
				test.BuildPodMetrics("p4", 401, 0),
				test.BuildPodMetrics("p5", 401, 0),
			},
			expectedPodsEvicted:            0,
			expectedPodsWithMetricsEvicted: 0,
		},
		{
			name: "without priorities, but only other node doesn't match pod node selector for p4 and p5",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:  30,
				v1.ResourcePods: 30,
			},
			targetThresholds: api.ResourceThresholds{
				v1.ResourceCPU:  50,
				v1.ResourcePods: 50,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 3000, 9, nil),
				test.BuildTestNode(n2NodeName, 4000, 3000, 10, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeSelectorKey: notMatchingNodeSelectorValue,
					}
				}),
			},
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p2", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p3", 400, 0, n1NodeName, test.SetRSOwnerRef),
				// These won't be evicted.
				test.BuildTestPod("p4", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A pod selecting nodes in the "west" datacenter
					test.SetNormalOwnerRef(pod)
					pod.Spec.NodeSelector = map[string]string{
						nodeSelectorKey: nodeSelectorValue,
					}
				}),
				test.BuildTestPod("p5", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A pod selecting nodes in the "west" datacenter
					test.SetNormalOwnerRef(pod)
					pod.Spec.NodeSelector = map[string]string{
						nodeSelectorKey: nodeSelectorValue,
					}
				}),
				test.BuildTestPod("p6", 400, 0, n1NodeName, test.SetDSOwnerRef),
				test.BuildTestPod("p7", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A pod with local storage.
					test.SetNormalOwnerRef(pod)
					pod.Spec.Volumes = []v1.Volume{
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
					pod.Annotations = test.GetMirrorPodAnnotation()
				}),
				test.BuildTestPod("p8", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A Critical Pod.
					test.SetNormalOwnerRef(pod)
					pod.Namespace = "kube-system"
					priority := utils.SystemCriticalPriority
					pod.Spec.Priority = &priority
				}),
			},
			nodemetricses: []*v1beta1.NodeMetrics{
				test.BuildNodeMetrics(n1NodeName, 3201, 0),
				test.BuildNodeMetrics(n2NodeName, 401, 0),
			},
			podmetricses: []*v1beta1.PodMetrics{
				test.BuildPodMetrics("p1", 401, 0),
				test.BuildPodMetrics("p2", 401, 0),
				test.BuildPodMetrics("p3", 401, 0),
			},
			expectedPodsEvicted:            3,
			expectedPodsWithMetricsEvicted: 3,
		},
		{
			name: "without priorities, but only other node doesn't match pod node affinity for p4 and p5",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:  30,
				v1.ResourcePods: 30,
			},
			targetThresholds: api.ResourceThresholds{
				v1.ResourceCPU:  50,
				v1.ResourcePods: 50,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 3000, 9, nil),
				test.BuildTestNode(n2NodeName, 4000, 3000, 10, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeSelectorKey: notMatchingNodeSelectorValue,
					}
				}),
			},
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p2", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p3", 400, 0, n1NodeName, test.SetRSOwnerRef),
				// These won't be evicted.
				test.BuildTestPod("p4", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A pod with affinity to run in the "west" datacenter upon scheduling
					test.SetNormalOwnerRef(pod)
					pod.Spec.Affinity = &v1.Affinity{
						NodeAffinity: &v1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
								NodeSelectorTerms: []v1.NodeSelectorTerm{
									{
										MatchExpressions: []v1.NodeSelectorRequirement{
											{
												Key:      nodeSelectorKey,
												Operator: "In",
												Values:   []string{nodeSelectorValue},
											},
										},
									},
								},
							},
						},
					}
				}),
				test.BuildTestPod("p5", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A pod with affinity to run in the "west" datacenter upon scheduling
					test.SetNormalOwnerRef(pod)
					pod.Spec.Affinity = &v1.Affinity{
						NodeAffinity: &v1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
								NodeSelectorTerms: []v1.NodeSelectorTerm{
									{
										MatchExpressions: []v1.NodeSelectorRequirement{
											{
												Key:      nodeSelectorKey,
												Operator: "In",
												Values:   []string{nodeSelectorValue},
											},
										},
									},
								},
							},
						},
					}
				}),
				test.BuildTestPod("p6", 400, 0, n1NodeName, test.SetDSOwnerRef),
				test.BuildTestPod("p7", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A pod with local storage.
					test.SetNormalOwnerRef(pod)
					pod.Spec.Volumes = []v1.Volume{
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
					pod.Annotations = test.GetMirrorPodAnnotation()
				}),
				test.BuildTestPod("p8", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A Critical Pod.
					test.SetNormalOwnerRef(pod)
					pod.Namespace = "kube-system"
					priority := utils.SystemCriticalPriority
					pod.Spec.Priority = &priority
				}),
				test.BuildTestPod("p9", 0, 0, n2NodeName, test.SetRSOwnerRef),
			},
			nodemetricses: []*v1beta1.NodeMetrics{
				test.BuildNodeMetrics(n1NodeName, 3201, 0),
				test.BuildNodeMetrics(n2NodeName, 401, 0),
			},
			podmetricses: []*v1beta1.PodMetrics{
				test.BuildPodMetrics("p1", 401, 0),
				test.BuildPodMetrics("p2", 401, 0),
				test.BuildPodMetrics("p3", 401, 0),
			},
			expectedPodsEvicted:            3,
			expectedPodsWithMetricsEvicted: 3,
		},
		{
			name: "deviation thresholds",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:  5,
				v1.ResourcePods: 5,
			},
			targetThresholds: api.ResourceThresholds{
				v1.ResourceCPU:  5,
				v1.ResourcePods: 5,
			},
			useDeviationThresholds: true,
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 3000, 9, nil),
				test.BuildTestNode(n2NodeName, 4000, 3000, 10, nil),
				test.BuildTestNode(n3NodeName, 4000, 3000, 10, test.SetNodeUnschedulable),
			},
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p2", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p3", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p4", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p5", 400, 0, n1NodeName, test.SetRSOwnerRef),
				// These won't be evicted.
				test.BuildTestPod("p6", 400, 0, n1NodeName, test.SetDSOwnerRef),
				test.BuildTestPod("p7", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A pod with local storage.
					test.SetNormalOwnerRef(pod)
					pod.Spec.Volumes = []v1.Volume{
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
					pod.Annotations = test.GetMirrorPodAnnotation()
				}),
				test.BuildTestPod("p8", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A Critical Pod.
					test.SetNormalOwnerRef(pod)
					pod.Namespace = "kube-system"
					priority := utils.SystemCriticalPriority
					pod.Spec.Priority = &priority
				}),
				test.BuildTestPod("p9", 400, 0, n2NodeName, test.SetRSOwnerRef),
			},
			nodemetricses: []*v1beta1.NodeMetrics{
				test.BuildNodeMetrics(n1NodeName, 3201, 0),
				test.BuildNodeMetrics(n2NodeName, 401, 0),
				test.BuildNodeMetrics(n3NodeName, 11, 0),
			},
			podmetricses: []*v1beta1.PodMetrics{
				test.BuildPodMetrics("p1", 401, 0),
				test.BuildPodMetrics("p2", 401, 0),
				test.BuildPodMetrics("p3", 401, 0),
				test.BuildPodMetrics("p4", 401, 0),
				test.BuildPodMetrics("p5", 401, 0),
			},
			expectedPodsEvicted:            2,
			expectedPodsWithMetricsEvicted: 2,
			evictedPods:                    []string{},
		},
		{
			name: "deviation thresholds and overevicting memory",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:  5,
				v1.ResourcePods: 5,
			},
			targetThresholds: api.ResourceThresholds{
				v1.ResourceCPU:  5,
				v1.ResourcePods: 5,
			},
			useDeviationThresholds: true,
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 3000, 10, nil),
				test.BuildTestNode(n2NodeName, 4000, 3000, 10, nil),
				test.BuildTestNode(n3NodeName, 4000, 3000, 10, test.SetNodeUnschedulable),
			},
			// totalcpuusage = 3600m, avgcpuusage = 3600/12000 = 0.3 => 30%
			// totalpodsusage = 9, avgpodsusage = 9/30 = 0.3 => 30%
			// n1 and n2 are fully memory utilized
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 375, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p2", 400, 375, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p3", 400, 375, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p4", 400, 375, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p5", 400, 375, n1NodeName, test.SetRSOwnerRef),
				// These won't be evicted.
				test.BuildTestPod("p6", 400, 375, n1NodeName, test.SetDSOwnerRef),
				test.BuildTestPod("p7", 400, 375, n1NodeName, func(pod *v1.Pod) {
					// A pod with local storage.
					test.SetNormalOwnerRef(pod)
					pod.Spec.Volumes = []v1.Volume{
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
					pod.Annotations = test.GetMirrorPodAnnotation()
				}),
				test.BuildTestPod("p8", 400, 375, n1NodeName, func(pod *v1.Pod) {
					// A Critical Pod.
					test.SetNormalOwnerRef(pod)
					pod.Namespace = "kube-system"
					priority := utils.SystemCriticalPriority
					pod.Spec.Priority = &priority
				}),
				test.BuildTestPod("p9", 400, 3000, n2NodeName, test.SetRSOwnerRef),
			},
			nodemetricses: []*v1beta1.NodeMetrics{
				test.BuildNodeMetrics(n1NodeName, 4000, 3000),
				test.BuildNodeMetrics(n2NodeName, 4000, 3000),
				test.BuildNodeMetrics(n3NodeName, 4000, 3000),
			},
			podmetricses: []*v1beta1.PodMetrics{
				test.BuildPodMetrics("p1", 400, 375),
				test.BuildPodMetrics("p2", 400, 375),
				test.BuildPodMetrics("p3", 400, 375),
				test.BuildPodMetrics("p4", 400, 375),
				test.BuildPodMetrics("p5", 400, 375),
				test.BuildPodMetrics("p6", 400, 375),
				test.BuildPodMetrics("p7", 400, 375),
				test.BuildPodMetrics("p8", 400, 375),
				test.BuildPodMetrics("p9", 400, 3000),
			},
			expectedPodsEvicted:            0,
			expectedPodsWithMetricsEvicted: 0,
			evictedPods:                    []string{},
		},
		{
			name: "without priorities different evictions for requested and actual resources",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:  30,
				v1.ResourcePods: 30,
			},
			targetThresholds: api.ResourceThresholds{
				v1.ResourceCPU:  50,
				v1.ResourcePods: 50,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 3000, 9, nil),
				test.BuildTestNode(n2NodeName, 4000, 3000, 10, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeSelectorKey: notMatchingNodeSelectorValue,
					}
				}),
			},
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p2", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p3", 400, 0, n1NodeName, test.SetRSOwnerRef),
				// These won't be evicted.
				test.BuildTestPod("p4", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A pod with affinity to run in the "west" datacenter upon scheduling
					test.SetNormalOwnerRef(pod)
					pod.Spec.Affinity = &v1.Affinity{
						NodeAffinity: &v1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
								NodeSelectorTerms: []v1.NodeSelectorTerm{
									{
										MatchExpressions: []v1.NodeSelectorRequirement{
											{
												Key:      nodeSelectorKey,
												Operator: "In",
												Values:   []string{nodeSelectorValue},
											},
										},
									},
								},
							},
						},
					}
				}),
				test.BuildTestPod("p5", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A pod with affinity to run in the "west" datacenter upon scheduling
					test.SetNormalOwnerRef(pod)
					pod.Spec.Affinity = &v1.Affinity{
						NodeAffinity: &v1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
								NodeSelectorTerms: []v1.NodeSelectorTerm{
									{
										MatchExpressions: []v1.NodeSelectorRequirement{
											{
												Key:      nodeSelectorKey,
												Operator: "In",
												Values:   []string{nodeSelectorValue},
											},
										},
									},
								},
							},
						},
					}
				}),
				test.BuildTestPod("p6", 400, 0, n1NodeName, test.SetDSOwnerRef),
				test.BuildTestPod("p7", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A pod with local storage.
					test.SetNormalOwnerRef(pod)
					pod.Spec.Volumes = []v1.Volume{
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
					pod.Annotations = test.GetMirrorPodAnnotation()
				}),
				test.BuildTestPod("p8", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A Critical Pod.
					test.SetNormalOwnerRef(pod)
					pod.Namespace = "kube-system"
					priority := utils.SystemCriticalPriority
					pod.Spec.Priority = &priority
				}),
				test.BuildTestPod("p9", 0, 0, n2NodeName, test.SetRSOwnerRef),
			},
			nodemetricses: []*v1beta1.NodeMetrics{
				test.BuildNodeMetrics(n1NodeName, 3201, 0),
				test.BuildNodeMetrics(n2NodeName, 401, 0),
			},
			podmetricses: []*v1beta1.PodMetrics{
				test.BuildPodMetrics("p1", 801, 0),
				test.BuildPodMetrics("p2", 801, 0),
				test.BuildPodMetrics("p3", 801, 0),
			},
			expectedPodsEvicted:            3,
			expectedPodsWithMetricsEvicted: 2,
		},
		{
			name: "without priorities with node eviction limit",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:  30,
				v1.ResourcePods: 30,
			},
			targetThresholds: api.ResourceThresholds{
				v1.ResourceCPU:  50,
				v1.ResourcePods: 50,
			},
			evictionLimits: &api.EvictionLimits{
				Node: ptr.To[uint](2),
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 3000, 9, nil),
				test.BuildTestNode(n2NodeName, 4000, 3000, 10, nil),
				test.BuildTestNode(n3NodeName, 4000, 3000, 10, test.SetNodeUnschedulable),
			},
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p2", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p3", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p4", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p5", 400, 0, n1NodeName, test.SetRSOwnerRef),
				// These won't be evicted.
				test.BuildTestPod("p6", 400, 0, n1NodeName, test.SetDSOwnerRef),
				test.BuildTestPod("p7", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A pod with local storage.
					test.SetNormalOwnerRef(pod)
					pod.Spec.Volumes = []v1.Volume{
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
					pod.Annotations = test.GetMirrorPodAnnotation()
				}),
				test.BuildTestPod("p8", 400, 0, n1NodeName, func(pod *v1.Pod) {
					// A Critical Pod.
					pod.Namespace = "kube-system"
					priority := utils.SystemCriticalPriority
					pod.Spec.Priority = &priority
				}),
				test.BuildTestPod("p9", 400, 0, n2NodeName, test.SetRSOwnerRef),
			},
			nodemetricses: []*v1beta1.NodeMetrics{
				test.BuildNodeMetrics(n1NodeName, 3201, 0),
				test.BuildNodeMetrics(n2NodeName, 401, 0),
				test.BuildNodeMetrics(n3NodeName, 11, 0),
			},
			podmetricses: []*v1beta1.PodMetrics{
				test.BuildPodMetrics("p1", 401, 0),
				test.BuildPodMetrics("p2", 401, 0),
				test.BuildPodMetrics("p3", 401, 0),
				test.BuildPodMetrics("p4", 401, 0),
				test.BuildPodMetrics("p5", 401, 0),
			},
			expectedPodsEvicted:            2,
			expectedPodsWithMetricsEvicted: 2,
		},
	}

	for _, tc := range testCases {
		testFnc := func(metricsEnabled bool, expectedPodsEvicted uint) func(t *testing.T) {
			return func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				var objs []runtime.Object
				for _, node := range tc.nodes {
					objs = append(objs, node)
				}
				for _, pod := range tc.pods {
					objs = append(objs, pod)
				}

				fakeClient := fake.NewSimpleClientset(objs...)

				var collector *metricscollector.MetricsCollector
				if metricsEnabled {
					metricsClientset := fakemetricsclient.NewSimpleClientset()
					for _, nodemetrics := range tc.nodemetricses {
						metricsClientset.Tracker().Create(nodesgvr, nodemetrics, "")
					}
					for _, podmetrics := range tc.podmetricses {
						metricsClientset.Tracker().Create(podsgvr, podmetrics, podmetrics.Namespace)
					}

					sharedInformerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
					nodeLister := sharedInformerFactory.Core().V1().Nodes().Lister()
					sharedInformerFactory.Start(ctx.Done())
					sharedInformerFactory.WaitForCacheSync(ctx.Done())

					collector = metricscollector.NewMetricsCollector(nodeLister, metricsClientset, labels.Everything())
					err := collector.Collect(ctx)
					if err != nil {
						t.Fatalf("unable to collect metrics: %v", err)
					}
				}

				podsForEviction := make(map[string]struct{})
				for _, pod := range tc.evictedPods {
					podsForEviction[pod] = struct{}{}
				}

				evictionFailed := false
				if len(tc.evictedPods) > 0 {
					fakeClient.Fake.AddReactor("create", "pods", func(action core.Action) (bool, runtime.Object, error) {
						getAction := action.(core.CreateAction)
						obj := getAction.GetObject()
						if eviction, ok := obj.(*policy.Eviction); ok {
							if _, exists := podsForEviction[eviction.Name]; exists {
								return true, obj, nil
							}
							evictionFailed = true
							return true, nil, fmt.Errorf("pod %q was unexpectedly evicted", eviction.Name)
						}
						return true, obj, nil
					})
				}

				handle, podEvictor, err := frameworktesting.InitFrameworkHandle(ctx, fakeClient, nil, defaultevictor.DefaultEvictorArgs{NodeFit: true}, nil)
				if err != nil {
					t.Fatalf("Unable to initialize a framework handle: %v", err)
				}
				handle.MetricsCollectorImpl = collector

				var metricsUtilization *MetricsUtilization
				if metricsEnabled {
					metricsUtilization = &MetricsUtilization{Source: api.KubernetesMetrics}
				}

				plugin, err := NewLowNodeUtilization(&LowNodeUtilizationArgs{
					Thresholds:             tc.thresholds,
					TargetThresholds:       tc.targetThresholds,
					UseDeviationThresholds: tc.useDeviationThresholds,
					EvictionLimits:         tc.evictionLimits,
					EvictableNamespaces:    tc.evictableNamespaces,
					MetricsUtilization:     metricsUtilization,
				},
					handle)
				if err != nil {
					t.Fatalf("Unable to initialize the plugin: %v", err)
				}
				plugin.(frameworktypes.BalancePlugin).Balance(ctx, tc.nodes)

				podsEvicted := podEvictor.TotalEvicted()
				if expectedPodsEvicted != podsEvicted {
					t.Errorf("Expected %v pods to be evicted but %v got evicted", expectedPodsEvicted, podsEvicted)
				}
				if evictionFailed {
					t.Errorf("Pod evictions failed unexpectedly")
				}
			}
		}
		t.Run(tc.name, testFnc(false, tc.expectedPodsEvicted))
		t.Run(tc.name+" with metrics enabled", testFnc(true, tc.expectedPodsWithMetricsEvicted))
	}
}

func TestLowNodeUtilizationWithTaints(t *testing.T) {
	ctx := context.Background()

	n1 := test.BuildTestNode("n1", 2000, 3000, 10, nil)
	n2 := test.BuildTestNode("n2", 1000, 3000, 10, nil)
	n3 := test.BuildTestNode("n3", 1000, 3000, 10, nil)
	n3withTaints := n3.DeepCopy()
	n3withTaints.Spec.Taints = []v1.Taint{
		{
			Key:    "key",
			Value:  "value",
			Effect: v1.TaintEffectNoSchedule,
		},
	}

	podThatToleratesTaint := test.BuildTestPod("tolerate_pod", 200, 0, n1.Name, test.SetRSOwnerRef)
	podThatToleratesTaint.Spec.Tolerations = []v1.Toleration{
		{
			Key:   "key",
			Value: "value",
		},
	}

	var uint0, uint1 uint = 0, 1
	tests := []struct {
		name                  string
		nodes                 []*v1.Node
		pods                  []*v1.Pod
		maxPodsToEvictPerNode *uint
		maxPodsToEvictTotal   *uint
		evictionsExpected     uint
	}{
		{
			name:  "No taints",
			nodes: []*v1.Node{n1, n2, n3},
			pods: []*v1.Pod{
				// Node 1 pods
				test.BuildTestPod(fmt.Sprintf("pod_1_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_2_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_3_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_4_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_5_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_6_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_7_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_8_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				// Node 2 pods
				test.BuildTestPod(fmt.Sprintf("pod_9_%s", n2.Name), 200, 0, n2.Name, test.SetRSOwnerRef),
			},
			evictionsExpected: 1,
		},
		{
			name:  "No pod tolerates node taint",
			nodes: []*v1.Node{n1, n3withTaints},
			pods: []*v1.Pod{
				// Node 1 pods
				test.BuildTestPod(fmt.Sprintf("pod_1_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_2_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_3_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_4_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_5_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_6_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_7_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_8_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				// Node 3 pods
				test.BuildTestPod(fmt.Sprintf("pod_9_%s", n3withTaints.Name), 200, 0, n3withTaints.Name, test.SetRSOwnerRef),
			},
			evictionsExpected: 0,
		},
		{
			name:  "Pod which tolerates node taint",
			nodes: []*v1.Node{n1, n3withTaints},
			pods: []*v1.Pod{
				// Node 1 pods
				test.BuildTestPod(fmt.Sprintf("pod_1_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_2_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_3_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_4_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_5_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_6_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_7_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				podThatToleratesTaint,
				// Node 3 pods
				test.BuildTestPod(fmt.Sprintf("pod_9_%s", n3withTaints.Name), 200, 0, n3withTaints.Name, test.SetRSOwnerRef),
			},
			evictionsExpected: 1,
		},
		{
			name:  "Pod which tolerates node taint, set maxPodsToEvictTotal(0), should not be expelled",
			nodes: []*v1.Node{n1, n3withTaints},
			pods: []*v1.Pod{
				// Node 1 pods
				test.BuildTestPod(fmt.Sprintf("pod_1_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_2_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_3_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_4_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_5_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_6_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				test.BuildTestPod(fmt.Sprintf("pod_7_%s", n1.Name), 200, 0, n1.Name, test.SetRSOwnerRef),
				podThatToleratesTaint,
				// Node 3 pods
				test.BuildTestPod(fmt.Sprintf("pod_9_%s", n3withTaints.Name), 200, 0, n3withTaints.Name, test.SetRSOwnerRef),
			},
			maxPodsToEvictPerNode: &uint1,
			maxPodsToEvictTotal:   &uint0,
			evictionsExpected:     0,
		},
	}

	for _, item := range tests {
		t.Run(item.name, func(t *testing.T) {
			var objs []runtime.Object
			for _, node := range item.nodes {
				objs = append(objs, node)
			}

			for _, pod := range item.pods {
				objs = append(objs, pod)
			}

			fakeClient := fake.NewSimpleClientset(objs...)

			handle, podEvictor, err := frameworktesting.InitFrameworkHandle(
				ctx,
				fakeClient,
				evictions.NewOptions().WithMaxPodsToEvictPerNode(&item.evictionsExpected),
				defaultevictor.DefaultEvictorArgs{NodeFit: true},
				nil,
			)
			if err != nil {
				t.Fatalf("Unable to initialize a framework handle: %v", err)
			}

			plugin, err := NewLowNodeUtilization(&LowNodeUtilizationArgs{
				Thresholds: api.ResourceThresholds{
					v1.ResourcePods: 20,
				},
				TargetThresholds: api.ResourceThresholds{
					v1.ResourcePods: 70,
				},
			},
				handle)
			if err != nil {
				t.Fatalf("Unable to initialize the plugin: %v", err)
			}
			plugin.(frameworktypes.BalancePlugin).Balance(ctx, item.nodes)

			if item.evictionsExpected != podEvictor.TotalEvicted() {
				t.Errorf("Expected %v evictions, got %v", item.evictionsExpected, podEvictor.TotalEvicted())
			}
		})
	}
}

func withLocalStorage(pod *v1.Pod) {
	// A pod with local storage.
	test.SetNormalOwnerRef(pod)
	pod.Spec.Volumes = []v1.Volume{
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
	pod.Annotations = test.GetMirrorPodAnnotation()
}

func withCriticalPod(pod *v1.Pod) {
	// A Critical Pod.
	test.SetNormalOwnerRef(pod)
	pod.Namespace = "kube-system"
	priority := utils.SystemCriticalPriority
	pod.Spec.Priority = &priority
}

func TestLowNodeUtilizationWithPrometheusMetrics(t *testing.T) {
	n1NodeName := "n1"
	n2NodeName := "n2"
	n3NodeName := "n3"

	testCases := []struct {
		name                string
		samples             model.Vector
		nodes               []*v1.Node
		pods                []*v1.Pod
		expectedPodsEvicted uint
		evictedPods         []string
		args                *LowNodeUtilizationArgs
	}{
		{
			name: "with instance:node_cpu:rate:sum query",
			args: &LowNodeUtilizationArgs{
				Thresholds: api.ResourceThresholds{
					MetricResource: 30,
				},
				TargetThresholds: api.ResourceThresholds{
					MetricResource: 50,
				},
				MetricsUtilization: &MetricsUtilization{
					Source: api.PrometheusMetrics,
					Prometheus: &Prometheus{
						Query: "instance:node_cpu:rate:sum",
					},
				},
			},
			samples: model.Vector{
				sample("instance:node_cpu:rate:sum", n1NodeName, 0.5695757575757561),
				sample("instance:node_cpu:rate:sum", n2NodeName, 0.4245454545454522),
				sample("instance:node_cpu:rate:sum", n3NodeName, 0.20381818181818104),
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 3000, 9, nil),
				test.BuildTestNode(n2NodeName, 4000, 3000, 10, nil),
				test.BuildTestNode(n3NodeName, 4000, 3000, 10, nil),
			},
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p2", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p3", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p4", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p5", 400, 0, n1NodeName, test.SetRSOwnerRef),
				// These won't be evicted.
				test.BuildTestPod("p6", 400, 0, n1NodeName, test.SetDSOwnerRef),
				test.BuildTestPod("p7", 400, 0, n1NodeName, withLocalStorage),
				test.BuildTestPod("p8", 400, 0, n1NodeName, withCriticalPod),
				test.BuildTestPod("p9", 400, 0, n2NodeName, test.SetRSOwnerRef),
			},
			expectedPodsEvicted: 1,
		},
		{
			name: "with instance:node_cpu:rate:sum query with more evictions",
			args: &LowNodeUtilizationArgs{
				Thresholds: api.ResourceThresholds{
					MetricResource: 30,
				},
				TargetThresholds: api.ResourceThresholds{
					MetricResource: 50,
				},
				EvictionLimits: &api.EvictionLimits{
					Node: ptr.To[uint](3),
				},
				MetricsUtilization: &MetricsUtilization{
					Source: api.PrometheusMetrics,
					Prometheus: &Prometheus{
						Query: "instance:node_cpu:rate:sum",
					},
				},
			},
			samples: model.Vector{
				sample("instance:node_cpu:rate:sum", n1NodeName, 0.5695757575757561),
				sample("instance:node_cpu:rate:sum", n2NodeName, 0.4245454545454522),
				sample("instance:node_cpu:rate:sum", n3NodeName, 0.20381818181818104),
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 3000, 9, nil),
				test.BuildTestNode(n2NodeName, 4000, 3000, 10, nil),
				test.BuildTestNode(n3NodeName, 4000, 3000, 10, nil),
			},
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p2", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p3", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p4", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p5", 400, 0, n1NodeName, test.SetRSOwnerRef),
				// These won't be evicted.
				test.BuildTestPod("p6", 400, 0, n1NodeName, test.SetDSOwnerRef),
				test.BuildTestPod("p7", 400, 0, n1NodeName, withLocalStorage),
				test.BuildTestPod("p8", 400, 0, n1NodeName, withCriticalPod),
				test.BuildTestPod("p9", 400, 0, n2NodeName, test.SetRSOwnerRef),
			},
			expectedPodsEvicted: 3,
		},
		{
			name: "with instance:node_cpu:rate:sum query with deviation",
			args: &LowNodeUtilizationArgs{
				Thresholds: api.ResourceThresholds{
					MetricResource: 5,
				},
				TargetThresholds: api.ResourceThresholds{
					MetricResource: 5,
				},
				EvictionLimits: &api.EvictionLimits{
					Node: ptr.To[uint](2),
				},
				UseDeviationThresholds: true,
				MetricsUtilization: &MetricsUtilization{
					Source: api.PrometheusMetrics,
					Prometheus: &Prometheus{
						Query: "instance:node_cpu:rate:sum",
					},
				},
			},
			samples: model.Vector{
				sample("instance:node_cpu:rate:sum", n1NodeName, 0.5695757575757561),
				sample("instance:node_cpu:rate:sum", n2NodeName, 0.4245454545454522),
				sample("instance:node_cpu:rate:sum", n3NodeName, 0.20381818181818104),
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 3000, 9, nil),
				test.BuildTestNode(n2NodeName, 4000, 3000, 10, nil),
				test.BuildTestNode(n3NodeName, 4000, 3000, 10, nil),
			},
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p2", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p3", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p4", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p5", 400, 0, n1NodeName, test.SetRSOwnerRef),
				// These won't be evicted.
				test.BuildTestPod("p6", 400, 0, n1NodeName, test.SetDSOwnerRef),
				test.BuildTestPod("p7", 400, 0, n1NodeName, withLocalStorage),
				test.BuildTestPod("p8", 400, 0, n1NodeName, withCriticalPod),
				test.BuildTestPod("p9", 400, 0, n2NodeName, test.SetRSOwnerRef),
			},
			expectedPodsEvicted: 2,
		},
		{
			name: "with instance:node_cpu:rate:sum query and deviation thresholds",
			args: &LowNodeUtilizationArgs{
				UseDeviationThresholds: true,
				Thresholds:             api.ResourceThresholds{MetricResource: 10},
				TargetThresholds:       api.ResourceThresholds{MetricResource: 10},
				MetricsUtilization: &MetricsUtilization{
					Source: api.PrometheusMetrics,
					Prometheus: &Prometheus{
						Query: "instance:node_cpu:rate:sum",
					},
				},
			},
			samples: model.Vector{
				sample("instance:node_cpu:rate:sum", n1NodeName, 1),
				sample("instance:node_cpu:rate:sum", n2NodeName, 0.5),
				sample("instance:node_cpu:rate:sum", n3NodeName, 0),
			},
			nodes: []*v1.Node{
				test.BuildTestNode(n1NodeName, 4000, 3000, 9, nil),
				test.BuildTestNode(n2NodeName, 4000, 3000, 10, nil),
				test.BuildTestNode(n3NodeName, 4000, 3000, 10, nil),
			},
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p2", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p3", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p4", 400, 0, n1NodeName, test.SetRSOwnerRef),
				test.BuildTestPod("p5", 400, 0, n1NodeName, test.SetRSOwnerRef),
				// These won't be evicted.
				test.BuildTestPod("p6", 400, 0, n1NodeName, test.SetDSOwnerRef),
				test.BuildTestPod("p7", 400, 0, n1NodeName, withLocalStorage),
				test.BuildTestPod("p8", 400, 0, n1NodeName, withCriticalPod),
				test.BuildTestPod("p9", 400, 0, n2NodeName, test.SetRSOwnerRef),
			},
			expectedPodsEvicted: 1,
		},
	}

	for _, tc := range testCases {
		testFnc := func(metricsEnabled bool, expectedPodsEvicted uint) func(t *testing.T) {
			return func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				var objs []runtime.Object
				for _, node := range tc.nodes {
					objs = append(objs, node)
				}
				for _, pod := range tc.pods {
					objs = append(objs, pod)
				}

				fakeClient := fake.NewSimpleClientset(objs...)

				podsForEviction := make(map[string]struct{})
				for _, pod := range tc.evictedPods {
					podsForEviction[pod] = struct{}{}
				}

				evictionFailed := false
				if len(tc.evictedPods) > 0 {
					fakeClient.Fake.AddReactor("create", "pods", func(action core.Action) (bool, runtime.Object, error) {
						getAction := action.(core.CreateAction)
						obj := getAction.GetObject()
						if eviction, ok := obj.(*policy.Eviction); ok {
							if _, exists := podsForEviction[eviction.Name]; exists {
								return true, obj, nil
							}
							evictionFailed = true
							return true, nil, fmt.Errorf("pod %q was unexpectedly evicted", eviction.Name)
						}
						return true, obj, nil
					})
				}

				handle, podEvictor, err := frameworktesting.InitFrameworkHandle(ctx, fakeClient, nil, defaultevictor.DefaultEvictorArgs{NodeFit: true}, nil)
				if err != nil {
					t.Fatalf("Unable to initialize a framework handle: %v", err)
				}

				handle.PrometheusClientImpl = &fakePromClient{
					result:   tc.samples,
					dataType: model.ValVector,
				}
				plugin, err := NewLowNodeUtilization(tc.args, handle)
				if err != nil {
					t.Fatalf("Unable to initialize the plugin: %v", err)
				}

				status := plugin.(frameworktypes.BalancePlugin).Balance(ctx, tc.nodes)
				if status != nil {
					t.Fatalf("Balance.err: %v", status.Err)
				}

				podsEvicted := podEvictor.TotalEvicted()
				if expectedPodsEvicted != podsEvicted {
					t.Errorf("Expected %v pods to be evicted but %v got evicted", expectedPodsEvicted, podsEvicted)
				}
				if evictionFailed {
					t.Errorf("Pod evictions failed unexpectedly")
				}
			}
		}
		t.Run(tc.name, testFnc(false, tc.expectedPodsEvicted))
	}
}
