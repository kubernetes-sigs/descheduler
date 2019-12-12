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

package interpodaffinity

import (
	"context"
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	clientsetfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/kubernetes/pkg/scheduler/algorithm/predicates"
	"k8s.io/kubernetes/pkg/scheduler/algorithm/priorities"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/migration"
	framework "k8s.io/kubernetes/pkg/scheduler/framework/v1alpha1"
	nodeinfosnapshot "k8s.io/kubernetes/pkg/scheduler/nodeinfo/snapshot"
)

var (
	defaultNamespace             = ""
	unschedulable                = framework.NewStatus(framework.Unschedulable, predicates.ErrPodAffinityNotMatch.GetReason())
	unschedulableAndUnresolvable = framework.NewStatus(framework.UnschedulableAndUnresolvable, predicates.ErrPodAffinityRulesNotMatch.GetReason())
)

func createPodWithAffinityTerms(namespace, nodeName string, labels map[string]string, affinity, antiAffinity []v1.PodAffinityTerm) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Labels:    labels,
			Namespace: namespace,
		},
		Spec: v1.PodSpec{
			NodeName: nodeName,
			Affinity: &v1.Affinity{
				PodAffinity: &v1.PodAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: affinity,
				},
				PodAntiAffinity: &v1.PodAntiAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: antiAffinity,
				},
			},
		},
	}

}

func TestSingleNode(t *testing.T) {
	podLabel := map[string]string{"service": "securityscan"}
	labels1 := map[string]string{
		"region": "r1",
		"zone":   "z11",
	}
	podLabel2 := map[string]string{"security": "S1"}
	node1 := v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "machine1", Labels: labels1}}
	tests := []struct {
		pod        *v1.Pod
		pods       []*v1.Pod
		node       *v1.Node
		name       string
		wantStatus *framework.Status
	}{
		{
			pod:  new(v1.Pod),
			node: &node1,
			name: "A pod that has no required pod affinity scheduling rules can schedule onto a node with no existing pods",
		},
		{
			pod: createPodWithAffinityTerms(defaultNamespace, "", podLabel2,
				[]v1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "service",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"securityscan", "value2"},
								},
							},
						},
						TopologyKey: "region",
					},
				}, nil),
			pods: []*v1.Pod{{Spec: v1.PodSpec{NodeName: "machine1"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabel}}},
			node: &node1,
			name: "satisfies with requiredDuringSchedulingIgnoredDuringExecution in PodAffinity using In operator that matches the existing pod",
		},
		{
			pod: createPodWithAffinityTerms(defaultNamespace, "", podLabel2,
				[]v1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "service",
									Operator: metav1.LabelSelectorOpNotIn,
									Values:   []string{"securityscan3", "value3"},
								},
							},
						},
						TopologyKey: "region",
					},
				}, nil),
			pods: []*v1.Pod{{Spec: v1.PodSpec{NodeName: "machine1"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabel}}},
			node: &node1,
			name: "satisfies the pod with requiredDuringSchedulingIgnoredDuringExecution in PodAffinity using not in operator in labelSelector that matches the existing pod",
		},
		{
			pod: createPodWithAffinityTerms(defaultNamespace, "", podLabel2,
				[]v1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "service",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"securityscan", "value2"},
								},
							},
						},
						Namespaces: []string{"DiffNameSpace"},
					},
				}, nil),
			pods:       []*v1.Pod{{Spec: v1.PodSpec{NodeName: "machine1"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabel, Namespace: "ns"}}},
			node:       &node1,
			name:       "Does not satisfy the PodAffinity with labelSelector because of diff Namespace",
			wantStatus: unschedulableAndUnresolvable,
		},
		{
			pod: createPodWithAffinityTerms(defaultNamespace, "", podLabel,
				[]v1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "service",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"antivirusscan", "value2"},
								},
							},
						},
					},
				}, nil),
			pods:       []*v1.Pod{{Spec: v1.PodSpec{NodeName: "machine1"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabel}}},
			node:       &node1,
			name:       "Doesn't satisfy the PodAffinity because of unmatching labelSelector with the existing pod",
			wantStatus: unschedulableAndUnresolvable,
		},
		{
			pod: createPodWithAffinityTerms(defaultNamespace, "", podLabel2,
				[]v1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "service",
									Operator: metav1.LabelSelectorOpExists,
								}, {
									Key:      "wrongkey",
									Operator: metav1.LabelSelectorOpDoesNotExist,
								},
							},
						},
						TopologyKey: "region",
					}, {
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "service",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"securityscan"},
								}, {
									Key:      "service",
									Operator: metav1.LabelSelectorOpNotIn,
									Values:   []string{"WrongValue"},
								},
							},
						},
						TopologyKey: "region",
					},
				}, nil),
			pods: []*v1.Pod{{Spec: v1.PodSpec{NodeName: "machine1"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabel}}},
			node: &node1,
			name: "satisfies the PodAffinity with different label Operators in multiple RequiredDuringSchedulingIgnoredDuringExecution ",
		},
		{
			pod: createPodWithAffinityTerms(defaultNamespace, "", podLabel2,
				[]v1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "service",
									Operator: metav1.LabelSelectorOpExists,
								}, {
									Key:      "wrongkey",
									Operator: metav1.LabelSelectorOpDoesNotExist,
								},
							},
						},
						TopologyKey: "region",
					}, {
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "service",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"securityscan2"},
								}, {
									Key:      "service",
									Operator: metav1.LabelSelectorOpNotIn,
									Values:   []string{"WrongValue"},
								},
							},
						},
						TopologyKey: "region",
					},
				}, nil),
			pods:       []*v1.Pod{{Spec: v1.PodSpec{NodeName: "machine1"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabel}}},
			node:       &node1,
			name:       "The labelSelector requirements(items of matchExpressions) are ANDed, the pod cannot schedule onto the node because one of the matchExpression item don't match.",
			wantStatus: unschedulableAndUnresolvable,
		},
		{
			pod: createPodWithAffinityTerms(defaultNamespace, "", podLabel2,
				[]v1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "service",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"securityscan", "value2"},
								},
							},
						},
						TopologyKey: "region",
					},
				},
				[]v1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "service",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"antivirusscan", "value2"},
								},
							},
						},
						TopologyKey: "node",
					},
				}),
			pods: []*v1.Pod{{Spec: v1.PodSpec{NodeName: "machine1"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabel}}},
			node: &node1,
			name: "satisfies the PodAffinity and PodAntiAffinity with the existing pod",
		},
		{
			pod: createPodWithAffinityTerms(defaultNamespace, "", podLabel2,
				[]v1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "service",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"securityscan", "value2"},
								},
							},
						},
						TopologyKey: "region",
					},
				},
				[]v1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "service",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"antivirusscan", "value2"},
								},
							},
						},
						TopologyKey: "node",
					},
				}),
			pods: []*v1.Pod{
				createPodWithAffinityTerms(defaultNamespace, "machine1", podLabel, nil,
					[]v1.PodAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "service",
										Operator: metav1.LabelSelectorOpIn,
										Values:   []string{"antivirusscan", "value2"},
									},
								},
							},
							TopologyKey: "node",
						},
					}),
			},
			node: &node1,
			name: "satisfies the PodAffinity and PodAntiAffinity and PodAntiAffinity symmetry with the existing pod",
		},
		{
			pod: createPodWithAffinityTerms(defaultNamespace, "", podLabel2,
				[]v1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "service",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"securityscan", "value2"},
								},
							},
						},
						TopologyKey: "region",
					},
				},
				[]v1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "service",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"securityscan", "value2"},
								},
							},
						},
						TopologyKey: "zone",
					},
				}),
			pods:       []*v1.Pod{{Spec: v1.PodSpec{NodeName: "machine1"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabel}}},
			node:       &node1,
			name:       "satisfies the PodAffinity but doesn't satisfy the PodAntiAffinity with the existing pod",
			wantStatus: unschedulable,
		},
		{
			pod: createPodWithAffinityTerms(defaultNamespace, "", podLabel,
				[]v1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "service",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"securityscan", "value2"},
								},
							},
						},
						TopologyKey: "region",
					},
				},
				[]v1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "service",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"antivirusscan", "value2"},
								},
							},
						},
						TopologyKey: "node",
					},
				}),
			pods: []*v1.Pod{
				createPodWithAffinityTerms(defaultNamespace, "machine1", podLabel, nil,
					[]v1.PodAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "service",
										Operator: metav1.LabelSelectorOpIn,
										Values:   []string{"securityscan", "value2"},
									},
								},
							},
							TopologyKey: "zone",
						},
					}),
			},
			node:       &node1,
			name:       "satisfies the PodAffinity and PodAntiAffinity but doesn't satisfy PodAntiAffinity symmetry with the existing pod",
			wantStatus: unschedulable,
		},
		{
			pod: createPodWithAffinityTerms(defaultNamespace, "", podLabel,
				[]v1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "service",
									Operator: metav1.LabelSelectorOpNotIn,
									Values:   []string{"securityscan", "value2"},
								},
							},
						},
						TopologyKey: "region",
					},
				}, nil),
			pods:       []*v1.Pod{{Spec: v1.PodSpec{NodeName: "machine2"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabel}}},
			node:       &node1,
			name:       "pod matches its own Label in PodAffinity and that matches the existing pod Labels",
			wantStatus: unschedulableAndUnresolvable,
		},
		{
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: podLabel,
				},
			},
			pods: []*v1.Pod{
				createPodWithAffinityTerms(defaultNamespace, "machine1", podLabel, nil,
					[]v1.PodAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "service",
										Operator: metav1.LabelSelectorOpIn,
										Values:   []string{"securityscan", "value2"},
									},
								},
							},
							TopologyKey: "zone",
						},
					}),
			},
			node:       &node1,
			name:       "verify that PodAntiAffinity from existing pod is respected when pod has no AntiAffinity constraints. doesn't satisfy PodAntiAffinity symmetry with the existing pod",
			wantStatus: unschedulable,
		},
		{
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: podLabel,
				},
			},
			pods: []*v1.Pod{
				createPodWithAffinityTerms(defaultNamespace, "machine1", podLabel, nil,
					[]v1.PodAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "service",
										Operator: metav1.LabelSelectorOpNotIn,
										Values:   []string{"securityscan", "value2"},
									},
								},
							},
							TopologyKey: "zone",
						},
					}),
			},
			node: &node1,
			name: "verify that PodAntiAffinity from existing pod is respected when pod has no AntiAffinity constraints. satisfy PodAntiAffinity symmetry with the existing pod",
		},
		{
			pod: createPodWithAffinityTerms(defaultNamespace, "", podLabel, nil,
				[]v1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "service",
									Operator: metav1.LabelSelectorOpExists,
								},
							},
						},
						TopologyKey: "region",
					},
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "security",
									Operator: metav1.LabelSelectorOpExists,
								},
							},
						},
						TopologyKey: "region",
					},
				}),
			pods: []*v1.Pod{
				createPodWithAffinityTerms(defaultNamespace, "machine1", podLabel2, nil,
					[]v1.PodAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "security",
										Operator: metav1.LabelSelectorOpExists,
									},
								},
							},
							TopologyKey: "zone",
						},
					}),
			},
			node:       &node1,
			name:       "satisfies the PodAntiAffinity with existing pod but doesn't satisfy PodAntiAffinity symmetry with incoming pod",
			wantStatus: unschedulable,
		},
		{
			pod: createPodWithAffinityTerms(defaultNamespace, "", podLabel, nil,
				[]v1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "service",
									Operator: metav1.LabelSelectorOpExists,
								},
							},
						},
						TopologyKey: "zone",
					},
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "security",
									Operator: metav1.LabelSelectorOpExists,
								},
							},
						},
						TopologyKey: "zone",
					},
				}),
			pods: []*v1.Pod{
				createPodWithAffinityTerms(defaultNamespace, "machine1", podLabel2, nil,
					[]v1.PodAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "security",
										Operator: metav1.LabelSelectorOpExists,
									},
								},
							},
							TopologyKey: "zone",
						},
					}),
			},
			node:       &node1,
			wantStatus: unschedulable,
			name:       "PodAntiAffinity symmetry check a1: incoming pod and existing pod partially match each other on AffinityTerms",
		},
		{
			pod: createPodWithAffinityTerms(defaultNamespace, "", podLabel2, nil,
				[]v1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "security",
									Operator: metav1.LabelSelectorOpExists,
								},
							},
						},
						TopologyKey: "zone",
					},
				}),
			pods: []*v1.Pod{
				createPodWithAffinityTerms(defaultNamespace, "machine1", podLabel, nil,
					[]v1.PodAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "service",
										Operator: metav1.LabelSelectorOpExists,
									},
								},
							},
							TopologyKey: "zone",
						},
						{
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "security",
										Operator: metav1.LabelSelectorOpExists,
									},
								},
							},
							TopologyKey: "zone",
						},
					}),
			},
			node:       &node1,
			wantStatus: unschedulable,
			name:       "PodAntiAffinity symmetry check a2: incoming pod and existing pod partially match each other on AffinityTerms",
		},
		{
			pod: createPodWithAffinityTerms(defaultNamespace, "", map[string]string{"abc": "", "xyz": ""}, nil,
				[]v1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "abc",
									Operator: metav1.LabelSelectorOpExists,
								},
							},
						},
						TopologyKey: "zone",
					},
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "def",
									Operator: metav1.LabelSelectorOpExists,
								},
							},
						},
						TopologyKey: "zone",
					},
				}),
			pods: []*v1.Pod{
				createPodWithAffinityTerms(defaultNamespace, "machine1", map[string]string{"def": "", "xyz": ""}, nil,
					[]v1.PodAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "abc",
										Operator: metav1.LabelSelectorOpExists,
									},
								},
							},
							TopologyKey: "zone",
						},
						{
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "def",
										Operator: metav1.LabelSelectorOpExists,
									},
								},
							},
							TopologyKey: "zone",
						},
					}),
			},
			node:       &node1,
			wantStatus: unschedulable,
			name:       "PodAntiAffinity symmetry check b1: incoming pod and existing pod partially match each other on AffinityTerms",
		},
		{
			pod: createPodWithAffinityTerms(defaultNamespace, "", map[string]string{"def": "", "xyz": ""}, nil,
				[]v1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "abc",
									Operator: metav1.LabelSelectorOpExists,
								},
							},
						},
						TopologyKey: "zone",
					},
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "def",
									Operator: metav1.LabelSelectorOpExists,
								},
							},
						},
						TopologyKey: "zone",
					},
				}),
			pods: []*v1.Pod{
				createPodWithAffinityTerms(defaultNamespace, "machine1", map[string]string{"abc": "", "xyz": ""}, nil,
					[]v1.PodAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "abc",
										Operator: metav1.LabelSelectorOpExists,
									},
								},
							},
							TopologyKey: "zone",
						},
						{
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "def",
										Operator: metav1.LabelSelectorOpExists,
									},
								},
							},
							TopologyKey: "zone",
						},
					}),
			},
			node:       &node1,
			wantStatus: unschedulable,
			name:       "PodAntiAffinity symmetry check b2: incoming pod and existing pod partially match each other on AffinityTerms",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := nodeinfosnapshot.NewSnapshot(nodeinfosnapshot.CreateNodeInfoMap(test.pods, []*v1.Node{test.node}))
			factory := &predicates.MetadataProducerFactory{}
			meta := factory.GetPredicateMetadata(test.pod, snapshot)
			state := framework.NewCycleState()
			state.Write(migration.PredicatesStateKey, &migration.PredicatesStateData{Reference: meta})

			p := &InterPodAffinity{
				predicate: predicates.NewPodAffinityPredicate(snapshot.NodeInfos(), snapshot.Pods()),
			}
			gotStatus := p.Filter(context.Background(), state, test.pod, snapshot.NodeInfoMap[test.node.Name])
			if !reflect.DeepEqual(gotStatus, test.wantStatus) {
				t.Errorf("status does not match: %v, want: %v", gotStatus, test.wantStatus)
			}
		})
	}
}

func TestMultipleNodes(t *testing.T) {
	podLabelA := map[string]string{
		"foo": "bar",
	}
	labelRgChina := map[string]string{
		"region": "China",
	}
	labelRgChinaAzAz1 := map[string]string{
		"region": "China",
		"az":     "az1",
	}
	labelRgIndia := map[string]string{
		"region": "India",
	}

	tests := []struct {
		pod          *v1.Pod
		pods         []*v1.Pod
		nodes        []*v1.Node
		wantStatuses []*framework.Status
		name         string
	}{
		{
			pod: createPodWithAffinityTerms(defaultNamespace, "", nil,
				[]v1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "foo",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"bar"},
								},
							},
						},
						TopologyKey: "region",
					},
				}, nil),
			pods: []*v1.Pod{
				{Spec: v1.PodSpec{NodeName: "machine1"}, ObjectMeta: metav1.ObjectMeta{Name: "p1", Labels: podLabelA}},
			},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "machine1", Labels: labelRgChina}},
				{ObjectMeta: metav1.ObjectMeta{Name: "machine2", Labels: labelRgChinaAzAz1}},
				{ObjectMeta: metav1.ObjectMeta{Name: "machine3", Labels: labelRgIndia}},
			},
			wantStatuses: []*framework.Status{nil, nil, unschedulableAndUnresolvable},
			name:         "A pod can be scheduled onto all the nodes that have the same topology key & label value with one of them has an existing pod that matches the affinity rules",
		},
		{
			pod: createPodWithAffinityTerms(defaultNamespace, "", map[string]string{"foo": "bar", "service": "securityscan"},
				[]v1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "foo",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"bar"},
								},
							},
						},
						TopologyKey: "zone",
					},
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "service",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"securityscan"},
								},
							},
						},
						TopologyKey: "zone",
					},
				}, nil),
			pods: []*v1.Pod{{Spec: v1.PodSpec{NodeName: "nodeA"}, ObjectMeta: metav1.ObjectMeta{Name: "p1", Labels: map[string]string{"foo": "bar"}}}},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeA", Labels: map[string]string{"zone": "az1", "hostname": "h1"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeB", Labels: map[string]string{"zone": "az2", "hostname": "h2"}}},
			},
			wantStatuses: []*framework.Status{nil, nil},
			name: "The affinity rule is to schedule all of the pods of this collection to the same zone. The first pod of the collection " +
				"should not be blocked from being scheduled onto any node, even there's no existing pod that matches the rule anywhere.",
		},
		{
			pod: createPodWithAffinityTerms(defaultNamespace, "", nil, nil,
				[]v1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "foo",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"abc"},
								},
							},
						},
						TopologyKey: "region",
					},
				}),
			pods: []*v1.Pod{
				{Spec: v1.PodSpec{NodeName: "nodeA"}, ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"foo": "abc"}}},
			},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeA", Labels: map[string]string{"region": "r1", "hostname": "nodeA"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeB", Labels: map[string]string{"region": "r1", "hostname": "nodeB"}}},
			},
			wantStatuses: []*framework.Status{unschedulable, unschedulable},
			name:         "NodeA and nodeB have same topologyKey and label value. NodeA has an existing pod that matches the inter pod affinity rule. The pod can not be scheduled onto nodeA and nodeB.",
		},
		{
			pod: createPodWithAffinityTerms(defaultNamespace, "", nil, nil,
				[]v1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "foo",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"abc"},
								},
							},
						},
						TopologyKey: "region",
					},
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "service",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"securityscan"},
								},
							},
						},
						TopologyKey: "zone",
					},
				}),
			pods: []*v1.Pod{
				{Spec: v1.PodSpec{NodeName: "nodeA"}, ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"foo": "abc", "service": "securityscan"}}},
			},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeA", Labels: map[string]string{"region": "r1", "zone": "z1", "hostname": "nodeA"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeB", Labels: map[string]string{"region": "r1", "zone": "z2", "hostname": "nodeB"}}},
			},
			wantStatuses: []*framework.Status{unschedulable, unschedulable},
			name:         "This test ensures that anti-affinity matches a pod when any term of the anti-affinity rule matches a pod.",
		},
		{
			pod: createPodWithAffinityTerms(defaultNamespace, "", nil, nil,
				[]v1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "foo",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"abc"},
								},
							},
						},
						TopologyKey: "region",
					},
				}),
			pods: []*v1.Pod{
				{Spec: v1.PodSpec{NodeName: "nodeA"}, ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"foo": "abc"}}},
			},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeA", Labels: labelRgChina}},
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeB", Labels: labelRgChinaAzAz1}},
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeC", Labels: labelRgIndia}},
			},
			wantStatuses: []*framework.Status{unschedulable, unschedulable, nil},
			name:         "NodeA and nodeB have same topologyKey and label value. NodeA has an existing pod that matches the inter pod affinity rule. The pod can not be scheduled onto nodeA and nodeB but can be scheduled onto nodeC",
		},
		{
			pod: createPodWithAffinityTerms("NS1", "", map[string]string{"foo": "123"}, nil,
				[]v1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "foo",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"bar"},
								},
							},
						},
						TopologyKey: "region",
					},
				}),
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels:    map[string]string{"foo": "bar"},
						Namespace: "NS1",
					},
					Spec: v1.PodSpec{NodeName: "nodeA"},
				},
				createPodWithAffinityTerms("NS2", "nodeC", nil, nil,
					[]v1.PodAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "foo",
										Operator: metav1.LabelSelectorOpIn,
										Values:   []string{"123"},
									},
								},
							},
							TopologyKey: "region",
						},
					}),
			},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeA", Labels: labelRgChina}},
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeB", Labels: labelRgChinaAzAz1}},
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeC", Labels: labelRgIndia}},
			},
			wantStatuses: []*framework.Status{unschedulable, unschedulable, nil},
			name:         "NodeA and nodeB have same topologyKey and label value. NodeA has an existing pod that matches the inter pod affinity rule. The pod can not be scheduled onto nodeA, nodeB, but can be scheduled onto nodeC (NodeC has an existing pod that match the inter pod affinity rule but in different namespace)",
		},
		{
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"foo": ""}},
			},
			pods: []*v1.Pod{
				createPodWithAffinityTerms(defaultNamespace, "nodeA", nil, nil,
					[]v1.PodAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "foo",
										Operator: metav1.LabelSelectorOpExists,
									},
								},
							},
							TopologyKey: "invalid-node-label",
						},
					}),
			},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeA", Labels: map[string]string{"region": "r1", "zone": "z1", "hostname": "nodeA"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeB", Labels: map[string]string{"region": "r1", "zone": "z1", "hostname": "nodeB"}}},
			},
			wantStatuses: []*framework.Status{nil, nil},
			name:         "Test existing pod's anti-affinity: if an existing pod has a term with invalid topologyKey, labelSelector of the term is firstly checked, and then topologyKey of the term is also checked",
		},
		{
			pod: createPodWithAffinityTerms(defaultNamespace, "", nil, nil,
				[]v1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "foo",
									Operator: metav1.LabelSelectorOpExists,
								},
							},
						},
						TopologyKey: "invalid-node-label",
					},
				}),
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"foo": ""}},
					Spec: v1.PodSpec{
						NodeName: "nodeA",
					},
				},
			},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeA", Labels: map[string]string{"region": "r1", "zone": "z1", "hostname": "nodeA"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeB", Labels: map[string]string{"region": "r1", "zone": "z1", "hostname": "nodeB"}}},
			},
			wantStatuses: []*framework.Status{nil, nil},
			name:         "Test incoming pod's anti-affinity: even if labelSelector matches, we still check if topologyKey matches",
		},
		{
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"foo": "", "bar": ""}},
			},
			pods: []*v1.Pod{
				createPodWithAffinityTerms(defaultNamespace, "nodeA", nil, nil,
					[]v1.PodAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "foo",
										Operator: metav1.LabelSelectorOpExists,
									},
								},
							},
							TopologyKey: "zone",
						},
					}),
				createPodWithAffinityTerms(defaultNamespace, "nodeA", nil, nil,
					[]v1.PodAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "bar",
										Operator: metav1.LabelSelectorOpExists,
									},
								},
							},
							TopologyKey: "region",
						},
					}),
			},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeA", Labels: map[string]string{"region": "r1", "zone": "z1", "hostname": "nodeA"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeB", Labels: map[string]string{"region": "r1", "zone": "z2", "hostname": "nodeB"}}},
			},
			wantStatuses: []*framework.Status{unschedulable, unschedulable},
			name:         "Test existing pod's anti-affinity: incoming pod wouldn't considered as a fit as it violates each existingPod's terms on all nodes",
		},
		{
			pod: createPodWithAffinityTerms(defaultNamespace, "", nil, nil,
				[]v1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "foo",
									Operator: metav1.LabelSelectorOpExists,
								},
							},
						},
						TopologyKey: "zone",
					},
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "bar",
									Operator: metav1.LabelSelectorOpExists,
								},
							},
						},
						TopologyKey: "region",
					},
				}),
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"foo": ""}},
					Spec: v1.PodSpec{
						NodeName: "nodeA",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"bar": ""}},
					Spec: v1.PodSpec{
						NodeName: "nodeB",
					},
				},
			},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeA", Labels: map[string]string{"region": "r1", "zone": "z1", "hostname": "nodeA"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeB", Labels: map[string]string{"region": "r1", "zone": "z2", "hostname": "nodeB"}}},
			},
			wantStatuses: []*framework.Status{unschedulable, unschedulable},
			name:         "Test incoming pod's anti-affinity: incoming pod wouldn't considered as a fit as it at least violates one anti-affinity rule of existingPod",
		},
		{
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"foo": "", "bar": ""}},
			},
			pods: []*v1.Pod{
				createPodWithAffinityTerms(defaultNamespace, "nodeA", nil, nil,
					[]v1.PodAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "foo",
										Operator: metav1.LabelSelectorOpExists,
									},
								},
							},
							TopologyKey: "invalid-node-label",
						},
						{
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "bar",
										Operator: metav1.LabelSelectorOpExists,
									},
								},
							},
							TopologyKey: "zone",
						},
					}),
			},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeA", Labels: map[string]string{"region": "r1", "zone": "z1", "hostname": "nodeA"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeB", Labels: map[string]string{"region": "r1", "zone": "z2", "hostname": "nodeB"}}},
			},
			wantStatuses: []*framework.Status{unschedulable, nil},
			name:         "Test existing pod's anti-affinity: only when labelSelector and topologyKey both match, it's counted as a single term match - case when one term has invalid topologyKey",
		},
		{
			pod: createPodWithAffinityTerms(defaultNamespace, "", nil, nil,
				[]v1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "foo",
									Operator: metav1.LabelSelectorOpExists,
								},
							},
						},
						TopologyKey: "invalid-node-label",
					},
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "bar",
									Operator: metav1.LabelSelectorOpExists,
								},
							},
						},
						TopologyKey: "zone",
					},
				}),
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "podA", Labels: map[string]string{"foo": "", "bar": ""}},
					Spec: v1.PodSpec{
						NodeName: "nodeA",
					},
				},
			},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeA", Labels: map[string]string{"region": "r1", "zone": "z1", "hostname": "nodeA"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeB", Labels: map[string]string{"region": "r1", "zone": "z2", "hostname": "nodeB"}}},
			},
			wantStatuses: []*framework.Status{unschedulable, nil},
			name:         "Test incoming pod's anti-affinity: only when labelSelector and topologyKey both match, it's counted as a single term match - case when one term has invalid topologyKey",
		},
		{
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"foo": "", "bar": ""}},
			},
			pods: []*v1.Pod{
				createPodWithAffinityTerms(defaultNamespace, "nodeA", nil, nil,
					[]v1.PodAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "foo",
										Operator: metav1.LabelSelectorOpExists,
									},
								},
							},
							TopologyKey: "region",
						},
						{
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "bar",
										Operator: metav1.LabelSelectorOpExists,
									},
								},
							},
							TopologyKey: "zone",
						},
					}),
			},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeA", Labels: map[string]string{"region": "r1", "zone": "z1", "hostname": "nodeA"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeB", Labels: map[string]string{"region": "r1", "zone": "z2", "hostname": "nodeB"}}},
			},
			wantStatuses: []*framework.Status{unschedulable, unschedulable},
			name:         "Test existing pod's anti-affinity: only when labelSelector and topologyKey both match, it's counted as a single term match - case when all terms have valid topologyKey",
		},
		{
			pod: createPodWithAffinityTerms(defaultNamespace, "", nil, nil,
				[]v1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "foo",
									Operator: metav1.LabelSelectorOpExists,
								},
							},
						},
						TopologyKey: "region",
					},
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "bar",
									Operator: metav1.LabelSelectorOpExists,
								},
							},
						},
						TopologyKey: "zone",
					},
				}),
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"foo": "", "bar": ""}},
					Spec: v1.PodSpec{
						NodeName: "nodeA",
					},
				},
			},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeA", Labels: map[string]string{"region": "r1", "zone": "z1", "hostname": "nodeA"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeB", Labels: map[string]string{"region": "r1", "zone": "z2", "hostname": "nodeB"}}},
			},
			wantStatuses: []*framework.Status{unschedulable, unschedulable},
			name:         "Test incoming pod's anti-affinity: only when labelSelector and topologyKey both match, it's counted as a single term match - case when all terms have valid topologyKey",
		},
		{
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"foo": "", "bar": ""}},
			},
			pods: []*v1.Pod{
				createPodWithAffinityTerms(defaultNamespace, "nodeA", nil, nil,
					[]v1.PodAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "foo",
										Operator: metav1.LabelSelectorOpExists,
									},
								},
							},
							TopologyKey: "zone",
						},
						{
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "labelA",
										Operator: metav1.LabelSelectorOpExists,
									},
								},
							},
							TopologyKey: "zone",
						},
					}),
				createPodWithAffinityTerms(defaultNamespace, "nodeB", nil, nil,
					[]v1.PodAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "bar",
										Operator: metav1.LabelSelectorOpExists,
									},
								},
							},
							TopologyKey: "zone",
						},
						{
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "labelB",
										Operator: metav1.LabelSelectorOpExists,
									},
								},
							},
							TopologyKey: "zone",
						},
					}),
			},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeA", Labels: map[string]string{"region": "r1", "zone": "z1", "hostname": "nodeA"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeB", Labels: map[string]string{"region": "r1", "zone": "z2", "hostname": "nodeB"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeC", Labels: map[string]string{"region": "r1", "zone": "z3", "hostname": "nodeC"}}},
			},
			wantStatuses: []*framework.Status{unschedulable, unschedulable, nil},
			name:         "Test existing pod's anti-affinity: existingPod on nodeA and nodeB has at least one anti-affinity term matches incoming pod, so incoming pod can only be scheduled to nodeC",
		},
		{
			pod: createPodWithAffinityTerms(defaultNamespace, "", nil,
				[]v1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "foo",
									Operator: metav1.LabelSelectorOpExists,
								},
							},
						},
						TopologyKey: "region",
					},
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "bar",
									Operator: metav1.LabelSelectorOpExists,
								},
							},
						},
						TopologyKey: "zone",
					},
				}, nil),
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod1", Labels: map[string]string{"foo": "", "bar": ""}},
					Spec: v1.PodSpec{
						NodeName: "nodeA",
					},
				},
			},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeA", Labels: map[string]string{"region": "r1", "zone": "z1", "hostname": "nodeA"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeB", Labels: map[string]string{"region": "r1", "zone": "z1", "hostname": "nodeB"}}},
			},
			wantStatuses: []*framework.Status{nil, nil},
			name:         "Test incoming pod's affinity: firstly check if all affinityTerms match, and then check if all topologyKeys match",
		},
		{
			pod: createPodWithAffinityTerms(defaultNamespace, "", nil,
				[]v1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "foo",
									Operator: metav1.LabelSelectorOpExists,
								},
							},
						},
						TopologyKey: "region",
					},
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "bar",
									Operator: metav1.LabelSelectorOpExists,
								},
							},
						},
						TopologyKey: "zone",
					},
				}, nil),
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod1", Labels: map[string]string{"foo": ""}},
					Spec: v1.PodSpec{
						NodeName: "nodeA",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod2", Labels: map[string]string{"bar": ""}},
					Spec: v1.PodSpec{
						NodeName: "nodeB",
					},
				},
			},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeA", Labels: map[string]string{"region": "r1", "zone": "z1", "hostname": "nodeA"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "nodeB", Labels: map[string]string{"region": "r1", "zone": "z2", "hostname": "nodeB"}}},
			},
			wantStatuses: []*framework.Status{unschedulableAndUnresolvable, unschedulableAndUnresolvable},
			name:         "Test incoming pod's affinity: firstly check if all affinityTerms match, and then check if all topologyKeys match, and the match logic should be satisfied on the same pod",
		},
	}

	for indexTest, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := nodeinfosnapshot.NewSnapshot(nodeinfosnapshot.CreateNodeInfoMap(test.pods, test.nodes))
			for indexNode, node := range test.nodes {
				factory := &predicates.MetadataProducerFactory{}
				meta := factory.GetPredicateMetadata(test.pod, snapshot)
				state := framework.NewCycleState()
				state.Write(migration.PredicatesStateKey, &migration.PredicatesStateData{Reference: meta})

				p := &InterPodAffinity{
					predicate: predicates.NewPodAffinityPredicate(snapshot.NodeInfos(), snapshot.Pods()),
				}
				gotStatus := p.Filter(context.Background(), state, test.pod, snapshot.NodeInfoMap[node.Name])
				if !reflect.DeepEqual(gotStatus, test.wantStatuses[indexNode]) {
					t.Errorf("index: %d status does not match: %v, want: %v", indexTest, gotStatus, test.wantStatuses[indexNode])
				}
			}
		})
	}
}

func TestInterPodAffinityPriority(t *testing.T) {
	labelRgChina := map[string]string{
		"region": "China",
	}
	labelRgIndia := map[string]string{
		"region": "India",
	}
	labelAzAz1 := map[string]string{
		"az": "az1",
	}
	labelAzAz2 := map[string]string{
		"az": "az2",
	}
	labelRgChinaAzAz1 := map[string]string{
		"region": "China",
		"az":     "az1",
	}
	podLabelSecurityS1 := map[string]string{
		"security": "S1",
	}
	podLabelSecurityS2 := map[string]string{
		"security": "S2",
	}
	// considered only preferredDuringSchedulingIgnoredDuringExecution in pod affinity
	stayWithS1InRegion := &v1.Affinity{
		PodAffinity: &v1.PodAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []v1.WeightedPodAffinityTerm{
				{
					Weight: 5,
					PodAffinityTerm: v1.PodAffinityTerm{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "security",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"S1"},
								},
							},
						},
						TopologyKey: "region",
					},
				},
			},
		},
	}
	stayWithS2InRegion := &v1.Affinity{
		PodAffinity: &v1.PodAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []v1.WeightedPodAffinityTerm{
				{
					Weight: 6,
					PodAffinityTerm: v1.PodAffinityTerm{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "security",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"S2"},
								},
							},
						},
						TopologyKey: "region",
					},
				},
			},
		},
	}
	affinity3 := &v1.Affinity{
		PodAffinity: &v1.PodAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []v1.WeightedPodAffinityTerm{
				{
					Weight: 8,
					PodAffinityTerm: v1.PodAffinityTerm{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "security",
									Operator: metav1.LabelSelectorOpNotIn,
									Values:   []string{"S1"},
								}, {
									Key:      "security",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"S2"},
								},
							},
						},
						TopologyKey: "region",
					},
				}, {
					Weight: 2,
					PodAffinityTerm: v1.PodAffinityTerm{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "security",
									Operator: metav1.LabelSelectorOpExists,
								}, {
									Key:      "wrongkey",
									Operator: metav1.LabelSelectorOpDoesNotExist,
								},
							},
						},
						TopologyKey: "region",
					},
				},
			},
		},
	}
	hardAffinity := &v1.Affinity{
		PodAffinity: &v1.PodAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
				{
					LabelSelector: &metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "security",
								Operator: metav1.LabelSelectorOpIn,
								Values:   []string{"S1", "value2"},
							},
						},
					},
					TopologyKey: "region",
				}, {
					LabelSelector: &metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "security",
								Operator: metav1.LabelSelectorOpExists,
							}, {
								Key:      "wrongkey",
								Operator: metav1.LabelSelectorOpDoesNotExist,
							},
						},
					},
					TopologyKey: "region",
				},
			},
		},
	}
	awayFromS1InAz := &v1.Affinity{
		PodAntiAffinity: &v1.PodAntiAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []v1.WeightedPodAffinityTerm{
				{
					Weight: 5,
					PodAffinityTerm: v1.PodAffinityTerm{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "security",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"S1"},
								},
							},
						},
						TopologyKey: "az",
					},
				},
			},
		},
	}
	// to stay away from security S2 in any az.
	awayFromS2InAz := &v1.Affinity{
		PodAntiAffinity: &v1.PodAntiAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []v1.WeightedPodAffinityTerm{
				{
					Weight: 5,
					PodAffinityTerm: v1.PodAffinityTerm{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "security",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"S2"},
								},
							},
						},
						TopologyKey: "az",
					},
				},
			},
		},
	}
	// to stay with security S1 in same region, stay away from security S2 in any az.
	stayWithS1InRegionAwayFromS2InAz := &v1.Affinity{
		PodAffinity: &v1.PodAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []v1.WeightedPodAffinityTerm{
				{
					Weight: 8,
					PodAffinityTerm: v1.PodAffinityTerm{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "security",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"S1"},
								},
							},
						},
						TopologyKey: "region",
					},
				},
			},
		},
		PodAntiAffinity: &v1.PodAntiAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []v1.WeightedPodAffinityTerm{
				{
					Weight: 5,
					PodAffinityTerm: v1.PodAffinityTerm{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "security",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"S2"},
								},
							},
						},
						TopologyKey: "az",
					},
				},
			},
		},
	}

	tests := []struct {
		pod          *v1.Pod
		pods         []*v1.Pod
		nodes        []*v1.Node
		expectedList framework.NodeScoreList
		name         string
	}{
		{
			pod: &v1.Pod{Spec: v1.PodSpec{NodeName: ""}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS1}},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "machine1", Labels: labelRgChina}},
				{ObjectMeta: metav1.ObjectMeta{Name: "machine2", Labels: labelRgIndia}},
				{ObjectMeta: metav1.ObjectMeta{Name: "machine3", Labels: labelAzAz1}},
			},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: 0}, {Name: "machine2", Score: 0}, {Name: "machine3", Score: 0}},
			name:         "all machines are same priority as Affinity is nil",
		},
		// the node(machine1) that have the label {"region": "China"} (match the topology key) and that have existing pods that match the labelSelector get high score
		// the node(machine3) that don't have the label {"region": "whatever the value is"} (mismatch the topology key) but that have existing pods that match the labelSelector get low score
		// the node(machine2) that have the label {"region": "China"} (match the topology key) but that have existing pods that mismatch the labelSelector get low score
		{
			pod: &v1.Pod{Spec: v1.PodSpec{NodeName: "", Affinity: stayWithS1InRegion}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS1}},
			pods: []*v1.Pod{
				{Spec: v1.PodSpec{NodeName: "machine1"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS1}},
				{Spec: v1.PodSpec{NodeName: "machine2"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS2}},
				{Spec: v1.PodSpec{NodeName: "machine3"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS1}},
			},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "machine1", Labels: labelRgChina}},
				{ObjectMeta: metav1.ObjectMeta{Name: "machine2", Labels: labelRgIndia}},
				{ObjectMeta: metav1.ObjectMeta{Name: "machine3", Labels: labelAzAz1}},
			},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: framework.MaxNodeScore}, {Name: "machine2", Score: 0}, {Name: "machine3", Score: 0}},
			name: "Affinity: pod that matches topology key & pods in nodes will get high score comparing to others" +
				"which doesn't match either pods in nodes or in topology key",
		},
		// the node1(machine1) that have the label {"region": "China"} (match the topology key) and that have existing pods that match the labelSelector get high score
		// the node2(machine2) that have the label {"region": "China"}, match the topology key and have the same label value with node1, get the same high score with node1
		// the node3(machine3) that have the label {"region": "India"}, match the topology key but have a different label value, don't have existing pods that match the labelSelector,
		// get a low score.
		{
			pod: &v1.Pod{Spec: v1.PodSpec{NodeName: "", Affinity: stayWithS1InRegion}},
			pods: []*v1.Pod{
				{Spec: v1.PodSpec{NodeName: "machine1"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS1}},
			},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "machine1", Labels: labelRgChina}},
				{ObjectMeta: metav1.ObjectMeta{Name: "machine2", Labels: labelRgChinaAzAz1}},
				{ObjectMeta: metav1.ObjectMeta{Name: "machine3", Labels: labelRgIndia}},
			},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: framework.MaxNodeScore}, {Name: "machine2", Score: framework.MaxNodeScore}, {Name: "machine3", Score: 0}},
			name:         "All the nodes that have the same topology key & label value with one of them has an existing pod that match the affinity rules, have the same score",
		},
		// there are 2 regions, say regionChina(machine1,machine3,machine4) and regionIndia(machine2,machine5), both regions have nodes that match the preference.
		// But there are more nodes(actually more existing pods) in regionChina that match the preference than regionIndia.
		// Then, nodes in regionChina get higher score than nodes in regionIndia, and all the nodes in regionChina should get a same score(high score),
		// while all the nodes in regionIndia should get another same score(low score).
		{
			pod: &v1.Pod{Spec: v1.PodSpec{NodeName: "", Affinity: stayWithS2InRegion}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS1}},
			pods: []*v1.Pod{
				{Spec: v1.PodSpec{NodeName: "machine1"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS2}},
				{Spec: v1.PodSpec{NodeName: "machine1"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS2}},
				{Spec: v1.PodSpec{NodeName: "machine2"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS2}},
				{Spec: v1.PodSpec{NodeName: "machine3"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS2}},
				{Spec: v1.PodSpec{NodeName: "machine4"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS2}},
				{Spec: v1.PodSpec{NodeName: "machine5"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS2}},
			},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "machine1", Labels: labelRgChina}},
				{ObjectMeta: metav1.ObjectMeta{Name: "machine2", Labels: labelRgIndia}},
				{ObjectMeta: metav1.ObjectMeta{Name: "machine3", Labels: labelRgChina}},
				{ObjectMeta: metav1.ObjectMeta{Name: "machine4", Labels: labelRgChina}},
				{ObjectMeta: metav1.ObjectMeta{Name: "machine5", Labels: labelRgIndia}},
			},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: framework.MaxNodeScore}, {Name: "machine2", Score: 50}, {Name: "machine3", Score: framework.MaxNodeScore}, {Name: "machine4", Score: framework.MaxNodeScore}, {Name: "machine5", Score: 50}},
			name:         "Affinity: nodes in one region has more matching pods comparing to other reqion, so the region which has more macthes will get high score",
		},
		// Test with the different operators and values for pod affinity scheduling preference, including some match failures.
		{
			pod: &v1.Pod{Spec: v1.PodSpec{NodeName: "", Affinity: affinity3}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS1}},
			pods: []*v1.Pod{
				{Spec: v1.PodSpec{NodeName: "machine1"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS1}},
				{Spec: v1.PodSpec{NodeName: "machine2"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS2}},
				{Spec: v1.PodSpec{NodeName: "machine3"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS1}},
			},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "machine1", Labels: labelRgChina}},
				{ObjectMeta: metav1.ObjectMeta{Name: "machine2", Labels: labelRgIndia}},
				{ObjectMeta: metav1.ObjectMeta{Name: "machine3", Labels: labelAzAz1}},
			},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: 20}, {Name: "machine2", Score: framework.MaxNodeScore}, {Name: "machine3", Score: 0}},
			name:         "Affinity: different Label operators and values for pod affinity scheduling preference, including some match failures ",
		},
		// Test the symmetry cases for affinity, the difference between affinity and symmetry is not the pod wants to run together with some existing pods,
		// but the existing pods have the inter pod affinity preference while the pod to schedule satisfy the preference.
		{
			pod: &v1.Pod{Spec: v1.PodSpec{NodeName: ""}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS2}},
			pods: []*v1.Pod{
				{Spec: v1.PodSpec{NodeName: "machine1", Affinity: stayWithS1InRegion}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS1}},
				{Spec: v1.PodSpec{NodeName: "machine2", Affinity: stayWithS2InRegion}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS2}},
			},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "machine1", Labels: labelRgChina}},
				{ObjectMeta: metav1.ObjectMeta{Name: "machine2", Labels: labelRgIndia}},
				{ObjectMeta: metav1.ObjectMeta{Name: "machine3", Labels: labelAzAz1}},
			},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: 0}, {Name: "machine2", Score: framework.MaxNodeScore}, {Name: "machine3", Score: 0}},
			name:         "Affinity symmetry: considered only the preferredDuringSchedulingIgnoredDuringExecution in pod affinity symmetry",
		},
		{
			pod: &v1.Pod{Spec: v1.PodSpec{NodeName: ""}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS1}},
			pods: []*v1.Pod{
				{Spec: v1.PodSpec{NodeName: "machine1", Affinity: hardAffinity}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS1}},
				{Spec: v1.PodSpec{NodeName: "machine2", Affinity: hardAffinity}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS2}},
			},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "machine1", Labels: labelRgChina}},
				{ObjectMeta: metav1.ObjectMeta{Name: "machine2", Labels: labelRgIndia}},
				{ObjectMeta: metav1.ObjectMeta{Name: "machine3", Labels: labelAzAz1}},
			},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: framework.MaxNodeScore}, {Name: "machine2", Score: framework.MaxNodeScore}, {Name: "machine3", Score: 0}},
			name:         "Affinity symmetry: considered RequiredDuringSchedulingIgnoredDuringExecution in pod affinity symmetry",
		},

		// The pod to schedule prefer to stay away from some existing pods at node level using the pod anti affinity.
		// the nodes that have the label {"node": "bar"} (match the topology key) and that have existing pods that match the labelSelector get low score
		// the nodes that don't have the label {"node": "whatever the value is"} (mismatch the topology key) but that have existing pods that match the labelSelector get high score
		// the nodes that have the label {"node": "bar"} (match the topology key) but that have existing pods that mismatch the labelSelector get high score
		// there are 2 nodes, say node1 and node2, both nodes have pods that match the labelSelector and have topology-key in node.Labels.
		// But there are more pods on node1 that match the preference than node2. Then, node1 get a lower score than node2.
		{
			pod: &v1.Pod{Spec: v1.PodSpec{NodeName: "", Affinity: awayFromS1InAz}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS1}},
			pods: []*v1.Pod{
				{Spec: v1.PodSpec{NodeName: "machine1"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS1}},
				{Spec: v1.PodSpec{NodeName: "machine2"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS2}},
			},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "machine1", Labels: labelAzAz1}},
				{ObjectMeta: metav1.ObjectMeta{Name: "machine2", Labels: labelRgChina}},
			},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: 0}, {Name: "machine2", Score: framework.MaxNodeScore}},
			name:         "Anti Affinity: pod that doesnot match existing pods in node will get high score ",
		},
		{
			pod: &v1.Pod{Spec: v1.PodSpec{NodeName: "", Affinity: awayFromS1InAz}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS1}},
			pods: []*v1.Pod{
				{Spec: v1.PodSpec{NodeName: "machine1"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS1}},
				{Spec: v1.PodSpec{NodeName: "machine2"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS1}},
			},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "machine1", Labels: labelAzAz1}},
				{ObjectMeta: metav1.ObjectMeta{Name: "machine2", Labels: labelRgChina}},
			},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: 0}, {Name: "machine2", Score: framework.MaxNodeScore}},
			name:         "Anti Affinity: pod that does not matches topology key & matches the pods in nodes will get higher score comparing to others ",
		},
		{
			pod: &v1.Pod{Spec: v1.PodSpec{NodeName: "", Affinity: awayFromS1InAz}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS1}},
			pods: []*v1.Pod{
				{Spec: v1.PodSpec{NodeName: "machine1"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS1}},
				{Spec: v1.PodSpec{NodeName: "machine1"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS1}},
				{Spec: v1.PodSpec{NodeName: "machine2"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS2}},
			},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "machine1", Labels: labelAzAz1}},
				{ObjectMeta: metav1.ObjectMeta{Name: "machine2", Labels: labelRgIndia}},
			},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: 0}, {Name: "machine2", Score: framework.MaxNodeScore}},
			name:         "Anti Affinity: one node has more matching pods comparing to other node, so the node which has more unmacthes will get high score",
		},
		// Test the symmetry cases for anti affinity
		{
			pod: &v1.Pod{Spec: v1.PodSpec{NodeName: ""}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS2}},
			pods: []*v1.Pod{
				{Spec: v1.PodSpec{NodeName: "machine1", Affinity: awayFromS2InAz}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS1}},
				{Spec: v1.PodSpec{NodeName: "machine2", Affinity: awayFromS1InAz}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS2}},
			},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "machine1", Labels: labelAzAz1}},
				{ObjectMeta: metav1.ObjectMeta{Name: "machine2", Labels: labelAzAz2}},
			},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: 0}, {Name: "machine2", Score: framework.MaxNodeScore}},
			name:         "Anti Affinity symmetry: the existing pods in node which has anti affinity match will get high score",
		},
		// Test both  affinity and anti-affinity
		{
			pod: &v1.Pod{Spec: v1.PodSpec{NodeName: "", Affinity: stayWithS1InRegionAwayFromS2InAz}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS1}},
			pods: []*v1.Pod{
				{Spec: v1.PodSpec{NodeName: "machine1"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS1}},
				{Spec: v1.PodSpec{NodeName: "machine2"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS1}},
			},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "machine1", Labels: labelRgChina}},
				{ObjectMeta: metav1.ObjectMeta{Name: "machine2", Labels: labelAzAz1}},
			},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: framework.MaxNodeScore}, {Name: "machine2", Score: 0}},
			name:         "Affinity and Anti Affinity: considered only preferredDuringSchedulingIgnoredDuringExecution in both pod affinity & anti affinity",
		},
		// Combined cases considering both affinity and anti-affinity, the pod to schedule and existing pods have the same labels (they are in the same RC/service),
		// the pod prefer to run together with its brother pods in the same region, but wants to stay away from them at node level,
		// so that all the pods of a RC/service can stay in a same region but trying to separate with each other
		// machine-1,machine-3,machine-4 are in ChinaRegion others machin-2,machine-5 are in IndiaRegion
		{
			pod: &v1.Pod{Spec: v1.PodSpec{NodeName: "", Affinity: stayWithS1InRegionAwayFromS2InAz}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS1}},
			pods: []*v1.Pod{
				{Spec: v1.PodSpec{NodeName: "machine1"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS1}},
				{Spec: v1.PodSpec{NodeName: "machine1"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS1}},
				{Spec: v1.PodSpec{NodeName: "machine2"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS1}},
				{Spec: v1.PodSpec{NodeName: "machine3"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS1}},
				{Spec: v1.PodSpec{NodeName: "machine3"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS1}},
				{Spec: v1.PodSpec{NodeName: "machine4"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS1}},
				{Spec: v1.PodSpec{NodeName: "machine5"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS1}},
			},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "machine1", Labels: labelRgChinaAzAz1}},
				{ObjectMeta: metav1.ObjectMeta{Name: "machine2", Labels: labelRgIndia}},
				{ObjectMeta: metav1.ObjectMeta{Name: "machine3", Labels: labelRgChina}},
				{ObjectMeta: metav1.ObjectMeta{Name: "machine4", Labels: labelRgChina}},
				{ObjectMeta: metav1.ObjectMeta{Name: "machine5", Labels: labelRgIndia}},
			},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: framework.MaxNodeScore}, {Name: "machine2", Score: 40}, {Name: "machine3", Score: framework.MaxNodeScore}, {Name: "machine4", Score: framework.MaxNodeScore}, {Name: "machine5", Score: 40}},
			name:         "Affinity and Anti Affinity: considering both affinity and anti-affinity, the pod to schedule and existing pods have the same labels",
		},
		// Consider Affinity, Anti Affinity and symmetry together.
		// for Affinity, the weights are:                8,  0,  0,  0
		// for Anti Affinity, the weights are:           0, -5,  0,  0
		// for Affinity symmetry, the weights are:       0,  0,  8,  0
		// for Anti Affinity symmetry, the weights are:  0,  0,  0, -5
		{
			pod: &v1.Pod{Spec: v1.PodSpec{NodeName: "", Affinity: stayWithS1InRegionAwayFromS2InAz}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS1}},
			pods: []*v1.Pod{
				{Spec: v1.PodSpec{NodeName: "machine1"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS1}},
				{Spec: v1.PodSpec{NodeName: "machine2"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS2}},
				{Spec: v1.PodSpec{NodeName: "machine3", Affinity: stayWithS1InRegionAwayFromS2InAz}},
				{Spec: v1.PodSpec{NodeName: "machine4", Affinity: awayFromS1InAz}},
			},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "machine1", Labels: labelRgChina}},
				{ObjectMeta: metav1.ObjectMeta{Name: "machine2", Labels: labelAzAz1}},
				{ObjectMeta: metav1.ObjectMeta{Name: "machine3", Labels: labelRgIndia}},
				{ObjectMeta: metav1.ObjectMeta{Name: "machine4", Labels: labelAzAz2}},
			},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: framework.MaxNodeScore}, {Name: "machine2", Score: 0}, {Name: "machine3", Score: framework.MaxNodeScore}, {Name: "machine4", Score: 0}},
			name:         "Affinity and Anti Affinity and symmetry: considered only preferredDuringSchedulingIgnoredDuringExecution in both pod affinity & anti affinity & symmetry",
		},
		// Cover https://github.com/kubernetes/kubernetes/issues/82796 which panics upon:
		// 1. Some nodes in a topology don't have pods with affinity, but other nodes in the same topology have.
		// 2. The incoming pod doesn't have affinity.
		{
			pod: &v1.Pod{Spec: v1.PodSpec{NodeName: ""}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS1}},
			pods: []*v1.Pod{
				{Spec: v1.PodSpec{NodeName: "machine1"}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelSecurityS1}},
				{Spec: v1.PodSpec{NodeName: "machine2", Affinity: stayWithS1InRegionAwayFromS2InAz}},
			},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "machine1", Labels: labelRgChina}},
				{ObjectMeta: metav1.ObjectMeta{Name: "machine2", Labels: labelRgChina}},
			},
			expectedList: []framework.NodeScore{{Name: "machine1", Score: framework.MaxNodeScore}, {Name: "machine2", Score: framework.MaxNodeScore}},
			name:         "Avoid panic when partial nodes in a topology don't have pods with affinity",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := framework.NewCycleState()
			snapshot := nodeinfosnapshot.NewSnapshot(nodeinfosnapshot.CreateNodeInfoMap(test.pods, test.nodes))
			fh, _ := framework.NewFramework(nil, nil, nil, framework.WithSnapshotSharedLister(snapshot))

			client := clientsetfake.NewSimpleClientset()
			informerFactory := informers.NewSharedInformerFactory(client, 0)

			metaDataProducer := priorities.NewMetadataFactory(
				informerFactory.Core().V1().Services().Lister(),
				informerFactory.Core().V1().ReplicationControllers().Lister(),
				informerFactory.Apps().V1().ReplicaSets().Lister(),
				informerFactory.Apps().V1().StatefulSets().Lister(),
				1,
			)

			metaData := metaDataProducer(test.pod, test.nodes, snapshot)

			state.Write(migration.PrioritiesStateKey, &migration.PrioritiesStateData{Reference: metaData})

			p, _ := New(nil, fh)
			var gotList framework.NodeScoreList
			for _, n := range test.nodes {
				nodeName := n.ObjectMeta.Name
				score, status := p.(framework.ScorePlugin).Score(context.Background(), state, test.pod, nodeName)
				if !status.IsSuccess() {
					t.Errorf("unexpected error: %v", status)
				}
				gotList = append(gotList, framework.NodeScore{Name: nodeName, Score: score})
			}

			status := p.(framework.ScorePlugin).ScoreExtensions().NormalizeScore(context.Background(), state, test.pod, gotList)
			if !status.IsSuccess() {
				t.Errorf("unexpected error: %v", status)
			}

			if !reflect.DeepEqual(test.expectedList, gotList) {
				t.Errorf("expected:\n\t%+v,\ngot:\n\t%+v", test.expectedList, gotList)
			}

		})
	}
}

func TestHardPodAffinitySymmetricWeight(t *testing.T) {
	podLabelServiceS1 := map[string]string{
		"service": "S1",
	}
	labelRgChina := map[string]string{
		"region": "China",
	}
	labelRgIndia := map[string]string{
		"region": "India",
	}
	labelAzAz1 := map[string]string{
		"az": "az1",
	}
	hardPodAffinity := &v1.Affinity{
		PodAffinity: &v1.PodAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
				{
					LabelSelector: &metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "service",
								Operator: metav1.LabelSelectorOpIn,
								Values:   []string{"S1"},
							},
						},
					},
					TopologyKey: "region",
				},
			},
		},
	}
	tests := []struct {
		pod                   *v1.Pod
		pods                  []*v1.Pod
		nodes                 []*v1.Node
		hardPodAffinityWeight int32
		expectedList          framework.NodeScoreList
		name                  string
	}{
		{
			pod: &v1.Pod{Spec: v1.PodSpec{NodeName: ""}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelServiceS1}},
			pods: []*v1.Pod{
				{Spec: v1.PodSpec{NodeName: "machine1", Affinity: hardPodAffinity}},
				{Spec: v1.PodSpec{NodeName: "machine2", Affinity: hardPodAffinity}},
			},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "machine1", Labels: labelRgChina}},
				{ObjectMeta: metav1.ObjectMeta{Name: "machine2", Labels: labelRgIndia}},
				{ObjectMeta: metav1.ObjectMeta{Name: "machine3", Labels: labelAzAz1}},
			},
			hardPodAffinityWeight: v1.DefaultHardPodAffinitySymmetricWeight,
			expectedList:          []framework.NodeScore{{Name: "machine1", Score: framework.MaxNodeScore}, {Name: "machine2", Score: framework.MaxNodeScore}, {Name: "machine3", Score: 0}},
			name:                  "Hard Pod Affinity symmetry: hard pod affinity symmetry weights 1 by default, then nodes that match the hard pod affinity symmetry rules, get a high score",
		},
		{
			pod: &v1.Pod{Spec: v1.PodSpec{NodeName: ""}, ObjectMeta: metav1.ObjectMeta{Labels: podLabelServiceS1}},
			pods: []*v1.Pod{
				{Spec: v1.PodSpec{NodeName: "machine1", Affinity: hardPodAffinity}},
				{Spec: v1.PodSpec{NodeName: "machine2", Affinity: hardPodAffinity}},
			},
			nodes: []*v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "machine1", Labels: labelRgChina}},
				{ObjectMeta: metav1.ObjectMeta{Name: "machine2", Labels: labelRgIndia}},
				{ObjectMeta: metav1.ObjectMeta{Name: "machine3", Labels: labelAzAz1}},
			},
			hardPodAffinityWeight: 0,
			expectedList:          []framework.NodeScore{{Name: "machine1", Score: 0}, {Name: "machine2", Score: 0}, {Name: "machine3", Score: 0}},
			name:                  "Hard Pod Affinity symmetry: hard pod affinity symmetry is closed(weights 0), then nodes that match the hard pod affinity symmetry rules, get same score with those not match",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := framework.NewCycleState()
			snapshot := nodeinfosnapshot.NewSnapshot(nodeinfosnapshot.CreateNodeInfoMap(test.pods, test.nodes))
			fh, _ := framework.NewFramework(nil, nil, nil, framework.WithSnapshotSharedLister(snapshot))

			client := clientsetfake.NewSimpleClientset()
			informerFactory := informers.NewSharedInformerFactory(client, 0)

			metaDataProducer := priorities.NewMetadataFactory(
				informerFactory.Core().V1().Services().Lister(),
				informerFactory.Core().V1().ReplicationControllers().Lister(),
				informerFactory.Apps().V1().ReplicaSets().Lister(),
				informerFactory.Apps().V1().StatefulSets().Lister(),
				test.hardPodAffinityWeight,
			)

			metaData := metaDataProducer(test.pod, test.nodes, snapshot)

			state.Write(migration.PrioritiesStateKey, &migration.PrioritiesStateData{Reference: metaData})

			p, _ := New(nil, fh)
			var gotList framework.NodeScoreList
			for _, n := range test.nodes {
				nodeName := n.ObjectMeta.Name
				score, status := p.(framework.ScorePlugin).Score(context.Background(), state, test.pod, nodeName)
				if !status.IsSuccess() {
					t.Errorf("unexpected error: %v", status)
				}
				gotList = append(gotList, framework.NodeScore{Name: nodeName, Score: score})
			}

			status := p.(framework.ScorePlugin).ScoreExtensions().NormalizeScore(context.Background(), state, test.pod, gotList)
			if !status.IsSuccess() {
				t.Errorf("unexpected error: %v", status)
			}

			if !reflect.DeepEqual(test.expectedList, gotList) {
				t.Errorf("expected:\n\t%+v,\ngot:\n\t%+v", test.expectedList, gotList)
			}
		})
	}
}
