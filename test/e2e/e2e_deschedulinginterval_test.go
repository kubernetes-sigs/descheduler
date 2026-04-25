/*
Copyright 2024 The Kubernetes Authors.

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
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	componentbaseconfig "k8s.io/component-base/config"

	apiv1alpha2 "sigs.k8s.io/descheduler/pkg/api/v1alpha2"
	"sigs.k8s.io/descheduler/pkg/descheduler/client"
)

// TestDeschedulingInterval verifies that without `--descheduling-interval` set
// (or with the default of 0), the descheduler binary runs a single pass and
// then exits cleanly.
//
// The previous version of this test invoked descheduler.RunDeschedulerStrategies
// in-process; that no longer reflects how the descheduler is shipped (an image
// running as a workload), so we now run it as a Pod with restartPolicy=Never
// and assert it terminates with `Succeeded` within a generous timeout.
func TestDeschedulingInterval(t *testing.T) {
	ctx := context.Background()

	clientSet, err := client.CreateClient(componentbaseconfig.ClientConnectionConfiguration{Kubeconfig: os.Getenv("KUBECONFIG")}, "")
	if err != nil {
		t.Errorf("Error during kubernetes client creation with %v", err)
	}

	// Empty policy is fine — we are testing the run-once-and-exit lifecycle, not
	// any specific descheduling decision.
	deschedulerPolicyConfigMapObj, err := deschedulerPolicyConfigMap(&apiv1alpha2.DeschedulerPolicy{})
	if err != nil {
		t.Fatalf("Error building descheduler policy configmap: %v", err)
	}

	t.Logf("Creating %q policy CM ...", deschedulerPolicyConfigMapObj.Name)
	if _, err := clientSet.CoreV1().ConfigMaps(deschedulerPolicyConfigMapObj.Namespace).Create(ctx, deschedulerPolicyConfigMapObj, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Error creating %q CM: %v", deschedulerPolicyConfigMapObj.Name, err)
	}
	defer func() {
		t.Logf("Deleting %q CM...", deschedulerPolicyConfigMapObj.Name)
		if err := clientSet.CoreV1().ConfigMaps(deschedulerPolicyConfigMapObj.Namespace).Delete(ctx, deschedulerPolicyConfigMapObj.Name, metav1.DeleteOptions{}); err != nil {
			t.Fatalf("Unable to delete %q CM: %v", deschedulerPolicyConfigMapObj.Name, err)
		}
	}()

	// Build a single-run Pod from the standard descheduler deployment template,
	// dropping the `--descheduling-interval` flag so the descheduler exits after
	// one iteration, and setting RestartPolicy=Never so the Pod doesn't get
	// restarted by the kubelet on completion.
	pod := newDeschedulerOneShotPod("descheduling-interval", deschedulerPolicyConfigMapObj.Name)

	t.Logf("Creating one-shot descheduler pod %v", pod.Name)
	if _, err := clientSet.CoreV1().Pods(pod.Namespace).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Error creating descheduler pod %q: %v", pod.Name, err)
	}
	defer func() {
		printPodLogs(ctx, t, clientSet, pod.Name)
		t.Logf("Deleting descheduler pod %q...", pod.Name)
		if err := clientSet.CoreV1().Pods(pod.Namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{}); err != nil {
			t.Fatalf("Unable to delete descheduler pod %q: %v", pod.Name, err)
		}
	}()

	t.Logf("Waiting for descheduler pod %q to reach Succeeded phase", pod.Name)
	if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
		current, err := clientSet.CoreV1().Pods(pod.Namespace).Get(ctx, pod.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		switch current.Status.Phase {
		case v1.PodSucceeded:
			return true, nil
		case v1.PodFailed:
			return false, nil
		default:
			t.Logf("Pod %q is in phase %q, waiting...", pod.Name, current.Status.Phase)
			return false, nil
		}
	}); err != nil {
		t.Errorf("Descheduler pod did not run-once-and-exit within timeout: %v", err)
	}
}

// newDeschedulerOneShotPod builds a Pod that runs the descheduler image once
// and exits, mirroring the container/security/volume configuration produced
// by deschedulerDeployment. Differences from the Deployment helper:
//
//   - RestartPolicy is Never (Pods default to Always otherwise).
//   - The `--descheduling-interval=100m` argument is dropped so the descheduler
//     follows the run-once-and-exit code path.
//   - No liveness probe — a one-shot run isn't long enough to make probing
//     meaningful, and a slow first probe could race the clean exit.
func newDeschedulerOneShotPod(testName, policyConfigMapName string) *v1.Pod {
	deploymentTemplate := deschedulerDeployment(testName)
	podTemplate := deploymentTemplate.Spec.Template

	podSpec := podTemplate.Spec
	podSpec.RestartPolicy = v1.RestartPolicyNever

	container := podSpec.Containers[0]
	container.Args = []string{
		"--policy-config-file", "/policy-dir/policy.yaml",
		"--v", "4",
	}
	container.LivenessProbe = nil
	podSpec.Containers[0] = container

	for i, vol := range podSpec.Volumes {
		if vol.ConfigMap != nil && vol.ConfigMap.LocalObjectReference.Name == "descheduler-policy-configmap" {
			podSpec.Volumes[i].ConfigMap.LocalObjectReference.Name = policyConfigMapName
		}
	}

	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "descheduler-oneshot-" + testName,
			Namespace: deploymentTemplate.Namespace,
			Labels:    deploymentTemplate.Spec.Template.Labels,
		},
		Spec: podSpec,
	}
}
