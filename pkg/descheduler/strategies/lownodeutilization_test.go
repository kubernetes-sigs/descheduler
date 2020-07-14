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

package strategies

import (
	"context"
	"fmt"
	"strings"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/api/policy/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	"sigs.k8s.io/descheduler/pkg/utils"
	"sigs.k8s.io/descheduler/test"
)

func TestLowNodeUtilization(t *testing.T) {
	ctx := context.Background()
	n1NodeName := "n1"
	n2NodeName := "n2"
	n3NodeName := "n3"

	testCases := []struct {
		name                         string
		thresholds, targetThresholds api.ResourceThresholds
		nodes                        map[string]*v1.Node
		pods                         map[string]*v1.PodList
		maxPodsToEvictPerNode        int
		expectedPodsEvicted          int
		evictedPods                  []string
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
			nodes: map[string]*v1.Node{
				n1NodeName: test.BuildTestNode(n1NodeName, 4000, 3000, 9, nil),
				n2NodeName: test.BuildTestNode(n2NodeName, 4000, 3000, 10, nil),
				n3NodeName: test.BuildTestNode(n3NodeName, 4000, 3000, 10, test.SetNodeUnschedulable),
			},
			pods: map[string]*v1.PodList{
				n1NodeName: {
					Items: []v1.Pod{
						// These won't be evicted.
						*test.BuildTestPod("p1", 400, 0, n1NodeName, test.SetDSOwnerRef),
						*test.BuildTestPod("p2", 400, 0, n1NodeName, test.SetDSOwnerRef),
						*test.BuildTestPod("p3", 400, 0, n1NodeName, test.SetDSOwnerRef),
						*test.BuildTestPod("p4", 400, 0, n1NodeName, test.SetDSOwnerRef),
						*test.BuildTestPod("p5", 400, 0, n1NodeName, func(pod *v1.Pod) {
							// A pod with local storage.
							test.SetNormalOwnerRef(pod)
							pod.Spec.Volumes = []v1.Volume{
								{
									Name: "sample",
									VolumeSource: v1.VolumeSource{
										HostPath: &v1.HostPathVolumeSource{Path: "somePath"},
										EmptyDir: &v1.EmptyDirVolumeSource{
											SizeLimit: resource.NewQuantity(int64(10), resource.BinarySI)},
									},
								},
							}
							// A Mirror Pod.
							pod.Annotations = test.GetMirrorPodAnnotation()
						}),
						*test.BuildTestPod("p6", 400, 0, n1NodeName, func(pod *v1.Pod) {
							// A Critical Pod.
							pod.Namespace = "kube-system"
							priority := utils.SystemCriticalPriority
							pod.Spec.Priority = &priority
						}),
					},
				},
				n2NodeName: {
					Items: []v1.Pod{
						*test.BuildTestPod("p9", 400, 0, n2NodeName, test.SetRSOwnerRef),
					},
				},
				n3NodeName: {},
			},
			maxPodsToEvictPerNode: 0,
			expectedPodsEvicted:   0,
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
			nodes: map[string]*v1.Node{
				n1NodeName: test.BuildTestNode(n1NodeName, 4000, 3000, 9, nil),
				n2NodeName: test.BuildTestNode(n2NodeName, 4000, 3000, 10, nil),
				n3NodeName: test.BuildTestNode(n3NodeName, 4000, 3000, 10, test.SetNodeUnschedulable),
			},
			pods: map[string]*v1.PodList{
				n1NodeName: {
					Items: []v1.Pod{
						*test.BuildTestPod("p1", 400, 0, n1NodeName, test.SetRSOwnerRef),
						*test.BuildTestPod("p2", 400, 0, n1NodeName, test.SetRSOwnerRef),
						*test.BuildTestPod("p3", 400, 0, n1NodeName, test.SetRSOwnerRef),
						*test.BuildTestPod("p4", 400, 0, n1NodeName, test.SetRSOwnerRef),
						*test.BuildTestPod("p5", 400, 0, n1NodeName, test.SetRSOwnerRef),
						// These won't be evicted.
						*test.BuildTestPod("p6", 400, 0, n1NodeName, test.SetDSOwnerRef),
						*test.BuildTestPod("p7", 400, 0, n1NodeName, func(pod *v1.Pod) {
							// A pod with local storage.
							test.SetNormalOwnerRef(pod)
							pod.Spec.Volumes = []v1.Volume{
								{
									Name: "sample",
									VolumeSource: v1.VolumeSource{
										HostPath: &v1.HostPathVolumeSource{Path: "somePath"},
										EmptyDir: &v1.EmptyDirVolumeSource{
											SizeLimit: resource.NewQuantity(int64(10), resource.BinarySI)},
									},
								},
							}
							// A Mirror Pod.
							pod.Annotations = test.GetMirrorPodAnnotation()
						}),
						*test.BuildTestPod("p8", 400, 0, n1NodeName, func(pod *v1.Pod) {
							// A Critical Pod.
							pod.Namespace = "kube-system"
							priority := utils.SystemCriticalPriority
							pod.Spec.Priority = &priority
						}),
					},
				},
				n2NodeName: {
					Items: []v1.Pod{
						*test.BuildTestPod("p9", 400, 0, n2NodeName, test.SetRSOwnerRef),
					},
				},
				n3NodeName: {},
			},
			maxPodsToEvictPerNode: 0,
			expectedPodsEvicted:   4,
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
			nodes: map[string]*v1.Node{
				n1NodeName: test.BuildTestNode(n1NodeName, 4000, 3000, 9, nil),
				n2NodeName: test.BuildTestNode(n2NodeName, 4000, 3000, 10, nil),
				n3NodeName: test.BuildTestNode(n3NodeName, 4000, 3000, 10, test.SetNodeUnschedulable),
			},
			pods: map[string]*v1.PodList{
				n1NodeName: {
					Items: []v1.Pod{
						*test.BuildTestPod("p1", 400, 300, n1NodeName, test.SetRSOwnerRef),
						*test.BuildTestPod("p2", 400, 300, n1NodeName, test.SetRSOwnerRef),
						*test.BuildTestPod("p3", 400, 300, n1NodeName, test.SetRSOwnerRef),
						*test.BuildTestPod("p4", 400, 300, n1NodeName, test.SetRSOwnerRef),
						*test.BuildTestPod("p5", 400, 300, n1NodeName, test.SetRSOwnerRef),
						// These won't be evicted.
						*test.BuildTestPod("p6", 400, 300, n1NodeName, test.SetDSOwnerRef),
						*test.BuildTestPod("p7", 400, 300, n1NodeName, func(pod *v1.Pod) {
							// A pod with local storage.
							test.SetNormalOwnerRef(pod)
							pod.Spec.Volumes = []v1.Volume{
								{
									Name: "sample",
									VolumeSource: v1.VolumeSource{
										HostPath: &v1.HostPathVolumeSource{Path: "somePath"},
										EmptyDir: &v1.EmptyDirVolumeSource{
											SizeLimit: resource.NewQuantity(int64(10), resource.BinarySI)},
									},
								},
							}
							// A Mirror Pod.
							pod.Annotations = test.GetMirrorPodAnnotation()
						}),
						*test.BuildTestPod("p8", 400, 300, n1NodeName, func(pod *v1.Pod) {
							// A Critical Pod.
							pod.Namespace = "kube-system"
							priority := utils.SystemCriticalPriority
							pod.Spec.Priority = &priority
						}),
					},
				},
				n2NodeName: {
					Items: []v1.Pod{
						*test.BuildTestPod("p9", 400, 2100, n2NodeName, test.SetRSOwnerRef),
					},
				},
				n3NodeName: {},
			},
			maxPodsToEvictPerNode: 0,
			// 4 pods available for eviction based on v1.ResourcePods, only 3 pods can be evicted before cpu is depleted
			expectedPodsEvicted: 3,
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
			nodes: map[string]*v1.Node{
				n1NodeName: test.BuildTestNode(n1NodeName, 4000, 3000, 9, nil),
				n2NodeName: test.BuildTestNode(n2NodeName, 4000, 3000, 10, nil),
				n3NodeName: test.BuildTestNode(n3NodeName, 4000, 3000, 10, test.SetNodeUnschedulable),
			},
			pods: map[string]*v1.PodList{
				n1NodeName: {
					Items: []v1.Pod{
						*test.BuildTestPod("p1", 400, 0, n1NodeName, func(pod *v1.Pod) {
							test.SetRSOwnerRef(pod)
							test.SetPodPriority(pod, int32(10000))
						}),
						*test.BuildTestPod("p2", 400, 0, n1NodeName, func(pod *v1.Pod) {
							test.SetRSOwnerRef(pod)
							test.SetPodPriority(pod, int32(10000))
						}),
						*test.BuildTestPod("p3", 400, 0, n1NodeName, func(pod *v1.Pod) {
							test.SetRSOwnerRef(pod)
							test.SetPodPriority(pod, int32(10000))
						}),
						*test.BuildTestPod("p4", 400, 0, n1NodeName, func(pod *v1.Pod) {
							test.SetRSOwnerRef(pod)
							test.SetPodPriority(pod, int32(10000))
						}),
						*test.BuildTestPod("p5", 400, 0, n1NodeName, func(pod *v1.Pod) {
							test.SetRSOwnerRef(pod)
							test.SetPodPriority(pod, int32(0))
						}),
						// These won't be evicted.
						*test.BuildTestPod("p6", 400, 0, n1NodeName, func(pod *v1.Pod) {
							test.SetDSOwnerRef(pod)
							test.SetPodPriority(pod, int32(10000))
						}),
						*test.BuildTestPod("p7", 400, 0, n1NodeName, func(pod *v1.Pod) {
							// A pod with local storage.
							test.SetNormalOwnerRef(pod)
							test.SetPodPriority(pod, int32(0))
							pod.Spec.Volumes = []v1.Volume{
								{
									Name: "sample",
									VolumeSource: v1.VolumeSource{
										HostPath: &v1.HostPathVolumeSource{Path: "somePath"},
										EmptyDir: &v1.EmptyDirVolumeSource{
											SizeLimit: resource.NewQuantity(int64(10), resource.BinarySI)},
									},
								},
							}
							// A Mirror Pod.
							pod.Annotations = test.GetMirrorPodAnnotation()
						}),
						*test.BuildTestPod("p8", 400, 0, n1NodeName, func(pod *v1.Pod) {
							// A Critical Pod.
							pod.Namespace = "kube-system"
							priority := utils.SystemCriticalPriority
							pod.Spec.Priority = &priority
						}),
					},
				},
				n2NodeName: {
					Items: []v1.Pod{
						*test.BuildTestPod("p9", 400, 0, n2NodeName, test.SetRSOwnerRef),
					},
				},
				n3NodeName: {},
			},
			maxPodsToEvictPerNode: 0,
			expectedPodsEvicted:   4,
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
			nodes: map[string]*v1.Node{
				n1NodeName: test.BuildTestNode(n1NodeName, 4000, 3000, 9, nil),
				n2NodeName: test.BuildTestNode(n2NodeName, 4000, 3000, 10, nil),
				n3NodeName: test.BuildTestNode(n3NodeName, 4000, 3000, 10, test.SetNodeUnschedulable),
			},
			// All pods are assumed to be burstable (test.BuildTestNode always sets both cpu/memory resource requests to some value)
			pods: map[string]*v1.PodList{
				n1NodeName: {
					Items: []v1.Pod{
						*test.BuildTestPod("p1", 400, 0, n1NodeName, func(pod *v1.Pod) {
							test.SetRSOwnerRef(pod)
							test.MakeBestEffortPod(pod)
						}),
						*test.BuildTestPod("p2", 400, 0, n1NodeName, func(pod *v1.Pod) {
							test.SetRSOwnerRef(pod)
							test.MakeBestEffortPod(pod)
						}),
						*test.BuildTestPod("p3", 400, 0, n1NodeName, func(pod *v1.Pod) {
							test.SetRSOwnerRef(pod)
						}),
						*test.BuildTestPod("p4", 400, 0, n1NodeName, func(pod *v1.Pod) {
							test.SetRSOwnerRef(pod)
							test.MakeBestEffortPod(pod)
						}),
						*test.BuildTestPod("p5", 400, 0, n1NodeName, func(pod *v1.Pod) {
							test.SetRSOwnerRef(pod)
							test.MakeBestEffortPod(pod)
						}),
						// These won't be evicted.
						*test.BuildTestPod("p6", 400, 0, n1NodeName, func(pod *v1.Pod) {
							test.SetDSOwnerRef(pod)
						}),
						*test.BuildTestPod("p7", 400, 0, n1NodeName, func(pod *v1.Pod) {
							// A pod with local storage.
							test.SetNormalOwnerRef(pod)
							pod.Spec.Volumes = []v1.Volume{
								{
									Name: "sample",
									VolumeSource: v1.VolumeSource{
										HostPath: &v1.HostPathVolumeSource{Path: "somePath"},
										EmptyDir: &v1.EmptyDirVolumeSource{
											SizeLimit: resource.NewQuantity(int64(10), resource.BinarySI)},
									},
								},
							}
							// A Mirror Pod.
							pod.Annotations = test.GetMirrorPodAnnotation()
						}),
						*test.BuildTestPod("p8", 400, 0, n1NodeName, func(pod *v1.Pod) {
							// A Critical Pod.
							pod.Namespace = "kube-system"
							priority := utils.SystemCriticalPriority
							pod.Spec.Priority = &priority
						}),
					},
				},
				n2NodeName: {
					Items: []v1.Pod{
						*test.BuildTestPod("p9", 400, 0, n2NodeName, test.SetRSOwnerRef),
					},
				},
				n3NodeName: {},
			},
			maxPodsToEvictPerNode: 0,
			expectedPodsEvicted:   4,
			evictedPods:           []string{"p1", "p2", "p4", "p5"},
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			fakeClient := &fake.Clientset{}
			fakeClient.Fake.AddReactor("list", "pods", func(action core.Action) (bool, runtime.Object, error) {
				list := action.(core.ListAction)
				fieldString := list.GetListRestrictions().Fields.String()
				if strings.Contains(fieldString, n1NodeName) {
					return true, test.pods[n1NodeName], nil
				}
				if strings.Contains(fieldString, n2NodeName) {
					return true, test.pods[n2NodeName], nil
				}
				if strings.Contains(fieldString, n3NodeName) {
					return true, test.pods[n3NodeName], nil
				}
				return true, nil, fmt.Errorf("Failed to list: %v", list)
			})
			fakeClient.Fake.AddReactor("get", "nodes", func(action core.Action) (bool, runtime.Object, error) {
				getAction := action.(core.GetAction)
				if node, exists := test.nodes[getAction.GetName()]; exists {
					return true, node, nil
				}
				return true, nil, fmt.Errorf("Wrong node: %v", getAction.GetName())
			})
			podsForEviction := make(map[string]struct{})
			for _, pod := range test.evictedPods {
				podsForEviction[pod] = struct{}{}
			}

			evictionFailed := false
			if len(test.evictedPods) > 0 {
				fakeClient.Fake.AddReactor("create", "pods", func(action core.Action) (bool, runtime.Object, error) {
					getAction := action.(core.CreateAction)
					obj := getAction.GetObject()
					if eviction, ok := obj.(*v1beta1.Eviction); ok {
						if _, exists := podsForEviction[eviction.Name]; exists {
							return true, obj, nil
						}
						evictionFailed = true
						return true, nil, fmt.Errorf("pod %q was unexpectedly evicted", eviction.Name)
					}
					return true, obj, nil
				})
			}

			var nodes []*v1.Node
			for _, node := range test.nodes {
				nodes = append(nodes, node)
			}

			podEvictor := evictions.NewPodEvictor(
				fakeClient,
				"v1",
				false,
				test.maxPodsToEvictPerNode,
				nodes,
				false,
			)

			strategy := api.DeschedulerStrategy{
				Enabled: true,
				Params: &api.StrategyParameters{
					NodeResourceUtilizationThresholds: &api.NodeResourceUtilizationThresholds{
						Thresholds:       test.thresholds,
						TargetThresholds: test.targetThresholds,
					},
				},
			}
			LowNodeUtilization(ctx, fakeClient, strategy, nodes, podEvictor)

			podsEvicted := podEvictor.TotalEvicted()
			if test.expectedPodsEvicted != podsEvicted {
				t.Errorf("Expected %#v pods to be evicted but %#v got evicted", test.expectedPodsEvicted, podsEvicted)
			}
			if evictionFailed {
				t.Errorf("Pod evictions failed unexpectedly")
			}
		})
	}
}

func TestLowNodeUtilizationValidateStrategyConfig(t *testing.T) {
	tests := []struct {
		name             string
		thresholds       api.ResourceThresholds
		targetThresholds api.ResourceThresholds
		errInfo          error
	}{
		{
			name: "passing invalid thresholds",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:    20,
				v1.ResourceMemory: 120,
			},
			targetThresholds: api.ResourceThresholds{
				v1.ResourceCPU:    80,
				v1.ResourceMemory: 80,
			},
			errInfo: fmt.Errorf("thresholds config is not valid: %v", fmt.Errorf(
				"%v threshold not in [%v, %v] range", v1.ResourceMemory, MinResourcePercentage, MaxResourcePercentage)),
		},
		{
			name: "passing invalid targetThresholds",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:    20,
				v1.ResourceMemory: 20,
			},
			targetThresholds: api.ResourceThresholds{
				v1.ResourceCPU:    80,
				"resourceInvalid": 80,
			},
			errInfo: fmt.Errorf("targetThresholds config is not valid: %v",
				fmt.Errorf("only cpu, memory, or pods thresholds can be specified")),
		},
		{
			name: "thresholds and targetThresholds configured different num of resources",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:    20,
				v1.ResourceMemory: 20,
			},
			targetThresholds: api.ResourceThresholds{
				v1.ResourceCPU:    80,
				v1.ResourceMemory: 80,
				v1.ResourcePods:   80,
			},
			errInfo: fmt.Errorf("thresholds and targetThresholds configured different resources"),
		},
		{
			name: "thresholds and targetThresholds configured different resources",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:    20,
				v1.ResourceMemory: 20,
			},
			targetThresholds: api.ResourceThresholds{
				v1.ResourceCPU:  80,
				v1.ResourcePods: 80,
			},
			errInfo: fmt.Errorf("thresholds and targetThresholds configured different resources"),
		},
		{
			name: "thresholds' CPU config value is greater than targetThresholds'",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:    90,
				v1.ResourceMemory: 20,
			},
			targetThresholds: api.ResourceThresholds{
				v1.ResourceCPU:    80,
				v1.ResourceMemory: 80,
			},
			errInfo: fmt.Errorf("thresholds' %v percentage is greater than targetThresholds'", v1.ResourceCPU),
		},
		{
			name: "passing valid strategy config",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:    20,
				v1.ResourceMemory: 20,
			},
			targetThresholds: api.ResourceThresholds{
				v1.ResourceCPU:    80,
				v1.ResourceMemory: 80,
			},
			errInfo: nil,
		},
	}

	for _, testCase := range tests {
		validateErr := validateLowNodeUtilizationStrategyConfig(testCase.thresholds, testCase.targetThresholds)

		if validateErr == nil || testCase.errInfo == nil {
			if validateErr != testCase.errInfo {
				t.Errorf("expected validity of strategy config: thresholds %#v targetThresholds %#v to be %v but got %v instead",
					testCase.thresholds, testCase.targetThresholds, testCase.errInfo, validateErr)
			}
		} else if validateErr.Error() != testCase.errInfo.Error() {
			t.Errorf("expected validity of strategy config: thresholds %#v targetThresholds %#v to be %v but got %v instead",
				testCase.thresholds, testCase.targetThresholds, testCase.errInfo, validateErr)
		}
	}
}

func TestValidateThresholds(t *testing.T) {
	tests := []struct {
		name    string
		input   api.ResourceThresholds
		errInfo error
	}{
		{
			name:    "passing nil map for threshold",
			input:   nil,
			errInfo: fmt.Errorf("no resource threshold is configured"),
		},
		{
			name:    "passing no threshold",
			input:   api.ResourceThresholds{},
			errInfo: fmt.Errorf("no resource threshold is configured"),
		},
		{
			name: "passing unsupported resource name",
			input: api.ResourceThresholds{
				v1.ResourceCPU:     40,
				v1.ResourceStorage: 25.5,
			},
			errInfo: fmt.Errorf("only cpu, memory, or pods thresholds can be specified"),
		},
		{
			name: "passing invalid resource name",
			input: api.ResourceThresholds{
				v1.ResourceCPU: 40,
				"coolResource": 42.0,
			},
			errInfo: fmt.Errorf("only cpu, memory, or pods thresholds can be specified"),
		},
		{
			name: "passing invalid resource value",
			input: api.ResourceThresholds{
				v1.ResourceCPU:    110,
				v1.ResourceMemory: 80,
			},
			errInfo: fmt.Errorf("%v threshold not in [%v, %v] range", v1.ResourceCPU, MinResourcePercentage, MaxResourcePercentage),
		},
		{
			name: "passing a valid threshold with max and min resource value",
			input: api.ResourceThresholds{
				v1.ResourceCPU:    100,
				v1.ResourceMemory: 0,
			},
			errInfo: nil,
		},
		{
			name: "passing a valid threshold with only cpu",
			input: api.ResourceThresholds{
				v1.ResourceCPU: 80,
			},
			errInfo: nil,
		},
		{
			name: "passing a valid threshold with cpu, memory and pods",
			input: api.ResourceThresholds{
				v1.ResourceCPU:    20,
				v1.ResourceMemory: 30,
				v1.ResourcePods:   40,
			},
			errInfo: nil,
		},
	}

	for _, test := range tests {
		validateErr := validateThresholds(test.input)

		if validateErr == nil || test.errInfo == nil {
			if validateErr != test.errInfo {
				t.Errorf("expected validity of threshold: %#v to be %v but got %v instead", test.input, test.errInfo, validateErr)
			}
		} else if validateErr.Error() != test.errInfo.Error() {
			t.Errorf("expected validity of threshold: %#v to be %v but got %v instead", test.input, test.errInfo, validateErr)
		}
	}
}

func newFake(objects ...runtime.Object) *core.Fake {
	scheme := runtime.NewScheme()
	codecs := serializer.NewCodecFactory(scheme)
	fake.AddToScheme(scheme)
	o := core.NewObjectTracker(scheme, codecs.UniversalDecoder())
	for _, obj := range objects {
		if err := o.Add(obj); err != nil {
			panic(err)
		}
	}

	fakePtr := core.Fake{}
	fakePtr.AddReactor("list", "pods", func(action core.Action) (bool, runtime.Object, error) {
		objs, err := o.List(
			schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
			schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
			action.GetNamespace(),
		)
		if err != nil {
			return true, nil, err
		}

		obj := &v1.PodList{
			Items: []v1.Pod{},
		}
		for _, pod := range objs.(*v1.PodList).Items {
			podFieldSet := fields.Set(map[string]string{
				"spec.nodeName": pod.Spec.NodeName,
				"status.phase":  string(pod.Status.Phase),
			})
			match := action.(core.ListAction).GetListRestrictions().Fields.Matches(podFieldSet)
			if !match {
				continue
			}
			obj.Items = append(obj.Items, *pod.DeepCopy())
		}
		return true, obj, nil
	})
	fakePtr.AddReactor("*", "*", core.ObjectReaction(o))
	fakePtr.AddWatchReactor("*", core.DefaultWatchReactor(watch.NewFake(), nil))

	return &fakePtr
}

func TestWithTaints(t *testing.T) {
	ctx := context.Background()
	strategy := api.DeschedulerStrategy{
		Enabled: true,
		Params: &api.StrategyParameters{
			NodeResourceUtilizationThresholds: &api.NodeResourceUtilizationThresholds{
				Thresholds: api.ResourceThresholds{
					v1.ResourcePods: 20,
				},
				TargetThresholds: api.ResourceThresholds{
					v1.ResourcePods: 70,
				},
			},
		},
	}

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

	tests := []struct {
		name              string
		nodes             []*v1.Node
		pods              []*v1.Pod
		evictionsExpected int
	}{
		{
			name:  "No taints",
			nodes: []*v1.Node{n1, n2, n3},
			pods: []*v1.Pod{
				//Node 1 pods
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
				//Node 1 pods
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
				//Node 1 pods
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

			fakePtr := newFake(objs...)
			var evictionCounter int
			fakePtr.PrependReactor("create", "pods", func(action core.Action) (bool, runtime.Object, error) {
				if action.GetSubresource() != "eviction" || action.GetResource().Resource != "pods" {
					return false, nil, nil
				}
				evictionCounter++
				return true, nil, nil
			})

			podEvictor := evictions.NewPodEvictor(
				&fake.Clientset{Fake: *fakePtr},
				"policy/v1",
				false,
				item.evictionsExpected,
				item.nodes,
				false,
			)

			LowNodeUtilization(ctx, &fake.Clientset{Fake: *fakePtr}, strategy, item.nodes, podEvictor)

			if item.evictionsExpected != evictionCounter {
				t.Errorf("Expected %v evictions, got %v", item.evictionsExpected, evictionCounter)
			}
		})
	}
}
