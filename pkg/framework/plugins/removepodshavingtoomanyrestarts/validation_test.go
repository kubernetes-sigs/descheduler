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
	"fmt"
	"testing"

	v1 "k8s.io/api/core/v1"
)

func TestValidateRemovePodsHavingTooManyRestartsArgs(t *testing.T) {
	testCases := []struct {
		description string
		args        *RemovePodsHavingTooManyRestartsArgs
		errInfo     error
	}{
		{
			description: "valid arg, no errors",
			args: &RemovePodsHavingTooManyRestartsArgs{
				PodRestartThreshold: 1,
				States:              []string{string(v1.PodRunning)},
			},
		},
		{
			description: "invalid PodRestartThreshold arg, expects errors",
			args: &RemovePodsHavingTooManyRestartsArgs{
				PodRestartThreshold: 0,
			},
			errInfo: fmt.Errorf(`invalid PodsHavingTooManyRestarts threshold`),
		},
		{
			description: "invalid States arg, expects errors",
			args: &RemovePodsHavingTooManyRestartsArgs{
				PodRestartThreshold: 1,
				States:              []string{string(v1.PodFailed)},
			},
			errInfo: fmt.Errorf(`states must be one of [CrashLoopBackOff Running]`),
		},
		{
			description: "allows CrashLoopBackOff state",
			args: &RemovePodsHavingTooManyRestartsArgs{
				PodRestartThreshold: 1,
				States:              []string{"CrashLoopBackOff"},
			},
		},
		{
			description: "invalid PodRestartThreshold arg and invalid States arg, expects errors",
			args: &RemovePodsHavingTooManyRestartsArgs{
				PodRestartThreshold: 0,
				States:              []string{string(v1.PodFailed)},
			},
			errInfo: fmt.Errorf(`[invalid PodsHavingTooManyRestarts threshold, states must be one of [CrashLoopBackOff Running]]`),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.description, func(t *testing.T) {
			validateErr := ValidateRemovePodsHavingTooManyRestartsArgs(testCase.args)
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
