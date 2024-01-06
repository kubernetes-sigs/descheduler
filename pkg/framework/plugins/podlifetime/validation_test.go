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

package podlifetime

import (
	"testing"

	v1 "k8s.io/api/core/v1"
)

func TestValidateRemovePodLifeTimeArgs(t *testing.T) {
	testCases := []struct {
		description string
		args        *PodLifeTimeArgs
		expectError bool
	}{
		{
			description: "valid arg, no errors",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: func(i uint) *uint { return &i }(1),
				States:                []string{string(v1.PodRunning)},
			},
			expectError: false,
		},
		{
			description: "Pod Status Reasons CrashLoopBackOff ",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: func(i uint) *uint { return &i }(1),
				States:                []string{"CrashLoopBackOff"},
			},
			expectError: false,
		},
		{
			description: "nil MaxPodLifeTimeSeconds arg, expects errors",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: nil,
			},
			expectError: true,
		},
		{
			description: "invalid pod state arg, expects errors",
			args: &PodLifeTimeArgs{
				States: []string{string(v1.NodeRunning)},
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			err := ValidatePodLifeTimeArgs(tc.args)

			hasError := err != nil
			if tc.expectError != hasError {
				t.Error("unexpected arg validation behavior")
			}
		})
	}
}
