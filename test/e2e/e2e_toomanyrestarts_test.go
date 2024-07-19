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
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	clientset "k8s.io/client-go/kubernetes"
	componentbaseconfig "k8s.io/component-base/config"
	utilptr "k8s.io/utils/ptr"

	"sigs.k8s.io/descheduler/cmd/descheduler/app/options"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler"
	"sigs.k8s.io/descheduler/pkg/descheduler/client"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodshavingtoomanyrestarts"
)

func tooManyRestartsPolicy(targetNamespace string, podRestartThresholds int32, includingInitContainers bool) *api.DeschedulerPolicy {
	return &api.DeschedulerPolicy{
		Profiles: []api.DeschedulerProfile{
			{
				Name: "TooManyRestartsProfile",
				PluginConfigs: []api.PluginConfig{
					{
						Name: removepodshavingtoomanyrestarts.PluginName,
						Args: &removepodshavingtoomanyrestarts.RemovePodsHavingTooManyRestartsArgs{
							PodRestartThreshold:     podRestartThresholds,
							IncludingInitContainers: includingInitContainers,
							Namespaces: &api.Namespaces{
								Include: []string{targetNamespace},
							},
						},
					},
					{
						Name: defaultevictor.PluginName,
						Args: &defaultevictor.DefaultEvictorArgs{
							EvictLocalStoragePods: true,
						},
					},
				},
				Plugins: api.Plugins{
					Filter: api.PluginSet{
						Enabled: []string{
							defaultevictor.PluginName,
						},
					},
					Deschedule: api.PluginSet{
						Enabled: []string{
							removepodshavingtoomanyrestarts.PluginName,
						},
					},
				},
			},
		},
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

	deploymentObj := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "restart-pod",
			Namespace: testNamespace.Name,
			Labels:    map[string]string{"test": "restart-pod", "name": "test-toomanyrestarts"},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: utilptr.To[int32](4),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"test": "restart-pod", "name": "test-toomanyrestarts"},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"test": "restart-pod", "name": "test-toomanyrestarts"},
				},
				Spec: v1.PodSpec{
					SecurityContext: &v1.PodSecurityContext{
						RunAsNonRoot: utilptr.To(true),
						RunAsUser:    utilptr.To[int64](1000),
						RunAsGroup:   utilptr.To[int64](1000),
						SeccompProfile: &v1.SeccompProfile{
							Type: v1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []v1.Container{{
						Name:            "pause",
						ImagePullPolicy: "Always",
						Image:           "registry.k8s.io/pause",
						Command:         []string{"/bin/sh"},
						Args:            []string{"-c", "sleep 1s && exit 1"},
						Ports:           []v1.ContainerPort{{ContainerPort: 80}},
						SecurityContext: &v1.SecurityContext{
							AllowPrivilegeEscalation: utilptr.To(false),
							Capabilities: &v1.Capabilities{
								Drop: []v1.Capability{
									"ALL",
								},
							},
						},
					}},
				},
			},
		},
	}

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

	// Need to wait restartCount more than 4
	result, err := waitPodRestartCount(ctx, clientSet, testNamespace.Name, t)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !result {
		t.Fatal("Pod restart count not as expected")
	}

	tests := []struct {
		name                    string
		policy                  *api.DeschedulerPolicy
		expectedEvictedPodCount uint
	}{
		{
			name:                    "test-no-evictions",
			policy:                  tooManyRestartsPolicy(testNamespace.Name, 10000, true),
			expectedEvictedPodCount: 0,
		},
		{
			name:                    "test-one-evictions",
			policy:                  tooManyRestartsPolicy(testNamespace.Name, 4, true),
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

			preRunNames := getCurrentPodNames(t, ctx, clientSet, testNamespace.Name)
			// Run RemovePodsHavingTooManyRestarts strategy
			t.Log("Running RemovePodsHavingTooManyRestarts strategy")
			err = descheduler.RunDeschedulerStrategies(ctx, rs, tc.policy, "v1")
			if err != nil {
				t.Fatalf("Failed running a descheduling cycle: %v", err)
			}

			t.Logf("Finished RemoveFailedPods strategy for %s", tc.name)
			waitForTerminatingPodsToDisappear(ctx, t, clientSet, testNamespace.Name)
			afterRunNames := getCurrentPodNames(t, ctx, clientSet, testNamespace.Name)
			namesInCommonCount := len(intersectStrings(preRunNames, afterRunNames))

			t.Logf("preRunNames: %v, afterRunNames: %v, namesInCommonLen: %v\n", preRunNames, afterRunNames, namesInCommonCount)
			actualEvictedPodCount := uint(len(afterRunNames) - namesInCommonCount)
			if actualEvictedPodCount < tc.expectedEvictedPodCount {
				t.Errorf("Test error for description: %s. Unexpected number of pods have been evicted, got %v, expected %v", tc.name, actualEvictedPodCount, tc.expectedEvictedPodCount)
			}
		})
	}
}

func waitPodRestartCount(ctx context.Context, clientSet clientset.Interface, namespace string, t *testing.T) (bool, error) {
	timeout := time.After(5 * time.Minute)
	tick := time.Tick(5 * time.Second)
	for {
		select {
		case <-timeout:
			t.Log("Timeout, still restart count not as expected")
			return false, fmt.Errorf("timeout Error")
		case <-tick:
			podList, err := clientSet.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
				LabelSelector: labels.SelectorFromSet(labels.Set(map[string]string{"test": "restart-pod", "name": "test-toomanyrestarts"})).String(),
			})
			if err != nil {
				t.Fatalf("Unexpected err: %v", err)
				return false, err
			}
			if len(podList.Items) < 4 {
				t.Log("Waiting for 4 pods")
				return false, nil
			}
			for i := 0; i < 4; i++ {
				if len(podList.Items[0].Status.ContainerStatuses) < 1 {
					t.Logf("Waiting for podList.Items[%v].Status.ContainerStatuses to be populated", i)
					return false, nil
				}
			}

			if podList.Items[0].Status.ContainerStatuses[0].RestartCount >= 4 && podList.Items[1].Status.ContainerStatuses[0].RestartCount >= 4 && podList.Items[2].Status.ContainerStatuses[0].RestartCount >= 4 && podList.Items[3].Status.ContainerStatuses[0].RestartCount >= 4 {
				t.Log("Pod restartCount as expected")
				return true, nil
			}
		}
	}
}
