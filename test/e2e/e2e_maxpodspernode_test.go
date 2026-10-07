/*
Copyright 2026 The Kubernetes Authors.

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

package e2e

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	componentbaseconfig "k8s.io/component-base/config"

	"sigs.k8s.io/descheduler/pkg/api"
	apiv1alpha2 "sigs.k8s.io/descheduler/pkg/api/v1alpha2"
	"sigs.k8s.io/descheduler/pkg/descheduler/client"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/maxpodspernode"
)

func maxPodsPerNodePolicy(maxPodsArgs *maxpodspernode.MaxPodsPerNodeArgs, evictorArgs *defaultevictor.DefaultEvictorArgs) *apiv1alpha2.DeschedulerPolicy {
	return &apiv1alpha2.DeschedulerPolicy{
		Profiles: []apiv1alpha2.DeschedulerProfile{
			{
				Name: maxpodspernode.PluginName + "Profile",
				PluginConfigs: []apiv1alpha2.PluginConfig{
					{
						Name: maxpodspernode.PluginName,
						Args: runtime.RawExtension{
							Object: maxPodsArgs,
						},
					},
					{
						Name: defaultevictor.PluginName,
						Args: runtime.RawExtension{
							Object: evictorArgs,
						},
					},
				},
				Plugins: apiv1alpha2.Plugins{
					Filter: apiv1alpha2.PluginSet{
						Enabled: []string{
							defaultevictor.PluginName,
						},
					},
					Deschedule: apiv1alpha2.PluginSet{
						Enabled: []string{
							maxpodspernode.PluginName,
						},
					},
				},
			},
		},
	}
}

func TestMaxPodsPerNode(t *testing.T) {
	ctx := context.Background()

	clientSet, err := client.CreateClient(componentbaseconfig.ClientConnectionConfiguration{Kubeconfig: os.Getenv("KUBECONFIG")}, "")
	if err != nil {
		t.Errorf("Error during kubernetes client creation with %v", err)
	}

	t.Log("Creating testing namespace")
	testNamespace := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "e2e-" + strings.ToLower(t.Name())}}
	if _, err := clientSet.CoreV1().Namespaces().Create(ctx, testNamespace, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Unable to create ns %v", testNamespace.Name)
	}
	defer clientSet.CoreV1().Namespaces().Delete(ctx, testNamespace.Name, metav1.DeleteOptions{})

	tests := []struct {
		name                    string
		replicaCount            int32
		maxPods                 int
		expectedEvictedPodCount int
	}{
		{
			name:                    "test-no-eviction-under-limit",
			replicaCount:            2,
			maxPods:                 5,
			expectedEvictedPodCount: 0,
		},
		{
			name:                    "test-eviction-over-limit",
			replicaCount:            6,
			maxPods:                 3,
			expectedEvictedPodCount: 3,
		},
		{
			name:                    "test-no-eviction-at-limit",
			replicaCount:            4,
			maxPods:                 4,
			expectedEvictedPodCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			testLabel := map[string]string{"test": tc.name, "name": tc.name}

			deploymentObj := buildTestDeployment(tc.name, testNamespace.Name, tc.replicaCount, testLabel, nil)

			t.Logf("Creating deployment %v with %d replicas", deploymentObj.Name, tc.replicaCount)
			_, err = clientSet.AppsV1().Deployments(deploymentObj.Namespace).Create(ctx, deploymentObj, metav1.CreateOptions{})
			if err != nil {
				t.Fatalf("Error creating deployment: %v", err)
			}
			defer func() {
				clientSet.AppsV1().Deployments(deploymentObj.Namespace).Delete(ctx, deploymentObj.Name, metav1.DeleteOptions{})
				waitForPodsToDisappear(ctx, t, clientSet, testLabel, testNamespace.Name)
			}()

			t.Logf("Waiting for %d pods to be running", tc.replicaCount)
			waitForPodsRunning(ctx, t, clientSet, testLabel, int(tc.replicaCount), testNamespace.Name)

			preRunNames := sets.NewString(getCurrentPodNames(ctx, clientSet, testNamespace.Name, t)...)

			evictorArgs := &defaultevictor.DefaultEvictorArgs{
				EvictLocalStoragePods:   true,
				EvictSystemCriticalPods: false,
				IgnorePvcPods:           false,
				EvictFailedBarePods:     false,
			}
			maxPodsArgs := &maxpodspernode.MaxPodsPerNodeArgs{
				MaxPods: tc.maxPods,
				Namespaces: &api.Namespaces{
					Include: []string{testNamespace.Name},
				},
			}

			deschedulerPolicyConfigMapObj, err := deschedulerPolicyConfigMap(maxPodsPerNodePolicy(maxPodsArgs, evictorArgs))
			if err != nil {
				t.Fatalf("Error creating %q CM: %v", deschedulerPolicyConfigMapObj.Name, err)
			}

			t.Logf("Creating %q policy CM with MaxPodsPerNode configured...", deschedulerPolicyConfigMapObj.Name)
			_, err = clientSet.CoreV1().ConfigMaps(deschedulerPolicyConfigMapObj.Namespace).Create(ctx, deschedulerPolicyConfigMapObj, metav1.CreateOptions{})
			if err != nil {
				t.Fatalf("Error creating %q CM: %v", deschedulerPolicyConfigMapObj.Name, err)
			}

			defer func() {
				t.Logf("Deleting %q CM...", deschedulerPolicyConfigMapObj.Name)
				err = clientSet.CoreV1().ConfigMaps(deschedulerPolicyConfigMapObj.Namespace).Delete(ctx, deschedulerPolicyConfigMapObj.Name, metav1.DeleteOptions{})
				if err != nil {
					t.Fatalf("Unable to delete %q CM: %v", deschedulerPolicyConfigMapObj.Name, err)
				}
			}()

			deschedulerDeploymentObj := deschedulerDeployment(testNamespace.Name)
			t.Logf("Creating descheduler deployment %v", deschedulerDeploymentObj.Name)
			_, err = clientSet.AppsV1().Deployments(deschedulerDeploymentObj.Namespace).Create(ctx, deschedulerDeploymentObj, metav1.CreateOptions{})
			if err != nil {
				t.Fatalf("Error creating %q deployment: %v", deschedulerDeploymentObj.Name, err)
			}

			deschedulerPodName := ""
			defer func() {
				if deschedulerPodName != "" {
					printPodLogs(ctx, t, clientSet, deschedulerPodName)
				}

				t.Logf("Deleting %q deployment...", deschedulerDeploymentObj.Name)
				err = clientSet.AppsV1().Deployments(deschedulerDeploymentObj.Namespace).Delete(ctx, deschedulerDeploymentObj.Name, metav1.DeleteOptions{})
				if err != nil {
					t.Fatalf("Unable to delete %q deployment: %v", deschedulerDeploymentObj.Name, err)
				}

				waitForPodsToDisappear(ctx, t, clientSet, deschedulerDeploymentObj.Labels, deschedulerDeploymentObj.Namespace)
			}()

			t.Logf("Waiting for the descheduler pod running")
			deschedulerPods := waitForPodsRunning(ctx, t, clientSet, deschedulerDeploymentObj.Labels, 1, deschedulerDeploymentObj.Namespace)
			if len(deschedulerPods) != 0 {
				deschedulerPodName = deschedulerPods[0].Name
			}

			var meetsExpectations bool
			var actualEvictedPodCount int
			if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 60*time.Second, true, func(ctx context.Context) (bool, error) {
				currentRunNames := sets.NewString(getCurrentPodNames(ctx, clientSet, testNamespace.Name, t)...)
				actualEvictedPod := preRunNames.Difference(currentRunNames)
				actualEvictedPodCount = actualEvictedPod.Len()
				t.Logf("preRunNames: %v, currentRunNames: %v, actualEvictedPodCount: %v\n",
					preRunNames.List(), currentRunNames.List(), actualEvictedPodCount)
				if actualEvictedPodCount < tc.expectedEvictedPodCount {
					t.Logf("Expecting at least %v pods evicted, got %v instead", tc.expectedEvictedPodCount, actualEvictedPodCount)
					return false, nil
				}
				meetsExpectations = true
				return true, nil
			}); err != nil {
				t.Errorf("Error waiting for fedscheduler running: %v", err)
			}

			if !meetsExpectations {
				t.Errorf("Unexpected number of pods have been evicted, got %v, expected at least %v",
					actualEvictedPodCount, tc.expectedEvictedPodCount)
			} else {
				t.Logf("Total of %d pods were evicted for %s", actualEvictedPodCount, tc.name)
			}
		})
	}
}

func waitForPodCount(ctx context.Context, t *testing.T, clientSet clientset.Interface, namespace string, labelMap map[string]string, desiredCount int) {
	if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 60*time.Second, true, func(ctx context.Context) (bool, error) {
		podList, err := clientSet.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: labels.SelectorFromSet(labelMap).String(),
		})
		if err != nil {
			return false, err
		}

		runningCount := 0
		for _, pod := range podList.Items {
			if pod.Status.Phase == v1.PodRunning {
				runningCount++
			}
		}

		if runningCount != desiredCount {
			t.Logf("Waiting for %d running pods, currently %d", desiredCount, runningCount)
			return false, nil
		}
		return true, nil
	}); err != nil {
		t.Fatalf("Error waiting for pod count: %v", err)
	}
}
