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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	componentbaseconfig "k8s.io/component-base/config"
	utilptr "k8s.io/utils/ptr"

	"sigs.k8s.io/descheduler/pkg/api"
	apiv1alpha2 "sigs.k8s.io/descheduler/pkg/api/v1alpha2"
	"sigs.k8s.io/descheduler/pkg/descheduler/client"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removeduplicates"
)

func removeDuplicatesPolicy(removeDuplicatesArgs *removeduplicates.RemoveDuplicatesArgs, evictorArgs *defaultevictor.DefaultEvictorArgs) *apiv1alpha2.DeschedulerPolicy {
	return &apiv1alpha2.DeschedulerPolicy{
		Profiles: []apiv1alpha2.DeschedulerProfile{
			{
				Name: removeduplicates.PluginName + "Profile",
				PluginConfigs: []apiv1alpha2.PluginConfig{
					{
						Name: removeduplicates.PluginName,
						Args: runtime.RawExtension{
							Object: removeDuplicatesArgs,
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
					Balance: apiv1alpha2.PluginSet{
						Enabled: []string{
							removeduplicates.PluginName,
						},
					},
				},
			},
		},
	}
}

func TestRemoveDuplicates(t *testing.T) {
	ctx := context.Background()

	clientSet, err := client.CreateClient(componentbaseconfig.ClientConnectionConfiguration{Kubeconfig: os.Getenv("KUBECONFIG")}, "")
	if err != nil {
		t.Errorf("Error during kubernetes client creation with %v", err)
	}

	nodeList, err := clientSet.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Errorf("Error listing node with %v", err)
	}

	_, workerNodes := splitNodesAndWorkerNodes(nodeList.Items)

	t.Log("Creating testing namespace")
	testNamespace := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "e2e-" + strings.ToLower(t.Name())}}
	if _, err := clientSet.CoreV1().Namespaces().Create(ctx, testNamespace, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Unable to create ns %v", testNamespace.Name)
	}
	defer clientSet.CoreV1().Namespaces().Delete(ctx, testNamespace.Name, metav1.DeleteOptions{})

	t.Log("Creating duplicates pods")
	testLabel := map[string]string{"app": "test-duplicate", "name": "test-duplicatePods"}
	deploymentObj := buildTestDeployment("duplicate-pod", testNamespace.Name, 0, testLabel, nil)

	tests := []struct {
		name                    string
		replicasNum             int
		beforeFunc              func(deployment *appsv1.Deployment)
		expectedEvictedPodCount int
		removeDuplicatesArgs    *removeduplicates.RemoveDuplicatesArgs
		evictorArgs             *defaultevictor.DefaultEvictorArgs
	}{
		{
			name:        "Evict Pod even Pods schedule to specific node",
			replicasNum: 4,
			beforeFunc: func(deployment *appsv1.Deployment) {
				deployment.Spec.Replicas = utilptr.To[int32](4)
				deployment.Spec.Template.Spec.NodeName = workerNodes[0].Name
			},
			expectedEvictedPodCount: 2,
			removeDuplicatesArgs:    &removeduplicates.RemoveDuplicatesArgs{},
			evictorArgs: &defaultevictor.DefaultEvictorArgs{
				EvictLocalStoragePods: true,
				MinReplicas:           3,
			},
		},
		{
			name:        "Evict Pod even Pods with local storage",
			replicasNum: 5,
			beforeFunc: func(deployment *appsv1.Deployment) {
				deployment.Spec.Replicas = utilptr.To[int32](5)
				deployment.Spec.Template.Spec.Volumes = []v1.Volume{
					{
						Name: "sample",
						VolumeSource: v1.VolumeSource{
							EmptyDir: &v1.EmptyDirVolumeSource{
								SizeLimit: resource.NewQuantity(int64(10), resource.BinarySI),
							},
						},
					},
				}
			},
			expectedEvictedPodCount: 2,
			removeDuplicatesArgs:    &removeduplicates.RemoveDuplicatesArgs{},
			evictorArgs: &defaultevictor.DefaultEvictorArgs{
				EvictLocalStoragePods: true,
				MinReplicas:           3,
			},
		},
		{
			name:        "Ignores eviction with minReplicas of 4",
			replicasNum: 3,
			beforeFunc: func(deployment *appsv1.Deployment) {
				deployment.Spec.Replicas = utilptr.To[int32](3)
			},
			expectedEvictedPodCount: 0,
			removeDuplicatesArgs:    &removeduplicates.RemoveDuplicatesArgs{},
			evictorArgs: &defaultevictor.DefaultEvictorArgs{
				EvictLocalStoragePods: true,
				MinReplicas:           4,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Creating deployment %v in %v namespace", deploymentObj.Name, deploymentObj.Namespace)
			tc.beforeFunc(deploymentObj)

			_, err = clientSet.AppsV1().Deployments(deploymentObj.Namespace).Create(ctx, deploymentObj, metav1.CreateOptions{})
			if err != nil {
				t.Logf("Error creating deployment: %v", err)
				if err = clientSet.AppsV1().Deployments(deploymentObj.Namespace).DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{
					LabelSelector: labels.SelectorFromSet(deploymentObj.Labels).String(),
				}); err != nil {
					t.Fatalf("Unable to delete deployment: %v", err)
				}
				return
			}
			defer func() {
				clientSet.AppsV1().Deployments(deploymentObj.Namespace).Delete(ctx, deploymentObj.Name, metav1.DeleteOptions{})
				waitForPodsToDisappear(ctx, t, clientSet, deploymentObj.Labels, deploymentObj.Namespace)
			}()
			waitForPodsRunning(ctx, t, clientSet, deploymentObj.Labels, tc.replicasNum, deploymentObj.Namespace)

			preRunNames := sets.NewString(getCurrentPodNames(t, ctx, clientSet, testNamespace.Name)...)

			// Deploy the descheduler with the configured policy
			tc.removeDuplicatesArgs.Namespaces = &api.Namespaces{
				Include: []string{testNamespace.Name},
			}
			deschedulerPolicyConfigMapObj, err := deschedulerPolicyConfigMap(removeDuplicatesPolicy(tc.removeDuplicatesArgs, tc.evictorArgs), nil)
			if err != nil {
				t.Fatalf("Error creating %q CM: %v", deschedulerPolicyConfigMapObj.Name, err)
			}

			t.Logf("Creating %q policy CM with RemoveDuplicates configured...", deschedulerPolicyConfigMapObj.Name)
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

			deschedulerDeploymentObj := deschedulerDeployment(testNamespace.Name, nil)
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
			deschedulerPodName = waitForPodsRunning(ctx, t, clientSet, deschedulerDeploymentObj.Labels, 1, deschedulerDeploymentObj.Namespace)

			// Run RemoveDuplicates strategy
			var meetsExpectations bool
			var actualEvictedPodCount int
			if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 60*time.Second, true, func(ctx context.Context) (bool, error) {
				currentRunNames := sets.NewString(getCurrentPodNames(t, ctx, clientSet, testNamespace.Name)...)
				actualEvictedPod := preRunNames.Difference(currentRunNames)
				actualEvictedPodCount = actualEvictedPod.Len()
				t.Logf("preRunNames: %v, currentRunNames: %v, actualEvictedPodCount: %v\n", preRunNames.List(), currentRunNames.List(), actualEvictedPodCount)
				if actualEvictedPodCount != tc.expectedEvictedPodCount {
					t.Logf("Expecting %v number of pods evicted, got %v instead", tc.expectedEvictedPodCount, actualEvictedPodCount)
					return false, nil
				}
				meetsExpectations = true
				return true, nil
			}); err != nil {
				t.Errorf("Error waiting for descheduler running: %v", err)
			}

			if !meetsExpectations {
				t.Errorf("Unexpected number of pods have been evicted, got %v, expected %v", actualEvictedPodCount, tc.expectedEvictedPodCount)
			} else {
				t.Logf("Total of %d Pods were evicted for %s", actualEvictedPodCount, tc.name)
			}
		})
	}
}
