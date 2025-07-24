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
		}, {
			name: "passing invalid no eviction policy",
			args: &DefaultEvictorArgs{
				NoEvictionPolicy: "invalid-no-eviction-policy",
			},
			errInfo: fmt.Errorf("noEvictionPolicy accepts only %q values", []NoEvictionPolicy{PreferredNoEvictionPolicy, MandatoryNoEvictionPolicy}),
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
