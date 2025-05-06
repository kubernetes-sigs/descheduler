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
	"sigs.k8s.io/descheduler/pkg/framework/plugins/nodeutilization"
)

func lowNodeUtilizationPolicy(lowNodeUtilizationArgs *nodeutilization.LowNodeUtilizationArgs, evictorArgs *defaultevictor.DefaultEvictorArgs, metricsCollectorEnabled bool) *apiv1alpha2.DeschedulerPolicy {
	return &apiv1alpha2.DeschedulerPolicy{
		MetricsCollector: &apiv1alpha2.MetricsCollector{
			Enabled: metricsCollectorEnabled,
		},
		Profiles: []apiv1alpha2.DeschedulerProfile{
			{
				Name: nodeutilization.LowNodeUtilizationPluginName + "Profile",
				PluginConfigs: []apiv1alpha2.PluginConfig{
					{
						Name: nodeutilization.LowNodeUtilizationPluginName,
						Args: runtime.RawExtension{
							Object: lowNodeUtilizationArgs,
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
							nodeutilization.LowNodeUtilizationPluginName,
						},
					},
				},
			},
		},
	}
}

func TestLowNodeUtilizationKubernetesMetrics(t *testing.T) {
	ctx := context.Background()

	clientSet, err := client.CreateClient(componentbaseconfig.ClientConnectionConfiguration{Kubeconfig: os.Getenv("KUBECONFIG")}, "")
	if err != nil {
		t.Errorf("Error during kubernetes client creation with %v", err)
	}

	metricsClient, err := client.CreateMetricsClient(componentbaseconfig.ClientConnectionConfiguration{Kubeconfig: os.Getenv("KUBECONFIG")}, "descheduler")
	if err != nil {
		t.Errorf("Error during kubernetes metrics client creation with %v", err)
	}

	nodeList, err := clientSet.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Errorf("Error listing node with %v", err)
	}

	_, workerNodes := splitNodesAndWorkerNodes(nodeList.Items)

	testNamespace := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "e2e-" + strings.ToLower(t.Name())}}
	t.Logf("Creating testing namespace %q", testNamespace.Name)
	if _, err := clientSet.CoreV1().Namespaces().Create(ctx, testNamespace, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Unable to create ns %v: %v", testNamespace.Name, err)
	}
	defer clientSet.CoreV1().Namespaces().Delete(ctx, testNamespace.Name, metav1.DeleteOptions{})

	t.Log("Creating duplicates pods")
	testLabel := map[string]string{"app": "test-lownodeutilization-kubernetes-metrics", "name": "test-lownodeutilization-kubernetes-metrics"}
	deploymentObj := buildTestDeployment("lownodeutilization-kubernetes-metrics-pod", testNamespace.Name, 0, testLabel, nil)
	deploymentObj.Spec.Template.Spec.Containers[0].Image = "narmidm/k8s-pod-cpu-stressor:latest"
	deploymentObj.Spec.Template.Spec.Containers[0].Args = []string{"-cpu=1.0", "-duration=10s", "-forever"}
	deploymentObj.Spec.Template.Spec.Containers[0].Resources = v1.ResourceRequirements{
		Limits: v1.ResourceList{
			v1.ResourceCPU: resource.MustParse("3000m"),
		},
		Requests: v1.ResourceList{
			v1.ResourceCPU: resource.MustParse("0m"),
		},
	}

	tests := []struct {
		name                    string
		replicasNum             int
		beforeFunc              func(deployment *appsv1.Deployment)
		expectedEvictedPodCount int
		lowNodeUtilizationArgs  *nodeutilization.LowNodeUtilizationArgs
		evictorArgs             *defaultevictor.DefaultEvictorArgs
		metricsCollectorEnabled bool
	}{
		{
			name:        "metric server not enabled",
			replicasNum: 4,
			beforeFunc: func(deployment *appsv1.Deployment) {
				deployment.Spec.Replicas = utilptr.To[int32](4)
				deployment.Spec.Template.Spec.NodeName = workerNodes[0].Name
			},
			expectedEvictedPodCount: 0,
			lowNodeUtilizationArgs: &nodeutilization.LowNodeUtilizationArgs{
				Thresholds: api.ResourceThresholds{
					v1.ResourceCPU:  30,
					v1.ResourcePods: 30,
				},
				TargetThresholds: api.ResourceThresholds{
					v1.ResourceCPU:  50,
					v1.ResourcePods: 50,
				},
				MetricsUtilization: &nodeutilization.MetricsUtilization{
					Source: api.KubernetesMetrics,
				},
			},
			evictorArgs:             &defaultevictor.DefaultEvictorArgs{},
			metricsCollectorEnabled: false,
		},
		{
			name:        "requested cpu resource zero, actual cpu utilization 3 per pod",
			replicasNum: 4,
			beforeFunc: func(deployment *appsv1.Deployment) {
				deployment.Spec.Replicas = utilptr.To[int32](4)
				deployment.Spec.Template.Spec.NodeName = workerNodes[0].Name
			},
			expectedEvictedPodCount: 2,
			lowNodeUtilizationArgs: &nodeutilization.LowNodeUtilizationArgs{
				Thresholds: api.ResourceThresholds{
					v1.ResourceCPU:  30,
					v1.ResourcePods: 30,
				},
				TargetThresholds: api.ResourceThresholds{
					v1.ResourceCPU:  50,
					v1.ResourcePods: 50,
				},
				MetricsUtilization: &nodeutilization.MetricsUtilization{
					Source: api.KubernetesMetrics,
				},
			},
			evictorArgs:             &defaultevictor.DefaultEvictorArgs{},
			metricsCollectorEnabled: true,
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
			// wait until workerNodes[0].Name has the right actual cpu utilization and all the testing pods are running
			// and producing ~12 cores in total
			wait.PollUntilWithContext(ctx, 5*time.Second, func(context.Context) (done bool, err error) {
				item, err := metricsClient.MetricsV1beta1().NodeMetricses().Get(ctx, workerNodes[0].Name, metav1.GetOptions{})
				t.Logf("Waiting for %q nodemetrics cpu utilization to get over 12, currently %v", workerNodes[0].Name, item.Usage.Cpu().Value())
				if item.Usage.Cpu().Value() < 12 {
					return false, nil
				}
				totalCpu := resource.NewMilliQuantity(0, resource.DecimalSI)
				podItems, err := metricsClient.MetricsV1beta1().PodMetricses(deploymentObj.Namespace).List(ctx, metav1.ListOptions{})
				if err != nil {
					t.Logf("unable to list podmetricses: %v", err)
					return false, nil
				}
				for _, podMetrics := range podItems.Items {
					for _, container := range podMetrics.Containers {
						if _, exists := container.Usage[v1.ResourceCPU]; !exists {
							continue
						}
						totalCpu.Add(container.Usage[v1.ResourceCPU])
					}
				}
				// Value() will round up (e.g. 11.1 -> 12), which is still ok
				t.Logf("Waiting for totalCpu to get to 12 at least, got %v\n", totalCpu.Value())
				return totalCpu.Value() >= 12, nil
			})

			preRunNames := sets.NewString(getCurrentPodNames(ctx, clientSet, testNamespace.Name, t)...)

			// Deploy the descheduler with the configured policy
			deschedulerPolicyConfigMapObj, err := deschedulerPolicyConfigMap(lowNodeUtilizationPolicy(tc.lowNodeUtilizationArgs, tc.evictorArgs, tc.metricsCollectorEnabled))
			if err != nil {
				t.Fatalf("Error creating %q CM: %v", deschedulerPolicyConfigMapObj.Name, err)
			}

			t.Logf("Creating %q policy CM with LowNodeUtilization configured...", deschedulerPolicyConfigMapObj.Name)
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

			// Run LowNodeUtilization plugin
			var meetsExpectations bool
			var actualEvictedPodCount int
			if err = wait.PollUntilContextTimeout(ctx, 5*time.Second, 60*time.Second, true, func(ctx context.Context) (bool, error) {
				currentRunNames := sets.NewString(getCurrentPodNames(ctx, clientSet, testNamespace.Name, t)...)
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
