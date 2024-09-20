package e2e

import (
	"context"
	"math"
	"os"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	componentbaseconfig "k8s.io/component-base/config"

	"sigs.k8s.io/descheduler/pkg/descheduler/client"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	eutils "sigs.k8s.io/descheduler/pkg/descheduler/evictions/utils"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodsviolatingtopologyspreadconstraint"
	frameworktesting "sigs.k8s.io/descheduler/pkg/framework/testing"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
)

const zoneTopologyKey string = "topology.kubernetes.io/zone"

func TestTopologySpreadConstraint(t *testing.T) {
	ctx := context.Background()
	clientSet, err := client.CreateClient(componentbaseconfig.ClientConnectionConfiguration{Kubeconfig: os.Getenv("KUBECONFIG")}, "")
	if err != nil {
		t.Errorf("Error during client creation with %v", err)
	}

	nodeList, err := clientSet.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Errorf("Error listing node with %v", err)
	}
	_, workerNodes := splitNodesAndWorkerNodes(nodeList.Items)
	t.Log("Creating testing namespace")
	testNamespace := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "e2e-" + strings.ToLower(t.Name())}}
	if _, err := clientSet.CoreV1().Namespaces().Create(ctx, testNamespace, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Unable to create ns %v", testNamespace.Name)
	}
	defer clientSet.CoreV1().Namespaces().Delete(ctx, testNamespace.Name, metav1.DeleteOptions{})

	testCases := map[string]struct {
		expectedEvictedCount     uint
		replicaCount             int
		topologySpreadConstraint v1.TopologySpreadConstraint
	}{
		"test-topology-spread-hard-constraint": {
			expectedEvictedCount: 1,
			replicaCount:         4,
			topologySpreadConstraint: v1.TopologySpreadConstraint{
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"test": "topology-spread-hard-constraint",
					},
				},
				MaxSkew:           1,
				TopologyKey:       zoneTopologyKey,
				WhenUnsatisfiable: v1.DoNotSchedule,
			},
		},
		"test-topology-spread-soft-constraint": {
			expectedEvictedCount: 1,
			replicaCount:         4,
			topologySpreadConstraint: v1.TopologySpreadConstraint{
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"test": "topology-spread-soft-constraint",
					},
				},
				MaxSkew:           1,
				TopologyKey:       zoneTopologyKey,
				WhenUnsatisfiable: v1.ScheduleAnyway,
			},
		},
		"test-node-taints-policy-honor": {
			expectedEvictedCount: 1,
			replicaCount:         4,
			topologySpreadConstraint: v1.TopologySpreadConstraint{
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"test": "node-taints-policy-honor",
					},
				},
				MaxSkew:           1,
				NodeTaintsPolicy:  nodeInclusionPolicyRef(v1.NodeInclusionPolicyHonor),
				TopologyKey:       zoneTopologyKey,
				WhenUnsatisfiable: v1.DoNotSchedule,
			},
		},
		"test-node-affinity-policy-ignore": {
			expectedEvictedCount: 1,
			replicaCount:         4,
			topologySpreadConstraint: v1.TopologySpreadConstraint{
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"test": "node-taints-policy-honor",
					},
				},
				MaxSkew:            1,
				NodeAffinityPolicy: nodeInclusionPolicyRef(v1.NodeInclusionPolicyIgnore),
				TopologyKey:        zoneTopologyKey,
				WhenUnsatisfiable:  v1.DoNotSchedule,
			},
		},
		"test-match-label-keys": {
			expectedEvictedCount: 0,
			replicaCount:         4,
			topologySpreadConstraint: v1.TopologySpreadConstraint{
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"test": "match-label-keys",
					},
				},
				MatchLabelKeys:    []string{appsv1.DefaultDeploymentUniqueLabelKey},
				MaxSkew:           1,
				TopologyKey:       zoneTopologyKey,
				WhenUnsatisfiable: v1.DoNotSchedule,
			},
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Logf("Creating Deployment %s with %d replicas", name, tc.replicaCount)
			deployLabels := tc.topologySpreadConstraint.LabelSelector.DeepCopy().MatchLabels
			deployLabels["name"] = name
			deployment := buildTestDeployment(name, testNamespace.Name, int32(tc.replicaCount), deployLabels, func(d *appsv1.Deployment) {
				d.Spec.Template.Spec.TopologySpreadConstraints = []v1.TopologySpreadConstraint{tc.topologySpreadConstraint}
			})
			if _, err := clientSet.AppsV1().Deployments(deployment.Namespace).Create(ctx, deployment, metav1.CreateOptions{}); err != nil {
				t.Fatalf("Error creating Deployment %s %v", name, err)
			}
			defer func() {
				clientSet.AppsV1().Deployments(deployment.Namespace).Delete(ctx, deployment.Name, metav1.DeleteOptions{})
				waitForPodsToDisappear(ctx, t, clientSet, deployment.Labels, deployment.Namespace)
			}()
			waitForPodsRunning(ctx, t, clientSet, deployment.Labels, tc.replicaCount, deployment.Namespace)

			// Create a "Violator" Deployment that has the same label and is forced to be on the same node using a nodeSelector
			violatorDeploymentName := name + "-violator"
			violatorCount := tc.topologySpreadConstraint.MaxSkew + 1
			violatorDeployLabels := tc.topologySpreadConstraint.LabelSelector.DeepCopy().MatchLabels
			violatorDeployLabels["name"] = violatorDeploymentName
			violatorDeployment := buildTestDeployment(violatorDeploymentName, testNamespace.Name, violatorCount, violatorDeployLabels, func(d *appsv1.Deployment) {
				d.Spec.Template.Spec.NodeSelector = map[string]string{zoneTopologyKey: workerNodes[0].Labels[zoneTopologyKey]}
			})
			if _, err := clientSet.AppsV1().Deployments(deployment.Namespace).Create(ctx, violatorDeployment, metav1.CreateOptions{}); err != nil {
				t.Fatalf("Error creating Deployment %s: %v", violatorDeploymentName, err)
			}
			defer func() {
				clientSet.AppsV1().Deployments(violatorDeployment.Namespace).Delete(ctx, violatorDeployment.Name, metav1.DeleteOptions{})
				waitForPodsToDisappear(ctx, t, clientSet, violatorDeployment.Labels, violatorDeployment.Namespace)
			}()
			waitForPodsRunning(ctx, t, clientSet, violatorDeployment.Labels, int(violatorCount), violatorDeployment.Namespace)

			evictionPolicyGroupVersion, err := eutils.SupportEviction(clientSet)
			if err != nil || len(evictionPolicyGroupVersion) == 0 {
				t.Fatalf("Error detecting eviction policy group: %v", err)
			}

			handle, podEvictor, err := frameworktesting.InitFrameworkHandle(
				ctx,
				clientSet,
				evictions.NewOptions().
					WithPolicyGroupVersion(evictionPolicyGroupVersion),
				defaultevictor.DefaultEvictorArgs{
					EvictLocalStoragePods: true,
				},
				nil,
			)
			if err != nil {
				t.Fatalf("Unable to initialize a framework handle: %v", err)
			}

			// Run TopologySpreadConstraint strategy
			t.Logf("Running RemovePodsViolatingTopologySpreadConstraint strategy for %s", name)

			plugin, err := removepodsviolatingtopologyspreadconstraint.New(&removepodsviolatingtopologyspreadconstraint.RemovePodsViolatingTopologySpreadConstraintArgs{
				Constraints: []v1.UnsatisfiableConstraintAction{tc.topologySpreadConstraint.WhenUnsatisfiable},
			},
				handle,
			)
			if err != nil {
				t.Fatalf("Unable to initialize the plugin: %v", err)
			}

			plugin.(frameworktypes.BalancePlugin).Balance(ctx, workerNodes)
			t.Logf("Finished RemovePodsViolatingTopologySpreadConstraint strategy for %s", name)

			t.Logf("Wait for terminating pods of %s to disappear", name)
			waitForTerminatingPodsToDisappear(ctx, t, clientSet, deployment.Namespace)

			if totalEvicted := podEvictor.TotalEvicted(); totalEvicted == tc.expectedEvictedCount {
				t.Logf("Total of %d Pods were evicted for %s", totalEvicted, name)
			} else {
				t.Fatalf("Expected %d evictions but got %d for %s TopologySpreadConstraint", tc.expectedEvictedCount, totalEvicted, name)
			}

			if tc.expectedEvictedCount == 0 {
				return
			}

			// Ensure recently evicted Pod are rescheduled and running before asserting for a balanced topology spread
			waitForPodsRunning(ctx, t, clientSet, deployment.Labels, tc.replicaCount, deployment.Namespace)

			listOptions := metav1.ListOptions{LabelSelector: labels.SelectorFromSet(tc.topologySpreadConstraint.LabelSelector.MatchLabels).String()}
			pods, err := clientSet.CoreV1().Pods(testNamespace.Name).List(ctx, listOptions)
			if err != nil {
				t.Errorf("Error listing pods for %s: %v", name, err)
			}

			nodePodCountMap := make(map[string]int)
			for _, pod := range pods.Items {
				nodePodCountMap[pod.Spec.NodeName]++
			}

			if len(nodePodCountMap) != len(workerNodes) {
				t.Errorf("%s Pods were scheduled on only '%d' nodes and were not properly distributed on the nodes", name, len(nodePodCountMap))
			}

			min, max := getMinAndMaxPodDistribution(nodePodCountMap)
			if max-min > int(tc.topologySpreadConstraint.MaxSkew) {
				t.Errorf("Pod distribution for %s is still violating the max skew of %d as it is %d", name, tc.topologySpreadConstraint.MaxSkew, max-min)
			}

			t.Logf("Pods for %s were distributed in line with max skew of %d", name, tc.topologySpreadConstraint.MaxSkew)
		})
	}
}

func getMinAndMaxPodDistribution(nodePodCountMap map[string]int) (int, int) {
	min := math.MaxInt32
	max := math.MinInt32
	for _, podCount := range nodePodCountMap {
		if podCount < min {
			min = podCount
		}
		if podCount > max {
			max = podCount
		}
	}

	return min, max
}

func nodeInclusionPolicyRef(policy v1.NodeInclusionPolicy) *v1.NodeInclusionPolicy {
	return &policy
}
