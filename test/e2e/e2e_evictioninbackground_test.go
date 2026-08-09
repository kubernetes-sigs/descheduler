package e2e

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	componentbaseconfig "k8s.io/component-base/config"
	"k8s.io/klog/v2"
	utilptr "k8s.io/utils/ptr"

	kvcorev1 "kubevirt.io/api/core/v1"
	kubevirtclient "kubevirt.io/client-go/kubevirt"

	"sigs.k8s.io/descheduler/pkg/api"
	apiv1alpha2 "sigs.k8s.io/descheduler/pkg/api/v1alpha2"
	"sigs.k8s.io/descheduler/pkg/descheduler/client"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/podlifetime"
)

const (
	vmiCount = 3
	// virtLauncherSelector selects KubeVirt virt-launcher pods only,
	// avoiding listing unrelated pods in the namespace.
	virtLauncherSelector = "kubevirt.io=virt-launcher"
)

func virtualMachineInstance(idx int, namespace string) *kvcorev1.VirtualMachineInstance {
	return &kvcorev1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("kubevirtvmi-%v", idx),
			Namespace: namespace,
			Annotations: map[string]string{
				"descheduler.alpha.kubernetes.io/request-evict-only": "",
			},
		},
		Spec: kvcorev1.VirtualMachineInstanceSpec{
			EvictionStrategy: utilptr.To[kvcorev1.EvictionStrategy](kvcorev1.EvictionStrategyLiveMigrate),
			Domain: kvcorev1.DomainSpec{
				Devices: kvcorev1.Devices{
					AutoattachPodInterface: utilptr.To[bool](false),
					Disks: []kvcorev1.Disk{
						{
							Name: "containerdisk",
							DiskDevice: kvcorev1.DiskDevice{
								Disk: &kvcorev1.DiskTarget{
									Bus: kvcorev1.DiskBusVirtio,
								},
							},
						},
					},
					Rng: &kvcorev1.Rng{},
				},
				Resources: kvcorev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("128M"),
					},
				},
			},
			TerminationGracePeriodSeconds: utilptr.To[int64](0),
			Volumes: []kvcorev1.Volume{
				{
					Name: "containerdisk",
					VolumeSource: kvcorev1.VolumeSource{
						ContainerDisk: &kvcorev1.ContainerDiskSource{
							Image: "quay.io/kubevirt/cirros-container-disk-demo:v1.8.2",
						},
					},
				},
			},
		},
	}
}

func formatContainerStatuses(pod *corev1.Pod) string {
	var parts []string
	formatList := func(prefix string, statuses []corev1.ContainerStatus) {
		for _, cs := range statuses {
			stateStr := "unknown"
			if cs.State.Waiting != nil {
				stateStr = fmt.Sprintf("waiting(reason=%q, message=%q)", cs.State.Waiting.Reason, cs.State.Waiting.Message)
			} else if cs.State.Terminated != nil {
				stateStr = fmt.Sprintf("terminated(exitCode=%d, reason=%q, message=%q)", cs.State.Terminated.ExitCode, cs.State.Terminated.Reason, cs.State.Terminated.Message)
			} else if cs.State.Running != nil {
				stateStr = "running"
			}
			parts = append(parts, fmt.Sprintf("%s%s: state=%s, ready=%v, restarts=%d", prefix, cs.Name, stateStr, cs.Ready, cs.RestartCount))
		}
	}
	formatList("init:", pod.Status.InitContainerStatuses)
	formatList("", pod.Status.ContainerStatuses)
	return strings.Join(parts, "; ")
}

// ensureVMIsLiveMigratable waits until every VMI reports the LiveMigratable
// condition with status True. If a VMI fails to become migratable within the
// per-attempt timeout, it is deleted and recreated. This works around an
// upstream KubeVirt race where virt-handler computes the containerdisk
// checksum before the disk socket is ready, fails, and never retries; the
// recreated VMI lands on a node that already has the containerdisk image
// cached, so the socket comes up before virt-handler's first attempt.
// See https://github.com/kubernetes-sigs/descheduler/pull/1874 for context.
func ensureVMIsLiveMigratable(t *testing.T, ctx context.Context, kvClient kubevirtclient.Interface, namespace string) {
	t.Helper()
	const (
		maxAttempts    = 3
		perAttemptWait = 120 * time.Second
		deleteWait     = 60 * time.Second
	)

	isLiveMigratable := func(vmi *kvcorev1.VirtualMachineInstance) bool {
		for _, c := range vmi.Status.Conditions {
			if c.Type == kvcorev1.VirtualMachineInstanceIsMigratable && c.Status == corev1.ConditionTrue {
				return true
			}
		}
		return false
	}

	for i := 1; i <= vmiCount; i++ {
		name := fmt.Sprintf("kubevirtvmi-%v", i)
		var lastVMI *kvcorev1.VirtualMachineInstance
		for attempt := 1; attempt <= maxAttempts; attempt++ {
			err := wait.PollUntilContextTimeout(ctx, 5*time.Second, perAttemptWait, true, func(ctx context.Context) (bool, error) {
				vmi, err := kvClient.KubevirtV1().VirtualMachineInstances(namespace).Get(ctx, name, metav1.GetOptions{})
				if err != nil {
					klog.Infof("Unable to get vmi %v: %v", name, err)
					return false, nil
				}
				lastVMI = vmi
				return isLiveMigratable(vmi), nil
			})
			if err == nil {
				klog.Infof("vmi %v is LiveMigratable (attempt %d/%d)", name, attempt, maxAttempts)
				break
			}
			if attempt == maxAttempts {
				if lastVMI != nil {
					klog.Infof("Final vmi %v status: phase=%v, conditions=%#v", name, lastVMI.Status.Phase, lastVMI.Status.Conditions)
				}
				t.Fatalf("vmi %v never became LiveMigratable after %d attempts", name, maxAttempts)
			}
			klog.Warningf("vmi %v not LiveMigratable after %v, recreating (attempt %d/%d) to work around virt-handler containerdisk-socket race", name, perAttemptWait, attempt, maxAttempts)
			if err := kvClient.KubevirtV1().VirtualMachineInstances(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
				t.Fatalf("Unable to delete vmi %v for retry: %v", name, err)
			}
			if err := wait.PollUntilContextTimeout(ctx, 2*time.Second, deleteWait, true, func(ctx context.Context) (bool, error) {
				_, err := kvClient.KubevirtV1().VirtualMachineInstances(namespace).Get(ctx, name, metav1.GetOptions{})
				return apierrors.IsNotFound(err), nil
			}); err != nil {
				t.Fatalf("Timed out waiting for vmi %v to be deleted: %v", name, err)
			}
			if _, err := kvClient.KubevirtV1().VirtualMachineInstances(namespace).Create(ctx, virtualMachineInstance(i, namespace), metav1.CreateOptions{}); err != nil {
				t.Fatalf("Unable to recreate vmi %v: %v", name, err)
			}
		}
	}
}

func waitForKubevirtReady(t *testing.T, ctx context.Context, kvClient kubevirtclient.Interface) {
	obj, err := kvClient.KubevirtV1().KubeVirts("kubevirt").Get(ctx, "kubevirt", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Unable to get kubevirt/kubevirt: %v", err)
	}
	available := false
	for _, condition := range obj.Status.Conditions {
		if condition.Type == kvcorev1.KubeVirtConditionAvailable {
			if condition.Status == corev1.ConditionTrue {
				available = true
			}
		}
	}
	if !available {
		t.Fatalf("Kubevirt is not available")
	}
	klog.Infof("Kubevirt is available")
}

func allVMIsHaveRunningPods(t *testing.T, ctx context.Context, kubeClient clientset.Interface, kvClient kubevirtclient.Interface, namespace string) (bool, error) {
	klog.Infof("Checking all vmi active pods are running")
	uidMap := make(map[types.UID]*corev1.Pod)
	podList, err := kubeClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: virtLauncherSelector,
	})
	if err != nil {
		if isClientRateLimiterError(err) {
			klog.Infof("Unable to list pods: %v", err)
			return false, nil
		}
		klog.Infof("Unable to list pods: %v", err)
		return false, err
	}

	for _, item := range podList.Items {
		pod := item
		klog.Infof("item: %#v\n", item.UID)
		uidMap[item.UID] = &pod
	}

	vmiList, err := kvClient.KubevirtV1().VirtualMachineInstances(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		klog.Infof("Unable to list VMIs: %v", err)
		return false, err
	}
	if len(vmiList.Items) != vmiCount {
		klog.Infof("Expected %v VMIs, got %v instead", vmiCount, len(vmiList.Items))
		return false, nil
	}

	for _, item := range vmiList.Items {
		atLeastOneVmiIsRunning := false
		for activePod := range item.Status.ActivePods {
			if _, exists := uidMap[activePod]; !exists {
				klog.Infof("Active pod %v not found", activePod)
				return false, nil
			}
			pod := uidMap[activePod]
			klog.Infof("Checking whether active pod %v (uid=%v) is running", pod.Name, activePod)
			if pod.Status.Phase == corev1.PodFailed {
				details := fmt.Sprintf("pod %s (phase=Failed, reason=%q, message=%q, containers=[%s])", pod.Name, pod.Status.Reason, pod.Status.Message, formatContainerStatuses(pod))
				klog.Infof("Active pod failed: %s", details)
				continue
			}
			if pod.Status.Phase == corev1.PodSucceeded {
				klog.Infof("Ignoring active pod %v, phase=%v", pod.Name, pod.Status.Phase)
				continue
			}
			if pod.Status.Phase != corev1.PodRunning {
				klog.Infof("activePod %v is not running: %v\n", pod.Name, pod.Status.Phase)
				return false, nil
			}
			atLeastOneVmiIsRunning = true
		}
		if !atLeastOneVmiIsRunning {
			klog.Infof("vmi %v does not have any activePod running\n", item.Name)
			return false, nil
		}
	}

	return true, nil
}

func podLifeTimePolicy(namespace string) *apiv1alpha2.DeschedulerPolicy {
	return &apiv1alpha2.DeschedulerPolicy{
		Profiles: []apiv1alpha2.DeschedulerProfile{
			{
				Name: "KubeVirtPodLifetimeProfile",
				PluginConfigs: []apiv1alpha2.PluginConfig{
					{
						Name: podlifetime.PluginName,
						Args: runtime.RawExtension{
							Object: &podlifetime.PodLifeTimeArgs{
								MaxPodLifeTimeSeconds: utilptr.To[uint](1), // set it to immediate eviction
								Namespaces: &api.Namespaces{
									Include: []string{namespace},
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

func kVirtRunningPodNames(t *testing.T, ctx context.Context, kubeClient clientset.Interface, namespace string) []string {
	names := []string{}
	if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 60*time.Second, true, func(ctx context.Context) (bool, error) {
		podList, err := kubeClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: virtLauncherSelector,
		})
		if err != nil {
			if isClientRateLimiterError(err) {
				t.Log(err)
				return false, nil
			}
			klog.Infof("Unable to list pods: %v", err)
			return false, err
		}

		for _, item := range podList.Items {
			if item.Status.Phase == corev1.PodRunning {
				names = append(names, item.Name)
			}
		}

		return true, nil
	}); err != nil {
		t.Fatalf("Unable to list running kvirt pod names: %v", err)
	}
	return names
}

func observeLiveMigration(t *testing.T, ctx context.Context, kubeClient clientset.Interface, namespace string, usedRunningPodNames map[string]struct{}) {
	prevTotal := uint(0)
	jumps := 0
	// keep running the descheduling cycle until the migration is triggered and completed few times or times out
	for i := 0; i < 240; i++ {
		// monitor how many pods get evicted
		names := kVirtRunningPodNames(t, ctx, kubeClient, namespace)
		klog.Infof("vmi pods: %#v\n", names)
		// The number of pods need to be kept between vmiCount and vmiCount+1.
		// At most two pods are expected to have virt-launcher-kubevirtvmi-X prefix name in common.
		prefixes := make(map[string]uint)
		for _, name := range names {
			// "virt-launcher-kubevirtvmi-"
			str := strings.Split(name, "-")[4]
			prefixes[str]++
			usedRunningPodNames[name] = struct{}{}
		}

		hasDouble := false
		total := uint(0)
		for idx, count := range prefixes {
			total += count
			if count > 2 {
				t.Fatalf("A vmi kubevirtvmi-%v has more than 2 running active pods (%v), not expected", idx, count)
			}
			if count == 2 {
				if !hasDouble {
					hasDouble = true
					continue
				}
				t.Fatalf("Another vmi with 2 running active pods, not expected")
			}
		}
		// The total sum can not be higher than vmiCount+1
		if total > vmiCount+1 {
			t.Fatalf("Total running pods (%v) are higher than expected vmiCount+1 (%v)", total, vmiCount+1)
		}

		if prevTotal != 0 && prevTotal != total {
			jumps++
		}
		// Expect at least 2 finished live migrations
		if jumps >= 4 {
			break
		}
		prevTotal = total
		time.Sleep(4 * time.Second)
	}

	if jumps < 4 {
		podList, err := kubeClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: virtLauncherSelector,
		})
		if err != nil {
			klog.Infof("Unable to list pods: %v", err)
		} else {
			for _, item := range podList.Items {
				klog.Infof("pod(%v): %#v", item.Name, item)
			}
		}

		t.Fatalf("Expected at least 2 finished live migrations, got less: %v", jumps/2.0)
	}
	klog.Infof("The live migration finished 2 times")

	// len(usedRunningPodNames) is expected to be vmiCount + jumps/2 + 1 (one more live migration could still be initiated)
	klog.Infof("len(usedRunningPodNames): %v, upper limit: %v\n", len(usedRunningPodNames), vmiCount+jumps/2+1)
	if len(usedRunningPodNames) > vmiCount+jumps/2+1 {
		t.Fatalf("Expected vmiCount + jumps/2 + 1 = %v running pods, got %v instead", vmiCount+jumps/2+1, len(usedRunningPodNames))
	}

	if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 60*time.Second, true, func(ctx context.Context) (bool, error) {
		names := kVirtRunningPodNames(t, ctx, kubeClient, namespace)
		klog.Infof("vmi pods: %#v\n", names)
		lNames := len(names)
		if lNames != vmiCount {
			klog.Infof("Waiting for the number of running vmi pods to be %v, got %v instead", vmiCount, lNames)
			return false, nil
		}
		klog.Infof("The number of running vmi pods is %v as expected", vmiCount)
		return true, nil
	}); err != nil {
		t.Fatalf("Error waiting for %v vmi active pods to be running: %v", vmiCount, err)
	}
}

func createAndWaitForDeschedulerRunning(t *testing.T, ctx context.Context, kubeClient clientset.Interface, deschedulerDeploymentObj *appsv1.Deployment) string {
	klog.Infof("Creating descheduler deployment %v", deschedulerDeploymentObj.Name)
	_, err := kubeClient.AppsV1().Deployments(deschedulerDeploymentObj.Namespace).Create(ctx, deschedulerDeploymentObj, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Error creating %q deployment: %v", deschedulerDeploymentObj.Name, err)
	}

	klog.Infof("Waiting for the descheduler pod running")
	deschedulerPods := waitForPodsRunning(ctx, t, kubeClient, deschedulerDeploymentObj.Labels, 1, deschedulerDeploymentObj.Namespace)
	if len(deschedulerPods) == 0 {
		t.Fatalf("Error waiting for %q deployment: no running pod found", deschedulerDeploymentObj.Name)
	}
	return deschedulerPods[0].Name
}

func updateDeschedulerPolicy(t *testing.T, ctx context.Context, kubeClient clientset.Interface, policy *apiv1alpha2.DeschedulerPolicy) {
	deschedulerPolicyConfigMapObj, err := deschedulerPolicyConfigMap(policy)
	if err != nil {
		t.Fatalf("Error creating %q CM with unlimited evictions: %v", deschedulerPolicyConfigMapObj.Name, err)
	}
	_, err = kubeClient.CoreV1().ConfigMaps(deschedulerPolicyConfigMapObj.Namespace).Update(ctx, deschedulerPolicyConfigMapObj, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Error updating %q CM: %v", deschedulerPolicyConfigMapObj.Name, err)
	}
}

func createKubevirtClient() (kubevirtclient.Interface, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.DefaultClientConfig = &clientcmd.DefaultClientConfig
	overrides := &clientcmd.ConfigOverrides{}
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)

	config, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, err
	}
	config.GroupVersion = &kvcorev1.StorageGroupVersion
	config.APIPath = "/apis"
	config.ContentType = runtime.ContentTypeJSON

	return kubevirtclient.NewForConfig(config)
}

func setupE2ELiveMigrationNamespace(t *testing.T, ctx context.Context, kubeClient clientset.Interface, kvClient kubevirtclient.Interface, vmiNamespace string) {
	t.Helper()
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: vmiNamespace}}
	if _, err := kubeClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("Unable to create namespace %v: %v", vmiNamespace, err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		for i := 1; i <= vmiCount; i++ {
			vmi := virtualMachineInstance(i, vmiNamespace)
			if err := kvClient.KubevirtV1().VirtualMachineInstances(vmiNamespace).Delete(cleanupCtx, vmi.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
				klog.Infof("Unable to delete vmi %v: %v", vmi.Name, err)
			}
		}
		wait.PollUntilContextTimeout(cleanupCtx, 5*time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
			podList, err := kubeClient.CoreV1().Pods(vmiNamespace).List(ctx, metav1.ListOptions{
				LabelSelector: virtLauncherSelector,
			})
			if err != nil {
				return false, err
			}
			lPods := len(podList.Items)
			if lPods > 0 {
				klog.Infof("Waiting until all virt-launcher pods under %v namespace are gone, %v remaining", vmiNamespace, lPods)
				return false, nil
			}
			return true, nil
		})
		if err := kubeClient.CoreV1().Namespaces().Delete(cleanupCtx, vmiNamespace, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			klog.Infof("Unable to delete namespace %v: %v", vmiNamespace, err)
		}
	})
}

func waitForVMIEvictionsWithNoLimits(t *testing.T, ctx context.Context, kubeClient clientset.Interface, vmiNamespace string, remainingPods map[string]struct{}) {
	t.Helper()
	klog.Infof("Waiting until all pods are evicted (no limit set)")
	if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 120*time.Second, true, func(ctx context.Context) (bool, error) {
		names := kVirtRunningPodNames(t, ctx, kubeClient, vmiNamespace)
		for _, name := range names {
			if _, exists := remainingPods[name]; exists {
				klog.Infof("Waiting for %v to disappear", name)
				return false, nil
			}
		}
		lNames := len(names)
		if lNames != vmiCount {
			klog.Infof("Waiting for the number of newly running vmi pods to be %v, got %v instead", vmiCount, lNames)
			return false, nil
		}
		klog.Infof("The number of newly running vmi pods is %v as expected", vmiCount)
		return true, nil
	}); err != nil {
		t.Fatalf("Error waiting for %v new vmi active pods to be running: %v", vmiCount, err)
	}
}

func TestLiveMigrationInBackground(t *testing.T) {
	initPluginRegistry()

	ctx := context.Background()

	kubeClient, err := client.CreateClient(componentbaseconfig.ClientConnectionConfiguration{Kubeconfig: os.Getenv("KUBECONFIG")}, "")
	if err != nil {
		t.Fatalf("Error during kubernetes client creation with %v", err)
	}

	kvClient, err := createKubevirtClient()
	if err != nil {
		t.Fatalf("Error during kvClient creation with %v", err)
	}

	waitForKubevirtReady(t, ctx, kvClient)

	vmiNamespace := "e2e-livemigration"
	setupE2ELiveMigrationNamespace(t, ctx, kubeClient, kvClient, vmiNamespace)

	for i := 1; i <= vmiCount; i++ {
		vmi := virtualMachineInstance(i, vmiNamespace)
		_, err = kvClient.KubevirtV1().VirtualMachineInstances(vmiNamespace).Create(context.Background(), vmi, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Unable to create KubeVirt vmi: %v\n", err)
		}
	}

	if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 300*time.Second, true, func(ctx context.Context) (bool, error) {
		return allVMIsHaveRunningPods(t, ctx, kubeClient, kvClient, vmiNamespace)
	}); err != nil {
		t.Fatalf("Error waiting for all vmi active pods to be running: %v", err)
	}

	ensureVMIsLiveMigratable(t, ctx, kvClient, vmiNamespace)

	if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 300*time.Second, true, func(ctx context.Context) (bool, error) {
		return allVMIsHaveRunningPods(t, ctx, kubeClient, kvClient, vmiNamespace)
	}); err != nil {
		t.Fatalf("Error waiting for all vmi active pods to be running after recreate: %v", err)
	}

	usedRunningPodNames := make(map[string]struct{})
	names := kVirtRunningPodNames(t, ctx, kubeClient, vmiNamespace)
	klog.Infof("vmi pods: %#v\n", names)
	if len(names) != vmiCount {
		t.Fatalf("Expected %v vmi pods, got %v instead", vmiCount, len(names))
	}
	for _, name := range names {
		usedRunningPodNames[name] = struct{}{}
	}

	policy := podLifeTimePolicy(vmiNamespace)
	policy.MaxNoOfPodsToEvictPerNamespace = utilptr.To[uint](1)
	deschedulerPolicyConfigMapObj, err := deschedulerPolicyConfigMap(policy)
	if err != nil {
		t.Fatalf("Error creating %q CM: %v", deschedulerPolicyConfigMapObj.Name, err)
	}
	klog.Infof("Creating %q policy CM with PodLifeTime configured...", deschedulerPolicyConfigMapObj.Name)
	_, err = kubeClient.CoreV1().ConfigMaps(deschedulerPolicyConfigMapObj.Namespace).Create(ctx, deschedulerPolicyConfigMapObj, metav1.CreateOptions{})
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			_ = kubeClient.CoreV1().ConfigMaps(deschedulerPolicyConfigMapObj.Namespace).Delete(ctx, deschedulerPolicyConfigMapObj.Name, metav1.DeleteOptions{})
			_, err = kubeClient.CoreV1().ConfigMaps(deschedulerPolicyConfigMapObj.Namespace).Create(ctx, deschedulerPolicyConfigMapObj, metav1.CreateOptions{})
		}
		if err != nil {
			t.Fatalf("Error creating %q CM: %v", deschedulerPolicyConfigMapObj.Name, err)
		}
	}
	t.Cleanup(func() {
		klog.Infof("Deleting %q CM...", deschedulerPolicyConfigMapObj.Name)
		if err := kubeClient.CoreV1().ConfigMaps(deschedulerPolicyConfigMapObj.Namespace).Delete(context.Background(), deschedulerPolicyConfigMapObj.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			klog.Infof("Unable to delete %q CM: %v", deschedulerPolicyConfigMapObj.Name, err)
		}
	})

	deschedulerDeploymentObj := deschedulerDeployment("kube-system")
	deschedulerDeploymentObj.Spec.Template.Spec.Containers[0].Args = []string{"--policy-config-file", "/policy-dir/policy.yaml", "--descheduling-interval", "10s", "--v", "4", "--feature-gates", "EvictionsInBackground=true"}

	deschedulerPodName := ""
	t.Cleanup(func() {
		if deschedulerPodName != "" {
			printPodLogs(context.Background(), t, kubeClient, deschedulerPodName)
		}

		klog.Infof("Deleting %q deployment...", deschedulerDeploymentObj.Name)
		if err := kubeClient.AppsV1().Deployments(deschedulerDeploymentObj.Namespace).Delete(context.Background(), deschedulerDeploymentObj.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			klog.Infof("Unable to delete %q deployment: %v", deschedulerDeploymentObj.Name, err)
		}
		waitForPodsToDisappear(context.Background(), t, kubeClient, deschedulerDeploymentObj.Labels, deschedulerDeploymentObj.Namespace)
	})

	deschedulerPodName = createAndWaitForDeschedulerRunning(t, ctx, kubeClient, deschedulerDeploymentObj)

	observeLiveMigration(t, ctx, kubeClient, vmiNamespace, usedRunningPodNames)

	printPodLogs(ctx, t, kubeClient, deschedulerPodName)

	klog.Infof("Deleting the current descheduler pod")
	err = kubeClient.AppsV1().Deployments(deschedulerDeploymentObj.Namespace).Delete(ctx, deschedulerDeploymentObj.Name, metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("Error deleting %q deployment: %v", deschedulerDeploymentObj.Name, err)
	}

	remainingPods := make(map[string]struct{})
	for _, name := range kVirtRunningPodNames(t, ctx, kubeClient, vmiNamespace) {
		remainingPods[name] = struct{}{}
	}

	klog.Infof("Configuring the descheduler policy %v for PodLifetime with no limits", deschedulerPolicyConfigMapObj.Name)
	policy.MaxNoOfPodsToEvictPerNamespace = nil
	updateDeschedulerPolicy(t, ctx, kubeClient, policy)

	deschedulerDeploymentObj = deschedulerDeployment("kube-system")
	deschedulerDeploymentObj.Spec.Template.Spec.Containers[0].Args = []string{"--policy-config-file", "/policy-dir/policy.yaml", "--descheduling-interval", "100m", "--v", "4", "--feature-gates", "EvictionsInBackground=true"}
	deschedulerPodName = createAndWaitForDeschedulerRunning(t, ctx, kubeClient, deschedulerDeploymentObj)

	waitForVMIEvictionsWithNoLimits(t, ctx, kubeClient, vmiNamespace, remainingPods)
}
