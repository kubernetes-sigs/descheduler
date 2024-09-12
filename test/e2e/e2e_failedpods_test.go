package e2e

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	componentbaseconfig "k8s.io/component-base/config"
	utilptr "k8s.io/utils/ptr"

	"sigs.k8s.io/descheduler/pkg/descheduler/client"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	eutils "sigs.k8s.io/descheduler/pkg/descheduler/evictions/utils"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removefailedpods"
	frameworktesting "sigs.k8s.io/descheduler/pkg/framework/testing"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
)

var oneHourPodLifetimeSeconds uint = 3600

func TestFailedPods(t *testing.T) {
	ctx := context.Background()

	clientSet, err := client.CreateClient(componentbaseconfig.ClientConnectionConfiguration{Kubeconfig: os.Getenv("KUBECONFIG")}, "")
	if err != nil {
		t.Errorf("Error during client creation with %v", err)
	}

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
		expectedEvictedCount uint
		args                 *removefailedpods.RemoveFailedPodsArgs
	}{
		"test-failed-pods-default-args": {
			expectedEvictedCount: 1,
			args:                 &removefailedpods.RemoveFailedPodsArgs{},
		},
		"test-failed-pods-reason-unmatched": {
			expectedEvictedCount: 0,
			args: &removefailedpods.RemoveFailedPodsArgs{
				Reasons: []string{"ReasonDoesNotMatch"},
			},
		},
		"test-failed-pods-min-age-unmet": {
			expectedEvictedCount: 0,
			args: &removefailedpods.RemoveFailedPodsArgs{
				MinPodLifetimeSeconds: &oneHourPodLifetimeSeconds,
			},
		},
		"test-failed-pods-exclude-job-kind": {
			expectedEvictedCount: 0,
			args: &removefailedpods.RemoveFailedPodsArgs{
				ExcludeOwnerKinds: []string{"Job"},
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

			evictionPolicyGroupVersion, err := eutils.SupportEviction(clientSet)
			if err != nil || len(evictionPolicyGroupVersion) == 0 {
				t.Fatalf("Error detecting eviction policy group: %v", err)
			}

			handle, podEvictor, err := frameworktesting.InitFrameworkHandle(
				ctx,
				clientSet,
				evictions.NewOptions().
					WithPolicyGroupVersion(evictionPolicyGroupVersion),
				defaultevictor.DefaultEvictorArgs{
					EvictLocalStoragePods: true,
				},
				nil,
			)
			if err != nil {
				t.Fatalf("Unable to initialize a framework handle: %v", err)
			}

			t.Logf("Running RemoveFailedPods strategy for %s", name)

			plugin, err := removefailedpods.New(&removefailedpods.RemoveFailedPodsArgs{
				Reasons:                 tc.args.Reasons,
				MinPodLifetimeSeconds:   tc.args.MinPodLifetimeSeconds,
				IncludingInitContainers: tc.args.IncludingInitContainers,
				ExcludeOwnerKinds:       tc.args.ExcludeOwnerKinds,
				LabelSelector:           tc.args.LabelSelector,
				Namespaces:              tc.args.Namespaces,
			},
				handle,
			)
			if err != nil {
				t.Fatalf("Unable to initialize the plugin: %v", err)
			}

			plugin.(frameworktypes.DeschedulePlugin).Deschedule(ctx, nodes)
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

func waitForJobPodPhase(ctx context.Context, t *testing.T, clientSet clientset.Interface, job *batchv1.Job, phase v1.PodPhase) {
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
