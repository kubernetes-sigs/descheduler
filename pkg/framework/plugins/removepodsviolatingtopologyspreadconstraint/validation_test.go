package removepodsviolatingtopologyspreadconstraint

import (
	"fmt"
	"testing"

	v1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/descheduler/pkg/api"
)

func TestValidateRemovePodsViolatingTopologySpreadConstraintArgs(t *testing.T) {
	testCases := []struct {
		description string
		args        *RemovePodsViolatingTopologySpreadConstraintArgs
		errInfo     error
	}{
		{
			description: "valid namespace args, no errors",
			args: &RemovePodsViolatingTopologySpreadConstraintArgs{
				Namespaces: &api.Namespaces{
					Include: []string{"default"},
				},
			},
		},
		{
			description: "invalid namespaces args, expects error",
			args: &RemovePodsViolatingTopologySpreadConstraintArgs{
				Namespaces: &api.Namespaces{
					Include: []string{"default"},
					Exclude: []string{"kube-system"},
				},
			},
			errInfo: fmt.Errorf(`only one of Include/Exclude namespaces can be set`),
		},
		{
			description: "valid label selector args, no errors",
			args: &RemovePodsViolatingTopologySpreadConstraintArgs{
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"role.kubernetes.io/node": ""},
				},
			},
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
			errInfo: fmt.Errorf(`failed to get label selectors from strategy's params: [key: Invalid value: "": name part must be non-empty; name part must consist of alphanumeric characters, '-', '_' or '.', and must start and end with an alphanumeric character (e.g. 'MyName',  or 'my.name',  or '123-abc', regex used for validation is '([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]'), values: Invalid value: []string(nil): for 'in', 'notin' operators, values set can't be empty]`),
		},
		{
			description: "valid constraints args, no errors",
			args: &RemovePodsViolatingTopologySpreadConstraintArgs{
				Constraints: []v1.UnsatisfiableConstraintAction{v1.DoNotSchedule, v1.ScheduleAnyway},
			},
		},
		{
			description: "invalid constraints args, expects errors",
			args: &RemovePodsViolatingTopologySpreadConstraintArgs{
				Constraints: []v1.UnsatisfiableConstraintAction{"foo"},
			},
			errInfo: fmt.Errorf(`constraint foo is not one of map[DoNotSchedule:{} ScheduleAnyway:{}]`),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.description, func(t *testing.T) {
			validateErr := ValidateRemovePodsViolatingTopologySpreadConstraintArgs(testCase.args)
			if validateErr == nil || testCase.errInfo == nil {
				if validateErr != testCase.errInfo {
					t.Errorf("expected validity of plugin config: %q but got %q instead", testCase.errInfo, validateErr)
				}
			} else if validateErr.Error() != testCase.errInfo.Error() {
				t.Errorf("expected validity of plugin config: %q but got %q instead", testCase.errInfo, validateErr)
			}
		})
	}
}
