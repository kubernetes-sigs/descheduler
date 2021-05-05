package utils

import (
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
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
