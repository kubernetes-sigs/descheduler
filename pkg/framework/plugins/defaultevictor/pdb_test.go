/*
Copyright 2022 The Kubernetes Authors.
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

package defaultevictor

import (
	"context"
	"fmt"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/informers"
	fakeclient "k8s.io/client-go/kubernetes/fake"
	"k8s.io/klog/v2"
	utilptr "k8s.io/utils/ptr"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	frameworkfake "sigs.k8s.io/descheduler/pkg/framework/fake"
	testutil "sigs.k8s.io/descheduler/test"
)

func TestShouldHandlePDB_SingleReplicaDeployment(t *testing.T) {
	ctx := context.Background()

	// Build a single-replica deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "single-replica-app",
			Namespace: "default",
			UID:       "deployment-uid-123",
			Labels: map[string]string{
				"app.kubernetes.io/name":      "single-replica-app",
				"app.kubernetes.io/component": "core",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: utilptr.To[int32](1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name":      "single-replica-app",
					"app.kubernetes.io/component": "core",
				},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/name":      "single-replica-app",
						"app.kubernetes.io/component": "core",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "app",
							Image: "nginx:latest",
						},
					},
				},
			},
		},
	}

	// Build a pod from the single-replica deployment
	pod := testutil.BuildTestPod("single-replica-app-xyz123-abc45", 400, 0, "node1", func(pod *v1.Pod) {
		pod.Labels = map[string]string{
			"app.kubernetes.io/name":      "single-replica-app",
			"app.kubernetes.io/component": "core",
		}
		pod.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "single-replica-app",
				UID:        "deployment-uid-123",
			},
		}
	})

	// Build a PDB that protects the pod (manually created, not operator-managed)
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "single-replica-app-min-replica-pdb",
			Namespace: "default",
			Labels: map[string]string{
				"app": "single-replica-app",
			},
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MaxUnavailable: utilptr.To[intstr.IntOrString](intstr.FromInt32(0)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name":      "single-replica-app",
					"app.kubernetes.io/component": "core",
				},
			},
		},
	}

	// Create fake client with deployment, pod, and PDB
	objs := []runtime.Object{deployment, pod, pdb}
	defaultEvictor := newTestEvictor(ctx, t, &DefaultEvictorArgs{
		DeletePDBsForSingleReplicaDeployments: true,
	}, objs...)

	logger := klog.FromContext(ctx)

	// Test shouldHandlePDB with single-replica deployment
	action, reason := defaultEvictor.shouldHandlePDB(pdb, []*v1.Pod{pod}, []*v1.Pod{pod}, logger)

	if action != PDBActionModify {
		t.Errorf("Expected action PDBActionModify for single-replica deployment, got %s (reason: %s)", action, reason)
	}
}

func TestShouldHandlePDB_MultiReplicaDeployment_AllOnNode(t *testing.T) {
	ctx := context.Background()

	// Build a multi-replica deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "multi-replica-gateway",
			Namespace: "networking",
			UID:       "deployment-uid-456",
			Labels: map[string]string{
				"app.kubernetes.io/name":      "multi-replica-gateway",
				"app.kubernetes.io/component": "gateway",
				"region":                      "us-west",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: utilptr.To[int32](3),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name":      "multi-replica-gateway",
					"app.kubernetes.io/component": "gateway",
				},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/name":      "multi-replica-gateway",
						"app.kubernetes.io/component": "gateway",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "gateway",
							Image: "gateway:latest",
						},
					},
				},
			},
		},
	}

	// Build pods from the multi-replica deployment (all on node1)
	pods := make([]*v1.Pod, 3)
	podNames := []string{"multi-replica-gateway-abc12-def34", "multi-replica-gateway-abc12-def35", "multi-replica-gateway-abc12-def36"}
	for i := 0; i < 3; i++ {
		pods[i] = testutil.BuildTestPod(podNames[i], 400, 0, "node1", func(pod *v1.Pod) {
			pod.Namespace = "networking"
			pod.Labels = map[string]string{
				"app.kubernetes.io/name":      "multi-replica-gateway",
				"app.kubernetes.io/component": "gateway",
			}
			pod.OwnerReferences = []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "multi-replica-gateway",
					UID:        "deployment-uid-456",
				},
			}
		})
	}

	// Build a PDB that protects the pods (Kyverno-generated style with 50% minAvailable)
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "multi-replica-gateway-pdb",
			Namespace: "networking",
			Labels: map[string]string{
				"app.kubernetes.io/managed-by":     "kyverno",
				"generate.kyverno.io/policy-name":  "zone-affinity-pdb-generator",
				"generate.kyverno.io/rule-name":    "zone-affinity-pdb",
				"generate.kyverno.io/trigger-kind": "Deployment",
				"generate.kyverno.io/trigger-name": "multi-replica-gateway",
			},
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: utilptr.To[intstr.IntOrString](intstr.FromInt32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name":      "multi-replica-gateway",
					"app.kubernetes.io/component": "gateway",
				},
			},
		},
	}

	// Create fake client with deployment, pods, and PDB
	objs := []runtime.Object{deployment, pdb}
	for _, pod := range pods {
		objs = append(objs, pod)
	}
	defaultEvictor := newTestEvictor(ctx, t, &DefaultEvictorArgs{
		DeletePDBsForSingleReplicaDeployments: true,
	}, objs...)

	logger := klog.FromContext(ctx)

	// Relaxing a healthy multi-replica workload is gated behind
	// deletePDBsForUnderreplicatedDeployments, which is off here. Enabling only
	// the single-replica behaviour must not touch this PDB.
	action, reason := defaultEvictor.shouldHandlePDB(pdb, pods, pods, logger)

	if action != PDBActionNone {
		t.Errorf("Expected action PDBActionNone for multi-replica when only single-replica handling is enabled, got %s (reason: %s)", action, reason)
	}

	// With the multi-replica flag on, all 3 pods being on the drain target means
	// the budget has nowhere to draw availability from, so it is relaxed.
	defaultEvictor.args.DeletePDBsForUnderreplicatedDeployments = true

	action, reason = defaultEvictor.shouldHandlePDB(pdb, pods, pods, logger)

	if action != PDBActionModify {
		t.Errorf("Expected action PDBActionModify for multi-replica with all on node, got %s (reason: %s)", action, reason)
	}
}

func TestShouldHandlePDB_SingleReplicaWithMultipleNamesOnNode(t *testing.T) {
	ctx := context.Background()

	// Build a single-replica deployment matching the real issue scenario (Calyptia logging pipeline)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gamma-cism-v1-0-0-gamma-cism-v1-0-0-logs",
			Namespace: "calyptia",
			UID:       "1b8e450d-6dc9-4fc5-8363-01f894227cdd",
			Labels: map[string]string{
				"app.kubernetes.io/name":      "gamma-cism-v1-0-0",
				"app.kubernetes.io/component": "calyptia-core",
				"core-pipeline":               "gamma-cism-v1-0-0-gamma-cism-v1-0-0-logs",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: utilptr.To[int32](1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/component": "calyptia-core",
					"core-pipeline":               "gamma-cism-v1-0-0-gamma-cism-v1-0-0-logs",
				},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/component": "calyptia-core",
						"core-pipeline":               "gamma-cism-v1-0-0-gamma-cism-v1-0-0-logs",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "calyptia",
							Image: "calyptia/core:latest",
						},
					},
				},
			},
		},
	}

	// Build a pod from the single-replica deployment (matching the issue scenario exactly)
	pod := testutil.BuildTestPod("gamma-cism-v1-0-0-gamma-cism-v1-0-0-logs-abc123-def456", 400, 0, "node1", func(pod *v1.Pod) {
		pod.Labels = map[string]string{
			"app.kubernetes.io/component": "calyptia-core",
			"core-pipeline":               "gamma-cism-v1-0-0-gamma-cism-v1-0-0-logs",
		}
		pod.Namespace = "calyptia"
		pod.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "gamma-cism-v1-0-0-gamma-cism-v1-0-0-logs",
				UID:        "1b8e450d-6dc9-4fc5-8363-01f894227cdd",
			},
		}
	})

	// Build a PDB with Kyverno-generated metadata (this is the key test case)
	// The old code would fail here because the PDB name doesn't contain "single-replica"
	// or "min-replica" in the name itself - it's named after the deployment
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gamma-cism-v1-0-0-gamma-cism-v1-0-0-logs-pdb",
			Namespace: "calyptia",
			UID:       "e9ca4f5a-79f6-4e8f-9f04-dbcc63953896",
			Labels: map[string]string{
				"app": "gamma-cism",
			},
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MaxUnavailable: utilptr.To[intstr.IntOrString](intstr.FromInt32(0)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/component": "calyptia-core",
					"core-pipeline":               "gamma-cism-v1-0-0-gamma-cism-v1-0-0-logs",
				},
			},
		},
	}

	// Create fake client
	objs := []runtime.Object{deployment, pod, pdb}
	defaultEvictor := newTestEvictor(ctx, t, &DefaultEvictorArgs{
		DeletePDBsForSingleReplicaDeployments: true,
	}, objs...)

	logger := klog.FromContext(ctx)

	// Test shouldHandlePDB - this is the critical test
	// The old code would return PDBActionNone because the PDB name doesn't contain
	// "min-replica" or "single-replica", even though all pods are from single-replica deployments
	action, reason := defaultEvictor.shouldHandlePDB(pdb, []*v1.Pod{pod}, []*v1.Pod{pod}, logger)

	if action != PDBActionModify {
		t.Errorf("Expected action PDBActionModify for single-replica deployment with generic PDB name, got %s (reason: %s)", action, reason)
	}
}

func TestCalculateMinRequiredPods(t *testing.T) {
	tests := []struct {
		name           string
		minAvailable   *intstr.IntOrString
		maxUnavailable *intstr.IntOrString
		totalPods      int
		want           int
	}{
		{
			name:           "maxUnavailable as an integer",
			maxUnavailable: utilptr.To[intstr.IntOrString](intstr.FromInt32(1)),
			totalPods:      3,
			want:           2,
		},
		{
			// Reading "50%" with IntValue() yields 0, which made minRequired equal
			// totalPods and relaxed every percentage-based PDB unconditionally.
			name:           "maxUnavailable as a percentage",
			maxUnavailable: utilptr.To[intstr.IntOrString](intstr.FromString("50%")),
			totalPods:      4,
			want:           2,
		},
		{
			name:         "minAvailable as an integer",
			minAvailable: utilptr.To[intstr.IntOrString](intstr.FromInt32(2)),
			totalPods:    5,
			want:         2,
		},
		{
			name:         "minAvailable as a percentage",
			minAvailable: utilptr.To[intstr.IntOrString](intstr.FromString("75%")),
			totalPods:    4,
			want:         3,
		},
		{
			name:           "unparseable value fails closed",
			maxUnavailable: utilptr.To[intstr.IntOrString](intstr.FromString("not-a-number")),
			totalPods:      3,
			want:           3,
		},
		{
			name:      "neither field set",
			totalPods: 3,
			want:      1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pdb := &policyv1.PodDisruptionBudget{
				Spec: policyv1.PodDisruptionBudgetSpec{
					MinAvailable:   tt.minAvailable,
					MaxUnavailable: tt.maxUnavailable,
				},
			}
			if got := calculateMinRequiredPods(pdb, tt.totalPods); got != tt.want {
				t.Errorf("calculateMinRequiredPods() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestShouldHandlePDB_HealthyPercentagePDBIsLeftAlone covers the case the
// IntValue() bug got wrong: a well-distributed multi-replica workload behind a
// percentage-based budget has real disruption headroom and must not be relaxed,
// even with every relaxation flag turned on.
func TestShouldHandlePDB_HealthyPercentagePDBIsLeftAlone(t *testing.T) {
	ctx := context.Background()

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "spread-app",
			Namespace: "default",
			UID:       "deployment-uid-789",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: utilptr.To[int32](4),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "spread-app"},
			},
		},
	}

	// Four healthy pods spread over four nodes.
	pods := make([]*v1.Pod, 4)
	nodes := []string{"node1", "node2", "node3", "node4"}
	for i, nodeName := range nodes {
		pods[i] = testutil.BuildTestPod(fmt.Sprintf("spread-app-abc12-%d", i), 400, 0, nodeName, func(pod *v1.Pod) {
			pod.Labels = map[string]string{"app": "spread-app"}
			pod.OwnerReferences = []metav1.OwnerReference{{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "spread-app",
				UID:        "deployment-uid-789",
			}}
		})
	}

	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "spread-app-pdb", Namespace: "default"},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MaxUnavailable: utilptr.To[intstr.IntOrString](intstr.FromString("50%")),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "spread-app"},
			},
		},
	}

	defaultEvictor := newTestEvictor(ctx, t, &DefaultEvictorArgs{
		DeletePDBsForSingleReplicaDeployments:   true,
		DeletePDBsForUnderreplicatedDeployments: true,
	}, deployment, pdb, pods[0], pods[1], pods[2], pods[3])

	// Only the pod on node1 is being drained.
	action, reason := defaultEvictor.shouldHandlePDB(pdb, pods, pods[:1], klog.FromContext(ctx))

	if action != PDBActionNone {
		t.Errorf("Expected action PDBActionNone for a healthy percentage-based PDB, got %s (reason: %s)", action, reason)
	}
}

// TestShouldHandlePDB_UnderreplicatedWorkload checks that "underreplicated" is
// judged against the owner's configured replica count rather than against how
// the pods happen to be spread across nodes.
func TestShouldHandlePDB_UnderreplicatedWorkload(t *testing.T) {
	ctx := context.Background()

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "degraded-app",
			Namespace: "default",
			UID:       "deployment-uid-abc",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: utilptr.To[int32](6),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "degraded-app"},
			},
		},
	}

	// Wants 6, has 4, spread across 4 nodes. Node spread is good; the workload is
	// still short two replicas, which is what should drive the decision.
	pods := make([]*v1.Pod, 4)
	nodes := []string{"node1", "node2", "node3", "node4"}
	for i, nodeName := range nodes {
		pods[i] = testutil.BuildTestPod(fmt.Sprintf("degraded-app-abc12-%d", i), 400, 0, nodeName, func(pod *v1.Pod) {
			pod.Labels = map[string]string{"app": "degraded-app"}
			pod.OwnerReferences = []metav1.OwnerReference{{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "degraded-app",
				UID:        "deployment-uid-abc",
			}}
		})
	}

	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "degraded-app-pdb", Namespace: "default"},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MaxUnavailable: utilptr.To[intstr.IntOrString](intstr.FromInt32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "degraded-app"},
			},
		},
	}

	logger := klog.FromContext(ctx)

	// Off by default, even though the workload is degraded.
	onlySingleReplica := newTestEvictor(ctx, t, &DefaultEvictorArgs{
		DeletePDBsForSingleReplicaDeployments: true,
	}, deployment, pdb, pods[0], pods[1], pods[2], pods[3])

	if action, reason := onlySingleReplica.shouldHandlePDB(pdb, pods, pods[:1], logger); action != PDBActionNone {
		t.Errorf("Expected action PDBActionNone without the underreplicated flag, got %s (reason: %s)", action, reason)
	}

	withUnderreplicated := newTestEvictor(ctx, t, &DefaultEvictorArgs{
		DeletePDBsForUnderreplicatedDeployments: true,
	}, deployment, pdb, pods[0], pods[1], pods[2], pods[3])

	action, reason := withUnderreplicated.shouldHandlePDB(pdb, pods, pods[:1], logger)
	if action != PDBActionModify {
		t.Errorf("Expected action PDBActionModify for an underreplicated workload, got %s (reason: %s)", action, reason)
	}
	if !strings.Contains(reason, "underreplicated-workload") {
		t.Errorf("Expected the underreplicated scenario to fire, got reason %q", reason)
	}
}

// TestRelaxAndRestorePDB walks the full round trip: a blocking budget is widened
// for the drain, then put back exactly as it was.
func TestRelaxAndRestorePDB(t *testing.T) {
	ctx := context.Background()

	node := testutil.BuildTestNode("node1", 2000, 3000, 10, nil)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "solo-app",
			Namespace: "default",
			UID:       "deployment-uid-solo",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: utilptr.To[int32](1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "solo-app"},
			},
		},
	}

	pod := testutil.BuildTestPod("solo-app-abc12-0", 400, 0, "node1", func(pod *v1.Pod) {
		pod.Labels = map[string]string{"app": "solo-app"}
		pod.OwnerReferences = []metav1.OwnerReference{{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       "solo-app",
			UID:        "deployment-uid-solo",
		}}
	})

	originalMinAvailable := intstr.FromInt32(1)
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "solo-app-pdb", Namespace: "default"},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &originalMinAvailable,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "solo-app"},
			},
		},
		// Pre-set so waitForPDBDisruptionsAllowed returns immediately; there is no
		// PDB controller behind the fake client to recompute it.
		Status: policyv1.PodDisruptionBudgetStatus{DisruptionsAllowed: 1},
	}

	defaultEvictor := newTestEvictor(ctx, t, &DefaultEvictorArgs{
		DeletePDBsForSingleReplicaDeployments: true,
	}, deployment, pdb, pod)

	logger := klog.FromContext(ctx)
	client := defaultEvictor.handle.ClientSet().PolicyV1().PodDisruptionBudgets("default")

	defaultEvictor.RelaxPDBsForNode(ctx, node, logger)

	relaxed, err := client.Get(ctx, "solo-app-pdb", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Unable to read the relaxed PDB: %v", err)
	}
	if relaxed.Spec.MinAvailable != nil {
		t.Errorf("Expected minAvailable to be cleared, got %v", relaxed.Spec.MinAvailable)
	}
	if relaxed.Spec.MaxUnavailable == nil || relaxed.Spec.MaxUnavailable.StrVal != "100%" {
		t.Errorf("Expected maxUnavailable 100%%, got %v", relaxed.Spec.MaxUnavailable)
	}
	if _, ok := relaxed.Annotations[pdbOriginalSpecAnnotationKey]; !ok {
		t.Fatalf("Expected the original settings to be recorded in %q", pdbOriginalSpecAnnotationKey)
	}
	if got := relaxed.Annotations[pdbRelaxedForNodeAnnotationKey]; got != "node1" {
		t.Errorf("Expected the PDB to be marked as relaxed for node1, got %q", got)
	}

	defaultEvictor.RestorePDBsForNode(ctx, node, logger)

	restored, err := client.Get(ctx, "solo-app-pdb", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Unable to read the restored PDB: %v", err)
	}
	if restored.Spec.MaxUnavailable != nil {
		t.Errorf("Expected maxUnavailable to be cleared on restore, got %v", restored.Spec.MaxUnavailable)
	}
	if restored.Spec.MinAvailable == nil || restored.Spec.MinAvailable.IntValue() != 1 {
		t.Errorf("Expected minAvailable to be restored to 1, got %v", restored.Spec.MinAvailable)
	}
	if _, ok := restored.Annotations[pdbOriginalSpecAnnotationKey]; ok {
		t.Errorf("Expected %q to be removed on restore", pdbOriginalSpecAnnotationKey)
	}
	if _, ok := restored.Annotations[pdbRelaxedForNodeAnnotationKey]; ok {
		t.Errorf("Expected %q to be removed on restore", pdbRelaxedForNodeAnnotationKey)
	}
}

// TestRestoreOrphanedPDB covers recovery after a descheduler process died
// mid-drain: the budget still carries the annotation but no live run owns it, so
// the next pass must put it back rather than leave it wide open.
func TestRestoreOrphanedPDB(t *testing.T) {
	ctx := context.Background()

	node := testutil.BuildTestNode("node1", 2000, 3000, 10, nil)

	orphan := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "orphaned-pdb",
			Namespace: "default",
			Annotations: map[string]string{
				pdbOriginalSpecAnnotationKey:   `{"minAvailable":2}`,
				pdbRelaxedForNodeAnnotationKey: "node9",
			},
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MaxUnavailable: utilptr.To[intstr.IntOrString](intstr.FromString("100%")),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "orphan-app"},
			},
		},
	}

	defaultEvictor := newTestEvictor(ctx, t, &DefaultEvictorArgs{
		DeletePDBsForSingleReplicaDeployments: true,
	}, orphan)

	defaultEvictor.RelaxPDBsForNode(ctx, node, klog.FromContext(ctx))

	restored, err := defaultEvictor.handle.ClientSet().PolicyV1().PodDisruptionBudgets("default").Get(ctx, "orphaned-pdb", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Unable to read the orphaned PDB: %v", err)
	}
	if restored.Spec.MinAvailable == nil || restored.Spec.MinAvailable.IntValue() != 2 {
		t.Errorf("Expected the orphaned PDB's minAvailable to be restored to 2, got %v", restored.Spec.MinAvailable)
	}
	if restored.Spec.MaxUnavailable != nil {
		t.Errorf("Expected maxUnavailable to be cleared on restore, got %v", restored.Spec.MaxUnavailable)
	}
	if _, ok := restored.Annotations[pdbOriginalSpecAnnotationKey]; ok {
		t.Errorf("Expected %q to be removed on restore", pdbOriginalSpecAnnotationKey)
	}
}

// newTestEvictor wires a DefaultEvictor up to a fake client seeded with objs.
// The informer factory is started after the plugin is built so that the
// informers New registers for the PDB path are actually running.
func newTestEvictor(ctx context.Context, t *testing.T, args *DefaultEvictorArgs, objs ...runtime.Object) *DefaultEvictor {
	t.Helper()

	fakeClient := fakeclient.NewSimpleClientset(objs...)
	sharedInformerFactory := informers.NewSharedInformerFactory(fakeClient, 0)

	podInformer := sharedInformerFactory.Core().V1().Pods().Informer()
	getPodsAssignedToNode, err := podutil.BuildGetPodsAssignedToNodeFunc(podInformer)
	if err != nil {
		t.Fatalf("Unable to build GetPodsAssignedToNodeFunc: %v", err)
	}

	evictorPlugin, err := New(ctx, args, &frameworkfake.HandleImpl{
		ClientsetImpl:                 fakeClient,
		GetPodsAssignedToNodeFuncImpl: getPodsAssignedToNode,
		SharedInformerFactoryImpl:     sharedInformerFactory,
	})
	if err != nil {
		t.Fatalf("Unable to initialize the plugin: %v", err)
	}

	sharedInformerFactory.Start(ctx.Done())
	sharedInformerFactory.WaitForCacheSync(ctx.Done())

	defaultEvictor, ok := evictorPlugin.(*DefaultEvictor)
	if !ok {
		t.Fatalf("Unable to cast plugin to DefaultEvictor")
	}
	return defaultEvictor
}
