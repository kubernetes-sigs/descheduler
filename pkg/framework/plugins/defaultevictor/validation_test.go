/*
Copyright 2024 The Kubernetes Authors.

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

package defaultevictor

import (
	"fmt"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	utilptr "k8s.io/utils/ptr"
	"sigs.k8s.io/descheduler/pkg/api"
)

func TestValidateDefaultEvictorArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    *DefaultEvictorArgs
		errInfo error
	}{
		{
			name: "passing invalid priority",
			args: &DefaultEvictorArgs{
				PriorityThreshold: &api.PriorityThreshold{
					Value: utilptr.To[int32](1),
					Name:  "priority-name",
				},
			},
			errInfo: fmt.Errorf("priority threshold misconfigured, only one of priorityThreshold fields can be set"),
		},
		{
			name: "passing invalid no eviction policy",
			args: &DefaultEvictorArgs{
				NoEvictionPolicy: "invalid-no-eviction-policy",
			},
			errInfo: fmt.Errorf("noEvictionPolicy accepts only %q values", []NoEvictionPolicy{PreferredNoEvictionPolicy, MandatoryNoEvictionPolicy}),
		},
		{
			name: "Valid configuration with no deprecated fields",
			args: &DefaultEvictorArgs{
				PodProtections: PodProtections{
					DefaultDisabled: []PodProtection{},
					ExtraEnabled:    []PodProtection{},
				},
			},
			errInfo: nil,
		},
		{
			name: "Valid configuration: both Disabled and ExtraEnabled",
			args: &DefaultEvictorArgs{
				PodProtections: PodProtections{
					DefaultDisabled: []PodProtection{
						DaemonSetPods,
						PodsWithLocalStorage,
					},
					ExtraEnabled: []PodProtection{
						PodsWithPVC,
					},
				},
			},
			errInfo: nil,
		},
		{
			name: "Valid configuration with ExtraEnabled",
			args: &DefaultEvictorArgs{
				PodProtections: PodProtections{
					ExtraEnabled: []PodProtection{
						PodsWithPVC,
					},
				},
			},
			errInfo: nil,
		},
		{
			name: "Invalid configuration: Deprecated field used with Disabled",
			args: &DefaultEvictorArgs{
				EvictLocalStoragePods: true,
				PodProtections: PodProtections{
					DefaultDisabled: []PodProtection{
						DaemonSetPods,
					},
				},
			},
			errInfo: fmt.Errorf("cannot use Deprecated fields alongside PodProtections.ExtraEnabled or PodProtections.DefaultDisabled"),
		},
		{
			name: "Invalid configuration: Deprecated field used with ExtraPodProtections",
			args: &DefaultEvictorArgs{
				EvictDaemonSetPods: true,
				PodProtections: PodProtections{
					ExtraEnabled: []PodProtection{
						PodsWithPVC,
					},
				},
			},
			errInfo: fmt.Errorf("cannot use Deprecated fields alongside PodProtections.ExtraEnabled or PodProtections.DefaultDisabled"),
		},
		{
			name: "MinReplicas warning logged but no error",
			args: &DefaultEvictorArgs{
				MinReplicas: 1,
			},
			errInfo: nil,
		},
		{
			name: "Invalid ExtraEnabled: Unknown policy",
			args: &DefaultEvictorArgs{
				PodProtections: PodProtections{
					ExtraEnabled: []PodProtection{"InvalidPolicy"},
				},
			},
			errInfo: fmt.Errorf(`invalid pod protection policy in ExtraEnabled: "InvalidPolicy". Valid options are: [PodsWithPVC PodsWithoutPDB PodsWithResourceClaims]`),
		},
		{
			name: "Invalid ExtraEnabled: Misspelled policy",
			args: &DefaultEvictorArgs{
				PodProtections: PodProtections{
					ExtraEnabled: []PodProtection{"PodsWithPVCC"},
				},
			},
			errInfo: fmt.Errorf(`invalid pod protection policy in ExtraEnabled: "PodsWithPVCC". Valid options are: [PodsWithPVC PodsWithoutPDB PodsWithResourceClaims]`),
		},
		{
			name: "Invalid ExtraEnabled: Policy from DefaultDisabled list",
			args: &DefaultEvictorArgs{
				PodProtections: PodProtections{
					ExtraEnabled: []PodProtection{DaemonSetPods},
				},
			},
			errInfo: fmt.Errorf(`invalid pod protection policy in ExtraEnabled: "DaemonSetPods". Valid options are: [PodsWithPVC PodsWithoutPDB PodsWithResourceClaims]`),
		},
		{
			name: "Invalid DefaultDisabled: Unknown policy",
			args: &DefaultEvictorArgs{
				PodProtections: PodProtections{
					DefaultDisabled: []PodProtection{"InvalidPolicy"},
				},
			},
			errInfo: fmt.Errorf(`invalid pod protection policy in DefaultDisabled: "InvalidPolicy". Valid options are: [PodsWithLocalStorage SystemCriticalPods FailedBarePods DaemonSetPods]`),
		},
		{
			name: "Invalid DefaultDisabled: Misspelled policy",
			args: &DefaultEvictorArgs{
				PodProtections: PodProtections{
					DefaultDisabled: []PodProtection{"PodsWithLocalStorag"},
				},
			},
			errInfo: fmt.Errorf(`invalid pod protection policy in DefaultDisabled: "PodsWithLocalStorag". Valid options are: [PodsWithLocalStorage SystemCriticalPods FailedBarePods DaemonSetPods]`),
		},
		{
			name: "Invalid DefaultDisabled: Policy from ExtraEnabled list",
			args: &DefaultEvictorArgs{
				PodProtections: PodProtections{
					DefaultDisabled: []PodProtection{PodsWithPVC},
				},
			},
			errInfo: fmt.Errorf(`invalid pod protection policy in DefaultDisabled: "PodsWithPVC". Valid options are: [PodsWithLocalStorage SystemCriticalPods FailedBarePods DaemonSetPods]`),
		},
		{
			name: "Invalid ExtraEnabled duplicate",
			args: &DefaultEvictorArgs{
				PodProtections: PodProtections{
					ExtraEnabled: []PodProtection{PodsWithPVC, PodsWithPVC},
				},
			},
			errInfo: fmt.Errorf(`PodProtections.ExtraEnabled contains duplicate entries`),
		},
		{
			name: "Invalid DefaultDisabled duplicate",
			args: &DefaultEvictorArgs{
				PodProtections: PodProtections{
					DefaultDisabled: []PodProtection{PodsWithLocalStorage, PodsWithLocalStorage},
				},
			},
			errInfo: fmt.Errorf(`PodProtections.DefaultDisabled contains duplicate entries`),
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			validateErr := ValidateDefaultEvictorArgs(runtime.Object(testCase.args))
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
