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
	"fmt"
	"os"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	componentbaseconfig "k8s.io/component-base/config"

	apiv1alpha2 "sigs.k8s.io/descheduler/pkg/api/v1alpha2"
	"sigs.k8s.io/descheduler/pkg/descheduler/client"
)

// TestDeschedulingInterval verifies that with `--descheduling-interval` set to
// 0 (the default), the descheduler binary runs a single pass and then exits
// cleanly.
func TestDeschedulingInterval(t *testing.T) {
	ctx := context.Background()

	clientSet, err := client.CreateClient(componentbaseconfig.ClientConnectionConfiguration{Kubeconfig: os.Getenv("KUBECONFIG")}, "")
	if err != nil {
		t.Fatalf("Error during kubernetes client creation with %v", err)
	}

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

	job := deschedulerRunOnceJob("descheduling-interval")
	jobClient := clientSet.BatchV1().Jobs(job.Namespace)

	t.Logf("Creating a run-once descheduler job %q", job.Name)
	if _, err := jobClient.Create(ctx, job, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Error creating descheduler job %q: %v", job.Name, err)
	}
	defer func() {
		printJobPodLogs(ctx, t, clientSet, job)
		t.Logf("Deleting descheduler job %q...", job.Name)
		deletePropagationPolicy := metav1.DeletePropagationForeground
		if err := jobClient.Delete(ctx, job.Name, metav1.DeleteOptions{PropagationPolicy: &deletePropagationPolicy}); err != nil {
			t.Fatalf("Unable to delete descheduler job %q: %v", job.Name, err)
		}
	}()

	t.Logf("Waiting for descheduler job %q to complete", job.Name)
	if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
		current, err := jobClient.Get(ctx, job.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		for _, condition := range current.Status.Conditions {
			if condition.Status != v1.ConditionTrue {
				continue
			}
			switch condition.Type {
			case batchv1.JobComplete:
				return true, nil
			case batchv1.JobFailed:
				return false, fmt.Errorf("job %q failed: %v: %v", job.Name, condition.Reason, condition.Message)
			}
		}
		t.Logf("Job %q has not completed yet, waiting...", job.Name)
		return false, nil
	}); err != nil {
		t.Errorf("Descheduler job did not run-once-and-exit within timeout: %v", err)
	}
}

// deschedulerRunOnceJob builds a Job that runs the descheduler image once and
// exits. The liveness probe is dropped since a run-once descheduler can exit
// before the first probe fires.
func deschedulerRunOnceJob(testName string) *batchv1.Job {
	podSpec := deschedulerPodSpec("0")
	podSpec.RestartPolicy = v1.RestartPolicyNever
	podSpec.Containers[0].LivenessProbe = nil

	labelsSet := map[string]string{"app": "descheduler", "test": testName}
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "descheduler-run-once-" + testName,
			Namespace: "kube-system",
			Labels:    labelsSet,
		},
		Spec: batchv1.JobSpec{
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labelsSet,
				},
				Spec: podSpec,
			},
		},
	}
}

// printJobPodLogs prints the logs of every pod created for the given job.
func printJobPodLogs(ctx context.Context, t *testing.T, clientSet clientset.Interface, job *batchv1.Job) {
	podList, err := clientSet.CoreV1().Pods(job.Namespace).List(ctx, metav1.ListOptions{LabelSelector: labels.FormatLabels(job.Labels)})
	if err != nil {
		t.Logf("Unable to list pods of job %q: %v", job.Name, err)
		return
	}
	for _, pod := range podList.Items {
		printPodLogs(ctx, t, clientSet, pod.Name)
	}
}
