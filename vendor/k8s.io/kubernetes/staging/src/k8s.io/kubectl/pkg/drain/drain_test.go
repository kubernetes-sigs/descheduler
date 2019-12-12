/*
Copyright 2015 The Kubernetes Authors.

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

package drain

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"reflect"
	"sort"
	"strconv"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/fake"
	ktest "k8s.io/client-go/testing"
)

func TestDeletePods(t *testing.T) {
	ifHasBeenCalled := map[string]bool{}
	tests := []struct {
		description       string
		interval          time.Duration
		timeout           time.Duration
		expectPendingPods bool
		expectError       bool
		expectedError     *error
		getPodFn          func(namespace, name string) (*corev1.Pod, error)
	}{
		{
			description:       "Wait for deleting to complete",
			interval:          100 * time.Millisecond,
			timeout:           10 * time.Second,
			expectPendingPods: false,
			expectError:       false,
			expectedError:     nil,
			getPodFn: func(namespace, name string) (*corev1.Pod, error) {
				oldPodMap, _ := createPods(false)
				newPodMap, _ := createPods(true)
				if oldPod, found := oldPodMap[name]; found {
					if _, ok := ifHasBeenCalled[name]; !ok {
						ifHasBeenCalled[name] = true
						return &oldPod, nil
					}
					if oldPod.ObjectMeta.Generation < 4 {
						newPod := newPodMap[name]
						return &newPod, nil
					}
					return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, name)

				}
				return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, name)
			},
		},
		{
			description:       "Deleting could timeout",
			interval:          200 * time.Millisecond,
			timeout:           3 * time.Second,
			expectPendingPods: true,
			expectError:       true,
			expectedError:     &wait.ErrWaitTimeout,
			getPodFn: func(namespace, name string) (*corev1.Pod, error) {
				oldPodMap, _ := createPods(false)
				if oldPod, found := oldPodMap[name]; found {
					return &oldPod, nil
				}
				return nil, fmt.Errorf("%q: not found", name)
			},
		},
		{
			description:       "Client error could be passed out",
			interval:          200 * time.Millisecond,
			timeout:           5 * time.Second,
			expectPendingPods: true,
			expectError:       true,
			expectedError:     nil,
			getPodFn: func(namespace, name string) (*corev1.Pod, error) {
				return nil, errors.New("This is a random error for testing")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			_, pods := createPods(false)
			ctx := context.TODO()
			pendingPods, err := waitForDelete(ctx, pods, test.interval, test.timeout, false, test.getPodFn, nil, time.Duration(math.MaxInt64))

			if test.expectError {
				if err == nil {
					t.Fatalf("%s: unexpected non-error", test.description)
				} else if test.expectedError != nil {
					if *test.expectedError != err {
						t.Fatalf("%s: the error does not match expected error", test.description)
					}
				}
			}
			if !test.expectError && err != nil {
				t.Fatalf("%s: unexpected error", test.description)
			}
			if test.expectPendingPods && len(pendingPods) == 0 {
				t.Fatalf("%s: unexpected empty pods", test.description)
			}
			if !test.expectPendingPods && len(pendingPods) > 0 {
				t.Fatalf("%s: unexpected pending pods", test.description)
			}
		})
	}
}

func createPods(ifCreateNewPods bool) (map[string]corev1.Pod, []corev1.Pod) {
	podMap := make(map[string]corev1.Pod)
	podSlice := []corev1.Pod{}
	for i := 0; i < 8; i++ {
		var uid types.UID
		if ifCreateNewPods {
			uid = types.UID(i)
		} else {
			uid = types.UID(strconv.Itoa(i) + strconv.Itoa(i))
		}
		pod := corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "pod" + strconv.Itoa(i),
				Namespace:  "default",
				UID:        uid,
				Generation: int64(i),
			},
		}
		podMap[pod.Name] = pod
		podSlice = append(podSlice, pod)
	}
	return podMap, podSlice
}

// addEvictionSupport implements simple fake eviction support on the fake.Clientset
func addEvictionSupport(t *testing.T, k *fake.Clientset) {
	podsEviction := metav1.APIResource{
		Name:    "pods/eviction",
		Kind:    "Eviction",
		Group:   "",
		Version: "v1",
	}
	coreResources := &metav1.APIResourceList{
		GroupVersion: "v1",
		APIResources: []metav1.APIResource{podsEviction},
	}

	policyResources := &metav1.APIResourceList{
		GroupVersion: "policy/v1",
	}
	k.Resources = append(k.Resources, coreResources, policyResources)

	// Delete pods when evict is called
	k.PrependReactor("create", "pods", func(action ktest.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() != "eviction" {
			return false, nil, nil
		}

		eviction := *action.(ktest.CreateAction).GetObject().(*policyv1beta1.Eviction)
		// Avoid the lock
		go func() {
			err := k.CoreV1().Pods(eviction.Namespace).Delete(eviction.Name, &metav1.DeleteOptions{})
			if err != nil {
				// Errorf because we can't call Fatalf from another goroutine
				t.Errorf("failed to delete pod: %s/%s", eviction.Namespace, eviction.Name)
			}
		}()

		return true, nil, nil
	})
}

func TestCheckEvictionSupport(t *testing.T) {
	for _, evictionSupported := range []bool{true, false} {
		evictionSupported := evictionSupported
		t.Run(fmt.Sprintf("evictionSupported=%v", evictionSupported),
			func(t *testing.T) {
				k := fake.NewSimpleClientset()
				if evictionSupported {
					addEvictionSupport(t, k)
				}

				apiGroup, err := CheckEvictionSupport(k)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				expectedAPIGroup := ""
				if evictionSupported {
					expectedAPIGroup = "policy/v1"
				}
				if apiGroup != expectedAPIGroup {
					t.Fatalf("expected apigroup %q, actual=%q", expectedAPIGroup, apiGroup)
				}
			})
	}
}

func TestDeleteOrEvict(t *testing.T) {
	for _, evictionSupported := range []bool{true, false} {
		evictionSupported := evictionSupported
		t.Run(fmt.Sprintf("evictionSupported=%v", evictionSupported),
			func(t *testing.T) {
				h := &Helper{
					Out:                os.Stdout,
					GracePeriodSeconds: 10,
				}

				// Create 4 pods, and try to remove the first 2
				var expectedEvictions []policyv1beta1.Eviction
				var create []runtime.Object
				deletePods := []corev1.Pod{}
				for i := 1; i <= 4; i++ {
					pod := &corev1.Pod{}
					pod.Name = fmt.Sprintf("mypod-%d", i)
					pod.Namespace = "default"

					create = append(create, pod)
					if i <= 2 {
						deletePods = append(deletePods, *pod)

						if evictionSupported {
							eviction := policyv1beta1.Eviction{}
							eviction.Kind = "Eviction"
							eviction.APIVersion = "policy/v1"
							eviction.Namespace = pod.Namespace
							eviction.Name = pod.Name

							gracePeriodSeconds := int64(h.GracePeriodSeconds)
							eviction.DeleteOptions = &metav1.DeleteOptions{
								GracePeriodSeconds: &gracePeriodSeconds,
							}

							expectedEvictions = append(expectedEvictions, eviction)
						}
					}
				}

				// Build the fake client
				k := fake.NewSimpleClientset(create...)
				if evictionSupported {
					addEvictionSupport(t, k)
				}
				h.Client = k

				// Do the eviction
				if err := h.DeleteOrEvictPods(deletePods); err != nil {
					t.Fatalf("error from DeleteOrEvictPods: %v", err)
				}

				// Test that other pods are still there
				var remainingPods []string
				{
					podList, err := k.CoreV1().Pods("").List(metav1.ListOptions{})
					if err != nil {
						t.Fatalf("error listing pods: %v", err)
					}

					for _, pod := range podList.Items {
						remainingPods = append(remainingPods, pod.Namespace+"/"+pod.Name)
					}
					sort.Strings(remainingPods)
				}
				expected := []string{"default/mypod-3", "default/mypod-4"}
				if !reflect.DeepEqual(remainingPods, expected) {
					t.Errorf("unexpected remaining pods after DeleteOrEvictPods; actual %v; expected %v", remainingPods, expected)
				}

				// Test that pods were evicted as expected
				var actualEvictions []policyv1beta1.Eviction
				for _, action := range k.Actions() {
					if action.GetVerb() != "create" || action.GetResource().Resource != "pods" || action.GetSubresource() != "eviction" {
						continue
					}
					eviction := *action.(ktest.CreateAction).GetObject().(*policyv1beta1.Eviction)
					actualEvictions = append(actualEvictions, eviction)
				}
				sort.Slice(actualEvictions, func(i, j int) bool {
					return actualEvictions[i].Name < actualEvictions[j].Name
				})
				if !reflect.DeepEqual(actualEvictions, expectedEvictions) {
					t.Errorf("unexpected evictions; actual %v; expected %v", actualEvictions, expectedEvictions)
				}
			})
	}
}
