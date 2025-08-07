/*
Copyright 2022 The Kubernetes Authors.

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

package removepodsviolatingnodeaffinity

import (
	"fmt"
	"testing"

	"sigs.k8s.io/descheduler/pkg/api"
)

func TestValidateRemovePodsViolatingNodeAffinityArgs(t *testing.T) {
	testCases := []struct {
		description string
		args        *RemovePodsViolatingNodeAffinityArgs
		errInfo     error
	}{
		{
			description: "nil NodeAffinityType args, expects errors",
			args: &RemovePodsViolatingNodeAffinityArgs{
				NodeAffinityType: nil,
			},
			errInfo: fmt.Errorf(`nodeAffinityType needs to be set`),
		},
		{
			description: "empty NodeAffinityType args, expects errors",
			args: &RemovePodsViolatingNodeAffinityArgs{
				NodeAffinityType: []string{},
			},
			errInfo: fmt.Errorf(`nodeAffinityType needs to be set`),
		},
		{
			description: "valid NodeAffinityType args, no errors",
			args: &RemovePodsViolatingNodeAffinityArgs{
				NodeAffinityType: []string{"requiredDuringSchedulingIgnoredDuringExecution"},
			},
		},
		{
			description: "invalid namespaces args, expects error",
			args: &RemovePodsViolatingNodeAffinityArgs{
				NodeAffinityType: []string{"requiredDuringSchedulingIgnoredDuringExecution"},
				Namespaces: &api.Namespaces{
					Include: []string{"default"},
					Exclude: []string{"kube-system"},
				},
			},
			errInfo: fmt.Errorf(`only one of Include/Exclude namespaces can be set`),
		},
		{
			description: "nil NodeAffinityType args and invalid namespaces args, expects error",
			args: &RemovePodsViolatingNodeAffinityArgs{
				NodeAffinityType: []string{},
				Namespaces: &api.Namespaces{
					Include: []string{"default"},
					Exclude: []string{"kube-system"},
				},
			},
			errInfo: fmt.Errorf(`[nodeAffinityType needs to be set, only one of Include/Exclude namespaces can be set]`),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.description, func(t *testing.T) {
			validateErr := ValidateRemovePodsViolatingNodeAffinityArgs(testCase.args)
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
