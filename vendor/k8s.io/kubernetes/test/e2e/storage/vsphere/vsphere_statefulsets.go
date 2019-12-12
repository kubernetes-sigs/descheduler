/*
Copyright 2017 The Kubernetes Authors.

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

package vsphere

import (
	"fmt"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
	e2esset "k8s.io/kubernetes/test/e2e/framework/statefulset"
	"k8s.io/kubernetes/test/e2e/storage/utils"
)

/*
	Test performs following operations

	Steps
	1. Create a storage class with thin diskformat.
	2. Create nginx service.
	3. Create nginx statefulsets with 3 replicas.
	4. Wait until all Pods are ready and PVCs are bounded with PV.
	5. Verify volumes are accessible in all statefulsets pods with creating empty file.
	6. Scale down statefulsets to 2 replicas.
	7. Scale up statefulsets to 4 replicas.
	8. Scale down statefulsets to 0 replicas and delete all pods.
	9. Delete all PVCs from the test namespace.
	10. Delete the storage class.
*/

const (
	manifestPath     = "test/e2e/testing-manifests/statefulset/nginx"
	mountPath        = "/usr/share/nginx/html"
	storageclassname = "nginx-sc"
)

var _ = utils.SIGDescribe("vsphere statefulset", func() {
	f := framework.NewDefaultFramework("vsphere-statefulset")
	var (
		namespace string
		client    clientset.Interface
	)
	ginkgo.BeforeEach(func() {
		framework.SkipUnlessProviderIs("vsphere")
		namespace = f.Namespace.Name
		client = f.ClientSet
		Bootstrap(f)
	})
	ginkgo.AfterEach(func() {
		framework.Logf("Deleting all statefulset in namespace: %v", namespace)
		e2esset.DeleteAllStatefulSets(client, namespace)
	})

	ginkgo.It("vsphere statefulset testing", func() {
		ginkgo.By("Creating StorageClass for Statefulset")
		scParameters := make(map[string]string)
		scParameters["diskformat"] = "thin"
		scSpec := getVSphereStorageClassSpec(storageclassname, scParameters, nil, "")
		sc, err := client.StorageV1().StorageClasses().Create(scSpec)
		framework.ExpectNoError(err)
		defer client.StorageV1().StorageClasses().Delete(sc.Name, nil)

		ginkgo.By("Creating statefulset")

		statefulset := e2esset.CreateStatefulSet(client, manifestPath, namespace)
		replicas := *(statefulset.Spec.Replicas)
		// Waiting for pods status to be Ready
		e2esset.WaitForStatusReadyReplicas(client, statefulset, replicas)
		framework.ExpectNoError(e2esset.CheckMount(client, statefulset, mountPath))
		ssPodsBeforeScaleDown := e2esset.GetPodList(client, statefulset)
		gomega.Expect(ssPodsBeforeScaleDown.Items).NotTo(gomega.BeEmpty(), fmt.Sprintf("Unable to get list of Pods from the Statefulset: %v", statefulset.Name))
		gomega.Expect(len(ssPodsBeforeScaleDown.Items) == int(replicas)).To(gomega.BeTrue(), "Number of Pods in the statefulset should match with number of replicas")

		// Get the list of Volumes attached to Pods before scale down
		volumesBeforeScaleDown := make(map[string]string)
		for _, sspod := range ssPodsBeforeScaleDown.Items {
			_, err := client.CoreV1().Pods(namespace).Get(sspod.Name, metav1.GetOptions{})
			framework.ExpectNoError(err)
			for _, volumespec := range sspod.Spec.Volumes {
				if volumespec.PersistentVolumeClaim != nil {
					volumePath := getvSphereVolumePathFromClaim(client, statefulset.Namespace, volumespec.PersistentVolumeClaim.ClaimName)
					volumesBeforeScaleDown[volumePath] = volumespec.PersistentVolumeClaim.ClaimName
				}
			}
		}

		ginkgo.By(fmt.Sprintf("Scaling down statefulsets to number of Replica: %v", replicas-1))
		_, scaledownErr := e2esset.Scale(client, statefulset, replicas-1)
		framework.ExpectNoError(scaledownErr)
		e2esset.WaitForStatusReadyReplicas(client, statefulset, replicas-1)

		// After scale down, verify vsphere volumes are detached from deleted pods
		ginkgo.By("Verify Volumes are detached from Nodes after Statefulsets is scaled down")
		for _, sspod := range ssPodsBeforeScaleDown.Items {
			_, err := client.CoreV1().Pods(namespace).Get(sspod.Name, metav1.GetOptions{})
			if err != nil {
				gomega.Expect(apierrs.IsNotFound(err), gomega.BeTrue())
				for _, volumespec := range sspod.Spec.Volumes {
					if volumespec.PersistentVolumeClaim != nil {
						vSpherediskPath := getvSphereVolumePathFromClaim(client, statefulset.Namespace, volumespec.PersistentVolumeClaim.ClaimName)
						framework.Logf("Waiting for Volume: %q to detach from Node: %q", vSpherediskPath, sspod.Spec.NodeName)
						framework.ExpectNoError(waitForVSphereDiskToDetach(vSpherediskPath, sspod.Spec.NodeName))
					}
				}
			}
		}

		ginkgo.By(fmt.Sprintf("Scaling up statefulsets to number of Replica: %v", replicas))
		_, scaleupErr := e2esset.Scale(client, statefulset, replicas)
		framework.ExpectNoError(scaleupErr)
		e2esset.WaitForStatusReplicas(client, statefulset, replicas)
		e2esset.WaitForStatusReadyReplicas(client, statefulset, replicas)

		ssPodsAfterScaleUp := e2esset.GetPodList(client, statefulset)
		gomega.Expect(ssPodsAfterScaleUp.Items).NotTo(gomega.BeEmpty(), fmt.Sprintf("Unable to get list of Pods from the Statefulset: %v", statefulset.Name))
		gomega.Expect(len(ssPodsAfterScaleUp.Items) == int(replicas)).To(gomega.BeTrue(), "Number of Pods in the statefulset should match with number of replicas")

		// After scale up, verify all vsphere volumes are attached to node VMs.
		ginkgo.By("Verify all volumes are attached to Nodes after Statefulsets is scaled up")
		for _, sspod := range ssPodsAfterScaleUp.Items {
			err := e2epod.WaitForPodsReady(client, statefulset.Namespace, sspod.Name, 0)
			framework.ExpectNoError(err)
			pod, err := client.CoreV1().Pods(namespace).Get(sspod.Name, metav1.GetOptions{})
			framework.ExpectNoError(err)
			for _, volumespec := range pod.Spec.Volumes {
				if volumespec.PersistentVolumeClaim != nil {
					vSpherediskPath := getvSphereVolumePathFromClaim(client, statefulset.Namespace, volumespec.PersistentVolumeClaim.ClaimName)
					framework.Logf("Verify Volume: %q is attached to the Node: %q", vSpherediskPath, sspod.Spec.NodeName)
					// Verify scale up has re-attached the same volumes and not introduced new volume
					gomega.Expect(volumesBeforeScaleDown[vSpherediskPath] == "").To(gomega.BeFalse())
					isVolumeAttached, verifyDiskAttachedError := diskIsAttached(vSpherediskPath, sspod.Spec.NodeName)
					gomega.Expect(isVolumeAttached).To(gomega.BeTrue())
					framework.ExpectNoError(verifyDiskAttachedError)
				}
			}
		}
	})
})
