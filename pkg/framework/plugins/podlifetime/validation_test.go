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
	"fmt"
	"testing"

	v1 "k8s.io/api/core/v1"
)

func TestValidateRemovePodLifeTimeArgs(t *testing.T) {
	testCases := []struct {
		description string
		args        *PodLifeTimeArgs
		errInfo     error
	}{
		{
			description: "valid arg, no errors",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: func(i uint) *uint { return &i }(1),
				States:                []string{string(v1.PodRunning)},
			},
		},
		{
			description: "Pod Status Reasons Succeeded or Failed",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: func(i uint) *uint { return &i }(1),
				States:                []string{string(v1.PodSucceeded), string(v1.PodFailed)},
			},
		},
		{
			description: "Pod Status Reasons CrashLoopBackOff ",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: func(i uint) *uint { return &i }(1),
				States:                []string{"CrashLoopBackOff"},
			},
		},
		{
			description: "nil MaxPodLifeTimeSeconds arg, expects errors",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: nil,
			},
			errInfo: fmt.Errorf("at least one filtering criterion must be specified (maxPodLifeTimeSeconds, states, conditions, or exitCodes)"),
		},
		{
			description: "invalid pod state arg, expects errors",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: func(i uint) *uint { return &i }(1),
				States:                []string{string("InvalidState")},
			},
			errInfo: fmt.Errorf("states must be one of [Completed ContainerCannotRun ContainerCreating CrashLoopBackOff CreateContainerConfigError CreateContainerError DeadlineExceeded ErrImagePull Error Evicted Failed ImagePullBackOff InvalidImageName NodeAffinity NodeLost OOMKilled Pending PodInitializing Running Shutdown StartError Succeeded UnexpectedAdmissionError Unknown]"),
		},
		{
			description: "nil MaxPodLifeTimeSeconds arg and invalid pod state arg, expects errors",
			args: &PodLifeTimeArgs{
				MaxPodLifeTimeSeconds: nil,
				States:                []string{string("InvalidState")},
			},
			errInfo: fmt.Errorf("states must be one of [Completed ContainerCannotRun ContainerCreating CrashLoopBackOff CreateContainerConfigError CreateContainerError DeadlineExceeded ErrImagePull Error Evicted Failed ImagePullBackOff InvalidImageName NodeAffinity NodeLost OOMKilled Pending PodInitializing Running Shutdown StartError Succeeded UnexpectedAdmissionError Unknown]"),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.description, func(t *testing.T) {
			validateErr := ValidatePodLifeTimeArgs(testCase.args)
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
