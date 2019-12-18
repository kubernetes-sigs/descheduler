/*
Copyright 2015 The Kubernetes Authors.

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

package storage

import (
	"fmt"
	"strings"
	"time"

	"github.com/onsi/ginkgo"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
	e2epv "k8s.io/kubernetes/test/e2e/framework/pv"
	e2esset "k8s.io/kubernetes/test/e2e/framework/statefulset"
	"k8s.io/kubernetes/test/e2e/framework/volume"
	"k8s.io/kubernetes/test/e2e/storage/utils"
	imageutils "k8s.io/kubernetes/test/utils/image"
)

// Validate PV/PVC, create and verify writer pod, delete the PVC, and validate the PV's
// phase. Note: the PV is deleted in the AfterEach, not here.
func completeTest(f *framework.Framework, c clientset.Interface, ns string, pv *v1.PersistentVolume, pvc *v1.PersistentVolumeClaim) {
	// 1. verify that the PV and PVC have bound correctly
	ginkgo.By("Validating the PV-PVC binding")
	framework.ExpectNoError(e2epv.WaitOnPVandPVC(c, ns, pv, pvc))

	// 2. create the nfs writer pod, test if the write was successful,
	//    then delete the pod and verify that it was deleted
	ginkgo.By("Checking pod has write access to PersistentVolume")
	framework.ExpectNoError(e2epod.CreateWaitAndDeletePod(c, ns, pvc, "touch /mnt/volume1/SUCCESS && (id -G | grep -E '\\b777\\b')"))

	// 3. delete the PVC, wait for PV to become "Released"
	ginkgo.By("Deleting the PVC to invoke the reclaim policy.")
	framework.ExpectNoError(e2epv.DeletePVCandValidatePV(c, ns, pvc, pv, v1.VolumeReleased))
}

// Validate pairs of PVs and PVCs, create and verify writer pod, delete PVC and validate
// PV. Ensure each step succeeds.
// Note: the PV is deleted in the AfterEach, not here.
// Note: this func is serialized, we wait for each pod to be deleted before creating the
//   next pod. Adding concurrency is a TODO item.
func completeMultiTest(f *framework.Framework, c clientset.Interface, ns string, pvols e2epv.PVMap, claims e2epv.PVCMap, expectPhase v1.PersistentVolumePhase) error {
	var err error

	// 1. verify each PV permits write access to a client pod
	ginkgo.By("Checking pod has write access to PersistentVolumes")
	for pvcKey := range claims {
		pvc, err := c.CoreV1().PersistentVolumeClaims(pvcKey.Namespace).Get(pvcKey.Name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("error getting pvc %q: %v", pvcKey.Name, err)
		}
		if len(pvc.Spec.VolumeName) == 0 {
			continue // claim is not bound
		}
		// sanity test to ensure our maps are in sync
		_, found := pvols[pvc.Spec.VolumeName]
		if !found {
			return fmt.Errorf("internal: pvols map is missing volume %q", pvc.Spec.VolumeName)
		}
		// TODO: currently a serialized test of each PV
		if err = e2epod.CreateWaitAndDeletePod(c, pvcKey.Namespace, pvc, "touch /mnt/volume1/SUCCESS && (id -G | grep -E '\\b777\\b')"); err != nil {
			return err
		}
	}

	// 2. delete each PVC, wait for its bound PV to reach `expectedPhase`
	ginkgo.By("Deleting PVCs to invoke reclaim policy")
	if err = e2epv.DeletePVCandValidatePVGroup(c, ns, pvols, claims, expectPhase); err != nil {
		return err
	}
	return nil
}

var _ = utils.SIGDescribe("PersistentVolumes", func() {

	// global vars for the ginkgo.Context()s and ginkgo.It()'s below
	f := framework.NewDefaultFramework("pv")
	var (
		c         clientset.Interface
		ns        string
		pvConfig  e2epv.PersistentVolumeConfig
		pvcConfig e2epv.PersistentVolumeClaimConfig
		volLabel  labels.Set
		selector  *metav1.LabelSelector
		pv        *v1.PersistentVolume
		pvc       *v1.PersistentVolumeClaim
		err       error
	)

	ginkgo.BeforeEach(func() {
		c = f.ClientSet
		ns = f.Namespace.Name
		// Enforce binding only within test space via selector labels
		volLabel = labels.Set{e2epv.VolumeSelectorKey: ns}
		selector = metav1.SetAsLabelSelector(volLabel)
	})

	// Testing configurations of a single a PV/PVC pair, multiple evenly paired PVs/PVCs,
	// and multiple unevenly paired PV/PVCs
	ginkgo.Describe("NFS", func() {

		var (
			nfsServerPod *v1.Pod
			serverIP     string
		)

		ginkgo.BeforeEach(func() {
			_, nfsServerPod, serverIP = volume.NewNFSServer(c, ns, []string{"-G", "777", "/exports"})
			pvConfig = e2epv.PersistentVolumeConfig{
				NamePrefix: "nfs-",
				Labels:     volLabel,
				PVSource: v1.PersistentVolumeSource{
					NFS: &v1.NFSVolumeSource{
						Server:   serverIP,
						Path:     "/exports",
						ReadOnly: false,
					},
				},
			}
			emptyStorageClass := ""
			pvcConfig = e2epv.PersistentVolumeClaimConfig{
				Selector:         selector,
				StorageClassName: &emptyStorageClass,
			}
		})

		ginkgo.AfterEach(func() {
			framework.ExpectNoError(e2epod.DeletePodWithWait(c, nfsServerPod), "AfterEach: Failed to delete pod ", nfsServerPod.Name)
			pv, pvc = nil, nil
			pvConfig, pvcConfig = e2epv.PersistentVolumeConfig{}, e2epv.PersistentVolumeClaimConfig{}
		})

		ginkgo.Context("with Single PV - PVC pairs", func() {
			// Note: this is the only code where the pv is deleted.
			ginkgo.AfterEach(func() {
				framework.Logf("AfterEach: Cleaning up test resources.")
				if errs := e2epv.PVPVCCleanup(c, ns, pv, pvc); len(errs) > 0 {
					framework.Failf("AfterEach: Failed to delete PVC and/or PV. Errors: %v", utilerrors.NewAggregate(errs))
				}
			})

			// Individual tests follow:
			//
			// Create an nfs PV, then a claim that matches the PV, and a pod that
			// contains the claim. Verify that the PV and PVC bind correctly, and
			// that the pod can write to the nfs volume.
			ginkgo.It("should create a non-pre-bound PV and PVC: test write access ", func() {
				pv, pvc, err = e2epv.CreatePVPVC(c, pvConfig, pvcConfig, ns, false)
				framework.ExpectNoError(err)
				completeTest(f, c, ns, pv, pvc)
			})

			// Create a claim first, then a nfs PV that matches the claim, and a
			// pod that contains the claim. Verify that the PV and PVC bind
			// correctly, and that the pod can write to the nfs volume.
			ginkgo.It("create a PVC and non-pre-bound PV: test write access", func() {
				pv, pvc, err = e2epv.CreatePVCPV(c, pvConfig, pvcConfig, ns, false)
				framework.ExpectNoError(err)
				completeTest(f, c, ns, pv, pvc)
			})

			// Create a claim first, then a pre-bound nfs PV that matches the claim,
			// and a pod that contains the claim. Verify that the PV and PVC bind
			// correctly, and that the pod can write to the nfs volume.
			ginkgo.It("create a PVC and a pre-bound PV: test write access", func() {
				pv, pvc, err = e2epv.CreatePVCPV(c, pvConfig, pvcConfig, ns, true)
				framework.ExpectNoError(err)
				completeTest(f, c, ns, pv, pvc)
			})

			// Create a nfs PV first, then a pre-bound PVC that matches the PV,
			// and a pod that contains the claim. Verify that the PV and PVC bind
			// correctly, and that the pod can write to the nfs volume.
			ginkgo.It("create a PV and a pre-bound PVC: test write access", func() {
				pv, pvc, err = e2epv.CreatePVPVC(c, pvConfig, pvcConfig, ns, true)
				framework.ExpectNoError(err)
				completeTest(f, c, ns, pv, pvc)
			})
		})

		// Create multiple pvs and pvcs, all in the same namespace. The PVs-PVCs are
		// verified to bind, though it's not known in advanced which PV will bind to
		// which claim. For each pv-pvc pair create a pod that writes to the nfs mount.
		// Note: when the number of PVs exceeds the number of PVCs the max binding wait
		//   time will occur for each PV in excess. This is expected but the delta
		//   should be kept small so that the tests aren't unnecessarily slow.
		// Note: future tests may wish to incorporate the following:
		//   a) pre-binding, b) create pvcs before pvs, c) create pvcs and pods
		//   in different namespaces.
		ginkgo.Context("with multiple PVs and PVCs all in same ns", func() {

			// scope the pv and pvc maps to be available in the AfterEach
			// note: these maps are created fresh in CreatePVsPVCs()
			var pvols e2epv.PVMap
			var claims e2epv.PVCMap

			ginkgo.AfterEach(func() {
				framework.Logf("AfterEach: deleting %v PVCs and %v PVs...", len(claims), len(pvols))
				errs := e2epv.PVPVCMapCleanup(c, ns, pvols, claims)
				if len(errs) > 0 {
					errmsg := []string{}
					for _, e := range errs {
						errmsg = append(errmsg, e.Error())
					}
					framework.Failf("AfterEach: Failed to delete 1 or more PVs/PVCs. Errors: %v", strings.Join(errmsg, "; "))
				}
			})

			// Create 2 PVs and 4 PVCs.
			// Note: PVs are created before claims and no pre-binding
			ginkgo.It("should create 2 PVs and 4 PVCs: test write access", func() {
				numPVs, numPVCs := 2, 4
				pvols, claims, err = e2epv.CreatePVsPVCs(numPVs, numPVCs, c, ns, pvConfig, pvcConfig)
				framework.ExpectNoError(err)
				framework.ExpectNoError(e2epv.WaitAndVerifyBinds(c, ns, pvols, claims, true))
				framework.ExpectNoError(completeMultiTest(f, c, ns, pvols, claims, v1.VolumeReleased))
			})

			// Create 3 PVs and 3 PVCs.
			// Note: PVs are created before claims and no pre-binding
			ginkgo.It("should create 3 PVs and 3 PVCs: test write access", func() {
				numPVs, numPVCs := 3, 3
				pvols, claims, err = e2epv.CreatePVsPVCs(numPVs, numPVCs, c, ns, pvConfig, pvcConfig)
				framework.ExpectNoError(err)
				framework.ExpectNoError(e2epv.WaitAndVerifyBinds(c, ns, pvols, claims, true))
				framework.ExpectNoError(completeMultiTest(f, c, ns, pvols, claims, v1.VolumeReleased))
			})

			// Create 4 PVs and 2 PVCs.
			// Note: PVs are created before claims and no pre-binding.
			ginkgo.It("should create 4 PVs and 2 PVCs: test write access [Slow]", func() {
				numPVs, numPVCs := 4, 2
				pvols, claims, err = e2epv.CreatePVsPVCs(numPVs, numPVCs, c, ns, pvConfig, pvcConfig)
				framework.ExpectNoError(err)
				framework.ExpectNoError(e2epv.WaitAndVerifyBinds(c, ns, pvols, claims, true))
				framework.ExpectNoError(completeMultiTest(f, c, ns, pvols, claims, v1.VolumeReleased))
			})
		})

		// This Context isolates and tests the "Recycle" reclaim behavior.  On deprecation of the
		// Recycler, this entire context can be removed without affecting the test suite or leaving behind
		// dead code.
		ginkgo.Context("when invoking the Recycle reclaim policy", func() {
			ginkgo.BeforeEach(func() {
				pvConfig.ReclaimPolicy = v1.PersistentVolumeReclaimRecycle
				pv, pvc, err = e2epv.CreatePVPVC(c, pvConfig, pvcConfig, ns, false)
				framework.ExpectNoError(err, "BeforeEach: Failed to create PV/PVC")
				framework.ExpectNoError(e2epv.WaitOnPVandPVC(c, ns, pv, pvc), "BeforeEach: WaitOnPVandPVC failed")
			})

			ginkgo.AfterEach(func() {
				framework.Logf("AfterEach: Cleaning up test resources.")
				if errs := e2epv.PVPVCCleanup(c, ns, pv, pvc); len(errs) > 0 {
					framework.Failf("AfterEach: Failed to delete PVC and/or PV. Errors: %v", utilerrors.NewAggregate(errs))
				}
			})

			// This ginkgo.It() tests a scenario where a PV is written to by a Pod, recycled, then the volume checked
			// for files. If files are found, the checking Pod fails, failing the test.  Otherwise, the pod
			// (and test) succeed.
			ginkgo.It("should test that a PV becomes Available and is clean after the PVC is deleted.", func() {
				ginkgo.By("Writing to the volume.")
				pod := e2epv.MakeWritePod(ns, pvc)
				pod, err = c.CoreV1().Pods(ns).Create(pod)
				framework.ExpectNoError(err)
				framework.ExpectNoError(e2epod.WaitForPodSuccessInNamespace(c, pod.Name, ns))

				ginkgo.By("Deleting the claim")
				framework.ExpectNoError(e2epod.DeletePodWithWait(c, pod))
				framework.ExpectNoError(e2epv.DeletePVCandValidatePV(c, ns, pvc, pv, v1.VolumeAvailable))

				ginkgo.By("Re-mounting the volume.")
				pvc = e2epv.MakePersistentVolumeClaim(pvcConfig, ns)
				pvc, err = e2epv.CreatePVC(c, ns, pvc)
				framework.ExpectNoError(err)
				framework.ExpectNoError(e2epv.WaitForPersistentVolumeClaimPhase(v1.ClaimBound, c, ns, pvc.Name, 2*time.Second, 60*time.Second), "Failed to reach 'Bound' for PVC ", pvc.Name)

				// If a file is detected in /mnt, fail the pod and do not restart it.
				ginkgo.By("Verifying the mount has been cleaned.")
				mount := pod.Spec.Containers[0].VolumeMounts[0].MountPath
				pod = e2epod.MakePod(ns, nil, []*v1.PersistentVolumeClaim{pvc}, true, fmt.Sprintf("[ $(ls -A %s | wc -l) -eq 0 ] && exit 0 || exit 1", mount))
				pod, err = c.CoreV1().Pods(ns).Create(pod)
				framework.ExpectNoError(err)
				framework.ExpectNoError(e2epod.WaitForPodSuccessInNamespace(c, pod.Name, ns))

				framework.ExpectNoError(e2epod.DeletePodWithWait(c, pod))
				framework.Logf("Pod exited without failure; the volume has been recycled.")
			})
		})
	})

	ginkgo.Describe("Default StorageClass", func() {
		ginkgo.Context("pods that use multiple volumes", func() {

			ginkgo.AfterEach(func() {
				e2esset.DeleteAllStatefulSets(c, ns)
			})

			ginkgo.It("should be reschedulable [Slow]", func() {
				// Only run on providers with default storageclass
				framework.SkipUnlessProviderIs("openstack", "gce", "gke", "vsphere", "azure")

				numVols := 4

				ginkgo.By("Creating a StatefulSet pod to initialize data")
				writeCmd := "true"
				for i := 0; i < numVols; i++ {
					writeCmd += fmt.Sprintf("&& touch %v", getVolumeFile(i))
				}
				writeCmd += "&& sleep 10000"

				probe := &v1.Probe{
					Handler: v1.Handler{
						Exec: &v1.ExecAction{
							// Check that the last file got created
							Command: []string{"test", "-f", getVolumeFile(numVols - 1)},
						},
					},
					InitialDelaySeconds: 1,
					PeriodSeconds:       1,
				}

				mounts := []v1.VolumeMount{}
				claims := []v1.PersistentVolumeClaim{}

				for i := 0; i < numVols; i++ {
					pvc := e2epv.MakePersistentVolumeClaim(e2epv.PersistentVolumeClaimConfig{}, ns)
					pvc.Name = getVolName(i)
					mounts = append(mounts, v1.VolumeMount{Name: pvc.Name, MountPath: getMountPath(i)})
					claims = append(claims, *pvc)
				}

				spec := makeStatefulSetWithPVCs(ns, writeCmd, mounts, claims, probe)
				ss, err := c.AppsV1().StatefulSets(ns).Create(spec)
				framework.ExpectNoError(err)
				e2esset.WaitForRunningAndReady(c, 1, ss)

				ginkgo.By("Deleting the StatefulSet but not the volumes")
				// Scale down to 0 first so that the Delete is quick
				ss, err = e2esset.Scale(c, ss, 0)
				framework.ExpectNoError(err)
				e2esset.WaitForStatusReplicas(c, ss, 0)
				err = c.AppsV1().StatefulSets(ns).Delete(ss.Name, &metav1.DeleteOptions{})
				framework.ExpectNoError(err)

				ginkgo.By("Creating a new Statefulset and validating the data")
				validateCmd := "true"
				for i := 0; i < numVols; i++ {
					validateCmd += fmt.Sprintf("&& test -f %v", getVolumeFile(i))
				}
				validateCmd += "&& sleep 10000"

				spec = makeStatefulSetWithPVCs(ns, validateCmd, mounts, claims, probe)
				ss, err = c.AppsV1().StatefulSets(ns).Create(spec)
				framework.ExpectNoError(err)
				e2esset.WaitForRunningAndReady(c, 1, ss)
			})
		})
	})
})

func getVolName(i int) string {
	return fmt.Sprintf("vol%v", i)
}

func getMountPath(i int) string {
	return fmt.Sprintf("/mnt/%v", getVolName(i))
}

func getVolumeFile(i int) string {
	return fmt.Sprintf("%v/data%v", getMountPath(i), i)
}

func makeStatefulSetWithPVCs(ns, cmd string, mounts []v1.VolumeMount, claims []v1.PersistentVolumeClaim, readyProbe *v1.Probe) *appsv1.StatefulSet {
	ssReplicas := int32(1)

	labels := map[string]string{"app": "many-volumes-test"}
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "many-volumes-test",
			Namespace: ns,
		},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "many-volumes-test"},
			},
			Replicas: &ssReplicas,
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:           "nginx",
							Image:          imageutils.GetE2EImage(imageutils.Nginx),
							Command:        []string{"/bin/sh"},
							Args:           []string{"-c", cmd},
							VolumeMounts:   mounts,
							ReadinessProbe: readyProbe,
						},
					},
				},
			},
			VolumeClaimTemplates: claims,
		},
	}
}
