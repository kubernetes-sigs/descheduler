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
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/events"
	"k8s.io/utils/pointer"

	utilpointer "k8s.io/utils/pointer"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	eutils "sigs.k8s.io/descheduler/pkg/descheduler/evictions/utils"
	frameworkfake "sigs.k8s.io/descheduler/pkg/framework/fake"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodshavingtoomanyrestarts"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
)

func TestTooManyRestarts(t *testing.T) {
	ctx := context.Background()

	clientSet, sharedInformerFactory, _, getPodsAssignedToNode := initializeClient(ctx, t)

	nodeList, err := clientSet.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Errorf("Error listing node with %v", err)
	}

	_, workerNodes := splitNodesAndWorkerNodes(nodeList.Items)

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
			Replicas: pointer.Int32(4),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"test": "restart-pod", "name": "test-toomanyrestarts"},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"test": "restart-pod", "name": "test-toomanyrestarts"},
				},
				Spec: v1.PodSpec{
					SecurityContext: &v1.PodSecurityContext{
						RunAsNonRoot: utilpointer.Bool(true),
						RunAsUser:    utilpointer.Int64(1000),
						RunAsGroup:   utilpointer.Int64(1000),
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
							AllowPrivilegeEscalation: utilpointer.Bool(false),
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

	createRemovePodsHavingTooManyRestartsAgrs := func(
		podRestartThresholds int32,
		includingInitContainers bool,
	) removepodshavingtoomanyrestarts.RemovePodsHavingTooManyRestartsArgs {
		return removepodshavingtoomanyrestarts.RemovePodsHavingTooManyRestartsArgs{
			PodRestartThreshold:     podRestartThresholds,
			IncludingInitContainers: includingInitContainers,
		}
	}

	tests := []struct {
		name                    string
		args                    removepodshavingtoomanyrestarts.RemovePodsHavingTooManyRestartsArgs
		expectedEvictedPodCount uint
	}{
		{
			name:                    "test-no-evictions",
			args:                    createRemovePodsHavingTooManyRestartsAgrs(10000, true),
			expectedEvictedPodCount: 0,
		},
		{
			name:                    "test-one-evictions",
			args:                    createRemovePodsHavingTooManyRestartsAgrs(4, true),
			expectedEvictedPodCount: 4,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			evictionPolicyGroupVersion, err := eutils.SupportEviction(clientSet)
			if err != nil || len(evictionPolicyGroupVersion) == 0 {
				t.Fatalf("Error creating eviction policy group: %v", err)
			}

			eventRecorder := &events.FakeRecorder{}

			podEvictor := evictions.NewPodEvictor(
				clientSet,
				evictionPolicyGroupVersion,
				false,
				nil,
				nil,
				false,
				eventRecorder,
			)

			defaultevictorArgs := &defaultevictor.DefaultEvictorArgs{
				EvictLocalStoragePods:   true,
				EvictSystemCriticalPods: false,
				IgnorePvcPods:           false,
				EvictFailedBarePods:     false,
			}

			evictorFilter, err := defaultevictor.New(
				defaultevictorArgs,
				&frameworkfake.HandleImpl{
					ClientsetImpl:                 clientSet,
					GetPodsAssignedToNodeFuncImpl: getPodsAssignedToNode,
					SharedInformerFactoryImpl:     sharedInformerFactory,
				},
			)
			if err != nil {
				t.Fatalf("Unable to initialize the plugin: %v", err)
			}

			plugin, err := removepodshavingtoomanyrestarts.New(
				&tc.args,
				&frameworkfake.HandleImpl{
					ClientsetImpl:                 clientSet,
					PodEvictorImpl:                podEvictor,
					EvictorFilterImpl:             evictorFilter.(frameworktypes.EvictorPlugin),
					SharedInformerFactoryImpl:     sharedInformerFactory,
					GetPodsAssignedToNodeFuncImpl: getPodsAssignedToNode,
				})
			if err != nil {
				t.Fatalf("Unable to initialize the plugin: %v", err)
			}

			// Run RemovePodsHavingTooManyRestarts strategy
			t.Log("Running RemovePodsHavingTooManyRestarts strategy")
			plugin.(frameworktypes.DeschedulePlugin).Deschedule(ctx, workerNodes)
			t.Logf("Finished RemoveFailedPods strategy for %s", tc.name)

			waitForTerminatingPodsToDisappear(ctx, t, clientSet, testNamespace.Name)
			actualEvictedPodCount := podEvictor.TotalEvicted()
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
