/*
Copyright 2021 The Kubernetes Authors.

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

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	componentbaseconfig "k8s.io/component-base/config"

	"sigs.k8s.io/descheduler/cmd/descheduler/app/options"
	"sigs.k8s.io/descheduler/pkg/api"
	apiv1alpha2 "sigs.k8s.io/descheduler/pkg/api/v1alpha2"
	"sigs.k8s.io/descheduler/pkg/descheduler/client"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodshavingtoomanyrestarts"
)

const deploymentReplicas = 4

func tooManyRestartsPolicy(targetNamespace string, podRestartThresholds int32, includingInitContainers bool, gracePeriodSeconds int64) *apiv1alpha2.DeschedulerPolicy {
	return &apiv1alpha2.DeschedulerPolicy{
		Profiles: []apiv1alpha2.DeschedulerProfile{
			{
				Name: "TooManyRestartsProfile",
				PluginConfigs: []apiv1alpha2.PluginConfig{
					{
						Name: removepodshavingtoomanyrestarts.PluginName,
						Args: runtime.RawExtension{
							Object: &removepodshavingtoomanyrestarts.RemovePodsHavingTooManyRestartsArgs{
								PodRestartThreshold:     podRestartThresholds,
								IncludingInitContainers: includingInitContainers,
								Namespaces: &api.Namespaces{
									Include: []string{targetNamespace},
								},
							},
						},
					},
					{
						Name: defaultevictor.PluginName,
						Args: runtime.RawExtension{
							Object: &defaultevictor.DefaultEvictorArgs{
								EvictLocalStoragePods: true,
							},
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
							removepodshavingtoomanyrestarts.PluginName,
						},
					},
				},
			},
		},
		GracePeriodSeconds: &gracePeriodSeconds,
	}
}

func TestTooManyRestarts(t *testing.T) {
	ctx := context.Background()
	initPluginRegistry()

	clientSet, err := client.CreateClient(componentbaseconfig.ClientConnectionConfiguration{Kubeconfig: os.Getenv("KUBECONFIG")}, "")
	if err != nil {
		t.Errorf("Error during kubernetes client creation with %v", err)
	}

	t.Logf("Creating testing namespace %v", t.Name())
	testNamespace := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "e2e-" + strings.ToLower(t.Name())}}
	if _, err := clientSet.CoreV1().Namespaces().Create(ctx, testNamespace, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Unable to create ns %v", testNamespace.Name)
	}
	defer clientSet.CoreV1().Namespaces().Delete(ctx, testNamespace.Name, metav1.DeleteOptions{})

	deploymentObj := buildTestDeployment("restart-pod", testNamespace.Name, deploymentReplicas, map[string]string{"test": "restart-pod", "name": "test-toomanyrestarts"}, func(deployment *appsv1.Deployment) {
		deployment.Spec.Template.Spec.Containers[0].Command = []string{"/bin/sh"}
		deployment.Spec.Template.Spec.Containers[0].Args = []string{"-c", "sleep 1s && exit 1"}
	})

	t.Logf("Creating deployment %v", deploymentObj.Name)
	_, err = clientSet.AppsV1().Deployments(deploymentObj.Namespace).Create(ctx, deploymentObj, metav1.CreateOptions{})
	if err != nil {
		t.Logf("Error creating deployment: %v", err)
		if err = clientSet.AppsV1().Deployments(deploymentObj.Namespace).DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{
			LabelSelector: labels.SelectorFromSet(labels.Set(map[string]string{"test": "restart-pod", "name": "test-toomanyrestarts"})).String(),
		}); err != nil {
			t.Fatalf("Unable to delete deployment: %v", err)
		}
		return
	}
	defer clientSet.AppsV1().Deployments(deploymentObj.Namespace).Delete(ctx, deploymentObj.Name, metav1.DeleteOptions{})

	// Wait for 3 restarts
	waitPodRestartCount(ctx, clientSet, testNamespace.Name, t, 3)

	tests := []struct {
		name                    string
		policy                  *apiv1alpha2.DeschedulerPolicy
		enableGracePeriod       bool
		expectedEvictedPodCount uint
	}{
		{
			name:                    "test-no-evictions",
			policy:                  tooManyRestartsPolicy(testNamespace.Name, 10000, true, 0),
			expectedEvictedPodCount: 0,
		},
		{
			name:                    "test-one-evictions",
			policy:                  tooManyRestartsPolicy(testNamespace.Name, 3, true, 0),
			expectedEvictedPodCount: 4,
		},
		{
			name:                    "test-one-evictions-use-gracePeriodSeconds",
			policy:                  tooManyRestartsPolicy(testNamespace.Name, 3, true, 60),
			enableGracePeriod:       true,
			expectedEvictedPodCount: 4,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rs, err := options.NewDeschedulerServer()
			if err != nil {
				t.Fatalf("Unable to initialize server: %v\n", err)
			}
			rs.Client = clientSet
			rs.EventClient = clientSet
			rs.DefaultFeatureGates = initFeatureGates()

			preRunNames := sets.NewString(getCurrentPodNames(ctx, clientSet, testNamespace.Name, t)...)
			// Deploy the descheduler with the configured policy
			deschedulerPolicyConfigMapObj, err := deschedulerPolicyConfigMap(tc.policy)
			if err != nil {
				t.Fatalf("Error creating %q CM: %v", deschedulerPolicyConfigMapObj.Name, err)
			}
			t.Logf("Creating %q policy CM with RemovePodsHavingTooManyRestarts configured...", deschedulerPolicyConfigMapObj.Name)
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

			// Check if grace period is enabled and wait accordingly
			if tc.enableGracePeriod {
				// Ensure no pods are evicted during the grace period
				// Wait for 55 seconds to ensure that the pods are not evicted during the grace period
				// We do not want to use an extreme waiting time, such as 59 seconds,
				// because the grace period is set to 60 seconds.
				// In order to avoid unnecessary flake failures,
				// we only need to make sure that the pod is not evicted within a certain range.
				t.Logf("Waiting for grace period of %d seconds", 55)
				if err := wait.PollUntilContextTimeout(ctx, 1*time.Second, time.Duration(55)*time.Second, true, func(ctx context.Context) (bool, error) {
					currentRunNames := sets.NewString(getCurrentPodNames(ctx, clientSet, testNamespace.Name, t)...)
					actualEvictedPod := preRunNames.Difference(currentRunNames)
					actualEvictedPodCount := uint(actualEvictedPod.Len())
					t.Logf("preRunNames: %v, currentRunNames: %v, actualEvictedPodCount: %v\n", preRunNames.List(), currentRunNames.List(), actualEvictedPodCount)
					if actualEvictedPodCount > 0 {
						t.Fatalf("Pods were evicted during grace period; expected 0, got %v", actualEvictedPodCount)
						return false, nil
					}
					return true, nil
				}); err != nil {
					t.Fatalf("Error waiting during grace period: %v", err)
				}
			}

			// Run RemovePodsHavingTooManyRestarts strategy
			if err := wait.PollUntilContextTimeout(ctx, 1*time.Second, 50*time.Second, true, func(ctx context.Context) (bool, error) {
				currentRunNames := sets.NewString(getCurrentPodNames(ctx, clientSet, testNamespace.Name, t)...)
				actualEvictedPod := preRunNames.Difference(currentRunNames)
				actualEvictedPodCount := uint(actualEvictedPod.Len())
				t.Logf("preRunNames: %v, currentRunNames: %v, actualEvictedPodCount: %v\n", preRunNames.List(), currentRunNames.List(), actualEvictedPodCount)
				if actualEvictedPodCount < tc.expectedEvictedPodCount {
					t.Logf("Expecting %v number of pods evicted, got %v instead", tc.expectedEvictedPodCount, actualEvictedPodCount)
					return false, nil
				}

				return true, nil
			}); err != nil {
				t.Fatalf("Error waiting for descheduler running: %v", err)
			}
			waitForTerminatingPodsToDisappear(ctx, t, clientSet, testNamespace.Name)
		})
	}
}

func waitPodRestartCount(ctx context.Context, clientSet clientset.Interface, namespace string, t *testing.T, expectedNumberOfRestarts int) {
	if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
		podList, err := clientSet.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: labels.SelectorFromSet(labels.Set(map[string]string{"test": "restart-pod", "name": "test-toomanyrestarts"})).String(),
		})
		if err != nil {
			t.Fatalf("Unexpected err: %v", err)
			return false, err
		}
		if len(podList.Items) < expectedNumberOfRestarts {
			t.Log("Waiting for 4 pods")
			return false, nil
		}
		for i := 0; i < 4; i++ {
			if len(podList.Items[i].Status.ContainerStatuses) < 1 {
				t.Logf("Waiting for podList.Items[%v].Status.ContainerStatuses to be populated", i)
				return false, nil
			}
			if podList.Items[i].Status.ContainerStatuses[0].RestartCount < int32(expectedNumberOfRestarts) {
				t.Logf("podList.Items[%v].Status.ContainerStatuses[0].RestartCount (%v) < %v", i, podList.Items[i].Status.ContainerStatuses[0].RestartCount, expectedNumberOfRestarts)
				return false, nil
			}
		}
		return true, nil
	}); err != nil {
		t.Fatalf("Error waiting for a workload running: %v", err)
	}
}
