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
	utilptr "k8s.io/utils/ptr"
	"sigs.k8s.io/descheduler/pkg/api"
	apiv1alpha2 "sigs.k8s.io/descheduler/pkg/api/v1alpha2"
	"sigs.k8s.io/descheduler/pkg/descheduler"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/podlifetime"
)

const deploymentReplicas = 5 // May not be the best value

func PodLifeTimePolicy(targetNamespace string, maxPodLifeTimeSeconds *uint) *apiv1alpha2.DeschedulerPolicy {
	return &apiv1alpha2.DeschedulerPolicy{
		Profiles: []apiv1alpha2.DeschedulerProfile{
			{
				Name: "PodLifeTimeProfile",
				PluginConfigs: []apiv1alpha2.PluginConfig{
					{
						Name: podlifetime.PluginName,
						Args: runtime.RawExtension{
							Object: &podlifetime.PodLifeTimeArgs{
								MaxPodLifeTimeSeconds: maxPodLifeTimeSeconds,
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
							podlifetime.PluginName,
						},
					},
				},
			},
		},
	}
}

func TestLeaderElection(t *testing.T) {
	descheduler.SetupPlugins()
	ctx := context.Background()

	clientSet, _, _, _ := initializeClient(ctx, t)

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

	deploymentObj1, err := createDeployment(ctx, clientSet, ns1, deploymentReplicas, t)
	if err != nil {
		t.Fatalf("create deployment 1: %v", err)
	}
	defer clientSet.AppsV1().Deployments(deploymentObj1.Namespace).Delete(ctx, deploymentObj1.Name, metav1.DeleteOptions{})

	deploymentObj2, err := createDeployment(ctx, clientSet, ns2, deploymentReplicas, t)
	if err != nil {
		t.Fatalf("create deployment 2: %v", err)
	}
	defer clientSet.AppsV1().Deployments(deploymentObj2.Namespace).Delete(ctx, deploymentObj2.Name, metav1.DeleteOptions{})

	waitForPodsRunning(ctx, t, clientSet, map[string]string{"test": "leaderelection", "name": "test-leaderelection"}, 5, ns1)

	podListAOrg := getPodNameList(ctx, clientSet, ns1, t)

	waitForPodsRunning(ctx, t, clientSet, map[string]string{"test": "leaderelection", "name": "test-leaderelection"}, 5, ns2)

	podListBOrg := getPodNameList(ctx, clientSet, ns2, t)

	// Delete the descheduler lease
	err = clientSet.CoordinationV1().Leases("kube-system").Delete(ctx, "descheduler", metav1.DeleteOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			t.Fatalf("Unable to remove kube-system/descheduler lease: %v", err)
		}
	}
	t.Logf("Removed kube-system/descheduler lease")

	t.Log("starting deschedulers deployments with leader election enabled")

	deschedulerDeploymentObj1 := createDeschedulerDeploymentWithPolicyConfigMap(t, ns1)
	deschedulerDeploymentObj2 := createDeschedulerDeploymentWithPolicyConfigMap(t, ns2)

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
		if left && right {
			t.Fatalf("No pods evicted. Probably none of the deschedulers were running.")
		} else {
			t.Fatalf("Pods are evicted in both namespaces.\n\tFor %s namespace\n\tPods before: %s,\n\tPods after %s.\n\tAnd, for %s namespace\n\tPods before: %s,\n\tPods after: %s", ns1, podListAOrg, podListA, ns2, podListBOrg, podListB)
		}
	}
}

func createDeployment(ctx context.Context, clientSet clientset.Interface, namespace string, replicas int32, t *testing.T) (*appsv1.Deployment, error) {
	deploymentObj := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "leaderelection",
			Namespace: namespace,
			Labels:    map[string]string{"test": "leaderelection", "name": "test-leaderelection"},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: utilptr.To[int32](replicas),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"test": "leaderelection", "name": "test-leaderelection"},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"test": "leaderelection", "name": "test-leaderelection"},
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

	t.Logf("Creating deployment %v for namespace %s", deploymentObj.Name, deploymentObj.Namespace)
	deploymentObj, err := clientSet.AppsV1().Deployments(deploymentObj.Namespace).Create(ctx, deploymentObj, metav1.CreateOptions{})
	if err != nil {
		t.Logf("Error creating deployment: %v", err)
		if err = clientSet.AppsV1().Deployments(deploymentObj.Namespace).DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{
			LabelSelector: labels.SelectorFromSet(labels.Set(map[string]string{"test": "leaderelection", "name": "test-leaderelection"})).String(),
		}); err != nil {
			t.Fatalf("Unable to delete deployment: %v", err)
		}
		return nil, fmt.Errorf("create deployment %v", err)
	}
	return deploymentObj, nil
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

// createDeschedulerDeploymentWithPolicyConfigMap Reuse function for 2 testNamespaces
func createDeschedulerDeploymentWithPolicyConfigMap(t *testing.T, targetNamespace string) *appsv1.Deployment {

	descheduler.SetupPlugins()
	ctx := context.Background()

	clientSet, _, _, _ := initializeClient(ctx, t)

	// test case
	tc := struct {
		name   string
		policy *apiv1alpha2.DeschedulerPolicy
	}{
		name:   "leaderelection",
		policy: PodLifeTimePolicy(targetNamespace, utilptr.To[uint](5)),
	}

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

	deschedulerDeploymentObj := deschedulerDeployment("kube-system")
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
		waitForDeschedulerPodAbsent(t, ctx, clientSet, "kube-system")
	}()

	t.Logf("Waiting for the descheduler pod running")
	deschedulerPodName = waitForDeschedulerPodRunning(t, ctx, clientSet, "kube-system")

	return deschedulerDeploymentObj

}
