package e2e

import (
	"context"
	"math"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	frameworkfake "sigs.k8s.io/descheduler/pkg/framework/fake"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodsviolatingtopologyspreadconstraint"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
)

const zoneTopologyKey string = "topology.kubernetes.io/zone"

func TestTopologySpreadConstraint(t *testing.T) {
	ctx := context.Background()
	clientSet, _, _, getPodsAssignedToNode, stopCh := initializeClient(t)
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

	testCases := map[string]struct {
		expectedEvictedCount     uint
		replicaCount             int
		topologySpreadConstraint v1.TopologySpreadConstraint
	}{
		"test-rc-topology-spread-hard-constraint": {
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
		"test-rc-topology-spread-soft-constraint": {
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
		"test-rc-node-taints-policy-honor": {
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
		"test-rc-node-affinity-policy-ignore": {
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
		"test-rc-match-label-keys": {
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
			t.Logf("Creating RC %s with %d replicas", name, tc.replicaCount)
			rc := RcByNameContainer(name, testNamespace.Name, int32(tc.replicaCount), tc.topologySpreadConstraint.LabelSelector.DeepCopy().MatchLabels, nil, "")
			rc.Spec.Template.Spec.TopologySpreadConstraints = []v1.TopologySpreadConstraint{tc.topologySpreadConstraint}
			if _, err := clientSet.CoreV1().ReplicationControllers(rc.Namespace).Create(ctx, rc, metav1.CreateOptions{}); err != nil {
				t.Fatalf("Error creating RC %s %v", name, err)
			}
			defer deleteRC(ctx, t, clientSet, rc)
			waitForRCPodsRunning(ctx, t, clientSet, rc)

			// Create a "Violator" RC that has the same label and is forced to be on the same node using a nodeSelector
			violatorRcName := name + "-violator"
			violatorCount := tc.topologySpreadConstraint.MaxSkew + 1
			violatorRc := RcByNameContainer(violatorRcName, testNamespace.Name, violatorCount, tc.topologySpreadConstraint.LabelSelector.DeepCopy().MatchLabels, nil, "")
			violatorRc.Spec.Template.Spec.NodeSelector = map[string]string{zoneTopologyKey: workerNodes[0].Labels[zoneTopologyKey]}
			rc.Spec.Template.Spec.TopologySpreadConstraints = []v1.TopologySpreadConstraint{tc.topologySpreadConstraint}
			if _, err := clientSet.CoreV1().ReplicationControllers(rc.Namespace).Create(ctx, violatorRc, metav1.CreateOptions{}); err != nil {
				t.Fatalf("Error creating RC %s: %v", violatorRcName, err)
			}
			defer deleteRC(ctx, t, clientSet, violatorRc)
			waitForRCPodsRunning(ctx, t, clientSet, violatorRc)

			podEvictor := initPodEvictorOrFail(t, clientSet, getPodsAssignedToNode, nodes)

			// Run TopologySpreadConstraint strategy
			t.Logf("Running RemovePodsViolatingTopologySpreadConstraint strategy for %s", name)

			defaultevictorArgs := &defaultevictor.DefaultEvictorArgs{
				EvictLocalStoragePods:   true,
				EvictSystemCriticalPods: false,
				IgnorePvcPods:           false,
				EvictFailedBarePods:     false,
			}

			filter, err := defaultevictor.New(
				defaultevictorArgs,
				&frameworkfake.HandleImpl{
					ClientsetImpl:                 clientSet,
					GetPodsAssignedToNodeFuncImpl: getPodsAssignedToNode,
				},
			)
			if err != nil {
				t.Fatalf("Unable to initialize the plugin: %v", err)
			}

			plugin, err := removepodsviolatingtopologyspreadconstraint.New(&removepodsviolatingtopologyspreadconstraint.RemovePodsViolatingTopologySpreadConstraintArgs{
				Constraints: []v1.UnsatisfiableConstraintAction{tc.topologySpreadConstraint.WhenUnsatisfiable},
			},
				&frameworkfake.HandleImpl{
					ClientsetImpl:                 clientSet,
					PodEvictorImpl:                podEvictor,
					EvictorFilterImpl:             filter.(frameworktypes.EvictorPlugin),
					GetPodsAssignedToNodeFuncImpl: getPodsAssignedToNode,
				},
			)
			if err != nil {
				t.Fatalf("Unable to initialize the plugin: %v", err)
			}

			plugin.(frameworktypes.BalancePlugin).Balance(ctx, workerNodes)
			t.Logf("Finished RemovePodsViolatingTopologySpreadConstraint strategy for %s", name)

			t.Logf("Wait for terminating pods of %s to disappear", name)
			waitForTerminatingPodsToDisappear(ctx, t, clientSet, rc.Namespace)

			if totalEvicted := podEvictor.TotalEvicted(); totalEvicted == tc.expectedEvictedCount {
				t.Logf("Total of %d Pods were evicted for %s", totalEvicted, name)
			} else {
				t.Fatalf("Expected %d evictions but got %d for %s TopologySpreadConstraint", tc.expectedEvictedCount, totalEvicted, name)
			}

			if tc.expectedEvictedCount == 0 {
				return
			}

			// Ensure recently evicted Pod are rescheduled and running before asserting for a balanced topology spread
			waitForRCPodsRunning(ctx, t, clientSet, rc)

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
