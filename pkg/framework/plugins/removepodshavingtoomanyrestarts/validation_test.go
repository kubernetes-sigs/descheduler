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

package removepodshavingtoomanyrestarts

import (
	"testing"

	v1 "k8s.io/api/core/v1"
)

func TestValidateRemovePodsHavingTooManyRestartsArgs(t *testing.T) {
	testCases := []struct {
		description string
		args        *RemovePodsHavingTooManyRestartsArgs
		expectError bool
	}{
		{
			description: "valid arg, no errors",
			args: &RemovePodsHavingTooManyRestartsArgs{
				PodRestartThreshold: 1,
				States:              []string{string(v1.PodRunning)},
			},
			expectError: false,
		},
		{
			description: "invalid PodRestartThreshold arg, expects errors",
			args: &RemovePodsHavingTooManyRestartsArgs{
				PodRestartThreshold: 0,
			},
			expectError: true,
		},
		{
			description: "invalid States arg, expects errors",
			args: &RemovePodsHavingTooManyRestartsArgs{
				PodRestartThreshold: 1,
				States:              []string{string(v1.PodFailed)},
			},
			expectError: true,
		},
		{
			description: "allows CrashLoopBackOff state",
			args: &RemovePodsHavingTooManyRestartsArgs{
				PodRestartThreshold: 1,
				States:              []string{"CrashLoopBackOff"},
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			err := ValidateRemovePodsHavingTooManyRestartsArgs(tc.args)

			hasError := err != nil
			if tc.expectError != hasError {
				t.Error("unexpected arg validation behavior")
			}
		})
	}
}
