package strategies

import (
	"context"
	"fmt"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/test"
)

func TestTopologySpreadConstraint(t *testing.T) {
	testCases := []struct {
		name                 string
		pods                 []*v1.Pod
		expectedEvictedCount uint
		nodes                []*v1.Node
		strategy             api.DeschedulerStrategy
		namespaces           []string
	}{
		{
			name: "2 domains, sizes [2,1], maxSkew=1, move 0 pods",
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneA" }),
				test.BuildTestNode("n2", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneB" }),
			},
			pods: createTestPods([]testPodList{
				{
					count:  1,
					node:   "n1",
					labels: map[string]string{"foo": "bar"},
					constraints: []v1.TopologySpreadConstraint{
						{
							MaxSkew:           1,
							TopologyKey:       "zone",
							WhenUnsatisfiable: v1.DoNotSchedule,
							LabelSelector:     &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
						},
					},
				},
				{
					count:  1,
					node:   "n1",
					labels: map[string]string{"foo": "bar"},
				},
				{
					count:  1,
					node:   "n2",
					labels: map[string]string{"foo": "bar"},
				},
			}),
			expectedEvictedCount: 0,
			strategy: api.DeschedulerStrategy{
				Params: &api.StrategyParameters{
					NodeFit: false,
				},
			},
			namespaces: []string{"ns1"},
		},
		{
			name: "2 domains, sizes [3,1], maxSkew=1, move 1 pod to achieve [2,2]",
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneA" }),
				test.BuildTestNode("n2", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneB" }),
			},
			pods: createTestPods([]testPodList{
				{
					count:       1,
					node:        "n1",
					labels:      map[string]string{"foo": "bar"},
					constraints: getDefaultTopologyConstraints(1),
				},
				{
					count:  2,
					node:   "n1",
					labels: map[string]string{"foo": "bar"},
				},
				{
					count:  1,
					node:   "n2",
					labels: map[string]string{"foo": "bar"},
				},
			}),
			expectedEvictedCount: 1,
			strategy: api.DeschedulerStrategy{
				Params: &api.StrategyParameters{
					NodeFit: false,
				},
			},
			namespaces: []string{"ns1"},
		},
		{
			name: "2 domains, sizes [3,1], maxSkew=1, move 1 pod to achieve [2,2] (soft constraints)",
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneA" }),
				test.BuildTestNode("n2", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneB" }),
			},
			pods: createTestPods([]testPodList{
				{
					count:  1,
					node:   "n1",
					labels: map[string]string{"foo": "bar"},
					constraints: []v1.TopologySpreadConstraint{
						{
							MaxSkew:           1,
							TopologyKey:       "zone",
							WhenUnsatisfiable: v1.ScheduleAnyway,
							LabelSelector:     &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
						},
					},
				},
				{
					count:  2,
					node:   "n1",
					labels: map[string]string{"foo": "bar"},
				},
				{
					count:  1,
					node:   "n2",
					labels: map[string]string{"foo": "bar"},
				},
			}),
			expectedEvictedCount: 1,
			strategy:             api.DeschedulerStrategy{Params: &api.StrategyParameters{IncludeSoftConstraints: true}},
			namespaces:           []string{"ns1"},
		},
		{
			name: "2 domains, sizes [3,1], maxSkew=1, no pods eligible, move 0 pods",
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneA" }),
				test.BuildTestNode("n2", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneB" }),
			},
			pods: createTestPods([]testPodList{
				{
					count:       1,
					node:        "n1",
					labels:      map[string]string{"foo": "bar"},
					constraints: getDefaultTopologyConstraints(1),
					noOwners:    true,
				},
				{
					count:    2,
					node:     "n1",
					labels:   map[string]string{"foo": "bar"},
					noOwners: true,
				},
				{
					count:    1,
					node:     "n2",
					labels:   map[string]string{"foo": "bar"},
					noOwners: true,
				},
			}),
			expectedEvictedCount: 0,
			strategy: api.DeschedulerStrategy{
				Params: &api.StrategyParameters{
					NodeFit: false,
				},
			},
			namespaces: []string{"ns1"},
		},
		{
			name: "2 domains, sizes [3,1], maxSkew=1, move 1 pod to achieve [2,2], exclude kube-system namespace",
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneA" }),
				test.BuildTestNode("n2", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneB" }),
			},
			pods: createTestPods([]testPodList{
				{
					count:       1,
					node:        "n1",
					labels:      map[string]string{"foo": "bar"},
					constraints: getDefaultTopologyConstraints(1),
				},
				{
					count:  2,
					node:   "n1",
					labels: map[string]string{"foo": "bar"},
				},
				{
					count:  1,
					node:   "n2",
					labels: map[string]string{"foo": "bar"},
				},
			}),
			expectedEvictedCount: 1,
			strategy:             api.DeschedulerStrategy{Enabled: true, Params: &api.StrategyParameters{NodeFit: true, Namespaces: &api.Namespaces{Exclude: []string{"kube-system"}}}},
			namespaces:           []string{"ns1"},
		},
		{
			name: "2 domains, sizes [5,2], maxSkew=1, move 1 pod to achieve [4,3]",
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneA" }),
				test.BuildTestNode("n2", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneB" }),
			},
			pods: createTestPods([]testPodList{
				{
					count:       1,
					node:        "n1",
					labels:      map[string]string{"foo": "bar"},
					constraints: getDefaultTopologyConstraints(1),
				},
				{
					count:  4,
					node:   "n1",
					labels: map[string]string{"foo": "bar"},
				},
				{
					count:  2,
					node:   "n2",
					labels: map[string]string{"foo": "bar"},
				},
			}),
			expectedEvictedCount: 1,
			strategy: api.DeschedulerStrategy{
				Params: &api.StrategyParameters{
					NodeFit: false,
				},
			},
			namespaces: []string{"ns1"},
		},
		{
			name: "2 domains, sizes [4,0], maxSkew=1, move 2 pods to achieve [2,2]",
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneA" }),
				test.BuildTestNode("n2", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneB" }),
			},
			pods: createTestPods([]testPodList{
				{
					count:       1,
					node:        "n1",
					labels:      map[string]string{"foo": "bar"},
					constraints: getDefaultTopologyConstraints(1),
				},
				{
					count:  3,
					node:   "n1",
					labels: map[string]string{"foo": "bar"},
				},
			}),
			expectedEvictedCount: 2,
			strategy: api.DeschedulerStrategy{
				Params: &api.StrategyParameters{
					NodeFit: false,
				},
			},
			namespaces: []string{"ns1"},
		},
		{
			name: "2 domains, sizes [4,0], maxSkew=1, only move 1 pod since pods with nodeSelector and nodeAffinity aren't evicted",
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneA" }),
				test.BuildTestNode("n2", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneB" }),
			},
			pods: createTestPods([]testPodList{
				{
					count:        1,
					node:         "n1",
					labels:       map[string]string{"foo": "bar"},
					constraints:  getDefaultTopologyConstraints(1),
					nodeSelector: map[string]string{"zone": "zoneA"},
				},
				{
					count:        1,
					node:         "n1",
					labels:       map[string]string{"foo": "bar"},
					nodeSelector: map[string]string{"zone": "zoneA"},
				},
				{
					count:  1,
					node:   "n1",
					labels: map[string]string{"foo": "bar"},
					nodeAffinity: &v1.Affinity{NodeAffinity: &v1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{NodeSelectorTerms: []v1.NodeSelectorTerm{
							{MatchExpressions: []v1.NodeSelectorRequirement{{Key: "foo", Values: []string{"bar"}, Operator: v1.NodeSelectorOpIn}}},
						}},
					}},
				},
				{
					count:  1,
					node:   "n1",
					labels: map[string]string{"foo": "bar"},
				},
			}),
			expectedEvictedCount: 1,
			strategy: api.DeschedulerStrategy{
				Params: &api.StrategyParameters{
					NodeFit: true,
				},
			},
			namespaces: []string{"ns1"},
		},
		{
			name: "2 domains, sizes [4,0], maxSkew=1, move 2 pods since selector matches multiple nodes",
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 2000, 3000, 10, func(n *v1.Node) {
					n.Labels["zone"] = "zoneA"
					n.Labels["region"] = "boston"
				}),
				test.BuildTestNode("n2", 2000, 3000, 10, func(n *v1.Node) {
					n.Labels["zone"] = "zoneB"
					n.Labels["region"] = "boston"
				}),
			},
			pods: createTestPods([]testPodList{
				{
					count:        1,
					node:         "n1",
					labels:       map[string]string{"foo": "bar"},
					constraints:  getDefaultTopologyConstraints(1),
					nodeSelector: map[string]string{"region": "boston"},
				},
				{
					count:        1,
					node:         "n1",
					labels:       map[string]string{"foo": "bar"},
					nodeSelector: map[string]string{"region": "boston"},
				},
				{
					count:        1,
					node:         "n1",
					labels:       map[string]string{"foo": "bar"},
					nodeSelector: map[string]string{"region": "boston"},
				},
				{
					count:        1,
					node:         "n1",
					labels:       map[string]string{"foo": "bar"},
					nodeSelector: map[string]string{"region": "boston"},
				},
			}),
			expectedEvictedCount: 2,
			strategy: api.DeschedulerStrategy{
				Params: &api.StrategyParameters{
					NodeFit: false,
				},
			},
			namespaces: []string{"ns1"},
		},
		{
			name: "3 domains, sizes [0, 1, 100], maxSkew=1, move 66 pods to get [34, 33, 34]",
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneA" }),
				test.BuildTestNode("n2", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneB" }),
				test.BuildTestNode("n3", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneC" }),
			},
			pods: createTestPods([]testPodList{
				{
					count:       1,
					node:        "n2",
					labels:      map[string]string{"foo": "bar"},
					constraints: getDefaultTopologyConstraints(1),
				},
				{
					count:  100,
					node:   "n3",
					labels: map[string]string{"foo": "bar"},
				},
			}),
			expectedEvictedCount: 66,
			strategy:             api.DeschedulerStrategy{},
			namespaces:           []string{"ns1"},
		},
		{
			name: "4 domains, sizes [0, 1, 3, 5], should move 3 to get [2, 2, 3, 2]",
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneA" }),
				test.BuildTestNode("n2", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneB" }),
				test.BuildTestNode("n3", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneC" }),
				test.BuildTestNode("n4", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneD" }),
			},
			pods: createTestPods([]testPodList{
				{
					count:       1,
					node:        "n2",
					labels:      map[string]string{"foo": "bar"},
					constraints: getDefaultTopologyConstraints(1),
				},
				{
					count:  3,
					node:   "n3",
					labels: map[string]string{"foo": "bar"},
				},
				{
					count:  5,
					node:   "n4",
					labels: map[string]string{"foo": "bar"},
				},
			}),
			expectedEvictedCount: 3,
			strategy: api.DeschedulerStrategy{
				Params: &api.StrategyParameters{
					NodeFit: false,
				},
			},
			namespaces: []string{"ns1"},
		},
		{
			name: "2 domains size [2 6], maxSkew=2, should move 1 to get [3 5]",
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneA" }),
				test.BuildTestNode("n2", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneB" }),
			},
			pods: createTestPods([]testPodList{
				{
					count:       1,
					node:        "n1",
					labels:      map[string]string{"foo": "bar"},
					constraints: getDefaultTopologyConstraints(2),
				},
				{
					count:  1,
					node:   "n1",
					labels: map[string]string{"foo": "bar"},
				},
				{
					count:  6,
					node:   "n2",
					labels: map[string]string{"foo": "bar"},
				},
			}),
			expectedEvictedCount: 1,
			strategy: api.DeschedulerStrategy{
				Params: &api.StrategyParameters{
					NodeFit: false,
				},
			},
			namespaces: []string{"ns1"},
		},
		{
			name: "2 domains size [2 6], maxSkew=2, can't move any because of node taints",
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 2000, 3000, 10, func(n *v1.Node) {
					n.Labels["zone"] = "zoneA"
					n.Spec.Taints = []v1.Taint{
						{
							Key:    "hardware",
							Value:  "gpu",
							Effect: v1.TaintEffectNoSchedule,
						},
					}
				}),
				test.BuildTestNode("n2", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneB" }),
			},
			pods: createTestPods([]testPodList{
				{
					count:  1,
					node:   "n1",
					labels: map[string]string{"foo": "bar"},
					constraints: []v1.TopologySpreadConstraint{
						{
							MaxSkew:           2,
							TopologyKey:       "zone",
							WhenUnsatisfiable: v1.DoNotSchedule,
							LabelSelector:     &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
						},
					},
				},
				{
					count:  1,
					node:   "n1",
					labels: map[string]string{"foo": "bar"},
				},
				{
					count:  6,
					node:   "n2",
					labels: map[string]string{"foo": "bar"},
				},
			}),
			expectedEvictedCount: 0,
			strategy: api.DeschedulerStrategy{
				Params: &api.StrategyParameters{
					NodeFit: true,
				},
			},
			namespaces: []string{"ns1"},
		},
		{
			name: "2 domains size [2 6], maxSkew=2, can't move any because node1 does not have enough CPU",
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 200, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneA" }),
				test.BuildTestNode("n2", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneB" }),
			},
			pods: createTestPods([]testPodList{
				{
					count:       1,
					node:        "n1",
					labels:      map[string]string{"foo": "bar"},
					constraints: getDefaultTopologyConstraints(2),
				},
				{
					count:  1,
					node:   "n1",
					labels: map[string]string{"foo": "bar"},
				},
				{
					count:  6,
					node:   "n2",
					labels: map[string]string{"foo": "bar"},
				},
			}),
			expectedEvictedCount: 0,
			strategy: api.DeschedulerStrategy{
				Params: &api.StrategyParameters{
					NodeFit: true,
				},
			},
			namespaces: []string{"ns1"},
		},
		{
			// see https://github.com/kubernetes-sigs/descheduler/issues/564
			name: "Multiple constraints (6 nodes/2 zones, 4 pods)",
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 2000, 3000, 10, func(n *v1.Node) {
					n.Labels = map[string]string{"topology.kubernetes.io/zone": "zoneA", "kubernetes.io/hostname": "n1"}
				}),
				test.BuildTestNode("n2", 2000, 3000, 10, func(n *v1.Node) {
					n.Labels = map[string]string{"topology.kubernetes.io/zone": "zoneA", "kubernetes.io/hostname": "n2"}
				}),
				test.BuildTestNode("n3", 2000, 3000, 10, func(n *v1.Node) {
					n.Labels = map[string]string{"topology.kubernetes.io/zone": "zoneA", "kubernetes.io/hostname": "n3"}
				}),
				test.BuildTestNode("n4", 2000, 3000, 10, func(n *v1.Node) {
					n.Labels = map[string]string{"topology.kubernetes.io/zone": "zoneB", "kubernetes.io/hostname": "n4"}
				}),
				test.BuildTestNode("n5", 2000, 3000, 10, func(n *v1.Node) {
					n.Labels = map[string]string{"topology.kubernetes.io/zone": "zoneB", "kubernetes.io/hostname": "n5"}
				}),
				test.BuildTestNode("n6", 2000, 3000, 10, func(n *v1.Node) {
					n.Labels = map[string]string{"topology.kubernetes.io/zone": "zoneB", "kubernetes.io/hostname": "n6"}
				}),
			},
			pods: createTestPods([]testPodList{
				{
					count:  1,
					node:   "n1",
					labels: map[string]string{"app": "whoami"},
					constraints: []v1.TopologySpreadConstraint{
						{
							LabelSelector:     &metav1.LabelSelector{MatchLabels: map[string]string{"app": "whoami"}},
							MaxSkew:           1,
							TopologyKey:       "topology.kubernetes.io/zone",
							WhenUnsatisfiable: v1.ScheduleAnyway,
						},
						{
							LabelSelector:     &metav1.LabelSelector{MatchLabels: map[string]string{"app": "whoami"}},
							MaxSkew:           1,
							TopologyKey:       "kubernetes.io/hostname",
							WhenUnsatisfiable: v1.DoNotSchedule,
						},
					},
				},
				{
					count:  1,
					node:   "n2",
					labels: map[string]string{"app": "whoami"},
					constraints: []v1.TopologySpreadConstraint{
						{
							LabelSelector:     &metav1.LabelSelector{MatchLabels: map[string]string{"app": "whoami"}},
							MaxSkew:           1,
							TopologyKey:       "topology.kubernetes.io/zone",
							WhenUnsatisfiable: v1.ScheduleAnyway,
						},
						{
							LabelSelector:     &metav1.LabelSelector{MatchLabels: map[string]string{"app": "whoami"}},
							MaxSkew:           1,
							TopologyKey:       "kubernetes.io/hostname",
							WhenUnsatisfiable: v1.DoNotSchedule,
						},
					},
				},
				{
					count:  1,
					node:   "n3",
					labels: map[string]string{"app": "whoami"},
					constraints: []v1.TopologySpreadConstraint{
						{
							LabelSelector:     &metav1.LabelSelector{MatchLabels: map[string]string{"app": "whoami"}},
							MaxSkew:           1,
							TopologyKey:       "topology.kubernetes.io/zone",
							WhenUnsatisfiable: v1.ScheduleAnyway,
						},
						{
							LabelSelector:     &metav1.LabelSelector{MatchLabels: map[string]string{"app": "whoami"}},
							MaxSkew:           1,
							TopologyKey:       "kubernetes.io/hostname",
							WhenUnsatisfiable: v1.DoNotSchedule,
						},
					},
				},
				{
					count:  1,
					node:   "n4",
					labels: map[string]string{"app": "whoami"},
					constraints: []v1.TopologySpreadConstraint{
						{
							LabelSelector:     &metav1.LabelSelector{MatchLabels: map[string]string{"app": "whoami"}},
							MaxSkew:           1,
							TopologyKey:       "topology.kubernetes.io/zone",
							WhenUnsatisfiable: v1.ScheduleAnyway,
						},
						{
							LabelSelector:     &metav1.LabelSelector{MatchLabels: map[string]string{"app": "whoami"}},
							MaxSkew:           1,
							TopologyKey:       "kubernetes.io/hostname",
							WhenUnsatisfiable: v1.DoNotSchedule,
						},
					},
				},
			}),
			expectedEvictedCount: 1,
			strategy:             api.DeschedulerStrategy{Params: &api.StrategyParameters{IncludeSoftConstraints: true}},
			namespaces:           []string{"ns1"},
		},
		{
			name: "3 domains size [8 7 0], maxSkew=1, should move 5 to get [5 5 5]",
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneA" }),
				test.BuildTestNode("n2", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneB" }),
				test.BuildTestNode("n3", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneC" }),
			},
			pods: createTestPods([]testPodList{
				{
					count:       8,
					node:        "n1",
					labels:      map[string]string{"foo": "bar"},
					constraints: getDefaultTopologyConstraints(1),
				},
				{
					count:       7,
					node:        "n2",
					labels:      map[string]string{"foo": "bar"},
					constraints: getDefaultTopologyConstraints(1),
				},
			}),
			expectedEvictedCount: 5,
			strategy:             api.DeschedulerStrategy{},
			namespaces:           []string{"ns1"},
		},
		{
			name: "3 domains size [5 5 5], maxSkew=1, should move 0 to retain [5 5 5]",
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneA" }),
				test.BuildTestNode("n2", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneB" }),
				test.BuildTestNode("n3", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneC" }),
			},
			pods: createTestPods([]testPodList{
				{
					count:       5,
					node:        "n1",
					labels:      map[string]string{"foo": "bar"},
					constraints: getDefaultTopologyConstraints(1),
				},
				{
					count:       5,
					node:        "n2",
					labels:      map[string]string{"foo": "bar"},
					constraints: getDefaultTopologyConstraints(1),
				},
				{
					count:       5,
					node:        "n3",
					labels:      map[string]string{"foo": "bar"},
					constraints: getDefaultTopologyConstraints(1),
				},
			}),
			expectedEvictedCount: 0,
			strategy:             api.DeschedulerStrategy{},
			namespaces:           []string{"ns1"},
		},
		{
			name: "2 domains, sizes [2,0], maxSkew=1, move 1 pod since pod tolerates the node with taint",
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneA" }),
				test.BuildTestNode("n2", 2000, 3000, 10, func(n *v1.Node) {
					n.Labels["zone"] = "zoneB"
					n.Spec.Taints = []v1.Taint{
						{
							Key:    "taint-test",
							Value:  "test",
							Effect: v1.TaintEffectNoSchedule,
						},
					}
				}),
			},
			pods: createTestPods([]testPodList{
				{
					count:       1,
					node:        "n1",
					labels:      map[string]string{"foo": "bar"},
					constraints: getDefaultTopologyConstraints(1),
					tolerations: []v1.Toleration{
						{
							Key:      "taint-test",
							Value:    "test",
							Operator: v1.TolerationOpEqual,
							Effect:   v1.TaintEffectNoSchedule,
						},
					},
				},
				{
					count:        1,
					node:         "n1",
					labels:       map[string]string{"foo": "bar"},
					nodeSelector: map[string]string{"zone": "zoneA"},
				},
			}),
			expectedEvictedCount: 1,
			strategy:             api.DeschedulerStrategy{},
			namespaces:           []string{"ns1"},
		},
		{
			name: "2 domains, sizes [2,0], maxSkew=1, move 1 pods since pod does not tolerate the tainted node",
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneA" }),
				test.BuildTestNode("n2", 2000, 3000, 10, func(n *v1.Node) {
					n.Labels["zone"] = "zoneB"
					n.Spec.Taints = []v1.Taint{
						{
							Key:    "taint-test",
							Value:  "test",
							Effect: v1.TaintEffectNoSchedule,
						},
					}
				}),
			},
			pods: createTestPods([]testPodList{
				{
					count:       1,
					node:        "n1",
					labels:      map[string]string{"foo": "bar"},
					constraints: getDefaultTopologyConstraints(1),
				},
				{
					count:        1,
					node:         "n1",
					labels:       map[string]string{"foo": "bar"},
					nodeSelector: map[string]string{"zone": "zoneA"},
				},
			}),
			expectedEvictedCount: 0,
			strategy:             api.DeschedulerStrategy{},
			namespaces:           []string{"ns1"},
		},
		{
			name: "2 domains, sizes [2,0], maxSkew=1, move 0 pods since pod does not tolerate the tainted node, and NodeFit is enabled",
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneA" }),
				test.BuildTestNode("n2", 2000, 3000, 10, func(n *v1.Node) {
					n.Labels["zone"] = "zoneB"
					n.Spec.Taints = []v1.Taint{
						{
							Key:    "taint-test",
							Value:  "test",
							Effect: v1.TaintEffectNoSchedule,
						},
					}
				}),
			},
			pods: createTestPods([]testPodList{
				{
					count:       1,
					node:        "n1",
					labels:      map[string]string{"foo": "bar"},
					constraints: getDefaultTopologyConstraints(1),
				},
				{
					count:        1,
					node:         "n1",
					labels:       map[string]string{"foo": "bar"},
					nodeSelector: map[string]string{"zone": "zoneA"},
				},
			}),
			expectedEvictedCount: 0,
			strategy: api.DeschedulerStrategy{
				Params: &api.StrategyParameters{
					NodeFit: true,
				},
			},
			namespaces: []string{"ns1"},
		},
		{
			name: "2 domains, sizes [2,0], maxSkew=1, move 1 pod for node with PreferNoSchedule Taint",
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneA" }),
				test.BuildTestNode("n2", 2000, 3000, 10, func(n *v1.Node) {
					n.Labels["zone"] = "zoneB"
					n.Spec.Taints = []v1.Taint{
						{
							Key:    "taint-test",
							Value:  "test",
							Effect: v1.TaintEffectPreferNoSchedule,
						},
					}
				}),
			},
			pods: createTestPods([]testPodList{
				{
					count:       1,
					node:        "n1",
					labels:      map[string]string{"foo": "bar"},
					constraints: getDefaultTopologyConstraints(1),
				},
				{
					count:        1,
					node:         "n1",
					labels:       map[string]string{"foo": "bar"},
					nodeSelector: map[string]string{"zone": "zoneA"},
				},
			}),
			expectedEvictedCount: 1,
			strategy:             api.DeschedulerStrategy{},
			namespaces:           []string{"ns1"},
		},
		{
			name: "2 domains, sizes [2,0], maxSkew=1, move 0 pod for node with unmatched label filtering",
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneA" }),
				test.BuildTestNode("n2", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneB" }),
			},
			pods: createTestPods([]testPodList{
				{
					count:       1,
					node:        "n1",
					labels:      map[string]string{"foo": "bar"},
					constraints: getDefaultTopologyConstraints(1),
				},
				{
					count:  1,
					node:   "n1",
					labels: map[string]string{"foo": "bar"},
				},
			}),
			expectedEvictedCount: 0,
			strategy: api.DeschedulerStrategy{
				Params: &api.StrategyParameters{
					LabelSelector: getLabelSelector("foo", []string{"baz"}, metav1.LabelSelectorOpIn),
				},
			},
			namespaces: []string{"ns1"},
		},
		{
			name: "2 domains, sizes [2,0], maxSkew=1, move 1 pod for node with matched label filtering",
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneA" }),
				test.BuildTestNode("n2", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneB" }),
			},
			pods: createTestPods([]testPodList{
				{
					count:       1,
					node:        "n1",
					labels:      map[string]string{"foo": "bar"},
					constraints: getDefaultTopologyConstraints(1),
				},
				{
					count:  1,
					node:   "n1",
					labels: map[string]string{"foo": "bar"},
				},
			}),
			expectedEvictedCount: 1,
			strategy: api.DeschedulerStrategy{
				Params: &api.StrategyParameters{
					LabelSelector: getLabelSelector("foo", []string{"bar"}, metav1.LabelSelectorOpIn),
				},
			},
			namespaces: []string{"ns1"},
		},
		{
			name: "2 domains, sizes [2,0], maxSkew=1, move 1 pod for node with matched label filtering (NotIn op)",
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneA" }),
				test.BuildTestNode("n2", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneB" }),
			},
			pods: createTestPods([]testPodList{
				{
					count:       1,
					node:        "n1",
					labels:      map[string]string{"foo": "bar"},
					constraints: getDefaultTopologyConstraints(1),
				},
				{
					count:  1,
					node:   "n1",
					labels: map[string]string{"foo": "bar"},
				},
			}),
			expectedEvictedCount: 1,
			strategy: api.DeschedulerStrategy{
				Params: &api.StrategyParameters{
					LabelSelector: getLabelSelector("foo", []string{"baz"}, metav1.LabelSelectorOpNotIn),
				},
			},
			namespaces: []string{"ns1"},
		},
		{
			name: "2 domains, sizes [4,2], maxSkew=1, 2 pods in termination; nothing should be moved",
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneA" }),
				test.BuildTestNode("n2", 2000, 3000, 10, func(n *v1.Node) { n.Labels["zone"] = "zoneB" }),
			},
			pods: createTestPods([]testPodList{
				{
					count:       2,
					node:        "n1",
					labels:      map[string]string{"foo": "bar"},
					constraints: getDefaultTopologyConstraints(1),
				},
				{
					count:             2,
					node:              "n1",
					labels:            map[string]string{"foo": "bar"},
					constraints:       getDefaultTopologyConstraints(1),
					deletionTimestamp: &metav1.Time{},
				},
				{
					count:       2,
					node:        "n2",
					labels:      map[string]string{"foo": "bar"},
					constraints: getDefaultTopologyConstraints(1),
				},
			}),
			expectedEvictedCount: 0,
			strategy: api.DeschedulerStrategy{
				Params: &api.StrategyParameters{
					LabelSelector: getLabelSelector("foo", []string{"bar"}, metav1.LabelSelectorOpIn),
				},
			},
			namespaces: []string{"ns1"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			var objs []runtime.Object
			for _, node := range tc.nodes {
				objs = append(objs, node)
			}
			for _, pod := range tc.pods {
				objs = append(objs, pod)
			}
			objs = append(objs, &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns1"}})
			fakeClient := fake.NewSimpleClientset(objs...)

			sharedInformerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
			podInformer := sharedInformerFactory.Core().V1().Pods()

			getPodsAssignedToNode, err := podutil.BuildGetPodsAssignedToNodeFunc(podInformer)
			if err != nil {
				t.Errorf("Build get pods assigned to node function error: %v", err)
			}

			sharedInformerFactory.Start(ctx.Done())
			sharedInformerFactory.WaitForCacheSync(ctx.Done())

			podEvictor := evictions.NewPodEvictor(
				fakeClient,
				"v1",
				false,
				nil,
				nil,
				tc.nodes,
				false,
				nil,
			)

			nodeFit := false
			if tc.strategy.Params != nil {
				nodeFit = tc.strategy.Params.NodeFit
			}

			evictorFilter := evictions.NewEvictorFilter(
				tc.nodes,
				getPodsAssignedToNode,
				false,
				false,
				false,
				false,
				evictions.WithNodeFit(nodeFit),
			)

			RemovePodsViolatingTopologySpreadConstraint(ctx, fakeClient, tc.strategy, tc.nodes, podEvictor, evictorFilter, getPodsAssignedToNode)
			podsEvicted := podEvictor.TotalEvicted()
			if podsEvicted != tc.expectedEvictedCount {
				t.Errorf("Test error for description: %s. Expected evicted pods count %v, got %v", tc.name, tc.expectedEvictedCount, podsEvicted)
			}
		})
	}
}

type testPodList struct {
	count             int
	node              string
	labels            map[string]string
	constraints       []v1.TopologySpreadConstraint
	nodeSelector      map[string]string
	nodeAffinity      *v1.Affinity
	noOwners          bool
	tolerations       []v1.Toleration
	deletionTimestamp *metav1.Time
}

func createTestPods(testPods []testPodList) []*v1.Pod {
	ownerRef1 := test.GetReplicaSetOwnerRefList()
	pods := make([]*v1.Pod, 0)
	podNum := 0
	for _, tp := range testPods {
		for i := 0; i < tp.count; i++ {
			pods = append(pods,
				test.BuildTestPod(fmt.Sprintf("pod-%d", podNum), 100, 0, tp.node, func(p *v1.Pod) {
					p.Labels = make(map[string]string)
					p.Labels = tp.labels
					p.Namespace = "ns1"
					if !tp.noOwners {
						p.ObjectMeta.OwnerReferences = ownerRef1
					}
					p.Spec.TopologySpreadConstraints = tp.constraints
					p.Spec.NodeSelector = tp.nodeSelector
					p.Spec.Affinity = tp.nodeAffinity
					p.Spec.Tolerations = tp.tolerations
					p.ObjectMeta.DeletionTimestamp = tp.deletionTimestamp
				}))
			podNum++
		}
	}
	return pods
}

func getLabelSelector(key string, values []string, operator metav1.LabelSelectorOperator) *metav1.LabelSelector {
	return &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{{Key: key, Operator: operator, Values: values}},
	}
}

func getDefaultTopologyConstraints(maxSkew int32) []v1.TopologySpreadConstraint {
	return []v1.TopologySpreadConstraint{
		{
			MaxSkew:           maxSkew,
			TopologyKey:       "zone",
			WhenUnsatisfiable: v1.DoNotSchedule,
			LabelSelector:     &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
		},
	}
}

func TestCheckIdenticalConstraints(t *testing.T) {
	newConstraintSame := v1.TopologySpreadConstraint{
		MaxSkew:           2,
		TopologyKey:       "zone",
		WhenUnsatisfiable: v1.DoNotSchedule,
		LabelSelector:     &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
	}
	newConstraintDifferent := v1.TopologySpreadConstraint{
		MaxSkew:           3,
		TopologyKey:       "node",
		WhenUnsatisfiable: v1.DoNotSchedule,
		LabelSelector:     &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
	}
	namespaceTopologySpreadConstraint := []v1.TopologySpreadConstraint{}
	namespaceTopologySpreadConstraint = []v1.TopologySpreadConstraint{
		{
			MaxSkew:           2,
			TopologyKey:       "zone",
			WhenUnsatisfiable: v1.DoNotSchedule,
			LabelSelector:     &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
		},
	}
	testCases := []struct {
		name                               string
		namespaceTopologySpreadConstraints []v1.TopologySpreadConstraint
		newConstraint                      v1.TopologySpreadConstraint
		expectedResult                     bool
	}{
		{
			name:                               "new constraint is identical",
			namespaceTopologySpreadConstraints: namespaceTopologySpreadConstraint,
			newConstraint:                      newConstraintSame,
			expectedResult:                     true,
		},
		{
			name:                               "new constraint is different",
			namespaceTopologySpreadConstraints: namespaceTopologySpreadConstraint,
			newConstraint:                      newConstraintDifferent,
			expectedResult:                     false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			isIdentical := hasIdenticalConstraints(tc.newConstraint, tc.namespaceTopologySpreadConstraints)
			if isIdentical != tc.expectedResult {
				t.Errorf("Test error for description: %s. Expected result %v, got %v", tc.name, tc.expectedResult, isIdentical)
			}
		})
	}
}
