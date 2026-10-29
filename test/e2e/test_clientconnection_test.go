package e2e

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	componentbaseconfig "k8s.io/component-base/config"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/client"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/podlifetime"
)

// TestClientConnectionConfiguration verifies the descheduler keeps working when
// it is fed a custom client connection configuration, and (see the TODO below)
// that the configuration actually reaches the REST client it builds.
//
// The descheduler is deployed from the shipped image with
// --client-connection-qps=1 and --client-connection-burst=1, so every kube
// client call after the first one waits for a token. The policy is a plain
// podlifetime profile: the pods in the test namespace are older than
// maxPodLifeTimeSeconds, hence a single descheduling pass has to evict all of
// them.
func TestClientConnectionConfiguration(t *testing.T) {
	ctx := context.Background()

	clientSet, err := client.CreateClient(componentbaseconfig.ClientConnectionConfiguration{Kubeconfig: os.Getenv("KUBECONFIG")}, "")
	if err != nil {
		t.Fatalf("Error during kubernetes client creation with %v", err)
	}

	namespace := "e2e-" + strings.ToLower(t.Name())
	t.Logf("Creating testing namespace %v", namespace)
	testNamespace := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
	if _, err := clientSet.CoreV1().Namespaces().Create(ctx, testNamespace, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Unable to create ns %v", testNamespace.Name)
	}
	defer clientSet.CoreV1().Namespaces().Delete(ctx, testNamespace.Name, metav1.DeleteOptions{})

	podsCount := 5
	testLabel := map[string]string{"test": "client-connection-configuration"}
	t.Logf("Creating %v pods for the descheduler to evict", podsCount)
	deployment := buildTestDeployment("client-connection-configuration", namespace, int32(podsCount), testLabel, nil)
	if _, err := clientSet.AppsV1().Deployments(namespace).Create(ctx, deployment, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Error creating %q deployment: %v", deployment.Name, err)
	}
	defer func() {
		clientSet.AppsV1().Deployments(namespace).Delete(ctx, deployment.Name, metav1.DeleteOptions{})
		waitForPodsToDisappear(ctx, t, clientSet, deployment.Labels, namespace)
	}()

	waitForPodsRunning(ctx, t, clientSet, deployment.Labels, podsCount, namespace)

	// podlifetime only looks at pods older than maxPodLifeTimeSeconds, so let the
	// pods age past the threshold before the descheduler starts.
	maxPodLifeTimeSeconds := uint(5)
	time.Sleep(time.Duration(maxPodLifeTimeSeconds+2) * time.Second)

	preRunNames := sets.NewString(getCurrentPodNames(ctx, clientSet, namespace, t)...)

	policyConfigMap, err := deschedulerPolicyConfigMap(podlifetimePolicy(
		&podlifetime.PodLifeTimeArgs{
			MaxPodLifeTimeSeconds: &maxPodLifeTimeSeconds,
			Namespaces:            &api.Namespaces{Include: []string{namespace}},
		},
		&defaultevictor.DefaultEvictorArgs{EvictLocalStoragePods: true},
	))
	if err != nil {
		t.Fatalf("Error creating %q CM: %v", policyConfigMap.Name, err)
	}
	createConfigMapWithCleanup(t, ctx, clientSet, policyConfigMap)

	deschedulerDeploymentObj := deschedulerDeployment(namespace)
	deschedulerDeploymentObj.Spec.Template.Spec.Containers[0].Args = []string{
		"--policy-config-file", "/policy-dir/policy.yaml",
		"--descheduling-interval", "100m",
		"--v", "4",
		"--client-connection-qps", "1",
		"--client-connection-burst", "1",
	}
	deschedulerPodName := createAndWaitForDeschedulerRunning(t, ctx, clientSet, deschedulerDeploymentObj)
	startedAt := time.Now()
	defer func() {
		printPodLogs(ctx, t, clientSet, deschedulerPodName)

		t.Logf("Deleting %q deployment...", deschedulerDeploymentObj.Name)
		if err := clientSet.AppsV1().Deployments(deschedulerDeploymentObj.Namespace).Delete(ctx, deschedulerDeploymentObj.Name, metav1.DeleteOptions{}); err != nil {
			t.Fatalf("Unable to delete %q deployment: %v", deschedulerDeploymentObj.Name, err)
		}
		waitForPodsToDisappear(ctx, t, clientSet, deschedulerDeploymentObj.Labels, deschedulerDeploymentObj.Namespace)
	}()

	var evictedPods int
	if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 180*time.Second, true, func(ctx context.Context) (bool, error) {
		currentRunNames := sets.NewString(getCurrentPodNames(ctx, clientSet, namespace, t)...)
		evictedPods = preRunNames.Difference(currentRunNames).Len()
		if evictedPods != podsCount {
			t.Logf("Expecting %v number of pods evicted, got %v instead", podsCount, evictedPods)
			return false, nil
		}
		return true, nil
	}); err != nil {
		t.Fatalf("Expected %v evicted pods, got %v: %v", podsCount, evictedPods, err)
	}

	// TODO: assert that the client connection configuration is honoured rather
	// than that the descheduler merely keeps working. One pass needs at least 7
	// kube client calls (list nodes, list pods, eviction subresource discovery
	// and one create eviction per pod), so at 1 qps/burst 1 it cannot complete
	// in under ~5s, while the default 5/10 keeps the whole pass well inside the
	// burst and finishes in under a second. The pass therefore has to be timed
	// against a stable reference point - "the container started" - which
	// startedAt above approximates only loosely (the first calls may already be
	// issued while the pod is still being observed as running).
	t.Logf("Descheduling pass took %v after the descheduler pod was seen running", time.Since(startedAt))
}
