/*
Copyright 2022 The Kubernetes Authors.
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

package defaultevictor

import (
	"context"
	"fmt"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"sigs.k8s.io/descheduler/pkg/api"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	frameworkfake "sigs.k8s.io/descheduler/pkg/framework/fake"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
	"sigs.k8s.io/descheduler/pkg/utils"
	"sigs.k8s.io/descheduler/test"
)

type testCase struct {
	description             string
	pods                    []*v1.Pod
	nodes                   []*v1.Node
	namespaces              []*v1.Namespace
	pdbs                    []*policyv1.PodDisruptionBudget
	evictFailedBarePods     bool
	evictLocalStoragePods   bool
	evictSystemCriticalPods bool
	priorityThreshold       *int32
	nodeFit                 bool
	useNamespaceSelector    bool
	minReplicas             uint
	minPodAge               *metav1.Duration
	result                  bool
	ignorePodsWithoutPDB    bool
}

var namespace = "test"
var namespaceSelector = &metav1.LabelSelector{
	MatchLabels: map[string]string{
		"kubernetes.io/metadata.name": namespace,
	},
}

func TestDefaultEvictorPreEvictionFilter(t *testing.T) {
	n1 := test.BuildTestNode("node1", 1000, 2000, 13, nil)

	nodeTaintKey := "hardware"
	nodeTaintValue := "gpu"

	nodeLabelKey := "datacenter"
	nodeLabelValue := "east"

	testCases := []testCase{
		{
			description: "Pod with no tolerations running on normal node, all other nodes tainted",
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
				}),
			},
			nodes: []*v1.Node{
				test.BuildTestNode("node2", 1000, 2000, 13, func(node *v1.Node) {
					node.Spec.Taints = []v1.Taint{
						{
							Key:    nodeTaintKey,
							Value:  nodeTaintValue,
							Effect: v1.TaintEffectNoSchedule,
						},
					}
				}),
				test.BuildTestNode("node3", 1000, 2000, 13, func(node *v1.Node) {
					node.Spec.Taints = []v1.Taint{
						{
							Key:    nodeTaintKey,
							Value:  nodeTaintValue,
							Effect: v1.TaintEffectNoSchedule,
						},
					}
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			nodeFit:                 true,
			result:                  false,
		}, {
			description: "Pod with correct tolerations running on normal node, all other nodes tainted",
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.Spec.Tolerations = []v1.Toleration{
						{
							Key:    nodeTaintKey,
							Value:  nodeTaintValue,
							Effect: v1.TaintEffectNoSchedule,
						},
					}
				}),
			},
			nodes: []*v1.Node{
				test.BuildTestNode("node2", 1000, 2000, 13, func(node *v1.Node) {
					node.Spec.Taints = []v1.Taint{
						{
							Key:    nodeTaintKey,
							Value:  nodeTaintValue,
							Effect: v1.TaintEffectNoSchedule,
						},
					}
				}),
				test.BuildTestNode("node3", 1000, 2000, 13, func(node *v1.Node) {
					node.Spec.Taints = []v1.Taint{
						{
							Key:    nodeTaintKey,
							Value:  nodeTaintValue,
							Effect: v1.TaintEffectNoSchedule,
						},
					}
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			nodeFit:                 true,
			result:                  true,
		}, {
			description: "Pod with incorrect node selector",
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.Spec.NodeSelector = map[string]string{
						nodeLabelKey: "fail",
					}
				}),
			},
			nodes: []*v1.Node{
				test.BuildTestNode("node2", 1000, 2000, 13, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}),
				test.BuildTestNode("node3", 1000, 2000, 13, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			nodeFit:                 true,
			result:                  false,
		}, {
			description: "Pod with correct node selector",
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.Spec.NodeSelector = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}),
			},
			nodes: []*v1.Node{
				test.BuildTestNode("node2", 1000, 2000, 13, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}),
				test.BuildTestNode("node3", 1000, 2000, 13, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			nodeFit:                 true,
			result:                  true,
		}, {
			description: "Pod with correct node selector, but only available node doesn't have enough CPU",
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 12, 8, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.Spec.NodeSelector = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}),
			},
			nodes: []*v1.Node{
				test.BuildTestNode("node2-TEST", 10, 16, 10, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}),
				test.BuildTestNode("node3-TEST", 10, 16, 10, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			nodeFit:                 true,
			result:                  false,
		}, {
			description: "Pod with correct node selector, and one node has enough memory",
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 12, 8, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.Spec.NodeSelector = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}),
				test.BuildTestPod("node2-pod-10GB-mem", 20, 10, "node2", func(pod *v1.Pod) {
					pod.ObjectMeta.Labels = map[string]string{
						"test": "true",
					}
				}),
				test.BuildTestPod("node3-pod-10GB-mem", 20, 10, "node3", func(pod *v1.Pod) {
					pod.ObjectMeta.Labels = map[string]string{
						"test": "true",
					}
				}),
			},
			nodes: []*v1.Node{
				test.BuildTestNode("node2", 100, 16, 10, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}),
				test.BuildTestNode("node3", 100, 20, 10, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			nodeFit:                 true,
			result:                  true,
		}, {
			description: "Pod with correct node selector, but both nodes don't have enough memory",
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 12, 8, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.Spec.NodeSelector = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}),
				test.BuildTestPod("node2-pod-10GB-mem", 10, 10, "node2", func(pod *v1.Pod) {
					pod.ObjectMeta.Labels = map[string]string{
						"test": "true",
					}
				}),
				test.BuildTestPod("node3-pod-10GB-mem", 10, 10, "node3", func(pod *v1.Pod) {
					pod.ObjectMeta.Labels = map[string]string{
						"test": "true",
					}
				}),
			},
			nodes: []*v1.Node{
				test.BuildTestNode("node2", 100, 16, 10, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}),
				test.BuildTestNode("node3", 100, 16, 10, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			nodeFit:                 true,
			result:                  false,
		}, {
			description: "Pod with incorrect node selector, but nodefit false, should still be evicted",
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.Spec.NodeSelector = map[string]string{
						nodeLabelKey: "fail",
					}
				}),
			},
			nodes: []*v1.Node{
				test.BuildTestNode("node2", 1000, 2000, 13, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}),
				test.BuildTestNode("node3", 1000, 2000, 13, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			nodeFit:                 false,
			result:                  true,
		},
		{
			description: "Pod with namespace matched namespace selector, should be evicted",
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.Namespace = namespace
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.Spec.NodeSelector = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}),
			},
			nodes: []*v1.Node{
				test.BuildTestNode("node2", 1000, 2000, 13, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}),
				test.BuildTestNode("node3", 1000, 2000, 13, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}),
			},
			namespaces: []*v1.Namespace{
				test.BuildTestNamespace("default"),
				test.BuildTestNamespace(namespace),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			nodeFit:                 true,
			useNamespaceSelector:    true,
			result:                  true,
		},
		{
			description: "Pod with namespace does not matched namespace selector, should not be evicted",
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.Spec.NodeSelector = map[string]string{
						nodeLabelKey: "fail",
					}
				}),
			},
			nodes: []*v1.Node{
				test.BuildTestNode("node2", 1000, 2000, 13, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}),
				test.BuildTestNode("node3", 1000, 2000, 13, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						nodeLabelKey: nodeLabelValue,
					}
				}),
			},
			namespaces: []*v1.Namespace{
				test.BuildTestNamespace("default"),
				test.BuildTestNamespace(namespace),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			nodeFit:                 true,
			useNamespaceSelector:    true,
			result:                  false,
		},
	}

	for _, test := range testCases {
		t.Run(test.description, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			evictorPlugin, err := initializePlugin(ctx, test)
			if err != nil {
				t.Fatalf("Unable to initialize the plugin: %v", err)
			}

			result := evictorPlugin.(frameworktypes.EvictorPlugin).PreEvictionFilter(test.pods[0])
			if (result) != test.result {
				t.Errorf("Filter should return for pod %s %t, but it returns %t", test.pods[0].Name, test.result, result)
			}
		})
	}
}

func TestDefaultEvictorFilter(t *testing.T) {
	n1 := test.BuildTestNode("node1", 1000, 2000, 13, nil)
	lowPriority := int32(800)
	highPriority := int32(900)

	minPodAge := metav1.Duration{Duration: 50 * time.Minute}

	nodeTaintKey := "hardware"
	nodeTaintValue := "gpu"

	ownerRefUUID := uuid.NewUUID()

	testCases := []testCase{
		{
			description: "Failed pod eviction with no ownerRefs",
			pods: []*v1.Pod{
				test.BuildTestPod("bare_pod_failed", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.Status.Phase = v1.PodFailed
				}),
			},
			evictFailedBarePods: false,
			result:              false,
		}, {
			description:         "Normal pod eviction with no ownerRefs and evictFailedBarePods enabled",
			pods:                []*v1.Pod{test.BuildTestPod("bare_pod", 400, 0, n1.Name, nil)},
			evictFailedBarePods: true,
			result:              false,
		}, {
			description: "Failed pod eviction with no ownerRefs",
			pods: []*v1.Pod{
				test.BuildTestPod("bare_pod_failed_but_can_be_evicted", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.Status.Phase = v1.PodFailed
				}),
			},
			evictFailedBarePods: true,
			result:              true,
		}, {
			description: "Normal pod eviction with normal ownerRefs",
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  true,
		}, {
			description: "Normal pod eviction with normal ownerRefs and descheduler.alpha.kubernetes.io/evict annotation",
			pods: []*v1.Pod{
				test.BuildTestPod("p2", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.Annotations = map[string]string{"descheduler.alpha.kubernetes.io/evict": "true"}
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  true,
		}, {
			description: "Normal pod eviction with replicaSet ownerRefs",
			pods: []*v1.Pod{
				test.BuildTestPod("p3", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  true,
		}, {
			description: "Normal pod eviction with replicaSet ownerRefs and descheduler.alpha.kubernetes.io/evict annotation",
			pods: []*v1.Pod{
				test.BuildTestPod("p4", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.Annotations = map[string]string{"descheduler.alpha.kubernetes.io/evict": "true"}
					pod.ObjectMeta.OwnerReferences = test.GetReplicaSetOwnerRefList()
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  true,
		}, {
			description: "Normal pod eviction with statefulSet ownerRefs",
			pods: []*v1.Pod{
				test.BuildTestPod("p18", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  true,
		}, {
			description: "Normal pod eviction with statefulSet ownerRefs and descheduler.alpha.kubernetes.io/evict annotation",
			pods: []*v1.Pod{
				test.BuildTestPod("p19", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.Annotations = map[string]string{"descheduler.alpha.kubernetes.io/evict": "true"}
					pod.ObjectMeta.OwnerReferences = test.GetStatefulSetOwnerRefList()
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  true,
		}, {
			description: "Pod not evicted because it is bound to a PV and evictLocalStoragePods = false",
			pods: []*v1.Pod{
				test.BuildTestPod("p5", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
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
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  false,
		}, {
			description: "Pod is evicted because it is bound to a PV and evictLocalStoragePods = true",
			pods: []*v1.Pod{
				test.BuildTestPod("p6", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
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
				}),
			},
			evictLocalStoragePods:   true,
			evictSystemCriticalPods: false,
			result:                  true,
		}, {
			description: "Pod is evicted because it is bound to a PV and evictLocalStoragePods = false, but it has scheduler.alpha.kubernetes.io/evict annotation",
			pods: []*v1.Pod{
				test.BuildTestPod("p7", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.Annotations = map[string]string{"descheduler.alpha.kubernetes.io/evict": "true"}
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
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
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  true,
		}, {
			description: "Pod not evicted because it is part of a daemonSet",
			pods: []*v1.Pod{
				test.BuildTestPod("p8", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.ObjectMeta.OwnerReferences = test.GetDaemonSetOwnerRefList()
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  false,
		}, {
			description: "Pod is evicted because it is part of a daemonSet, but it has scheduler.alpha.kubernetes.io/evict annotation",
			pods: []*v1.Pod{
				test.BuildTestPod("p9", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.Annotations = map[string]string{"descheduler.alpha.kubernetes.io/evict": "true"}
					pod.ObjectMeta.OwnerReferences = test.GetDaemonSetOwnerRefList()
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  true,
		}, {
			description: "Pod not evicted because it is a mirror poddsa",
			pods: []*v1.Pod{
				test.BuildTestPod("p10", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.Annotations = test.GetMirrorPodAnnotation()
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  false,
		}, {
			description: "Pod is evicted because it is a mirror pod, but it has scheduler.alpha.kubernetes.io/evict annotation",
			pods: []*v1.Pod{
				test.BuildTestPod("p11", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.Annotations = test.GetMirrorPodAnnotation()
					pod.Annotations["descheduler.alpha.kubernetes.io/evict"] = "true"
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  true,
		}, {
			description: "Pod not evicted because it has system critical priority",
			pods: []*v1.Pod{
				test.BuildTestPod("p12", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					priority := utils.SystemCriticalPriority
					pod.Spec.Priority = &priority
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  false,
		}, {
			description: "Pod is evicted because it has system critical priority, but it has scheduler.alpha.kubernetes.io/evict annotation",
			pods: []*v1.Pod{
				test.BuildTestPod("p13", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					priority := utils.SystemCriticalPriority
					pod.Spec.Priority = &priority
					pod.Annotations = map[string]string{
						"descheduler.alpha.kubernetes.io/evict": "true",
					}
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			result:                  true,
		}, {
			description: "Pod not evicted because it has a priority higher than the configured priority threshold",
			pods: []*v1.Pod{
				test.BuildTestPod("p14", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.Spec.Priority = &highPriority
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			priorityThreshold:       &lowPriority,
			result:                  false,
		}, {
			description: "Pod is evicted because it has a priority higher than the configured priority threshold, but it has scheduler.alpha.kubernetes.io/evict annotation",
			pods: []*v1.Pod{
				test.BuildTestPod("p15", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.Annotations = map[string]string{"descheduler.alpha.kubernetes.io/evict": "true"}
					pod.Spec.Priority = &highPriority
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			priorityThreshold:       &lowPriority,
			result:                  true,
		}, {
			description: "Pod is evicted because it has system critical priority, but evictSystemCriticalPods = true",
			pods: []*v1.Pod{
				test.BuildTestPod("p16", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					priority := utils.SystemCriticalPriority
					pod.Spec.Priority = &priority
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: true,
			result:                  true,
		}, {
			description: "Pod is evicted because it has system critical priority, but evictSystemCriticalPods = true and it has scheduler.alpha.kubernetes.io/evict annotation",
			pods: []*v1.Pod{
				test.BuildTestPod("p16", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.Annotations = map[string]string{"descheduler.alpha.kubernetes.io/evict": "true"}
					priority := utils.SystemCriticalPriority
					pod.Spec.Priority = &priority
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: true,
			result:                  true,
		}, {
			description: "Pod is evicted because it has a priority higher than the configured priority threshold, but evictSystemCriticalPods = true",
			pods: []*v1.Pod{
				test.BuildTestPod("p17", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.Spec.Priority = &highPriority
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: true,
			priorityThreshold:       &lowPriority,
			result:                  true,
		}, {
			description: "Pod is evicted because it has a priority higher than the configured priority threshold, but evictSystemCriticalPods = true and it has scheduler.alpha.kubernetes.io/evict annotation",
			pods: []*v1.Pod{
				test.BuildTestPod("p17", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.Annotations = map[string]string{"descheduler.alpha.kubernetes.io/evict": "true"}
					pod.Spec.Priority = &highPriority
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: true,
			priorityThreshold:       &lowPriority,
			result:                  true,
		}, {
			description: "Pod with no tolerations running on normal node, all other nodes tainted, no PreEvictionFilter, should ignore nodeFit",
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 400, 0, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
				}),
			},
			nodes: []*v1.Node{
				test.BuildTestNode("node2", 1000, 2000, 13, func(node *v1.Node) {
					node.Spec.Taints = []v1.Taint{
						{
							Key:    nodeTaintKey,
							Value:  nodeTaintValue,
							Effect: v1.TaintEffectNoSchedule,
						},
					}
				}),
				test.BuildTestNode("node3", 1000, 2000, 13, func(node *v1.Node) {
					node.Spec.Taints = []v1.Taint{
						{
							Key:    nodeTaintKey,
							Value:  nodeTaintValue,
							Effect: v1.TaintEffectNoSchedule,
						},
					}
				}),
			},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			nodeFit:                 true,
			result:                  true,
		}, {
			description: "minReplicas of 2, owner with 2 replicas, evicts",
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 1, 1, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.ObjectMeta.OwnerReferences[0].UID = ownerRefUUID
				}),
				test.BuildTestPod("p2", 1, 1, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.ObjectMeta.OwnerReferences[0].UID = ownerRefUUID
				}),
			},
			minReplicas: 2,
			result:      true,
		}, {
			description: "minReplicas of 3, owner with 2 replicas, no eviction",
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 1, 1, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.ObjectMeta.OwnerReferences[0].UID = ownerRefUUID
				}),
				test.BuildTestPod("p2", 1, 1, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.ObjectMeta.OwnerReferences[0].UID = ownerRefUUID
				}),
			},
			minReplicas: 3,
			result:      false,
		}, {
			description: "minReplicas of 2, multiple owners, no eviction",
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 1, 1, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = append(test.GetNormalPodOwnerRefList(), test.GetNormalPodOwnerRefList()...)
					pod.ObjectMeta.OwnerReferences[0].UID = ownerRefUUID
				}),
				test.BuildTestPod("p2", 1, 1, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
				}),
			},
			minReplicas: 2,
			result:      true,
		}, {
			description: "minPodAge of 50, pod created 10 minutes ago, no eviction",
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 1, 1, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					podStartTime := metav1.Now().Add(time.Minute * time.Duration(-10))
					pod.Status.StartTime = &metav1.Time{Time: podStartTime}
				}),
			},
			minPodAge: &minPodAge,
			result:    false,
		}, {
			description: "minPodAge of 50, pod created 60 minutes ago, evicts",
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 1, 1, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					podStartTime := metav1.Now().Add(time.Minute * time.Duration(-60))
					pod.Status.StartTime = &metav1.Time{Time: podStartTime}
				}),
			},
			minPodAge: &minPodAge,
			result:    true,
		}, {
			description: "nil minPodAge, pod created 60 minutes ago, evicts",
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 1, 1, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					podStartTime := metav1.Now().Add(time.Minute * time.Duration(-60))
					pod.Status.StartTime = &metav1.Time{Time: podStartTime}
				}),
			},
			result: true,
		}, {
			description: "ignorePodsWithoutPDB, pod with no PDBs, no eviction",
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 1, 1, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.Labels = map[string]string{
						"app": "foo",
					}
				}),
			},
			ignorePodsWithoutPDB: true,
			result:               false,
		}, {
			description: "ignorePodsWithoutPDB, pod with PDBs, evicts",
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 1, 1, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
					pod.Labels = map[string]string{
						"app": "foo",
					}
				}),
			},
			pdbs: []*policyv1.PodDisruptionBudget{
				test.BuildTestPDB("pdb1", "foo"),
			},
			ignorePodsWithoutPDB: true,
			result:               true,
		},
	}

	for _, test := range testCases {
		t.Run(test.description, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			evictorPlugin, err := initializePlugin(ctx, test)
			if err != nil {
				t.Fatalf("Unable to initialize the plugin: %v", err)
			}

			result := evictorPlugin.(frameworktypes.EvictorPlugin).Filter(test.pods[0])
			if (result) != test.result {
				t.Errorf("Filter should return for pod %s %t, but it returns %t", test.pods[0].Name, test.result, result)
			}
		})
	}
}

func TestReinitialization(t *testing.T) {
	n1 := test.BuildTestNode("node1", 1000, 2000, 13, nil)
	ownerRefUUID := uuid.NewUUID()

	testCases := []testCase{
		{
			description: "minReplicas of 2, multiple owners, eviction",
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 1, 1, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = append(test.GetNormalPodOwnerRefList(), test.GetNormalPodOwnerRefList()...)
					pod.ObjectMeta.OwnerReferences[0].UID = ownerRefUUID
				}),
				test.BuildTestPod("p2", 1, 1, n1.Name, func(pod *v1.Pod) {
					pod.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
				}),
			},
			minReplicas: 2,
			result:      true,
		},
	}

	for _, test := range testCases {
		t.Run(test.description, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			evictorPlugin, err := initializePlugin(ctx, test)
			if err != nil {
				t.Fatalf("Unable to initialize the plugin: %v", err)
			}

			defaultEvictor, ok := evictorPlugin.(*DefaultEvictor)
			if !ok {
				t.Fatalf("Unable to initialize as a DefaultEvictor plugin")
			}
			_, err = New(defaultEvictor.args, defaultEvictor.handle)
			if err != nil {
				t.Fatalf("Unable to reinitialize the plugin: %v", err)
			}
		})
	}
}

func initializePlugin(ctx context.Context, test testCase) (frameworktypes.Plugin, error) {
	var objs []runtime.Object

	for _, node := range test.nodes {
		objs = append(objs, node)
	}

	for _, pod := range test.pods {
		objs = append(objs, pod)
	}
	for _, pdb := range test.pdbs {
		objs = append(objs, pdb)
	}

	for _, namespace := range test.namespaces {
		objs = append(objs, namespace)
	}

	fakeClient := fake.NewSimpleClientset(objs...)

	sharedInformerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
	podInformer := sharedInformerFactory.Core().V1().Pods().Informer()
	_ = sharedInformerFactory.Policy().V1().PodDisruptionBudgets().Lister()

	getPodsAssignedToNode, err := podutil.BuildGetPodsAssignedToNodeFunc(podInformer)
	if err != nil {
		return nil, fmt.Errorf("build get pods assigned to node function error: %v", err)
	}

	sharedInformerFactory.Start(ctx.Done())
	sharedInformerFactory.WaitForCacheSync(ctx.Done())

	defaultEvictorArgs := &DefaultEvictorArgs{
		EvictLocalStoragePods:   test.evictLocalStoragePods,
		EvictSystemCriticalPods: test.evictSystemCriticalPods,
		IgnorePvcPods:           false,
		EvictFailedBarePods:     test.evictFailedBarePods,
		PriorityThreshold: &api.PriorityThreshold{
			Value: test.priorityThreshold,
		},
		NodeFit:              test.nodeFit,
		MinReplicas:          test.minReplicas,
		MinPodAge:            test.minPodAge,
		IgnorePodsWithoutPDB: test.ignorePodsWithoutPDB,
	}

	if test.useNamespaceSelector {
		defaultEvictorArgs.NamespaceLabelSelector = namespaceSelector
	}

	evictorPlugin, err := New(
		defaultEvictorArgs,
		&frameworkfake.HandleImpl{
			ClientsetImpl:                 fakeClient,
			GetPodsAssignedToNodeFuncImpl: getPodsAssignedToNode,
			SharedInformerFactoryImpl:     sharedInformerFactory,
		})
	if err != nil {
		return nil, fmt.Errorf("unable to initialize the plugin: %v", err)
	}

	return evictorPlugin, nil
}
