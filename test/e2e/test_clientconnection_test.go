package e2e

import (
	"context"
	"os"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	componentbaseconfig "k8s.io/component-base/config"

	apiv1alpha2 "sigs.k8s.io/descheduler/pkg/api/v1alpha2"
	"sigs.k8s.io/descheduler/pkg/descheduler/client"
)

func TestClientConnectionConfiguration(t *testing.T) {
	ctx := context.Background()
	testName := "test-clientconnection"
	clientSet, err := client.CreateClient(componentbaseconfig.ClientConnectionConfiguration{
		Kubeconfig: os.Getenv("KUBECONFIG"),
		QPS:        50,
		Burst:      100,
	}, "")
	if err != nil {
		t.Errorf("Error during kubernetes client creation with %v", err)
	}

	deschedulerPolicyConfigMapObj, err := deschedulerPolicyConfigMap(&apiv1alpha2.DeschedulerPolicy{})
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

	deschedulerDeploymentObj := deschedulerDeployment(testName)
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
	deschedulerPodName = waitForPodsRunning(ctx, t, clientSet, map[string]string{"app": "descheduler", "test": testName}, 1, deschedulerDeploymentObj.Namespace)
}
