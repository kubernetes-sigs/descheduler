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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	componentbaseconfig "k8s.io/component-base/config"
	utilptr "k8s.io/utils/ptr"

	"sigs.k8s.io/descheduler/pkg/api"
	apiv1alpha2 "sigs.k8s.io/descheduler/pkg/api/v1alpha2"
	"sigs.k8s.io/descheduler/pkg/descheduler/client"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removefailedpods"
)

var (
	oneHourPodLifetimeSeconds   uint = 3600
	oneSecondPodLifetimeSeconds uint = 1
)

func removeFailedPodsPolicy(removeFailedPodsArgs *removefailedpods.RemoveFailedPodsArgs, evictorArgs *defaultevictor.DefaultEvictorArgs) *apiv1alpha2.DeschedulerPolicy {
	return &apiv1alpha2.DeschedulerPolicy{
		Profiles: []apiv1alpha2.DeschedulerProfile{
			{
				Name: removefailedpods.PluginName + "Profile",
				PluginConfigs: []apiv1alpha2.PluginConfig{
					{
						Name: removefailedpods.PluginName,
						Args: runtime.RawExtension{
							Object: removeFailedPodsArgs,
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
							removefailedpods.PluginName,
						},
					},
				},
			},
		},
	}
}

func TestFailedPods(t *testing.T) {
	ctx := context.Background()

	clientSet, err := client.CreateClient(componentbaseconfig.ClientConnectionConfiguration{Kubeconfig: os.Getenv("KUBECONFIG")}, "")
	if err != nil {
		t.Errorf("Error during kubernetes client creation with %v", err)
	}

	t.Log("Creating testing namespace")
	testNamespace := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "e2e-" + strings.ToLower(t.Name())}}
	if _, err := clientSet.CoreV1().Namespaces().Create(ctx, testNamespace, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Unable to create ns %v", testNamespace.Name)
	}
	defer clientSet.CoreV1().Namespaces().Delete(ctx, testNamespace.Name, metav1.DeleteOptions{})

	tests := []struct {
		name                    string
		expectedEvictedPodCount int
		removeFailedPodsArgs    *removefailedpods.RemoveFailedPodsArgs
	}{
		{
			name:                    "test-failed-pods-default-args",
			expectedEvictedPodCount: 1,
			removeFailedPodsArgs: &removefailedpods.RemoveFailedPodsArgs{
				MinPodLifetimeSeconds: &oneSecondPodLifetimeSeconds,
			},
		},
		{
			name:                    "test-failed-pods-reason-unmatched",
			expectedEvictedPodCount: 0,
			removeFailedPodsArgs: &removefailedpods.RemoveFailedPodsArgs{
				Reasons:               []string{"ReasonDoesNotMatch"},
				MinPodLifetimeSeconds: &oneSecondPodLifetimeSeconds,
			},
		},
		{
			name:                    "test-failed-pods-min-age-unmet",
			expectedEvictedPodCount: 0,
			removeFailedPodsArgs: &removefailedpods.RemoveFailedPodsArgs{
				MinPodLifetimeSeconds: &oneHourPodLifetimeSeconds,
			},
		},
		{
			name:                    "test-failed-pods-exclude-job-kind",
			expectedEvictedPodCount: 0,
			removeFailedPodsArgs: &removefailedpods.RemoveFailedPodsArgs{
				ExcludeOwnerKinds:     []string{"Job"},
				MinPodLifetimeSeconds: &oneSecondPodLifetimeSeconds,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			job := initFailedJob(tc.name, testNamespace.Namespace)
			t.Logf("Creating job %s in %s namespace", job.Name, job.Namespace)
			jobClient := clientSet.BatchV1().Jobs(testNamespace.Name)
			if _, err := jobClient.Create(ctx, job, metav1.CreateOptions{}); err != nil {
				t.Fatalf("Error creating Job %s: %v", tc.name, err)
			}
			deletePropagationPolicy := metav1.DeletePropagationForeground
			defer func() {
				jobClient.Delete(ctx, job.Name, metav1.DeleteOptions{PropagationPolicy: &deletePropagationPolicy})
				waitForPodsToDisappear(ctx, t, clientSet, job.Labels, job.Namespace)
			}()
			waitForJobPodPhase(ctx, t, clientSet, job, v1.PodFailed)

			preRunNames := sets.NewString(getCurrentPodNames(t, ctx, clientSet, testNamespace.Name)...)

			// Deploy the descheduler with the configured policy
			evictorArgs := &defaultevictor.DefaultEvictorArgs{
				EvictLocalStoragePods:   true,
				EvictSystemCriticalPods: false,
				IgnorePvcPods:           false,
				EvictFailedBarePods:     false,
			}
			tc.removeFailedPodsArgs.Namespaces = &api.Namespaces{
				Include: []string{testNamespace.Name},
			}

			deschedulerPolicyConfigMapObj, err := deschedulerPolicyConfigMap(removeFailedPodsPolicy(tc.removeFailedPodsArgs, evictorArgs))
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

			deschedulerDeploymentObj := deschedulerDeployment(testNamespace.Name)
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
			deschedulerPodName = waitForPodsRunning(ctx, t, clientSet, deschedulerDeploymentObj.Labels, 1, deschedulerDeploymentObj.Namespace)

			// Run RemoveDuplicates strategy
			var meetsExpectations bool
			var actualEvictedPodCount int
			if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 60*time.Second, true, func(ctx context.Context) (bool, error) {
				currentRunNames := sets.NewString(getCurrentPodNames(t, ctx, clientSet, testNamespace.Name)...)
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
				t.Errorf("Error waiting for descheduler running: %v", err)
			}

			if !meetsExpectations {
				t.Errorf("Unexpected number of pods have been evicted, got %v, expected %v", actualEvictedPodCount, tc.expectedEvictedPodCount)
			} else {
				t.Logf("Total of %d Pods were evicted for %s", actualEvictedPodCount, tc.name)
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
