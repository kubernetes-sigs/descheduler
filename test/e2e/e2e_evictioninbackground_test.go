package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
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
	generatedclient "kubevirt.io/client-go/generated/kubevirt/clientset/versioned"

	"sigs.k8s.io/descheduler/pkg/api"
	apiv1alpha2 "sigs.k8s.io/descheduler/pkg/api/v1alpha2"
	"sigs.k8s.io/descheduler/pkg/descheduler/client"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/podlifetime"
)

const (
	vmiCount = 3
)

func virtualMachineInstance(idx int) *kvcorev1.VirtualMachineInstance {
	return &kvcorev1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("kubevirtvmi-%v", idx),
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
						{
							Name: "cloudinitdisk",
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
					Requests: v1.ResourceList{
						v1.ResourceMemory: resource.MustParse("1024M"),
					},
				},
			},
			TerminationGracePeriodSeconds: utilptr.To[int64](0),
			Volumes: []kvcorev1.Volume{
				{
					Name: "containerdisk",
					VolumeSource: kvcorev1.VolumeSource{
						ContainerDisk: &kvcorev1.ContainerDiskSource{
							Image: "quay.io/kubevirt/fedora-with-test-tooling-container-disk:20240710_1265d1090",
						},
					},
				},
				{
					Name: "cloudinitdisk",
					VolumeSource: kvcorev1.VolumeSource{
						CloudInitNoCloud: &kvcorev1.CloudInitNoCloudSource{
							UserData: `#cloud-config
password: fedora
chpasswd: { expire: False }
packages:
  - nginx
runcmd:
  - [ "systemctl", "enable", "--now", "nginx" ]`,
							NetworkData: `version: 2
ethernets:
  eth0:
    addresses: [ fd10:0:2::2/120 ]
    dhcp4: true
    gateway6: fd10:0:2::1`,
						},
					},
				},
			},
		},
	}
}

func waitForKubevirtReady(t *testing.T, ctx context.Context, kvClient generatedclient.Interface) {
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

func allVMIsHaveRunningPods(t *testing.T, ctx context.Context, kubeClient clientset.Interface, kvClient generatedclient.Interface) (bool, error) {
	klog.Infof("Checking all vmi active pods are running")
	uidMap := make(map[types.UID]*corev1.Pod)
	podList, err := kubeClient.CoreV1().Pods("default").List(ctx, metav1.ListOptions{})
	if err != nil {
		if strings.Contains(err.Error(), "client rate limiter") {
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

	vmiList, err := kvClient.KubevirtV1().VirtualMachineInstances("default").List(ctx, metav1.ListOptions{})
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
			klog.Infof("Checking whether active pod %v (uid=%v) is running", uidMap[activePod].Name, activePod)
			// ignore completed/failed pods
			if uidMap[activePod].Status.Phase == corev1.PodFailed || uidMap[activePod].Status.Phase == corev1.PodSucceeded {
				klog.Infof("Ignoring active pod %v, phase=%v", uidMap[activePod].Name, uidMap[activePod].Status.Phase)
				continue
			}
			if uidMap[activePod].Status.Phase != corev1.PodRunning {
				klog.Infof("activePod %v is not running: %v\n", uidMap[activePod].Name, uidMap[activePod].Status.Phase)
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

func podLifeTimePolicy() *apiv1alpha2.DeschedulerPolicy {
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
									Include: []string{"default"},
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

func kVirtRunningPodNames(t *testing.T, ctx context.Context, kubeClient clientset.Interface) []string {
	names := []string{}
	if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 60*time.Second, true, func(ctx context.Context) (bool, error) {
		podList, err := kubeClient.CoreV1().Pods("default").List(ctx, metav1.ListOptions{})
		if err != nil {
			if isClientRateLimiterError(err) {
				t.Log(err)
				return false, nil
			}
			klog.Infof("Unable to list pods: %v", err)
			return false, err
		}

		for _, item := range podList.Items {
			if !strings.HasPrefix(item.Name, "virt-launcher-kubevirtvmi-") {
				t.Fatalf("Only pod names with 'virt-launcher-kubevirtvmi-' prefix are expected, got %q instead", item.Name)
			}
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

func observeLiveMigration(t *testing.T, ctx context.Context, kubeClient clientset.Interface, usedRunningPodNames map[string]struct{}, kvClient generatedclient.Interface) {
	prevTotal := uint(0)
	jumps := 0
	// keep running the descheduling cycle until the migration is triggered and completed few times or times out
	for i := 0; i < 240; i++ {
		// monitor how many pods get evicted
		names := kVirtRunningPodNames(t, ctx, kubeClient)
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
		// Expect at least 3 finished live migrations (two should be enough as well, though ...)
		if jumps >= 6 {
			break
		}
		prevTotal = total
		time.Sleep(time.Second)
	}

	if jumps < 6 {
		podList, err := kubeClient.CoreV1().Pods("default").List(ctx, metav1.ListOptions{})
		if err != nil {
			klog.Infof("Unable to list pods: %v", err)
		} else {
			for _, item := range podList.Items {
				klog.Infof("pod(%v): %#v", item.Name, item)
			}
		}

		vmiList, err := kvClient.KubevirtV1().VirtualMachineInstances("default").List(ctx, metav1.ListOptions{})
		if err != nil {
			klog.Infof("Unable to list VMIs: %v", err)
		} else {
			for _, item := range vmiList.Items {
				klog.Info("---")
				klog.Infof("vmi %s:", item.Name)
				data, _ := json.MarshalIndent(item, "", "  ")
				klog.Info(string(data))
			}
		}

		t.Fatalf("Expected at least 3 finished live migrations, got less: %v", jumps/2.0)
	}
	klog.Infof("The live migration finished 3 times")

	// len(usedRunningPodNames) is expected to be vmiCount + jumps/2 + 1 (one more live migration could still be initiated)
	klog.Infof("len(usedRunningPodNames): %v, upper limit: %v\n", len(usedRunningPodNames), vmiCount+jumps/2+1)
	if len(usedRunningPodNames) > vmiCount+jumps/2+1 {
		t.Fatalf("Expected vmiCount + jumps/2 + 1 = %v running pods, got %v instead", vmiCount+jumps/2+1, len(usedRunningPodNames))
	}

	if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 60*time.Second, true, func(ctx context.Context) (bool, error) {
		names := kVirtRunningPodNames(t, ctx, kubeClient)
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

func createKubevirtClient() (generatedclient.Interface, error) {
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

	return generatedclient.NewForConfig(config)
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

	// Delete all VMIs
	defer func() {
		for i := 1; i <= vmiCount; i++ {
			vmi := virtualMachineInstance(i)
			err := kvClient.KubevirtV1().VirtualMachineInstances("default").Delete(context.Background(), vmi.Name, metav1.DeleteOptions{})
			if err != nil && !apierrors.IsNotFound(err) {
				klog.Infof("Unable to delete vmi %v: %v", vmi.Name, err)
			}
		}
		wait.PollUntilContextTimeout(ctx, 5*time.Second, 60*time.Second, true, func(ctx context.Context) (bool, error) {
			podList, err := kubeClient.CoreV1().Pods("default").List(ctx, metav1.ListOptions{})
			if err != nil {
				return false, err
			}
			lPods := len(podList.Items)
			if lPods > 0 {
				klog.Infof("Waiting until all pods under default namespace are gone, %v remaining", lPods)
				return false, nil
			}
			return true, nil
		})
	}()

	// Create N vmis and wait for the corresponding vm pods to be ready and running
	for i := 1; i <= vmiCount; i++ {
		vmi := virtualMachineInstance(i)
		_, err = kvClient.KubevirtV1().VirtualMachineInstances("default").Create(context.Background(), vmi, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Unable to create KubeVirt vmi: %v\n", err)
		}
	}

	// Wait until all VMIs have running pods
	if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 300*time.Second, true, func(ctx context.Context) (bool, error) {
		return allVMIsHaveRunningPods(t, ctx, kubeClient, kvClient)
	}); err != nil {
		t.Fatalf("Error waiting for all vmi active pods to be running: %v", err)
	}

	usedRunningPodNames := make(map[string]struct{})
	// vmiCount number of names is expected
	names := kVirtRunningPodNames(t, ctx, kubeClient)
	klog.Infof("vmi pods: %#v\n", names)
	if len(names) != vmiCount {
		t.Fatalf("Expected %v vmi pods, got %v instead", vmiCount, len(names))
	}
	for _, name := range names {
		usedRunningPodNames[name] = struct{}{}
	}

	policy := podLifeTimePolicy()
	// Allow only a single eviction simultaneously
	policy.MaxNoOfPodsToEvictPerNamespace = utilptr.To[uint](1)
	// Deploy the descheduler with the configured policy
	deschedulerPolicyConfigMapObj, err := deschedulerPolicyConfigMap(policy)
	if err != nil {
		t.Fatalf("Error creating %q CM: %v", deschedulerPolicyConfigMapObj.Name, err)
	}
	klog.Infof("Creating %q policy CM with RemovePodsHavingTooManyRestarts configured...", deschedulerPolicyConfigMapObj.Name)
	_, err = kubeClient.CoreV1().ConfigMaps(deschedulerPolicyConfigMapObj.Namespace).Create(ctx, deschedulerPolicyConfigMapObj, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Error creating %q CM: %v", deschedulerPolicyConfigMapObj.Name, err)
	}
	defer func() {
		klog.Infof("Deleting %q CM...", deschedulerPolicyConfigMapObj.Name)
		err = kubeClient.CoreV1().ConfigMaps(deschedulerPolicyConfigMapObj.Namespace).Delete(ctx, deschedulerPolicyConfigMapObj.Name, metav1.DeleteOptions{})
		if err != nil {
			t.Fatalf("Unable to delete %q CM: %v", deschedulerPolicyConfigMapObj.Name, err)
		}
	}()

	deschedulerDeploymentObj := deschedulerDeployment("kube-system")
	// Set the descheduling interval to 10s
	deschedulerDeploymentObj.Spec.Template.Spec.Containers[0].Args = []string{"--policy-config-file", "/policy-dir/policy.yaml", "--descheduling-interval", "10s", "--v", "4", "--feature-gates", "EvictionsInBackground=true"}

	deschedulerPodName := ""
	defer func() {
		if deschedulerPodName != "" {
			printPodLogs(ctx, t, kubeClient, deschedulerPodName)
		}

		klog.Infof("Deleting %q deployment...", deschedulerDeploymentObj.Name)
		err = kubeClient.AppsV1().Deployments(deschedulerDeploymentObj.Namespace).Delete(ctx, deschedulerDeploymentObj.Name, metav1.DeleteOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return
			}
			t.Fatalf("Unable to delete %q deployment: %v", deschedulerDeploymentObj.Name, err)
		}
		waitForPodsToDisappear(ctx, t, kubeClient, deschedulerDeploymentObj.Labels, deschedulerDeploymentObj.Namespace)
	}()

	deschedulerPodName = createAndWaitForDeschedulerRunning(t, ctx, kubeClient, deschedulerDeploymentObj)

	observeLiveMigration(t, ctx, kubeClient, usedRunningPodNames, kvClient)

	printPodLogs(ctx, t, kubeClient, deschedulerPodName)

	klog.Infof("Deleting the current descheduler pod")
	err = kubeClient.AppsV1().Deployments(deschedulerDeploymentObj.Namespace).Delete(ctx, deschedulerDeploymentObj.Name, metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("Error deleting %q deployment: %v", deschedulerDeploymentObj.Name, err)
	}

	remainingPods := make(map[string]struct{})
	for _, name := range kVirtRunningPodNames(t, ctx, kubeClient) {
		remainingPods[name] = struct{}{}
	}

	klog.Infof("Configuring the descheduler policy %v for PodLifetime with no limits", deschedulerPolicyConfigMapObj.Name)
	policy.MaxNoOfPodsToEvictPerNamespace = nil
	updateDeschedulerPolicy(t, ctx, kubeClient, policy)

	deschedulerDeploymentObj = deschedulerDeployment("kube-system")
	deschedulerDeploymentObj.Spec.Template.Spec.Containers[0].Args = []string{"--policy-config-file", "/policy-dir/policy.yaml", "--descheduling-interval", "100m", "--v", "4", "--feature-gates", "EvictionsInBackground=true"}
	deschedulerPodName = createAndWaitForDeschedulerRunning(t, ctx, kubeClient, deschedulerDeploymentObj)

	klog.Infof("Waiting until all pods are evicted (no limit set)")
	if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 120*time.Second, true, func(ctx context.Context) (bool, error) {
		names := kVirtRunningPodNames(t, ctx, kubeClient)
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
