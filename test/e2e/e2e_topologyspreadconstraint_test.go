package e2e

import (
	"context"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	componentbaseconfig "k8s.io/component-base/config"

	"sigs.k8s.io/descheduler/pkg/api"
	apiv1alpha2 "sigs.k8s.io/descheduler/pkg/api/v1alpha2"
	"sigs.k8s.io/descheduler/pkg/descheduler/client"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodsviolatingtopologyspreadconstraint"
)

const zoneTopologyKey string = "topology.kubernetes.io/zone"

func topologySpreadConstraintPolicy(constraintArgs *removepodsviolatingtopologyspreadconstraint.RemovePodsViolatingTopologySpreadConstraintArgs,
	evictorArgs *defaultevictor.DefaultEvictorArgs,
) *apiv1alpha2.DeschedulerPolicy {
	return &apiv1alpha2.DeschedulerPolicy{
		Profiles: []apiv1alpha2.DeschedulerProfile{
			{
				Name: removepodsviolatingtopologyspreadconstraint.PluginName + "Profile",
				PluginConfigs: []apiv1alpha2.PluginConfig{
					{
						Name: removepodsviolatingtopologyspreadconstraint.PluginName,
						Args: runtime.RawExtension{
							Object: constraintArgs,
						},
					},
					{
						Name: defaultevictor.PluginName,
						Args: runtime.RawExtension{
							Object: evictorArgs,
						},
					},
				},
				Plugins: apiv1alpha2.Plugins{
					Filter: apiv1alpha2.PluginSet{
						Enabled: []string{
							defaultevictor.PluginName,
						},
					},
					Balance: apiv1alpha2.PluginSet{
						Enabled: []string{
							removepodsviolatingtopologyspreadconstraint.PluginName,
						},
					},
				},
			},
		},
	}
}

func TestTopologySpreadConstraint(t *testing.T) {
	ctx := context.Background()

	clientSet, err := client.CreateClient(componentbaseconfig.ClientConnectionConfiguration{Kubeconfig: os.Getenv("KUBECONFIG")}, "")
	if err != nil {
		t.Errorf("Error during kubernetes client creation with %v", err)
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

	testCases := []struct {
		name                     string
		expectedEvictedPodCount  int
		replicaCount             int
		topologySpreadConstraint v1.TopologySpreadConstraint
	}{
		{
			name:                    "test-topology-spread-hard-constraint",
			expectedEvictedPodCount: 1,
			replicaCount:            4,
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
		{
			name:                    "test-topology-spread-soft-constraint",
			expectedEvictedPodCount: 1,
			replicaCount:            4,
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
		{
			name:                    "test-node-taints-policy-honor",
			expectedEvictedPodCount: 1,
			replicaCount:            4,
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
		{
			name:                    "test-node-affinity-policy-ignore",
			expectedEvictedPodCount: 1,
			replicaCount:            4,
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
		{
			name:                    "test-match-label-keys",
			expectedEvictedPodCount: 0,
			replicaCount:            4,
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
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Creating Deployment %s with %d replicas", tc.name, tc.replicaCount)
			deployLabels := tc.topologySpreadConstraint.LabelSelector.DeepCopy().MatchLabels
			deployLabels["name"] = tc.name
			deployment := buildTestDeployment(tc.name, testNamespace.Name, int32(tc.replicaCount), deployLabels, func(d *appsv1.Deployment) {
				d.Spec.Template.Spec.TopologySpreadConstraints = []v1.TopologySpreadConstraint{tc.topologySpreadConstraint}
			})
			if _, err := clientSet.AppsV1().Deployments(deployment.Namespace).Create(ctx, deployment, metav1.CreateOptions{}); err != nil {
				t.Fatalf("Error creating Deployment %s %v", tc.name, err)
			}
			defer func() {
				clientSet.AppsV1().Deployments(deployment.Namespace).Delete(ctx, deployment.Name, metav1.DeleteOptions{})
				waitForPodsToDisappear(ctx, t, clientSet, deployment.Labels, deployment.Namespace)
			}()
			waitForPodsRunning(ctx, t, clientSet, deployment.Labels, tc.replicaCount, deployment.Namespace)

			// Create a "Violator" Deployment that has the same label and is forced to be on the same node using a nodeSelector
			violatorDeploymentName := tc.name + "-violator"
			violatorDeployLabels := tc.topologySpreadConstraint.LabelSelector.DeepCopy().MatchLabels
			violatorDeployLabels["name"] = violatorDeploymentName
			violatorDeployment := buildTestDeployment(violatorDeploymentName, testNamespace.Name, tc.topologySpreadConstraint.MaxSkew+1, violatorDeployLabels, func(d *appsv1.Deployment) {
				d.Spec.Template.Spec.NodeSelector = map[string]string{zoneTopologyKey: workerNodes[0].Labels[zoneTopologyKey]}
			})
			if _, err := clientSet.AppsV1().Deployments(violatorDeployment.Namespace).Create(ctx, violatorDeployment, metav1.CreateOptions{}); err != nil {
				t.Fatalf("Error creating Deployment %s: %v", violatorDeployment.Name, err)
			}
			defer func() {
				clientSet.AppsV1().Deployments(violatorDeployment.Namespace).Delete(ctx, violatorDeployment.Name, metav1.DeleteOptions{})
				waitForPodsToDisappear(ctx, t, clientSet, violatorDeployment.Labels, violatorDeployment.Namespace)
			}()
			waitForPodsRunning(ctx, t, clientSet, violatorDeployment.Labels, int(*violatorDeployment.Spec.Replicas), violatorDeployment.Namespace)

			// Run TopologySpreadConstraint strategy
			t.Logf("Running RemovePodsViolatingTopologySpreadConstraint strategy for %s", tc.name)

			preRunNames := sets.NewString(getCurrentPodNames(ctx, clientSet, testNamespace.Name, t)...)

			evictorArgs := &defaultevictor.DefaultEvictorArgs{
				EvictLocalStoragePods:   true,
				EvictSystemCriticalPods: false,
				IgnorePvcPods:           false,
				EvictFailedBarePods:     false,
			}
			constraintArgs := &removepodsviolatingtopologyspreadconstraint.RemovePodsViolatingTopologySpreadConstraintArgs{
				Constraints: []v1.UnsatisfiableConstraintAction{tc.topologySpreadConstraint.WhenUnsatisfiable},
				Namespaces: &api.Namespaces{
					Include: []string{testNamespace.Name},
				},
			}
			deschedulerPolicyConfigMapObj, err := deschedulerPolicyConfigMap(topologySpreadConstraintPolicy(constraintArgs, evictorArgs))
			if err != nil {
				t.Fatalf("Error creating %q CM: %v", deschedulerPolicyConfigMapObj.Name, err)
			}

			t.Logf("Creating %q policy CM with RemovePodsHavingTooManyRestarts configured...", deschedulerPolicyConfigMapObj.Name)
			_, err = clientSet.CoreV1().ConfigMaps(deschedulerPolicyConfigMapObj.Namespace).Create(ctx, deschedulerPolicyConfigMapObj, metav1.CreateOptions{})
			if err != nil {
				t.Fatalf("Error creating %q CM: %v", deschedulerPolicyConfigMapObj.Name, err)
			}

			defer func() {
				t.Logf("Deleting %q CM...", deschedulerPolicyConfigMapObj.Name)
				err = clientSet.CoreV1().ConfigMaps(deschedulerPolicyConfigMapObj.Namespace).Delete(ctx, deschedulerPolicyConfigMapObj.Name, metav1.DeleteOptions{})
				if err != nil {
					t.Fatalf("Unable to delete %q CM: %v", deschedulerPolicyConfigMapObj.Name, err)
				}
			}()
			deschedulerDeploymentObj := deschedulerDeployment(testNamespace.Name)
			t.Logf("Creating descheduler deployment %v", deschedulerDeploymentObj.Name)
			_, err = clientSet.AppsV1().Deployments(deschedulerDeploymentObj.Namespace).Create(ctx, deschedulerDeploymentObj, metav1.CreateOptions{})
			if err != nil {
				t.Fatalf("Error creating %q deployment: %v", deschedulerDeploymentObj.Name, err)
			}

			deschedulerPodName := ""
			defer func() {
				if deschedulerPodName != "" {
					printPodLogs(ctx, t, clientSet, deschedulerPodName)
				}

				t.Logf("Deleting %q deployment...", deschedulerDeploymentObj.Name)
				err = clientSet.AppsV1().Deployments(deschedulerDeploymentObj.Namespace).Delete(ctx, deschedulerDeploymentObj.Name, metav1.DeleteOptions{})
				if err != nil {
					t.Fatalf("Unable to delete %q deployment: %v", deschedulerDeploymentObj.Name, err)
				}
				waitForPodsToDisappear(ctx, t, clientSet, deschedulerDeploymentObj.Labels, deschedulerDeploymentObj.Namespace)
			}()

			t.Logf("Waiting for the descheduler pod running")
			deschedulerPods := waitForPodsRunning(ctx, t, clientSet, deschedulerDeploymentObj.Labels, 1, deschedulerDeploymentObj.Namespace)
			if len(deschedulerPods) != 0 {
				deschedulerPodName = deschedulerPods[0].Name
			}

			// Run RemovePodsHavingTooManyRestarts strategy
			var meetsEvictedExpectations bool
			var actualEvictedPodCount int
			t.Logf("Check whether the number of evicted pods meets the expectation")
			if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 60*time.Second, true, func(ctx context.Context) (bool, error) {
				currentRunNames := sets.NewString(getCurrentPodNames(ctx, clientSet, testNamespace.Name, t)...)
				actualEvictedPod := preRunNames.Difference(currentRunNames)
				actualEvictedPodCount = actualEvictedPod.Len()
				t.Logf("preRunNames: %v, currentRunNames: %v, actualEvictedPodCount: %v\n", preRunNames.List(), currentRunNames.List(), actualEvictedPodCount)
				if actualEvictedPodCount != tc.expectedEvictedPodCount {
					t.Logf("Expecting %v number of pods evicted, got %v instead", tc.expectedEvictedPodCount, actualEvictedPodCount)
					return false, nil
				}
				meetsEvictedExpectations = true
				return true, nil
			}); err != nil {
				t.Errorf("Error waiting for descheduler running: %v", err)
			}

			if !meetsEvictedExpectations {
				t.Errorf("Unexpected number of pods have been evicted, got %v, expected %v", actualEvictedPodCount, tc.expectedEvictedPodCount)
			} else {
				t.Logf("Total of %d Pods were evicted for %s", actualEvictedPodCount, tc.name)
			}

			if tc.expectedEvictedPodCount == 0 {
				return
			}

			var meetsSkewExpectations bool
			var skewVal int
			t.Logf("Check whether the skew meets the expectation")
			if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 60*time.Second, true, func(ctx context.Context) (bool, error) {
				listOptions := metav1.ListOptions{LabelSelector: labels.SelectorFromSet(tc.topologySpreadConstraint.LabelSelector.MatchLabels).String()}
				pods, err := clientSet.CoreV1().Pods(testNamespace.Name).List(ctx, listOptions)
				if err != nil {
					t.Errorf("Error listing pods for %s: %v", tc.name, err)
				}

				nodePodCountMap := make(map[string]int)
				for _, pod := range pods.Items {
					nodePodCountMap[pod.Spec.NodeName]++
				}

				if len(nodePodCountMap) != len(workerNodes) {
					t.Errorf("%s Pods were scheduled on only '%d' nodes and were not properly distributed on the nodes", tc.name, len(nodePodCountMap))
					return false, nil
				}

				skewVal = getSkewValPodDistribution(nodePodCountMap)
				if skewVal > int(tc.topologySpreadConstraint.MaxSkew) {
					t.Errorf("Pod distribution for %s is still violating the max skew of %d as it is %d", tc.name, tc.topologySpreadConstraint.MaxSkew, skewVal)
					return false, nil
				}

				meetsSkewExpectations = true
				return true, nil
			}); err != nil {
				t.Errorf("Error waiting for descheduler running: %v", err)
			}

			if !meetsSkewExpectations {
				t.Errorf("Pod distribution for %s is still violating the max skew of %d as it is %d", tc.name, tc.topologySpreadConstraint.MaxSkew, skewVal)
			} else {
				t.Logf("Pods for %s were distributed in line with max skew of %d", tc.name, tc.topologySpreadConstraint.MaxSkew)
			}
		})
	}
}

func getSkewValPodDistribution(nodePodCountMap map[string]int) int {
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

	return max - min
}

func nodeInclusionPolicyRef(policy v1.NodeInclusionPolicy) *v1.NodeInclusionPolicy {
	return &policy
}
