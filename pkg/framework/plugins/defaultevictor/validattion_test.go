/*
Copyright 2025 The Kubernetes Authors.
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
	"testing"

	"k8s.io/utils/ptr"

	"sigs.k8s.io/descheduler/pkg/api"
)

func TestValidateDefaultEvictorArgs(t *testing.T) {
	tests := []struct {
		name          string
		args          *DefaultEvictorArgs
		expectedError string
	}{
		{
			name: "Valid configuration with no deprecated fields",
			args: &DefaultEvictorArgs{
				DisabledDefaultPodProtections: []DisabledDefaultPodProtection{WithLocalStorage},
			},
			expectedError: "",
		},
		{
			name: "Valid configuration: both DisabledDefaultPodProtections and ExtraPodProtections",
			args: &DefaultEvictorArgs{
				DisabledDefaultPodProtections: []DisabledDefaultPodProtection{WithLocalStorage},
				ExtraPodProtections:           []ExtraPodProtection{WithPVC},
			},
			expectedError: "",
		},
		{
			name: "Valid configuration with ExtraPodProtections",
			args: &DefaultEvictorArgs{
				ExtraPodProtections: []ExtraPodProtection{WithPVC},
			},
			expectedError: "",
		},
		{
			name: "Invalid configuration: Deprecated field used with DisabledDefaultPodProtections",
			args: &DefaultEvictorArgs{
				EvictLocalStoragePods:         true,
				DisabledDefaultPodProtections: []DisabledDefaultPodProtection{WithLocalStorage},
			},
			expectedError: "cannot use Deprecated fields alongside DisabledDefaultPodProtections or ExtraPodProtections",
		},
		{
			name: "Invalid configuration: Deprecated field used with ExtraPodProtections",
			args: &DefaultEvictorArgs{
				EvictDaemonSetPods:  true,
				ExtraPodProtections: []ExtraPodProtection{WithPVC},
			},
			expectedError: "cannot use Deprecated fields alongside DisabledDefaultPodProtections or ExtraPodProtections",
		},
		{
			name: "Invalid configuration: PriorityThreshold misconfigured",
			args: &DefaultEvictorArgs{
				PriorityThreshold: &api.PriorityThreshold{
					Value: ptr.To[int32](10),
					Name:  "high-priority",
				},
			},
			expectedError: `priority threshold misconfigured, only one of priorityThreshold fields can be set, got Value: 10, Name: "high-priority"`,
		},
		{
			name: "MinReplicas warning logged but no error",
			args: &DefaultEvictorArgs{
				MinReplicas: 1,
			},
			expectedError: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDefaultEvictorArgs(tt.args)
			if tt.expectedError == "" {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			} else {
				if err == nil || err.Error() != tt.expectedError {
					t.Errorf("expected error %q, got %v", tt.expectedError, err)
				}
			}
		})
	}
}
