package e2e

import (
	"context"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"strings"
	"testing"
	"time"

	deschedulerapi "sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/strategies"
)

var oneHourPodLifetimeSeconds uint = 3600

func TestFailedPods(t *testing.T) {
	ctx := context.Background()
	clientSet, _, stopCh := initializeClient(t)
	defer close(stopCh)
	nodeList, err := clientSet.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Errorf("Error listing node with %v", err)
	}
	nodes, _ := splitNodesAndWorkerNodes(nodeList.Items)
	t.Log("Creating testing namespace")
	testNamespace := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "e2e-" + strings.ToLower(t.Name())}}
	if _, err := clientSet.CoreV1().Namespaces().Create(ctx, testNamespace, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Unable to create ns %v", testNamespace.Name)
	}
	defer clientSet.CoreV1().Namespaces().Delete(ctx, testNamespace.Name, metav1.DeleteOptions{})
	testCases := map[string]struct {
		expectedEvictedCount int
		strategyParams       *deschedulerapi.StrategyParameters
	}{
		"test-failed-pods-nil-strategy": {
			expectedEvictedCount: 1,
			strategyParams:       nil,
		},
		"test-failed-pods-default-strategy": {
			expectedEvictedCount: 1,
			strategyParams:       &deschedulerapi.StrategyParameters{},
		},
		"test-failed-pods-default-failed-pods": {
			expectedEvictedCount: 1,
			strategyParams: &deschedulerapi.StrategyParameters{
				FailedPods: &deschedulerapi.FailedPods{},
			},
		},
		"test-failed-pods-reason-unmatched": {
			expectedEvictedCount: 0,
			strategyParams: &deschedulerapi.StrategyParameters{
				FailedPods: &deschedulerapi.FailedPods{Reasons: []string{"ReasonDoesNotMatch"}},
			},
		},
		"test-failed-pods-min-age-unmet": {
			expectedEvictedCount: 0,
			strategyParams: &deschedulerapi.StrategyParameters{
				FailedPods: &deschedulerapi.FailedPods{MinPodLifetimeSeconds: &oneHourPodLifetimeSeconds},
			},
		},
		"test-failed-pods-exclude-job-kind": {
			expectedEvictedCount: 0,
			strategyParams: &deschedulerapi.StrategyParameters{
				FailedPods: &deschedulerapi.FailedPods{ExcludeOwnerKinds: []string{"Job"}},
			},
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			job := initFailedJob(name, testNamespace.Namespace)
			t.Logf("Creating job %s in %s namespace", job.Name, job.Namespace)
			jobClient := clientSet.BatchV1().Jobs(testNamespace.Name)
			if _, err := jobClient.Create(ctx, job, metav1.CreateOptions{}); err != nil {
				t.Fatalf("Error creating Job %s: %v", name, err)
			}
			deletePropagationPolicy := metav1.DeletePropagationForeground
			defer jobClient.Delete(ctx, job.Name, metav1.DeleteOptions{PropagationPolicy: &deletePropagationPolicy})
			waitForJobPodPhase(ctx, t, clientSet, job, v1.PodFailed)

			podEvictor := initPodEvictorOrFail(t, clientSet, nodes)

			t.Logf("Running RemoveFailedPods strategy for %s", name)
			strategies.RemoveFailedPods(
				ctx,
				clientSet,
				deschedulerapi.DeschedulerStrategy{
					Enabled: true,
					Params:  tc.strategyParams,
				},
				nodes,
				podEvictor,
			)
			t.Logf("Finished RemoveFailedPods strategy for %s", name)

			if actualEvictedCount := podEvictor.TotalEvicted(); actualEvictedCount == tc.expectedEvictedCount {
				t.Logf("Total of %d Pods were evicted for %s", actualEvictedCount, name)
			} else {
				t.Errorf("Unexpected number of pods have been evicted, got %v, expected %v", actualEvictedCount, tc.expectedEvictedCount)
			}
		})
	}
}

func initFailedJob(name, namespace string) *batchv1.Job {
	podSpec := MakePodSpec("", nil)
	podSpec.Containers[0].Command = []string{"/bin/false"}
	podSpec.RestartPolicy = v1.RestartPolicyNever
	labelsSet := labels.Set{"test": name, "name": name}
	jobBackoffLimit := int32(0)
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
			BackoffLimit: &jobBackoffLimit,
		},
	}
}

func waitForJobPodPhase(ctx context.Context, t *testing.T, clientSet clientset.Interface, job *batchv1.Job, phase v1.PodPhase) {
	podClient := clientSet.CoreV1().Pods(job.Namespace)
	if err := wait.PollImmediate(5*time.Second, 30*time.Second, func() (bool, error) {
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
