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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	"sigs.k8s.io/descheduler/pkg/utils"
	"sigs.k8s.io/descheduler/test"
)

func TestHighNodeUtilization(t *testing.T) {
	ctx := context.Background()
	n1NodeName := "n1"
	n2NodeName := "n2"
	n3NodeName := "n3"

	testCases := []struct {
		name                         string
		thresholds, targetThresholds api.ResourceThresholds
		nodes                        map[string]*v1.Node
		pods                         map[string]*v1.PodList
		// TODO: divide expectedPodsEvicted into two params like other tests
		// expectedPodsEvicted should be the result num of pods that this testCase expected but now it represents both
		// MaxNoOfPodsToEvictPerNode and the test's expected result
		expectedPodsEvicted int
		evictedPods         []string
	}{
		{
			name: "no eviction - nodes above target thresholds",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:  80,
				v1.ResourcePods: 80,
			},
			targetThresholds: api.ResourceThresholds{
				v1.ResourceCPU:  20,
				v1.ResourcePods: 20,
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
					},
				},
				n2NodeName: {
					Items: []v1.Pod{
						*test.BuildTestPod("p9", 400, 0, n2NodeName, test.SetRSOwnerRef),
						*test.BuildTestPod("p10", 400, 0, n2NodeName, test.SetRSOwnerRef),
						*test.BuildTestPod("p11", 400, 0, n2NodeName, test.SetRSOwnerRef),
					},
				},
				n3NodeName: {},
			},
			expectedPodsEvicted: 0,
		},
		{
			name: "evict only evictable pods",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:  80,
				v1.ResourcePods: 80,
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
						*test.BuildTestPod("p3", 400, 0, n1NodeName, test.SetDSOwnerRef),
						*test.BuildTestPod("p4", 400, 0, n1NodeName, test.SetDSOwnerRef),
						*test.BuildTestPod("p5", 400, 0, n1NodeName, test.SetDSOwnerRef),
					},
				},
				n2NodeName: {
					Items: []v1.Pod{
						*test.BuildTestPod("p6", 400, 0, n2NodeName, test.SetRSOwnerRef),
						// This wont be evicted
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
						// This wont be evicted
						*test.BuildTestPod("p8", 400, 0, n1NodeName, func(pod *v1.Pod) {
							// A Critical Pod.
							pod.Namespace = "kube-system"
							priority := utils.SystemCriticalPriority
							pod.Spec.Priority = &priority
						}),
					},
				},
				n3NodeName: {},
			},
			expectedPodsEvicted: 1,
			evictedPods:         []string{"p6"},
		},
		{
			name: "without priorities stop when cpu capacity is depleted",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:  80,
				v1.ResourcePods: 80,
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
						*test.BuildTestPod("p6", 400, 300, n1NodeName, test.SetRSOwnerRef),
					},
				},
				n2NodeName: {
					Items: []v1.Pod{
						*test.BuildTestPod("p7", 400, 300, n2NodeName, test.SetRSOwnerRef),
						*test.BuildTestPod("p8", 400, 300, n2NodeName, test.SetRSOwnerRef),
						*test.BuildTestPod("p9", 400, 300, n2NodeName, test.SetRSOwnerRef),
						*test.BuildTestPod("p10", 400, 300, n2NodeName, test.SetRSOwnerRef),
					},
				},
				n3NodeName: {},
			},
			// 4 pods available for eviction based on v1.ResourcePods, only 2 pods can be evicted before cpu is depleted
			expectedPodsEvicted: 2,
			evictedPods:         []string{"p7", "p8"},
		},
		{
			name: "without priorities stop when memory capacity is depleted",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:  80,
				v1.ResourcePods: 80,
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
						*test.BuildTestPod("p6", 400, 300, n1NodeName, test.SetRSOwnerRef),
					},
				},
				n2NodeName: {
					Items: []v1.Pod{
						*test.BuildTestPod("p7", 400, 500, n2NodeName, test.SetRSOwnerRef),
						*test.BuildTestPod("p8", 400, 300, n2NodeName, test.SetRSOwnerRef),
						*test.BuildTestPod("p9", 400, 300, n2NodeName, test.SetRSOwnerRef),
						*test.BuildTestPod("p10", 400, 300, n2NodeName, test.SetRSOwnerRef),
					},
				},
				n3NodeName: {},
			},
			// 4 pods available for eviction based on v1.ResourcePods, only 1 pods can be evicted before cpu is depleted
			expectedPodsEvicted: 1,
			evictedPods:         []string{"p7"},
		},
		{
			name: "with priorities",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:  80,
				v1.ResourcePods: 80,
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
						*test.BuildTestPod("p1", 400, 0, n2NodeName, test.SetRSOwnerRef),
						*test.BuildTestPod("p2", 400, 0, n2NodeName, test.SetRSOwnerRef),
						*test.BuildTestPod("p3", 400, 0, n2NodeName, test.SetRSOwnerRef),
						*test.BuildTestPod("p4", 400, 0, n2NodeName, test.SetRSOwnerRef),
						*test.BuildTestPod("p5", 400, 0, n2NodeName, test.SetRSOwnerRef),
						*test.BuildTestPod("p6", 400, 0, n2NodeName, test.SetRSOwnerRef),
						*test.BuildTestPod("p7", 400, 0, n2NodeName, test.SetRSOwnerRef),
					},
				},
				n2NodeName: {
					Items: []v1.Pod{
						*test.BuildTestPod("p8", 400, 0, n1NodeName, func(pod *v1.Pod) {
							test.SetRSOwnerRef(pod)
							test.SetPodPriority(pod, int32(10000))
						}),
						*test.BuildTestPod("p9", 400, 0, n1NodeName, func(pod *v1.Pod) {
							test.SetRSOwnerRef(pod)
							test.SetPodPriority(pod, int32(0))
						}),
						//This wont be evicted
						*test.BuildTestPod("p10", 400, 0, n1NodeName, func(pod *v1.Pod) {
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
						//This wont be evicted
						*test.BuildTestPod("p11", 400, 0, n1NodeName, func(pod *v1.Pod) {
							// A Critical Pod.
							pod.Namespace = "kube-system"
							priority := utils.SystemCriticalPriority
							pod.Spec.Priority = &priority
						}),
					},
				},
				n3NodeName: {},
			},
			expectedPodsEvicted: 1,
			evictedPods:         []string{"p9"},
		},
		{
			name: "without priorities evicting best-effort pods only",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:  80,
				v1.ResourcePods: 80,
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
						*test.BuildTestPod("p1", 400, 0, n2NodeName, test.SetRSOwnerRef),
						*test.BuildTestPod("p2", 400, 0, n2NodeName, test.SetRSOwnerRef),
						*test.BuildTestPod("p3", 400, 0, n2NodeName, test.SetRSOwnerRef),
						*test.BuildTestPod("p4", 400, 0, n2NodeName, test.SetRSOwnerRef),
						*test.BuildTestPod("p5", 400, 0, n2NodeName, test.SetRSOwnerRef),
						*test.BuildTestPod("p6", 400, 0, n2NodeName, test.SetRSOwnerRef),
					},
				},
				n2NodeName: {
					Items: []v1.Pod{
						*test.BuildTestPod("p7", 400, 0, n1NodeName, func(pod *v1.Pod) {
							test.SetRSOwnerRef(pod)
							test.MakeBestEffortPod(pod)
						}),
						*test.BuildTestPod("p8", 400, 0, n1NodeName, func(pod *v1.Pod) {
							test.SetRSOwnerRef(pod)
							test.MakeBestEffortPod(pod)
						}),
						*test.BuildTestPod("p9", 400, 0, n1NodeName, func(pod *v1.Pod) {
							test.SetRSOwnerRef(pod)
							test.MakeBurstablePod(pod)
						}),
						//This wont be evicted
						*test.BuildTestPod("p10", 400, 0, n1NodeName, func(pod *v1.Pod) {
							// A Critical Pod.
							pod.Namespace = "kube-system"
							priority := utils.SystemCriticalPriority
							pod.Spec.Priority = &priority
						}),
					},
				},
				n3NodeName: {},
			},
			expectedPodsEvicted: 2,
			evictedPods:         []string{"p7", "p8"},
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
				test.expectedPodsEvicted,
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
			HighNodeUtilization(ctx, fakeClient, strategy, nodes, podEvictor)

			podsEvicted := podEvictor.TotalEvicted()
			if test.expectedPodsEvicted != podsEvicted {
				t.Errorf("Expected %#v pods to be evicted but %#v got evicted - %s", test.expectedPodsEvicted, podsEvicted, test.name)
			}
			if evictionFailed {
				t.Errorf("Pod evictions failed unexpectedly")
			}
		})
	}
}

func TestValidateHighNodeUtilizationStrategyConfig(t *testing.T) {
	tests := []struct {
		name             string
		thresholds       api.ResourceThresholds
		targetThresholds api.ResourceThresholds
		errInfo          error
		allow            bool
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
				v1.ResourceCPU:    80,
				v1.ResourceMemory: 20,
			},
			targetThresholds: api.ResourceThresholds{
				v1.ResourceCPU:  20,
				v1.ResourcePods: 80,
			},
			errInfo: fmt.Errorf("thresholds and targetThresholds configured different resources"),
		},
		{
			allow: true,
			name:  "thresholds' CPU config value is lesser than targetThresholds'",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:    50,
				v1.ResourceMemory: 20,
			},
			targetThresholds: api.ResourceThresholds{
				v1.ResourceCPU:    40,
				v1.ResourceMemory: 80,
			},
			errInfo: fmt.Errorf("thresholds' %v percentage is lesser than targetThresholds'", v1.ResourceMemory),
		},
		{
			name: "passing valid strategy config",
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:    80,
				v1.ResourceMemory: 80,
			},
			targetThresholds: api.ResourceThresholds{
				v1.ResourceCPU:    20,
				v1.ResourceMemory: 20,
			},
			errInfo: nil,
		},
	}

	for _, testCase := range tests {
		if !testCase.allow {
			continue
		}
		validateErr := validateHighNodeUtilizationStrategyConfig(testCase.thresholds, testCase.targetThresholds)

		if validateErr == nil || testCase.errInfo == nil {
			if validateErr != testCase.errInfo {
				t.Errorf("expected validity of strategy config: thresholds %#v targetThresholds %#v\nto be %v but got %v instead",
					testCase.thresholds, testCase.targetThresholds, testCase.errInfo, validateErr)
			}
		} else if validateErr.Error() != testCase.errInfo.Error() {
			t.Errorf("expected validity of strategy config: thresholds %#v targetThresholds %#v\nto be %v but got %v instead",
				testCase.thresholds, testCase.targetThresholds, testCase.errInfo, validateErr)
		}
	}
}
