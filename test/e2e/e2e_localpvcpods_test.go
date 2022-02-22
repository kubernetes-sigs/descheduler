/*
Copyright 2021 The Kubernetes Authors.

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

package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	//persistentvolumev1 "k8s.io.api/persistentvolume/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	deschedulerapi "sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	eutils "sigs.k8s.io/descheduler/pkg/descheduler/evictions/utils"
	"sigs.k8s.io/descheduler/pkg/descheduler/strategies"
)

// FIXME tests are disabled
// Is it possible to do PV/PVC testing with Kind?
// PVCs remain in Pending forever, won't attach to PVs
func TestRemoveLocalPVCPods(t *testing.T) {
	ctx := context.Background()

	clientSet, _, getPodsAssignedToNode, stopCh := initializeClient(t)
	defer close(stopCh)

	nodeList, err := clientSet.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Errorf("Error listing node with %v", err)
	}

	nodes, workerNodes := splitNodesAndWorkerNodes(nodeList.Items)

	t.Log("Creating testing namespace")
	testNamespace := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "e2e-" + strings.ToLower(t.Name())}}
	if _, err := clientSet.CoreV1().Namespaces().Create(ctx, testNamespace, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Unable to create ns %v", testNamespace.Name)
	}
	defer clientSet.CoreV1().Namespaces().Delete(ctx, testNamespace.Name, metav1.DeleteOptions{})

	t.Log("Creating StorageClass")

	var bindingMode storagev1.VolumeBindingMode
	bindingMode = "WaitForFirstConsumer"
	var reclaimPolicy v1.PersistentVolumeReclaimPolicy
	reclaimPolicy = "Delete"

	storageClassObj := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "local-ssd",
			Namespace: testNamespace.Name,
		},
		Provisioner:       "kubernetes.io/no-provisioner",
		VolumeBindingMode: &bindingMode,
		ReclaimPolicy:     &reclaimPolicy,
	}

	t.Log(storageClassObj)

	_, err = clientSet.StorageV1().StorageClasses().Create(ctx, storageClassObj, metav1.CreateOptions{})
	if err != nil {
		t.Logf("Error creating storageClass: %v", err)
		if err = clientSet.StorageV1().StorageClasses().Delete(ctx, storageClassObj.Name, metav1.DeleteOptions{}); err != nil {
			t.Fatalf("Unable to delete storageClass: %v", err)
		}
		return
	}
	defer clientSet.StorageV1().StorageClasses().Delete(ctx, storageClassObj.Name, metav1.DeleteOptions{})

	t.Log("Creating local Persistent Volumes")

	persistentVolume1Obj := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:     "pv1",
			SelfLink: fmt.Sprintf("/api/v1/persistentvolume/%s", "pv1"),
			Labels:   map[string]string{},
		},
		Spec: v1.PersistentVolumeSpec{
			StorageClassName: "local-ssd",
			PersistentVolumeSource: v1.PersistentVolumeSource{
				Local: &v1.LocalVolumeSource{
					Path: fmt.Sprintf("/tmp/%s", "pv1"),
				},
			},
			PersistentVolumeReclaimPolicy: "Delete",
			Capacity: v1.ResourceList{
				v1.ResourceName(v1.ResourceStorage): resource.MustParse("2G"),
			},
			AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
			NodeAffinity: &v1.VolumeNodeAffinity{
				&v1.NodeSelector{
					NodeSelectorTerms: []v1.NodeSelectorTerm{
						{
							MatchExpressions: []v1.NodeSelectorRequirement{
								{
									Key:      "kubernetes.io/hostname",
									Operator: "In",
									Values:   []string{"kind-worker"},
								},
							},
						},
					},
				},
			},
		},
	}

	_, err = clientSet.CoreV1().PersistentVolumes().Create(ctx, persistentVolume1Obj, metav1.CreateOptions{})
	if err != nil {
		t.Logf("Error creating PV1: %v", err)
		if err = clientSet.CoreV1().PersistentVolumes().Delete(ctx, persistentVolume1Obj.Name, metav1.DeleteOptions{}); err != nil {
			t.Fatalf("Unable to delete PV1: %v", err)
		}
		return
	}

	defer clientSet.CoreV1().PersistentVolumes().Delete(ctx, persistentVolume1Obj.Name, metav1.DeleteOptions{})

	persistentVolume2Obj := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:     "pv2",
			SelfLink: fmt.Sprintf("/api/v1/persistentvolume/%s", "pv2"),
			Labels:   map[string]string{},
		},
		Spec: v1.PersistentVolumeSpec{
			StorageClassName: "local-ssd",
			PersistentVolumeSource: v1.PersistentVolumeSource{
				Local: &v1.LocalVolumeSource{
					Path: fmt.Sprintf("/tmp/%s", "pv2"),
				},
			},
			Capacity: v1.ResourceList{
				v1.ResourceName(v1.ResourceStorage): resource.MustParse("2G"),
			},
			PersistentVolumeReclaimPolicy: "Delete",
			AccessModes:                   []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
			NodeAffinity: &v1.VolumeNodeAffinity{
				&v1.NodeSelector{
					NodeSelectorTerms: []v1.NodeSelectorTerm{
						{
							MatchExpressions: []v1.NodeSelectorRequirement{
								{
									Key:      "kubernetes.io/hostname",
									Operator: "In",
									Values:   []string{"kind-worker2"},
								},
							},
						},
					},
				},
			},
		},
	}

	_, err = clientSet.CoreV1().PersistentVolumes().Create(ctx, persistentVolume2Obj, metav1.CreateOptions{})
	if err != nil {
		t.Logf("Error creating PV2: %v", err)
		if err = clientSet.CoreV1().PersistentVolumes().Delete(ctx, persistentVolume2Obj.Name, metav1.DeleteOptions{}); err != nil {
			t.Fatalf("Unable to delete PV2: %v", err)
		}
		return
	}

	defer clientSet.CoreV1().PersistentVolumes().Delete(ctx, persistentVolume2Obj.Name, metav1.DeleteOptions{})

	t.Log("Creating localPVC pods")

	statefulSetObj := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "localpvcpods",
			Namespace: testNamespace.Name,
			Labels:    map[string]string{"app": "localpvc", "name": "localpvcpods"},
		},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "localpvc", "name": "localpvcpods"},
			},
			VolumeClaimTemplates: []v1.PersistentVolumeClaim{{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "www",
					Labels: map[string]string{"app": "localpvc", "name": "localpvcpods"},
				},
				Spec: v1.PersistentVolumeClaimSpec{
					AccessModes: []v1.PersistentVolumeAccessMode{
						v1.ReadWriteOnce,
					},
					VolumeName:       "www",
					StorageClassName: &storageClassObj.Name,
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							"storage": resource.MustParse("1G"),
						},
					},
				},
			}},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "testpvcpod",
					Labels: map[string]string{"app": "localpvc", "name": "localpvcpods"},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:            "pause",
							ImagePullPolicy: "Always",
							Image:           "kubernetes/pause",
							Ports:           []v1.ContainerPort{{ContainerPort: 80}},
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "www",
									MountPath: "/data/www",
								},
							},
						},
					},
					// Volumes: []v1.Volume{
					// 	{
					// 		Name: "www",
					// 		VolumeSource: v1.VolumeSource{
					// 			PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
					// 				ClaimName: "localpvcpods",
					// 			},
					// 		},
					// 	},
					// },
				},
			},
		},
	}

	tests := []struct {
		description             string
		replicasNum             int
		beforeFunc              func(statefulSet *appsv1.StatefulSet)
		expectedEvictedPodCount uint
		MaxPodLifeTimeSeconds   uint
		ignoreLocalPvcPods      bool
		ignorePvcPods           bool
		evictLocalStoragePods   bool
	}{
		// {
		// 	description: "Should not evict pods when ignoreLocalPvcPods is true",
		// 	replicasNum: 2,
		// 	beforeFunc: func(statefulSet *appsv1.StatefulSet) {
		// 		statefulSet.Spec.Replicas = func(i int32) *int32 { return &i }(2)
		// 		// statefulSet.Spec.Template.Spec.NodeName = workerNodes[0].Name
		// 	},
		// 	expectedEvictedPodCount: 0,
		// 	MaxPodLifeTimeSeconds:   60,
		// 	evictLocalStoragePods:   false,
		// 	ignoreLocalPvcPods:      true,
		// 	ignorePvcPods:           true,
		// },
		// {
		// 	description: "Evict Pods with local PVC storage",
		// 	replicasNum: 2,
		// 	expectedEvictedPodCount: 2,
		// 	MaxPodLifeTimeSeconds:   60,
		// 	evictLocalStoragePods:   true,
		// 	ignoreLocalPvcPods:      false,
		// 	ignorePvcPods:           false,
		// },
	}
	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			t.Logf("Creating statefulset %v in %v namespace", statefulSetObj.Name, statefulSetObj.Namespace)
			tc.beforeFunc(statefulSetObj)

			_, err = clientSet.AppsV1().StatefulSets(statefulSetObj.Namespace).Create(ctx, statefulSetObj, metav1.CreateOptions{})
			if err != nil {
				t.Logf("Error creating statefulset: %v", err)
				if err = clientSet.AppsV1().StatefulSets(statefulSetObj.Namespace).DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{
					LabelSelector: labels.SelectorFromSet(labels.Set(map[string]string{"app": "localpvcPods", "name": "localpvcPodsPods"})).String(),
				}); err != nil {
					t.Fatalf("Unable to delete statefulset: %v", err)
				}
				return
			}

			defer clientSet.AppsV1().StatefulSets(statefulSetObj.Namespace).Delete(ctx, statefulSetObj.Name, metav1.DeleteOptions{})
			waitForPodsRunning(ctx, t, clientSet, map[string]string{"app": "localpvcPods", "name": "localpvcPods"}, tc.replicasNum, testNamespace.Name)

			// Run DeschedulerStrategy strategy
			evictionPolicyGroupVersion, err := eutils.SupportEviction(clientSet)
			if err != nil || len(evictionPolicyGroupVersion) == 0 {
				t.Fatalf("Error creating eviction policy group %v", err)
			}
			podEvictor := evictions.NewPodEvictor(
				clientSet,
				evictionPolicyGroupVersion,
				false,
				nil,
				nil,
				nodes,
				tc.evictLocalStoragePods,
				false,
				tc.ignorePvcPods,
				tc.ignoreLocalPvcPods,
				false,
				false,
			)
			t.Log("Running DeschedulerStrategy strategy")
			strategies.PodLifeTime(
				ctx,
				clientSet,
				deschedulerapi.DeschedulerStrategy{
					Enabled: true,
					Params: &deschedulerapi.StrategyParameters{
						PodLifeTime: &deschedulerapi.PodLifeTime{
							MaxPodLifeTimeSeconds: &tc.MaxPodLifeTimeSeconds},
					},
				},
				workerNodes,
				podEvictor,
				getPodsAssignedToNode,
			)

			waitForTerminatingPodsToDisappear(ctx, t, clientSet, testNamespace.Name)
			actualEvictedPodCount := podEvictor.TotalEvicted()
			if actualEvictedPodCount != tc.expectedEvictedPodCount {
				t.Errorf("Test error for description: %s. Unexpected number of pods have been evicted, got %v, expected %v", tc.description, actualEvictedPodCount, tc.expectedEvictedPodCount)
			}
		})
	}
}
