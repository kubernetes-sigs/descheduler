/*
Copyright 2026 The Kubernetes Authors.

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
	"os"
	"strings"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	componentbaseconfig "k8s.io/component-base/config"

	"sigs.k8s.io/descheduler/pkg/api"
	apiv1alpha2 "sigs.k8s.io/descheduler/pkg/api/v1alpha2"
	"sigs.k8s.io/descheduler/pkg/descheduler/client"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodsviolatingnodetaints"
)

const nodeTaintsE2ETaintKey = "descheduler-e2e.node-taint"

func nodeTaintsPolicy(targetNamespace string, cordon bool, gracePeriodSeconds int64) *apiv1alpha2.DeschedulerPolicy {
	return &apiv1alpha2.DeschedulerPolicy{
		Profiles: []apiv1alpha2.DeschedulerProfile{
			{
				Name: "RemovePodsViolatingNodeTaintsProfile",
				PluginConfigs: []apiv1alpha2.PluginConfig{
					{
						Name: removepodsviolatingnodetaints.PluginName,
						Args: runtime.RawExtension{
							Object: &removepodsviolatingnodetaints.RemovePodsViolatingNodeTaintsArgs{
								Cordon: cordon,
								Namespaces: &api.Namespaces{
									Include: []string{targetNamespace},
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
						Enabled: []string{defaultevictor.PluginName},
					},
					Deschedule: apiv1alpha2.PluginSet{
						Enabled: []string{removepodsviolatingnodetaints.PluginName},
					},
				},
			},
		},
		GracePeriodSeconds: &gracePeriodSeconds,
	}
}

func TestNodeTaintsCordon(t *testing.T) {
	ctx := context.Background()
	initPluginRegistry()

	clientSet, err := client.CreateClient(componentbaseconfig.ClientConnectionConfiguration{Kubeconfig: os.Getenv("KUBECONFIG")}, "")
	if err != nil {
		t.Fatalf("Error during kubernetes client creation: %v", err)
	}

	nodeList, err := clientSet.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("Error listing nodes: %v", err)
	}
	_, workerNodes := splitNodesAndWorkerNodes(nodeList.Items)
	if len(workerNodes) == 0 {
		t.Skip("Skipping test as there are no worker nodes")
	}
	node := workerNodes[0]

	// Start from a clean state: uncordoned and without the test taint.
	if err := setNodeUnschedulable(ctx, clientSet, node.Name, false); err != nil {
		t.Fatalf("Unable to uncordon node %v: %v", node.Name, err)
	}
	taint := v1.Taint{Key: nodeTaintsE2ETaintKey, Value: "true", Effect: v1.TaintEffectNoSchedule}
	if err := addNodeTaint(ctx, clientSet, node.Name, taint); err != nil {
		t.Fatalf("Unable to taint node %v: %v", node.Name, err)
	}
	defer func() {
		_ = removeNodeTaint(ctx, clientSet, node.Name, taint)
		_ = setNodeUnschedulable(ctx, clientSet, node.Name, false)
	}()

	t.Logf("Creating testing namespace %v", t.Name())
	testNamespace := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "e2e-" + strings.ToLower(t.Name())}}
	if _, err := clientSet.CoreV1().Namespaces().Create(ctx, testNamespace, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Unable to create ns %v", testNamespace.Name)
	}
	defer clientSet.CoreV1().Namespaces().Delete(ctx, testNamespace.Name, metav1.DeleteOptions{})

	// Pin a single pod to the tainted node without a matching toleration. The
	// nodeName pin bypasses scheduling so the pod starts despite the taint, and
	// the descheduler is then expected to cordon the node and evict the pod.
	rc := RcByNameContainer("test-rc-node-taint", testNamespace.Name, 1, map[string]string{"test": "node-taint"}, nil, "")
	rc.Spec.Template.Spec.NodeName = node.Name
	if _, err := clientSet.CoreV1().ReplicationControllers(rc.Namespace).Create(ctx, rc, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Error creating replication controller %v: %v", rc.Name, err)
	}
	defer deleteRC(ctx, t, clientSet, rc)
	waitForRCPodsRunning(ctx, t, clientSet, rc)

	preRunNames := sets.NewString(getCurrentPodNames(ctx, clientSet, testNamespace.Name, t)...)
	createPolicyConfigMap(t, ctx, clientSet, nodeTaintsPolicy(testNamespace.Name, true, 0))
	deschedulerDeploymentObj := deschedulerDeployment(testNamespace.Name)
	createDeschedulerDeploymentWithCleanup(t, ctx, clientSet, deschedulerDeploymentObj)

	// Wait for the descheduler to evict the pod.
	if err := wait.PollUntilContextTimeout(ctx, 1*time.Second, 60*time.Second, true, func(ctx context.Context) (bool, error) {
		currentRunNames := sets.NewString(getCurrentPodNames(ctx, clientSet, testNamespace.Name, t)...)
		actualEvictedPods := preRunNames.Difference(currentRunNames)
		if actualEvictedPods.Len() < 1 {
			t.Logf("Waiting for at least one pod to be evicted, got %v", actualEvictedPods.List())
			return false, nil
		}
		return true, nil
	}); err != nil {
		t.Fatalf("Error waiting for pod eviction: %v", err)
	}
	waitForTerminatingPodsToDisappear(ctx, t, clientSet, testNamespace.Name)

	// The plugin must have cordoned the node before evicting the pod.
	updatedNode, err := clientSet.CoreV1().Nodes().Get(ctx, node.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Unable to get node %v: %v", node.Name, err)
	}
	if !updatedNode.Spec.Unschedulable {
		t.Fatalf("Expected node %v to be cordoned", node.Name)
	}
}

func setNodeUnschedulable(ctx context.Context, clientSet clientset.Interface, nodeName string, unschedulable bool) error {
	return retryOnConflict(func() error {
		node, err := clientSet.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if node.Spec.Unschedulable == unschedulable {
			return nil
		}
		node.Spec.Unschedulable = unschedulable
		_, err = clientSet.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
		return err
	})
}

func addNodeTaint(ctx context.Context, clientSet clientset.Interface, nodeName string, taint v1.Taint) error {
	return retryOnConflict(func() error {
		node, err := clientSet.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		for _, t := range node.Spec.Taints {
			if t.MatchTaint(&taint) {
				return nil
			}
		}
		node.Spec.Taints = append(node.Spec.Taints, taint)
		_, err = clientSet.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
		return err
	})
}

func removeNodeTaint(ctx context.Context, clientSet clientset.Interface, nodeName string, taint v1.Taint) error {
	return retryOnConflict(func() error {
		node, err := clientSet.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		var taints []v1.Taint
		for _, t := range node.Spec.Taints {
			if !t.MatchTaint(&taint) {
				taints = append(taints, t)
			}
		}
		node.Spec.Taints = taints
		_, err = clientSet.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
		return err
	})
}

func retryOnConflict(fn func() error) error {
	var err error
	for range 5 {
		err = fn()
		if err == nil || !apierrors.IsConflict(err) {
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}
	return err
}
