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
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	deschedulerapi "sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	eutils "sigs.k8s.io/descheduler/pkg/descheduler/evictions/utils"
	"sigs.k8s.io/descheduler/pkg/descheduler/strategies"
)

func TestRemoveDuplicates(t *testing.T) {
	ctx := context.Background()

	clientSet, _, stopCh := initializeClient(t)
	defer close(stopCh)

	nodeList, err := clientSet.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Errorf("Error listing node with %v", err)
	}

	nodes, workerNodes := splitNodesAndWorkerNodes(nodeList.Items)

	t.Log("Creating testing namespace")
	testNamespace := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "e2e-" + strings.ToLower(t.Name())}}
	if _, err := clientSet.CoreV1().Namespaces().Create(ctx, testNamespace, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Unable to create ns %v", testNamespace.Name)
	}
	defer clientSet.CoreV1().Namespaces().Delete(ctx, testNamespace.Name, metav1.DeleteOptions{})

	t.Log("Creating duplicates pods")

	deploymentObj := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "duplicate-pod",
			Namespace: testNamespace.Name,
			Labels:    map[string]string{"app": "test-duplicate", "name": "test-duplicatePods"},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test-duplicate", "name": "test-duplicatePods"},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test-duplicate", "name": "test-duplicatePods"},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{
						Name:            "pause",
						ImagePullPolicy: "Always",
						Image:           "kubernetes/pause",
						Ports:           []v1.ContainerPort{{ContainerPort: 80}},
					}},
				},
			},
		},
	}

	tests := []struct {
		description             string
		replicasNum             int
		beforeFunc              func(deployment *appsv1.Deployment)
		expectedEvictedPodCount int
	}{
		{
			description: "Evict Pod even Pods schedule to specific node",
			replicasNum: 4,
			beforeFunc: func(deployment *appsv1.Deployment) {
				deployment.Spec.Replicas = func(i int32) *int32 { return &i }(4)
				deployment.Spec.Template.Spec.NodeName = workerNodes[0].Name
			},
			expectedEvictedPodCount: 2,
		},
		{
			description: "Evict Pod even Pods with local storage",
			replicasNum: 5,
			beforeFunc: func(deployment *appsv1.Deployment) {
				deployment.Spec.Replicas = func(i int32) *int32 { return &i }(5)
				deployment.Spec.Template.Spec.Volumes = []v1.Volume{
					{
						Name: "sample",
						VolumeSource: v1.VolumeSource{
							EmptyDir: &v1.EmptyDirVolumeSource{
								SizeLimit: resource.NewQuantity(int64(10), resource.BinarySI)},
						},
					},
				}
			},
			expectedEvictedPodCount: 2,
		},
	}
	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			t.Logf("Creating deployment %v in %v namespace", deploymentObj.Name, deploymentObj.Namespace)
			tc.beforeFunc(deploymentObj)

			_, err = clientSet.AppsV1().Deployments(deploymentObj.Namespace).Create(ctx, deploymentObj, metav1.CreateOptions{})
			if err != nil {
				t.Logf("Error creating deployment: %v", err)
				if err = clientSet.AppsV1().Deployments(deploymentObj.Namespace).DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{
					LabelSelector: labels.SelectorFromSet(labels.Set(map[string]string{"app": "test-duplicate", "name": "test-duplicatePods"})).String(),
				}); err != nil {
					t.Fatalf("Unable to delete deployment: %v", err)
				}
				return
			}
			defer clientSet.AppsV1().Deployments(deploymentObj.Namespace).Delete(ctx, deploymentObj.Name, metav1.DeleteOptions{})
			waitForPodsRunning(ctx, t, clientSet, map[string]string{"app": "test-duplicate", "name": "test-duplicatePods"}, tc.replicasNum, testNamespace.Name)

			// Run DeschedulerStrategy strategy
			evictionPolicyGroupVersion, err := eutils.SupportEviction(clientSet)
			if err != nil || len(evictionPolicyGroupVersion) == 0 {
				t.Fatalf("Error creating eviction policy group %v", err)
			}
			podEvictor := evictions.NewPodEvictor(
				clientSet,
				evictionPolicyGroupVersion,
				false,
				0,
				nodes,
				true,
				false,
				false,
			)

			t.Log("Running DeschedulerStrategy strategy")
			strategies.RemoveDuplicatePods(
				ctx,
				clientSet,
				deschedulerapi.DeschedulerStrategy{
					Enabled: true,
					Params: &deschedulerapi.StrategyParameters{
						RemoveDuplicates: &deschedulerapi.RemoveDuplicates{},
					},
				},
				workerNodes,
				podEvictor,
			)

			waitForTerminatingPodsToDisappear(ctx, t, clientSet, testNamespace.Name)
			actualEvictedPodCount := podEvictor.TotalEvicted()
			if actualEvictedPodCount != tc.expectedEvictedPodCount {
				t.Errorf("Test error for description: %s. Unexpected number of pods have been evicted, got %v, expected %v", tc.description, actualEvictedPodCount, tc.expectedEvictedPodCount)
			}
		})
	}
}

func waitForPodsRunning(ctx context.Context, t *testing.T, clientSet clientset.Interface, labelMap map[string]string, desireRunningPodNum int, namespace string) {
	if err := wait.PollImmediate(10*time.Second, 60*time.Second, func() (bool, error) {
		podList, err := clientSet.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: labels.SelectorFromSet(labelMap).String(),
		})
		if err != nil {
			return false, err
		}
		if len(podList.Items) != desireRunningPodNum {
			t.Logf("Waiting for %v pods to be running, got %v instead", desireRunningPodNum, len(podList.Items))
			return false, nil
		}
		for _, pod := range podList.Items {
			if pod.Status.Phase != v1.PodRunning {
				t.Logf("Pod %v not running yet, is %v instead", pod.Name, pod.Status.Phase)
				return false, nil
			}
		}
		return true, nil
	}); err != nil {
		t.Fatalf("Error waiting for pods running: %v", err)
	}
}
