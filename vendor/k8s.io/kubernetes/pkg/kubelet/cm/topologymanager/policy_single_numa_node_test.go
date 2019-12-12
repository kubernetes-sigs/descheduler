/*
Copyright 2019 The Kubernetes Authors.

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

package topologymanager

import (
	"testing"
)

func TestPolicySingleNumaNodeCanAdmitPodResult(t *testing.T) {
	tcases := []struct {
		name     string
		hint     TopologyHint
		expected bool
	}{
		{
			name:     "Preferred is set to false in topology hints",
			hint:     TopologyHint{nil, false},
			expected: false,
		},
	}

	for _, tc := range tcases {
		numaNodes := []int{0, 1}
		policy := NewSingleNumaNodePolicy(numaNodes)
		result := policy.(*singleNumaNodePolicy).canAdmitPodResult(&tc.hint)

		if result.Admit != tc.expected {
			t.Errorf("Expected Admit field in result to be %t, got %t", tc.expected, result.Admit)
		}

		if tc.expected == false {
			if len(result.Reason) == 0 {
				t.Errorf("Expected Reason field to be not empty")
			}
			if len(result.Message) == 0 {
				t.Errorf("Expected Message field to be not empty")
			}
		}
	}
}

func TestSingleNumaNodePolicyMerge(t *testing.T) {
	numaNodes := []int{0, 1}
	policy := NewSingleNumaNodePolicy(numaNodes)

	tcases := commonPolicyMergeTestCases(numaNodes)
	tcases = append(tcases, policy.(*singleNumaNodePolicy).mergeTestCases(numaNodes)...)

	testPolicyMerge(policy, tcases, t)
}
