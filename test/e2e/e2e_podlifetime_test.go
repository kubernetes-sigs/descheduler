package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	utilptr "k8s.io/utils/ptr"

	deschedulerapi "sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/podlifetime"
)

func TestPodLifeTime_FailedPods(t *testing.T) {
	ctx := context.Background()

	clientSet, _, nodeLister, _ := initializeClient(ctx, t)

	t.Log("Creating testing namespace")
	testNamespace := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "e2e-" + strings.ReplaceAll(strings.ToLower(t.Name()), "_", "-")}}
	if _, err := clientSet.CoreV1().Namespaces().Create(ctx, testNamespace, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Unable to create ns %v", testNamespace.Name)
	}
	defer clientSet.CoreV1().Namespaces().Delete(ctx, testNamespace.Name, metav1.DeleteOptions{})

	tests := []struct {
		name                    string
		expectedEvictedPodCount int
		args                    *podlifetime.PodLifeTimeArgs
	}{
		{
			name:                    "test-transition-failed-pods-default",
			expectedEvictedPodCount: 1,
			args: &podlifetime.PodLifeTimeArgs{
				States: []string{string(v1.PodFailed)},
			},
		},
		{
			name:                    "test-transition-failed-pods-exclude-job",
			expectedEvictedPodCount: 0,
			args: &podlifetime.PodLifeTimeArgs{
				States:     []string{string(v1.PodFailed)},
				OwnerKinds: &podlifetime.OwnerKinds{Exclude: []string{"Job"}},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			job := initTransitionTestJob(tc.name, testNamespace.Name)
			t.Logf("Creating job %s in %s namespace", job.Name, testNamespace.Name)
			jobClient := clientSet.BatchV1().Jobs(testNamespace.Name)
			if _, err := jobClient.Create(ctx, job, metav1.CreateOptions{}); err != nil {
				t.Fatalf("Error creating Job %s: %v", tc.name, err)
			}
			deletePropagationPolicy := metav1.DeletePropagationForeground
			defer func() {
				jobClient.Delete(ctx, job.Name, metav1.DeleteOptions{PropagationPolicy: &deletePropagationPolicy})
				waitForPodsToDisappear(ctx, t, clientSet, job.Labels, testNamespace.Name)
			}()
			waitForTransitionJobPodPhase(ctx, t, clientSet, job, v1.PodFailed)

			preRunNames := sets.NewString(getCurrentPodNames(ctx, clientSet, testNamespace.Name, t)...)

			tc.args.Namespaces = &deschedulerapi.Namespaces{
				Include: []string{testNamespace.Name},
			}
			runPodLifetimePlugin(ctx, t, clientSet, nodeLister, tc.args,
				defaultevictor.DefaultEvictorArgs{EvictLocalStoragePods: true},
				nil,
			)

			var meetsExpectations bool
			var actualEvictedPodCount int
			if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 60*time.Second, true, func(ctx context.Context) (bool, error) {
				currentRunNames := sets.NewString(getCurrentPodNames(ctx, clientSet, testNamespace.Name, t)...)
				actualEvictedPod := preRunNames.Difference(currentRunNames)
				actualEvictedPodCount = actualEvictedPod.Len()
				t.Logf("preRunNames: %v, currentRunNames: %v, actualEvictedPodCount: %v\n", preRunNames.List(), currentRunNames.List(), actualEvictedPodCount)
				if actualEvictedPodCount != tc.expectedEvictedPodCount {
					t.Logf("Expecting %v number of pods evicted, got %v instead", tc.expectedEvictedPodCount, actualEvictedPodCount)
					return false, nil
				}
				meetsExpectations = true
				return true, nil
			}); err != nil {
				t.Errorf("Error waiting for expected eviction count: %v", err)
			}

			if !meetsExpectations {
				t.Errorf("Unexpected number of pods have been evicted, got %v, expected %v", actualEvictedPodCount, tc.expectedEvictedPodCount)
			} else {
				t.Logf("Total of %d Pods were evicted for %s", actualEvictedPodCount, tc.name)
			}
		})
	}
}

func TestPodLifeTime_SucceededPods(t *testing.T) {
	ctx := context.Background()

	clientSet, _, nodeLister, _ := initializeClient(ctx, t)

	t.Log("Creating testing namespace")
	testNamespace := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "e2e-" + strings.ReplaceAll(strings.ToLower(t.Name()), "_", "-")}}
	if _, err := clientSet.CoreV1().Namespaces().Create(ctx, testNamespace, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Unable to create ns %v", testNamespace.Name)
	}
	defer clientSet.CoreV1().Namespaces().Delete(ctx, testNamespace.Name, metav1.DeleteOptions{})

	tests := []struct {
		name                    string
		expectedEvictedPodCount int
		args                    *podlifetime.PodLifeTimeArgs
	}{
		{
			name:                    "test-transition-succeeded-pods",
			expectedEvictedPodCount: 1,
			args: &podlifetime.PodLifeTimeArgs{
				States: []string{string(v1.PodSucceeded)},
			},
		},
		{
			name:                    "test-transition-succeeded-condition",
			expectedEvictedPodCount: 1,
			args: &podlifetime.PodLifeTimeArgs{
				States: []string{string(v1.PodSucceeded)},
				Conditions: []podlifetime.PodConditionFilter{
					{Reason: "PodCompleted", Status: "True"},
				},
			},
		},
		{
			name:                    "test-transition-succeeded-condition-unmatched",
			expectedEvictedPodCount: 0,
			args: &podlifetime.PodLifeTimeArgs{
				States: []string{string(v1.PodSucceeded)},
				Conditions: []podlifetime.PodConditionFilter{
					{Reason: "ReasonDoesNotMatch"},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			job := initTransitionSucceededJob(tc.name, testNamespace.Name)
			t.Logf("Creating job %s in %s namespace", job.Name, testNamespace.Name)
			jobClient := clientSet.BatchV1().Jobs(testNamespace.Name)
			if _, err := jobClient.Create(ctx, job, metav1.CreateOptions{}); err != nil {
				t.Fatalf("Error creating Job %s: %v", tc.name, err)
			}
			deletePropagationPolicy := metav1.DeletePropagationForeground
			defer func() {
				jobClient.Delete(ctx, job.Name, metav1.DeleteOptions{PropagationPolicy: &deletePropagationPolicy})
				waitForPodsToDisappear(ctx, t, clientSet, job.Labels, testNamespace.Name)
			}()
			waitForTransitionJobPodPhase(ctx, t, clientSet, job, v1.PodSucceeded)

			preRunNames := sets.NewString(getCurrentPodNames(ctx, clientSet, testNamespace.Name, t)...)

			tc.args.Namespaces = &deschedulerapi.Namespaces{
				Include: []string{testNamespace.Name},
			}
			runPodLifetimePlugin(ctx, t, clientSet, nodeLister, tc.args,
				defaultevictor.DefaultEvictorArgs{EvictLocalStoragePods: true},
				nil,
			)

			var meetsExpectations bool
			var actualEvictedPodCount int
			if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 60*time.Second, true, func(ctx context.Context) (bool, error) {
				currentRunNames := sets.NewString(getCurrentPodNames(ctx, clientSet, testNamespace.Name, t)...)
				actualEvictedPod := preRunNames.Difference(currentRunNames)
				actualEvictedPodCount = actualEvictedPod.Len()
				t.Logf("preRunNames: %v, currentRunNames: %v, actualEvictedPodCount: %v\n", preRunNames.List(), currentRunNames.List(), actualEvictedPodCount)
				if actualEvictedPodCount != tc.expectedEvictedPodCount {
					t.Logf("Expecting %v number of pods evicted, got %v instead", tc.expectedEvictedPodCount, actualEvictedPodCount)
					return false, nil
				}
				meetsExpectations = true
				return true, nil
			}); err != nil {
				t.Errorf("Error waiting for expected eviction count: %v", err)
			}

			if !meetsExpectations {
				t.Errorf("Unexpected number of pods have been evicted, got %v, expected %v", actualEvictedPodCount, tc.expectedEvictedPodCount)
			} else {
				t.Logf("Total of %d Pods were evicted for %s", actualEvictedPodCount, tc.name)
			}
		})
	}
}

func initTransitionTestJob(name, namespace string) *batchv1.Job {
	podSpec := makePodSpec("", nil)
	podSpec.Containers[0].Command = []string{"/bin/false"}
	podSpec.RestartPolicy = v1.RestartPolicyNever
	labelsSet := labels.Set{"test": name, "name": name}
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Labels:    labelsSet,
			Name:      name,
			Namespace: namespace,
		},
		Spec: batchv1.JobSpec{
			Template: v1.PodTemplateSpec{
				Spec:       podSpec,
				ObjectMeta: metav1.ObjectMeta{Labels: labelsSet},
			},
			BackoffLimit: utilptr.To[int32](0),
		},
	}
}

func initTransitionSucceededJob(name, namespace string) *batchv1.Job {
	podSpec := makePodSpec("", nil)
	podSpec.Containers[0].Image = "registry.k8s.io/e2e-test-images/agnhost:2.43"
	podSpec.Containers[0].Command = []string{"/bin/sh", "-c", "exit 0"}
	podSpec.RestartPolicy = v1.RestartPolicyNever
	labelsSet := labels.Set{"test": name, "name": name}
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Labels:    labelsSet,
			Name:      name,
			Namespace: namespace,
		},
		Spec: batchv1.JobSpec{
			Template: v1.PodTemplateSpec{
				Spec:       podSpec,
				ObjectMeta: metav1.ObjectMeta{Labels: labelsSet},
			},
			BackoffLimit: utilptr.To[int32](0),
		},
	}
}

func waitForTransitionJobPodPhase(ctx context.Context, t *testing.T, clientSet clientset.Interface, job *batchv1.Job, phase v1.PodPhase) {
	podClient := clientSet.CoreV1().Pods(job.Namespace)
	if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
		t.Log(labels.FormatLabels(job.Labels))
		if podList, err := podClient.List(ctx, metav1.ListOptions{LabelSelector: labels.FormatLabels(job.Labels)}); err != nil {
			return false, err
		} else {
			if len(podList.Items) == 0 {
				t.Logf("Job controller has not created Pod for job %s yet", job.Name)
				return false, nil
			}
			for _, pod := range podList.Items {
				if pod.Status.Phase != phase {
					t.Logf("Pod %v not in %s phase yet, is %v instead", pod.Name, phase, pod.Status.Phase)
					return false, nil
				}
			}
			t.Logf("Job %v Pod is in %s phase now", job.Name, phase)
			return true, nil
		}
	}); err != nil {
		t.Fatalf("Error waiting for pods in %s phase: %v", phase, err)
	}
}
