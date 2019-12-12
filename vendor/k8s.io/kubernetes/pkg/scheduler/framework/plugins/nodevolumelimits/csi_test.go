/*
Copyright 2019 The Kubernetes Authors.

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

package nodevolumelimits

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	csilibplugins "k8s.io/csi-translation-lib/plugins"
	"k8s.io/kubernetes/pkg/features"
	"k8s.io/kubernetes/pkg/scheduler/algorithm/predicates"
	framework "k8s.io/kubernetes/pkg/scheduler/framework/v1alpha1"
	fakelisters "k8s.io/kubernetes/pkg/scheduler/listers/fake"
	schedulernodeinfo "k8s.io/kubernetes/pkg/scheduler/nodeinfo"
	volumeutil "k8s.io/kubernetes/pkg/volume/util"
	utilpointer "k8s.io/utils/pointer"
)

const (
	ebsCSIDriverName = csilibplugins.AWSEBSDriverName
	gceCSIDriverName = csilibplugins.GCEPDDriverName

	hostpathInTreePluginName = "kubernetes.io/hostpath"
)

// getVolumeLimitKey returns a ResourceName by filter type
func getVolumeLimitKey(filterType string) v1.ResourceName {
	switch filterType {
	case predicates.EBSVolumeFilterType:
		return v1.ResourceName(volumeutil.EBSVolumeLimitKey)
	case predicates.GCEPDVolumeFilterType:
		return v1.ResourceName(volumeutil.GCEVolumeLimitKey)
	case predicates.AzureDiskVolumeFilterType:
		return v1.ResourceName(volumeutil.AzureVolumeLimitKey)
	case predicates.CinderVolumeFilterType:
		return v1.ResourceName(volumeutil.CinderVolumeLimitKey)
	default:
		return v1.ResourceName(volumeutil.GetCSIAttachLimitKey(filterType))
	}
}

func TestCSILimits(t *testing.T) {
	runningPod := &v1.Pod{
		Spec: v1.PodSpec{
			Volumes: []v1.Volume{
				{
					VolumeSource: v1.VolumeSource{
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
							ClaimName: "csi-ebs.csi.aws.com-3",
						},
					},
				},
			},
		},
	}

	pendingVolumePod := &v1.Pod{
		Spec: v1.PodSpec{
			Volumes: []v1.Volume{
				{
					VolumeSource: v1.VolumeSource{
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
							ClaimName: "csi-4",
						},
					},
				},
			},
		},
	}

	// Different pod than pendingVolumePod, but using the same unbound PVC
	unboundPVCPod2 := &v1.Pod{
		Spec: v1.PodSpec{
			Volumes: []v1.Volume{
				{
					VolumeSource: v1.VolumeSource{
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
							ClaimName: "csi-4",
						},
					},
				},
			},
		},
	}

	missingPVPod := &v1.Pod{
		Spec: v1.PodSpec{
			Volumes: []v1.Volume{
				{
					VolumeSource: v1.VolumeSource{
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
							ClaimName: "csi-6",
						},
					},
				},
			},
		},
	}

	noSCPVCPod := &v1.Pod{
		Spec: v1.PodSpec{
			Volumes: []v1.Volume{
				{
					VolumeSource: v1.VolumeSource{
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
							ClaimName: "csi-5",
						},
					},
				},
			},
		},
	}
	gceTwoVolPod := &v1.Pod{
		Spec: v1.PodSpec{
			Volumes: []v1.Volume{
				{
					VolumeSource: v1.VolumeSource{
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
							ClaimName: "csi-pd.csi.storage.gke.io-1",
						},
					},
				},
				{
					VolumeSource: v1.VolumeSource{
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
							ClaimName: "csi-pd.csi.storage.gke.io-2",
						},
					},
				},
			},
		},
	}
	// In-tree volumes
	inTreeOneVolPod := &v1.Pod{
		Spec: v1.PodSpec{
			Volumes: []v1.Volume{
				{
					VolumeSource: v1.VolumeSource{
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
							ClaimName: "csi-kubernetes.io/aws-ebs-0",
						},
					},
				},
			},
		},
	}
	inTreeTwoVolPod := &v1.Pod{
		Spec: v1.PodSpec{
			Volumes: []v1.Volume{
				{
					VolumeSource: v1.VolumeSource{
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
							ClaimName: "csi-kubernetes.io/aws-ebs-1",
						},
					},
				},
				{
					VolumeSource: v1.VolumeSource{
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
							ClaimName: "csi-kubernetes.io/aws-ebs-2",
						},
					},
				},
			},
		},
	}
	// pods with matching csi driver names
	csiEBSOneVolPod := &v1.Pod{
		Spec: v1.PodSpec{
			Volumes: []v1.Volume{
				{
					VolumeSource: v1.VolumeSource{
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
							ClaimName: "csi-ebs.csi.aws.com-0",
						},
					},
				},
			},
		},
	}
	csiEBSTwoVolPod := &v1.Pod{
		Spec: v1.PodSpec{
			Volumes: []v1.Volume{
				{
					VolumeSource: v1.VolumeSource{
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
							ClaimName: "csi-ebs.csi.aws.com-1",
						},
					},
				},
				{
					VolumeSource: v1.VolumeSource{
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
							ClaimName: "csi-ebs.csi.aws.com-2",
						},
					},
				},
			},
		},
	}
	inTreeNonMigratableOneVolPod := &v1.Pod{
		Spec: v1.PodSpec{
			Volumes: []v1.Volume{
				{
					VolumeSource: v1.VolumeSource{
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
							ClaimName: "csi-kubernetes.io/hostpath-0",
						},
					},
				},
			},
		},
	}

	tests := []struct {
		newPod                *v1.Pod
		existingPods          []*v1.Pod
		filterName            string
		maxVols               int
		driverNames           []string
		test                  string
		migrationEnabled      bool
		limitSource           string
		expectedFailureReason *predicates.PredicateFailureError
		wantStatus            *framework.Status
	}{
		{
			newPod:       csiEBSOneVolPod,
			existingPods: []*v1.Pod{runningPod, csiEBSTwoVolPod},
			filterName:   "csi",
			maxVols:      4,
			driverNames:  []string{ebsCSIDriverName},
			test:         "fits when node volume limit >= new pods CSI volume",
			limitSource:  "node",
		},
		{
			newPod:       csiEBSOneVolPod,
			existingPods: []*v1.Pod{runningPod, csiEBSTwoVolPod},
			filterName:   "csi",
			maxVols:      2,
			driverNames:  []string{ebsCSIDriverName},
			test:         "doesn't when node volume limit <= pods CSI volume",
			limitSource:  "node",
			wantStatus:   framework.NewStatus(framework.Unschedulable, predicates.ErrMaxVolumeCountExceeded.GetReason()),
		},
		{
			newPod:       csiEBSOneVolPod,
			existingPods: []*v1.Pod{runningPod, csiEBSTwoVolPod},
			filterName:   "csi",
			maxVols:      2,
			driverNames:  []string{ebsCSIDriverName},
			test:         "should when driver does not support volume limits",
			limitSource:  "csinode-with-no-limit",
		},
		// should count pending PVCs
		{
			newPod:       csiEBSOneVolPod,
			existingPods: []*v1.Pod{pendingVolumePod, csiEBSTwoVolPod},
			filterName:   "csi",
			maxVols:      2,
			driverNames:  []string{ebsCSIDriverName},
			test:         "count pending PVCs towards volume limit <= pods CSI volume",
			limitSource:  "node",
			wantStatus:   framework.NewStatus(framework.Unschedulable, predicates.ErrMaxVolumeCountExceeded.GetReason()),
		},
		// two same pending PVCs should be counted as 1
		{
			newPod:       csiEBSOneVolPod,
			existingPods: []*v1.Pod{pendingVolumePod, unboundPVCPod2, csiEBSTwoVolPod},
			filterName:   "csi",
			maxVols:      4,
			driverNames:  []string{ebsCSIDriverName},
			test:         "count multiple pending pvcs towards volume limit >= pods CSI volume",
			limitSource:  "node",
		},
		// should count PVCs with invalid PV name but valid SC
		{
			newPod:       csiEBSOneVolPod,
			existingPods: []*v1.Pod{missingPVPod, csiEBSTwoVolPod},
			filterName:   "csi",
			maxVols:      2,
			driverNames:  []string{ebsCSIDriverName},
			test:         "should count PVCs with invalid PV name but valid SC",
			limitSource:  "node",
			wantStatus:   framework.NewStatus(framework.Unschedulable, predicates.ErrMaxVolumeCountExceeded.GetReason()),
		},
		// don't count a volume which has storageclass missing
		{
			newPod:       csiEBSOneVolPod,
			existingPods: []*v1.Pod{runningPod, noSCPVCPod},
			filterName:   "csi",
			maxVols:      2,
			driverNames:  []string{ebsCSIDriverName},
			test:         "don't count pvcs with missing SC towards volume limit",
			limitSource:  "node",
		},
		// don't count multiple volume types
		{
			newPod:       csiEBSOneVolPod,
			existingPods: []*v1.Pod{gceTwoVolPod, csiEBSTwoVolPod},
			filterName:   "csi",
			maxVols:      2,
			driverNames:  []string{ebsCSIDriverName, gceCSIDriverName},
			test:         "count pvcs with the same type towards volume limit",
			limitSource:  "node",
			wantStatus:   framework.NewStatus(framework.Unschedulable, predicates.ErrMaxVolumeCountExceeded.GetReason()),
		},
		{
			newPod:       gceTwoVolPod,
			existingPods: []*v1.Pod{csiEBSTwoVolPod, runningPod},
			filterName:   "csi",
			maxVols:      2,
			driverNames:  []string{ebsCSIDriverName, gceCSIDriverName},
			test:         "don't count pvcs with different type towards volume limit",
			limitSource:  "node",
		},
		// Tests for in-tree volume migration
		{
			newPod:           inTreeOneVolPod,
			existingPods:     []*v1.Pod{inTreeTwoVolPod},
			filterName:       "csi",
			maxVols:          2,
			driverNames:      []string{csilibplugins.AWSEBSInTreePluginName, ebsCSIDriverName},
			migrationEnabled: true,
			limitSource:      "csinode",
			test:             "should count in-tree volumes if migration is enabled",
			wantStatus:       framework.NewStatus(framework.Unschedulable, predicates.ErrMaxVolumeCountExceeded.GetReason()),
		},
		{
			newPod:           pendingVolumePod,
			existingPods:     []*v1.Pod{inTreeTwoVolPod},
			filterName:       "csi",
			maxVols:          2,
			driverNames:      []string{csilibplugins.AWSEBSInTreePluginName, ebsCSIDriverName},
			migrationEnabled: true,
			limitSource:      "csinode",
			test:             "should count unbound in-tree volumes if migration is enabled",
			wantStatus:       framework.NewStatus(framework.Unschedulable, predicates.ErrMaxVolumeCountExceeded.GetReason()),
		},
		{
			newPod:           inTreeOneVolPod,
			existingPods:     []*v1.Pod{inTreeTwoVolPod},
			filterName:       "csi",
			maxVols:          2,
			driverNames:      []string{csilibplugins.AWSEBSInTreePluginName, ebsCSIDriverName},
			migrationEnabled: false,
			limitSource:      "csinode",
			test:             "should not count in-tree volume if migration is disabled",
		},
		{
			newPod:           inTreeOneVolPod,
			existingPods:     []*v1.Pod{inTreeTwoVolPod},
			filterName:       "csi",
			maxVols:          2,
			driverNames:      []string{csilibplugins.AWSEBSInTreePluginName, ebsCSIDriverName},
			migrationEnabled: true,
			limitSource:      "csinode-with-no-limit",
			test:             "should not limit pod if volume used does not report limits",
		},
		{
			newPod:           inTreeOneVolPod,
			existingPods:     []*v1.Pod{inTreeTwoVolPod},
			filterName:       "csi",
			maxVols:          2,
			driverNames:      []string{csilibplugins.AWSEBSInTreePluginName, ebsCSIDriverName},
			migrationEnabled: false,
			limitSource:      "csinode-with-no-limit",
			test:             "should not limit in-tree pod if migration is disabled",
		},
		{
			newPod:           inTreeNonMigratableOneVolPod,
			existingPods:     []*v1.Pod{csiEBSTwoVolPod},
			filterName:       "csi",
			maxVols:          2,
			driverNames:      []string{hostpathInTreePluginName, ebsCSIDriverName},
			migrationEnabled: true,
			limitSource:      "csinode",
			test:             "should not count non-migratable in-tree volumes",
		},
		// mixed volumes
		{
			newPod:           inTreeOneVolPod,
			existingPods:     []*v1.Pod{csiEBSTwoVolPod},
			filterName:       "csi",
			maxVols:          2,
			driverNames:      []string{csilibplugins.AWSEBSInTreePluginName, ebsCSIDriverName},
			migrationEnabled: true,
			limitSource:      "csinode",
			test:             "should count in-tree and csi volumes if migration is enabled (when scheduling in-tree volumes)",
			wantStatus:       framework.NewStatus(framework.Unschedulable, predicates.ErrMaxVolumeCountExceeded.GetReason()),
		},
		{
			newPod:           csiEBSOneVolPod,
			existingPods:     []*v1.Pod{inTreeTwoVolPod},
			filterName:       "csi",
			maxVols:          2,
			driverNames:      []string{csilibplugins.AWSEBSInTreePluginName, ebsCSIDriverName},
			migrationEnabled: true,
			limitSource:      "csinode",
			test:             "should count in-tree and csi volumes if migration is enabled (when scheduling csi volumes)",
			wantStatus:       framework.NewStatus(framework.Unschedulable, predicates.ErrMaxVolumeCountExceeded.GetReason()),
		},
		{
			newPod:           csiEBSOneVolPod,
			existingPods:     []*v1.Pod{csiEBSTwoVolPod, inTreeTwoVolPod},
			filterName:       "csi",
			maxVols:          3,
			driverNames:      []string{csilibplugins.AWSEBSInTreePluginName, ebsCSIDriverName},
			migrationEnabled: false,
			limitSource:      "csinode",
			test:             "should not count in-tree and count csi volumes if migration is disabled (when scheduling csi volumes)",
		},
		{
			newPod:           inTreeOneVolPod,
			existingPods:     []*v1.Pod{csiEBSTwoVolPod},
			filterName:       "csi",
			maxVols:          2,
			driverNames:      []string{csilibplugins.AWSEBSInTreePluginName, ebsCSIDriverName},
			migrationEnabled: false,
			limitSource:      "csinode",
			test:             "should not count in-tree and count csi volumes if migration is disabled (when scheduling in-tree volumes)",
		},
	}

	// running attachable predicate tests with feature gate and limit present on nodes
	for _, test := range tests {
		t.Run(test.test, func(t *testing.T) {
			node, csiNode := getNodeWithPodAndVolumeLimits(test.limitSource, test.existingPods, int64(test.maxVols), test.driverNames...)
			if test.migrationEnabled {
				defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.CSIMigration, true)()
				defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.CSIMigrationAWS, true)()
				enableMigrationOnNode(csiNode, csilibplugins.AWSEBSInTreePluginName)
			} else {
				defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.CSIMigration, false)()
				defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.CSIMigrationAWS, false)()
			}

			p := &CSILimits{
				predicate: predicates.NewCSIMaxVolumeLimitPredicate(getFakeCSINodeLister(csiNode), getFakeCSIPVLister(test.filterName, test.driverNames...), getFakeCSIPVCLister(test.filterName, "csi-sc", test.driverNames...), getFakeCSIStorageClassLister("csi-sc", test.driverNames[0])),
			}
			gotStatus := p.Filter(context.Background(), nil, test.newPod, node)
			if !reflect.DeepEqual(gotStatus, test.wantStatus) {
				t.Errorf("status does not match: %v, want: %v", gotStatus, test.wantStatus)
			}
		})
	}
}

func getFakeCSIPVLister(volumeName string, driverNames ...string) fakelisters.PersistentVolumeLister {
	pvLister := fakelisters.PersistentVolumeLister{}
	for _, driver := range driverNames {
		for j := 0; j < 4; j++ {
			volumeHandle := fmt.Sprintf("%s-%s-%d", volumeName, driver, j)
			pv := v1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{Name: volumeHandle},
				Spec: v1.PersistentVolumeSpec{
					PersistentVolumeSource: v1.PersistentVolumeSource{
						CSI: &v1.CSIPersistentVolumeSource{
							Driver:       driver,
							VolumeHandle: volumeHandle,
						},
					},
				},
			}

			switch driver {
			case csilibplugins.AWSEBSInTreePluginName:
				pv.Spec.PersistentVolumeSource = v1.PersistentVolumeSource{
					AWSElasticBlockStore: &v1.AWSElasticBlockStoreVolumeSource{
						VolumeID: volumeHandle,
					},
				}
			case hostpathInTreePluginName:
				pv.Spec.PersistentVolumeSource = v1.PersistentVolumeSource{
					HostPath: &v1.HostPathVolumeSource{
						Path: "/tmp",
					},
				}
			default:
				pv.Spec.PersistentVolumeSource = v1.PersistentVolumeSource{
					CSI: &v1.CSIPersistentVolumeSource{
						Driver:       driver,
						VolumeHandle: volumeHandle,
					},
				}
			}
			pvLister = append(pvLister, pv)
		}

	}
	return pvLister
}

func getFakeCSIPVCLister(volumeName, scName string, driverNames ...string) fakelisters.PersistentVolumeClaimLister {
	pvcLister := fakelisters.PersistentVolumeClaimLister{}
	for _, driver := range driverNames {
		for j := 0; j < 4; j++ {
			v := fmt.Sprintf("%s-%s-%d", volumeName, driver, j)
			pvc := v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{Name: v},
				Spec:       v1.PersistentVolumeClaimSpec{VolumeName: v},
			}
			pvcLister = append(pvcLister, pvc)
		}
	}

	pvcLister = append(pvcLister, v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: volumeName + "-4"},
		Spec:       v1.PersistentVolumeClaimSpec{StorageClassName: &scName},
	})
	pvcLister = append(pvcLister, v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: volumeName + "-5"},
		Spec:       v1.PersistentVolumeClaimSpec{},
	})
	// a pvc with missing PV but available storageclass.
	pvcLister = append(pvcLister, v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: volumeName + "-6"},
		Spec:       v1.PersistentVolumeClaimSpec{StorageClassName: &scName, VolumeName: "missing-in-action"},
	})
	return pvcLister
}

func enableMigrationOnNode(csiNode *storagev1.CSINode, pluginName string) {
	nodeInfoAnnotations := csiNode.GetAnnotations()
	if nodeInfoAnnotations == nil {
		nodeInfoAnnotations = map[string]string{}
	}

	newAnnotationSet := sets.NewString()
	newAnnotationSet.Insert(pluginName)
	nas := strings.Join(newAnnotationSet.List(), ",")
	nodeInfoAnnotations[v1.MigratedPluginsAnnotationKey] = nas

	csiNode.Annotations = nodeInfoAnnotations
}

func getFakeCSIStorageClassLister(scName, provisionerName string) fakelisters.StorageClassLister {
	return fakelisters.StorageClassLister{
		{
			ObjectMeta:  metav1.ObjectMeta{Name: scName},
			Provisioner: provisionerName,
		},
	}
}

func getFakeCSINodeLister(csiNode *storagev1.CSINode) fakelisters.CSINodeLister {
	if csiNode != nil {
		return fakelisters.CSINodeLister(*csiNode)
	}
	return fakelisters.CSINodeLister{}
}

func getNodeWithPodAndVolumeLimits(limitSource string, pods []*v1.Pod, limit int64, driverNames ...string) (*schedulernodeinfo.NodeInfo, *storagev1.CSINode) {
	nodeInfo := schedulernodeinfo.NewNodeInfo(pods...)
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-for-max-pd-test-1"},
		Status: v1.NodeStatus{
			Allocatable: v1.ResourceList{},
		},
	}
	var csiNode *storagev1.CSINode

	addLimitToNode := func() {
		for _, driver := range driverNames {
			node.Status.Allocatable[getVolumeLimitKey(driver)] = *resource.NewQuantity(limit, resource.DecimalSI)
		}
	}

	initCSINode := func() {
		csiNode = &storagev1.CSINode{
			ObjectMeta: metav1.ObjectMeta{Name: "csi-node-for-max-pd-test-1"},
			Spec: storagev1.CSINodeSpec{
				Drivers: []storagev1.CSINodeDriver{},
			},
		}
	}

	addDriversCSINode := func(addLimits bool) {
		initCSINode()
		for _, driver := range driverNames {
			driver := storagev1.CSINodeDriver{
				Name:   driver,
				NodeID: "node-for-max-pd-test-1",
			}
			if addLimits {
				driver.Allocatable = &storagev1.VolumeNodeResources{
					Count: utilpointer.Int32Ptr(int32(limit)),
				}
			}
			csiNode.Spec.Drivers = append(csiNode.Spec.Drivers, driver)
		}
	}

	switch limitSource {
	case "node":
		addLimitToNode()
	case "csinode":
		addDriversCSINode(true)
	case "both":
		addLimitToNode()
		addDriversCSINode(true)
	case "csinode-with-no-limit":
		addDriversCSINode(false)
	case "no-csi-driver":
		initCSINode()
	default:
		// Do nothing.
	}

	nodeInfo.SetNode(node)
	return nodeInfo, csiNode
}
