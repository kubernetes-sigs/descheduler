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
	"reflect"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/events"
	"k8s.io/component-base/featuregate"
	"k8s.io/klog/v2"
	utilptr "k8s.io/utils/ptr"

	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/features"
	"sigs.k8s.io/descheduler/pkg/utils"
	"sigs.k8s.io/descheduler/test"
)

const (
	notFoundText    = "pod not found when evicting \"%s\": pods \"%s\" not found"
	tooManyRequests = "error when evicting pod (ignoring) \"%s\": Too many requests: too many requests"
)

func initFeatureGates() featuregate.FeatureGate {
	featureGates := featuregate.NewFeatureGate()
	featureGates.Add(map[featuregate.Feature]featuregate.FeatureSpec{
		features.EvictionsInBackground: {Default: true, PreRelease: featuregate.Alpha},
	})
	return featureGates
}

func TestEvictPod(t *testing.T) {
	node1 := test.BuildTestNode("node1", 1000, 2000, 9, nil)
	pod1 := test.BuildTestPod("p1", 400, 0, "node1", nil)
	tests := []struct {
		description string
		node        *v1.Node
		evictedPod  *v1.Pod
		pods        []runtime.Object
		wantErr     error
	}{
		{
			description: "test pod eviction - pod present",
			node:        node1,
			evictedPod:  pod1,
			pods:        []runtime.Object{pod1},
		},
		{
			description: "test pod eviction - pod absent (not found error)",
			node:        node1,
			evictedPod:  pod1,
			pods:        []runtime.Object{test.BuildTestPod("p2", 400, 0, "node1", nil), test.BuildTestPod("p3", 450, 0, "node1", nil)},
			wantErr:     fmt.Errorf(notFoundText, pod1.Name, pod1.Name),
		},
		{
			description: "test pod eviction - pod absent (too many requests error)",
			node:        node1,
			evictedPod:  pod1,
			pods:        []runtime.Object{test.BuildTestPod("p2", 400, 0, "node1", nil), test.BuildTestPod("p3", 450, 0, "node1", nil)},
			wantErr:     fmt.Errorf(tooManyRequests, pod1.Name),
		},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			ctx := context.Background()
			fakeClient := fake.NewClientset(test.pods...)
			fakeClient.PrependReactor("create", "pods/eviction", func(action core.Action) (handled bool, ret runtime.Object, err error) {
				return true, nil, test.wantErr
			})
			sharedInformerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
			sharedInformerFactory.Start(ctx.Done())
			sharedInformerFactory.WaitForCacheSync(ctx.Done())

			eventRecorder := &events.FakeRecorder{}
			podEvictor, err := NewPodEvictor(
				ctx,
				fakeClient,
				eventRecorder,
				sharedInformerFactory.Core().V1().Pods().Informer(),
				initFeatureGates(),
				NewOptions(),
			)
			if err != nil {
				t.Fatalf("Unexpected error when creating a pod evictor: %v", err)
			}

			_, got := podEvictor.evictPod(ctx, test.evictedPod)
			if got != test.wantErr {
				t.Errorf("Test error for Desc: %s. Expected %v pod eviction to be %v, got %v", test.description, test.evictedPod.Name, test.wantErr, got)
			}
		})
	}
}

func TestPodTypes(t *testing.T) {
	n1 := test.BuildTestNode("node1", 1000, 2000, 9, nil)
	p1 := test.BuildTestPod("p1", 400, 0, n1.Name, nil)

	// These won't be evicted.
	p2 := test.BuildTestPod("p2", 400, 0, n1.Name, nil)
	p3 := test.BuildTestPod("p3", 400, 0, n1.Name, nil)
	p4 := test.BuildTestPod("p4", 400, 0, n1.Name, nil)

	p1.ObjectMeta.OwnerReferences = test.GetReplicaSetOwnerRefList()
	// The following 4 pods won't get evicted.
	// A daemonset.
	// p2.Annotations = test.GetDaemonSetAnnotation()
	p2.ObjectMeta.OwnerReferences = test.GetDaemonSetOwnerRefList()
	// A pod with local storage.
	p3.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
	p3.Spec.Volumes = []v1.Volume{
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
	p4.Annotations = test.GetMirrorPodAnnotation()
	if !utils.IsMirrorPod(p4) {
		t.Errorf("Expected p4 to be a mirror pod.")
	}
	if !utils.IsPodWithLocalStorage(p3) {
		t.Errorf("Expected p3 to be a pod with local storage.")
	}
	ownerRefList := podutil.OwnerRef(p2)
	if !utils.IsDaemonsetPod(ownerRefList) {
		t.Errorf("Expected p2 to be a daemonset pod.")
	}
	ownerRefList = podutil.OwnerRef(p1)
	if utils.IsDaemonsetPod(ownerRefList) || utils.IsPodWithLocalStorage(p1) || utils.IsCriticalPriorityPod(p1) || utils.IsMirrorPod(p1) || utils.IsStaticPod(p1) {
		t.Errorf("Expected p1 to be a normal pod.")
	}
}

func TestNewPodEvictor(t *testing.T) {
	ctx := context.Background()

	pod1 := test.BuildTestPod("pod", 400, 0, "node", nil)
	type podEvictorTest struct {
		description                      string
		pod                              *v1.Pod
		dryRun                           bool
		evictionFailureEventNotification *bool
		maxPodsToEvictTotal              *uint
		maxPodsToEvictPerNode            *uint
		maxPodsToEvictPerNamespace       *uint
		expectedNodeEvictions            uint
		expectedTotalEvictions           uint
		expectedError                    error
		// expectedEvent is a slice of strings representing expected events.
		// Each string in the slice should follow the format: "EventType Reason Message".
		// - "Warning Failed processing failed"
		events []string
	}
	tests := []podEvictorTest{
		{
			description:                      "one eviction expected with eviction failure event notification",
			pod:                              pod1,
			evictionFailureEventNotification: utilptr.To[bool](true),
			maxPodsToEvictTotal:              utilptr.To[uint](1),
			maxPodsToEvictPerNode:            utilptr.To[uint](1),
			maxPodsToEvictPerNamespace:       utilptr.To[uint](1),
			expectedNodeEvictions:            1,
			expectedTotalEvictions:           1,
			expectedError:                    nil,
			events:                           []string{"Normal NotSet pod eviction from node node by sigs.k8s.io/descheduler"},
		},
		{
			description:                      "eviction limit exceeded on total with eviction failure event notification",
			pod:                              pod1,
			evictionFailureEventNotification: utilptr.To[bool](true),
			maxPodsToEvictTotal:              utilptr.To[uint](0),
			maxPodsToEvictPerNode:            utilptr.To[uint](1),
			maxPodsToEvictPerNamespace:       utilptr.To[uint](1),
			expectedNodeEvictions:            0,
			expectedTotalEvictions:           0,
			expectedError:                    NewEvictionTotalLimitError(),
			events:                           []string{"Warning EvictionFailed pod eviction from node node by sigs.k8s.io/descheduler failed: total eviction limit exceeded (0)"},
		},
		{
			description:                      "eviction limit exceeded on node with eviction failure event notification",
			pod:                              pod1,
			evictionFailureEventNotification: utilptr.To[bool](true),
			maxPodsToEvictTotal:              utilptr.To[uint](1),
			maxPodsToEvictPerNode:            utilptr.To[uint](0),
			maxPodsToEvictPerNamespace:       utilptr.To[uint](1),
			expectedNodeEvictions:            0,
			expectedTotalEvictions:           0,
			expectedError:                    NewEvictionNodeLimitError("node"),
			events:                           []string{"Warning EvictionFailed pod eviction from node node by sigs.k8s.io/descheduler failed: node eviction limit exceeded (0)"},
		},
		{
			description:                      "eviction limit exceeded on node with eviction failure event notification",
			pod:                              pod1,
			evictionFailureEventNotification: utilptr.To[bool](true),
			maxPodsToEvictTotal:              utilptr.To[uint](1),
			maxPodsToEvictPerNode:            utilptr.To[uint](1),
			maxPodsToEvictPerNamespace:       utilptr.To[uint](0),
			expectedNodeEvictions:            0,
			expectedTotalEvictions:           0,
			expectedError:                    NewEvictionNamespaceLimitError("default"),
			events:                           []string{"Warning EvictionFailed pod eviction from node node by sigs.k8s.io/descheduler failed: namespace eviction limit exceeded (0)"},
		},
		{
			description:                      "eviction error with eviction failure event notification",
			pod:                              pod1,
			evictionFailureEventNotification: utilptr.To[bool](true),
			maxPodsToEvictTotal:              utilptr.To[uint](1),
			maxPodsToEvictPerNode:            utilptr.To[uint](1),
			maxPodsToEvictPerNamespace:       utilptr.To[uint](1),
			expectedNodeEvictions:            0,
			expectedTotalEvictions:           0,
			expectedError:                    fmt.Errorf("eviction error"),
			events:                           []string{"Warning EvictionFailed pod eviction from node node by sigs.k8s.io/descheduler failed: eviction error"},
		},
		{
			description:                      "eviction with dryRun with eviction failure event notification",
			pod:                              pod1,
			dryRun:                           true,
			evictionFailureEventNotification: utilptr.To[bool](true),
			maxPodsToEvictTotal:              utilptr.To[uint](1),
			maxPodsToEvictPerNode:            utilptr.To[uint](1),
			maxPodsToEvictPerNamespace:       utilptr.To[uint](1),
			expectedNodeEvictions:            1,
			expectedTotalEvictions:           1,
			expectedError:                    nil,
		},
		{
			description:                "one eviction expected without eviction failure event notification",
			pod:                        pod1,
			maxPodsToEvictTotal:        utilptr.To[uint](1),
			maxPodsToEvictPerNode:      utilptr.To[uint](1),
			maxPodsToEvictPerNamespace: utilptr.To[uint](1),
			expectedNodeEvictions:      1,
			expectedTotalEvictions:     1,
			expectedError:              nil,
			events:                     []string{"Normal NotSet pod eviction from node node by sigs.k8s.io/descheduler"},
		},
		{
			description:                "eviction limit exceeded on total without eviction failure event notification",
			pod:                        pod1,
			maxPodsToEvictTotal:        utilptr.To[uint](0),
			maxPodsToEvictPerNode:      utilptr.To[uint](1),
			maxPodsToEvictPerNamespace: utilptr.To[uint](1),
			expectedNodeEvictions:      0,
			expectedTotalEvictions:     0,
			expectedError:              NewEvictionTotalLimitError(),
		},
		{
			description:                "eviction limit exceeded on node without eviction failure event notification",
			pod:                        pod1,
			maxPodsToEvictTotal:        utilptr.To[uint](1),
			maxPodsToEvictPerNode:      utilptr.To[uint](0),
			maxPodsToEvictPerNamespace: utilptr.To[uint](1),
			expectedNodeEvictions:      0,
			expectedTotalEvictions:     0,
			expectedError:              NewEvictionNodeLimitError("node"),
		},
		{
			description:                "eviction limit exceeded on node without eviction failure event notification",
			pod:                        pod1,
			maxPodsToEvictTotal:        utilptr.To[uint](1),
			maxPodsToEvictPerNode:      utilptr.To[uint](1),
			maxPodsToEvictPerNamespace: utilptr.To[uint](0),
			expectedNodeEvictions:      0,
			expectedTotalEvictions:     0,
			expectedError:              NewEvictionNamespaceLimitError("default"),
		},
		{
			description:                "eviction error without eviction failure event notification",
			pod:                        pod1,
			maxPodsToEvictTotal:        utilptr.To[uint](1),
			maxPodsToEvictPerNode:      utilptr.To[uint](1),
			maxPodsToEvictPerNamespace: utilptr.To[uint](1),
			expectedNodeEvictions:      0,
			expectedTotalEvictions:     0,
			expectedError:              fmt.Errorf("eviction error"),
		},
		{
			description:                "eviction without dryRun with eviction failure event notification",
			pod:                        pod1,
			dryRun:                     true,
			maxPodsToEvictTotal:        utilptr.To[uint](1),
			maxPodsToEvictPerNode:      utilptr.To[uint](1),
			maxPodsToEvictPerNamespace: utilptr.To[uint](1),
			expectedNodeEvictions:      1,
			expectedTotalEvictions:     1,
			expectedError:              nil,
		},
	}
	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			fakeClient := fake.NewSimpleClientset(pod1)
			fakeClient.PrependReactor("create", "pods/eviction", func(action core.Action) (handled bool, ret runtime.Object, err error) {
				return true, nil, test.expectedError
			})

			sharedInformerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
			sharedInformerFactory.Start(ctx.Done())
			sharedInformerFactory.WaitForCacheSync(ctx.Done())

			eventRecorder := events.NewFakeRecorder(100)

			podEvictor, err := NewPodEvictor(
				ctx,
				fakeClient,
				eventRecorder,
				sharedInformerFactory.Core().V1().Pods().Informer(),
				initFeatureGates(),
				NewOptions().
					WithDryRun(test.dryRun).
					WithMaxPodsToEvictTotal(test.maxPodsToEvictTotal).
					WithMaxPodsToEvictPerNode(test.maxPodsToEvictPerNode).
					WithEvictionFailureEventNotification(test.evictionFailureEventNotification).
					WithMaxPodsToEvictPerNamespace(test.maxPodsToEvictPerNamespace),
			)
			if err != nil {
				t.Fatalf("Unexpected error when creating a pod evictor: %v", err)
			}

			stubNode := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node"}}

			if actualErr := podEvictor.EvictPod(ctx, test.pod, EvictOptions{}); actualErr != nil && actualErr.Error() != test.expectedError.Error() {
				t.Errorf("Expected error: %v, got: %v", test.expectedError, actualErr)
			}

			if evictions := podEvictor.NodeEvicted(stubNode); evictions != test.expectedNodeEvictions {
				t.Errorf("Expected %d node evictions, got %d instead", test.expectedNodeEvictions, evictions)
			}

			if evictions := podEvictor.TotalEvicted(); evictions != test.expectedTotalEvictions {
				t.Errorf("Expected %d total evictions, got %d instead", test.expectedTotalEvictions, evictions)
			}

			// Assert that the events are correct.
			assertEqualEvents(t, test.events, eventRecorder.Events)
		})
	}
}

func TestEvictionRequestsCacheCleanup(t *testing.T) {
	ctx := context.Background()
	node1 := test.BuildTestNode("n1", 2000, 3000, 10, nil)

	ownerRef1 := test.GetReplicaSetOwnerRefList()
	updatePod := func(pod *v1.Pod) {
		pod.Namespace = "dev"
		pod.ObjectMeta.OwnerReferences = ownerRef1
	}
	updatePodWithEvictionInBackground := func(pod *v1.Pod) {
		updatePod(pod)
		pod.Annotations = map[string]string{
			EvictionRequestAnnotationKey: "",
		}
	}

	p1 := test.BuildTestPod("p1", 100, 0, node1.Name, updatePodWithEvictionInBackground)
	p2 := test.BuildTestPod("p2", 100, 0, node1.Name, updatePodWithEvictionInBackground)
	p3 := test.BuildTestPod("p3", 100, 0, node1.Name, updatePod)
	p4 := test.BuildTestPod("p4", 100, 0, node1.Name, updatePod)

	client := fakeclientset.NewSimpleClientset(node1, p1, p2, p3, p4)
	sharedInformerFactory := informers.NewSharedInformerFactory(client, 0)
	_, eventRecorder := utils.GetRecorderAndBroadcaster(ctx, client)

	podEvictor, err := NewPodEvictor(
		ctx,
		client,
		eventRecorder,
		sharedInformerFactory.Core().V1().Pods().Informer(),
		initFeatureGates(),
		nil,
	)
	if err != nil {
		t.Fatalf("Unexpected error when creating a pod evictor: %v", err)
	}

	client.PrependReactor("create", "pods", func(action core.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() == "eviction" {
			createAct, matched := action.(core.CreateActionImpl)
			if !matched {
				return false, nil, fmt.Errorf("unable to convert action to core.CreateActionImpl")
			}
			if eviction, matched := createAct.Object.(*policy.Eviction); matched {
				podName := eviction.GetName()
				if podName == "p1" || podName == "p2" {
					return true, nil, &apierrors.StatusError{
						ErrStatus: metav1.Status{
							Reason:  metav1.StatusReasonTooManyRequests,
							Message: "Eviction triggered evacuation",
						},
					}
				}
				return true, nil, nil
			}
		}
		return false, nil, nil
	})

	sharedInformerFactory.Start(ctx.Done())
	sharedInformerFactory.WaitForCacheSync(ctx.Done())

	podEvictor.EvictPod(ctx, p1, EvictOptions{})
	podEvictor.EvictPod(ctx, p2, EvictOptions{})
	podEvictor.EvictPod(ctx, p3, EvictOptions{})
	podEvictor.EvictPod(ctx, p4, EvictOptions{})

	klog.Infof("2 evictions in background expected, 2 normal evictions")
	if total := podEvictor.TotalEvictionRequests(); total != 2 {
		t.Fatalf("Expected %v total eviction requests, got %v instead", 2, total)
	}
	if total := podEvictor.TotalEvicted(); total != 2 {
		t.Fatalf("Expected %v total evictions, got %v instead", 2, total)
	}

	klog.Infof("2 evictions in background assumed. Wait for few seconds and check the assumed requests timed out")
	time.Sleep(2 * time.Second)
	klog.Infof("Checking the assumed requests timed out and were deleted")
	// Set the timeout to 1s so the cleaning can be tested
	podEvictor.erCache.assumedRequestTimeoutSeconds = 1
	podEvictor.erCache.cleanCache(ctx)
	if totalERs := podEvictor.TotalEvictionRequests(); totalERs > 0 {
		t.Fatalf("Expected 0 eviction requests, got %v instead", totalERs)
	}
}

func assertEqualEvents(t *testing.T, expected []string, actual <-chan string) {
	t.Logf("Assert for events: %v", expected)
	c := time.After(wait.ForeverTestTimeout)
	for _, e := range expected {
		select {
		case a := <-actual:
			if !reflect.DeepEqual(a, e) {
				t.Errorf("Expected event %q, got %q instead", e, a)
			}
		case <-c:
			t.Errorf("Expected event %q, got nothing", e)
			// continue iterating to print all expected events
		}
	}
	for {
		select {
		case a := <-actual:
			t.Errorf("Unexpected event: %q", a)
		default:
			return // No more events, as expected.
		}
	}
}
