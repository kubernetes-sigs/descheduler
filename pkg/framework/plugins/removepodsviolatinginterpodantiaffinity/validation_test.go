package removepodsviolatinginterpodantiaffinity

import (
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/descheduler/pkg/api"
)

func TestValidateRemovePodsViolatingInterPodAntiAffinityArgs(t *testing.T) {
	testCases := []struct {
		description string
		args        *RemovePodsViolatingInterPodAntiAffinityArgs
		errInfo     error
	}{
		{
			description: "valid namespace args, no errors",
			args: &RemovePodsViolatingInterPodAntiAffinityArgs{
				Namespaces: &api.Namespaces{
					Include: []string{"default"},
				},
			},
		},
		{
			description: "invalid namespaces args, expects error",
			args: &RemovePodsViolatingInterPodAntiAffinityArgs{
				Namespaces: &api.Namespaces{
					Include: []string{"default"},
					Exclude: []string{"kube-system"},
				},
			},
			errInfo: fmt.Errorf(`only one of Include/Exclude namespaces can be set`),
		},
		{
			description: "valid label selector args, no errors",
			args: &RemovePodsViolatingInterPodAntiAffinityArgs{
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"role.kubernetes.io/node": ""},
				},
			},
		},
		{
			description: "invalid label selector args, expects errors",
			args: &RemovePodsViolatingInterPodAntiAffinityArgs{
				LabelSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Operator: metav1.LabelSelectorOpIn,
						},
					},
				},
			},
			errInfo: fmt.Errorf(`failed to get label selectors from strategy's params: [key: Invalid value: "": name part must be non-empty; name part must consist of alphanumeric characters, '-', '_' or '.', and must start and end with an alphanumeric character (e.g. 'MyName',  or 'my.name',  or '123-abc', regex used for validation is '([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]'), values: Invalid value: null: for 'in', 'notin' operators, values set can't be empty]`),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.description, func(t *testing.T) {
			validateErr := ValidateRemovePodsViolatingInterPodAntiAffinityArgs(testCase.args)
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
