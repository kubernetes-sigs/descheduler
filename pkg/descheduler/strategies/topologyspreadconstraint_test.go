package strategies

import (
	"context"
	"fmt"
	"testing"

	"sigs.k8s.io/descheduler/pkg/api"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	"sigs.k8s.io/descheduler/test"
)

func TestTopologySpreadConstraint(t *testing.T) {
	ctx := context.Background()
	testCases := []struct {
		name                 string
		pods                 []*v1.Pod
		expectedEvictedCount int
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
			strategy:             api.DeschedulerStrategy{},
			namespaces:           []string{"ns1"},
		},
		{
			name: "2 domains, sizes [3,1], maxSkew=1, move 1 pod to achieve [2,2]",
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
			strategy:             api.DeschedulerStrategy{},
			namespaces:           []string{"ns1"},
		},
		{
			name: "NEW",
			//name: "2 domains, sizes [3,1], maxSkew=1, no pods eligible, move 0 pods",
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
					noOwners: true,
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
			strategy:             api.DeschedulerStrategy{},
			namespaces:           []string{"ns1"},
		},
		{
			name: "2 domains, sizes [3,1], maxSkew=1, move 1 pod to achieve [2,2], exclude kube-system namespace",
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
			strategy:             api.DeschedulerStrategy{Enabled: true, Params: &api.StrategyParameters{Namespaces: &api.Namespaces{Exclude: []string{"kube-system"}}}},
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
			strategy:             api.DeschedulerStrategy{},
			namespaces:           []string{"ns1"},
		},
		{
			name: "2 domains, sizes [4,0], maxSkew=1, move 2 pods to achieve [2,2]",
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
					count:  3,
					node:   "n1",
					labels: map[string]string{"foo": "bar"},
				},
			}),
			expectedEvictedCount: 2,
			strategy:             api.DeschedulerStrategy{},
			namespaces:           []string{"ns1"},
		},
		{
			name: "2 domains, sizes [4,0], maxSkew=1, only move 1 pod since pods with nodeSelector and nodeAffinity aren't evicted",
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
			strategy:             api.DeschedulerStrategy{},
			namespaces:           []string{"ns1"},
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
					count:  1,
					node:   "n2",
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
					count:  1,
					node:   "n2",
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
			strategy:             api.DeschedulerStrategy{},
			namespaces:           []string{"ns1"},
		},
		{
			name: "2 domains size [2 6], maxSkew=2, should move 1 to get [3 5]",
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
			expectedEvictedCount: 1,
			strategy:             api.DeschedulerStrategy{},
			namespaces:           []string{"ns1"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeClient := &fake.Clientset{}
			fakeClient.Fake.AddReactor("list", "pods", func(action core.Action) (bool, runtime.Object, error) {
				podList := make([]v1.Pod, 0, len(tc.pods))
				for _, pod := range tc.pods {
					podList = append(podList, *pod)
				}
				return true, &v1.PodList{Items: podList}, nil
			})
			fakeClient.Fake.AddReactor("list", "namespaces", func(action core.Action) (bool, runtime.Object, error) {
				return true, &v1.NamespaceList{Items: []v1.Namespace{{ObjectMeta: metav1.ObjectMeta{Name: "ns1", Namespace: "ns1"}}}}, nil
			})

			podEvictor := evictions.NewPodEvictor(
				fakeClient,
				"v1",
				false,
				100,
				tc.nodes,
				false,
				false,
				false,
			)
			RemovePodsViolatingTopologySpreadConstraint(ctx, fakeClient, tc.strategy, tc.nodes, podEvictor)
			podsEvicted := podEvictor.TotalEvicted()
			if podsEvicted != tc.expectedEvictedCount {
				t.Errorf("Test error for description: %s. Expected evicted pods count %v, got %v", tc.name, tc.expectedEvictedCount, podsEvicted)
			}
		})
	}
}

type testPodList struct {
	count        int
	node         string
	labels       map[string]string
	constraints  []v1.TopologySpreadConstraint
	nodeSelector map[string]string
	nodeAffinity *v1.Affinity
	noOwners     bool
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
				}))
			podNum++
		}
	}
	return pods
}
