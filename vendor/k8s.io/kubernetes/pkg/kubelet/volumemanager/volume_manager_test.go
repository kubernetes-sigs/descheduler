/*
Copyright 2016 The Kubernetes Authors.

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

package volumemanager

import (
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"k8s.io/utils/mount"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/record"
	utiltesting "k8s.io/client-go/util/testing"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"k8s.io/kubernetes/pkg/features"
	"k8s.io/kubernetes/pkg/kubelet/config"
	"k8s.io/kubernetes/pkg/kubelet/configmap"
	containertest "k8s.io/kubernetes/pkg/kubelet/container/testing"
	kubepod "k8s.io/kubernetes/pkg/kubelet/pod"
	podtest "k8s.io/kubernetes/pkg/kubelet/pod/testing"
	"k8s.io/kubernetes/pkg/kubelet/secret"
	"k8s.io/kubernetes/pkg/kubelet/status"
	statustest "k8s.io/kubernetes/pkg/kubelet/status/testing"
	"k8s.io/kubernetes/pkg/volume"
	volumetest "k8s.io/kubernetes/pkg/volume/testing"
	"k8s.io/kubernetes/pkg/volume/util"
	"k8s.io/kubernetes/pkg/volume/util/hostutil"
	"k8s.io/kubernetes/pkg/volume/util/types"
)

const (
	testHostname = "test-hostname"
)

func TestGetMountedVolumesForPodAndGetVolumesInUse(t *testing.T) {
	tests := []struct {
		name                string
		pvMode, podMode     v1.PersistentVolumeMode
		disableBlockFeature bool
		expectMount         bool
		expectError         bool
	}{
		{
			name:        "filesystem volume",
			pvMode:      v1.PersistentVolumeFilesystem,
			podMode:     v1.PersistentVolumeFilesystem,
			expectMount: true,
			expectError: false,
		},
		{
			name:        "block volume",
			pvMode:      v1.PersistentVolumeBlock,
			podMode:     v1.PersistentVolumeBlock,
			expectMount: true,
			expectError: false,
		},
		{
			name:                "block volume with block feature off",
			pvMode:              v1.PersistentVolumeBlock,
			podMode:             v1.PersistentVolumeBlock,
			disableBlockFeature: true,
			expectMount:         false,
			expectError:         false,
		},
		{
			name:        "mismatched volume",
			pvMode:      v1.PersistentVolumeBlock,
			podMode:     v1.PersistentVolumeFilesystem,
			expectMount: false,
			expectError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.disableBlockFeature {
				defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.BlockVolume, false)()
			}

			tmpDir, err := utiltesting.MkTmpdir("volumeManagerTest")
			if err != nil {
				t.Fatalf("can't make a temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)
			cpm := podtest.NewMockCheckpointManager()
			podManager := kubepod.NewBasicPodManager(podtest.NewFakeMirrorClient(), secret.NewFakeManager(), configmap.NewFakeManager(), cpm)

			node, pod, pv, claim := createObjects(test.pvMode, test.podMode)
			kubeClient := fake.NewSimpleClientset(node, pod, pv, claim)

			manager := newTestVolumeManager(tmpDir, podManager, kubeClient)

			stopCh := runVolumeManager(manager)
			defer close(stopCh)

			podManager.SetPods([]*v1.Pod{pod})

			// Fake node status update
			go simulateVolumeInUseUpdate(
				v1.UniqueVolumeName(node.Status.VolumesAttached[0].Name),
				stopCh,
				manager)

			err = manager.WaitForAttachAndMount(pod)
			if err != nil && !test.expectError {
				t.Errorf("Expected success: %v", err)
			}
			if err == nil && test.expectError {
				t.Errorf("Expected error, got none")
			}

			expectedMounted := pod.Spec.Volumes[0].Name
			actualMounted := manager.GetMountedVolumesForPod(types.UniquePodName(pod.ObjectMeta.UID))
			if test.expectMount {
				if _, ok := actualMounted[expectedMounted]; !ok || (len(actualMounted) != 1) {
					t.Errorf("Expected %v to be mounted to pod but got %v", expectedMounted, actualMounted)
				}
			} else {
				if _, ok := actualMounted[expectedMounted]; ok || (len(actualMounted) != 0) {
					t.Errorf("Expected %v not to be mounted to pod but got %v", expectedMounted, actualMounted)
				}
			}

			expectedInUse := []v1.UniqueVolumeName{}
			if test.expectMount {
				expectedInUse = []v1.UniqueVolumeName{v1.UniqueVolumeName(node.Status.VolumesAttached[0].Name)}
			}
			actualInUse := manager.GetVolumesInUse()
			if !reflect.DeepEqual(expectedInUse, actualInUse) {
				t.Errorf("Expected %v to be in use but got %v", expectedInUse, actualInUse)
			}
		})
	}
}

func TestInitialPendingVolumesForPodAndGetVolumesInUse(t *testing.T) {
	tmpDir, err := utiltesting.MkTmpdir("volumeManagerTest")
	if err != nil {
		t.Fatalf("can't make a temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	cpm := podtest.NewMockCheckpointManager()
	podManager := kubepod.NewBasicPodManager(podtest.NewFakeMirrorClient(), secret.NewFakeManager(), configmap.NewFakeManager(), cpm)

	node, pod, pv, claim := createObjects(v1.PersistentVolumeFilesystem, v1.PersistentVolumeFilesystem)
	claim.Status = v1.PersistentVolumeClaimStatus{
		Phase: v1.ClaimPending,
	}

	kubeClient := fake.NewSimpleClientset(node, pod, pv, claim)

	manager := newTestVolumeManager(tmpDir, podManager, kubeClient)

	stopCh := runVolumeManager(manager)
	defer close(stopCh)

	podManager.SetPods([]*v1.Pod{pod})

	// Fake node status update
	go simulateVolumeInUseUpdate(
		v1.UniqueVolumeName(node.Status.VolumesAttached[0].Name),
		stopCh,
		manager)

	// delayed claim binding
	go delayClaimBecomesBound(kubeClient, claim.GetNamespace(), claim.ObjectMeta.Name)

	err = wait.Poll(100*time.Millisecond, 1*time.Second, func() (bool, error) {
		err = manager.WaitForAttachAndMount(pod)
		if err != nil {
			// Few "PVC not bound" errors are expected
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		t.Errorf("Expected a volume to be mounted, got: %s", err)
	}

}

func TestGetExtraSupplementalGroupsForPod(t *testing.T) {
	tmpDir, err := utiltesting.MkTmpdir("volumeManagerTest")
	if err != nil {
		t.Fatalf("can't make a temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	cpm := podtest.NewMockCheckpointManager()
	podManager := kubepod.NewBasicPodManager(podtest.NewFakeMirrorClient(), secret.NewFakeManager(), configmap.NewFakeManager(), cpm)

	node, pod, _, claim := createObjects(v1.PersistentVolumeFilesystem, v1.PersistentVolumeFilesystem)

	existingGid := pod.Spec.SecurityContext.SupplementalGroups[0]

	cases := []struct {
		gidAnnotation string
		expected      []int64
	}{
		{
			gidAnnotation: "777",
			expected:      []int64{777},
		},
		{
			gidAnnotation: strconv.FormatInt(int64(existingGid), 10),
			expected:      []int64{},
		},
		{
			gidAnnotation: "a",
			expected:      []int64{},
		},
		{
			gidAnnotation: "",
			expected:      []int64{},
		},
	}

	for _, tc := range cases {
		fs := v1.PersistentVolumeFilesystem
		pv := &v1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name: "pvA",
				Annotations: map[string]string{
					util.VolumeGidAnnotationKey: tc.gidAnnotation,
				},
			},
			Spec: v1.PersistentVolumeSpec{
				PersistentVolumeSource: v1.PersistentVolumeSource{
					GCEPersistentDisk: &v1.GCEPersistentDiskVolumeSource{
						PDName: "fake-device",
					},
				},
				ClaimRef: &v1.ObjectReference{
					Name:      claim.ObjectMeta.Name,
					Namespace: claim.ObjectMeta.Namespace,
				},
				VolumeMode: &fs,
			},
		}
		kubeClient := fake.NewSimpleClientset(node, pod, pv, claim)

		manager := newTestVolumeManager(tmpDir, podManager, kubeClient)

		stopCh := runVolumeManager(manager)
		defer close(stopCh)

		podManager.SetPods([]*v1.Pod{pod})

		// Fake node status update
		go simulateVolumeInUseUpdate(
			v1.UniqueVolumeName(node.Status.VolumesAttached[0].Name),
			stopCh,
			manager)

		err = manager.WaitForAttachAndMount(pod)
		if err != nil {
			t.Errorf("Expected success: %v", err)
			continue
		}

		actual := manager.GetExtraSupplementalGroupsForPod(pod)
		if !reflect.DeepEqual(tc.expected, actual) {
			t.Errorf("Expected supplemental groups %v, got %v", tc.expected, actual)
		}
	}
}

func newTestVolumeManager(tmpDir string, podManager kubepod.Manager, kubeClient clientset.Interface) VolumeManager {
	plug := &volumetest.FakeVolumePlugin{PluginName: "fake", Host: nil}
	fakeRecorder := &record.FakeRecorder{}
	plugMgr := &volume.VolumePluginMgr{}
	// TODO (#51147) inject mock prober
	plugMgr.InitPlugins([]volume.VolumePlugin{plug}, nil /* prober */, volumetest.NewFakeVolumeHost(tmpDir, kubeClient, nil))
	statusManager := status.NewManager(kubeClient, podManager, &statustest.FakePodDeletionSafetyProvider{})
	fakePathHandler := volumetest.NewBlockVolumePathHandler()
	vm := NewVolumeManager(
		true,
		testHostname,
		podManager,
		statusManager,
		kubeClient,
		plugMgr,
		&containertest.FakeRuntime{},
		mount.NewFakeMounter(nil),
		hostutil.NewFakeHostUtil(nil),
		"",
		fakeRecorder,
		false, /* experimentalCheckNodeCapabilitiesBeforeMount */
		false, /* keepTerminatedPodVolumes */
		fakePathHandler)

	return vm
}

// createObjects returns objects for making a fake clientset. The pv is
// already attached to the node and bound to the claim used by the pod.
func createObjects(pvMode, podMode v1.PersistentVolumeMode) (*v1.Node, *v1.Pod, *v1.PersistentVolume, *v1.PersistentVolumeClaim) {
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: testHostname},
		Status: v1.NodeStatus{
			VolumesAttached: []v1.AttachedVolume{
				{
					Name:       "fake/fake-device",
					DevicePath: "fake/path",
				},
			}},
	}
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "abc",
			Namespace: "nsA",
			UID:       "1234",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name: "container1",
				},
			},
			Volumes: []v1.Volume{
				{
					Name: "vol1",
					VolumeSource: v1.VolumeSource{
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
							ClaimName: "claimA",
						},
					},
				},
			},
			SecurityContext: &v1.PodSecurityContext{
				SupplementalGroups: []int64{555},
			},
		},
	}
	switch podMode {
	case v1.PersistentVolumeBlock:
		pod.Spec.Containers[0].VolumeDevices = []v1.VolumeDevice{
			{
				Name:       "vol1",
				DevicePath: "/dev/vol1",
			},
		}
	case v1.PersistentVolumeFilesystem:
		pod.Spec.Containers[0].VolumeMounts = []v1.VolumeMount{
			{
				Name:      "vol1",
				MountPath: "/mnt/vol1",
			},
		}
	default:
		// The volume is not mounted nor mapped
	}
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pvA",
		},
		Spec: v1.PersistentVolumeSpec{
			PersistentVolumeSource: v1.PersistentVolumeSource{
				GCEPersistentDisk: &v1.GCEPersistentDiskVolumeSource{
					PDName: "fake-device",
				},
			},
			ClaimRef: &v1.ObjectReference{
				Namespace: "nsA",
				Name:      "claimA",
			},
			VolumeMode: &pvMode,
		},
	}
	claim := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "claimA",
			Namespace: "nsA",
		},
		Spec: v1.PersistentVolumeClaimSpec{
			VolumeName: "pvA",
			VolumeMode: &pvMode,
		},
		Status: v1.PersistentVolumeClaimStatus{
			Phase: v1.ClaimBound,
		},
	}
	return node, pod, pv, claim
}

func simulateVolumeInUseUpdate(volumeName v1.UniqueVolumeName, stopCh <-chan struct{}, volumeManager VolumeManager) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			volumeManager.MarkVolumesAsReportedInUse(
				[]v1.UniqueVolumeName{volumeName})
		case <-stopCh:
			return
		}
	}
}

func delayClaimBecomesBound(
	kubeClient clientset.Interface,
	namespace, claimName string,
) {
	time.Sleep(500 * time.Millisecond)
	volumeClaim, _ :=
		kubeClient.CoreV1().PersistentVolumeClaims(namespace).Get(claimName, metav1.GetOptions{})
	volumeClaim.Status = v1.PersistentVolumeClaimStatus{
		Phase: v1.ClaimBound,
	}
	kubeClient.CoreV1().PersistentVolumeClaims(namespace).Update(volumeClaim)
}

func runVolumeManager(manager VolumeManager) chan struct{} {
	stopCh := make(chan struct{})
	//readyCh := make(chan bool, 1)
	//readyCh <- true
	sourcesReady := config.NewSourcesReady(func(_ sets.String) bool { return true })
	go manager.Run(sourcesReady, stopCh)
	return stopCh
}
