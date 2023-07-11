package removepodsviolatingtopologyspreadconstraint

import (
	"testing"

	v1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/descheduler/pkg/api"
)

func TestValidateRemovePodsViolatingTopologySpreadConstraintArgs(t *testing.T) {
	testCases := []struct {
		description string
		args        *RemovePodsViolatingTopologySpreadConstraintArgs
		expectError bool
	}{
		{
			description: "valid namespace args, no errors",
			args: &RemovePodsViolatingTopologySpreadConstraintArgs{
				Namespaces: &api.Namespaces{
					Include: []string{"default"},
				},
			},
			expectError: false,
		},
		{
			description: "invalid namespaces args, expects error",
			args: &RemovePodsViolatingTopologySpreadConstraintArgs{
				Namespaces: &api.Namespaces{
					Include: []string{"default"},
					Exclude: []string{"kube-system"},
				},
			},
			expectError: true,
		},
		{
			description: "valid label selector args, no errors",
			args: &RemovePodsViolatingTopologySpreadConstraintArgs{
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"role.kubernetes.io/node": ""},
				},
			},
			expectError: false,
		},
		{
			description: "invalid label selector args, expects errors",
			args: &RemovePodsViolatingTopologySpreadConstraintArgs{
				LabelSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Operator: metav1.LabelSelectorOpIn,
						},
					},
				},
			},
			expectError: true,
		},
		{
			description: "valid constraints args, no errors",
			args: &RemovePodsViolatingTopologySpreadConstraintArgs{
				Constraints: []v1.UnsatisfiableConstraintAction{v1.DoNotSchedule, v1.ScheduleAnyway},
			},
			expectError: false,
		},
		{
			description: "invalid constraints args, expects errors",
			args: &RemovePodsViolatingTopologySpreadConstraintArgs{
				Constraints: []v1.UnsatisfiableConstraintAction{"foo"},
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			err := ValidateRemovePodsViolatingTopologySpreadConstraintArgs(tc.args)

			hasError := err != nil
			if tc.expectError != hasError {
				t.Error("unexpected arg validation behavior")
			}
		})
	}
}
