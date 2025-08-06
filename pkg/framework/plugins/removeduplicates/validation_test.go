package removeduplicates

import (
	"fmt"
	"testing"

	"sigs.k8s.io/descheduler/pkg/api"
)

func TestValidateRemovePodsViolatingNodeTaintsArgs(t *testing.T) {
	testCases := []struct {
		description string
		args        *RemoveDuplicatesArgs
		expectError bool
		errInfo     error
	}{
		{
			description: "valid namespace args, no errors",
			args: &RemoveDuplicatesArgs{
				ExcludeOwnerKinds: []string{"Job"},
				Namespaces: &api.Namespaces{
					Include: []string{"default"},
				},
			},
		},
		{
			description: "invalid namespaces args, expects error",
			args: &RemoveDuplicatesArgs{
				ExcludeOwnerKinds: []string{"Job"},
				Namespaces: &api.Namespaces{
					Include: []string{"default"},
					Exclude: []string{"kube-system"},
				},
			},
			errInfo: fmt.Errorf("only one of Include/Exclude namespaces can be set"),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.description, func(t *testing.T) {
			validateErr := ValidateRemoveDuplicatesArgs(testCase.args)
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
