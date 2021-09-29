package e2e

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	deschedulerapi "sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/strategies"
)

const zoneTopologyKey string = "topology.kubernetes.io/zone"

func TestTopologySpreadConstraint(t *testing.T) {
	ctx := context.Background()
	clientSet, _, stopCh := initializeClient(t)
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
		replicaCount int
		maxSkew      int
		labelKey     string
		labelValue   string
		constraint   v1.UnsatisfiableConstraintAction
	}{
		"test-rc-topology-spread-hard-constraint": {
			replicaCount: 4,
			maxSkew:      1,
			labelKey:     "test",
			labelValue:   "topology-spread-hard-constraint",
			constraint:   v1.DoNotSchedule,
		},
		"test-rc-topology-spread-soft-constraint": {
			replicaCount: 4,
			maxSkew:      1,
			labelKey:     "test",
			labelValue:   "topology-spread-soft-constraint",
			constraint:   v1.ScheduleAnyway,
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Logf("Creating RC %s with %d replicas", name, tc.replicaCount)
			rc := RcByNameContainer(name, testNamespace.Name, int32(tc.replicaCount), map[string]string{tc.labelKey: tc.labelValue}, nil, "")
			rc.Spec.Template.Spec.TopologySpreadConstraints = makeTopologySpreadConstraints(tc.maxSkew, tc.labelKey, tc.labelValue, tc.constraint)
			if _, err := clientSet.CoreV1().ReplicationControllers(rc.Namespace).Create(ctx, rc, metav1.CreateOptions{}); err != nil {
				t.Fatalf("Error creating RC %s %v", name, err)
			}
			defer deleteRC(ctx, t, clientSet, rc)
			waitForRCPodsRunning(ctx, t, clientSet, rc)

			// Create a "Violator" RC that has the same label and is forced to be on the same node using a nodeSelector
			violatorRcName := name + "-violator"
			violatorCount := tc.maxSkew + 1
			violatorRc := RcByNameContainer(violatorRcName, testNamespace.Name, int32(violatorCount), map[string]string{tc.labelKey: tc.labelValue}, nil, "")
			violatorRc.Spec.Template.Spec.NodeSelector = map[string]string{zoneTopologyKey: workerNodes[0].Labels[zoneTopologyKey]}
			rc.Spec.Template.Spec.TopologySpreadConstraints = makeTopologySpreadConstraints(tc.maxSkew, tc.labelKey, tc.labelValue, tc.constraint)
			if _, err := clientSet.CoreV1().ReplicationControllers(rc.Namespace).Create(ctx, violatorRc, metav1.CreateOptions{}); err != nil {
				t.Fatalf("Error creating RC %s: %v", violatorRcName, err)
			}
			defer deleteRC(ctx, t, clientSet, violatorRc)
			waitForRCPodsRunning(ctx, t, clientSet, violatorRc)

			podEvictor := initPodEvictorOrFail(t, clientSet, nodes)

			// Run TopologySpreadConstraint strategy
			t.Logf("Running RemovePodsViolatingTopologySpreadConstraint strategy for %s", name)
			strategies.RemovePodsViolatingTopologySpreadConstraint(
				ctx,
				clientSet,
				deschedulerapi.DeschedulerStrategy{
					Enabled: true,
					Params: &deschedulerapi.StrategyParameters{
						IncludeSoftConstraints: tc.constraint != v1.DoNotSchedule,
					},
				},
				nodes,
				podEvictor,
			)
			t.Logf("Finished RemovePodsViolatingTopologySpreadConstraint strategy for %s", name)

			t.Logf("Wait for terminating pods of %s to disappear", name)
			waitForTerminatingPodsToDisappear(ctx, t, clientSet, rc.Namespace)

			if totalEvicted := podEvictor.TotalEvicted(); totalEvicted > 0 {
				t.Logf("Total of %d Pods were evicted for %s", totalEvicted, name)
			} else {
				t.Fatalf("Pods were not evicted for %s TopologySpreadConstraint", name)
			}

			// Ensure recently evicted Pod are rescheduled and running before asserting for a balanced topology spread
			waitForRCPodsRunning(ctx, t, clientSet, rc)

			pods, err := clientSet.CoreV1().Pods(testNamespace.Name).List(ctx, metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s", tc.labelKey, tc.labelValue)})
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
			if max-min > tc.maxSkew {
				t.Errorf("Pod distribution for %s is still violating the max skew of %d as it is %d", name, tc.maxSkew, max-min)
			}

			t.Logf("Pods for %s were distributed in line with max skew of %d", name, tc.maxSkew)
		})
	}
}

func makeTopologySpreadConstraints(maxSkew int, labelKey, labelValue string, constraint v1.UnsatisfiableConstraintAction) []v1.TopologySpreadConstraint {
	return []v1.TopologySpreadConstraint{
		{
			MaxSkew:           int32(maxSkew),
			TopologyKey:       zoneTopologyKey,
			WhenUnsatisfiable: constraint,
			LabelSelector:     &metav1.LabelSelector{MatchLabels: map[string]string{labelKey: labelValue}},
		},
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
