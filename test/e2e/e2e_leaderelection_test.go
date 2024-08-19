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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	clientset "k8s.io/client-go/kubernetes"
	componentbaseconfig "k8s.io/component-base/config"

	"sigs.k8s.io/descheduler/pkg/api"
	apiv1alpha2 "sigs.k8s.io/descheduler/pkg/api/v1alpha2"
	"sigs.k8s.io/descheduler/pkg/descheduler/client"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/podlifetime"
)

func podlifetimePolicy(podLifeTimeArgs *podlifetime.PodLifeTimeArgs, evictorArgs *defaultevictor.DefaultEvictorArgs) *apiv1alpha2.DeschedulerPolicy {
	return &apiv1alpha2.DeschedulerPolicy{
		Profiles: []apiv1alpha2.DeschedulerProfile{
			{
				Name: podlifetime.PluginName + "Profile",
				PluginConfigs: []apiv1alpha2.PluginConfig{
					{
						Name: podlifetime.PluginName,
						Args: runtime.RawExtension{
							Object: podLifeTimeArgs,
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
							podlifetime.PluginName,
						},
					},
				},
			},
		},
	}
}

func TestLeaderElection(t *testing.T) {
	ctx := context.Background()

	clientSet, err := client.CreateClient(componentbaseconfig.ClientConnectionConfiguration{Kubeconfig: os.Getenv("KUBECONFIG")}, "")
	if err != nil {
		t.Errorf("Error during kubernetes client creation with %v", err)
	}

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

	testLabel := map[string]string{"test": "leaderelection", "name": "test-leaderelection"}
	deployment1 := buildTestDeployment("leaderelection", ns1, 5, testLabel, nil)
	err = createDeployment(t, ctx, clientSet, deployment1)
	if err != nil {
		t.Fatalf("create deployment 1: %v", err)
	}

	deployment2 := buildTestDeployment("leaderelection", ns2, 5, testLabel, nil)
	err = createDeployment(t, ctx, clientSet, deployment2)
	if err != nil {
		t.Fatalf("create deployment 2: %v", err)
	}
	defer func() {
		clientSet.AppsV1().Deployments(deployment1.Namespace).Delete(ctx, deployment1.Name, metav1.DeleteOptions{})
		clientSet.AppsV1().Deployments(deployment2.Namespace).Delete(ctx, deployment2.Name, metav1.DeleteOptions{})
	}()

	waitForPodsRunning(ctx, t, clientSet, deployment1.Labels, 5, deployment1.Namespace)
	podListAOrg := getCurrentPodNames(t, ctx, clientSet, ns1)

	waitForPodsRunning(ctx, t, clientSet, deployment2.Labels, 5, deployment2.Namespace)
	podListBOrg := getCurrentPodNames(t, ctx, clientSet, ns2)

	t.Log("Starting deschedulers")
	// Delete the descheduler lease
	err = clientSet.CoordinationV1().Leases("kube-system").Delete(ctx, "descheduler", metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		t.Fatalf("Unable to remove kube-system/descheduler lease: %v", err)
	}
	t.Logf("Removed kube-system/descheduler lease")
	descheduler1 := startDeschedulerServer(t, ctx, clientSet, ns1)
	time.Sleep(1 * time.Second)
	descheduler2 := startDeschedulerServer(t, ctx, clientSet, ns2)
	defer func() {
		for _, deploy := range []*appsv1.Deployment{descheduler1, descheduler2} {
			printPodLogs(ctx, t, clientSet, deploy.Name)
			t.Logf("Deleting %q deployment...", deploy.Name)
			err = clientSet.AppsV1().Deployments(deploy.Namespace).Delete(ctx, deploy.Name, metav1.DeleteOptions{})
			if err != nil {
				t.Fatalf("Unable to delete %q deployment: %v", deploy.Name, err)
			}

			waitForPodsToDisappear(ctx, t, clientSet, deploy.Labels, deploy.Namespace)
		}

		clientSet.CoordinationV1().Leases("kube-system").Delete(ctx, "descheduler", metav1.DeleteOptions{})
	}()

	// wait for a while so all the pods are 5 seconds older
	time.Sleep(7 * time.Second)

	// validate only pods from e2e-testleaderelection-a namespace are evicted.
	podListA := getCurrentPodNames(t, ctx, clientSet, ns1)
	podListB := getCurrentPodNames(t, ctx, clientSet, ns2)

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
		if left && right {
			t.Fatalf("No pods evicted. Probably none of the deschedulers were running.")
		} else {
			t.Fatalf("Pods are evicted in both namespaces.\n\tFor %s namespace\n\tPods before: %s,\n\tPods after %s.\n\tAnd, for %s namespace\n\tPods before: %s,\n\tPods after: %s", ns1, podListAOrg, podListA, ns2, podListBOrg, podListB)
		}
	}
}

func createDeployment(t *testing.T, ctx context.Context, clientSet clientset.Interface, deployment *appsv1.Deployment) error {
	t.Logf("Creating deployment %v for namespace %s", deployment.Name, deployment.Namespace)
	_, err := clientSet.AppsV1().Deployments(deployment.Namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		t.Logf("Error creating deployment: %v", err)
		if err = clientSet.AppsV1().Deployments(deployment.Namespace).DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{
			LabelSelector: labels.SelectorFromSet(deployment.Labels).String(),
		}); err != nil {
			t.Fatalf("Unable to delete deployment: %v", err)
		}
		return fmt.Errorf("create deployment %v", err)
	}
	return nil
}

func startDeschedulerServer(t *testing.T, ctx context.Context, clientSet clientset.Interface, testName string) *appsv1.Deployment {
	var maxLifeTime uint = 5
	podLifeTimeArgs := &podlifetime.PodLifeTimeArgs{
		MaxPodLifeTimeSeconds: &maxLifeTime,
		Namespaces: &api.Namespaces{
			Include: []string{testName},
		},
	}

	// Deploy the descheduler with the configured policy
	evictorArgs := &defaultevictor.DefaultEvictorArgs{
		EvictLocalStoragePods:   true,
		EvictSystemCriticalPods: false,
		IgnorePvcPods:           false,
		EvictFailedBarePods:     false,
		MinPodAge:               &metav1.Duration{Duration: 1 * time.Second},
	}
	deschedulerPolicyConfigMapObj, err := deschedulerPolicyConfigMap(podlifetimePolicy(podLifeTimeArgs, evictorArgs))
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

	deschedulerDeploymentObj := deschedulerDeployment(testName)
	deschedulerDeploymentObj.Name = fmt.Sprintf("%s-%s", deschedulerDeploymentObj.Name, testName)
	t.Logf("Creating descheduler deployment %v", deschedulerDeploymentObj.Name)
	_, err = clientSet.AppsV1().Deployments(deschedulerDeploymentObj.Namespace).Create(ctx, deschedulerDeploymentObj, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Error creating %q deployment: %v", deschedulerDeploymentObj.Name, err)
	}

	t.Logf("Waiting for the descheduler pod running")
	waitForPodsRunning(ctx, t, clientSet, deschedulerDeploymentObj.Labels, 1, deschedulerDeploymentObj.Namespace)
	return deschedulerDeploymentObj
}
