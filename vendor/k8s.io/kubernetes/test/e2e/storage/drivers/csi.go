/*
Copyright 2018 The Kubernetes Authors.

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

/*
 * This file defines various csi volume test drivers for TestSuites.
 *
 * There are two ways, how to prepare test drivers:
 * 1) With containerized server (NFS, Ceph, Gluster, iSCSI, ...)
 * It creates a server pod which defines one volume for the tests.
 * These tests work only when privileged containers are allowed, exporting
 * various filesystems (NFS, GlusterFS, ...) usually needs some mounting or
 * other privileged magic in the server pod.
 *
 * Note that the server containers are for testing purposes only and should not
 * be used in production.
 *
 * 2) With server or cloud provider outside of Kubernetes (Cinder, GCE, AWS, Azure, ...)
 * Appropriate server or cloud provider must exist somewhere outside
 * the tested Kubernetes cluster. CreateVolume will create a new volume to be
 * used in the TestSuites for inlineVolume or DynamicPV tests.
 */

package drivers

import (
	"fmt"
	"strconv"
	"time"

	"github.com/onsi/ginkgo"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	storagev1beta1 "k8s.io/api/storage/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
	"k8s.io/kubernetes/test/e2e/framework/volume"
	"k8s.io/kubernetes/test/e2e/storage/testpatterns"
	"k8s.io/kubernetes/test/e2e/storage/testsuites"
	"k8s.io/kubernetes/test/e2e/storage/utils"
)

const (
	// GCEPDCSIDriverName is the name of GCE Persistent Disk CSI driver
	GCEPDCSIDriverName = "pd.csi.storage.gke.io"
	// GCEPDCSIZoneTopologyKey is the key of GCE Persistent Disk CSI zone topology
	GCEPDCSIZoneTopologyKey = "topology.gke.io/zone"
)

// hostpathCSI
type hostpathCSIDriver struct {
	driverInfo       testsuites.DriverInfo
	manifests        []string
	volumeAttributes []map[string]string
}

func initHostPathCSIDriver(name string, capabilities map[testsuites.Capability]bool, volumeAttributes []map[string]string, manifests ...string) testsuites.TestDriver {
	return &hostpathCSIDriver{
		driverInfo: testsuites.DriverInfo{
			Name:        name,
			FeatureTag:  "",
			MaxFileSize: testpatterns.FileSizeMedium,
			SupportedFsType: sets.NewString(
				"", // Default fsType
			),
			SupportedSizeRange: volume.SizeRange{
				Min: "1Mi",
			},
			Capabilities: capabilities,
		},
		manifests:        manifests,
		volumeAttributes: volumeAttributes,
	}
}

var _ testsuites.TestDriver = &hostpathCSIDriver{}
var _ testsuites.DynamicPVTestDriver = &hostpathCSIDriver{}
var _ testsuites.SnapshottableTestDriver = &hostpathCSIDriver{}
var _ testsuites.EphemeralTestDriver = &hostpathCSIDriver{}

// InitHostPathCSIDriver returns hostpathCSIDriver that implements TestDriver interface
func InitHostPathCSIDriver() testsuites.TestDriver {
	capabilities := map[testsuites.Capability]bool{
		testsuites.CapPersistence:         true,
		testsuites.CapSnapshotDataSource:  true,
		testsuites.CapMultiPODs:           true,
		testsuites.CapBlock:               true,
		testsuites.CapPVCDataSource:       true,
		testsuites.CapControllerExpansion: true,
		testsuites.CapSingleNodeVolume:    true,
		testsuites.CapVolumeLimits:        true,
	}
	return initHostPathCSIDriver("csi-hostpath",
		capabilities,
		// Volume attributes don't matter, but we have to provide at least one map.
		[]map[string]string{
			{"foo": "bar"},
		},
		"test/e2e/testing-manifests/storage-csi/external-attacher/rbac.yaml",
		"test/e2e/testing-manifests/storage-csi/external-provisioner/rbac.yaml",
		"test/e2e/testing-manifests/storage-csi/external-snapshotter/rbac.yaml",
		"test/e2e/testing-manifests/storage-csi/external-resizer/rbac.yaml",
		"test/e2e/testing-manifests/storage-csi/hostpath/hostpath/csi-hostpath-attacher.yaml",
		"test/e2e/testing-manifests/storage-csi/hostpath/hostpath/csi-hostpath-driverinfo.yaml",
		"test/e2e/testing-manifests/storage-csi/hostpath/hostpath/csi-hostpath-plugin.yaml",
		"test/e2e/testing-manifests/storage-csi/hostpath/hostpath/csi-hostpath-provisioner.yaml",
		"test/e2e/testing-manifests/storage-csi/hostpath/hostpath/csi-hostpath-resizer.yaml",
		"test/e2e/testing-manifests/storage-csi/hostpath/hostpath/csi-hostpath-snapshotter.yaml",
		"test/e2e/testing-manifests/storage-csi/hostpath/hostpath/e2e-test-rbac.yaml",
	)
}

func (h *hostpathCSIDriver) GetDriverInfo() *testsuites.DriverInfo {
	return &h.driverInfo
}

func (h *hostpathCSIDriver) SkipUnsupportedTest(pattern testpatterns.TestPattern) {
	if pattern.VolType == testpatterns.CSIInlineVolume && len(h.volumeAttributes) == 0 {
		framework.Skipf("%s has no volume attributes defined, doesn't support ephemeral inline volumes", h.driverInfo.Name)
	}
}

func (h *hostpathCSIDriver) GetDynamicProvisionStorageClass(config *testsuites.PerTestConfig, fsType string) *storagev1.StorageClass {
	provisioner := config.GetUniqueDriverName()
	parameters := map[string]string{}
	ns := config.Framework.Namespace.Name
	suffix := fmt.Sprintf("%s-sc", provisioner)

	return testsuites.GetStorageClass(provisioner, parameters, nil, ns, suffix)
}

func (h *hostpathCSIDriver) GetVolume(config *testsuites.PerTestConfig, volumeNumber int) (map[string]string, bool, bool) {
	return h.volumeAttributes[volumeNumber%len(h.volumeAttributes)], false /* not shared */, false /* read-write */
}

func (h *hostpathCSIDriver) GetCSIDriverName(config *testsuites.PerTestConfig) string {
	return config.GetUniqueDriverName()
}

func (h *hostpathCSIDriver) GetSnapshotClass(config *testsuites.PerTestConfig) *unstructured.Unstructured {
	snapshotter := config.GetUniqueDriverName()
	parameters := map[string]string{}
	ns := config.Framework.Namespace.Name
	suffix := fmt.Sprintf("%s-vsc", snapshotter)

	return testsuites.GetSnapshotClass(snapshotter, parameters, ns, suffix)
}

func (h *hostpathCSIDriver) PrepareTest(f *framework.Framework) (*testsuites.PerTestConfig, func()) {
	ginkgo.By(fmt.Sprintf("deploying %s driver", h.driverInfo.Name))
	cancelLogging := testsuites.StartPodLogs(f)
	cs := f.ClientSet

	// The hostpath CSI driver only works when everything runs on the same node.
	node, err := e2enode.GetRandomReadySchedulableNode(cs)
	framework.ExpectNoError(err)
	config := &testsuites.PerTestConfig{
		Driver:         h,
		Prefix:         "hostpath",
		Framework:      f,
		ClientNodeName: node.Name,
	}

	o := utils.PatchCSIOptions{
		OldDriverName:            h.driverInfo.Name,
		NewDriverName:            config.GetUniqueDriverName(),
		DriverContainerName:      "hostpath",
		DriverContainerArguments: []string{"--drivername=" + config.GetUniqueDriverName()},
		ProvisionerContainerName: "csi-provisioner",
		SnapshotterContainerName: "csi-snapshotter",
		NodeName:                 node.Name,
	}
	cleanup, err := utils.CreateFromManifests(config.Framework, func(item interface{}) error {
		return utils.PatchCSIDeployment(config.Framework, o, item)
	},
		h.manifests...)
	if err != nil {
		framework.Failf("deploying %s driver: %v", h.driverInfo.Name, err)
	}

	return config, func() {
		ginkgo.By(fmt.Sprintf("uninstalling %s driver", h.driverInfo.Name))
		cleanup()
		cancelLogging()
	}
}

// mockCSI
type mockCSIDriver struct {
	driverInfo          testsuites.DriverInfo
	manifests           []string
	podInfo             *bool
	attachable          bool
	attachLimit         int
	enableNodeExpansion bool
}

// CSIMockDriverOpts defines options used for csi driver
type CSIMockDriverOpts struct {
	RegisterDriver      bool
	DisableAttach       bool
	PodInfo             *bool
	AttachLimit         int
	EnableResizing      bool
	EnableNodeExpansion bool
}

var _ testsuites.TestDriver = &mockCSIDriver{}
var _ testsuites.DynamicPVTestDriver = &mockCSIDriver{}

// InitMockCSIDriver returns a mockCSIDriver that implements TestDriver interface
func InitMockCSIDriver(driverOpts CSIMockDriverOpts) testsuites.TestDriver {
	driverManifests := []string{
		"test/e2e/testing-manifests/storage-csi/external-attacher/rbac.yaml",
		"test/e2e/testing-manifests/storage-csi/external-provisioner/rbac.yaml",
		"test/e2e/testing-manifests/storage-csi/external-resizer/rbac.yaml",
		"test/e2e/testing-manifests/storage-csi/mock/csi-mock-rbac.yaml",
		"test/e2e/testing-manifests/storage-csi/mock/csi-storageclass.yaml",
		"test/e2e/testing-manifests/storage-csi/mock/csi-mock-driver.yaml",
	}

	if driverOpts.RegisterDriver {
		driverManifests = append(driverManifests, "test/e2e/testing-manifests/storage-csi/mock/csi-mock-driverinfo.yaml")
	}

	if !driverOpts.DisableAttach {
		driverManifests = append(driverManifests, "test/e2e/testing-manifests/storage-csi/mock/csi-mock-driver-attacher.yaml")
	}

	if driverOpts.EnableResizing {
		driverManifests = append(driverManifests, "test/e2e/testing-manifests/storage-csi/mock/csi-mock-driver-resizer.yaml")
	}

	return &mockCSIDriver{
		driverInfo: testsuites.DriverInfo{
			Name:        "csi-mock",
			FeatureTag:  "",
			MaxFileSize: testpatterns.FileSizeMedium,
			SupportedFsType: sets.NewString(
				"", // Default fsType
			),
			Capabilities: map[testsuites.Capability]bool{
				testsuites.CapPersistence:  false,
				testsuites.CapFsGroup:      false,
				testsuites.CapExec:         false,
				testsuites.CapVolumeLimits: true,
			},
		},
		manifests:           driverManifests,
		podInfo:             driverOpts.PodInfo,
		attachable:          !driverOpts.DisableAttach,
		attachLimit:         driverOpts.AttachLimit,
		enableNodeExpansion: driverOpts.EnableNodeExpansion,
	}
}

func (m *mockCSIDriver) GetDriverInfo() *testsuites.DriverInfo {
	return &m.driverInfo
}

func (m *mockCSIDriver) SkipUnsupportedTest(pattern testpatterns.TestPattern) {
}

func (m *mockCSIDriver) GetDynamicProvisionStorageClass(config *testsuites.PerTestConfig, fsType string) *storagev1.StorageClass {
	provisioner := config.GetUniqueDriverName()
	parameters := map[string]string{}
	ns := config.Framework.Namespace.Name
	suffix := fmt.Sprintf("%s-sc", provisioner)

	return testsuites.GetStorageClass(provisioner, parameters, nil, ns, suffix)
}

func (m *mockCSIDriver) PrepareTest(f *framework.Framework) (*testsuites.PerTestConfig, func()) {
	ginkgo.By("deploying csi mock driver")
	cancelLogging := testsuites.StartPodLogs(f)
	cs := f.ClientSet

	// pods should be scheduled on the node
	node, err := e2enode.GetRandomReadySchedulableNode(cs)
	framework.ExpectNoError(err)
	config := &testsuites.PerTestConfig{
		Driver:         m,
		Prefix:         "mock",
		Framework:      f,
		ClientNodeName: node.Name,
	}

	containerArgs := []string{"--name=csi-mock-" + f.UniqueName}
	if !m.attachable {
		containerArgs = append(containerArgs, "--disable-attach")
	}

	if m.attachLimit > 0 {
		containerArgs = append(containerArgs, "--attach-limit", strconv.Itoa(m.attachLimit))
	}

	if m.enableNodeExpansion {
		containerArgs = append(containerArgs, "--node-expand-required=true")
	}

	o := utils.PatchCSIOptions{
		OldDriverName:            "csi-mock",
		NewDriverName:            "csi-mock-" + f.UniqueName,
		DriverContainerName:      "mock",
		DriverContainerArguments: containerArgs,
		ProvisionerContainerName: "csi-provisioner",
		NodeName:                 config.ClientNodeName,
		PodInfo:                  m.podInfo,
		CanAttach:                &m.attachable,
		VolumeLifecycleModes: &[]storagev1beta1.VolumeLifecycleMode{
			storagev1beta1.VolumeLifecyclePersistent,
			storagev1beta1.VolumeLifecycleEphemeral,
		},
	}
	cleanup, err := utils.CreateFromManifests(f, func(item interface{}) error {
		return utils.PatchCSIDeployment(f, o, item)
	},
		m.manifests...)
	if err != nil {
		framework.Failf("deploying csi mock driver: %v", err)
	}

	return config, func() {
		ginkgo.By("uninstalling csi mock driver")
		cleanup()
		cancelLogging()
	}
}

// gce-pd
type gcePDCSIDriver struct {
	driverInfo testsuites.DriverInfo
}

var _ testsuites.TestDriver = &gcePDCSIDriver{}
var _ testsuites.DynamicPVTestDriver = &gcePDCSIDriver{}

// InitGcePDCSIDriver returns gcePDCSIDriver that implements TestDriver interface
func InitGcePDCSIDriver() testsuites.TestDriver {
	return &gcePDCSIDriver{
		driverInfo: testsuites.DriverInfo{
			Name:        GCEPDCSIDriverName,
			FeatureTag:  "[Serial]",
			MaxFileSize: testpatterns.FileSizeMedium,
			SupportedSizeRange: volume.SizeRange{
				Min: "5Gi",
			},
			SupportedFsType: sets.NewString(
				"", // Default fsType
				"ext2",
				"ext3",
				"ext4",
				"xfs",
			),
			SupportedMountOption: sets.NewString("debug", "nouid32"),
			Capabilities: map[testsuites.Capability]bool{
				testsuites.CapPersistence: true,
				testsuites.CapBlock:       true,
				testsuites.CapFsGroup:     true,
				testsuites.CapExec:        true,
				testsuites.CapMultiPODs:   true,
				// GCE supports volume limits, but the test creates large
				// number of volumes and times out test suites.
				testsuites.CapVolumeLimits:        false,
				testsuites.CapTopology:            true,
				testsuites.CapControllerExpansion: true,
				testsuites.CapNodeExpansion:       true,
			},
			RequiredAccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
			TopologyKeys:        []string{GCEPDCSIZoneTopologyKey},
		},
	}
}

func (g *gcePDCSIDriver) GetDriverInfo() *testsuites.DriverInfo {
	return &g.driverInfo
}

func (g *gcePDCSIDriver) SkipUnsupportedTest(pattern testpatterns.TestPattern) {
	framework.SkipUnlessProviderIs("gce", "gke")
	if pattern.FsType == "xfs" {
		framework.SkipUnlessNodeOSDistroIs("ubuntu", "custom")
	}
	if pattern.FeatureTag == "[sig-windows]" {
		framework.Skipf("Skipping tests for windows since CSI does not support it yet")
	}
}

func (g *gcePDCSIDriver) GetDynamicProvisionStorageClass(config *testsuites.PerTestConfig, fsType string) *storagev1.StorageClass {
	ns := config.Framework.Namespace.Name
	provisioner := g.driverInfo.Name
	suffix := fmt.Sprintf("%s-sc", g.driverInfo.Name)

	parameters := map[string]string{"type": "pd-standard"}
	if fsType != "" {
		parameters["csi.storage.k8s.io/fstype"] = fsType
	}
	delayedBinding := storagev1.VolumeBindingWaitForFirstConsumer

	return testsuites.GetStorageClass(provisioner, parameters, &delayedBinding, ns, suffix)
}

func (g *gcePDCSIDriver) PrepareTest(f *framework.Framework) (*testsuites.PerTestConfig, func()) {
	ginkgo.By("deploying csi gce-pd driver")
	cancelLogging := testsuites.StartPodLogs(f)
	// It would be safer to rename the gcePD driver, but that
	// hasn't been done before either and attempts to do so now led to
	// errors during driver registration, therefore it is disabled
	// by passing a nil function below.
	//
	// These are the options which would have to be used:
	// o := utils.PatchCSIOptions{
	// 	OldDriverName:            g.driverInfo.Name,
	// 	NewDriverName:            testsuites.GetUniqueDriverName(g),
	// 	DriverContainerName:      "gce-driver",
	// 	ProvisionerContainerName: "csi-external-provisioner",
	// }
	createGCESecrets(f.ClientSet, f.Namespace.Name)

	manifests := []string{
		"test/e2e/testing-manifests/storage-csi/external-attacher/rbac.yaml",
		"test/e2e/testing-manifests/storage-csi/external-provisioner/rbac.yaml",
		"test/e2e/testing-manifests/storage-csi/gce-pd/csi-controller-rbac.yaml",
		"test/e2e/testing-manifests/storage-csi/gce-pd/node_ds.yaml",
		"test/e2e/testing-manifests/storage-csi/gce-pd/controller_ss.yaml",
	}

	cleanup, err := utils.CreateFromManifests(f, nil, manifests...)
	if err != nil {
		framework.Failf("deploying csi gce-pd driver: %v", err)
	}

	if err = waitForCSIDriverRegistrationOnAllNodes(GCEPDCSIDriverName, f.ClientSet); err != nil {
		framework.Failf("waiting for csi driver node registration on: %v", err)
	}

	return &testsuites.PerTestConfig{
			Driver:    g,
			Prefix:    "gcepd",
			Framework: f,
		}, func() {
			ginkgo.By("uninstalling gce-pd driver")
			cleanup()
			cancelLogging()
		}
}

func waitForCSIDriverRegistrationOnAllNodes(driverName string, cs clientset.Interface) error {
	nodes, err := e2enode.GetReadySchedulableNodes(cs)
	if err != nil {
		return err
	}
	for _, node := range nodes.Items {
		if err := waitForCSIDriverRegistrationOnNode(node.Name, driverName, cs); err != nil {
			return err
		}
	}
	return nil
}

func waitForCSIDriverRegistrationOnNode(nodeName string, driverName string, cs clientset.Interface) error {
	const csiNodeRegisterTimeout = 1 * time.Minute

	return wait.PollImmediate(10*time.Second, csiNodeRegisterTimeout, func() (bool, error) {
		csiNode, err := cs.StorageV1().CSINodes().Get(nodeName, metav1.GetOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return false, err
		}
		for _, driver := range csiNode.Spec.Drivers {
			if driver.Name == driverName {
				return true, nil
			}
		}
		return false, nil
	})
}
