package utils

import (
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/descheduler/test"
)

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

func createNodeSelectorTerm(key, value string) v1.NodeSelectorTerm {
	return v1.NodeSelectorTerm{
		MatchExpressions: []v1.NodeSelectorRequirement{
			{
				Key:      key,
				Operator: "In",
				Values:   []string{value},
			},
		},
	}
}

func TestPodNodeAffinityWeight(t *testing.T) {
	defaultNode := v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"key1": "value1",
				"key2": "value2",
				"key3": "value3",
			},
		},
	}
	tests := []struct {
		name           string
		affinity       *v1.Affinity
		expectedWeight int32
	}{
		{
			name:           "No affinity",
			affinity:       nil,
			expectedWeight: 0,
		},
		{
			name:           "No node affinity",
			affinity:       &v1.Affinity{},
			expectedWeight: 0,
		},
		{
			name: "Empty preferred node affinity, but matching required node affinity",
			affinity: &v1.Affinity{
				NodeAffinity: &v1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
						NodeSelectorTerms: []v1.NodeSelectorTerm{
							createNodeSelectorTerm("key1", "value1"),
						},
					},
				},
			},
			expectedWeight: 0,
		},
		{
			name: "Matching single key in preferred node affinity",
			affinity: &v1.Affinity{
				NodeAffinity: &v1.NodeAffinity{
					PreferredDuringSchedulingIgnoredDuringExecution: []v1.PreferredSchedulingTerm{
						{
							Weight:     10,
							Preference: createNodeSelectorTerm("key1", "value1"),
						},
						{
							Weight:     5,
							Preference: createNodeSelectorTerm("key1", "valueX"),
						},
					},
				},
			},
			expectedWeight: 10,
		},
		{
			name: "Matching two keys in preferred node affinity",
			affinity: &v1.Affinity{
				NodeAffinity: &v1.NodeAffinity{
					PreferredDuringSchedulingIgnoredDuringExecution: []v1.PreferredSchedulingTerm{
						{
							Weight:     10,
							Preference: createNodeSelectorTerm("key1", "value1"),
						},
						{
							Weight:     5,
							Preference: createNodeSelectorTerm("key2", "value2"),
						},
					},
				},
			},
			expectedWeight: 15,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pod := v1.Pod{}
			pod.Spec.Affinity = test.affinity
			totalWeight, err := GetNodeWeightGivenPodPreferredAffinity(&pod, &defaultNode)
			if err != nil {
				t.Error("Found non nil error")
			}
			if totalWeight != test.expectedWeight {
				t.Errorf("Expected total weight is %v but actual total weight is %v", test.expectedWeight, totalWeight)
			}
		})
	}
}

func TestCheckPodsWithAntiAffinityExist(t *testing.T) {
	tests := []struct {
		name            string
		pod             *v1.Pod
		podsInNamespace map[string][]*v1.Pod
		nodeMap         map[string]*v1.Node
		affinity        *v1.PodAntiAffinity
		expMatch        bool
	}{
		{
			name: "found pod matching pod anti-affinity",
			pod:  test.PodWithPodAntiAffinity(test.BuildTestPod("p1", 1000, 1000, "node", nil), "foo", "bar"),
			podsInNamespace: map[string][]*v1.Pod{
				"default": {
					test.PodWithPodAntiAffinity(test.BuildTestPod("p2", 1000, 1000, "node", nil), "foo", "bar"),
				},
			},
			nodeMap: map[string]*v1.Node{
				"node": test.BuildTestNode("node", 64000, 128*1000*1000*1000, 2, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						"region": "main-region",
					}
				}),
			},
			expMatch: true,
		},
		{
			name: "no match with invalid label selector",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					Affinity: &v1.Affinity{
						PodAntiAffinity: &v1.PodAntiAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"wrong": "selector",
										},
									},
								},
							},
						},
					},
				},
			},
			podsInNamespace: map[string][]*v1.Pod{},
			nodeMap:         map[string]*v1.Node{},
			expMatch:        false,
		},
		{
			name: "no match if pod does not match terms namespace",
			pod: test.PodWithPodAntiAffinity(test.BuildTestPod("p1", 1000, 1000, "node", func(pod *v1.Pod) {
				pod.Namespace = "other"
			}), "foo", "bar"),
			podsInNamespace: map[string][]*v1.Pod{},
			nodeMap:         map[string]*v1.Node{},
			expMatch:        false,
		},
		{
			name: "found pod with matching anti-affinity on different node",
			pod:  test.PodWithPodAntiAffinity(test.BuildTestPod("current-pod", 1000, 1000, "current-node", nil), "foo", "bar"),
			podsInNamespace: map[string][]*v1.Pod{
				"default": {
					test.PodWithPodAntiAffinity(test.BuildTestPod("other-pod", 1000, 1000, "other-node", nil), "foo", "bar"),
				},
			},
			nodeMap: map[string]*v1.Node{
				"current-node": test.BuildTestNode("current-node", 64000, 128*1000*1000*1000, 2, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						"region": "main-region",
					}
				}),
				"other-node": test.BuildTestNode("other-node", 64000, 128*1000*1000*1000, 2, func(node *v1.Node) {
					node.ObjectMeta.Labels = map[string]string{
						"region": "main-region",
					}
				}),
			},
			expMatch: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if match := CheckPodsWithAntiAffinityExist(test.pod, test.podsInNamespace, test.nodeMap); match != test.expMatch {
				t.Errorf("exp %v got %v", test.expMatch, match)
			}
		})
	}
}
