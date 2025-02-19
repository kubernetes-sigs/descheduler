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
	"context"
	"testing"

	"k8s.io/klog/v2"
)

func TestValidateRemovePodsViolatingNodeAffinityArgs(t *testing.T) {
	testCases := []struct {
		description string
		args        *RemovePodsViolatingNodeAffinityArgs
		expectError bool
	}{
		{
			description: "nil NodeAffinityType args, expects errors",
			args: &RemovePodsViolatingNodeAffinityArgs{
				NodeAffinityType: nil,
			},
			expectError: true,
		},
		{
			description: "empty NodeAffinityType args, expects errors",
			args: &RemovePodsViolatingNodeAffinityArgs{
				NodeAffinityType: []string{},
			},
			expectError: true,
		},
		{
			description: "valid NodeAffinityType args, no errors",
			args: &RemovePodsViolatingNodeAffinityArgs{
				NodeAffinityType: []string{"requiredDuringSchedulingIgnoredDuringExecution"},
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			err := ValidateRemovePodsViolatingNodeAffinityArgs(klog.FromContext(context.Background()), tc.args)

			hasError := err != nil
			if tc.expectError != hasError {
				t.Error("unexpected arg validation behavior")
			}
		})
	}
}
