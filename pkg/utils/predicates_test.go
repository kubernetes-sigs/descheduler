package utils

import (
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodMatchesNodeLabels(t *testing.T) {
	tests := []struct {
		name                      string
		pod                       *v1.Pod
		node                      *v1.Node
		considerPreferredAffinity bool
		expectedResult            bool
	}{
		{
			name: "pod with node selector that matches node",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					NodeSelector: map[string]string{
						"kubernetes.io/hostname": "worker0",
					},
				},
			},
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"kubernetes.io/hostname": "worker0",
					},
				},
			},
			considerPreferredAffinity: false,
			expectedResult:            true,
		},
		{
			name: "pod with node selector that doesn't match the node",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					NodeSelector: map[string]string{
						"kubernetes.io/hostname": "worker1",
					},
				},
			},
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"kubernetes.io/hostname": "worker0",
					},
				},
			},
			considerPreferredAffinity: false,
			expectedResult:            false,
		},
		{
			name: "considerPreferredAffinity false, pod with required node affinity matches node, but preferred affinity don't",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					Affinity: &v1.Affinity{
						NodeAffinity: &v1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
								NodeSelectorTerms: []v1.NodeSelectorTerm{
									{
										MatchExpressions: []v1.NodeSelectorRequirement{
											{
												Key:      "failure-domain.beta.kubernetes.io/zone",
												Operator: "In",
												Values:   []string{"A"},
											},
										},
									},
								},
							},
							PreferredDuringSchedulingIgnoredDuringExecution: []v1.PreferredSchedulingTerm{
								{
									Preference: v1.NodeSelectorTerm{
										MatchExpressions: []v1.NodeSelectorRequirement{
											{
												Key:      "kubernetes.io/hostname",
												Operator: "In",
												Values:   []string{"worker1"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"kubernetes.io/hostname":                 "worker0",
						"failure-domain.beta.kubernetes.io/zone": "A",
					},
				},
			},
			considerPreferredAffinity: false,
			expectedResult:            true,
		},
		{
			name: "considerPreferredAffinity true, pod with required node affinity matches node, but preferred affinity don't",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					Affinity: &v1.Affinity{
						NodeAffinity: &v1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
								NodeSelectorTerms: []v1.NodeSelectorTerm{
									{
										MatchExpressions: []v1.NodeSelectorRequirement{
											{
												Key:      "failure-domain.beta.kubernetes.io/zone",
												Operator: "In",
												Values:   []string{"A"},
											},
										},
									},
								},
							},
							PreferredDuringSchedulingIgnoredDuringExecution: []v1.PreferredSchedulingTerm{
								{
									Preference: v1.NodeSelectorTerm{
										MatchExpressions: []v1.NodeSelectorRequirement{
											{
												Key:      "kubernetes.io/hostname",
												Operator: "In",
												Values:   []string{"worker1"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"kubernetes.io/hostname":                 "worker0",
						"failure-domain.beta.kubernetes.io/zone": "A",
					},
				},
			},
			considerPreferredAffinity: true,
			expectedResult:            false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := podMatchesNodeLabels(test.pod, test.node, test.considerPreferredAffinity)
			if result != test.expectedResult {
				if test.expectedResult {
					t.Errorf("the test expected a match between the pod %#v and the node %#v, but it didn't", test.pod, test.node)
				} else {
					t.Errorf("the test didn't expect a match between the pod %#v and the node %#v, but it did", test.pod, test.node)
				}
			}
		})
	}
}

func TestUniqueSortTolerations(t *testing.T) {
	tests := []struct {
		name                string
		tolerations         []v1.Toleration
		expectedTolerations []v1.Toleration
	}{
		{
			name: "sort by key",
			tolerations: []v1.Toleration{
				{
					Key:      "key2",
					Operator: v1.TolerationOpEqual,
					Value:    "value2",
					Effect:   v1.TaintEffectNoSchedule,
				},
				{
					Key:      "key3",
					Operator: v1.TolerationOpEqual,
					Value:    "value3",
					Effect:   v1.TaintEffectNoSchedule,
				},
				{
					Key:      "key1",
					Operator: v1.TolerationOpEqual,
					Value:    "value1",
					Effect:   v1.TaintEffectNoSchedule,
				},
			},
			expectedTolerations: []v1.Toleration{
				{
					Key:      "key1",
					Operator: v1.TolerationOpEqual,
					Value:    "value1",
					Effect:   v1.TaintEffectNoSchedule,
				},
				{
					Key:      "key2",
					Operator: v1.TolerationOpEqual,
					Value:    "value2",
					Effect:   v1.TaintEffectNoSchedule,
				},
				{
					Key:      "key3",
					Operator: v1.TolerationOpEqual,
					Value:    "value3",
					Effect:   v1.TaintEffectNoSchedule,
				},
			},
		},
		{
			name: "sort by value",
			tolerations: []v1.Toleration{
				{
					Key:      "key1",
					Operator: v1.TolerationOpEqual,
					Value:    "value2",
					Effect:   v1.TaintEffectNoSchedule,
				},
				{
					Key:      "key1",
					Operator: v1.TolerationOpEqual,
					Value:    "value3",
					Effect:   v1.TaintEffectNoSchedule,
				},
				{
					Key:      "key1",
					Operator: v1.TolerationOpEqual,
					Value:    "value1",
					Effect:   v1.TaintEffectNoSchedule,
				},
			},
			expectedTolerations: []v1.Toleration{
				{
					Key:      "key1",
					Operator: v1.TolerationOpEqual,
					Value:    "value1",
					Effect:   v1.TaintEffectNoSchedule,
				},
				{
					Key:      "key1",
					Operator: v1.TolerationOpEqual,
					Value:    "value2",
					Effect:   v1.TaintEffectNoSchedule,
				},
				{
					Key:      "key1",
					Operator: v1.TolerationOpEqual,
					Value:    "value3",
					Effect:   v1.TaintEffectNoSchedule,
				},
			},
		},
		{
			name: "sort by effect",
			tolerations: []v1.Toleration{
				{
					Key:      "key1",
					Operator: v1.TolerationOpEqual,
					Value:    "value1",
					Effect:   v1.TaintEffectNoSchedule,
				},
				{
					Key:      "key1",
					Operator: v1.TolerationOpEqual,
					Value:    "value1",
					Effect:   v1.TaintEffectNoExecute,
				},
				{
					Key:      "key1",
					Operator: v1.TolerationOpEqual,
					Value:    "value1",
					Effect:   v1.TaintEffectPreferNoSchedule,
				},
			},
			expectedTolerations: []v1.Toleration{
				{
					Key:      "key1",
					Operator: v1.TolerationOpEqual,
					Value:    "value1",
					Effect:   v1.TaintEffectNoExecute,
				},
				{
					Key:      "key1",
					Operator: v1.TolerationOpEqual,
					Value:    "value1",
					Effect:   v1.TaintEffectNoSchedule,
				},
				{
					Key:      "key1",
					Operator: v1.TolerationOpEqual,
					Value:    "value1",
					Effect:   v1.TaintEffectPreferNoSchedule,
				},
			},
		},
		{
			name: "sort unique",
			tolerations: []v1.Toleration{
				{
					Key:      "key1",
					Operator: v1.TolerationOpEqual,
					Value:    "value1",
					Effect:   v1.TaintEffectNoSchedule,
				},
				{
					Key:      "key2",
					Operator: v1.TolerationOpEqual,
					Value:    "value2",
					Effect:   v1.TaintEffectNoSchedule,
				},
				{
					Key:      "key2",
					Operator: v1.TolerationOpEqual,
					Value:    "value2",
					Effect:   v1.TaintEffectNoSchedule,
				},
			},
			expectedTolerations: []v1.Toleration{
				{
					Key:      "key1",
					Operator: v1.TolerationOpEqual,
					Value:    "value1",
					Effect:   v1.TaintEffectNoSchedule,
				},
				{
					Key:      "key2",
					Operator: v1.TolerationOpEqual,
					Value:    "value2",
					Effect:   v1.TaintEffectNoSchedule,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resultTolerations := uniqueSortTolerations(test.tolerations)
			if !reflect.DeepEqual(resultTolerations, test.expectedTolerations) {
				t.Errorf("tolerations not sorted as expected, \n\tgot: %#v, \n\texpected: %#v", resultTolerations, test.expectedTolerations)
			}
		})
	}
}

func TestTolerationsEqual(t *testing.T) {
	tests := []struct {
		name                              string
		leftTolerations, rightTolerations []v1.Toleration
		equal                             bool
	}{
		{
			name: "identical lists",
			leftTolerations: []v1.Toleration{
				{
					Key:      "key1",
					Operator: v1.TolerationOpEqual,
					Value:    "value1",
					Effect:   v1.TaintEffectNoSchedule,
				},
				{
					Key:      "key2",
					Operator: v1.TolerationOpEqual,
					Value:    "value2",
					Effect:   v1.TaintEffectNoSchedule,
				},
			},
			rightTolerations: []v1.Toleration{
				{
					Key:      "key1",
					Operator: v1.TolerationOpEqual,
					Value:    "value1",
					Effect:   v1.TaintEffectNoSchedule,
				},
				{
					Key:      "key2",
					Operator: v1.TolerationOpEqual,
					Value:    "value2",
					Effect:   v1.TaintEffectNoSchedule,
				},
			},
			equal: true,
		},
		{
			name: "equal lists",
			leftTolerations: []v1.Toleration{
				{
					Key:      "key1",
					Operator: v1.TolerationOpEqual,
					Value:    "value1",
					Effect:   v1.TaintEffectNoSchedule,
				},
				{
					Key:      "key2",
					Operator: v1.TolerationOpEqual,
					Value:    "value2",
					Effect:   v1.TaintEffectNoSchedule,
				},
				{
					Key:      "key2",
					Operator: v1.TolerationOpEqual,
					Value:    "value2",
					Effect:   v1.TaintEffectNoSchedule,
				},
			},
			rightTolerations: []v1.Toleration{
				{
					Key:      "key1",
					Operator: v1.TolerationOpEqual,
					Value:    "value1",
					Effect:   v1.TaintEffectNoSchedule,
				},
				{
					Key:      "key2",
					Operator: v1.TolerationOpEqual,
					Value:    "value2",
					Effect:   v1.TaintEffectNoSchedule,
				},
			},
			equal: true,
		},
		{
			name: "non-equal lists",
			leftTolerations: []v1.Toleration{
				{
					Key:      "key1",
					Operator: v1.TolerationOpEqual,
					Value:    "value1",
					Effect:   v1.TaintEffectNoSchedule,
				},
				{
					Key:      "key2",
					Operator: v1.TolerationOpEqual,
					Value:    "value2",
					Effect:   v1.TaintEffectNoSchedule,
				},
			},
			rightTolerations: []v1.Toleration{
				{
					Key:      "key1",
					Operator: v1.TolerationOpEqual,
					Value:    "value1",
					Effect:   v1.TaintEffectNoSchedule,
				},
				{
					Key:      "key2",
					Operator: v1.TolerationOpEqual,
					Value:    "value2",
					Effect:   v1.TaintEffectNoExecute,
				},
			},
			equal: false,
		},
		{
			name: "different sizes lists",
			leftTolerations: []v1.Toleration{
				{
					Key:      "key1",
					Operator: v1.TolerationOpEqual,
					Value:    "value1",
					Effect:   v1.TaintEffectNoSchedule,
				},
			},
			rightTolerations: []v1.Toleration{
				{
					Key:      "key1",
					Operator: v1.TolerationOpEqual,
					Value:    "value1",
					Effect:   v1.TaintEffectNoSchedule,
				},
				{
					Key:      "key2",
					Operator: v1.TolerationOpEqual,
					Value:    "value2",
					Effect:   v1.TaintEffectNoExecute,
				},
			},
			equal: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			equal := TolerationsEqual(test.leftTolerations, test.rightTolerations)
			if equal != test.equal {
				t.Errorf("TolerationsEqual expected to be %v, got %v", test.equal, equal)
			}
		})
	}
}

func TestUniqueSortNodeSelectorRequirements(t *testing.T) {
	tests := []struct {
		name                 string
		requirements         []v1.NodeSelectorRequirement
		expectedRequirements []v1.NodeSelectorRequirement
	}{
		{
			name: "Identical requirements",
			requirements: []v1.NodeSelectorRequirement{
				{
					Key:      "k1",
					Operator: v1.NodeSelectorOpIn,
					Values:   []string{"v1"},
				},
				{
					Key:      "k1",
					Operator: v1.NodeSelectorOpIn,
					Values:   []string{"v1"},
				},
			},
			expectedRequirements: []v1.NodeSelectorRequirement{
				{
					Key:      "k1",
					Operator: v1.NodeSelectorOpIn,
					Values:   []string{"v1"},
				},
			},
		},
		{
			name: "Sorted requirements",
			requirements: []v1.NodeSelectorRequirement{
				{
					Key:      "k1",
					Operator: v1.NodeSelectorOpIn,
					Values:   []string{"v1"},
				},
				{
					Key:      "k2",
					Operator: v1.NodeSelectorOpIn,
					Values:   []string{"v2"},
				},
			},
			expectedRequirements: []v1.NodeSelectorRequirement{
				{
					Key:      "k1",
					Operator: v1.NodeSelectorOpIn,
					Values:   []string{"v1"},
				},
				{
					Key:      "k2",
					Operator: v1.NodeSelectorOpIn,
					Values:   []string{"v2"},
				},
			},
		},
		{
			name: "Sort values",
			requirements: []v1.NodeSelectorRequirement{
				{
					Key:      "k1",
					Operator: v1.NodeSelectorOpIn,
					Values:   []string{"v2", "v1"},
				},
			},
			expectedRequirements: []v1.NodeSelectorRequirement{
				{
					Key:      "k1",
					Operator: v1.NodeSelectorOpIn,
					Values:   []string{"v1", "v2"},
				},
			},
		},
		{
			name: "Sort by key",
			requirements: []v1.NodeSelectorRequirement{
				{
					Key:      "k3",
					Operator: v1.NodeSelectorOpIn,
					Values:   []string{"v1", "v2"},
				},
				{
					Key:      "k2",
					Operator: v1.NodeSelectorOpIn,
					Values:   []string{"v1", "v2"},
				},
				{
					Key:      "k1",
					Operator: v1.NodeSelectorOpIn,
					Values:   []string{"v1", "v2"},
				},
			},
			expectedRequirements: []v1.NodeSelectorRequirement{
				{
					Key:      "k1",
					Operator: v1.NodeSelectorOpIn,
					Values:   []string{"v1", "v2"},
				},
				{
					Key:      "k2",
					Operator: v1.NodeSelectorOpIn,
					Values:   []string{"v1", "v2"},
				},
				{
					Key:      "k3",
					Operator: v1.NodeSelectorOpIn,
					Values:   []string{"v1", "v2"},
				},
			},
		},
		{
			name: "Sort by operator",
			requirements: []v1.NodeSelectorRequirement{
				{
					Key:      "k1",
					Operator: v1.NodeSelectorOpIn,
					Values:   []string{"v1", "v2"},
				},
				{
					Key:      "k1",
					Operator: v1.NodeSelectorOpExists,
					Values:   []string{"v1", "v2"},
				},
				{
					Key:      "k1",
					Operator: v1.NodeSelectorOpGt,
					Values:   []string{"v1", "v2"},
				},
			},
			expectedRequirements: []v1.NodeSelectorRequirement{
				{
					Key:      "k1",
					Operator: v1.NodeSelectorOpExists,
					Values:   []string{"v1", "v2"},
				},
				{
					Key:      "k1",
					Operator: v1.NodeSelectorOpGt,
					Values:   []string{"v1", "v2"},
				},
				{
					Key:      "k1",
					Operator: v1.NodeSelectorOpIn,
					Values:   []string{"v1", "v2"},
				},
			},
		},
		{
			name: "Sort by values",
			requirements: []v1.NodeSelectorRequirement{
				{
					Key:      "k1",
					Operator: v1.NodeSelectorOpIn,
					Values:   []string{"v6", "v5"},
				},
				{
					Key:      "k1",
					Operator: v1.NodeSelectorOpIn,
					Values:   []string{"v2", "v1"},
				},
				{
					Key:      "k1",
					Operator: v1.NodeSelectorOpIn,
					Values:   []string{"v4", "v1"},
				},
			},
			expectedRequirements: []v1.NodeSelectorRequirement{
				{
					Key:      "k1",
					Operator: v1.NodeSelectorOpIn,
					Values:   []string{"v1", "v2"},
				},
				{
					Key:      "k1",
					Operator: v1.NodeSelectorOpIn,
					Values:   []string{"v1", "v4"},
				},
				{
					Key:      "k1",
					Operator: v1.NodeSelectorOpIn,
					Values:   []string{"v5", "v6"},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resultRequirements := uniqueSortNodeSelectorRequirements(test.requirements)
			if !reflect.DeepEqual(resultRequirements, test.expectedRequirements) {
				t.Errorf("Requirements not sorted as expected, \n\tgot: %#v, \n\texpected: %#v", resultRequirements, test.expectedRequirements)
			}
		})
	}
}

func TestUniqueSortNodeSelectorTerms(t *testing.T) {
	tests := []struct {
		name          string
		terms         []v1.NodeSelectorTerm
		expectedTerms []v1.NodeSelectorTerm
	}{
		{
			name: "Identical terms",
			terms: []v1.NodeSelectorTerm{
				{
					MatchExpressions: []v1.NodeSelectorRequirement{
						{
							Key:      "k1",
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"v1"},
						},
						{
							Key:      "k1",
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"v1"},
						},
					},
					MatchFields: []v1.NodeSelectorRequirement{
						{
							Key:      "k1",
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"v1"},
						},
						{
							Key:      "k1",
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"v1"},
						},
					},
				},
			},
			expectedTerms: []v1.NodeSelectorTerm{
				{
					MatchExpressions: []v1.NodeSelectorRequirement{
						{
							Key:      "k1",
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"v1"},
						},
					},
					MatchFields: []v1.NodeSelectorRequirement{
						{
							Key:      "k1",
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"v1"},
						},
					},
				},
			},
		},
		{
			name: "Sorted terms",
			terms: []v1.NodeSelectorTerm{
				{
					MatchExpressions: []v1.NodeSelectorRequirement{
						{
							Key:      "k1",
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"v1"},
						},
						{
							Key:      "k2",
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"v1"},
						},
					},
					MatchFields: []v1.NodeSelectorRequirement{
						{
							Key:      "k1",
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"v1"},
						},
						{
							Key:      "k2",
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"v1"},
						},
					},
				},
			},
			expectedTerms: []v1.NodeSelectorTerm{
				{
					MatchExpressions: []v1.NodeSelectorRequirement{
						{
							Key:      "k1",
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"v1"},
						},
						{
							Key:      "k2",
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"v1"},
						},
					},
					MatchFields: []v1.NodeSelectorRequirement{
						{
							Key:      "k1",
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"v1"},
						},
						{
							Key:      "k2",
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"v1"},
						},
					},
				},
			},
		},
		{
			name: "Sort terms",
			terms: []v1.NodeSelectorTerm{
				{
					MatchExpressions: []v1.NodeSelectorRequirement{
						{
							Key:      "k2",
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"v1"},
						},
						{
							Key:      "k1",
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"v2", "v1"},
						},
						{
							Key:      "k1",
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"v3", "v1"},
						},
					},
				},
			},
			expectedTerms: []v1.NodeSelectorTerm{
				{
					MatchExpressions: []v1.NodeSelectorRequirement{
						{
							Key:      "k1",
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"v1", "v2"},
						},
						{
							Key:      "k1",
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"v1", "v3"},
						},
						{
							Key:      "k2",
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"v1"},
						},
					},
				},
			},
		},
		{
			name: "Unique sort terms",
			terms: []v1.NodeSelectorTerm{
				{
					MatchExpressions: []v1.NodeSelectorRequirement{
						{
							Key:      "k1",
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"v2", "v1"},
						},
						{
							Key:      "k2",
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"v1"},
						},
						{
							Key:      "k1",
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"v2", "v1"},
						},
					},
				},
				{
					MatchExpressions: []v1.NodeSelectorRequirement{
						{
							Key:      "k2",
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"v1"},
						},
						{
							Key:      "k1",
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"v2", "v1"},
						},
					},
				},
			},
			expectedTerms: []v1.NodeSelectorTerm{
				{
					MatchExpressions: []v1.NodeSelectorRequirement{
						{
							Key:      "k1",
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"v1", "v2"},
						},
						{
							Key:      "k2",
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"v1"},
						},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resultTerms := uniqueSortNodeSelectorTerms(test.terms)
			if !reflect.DeepEqual(resultTerms, test.expectedTerms) {
				t.Errorf("Terms not sorted as expected, \n\tgot: %#v, \n\texpected: %#v", resultTerms, test.expectedTerms)
			}
		})
	}
}

func TestNodeSelectorTermsEqual(t *testing.T) {
	tests := []struct {
		name                        string
		leftSelector, rightSelector v1.NodeSelector
		equal                       bool
	}{
		{
			name: "identical selectors",
			leftSelector: v1.NodeSelector{
				NodeSelectorTerms: []v1.NodeSelectorTerm{
					{
						MatchExpressions: []v1.NodeSelectorRequirement{
							{
								Key:      "k1",
								Operator: v1.NodeSelectorOpIn,
								Values:   []string{"v1", "v2"},
							},
							{
								Key:      "k2",
								Operator: v1.NodeSelectorOpIn,
								Values:   []string{"v1"},
							},
						},
					},
				},
			},
			rightSelector: v1.NodeSelector{
				NodeSelectorTerms: []v1.NodeSelectorTerm{
					{
						MatchExpressions: []v1.NodeSelectorRequirement{
							{
								Key:      "k1",
								Operator: v1.NodeSelectorOpIn,
								Values:   []string{"v1", "v2"},
							},
							{
								Key:      "k2",
								Operator: v1.NodeSelectorOpIn,
								Values:   []string{"v1"},
							},
						},
					},
				},
			},
			equal: true,
		},
		{
			name: "equal selectors",
			leftSelector: v1.NodeSelector{
				NodeSelectorTerms: []v1.NodeSelectorTerm{
					{
						MatchExpressions: []v1.NodeSelectorRequirement{
							{
								Key:      "k2",
								Operator: v1.NodeSelectorOpIn,
								Values:   []string{"v1", "v1"},
							},
							{
								Key:      "k1",
								Operator: v1.NodeSelectorOpIn,
								Values:   []string{"v1", "v2"},
							},
						},
					},
				},
			},
			rightSelector: v1.NodeSelector{
				NodeSelectorTerms: []v1.NodeSelectorTerm{
					{
						MatchExpressions: []v1.NodeSelectorRequirement{
							{
								Key:      "k1",
								Operator: v1.NodeSelectorOpIn,
								Values:   []string{"v1", "v2"},
							},
							{
								Key:      "k2",
								Operator: v1.NodeSelectorOpIn,
								Values:   []string{"v1"},
							},
						},
					},
				},
			},
			equal: true,
		},
		{
			name: "non-equal selectors in values",
			leftSelector: v1.NodeSelector{
				NodeSelectorTerms: []v1.NodeSelectorTerm{
					{
						MatchExpressions: []v1.NodeSelectorRequirement{
							{
								Key:      "k1",
								Operator: v1.NodeSelectorOpIn,
								Values:   []string{"v1", "v2"},
							},
							{
								Key:      "k2",
								Operator: v1.NodeSelectorOpIn,
								Values:   []string{"v1"},
							},
						},
					},
				},
			},
			rightSelector: v1.NodeSelector{
				NodeSelectorTerms: []v1.NodeSelectorTerm{
					{
						MatchExpressions: []v1.NodeSelectorRequirement{
							{
								Key:      "k1",
								Operator: v1.NodeSelectorOpIn,
								Values:   []string{"v1", "v2"},
							},
							{
								Key:      "k2",
								Operator: v1.NodeSelectorOpIn,
								Values:   []string{"v1", "v2"},
							},
						},
					},
				},
			},
			equal: false,
		},
		{
			name: "non-equal selectors in keys",
			leftSelector: v1.NodeSelector{
				NodeSelectorTerms: []v1.NodeSelectorTerm{
					{
						MatchExpressions: []v1.NodeSelectorRequirement{
							{
								Key:      "k3",
								Operator: v1.NodeSelectorOpIn,
								Values:   []string{"v1"},
							},
							{
								Key:      "k1",
								Operator: v1.NodeSelectorOpIn,
								Values:   []string{"v1", "v2"},
							},
							{
								Key:      "k2",
								Operator: v1.NodeSelectorOpIn,
								Values:   []string{"v1"},
							},
						},
					},
				},
			},
			rightSelector: v1.NodeSelector{
				NodeSelectorTerms: []v1.NodeSelectorTerm{
					{
						MatchExpressions: []v1.NodeSelectorRequirement{
							{
								Key:      "k1",
								Operator: v1.NodeSelectorOpIn,
								Values:   []string{"v1", "v2"},
							},
							{
								Key:      "k2",
								Operator: v1.NodeSelectorOpIn,
								Values:   []string{"v1", "v2"},
							},
						},
					},
				},
			},
			equal: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			equal := NodeSelectorsEqual(&test.leftSelector, &test.rightSelector)
			if equal != test.equal {
				t.Errorf("NodeSelectorsEqual expected to be %v, got %v", test.equal, equal)
			}
		})
	}
}
