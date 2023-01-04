/*
Copyright 2022 The Kubernetes Authors.

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
	"reflect"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/utils/pointer"

	utilpointer "k8s.io/utils/pointer"
	"sigs.k8s.io/descheduler/cmd/descheduler/app/options"
	"sigs.k8s.io/descheduler/pkg/descheduler"
)

func TestLeaderElection(t *testing.T) {
	descheduler.SetupPlugins()
	ctx := context.Background()

	clientSet, _, _, _, stopCh := initializeClient(t)
	defer close(stopCh)

	ns1 := "e2e-" + strings.ToLower(t.Name()+"-a")
	ns2 := "e2e-" + strings.ToLower(t.Name()+"-b")

	t.Logf("Creating testing namespace %v", ns1)
	testNamespace1 := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns1}}
	if _, err := clientSet.CoreV1().Namespaces().Create(ctx, testNamespace1, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Unable to create ns %v", testNamespace1.Name)
	}
	defer clientSet.CoreV1().Namespaces().Delete(ctx, testNamespace1.Name, metav1.DeleteOptions{})

	t.Logf("Creating testing namespace %v", ns2)
	testNamespace2 := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns2}}
	if _, err := clientSet.CoreV1().Namespaces().Create(ctx, testNamespace2, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Unable to create ns %v", testNamespace2.Name)
	}
	defer clientSet.CoreV1().Namespaces().Delete(ctx, testNamespace2.Name, metav1.DeleteOptions{})

	deployment1, err := createDeployment(ctx, clientSet, ns1, 5, t)
	if err != nil {
		t.Fatalf("create deployment 1: %v", err)
	}
	defer clientSet.AppsV1().Deployments(deployment1.Namespace).Delete(ctx, deployment1.Name, metav1.DeleteOptions{})

	deployment2, err := createDeployment(ctx, clientSet, ns2, 5, t)
	if err != nil {
		t.Fatalf("create deployment 2: %v", err)
	}
	defer clientSet.AppsV1().Deployments(deployment2.Namespace).Delete(ctx, deployment2.Name, metav1.DeleteOptions{})

	waitForPodsRunning(ctx, t, clientSet, map[string]string{"test": "leaderelection", "name": "test-leaderelection"}, 5, ns1)

	podListAOrg := getPodNameList(ctx, clientSet, ns1, t)

	waitForPodsRunning(ctx, t, clientSet, map[string]string{"test": "leaderelection", "name": "test-leaderelection"}, 5, ns2)

	podListBOrg := getPodNameList(ctx, clientSet, ns2, t)

	s1, err := options.NewDeschedulerServer()
	if err != nil {
		t.Fatalf("unable to initialize server: %v", err)
	}
	s1.Client = clientSet
	s1.DeschedulingInterval = 5 * time.Second
	s1.LeaderElection.LeaderElect = true
	s1.ClientConnection.Kubeconfig = os.Getenv("KUBECONFIG")
	s1.PolicyConfigFile = "./policy_leaderelection_a.yaml"

	s2, err := options.NewDeschedulerServer()
	if err != nil {
		t.Fatalf("unable to initialize server: %v", err)
	}
	s2.Client = clientSet
	s2.DeschedulingInterval = 5 * time.Second
	s2.LeaderElection.LeaderElect = true
	s2.ClientConnection.Kubeconfig = os.Getenv("KUBECONFIG")
	s2.PolicyConfigFile = "./policy_leaderelection_b.yaml"

	t.Log("starting deschedulers")

	go func() {
		err := descheduler.Run(ctx, s1)
		if err != nil {
			t.Errorf("unable to start descheduler: %v", err)
			return
		}
	}()

	time.Sleep(1 * time.Second)

	go func() {
		err := descheduler.Run(ctx, s2)
		if err != nil {
			t.Errorf("unable to start descheduler: %v", err)
			return
		}
	}()

	defer clientSet.CoordinationV1().Leases(s1.LeaderElection.ResourceNamespace).Delete(ctx, s1.LeaderElection.ResourceName, metav1.DeleteOptions{})
	defer clientSet.CoordinationV1().Leases(s2.LeaderElection.ResourceNamespace).Delete(ctx, s2.LeaderElection.ResourceName, metav1.DeleteOptions{})

	// wait for a while so all the pods are 5 seconds older
	time.Sleep(7 * time.Second)

	// validate only pods from e2e-testleaderelection-a namespace are evicted.
	podListA := getPodNameList(ctx, clientSet, ns1, t)

	podListB := getPodNameList(ctx, clientSet, ns2, t)

	left := reflect.DeepEqual(podListAOrg, podListA)
	right := reflect.DeepEqual(podListBOrg, podListB)

	singleNamespaceEvicted := (left && !right) || (!left && right)

	if singleNamespaceEvicted {
		if !left {
			t.Logf("Only the pods in %s namespace are evicted. Pods before: %s, Pods after %s", ns1, podListAOrg, podListA)
		} else {
			t.Logf("Only the pods in %s namespace are evicted. Pods before: %s, Pods after %s", ns2, podListBOrg, podListB)
		}
	} else {
		t.Fatalf("Pods are evicted in both namespaces. For %s namespace Pods before: %s, Pods after %s. And, for %s namespace Pods before: %s, Pods after: %s", ns1, podListAOrg, podListA, ns2, podListBOrg, podListB)
	}
}

func createDeployment(ctx context.Context, clientSet clientset.Interface, namespace string, replicas int32, t *testing.T) (*appsv1.Deployment, error) {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "leaderelection",
			Namespace: namespace,
			Labels:    map[string]string{"test": "leaderelection", "name": "test-leaderelection"},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointer.Int32(replicas),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"test": "leaderelection", "name": "test-leaderelection"},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"test": "leaderelection", "name": "test-leaderelection"},
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
						Image:           "kubernetes/pause",
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

	t.Logf("Creating deployment %v for namespace %s", deployment.Name, deployment.Namespace)
	deployment, err := clientSet.AppsV1().Deployments(deployment.Namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		t.Logf("Error creating deployment: %v", err)
		if err = clientSet.AppsV1().Deployments(deployment.Namespace).DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{
			LabelSelector: labels.SelectorFromSet(labels.Set(map[string]string{"test": "leaderelection", "name": "test-leaderelection"})).String(),
		}); err != nil {
			t.Fatalf("Unable to delete deployment: %v", err)
		}
		return nil, fmt.Errorf("create deployment %v", err)
	}
	return deployment, nil
}

func getPodNameList(ctx context.Context, clientSet clientset.Interface, namespace string, t *testing.T) []string {
	podList, err := clientSet.CoreV1().Pods(namespace).List(
		ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(labels.Set(map[string]string{"test": "leaderelection", "name": "test-leaderelection"})).String()})
	if err != nil {
		t.Fatalf("Unable to list pods from ns: %s: %v", namespace, err)
	}
	podNames := make([]string, len(podList.Items))
	for i, pod := range podList.Items {
		podNames[i] = pod.Name
	}
	return podNames
}
