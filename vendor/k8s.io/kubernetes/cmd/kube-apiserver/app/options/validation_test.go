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

package options

import (
	"net"
	"testing"

	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"k8s.io/kubernetes/pkg/features"
)

func makeOptionsWithCIDRs(serviceCIDR string, secondaryServiceCIDR string) *ServerRunOptions {
	value := serviceCIDR
	if len(secondaryServiceCIDR) > 0 {
		value = value + "," + secondaryServiceCIDR
	}

	var primaryCIDR, secondaryCIDR net.IPNet
	if len(serviceCIDR) > 0 {
		_, cidr, _ := net.ParseCIDR(serviceCIDR)
		if cidr != nil {
			primaryCIDR = *(cidr)
		}
	}

	if len(secondaryServiceCIDR) > 0 {
		_, cidr, _ := net.ParseCIDR(secondaryServiceCIDR)
		if cidr != nil {
			secondaryCIDR = *(cidr)
		}
	}
	return &ServerRunOptions{
		ServiceClusterIPRanges:         value,
		PrimaryServiceClusterIPRange:   primaryCIDR,
		SecondaryServiceClusterIPRange: secondaryCIDR,
	}
}

func TestClusterSerivceIPRange(t *testing.T) {
	testCases := []struct {
		name            string
		options         *ServerRunOptions
		enableDualStack bool
		expectErrors    bool
	}{
		{
			name:            "no service cidr",
			expectErrors:    true,
			options:         makeOptionsWithCIDRs("", ""),
			enableDualStack: false,
		},
		{
			name:            "only secondary service cidr, dual stack gate on",
			expectErrors:    true,
			options:         makeOptionsWithCIDRs("", "10.0.0.0/16"),
			enableDualStack: true,
		},
		{
			name:            "only secondary service cidr, dual stack gate off",
			expectErrors:    true,
			options:         makeOptionsWithCIDRs("", "10.0.0.0/16"),
			enableDualStack: false,
		},
		{
			name:            "primary and secondary are provided but not dual stack v4-v4",
			expectErrors:    true,
			options:         makeOptionsWithCIDRs("10.0.0.0/16", "11.0.0.0/16"),
			enableDualStack: true,
		},
		{
			name:            "primary and secondary are provided but not dual stack v6-v6",
			expectErrors:    true,
			options:         makeOptionsWithCIDRs("2000::/108", "3000::/108"),
			enableDualStack: true,
		},
		{
			name:            "valid dual stack with gate disabled",
			expectErrors:    true,
			options:         makeOptionsWithCIDRs("10.0.0.0/16", "3000::/108"),
			enableDualStack: false,
		},
		/* success cases */
		{
			name:            "valid primary",
			expectErrors:    false,
			options:         makeOptionsWithCIDRs("10.0.0.0/16", ""),
			enableDualStack: false,
		},
		{
			name:            "valid v4-v6 dual stack + gate on",
			expectErrors:    false,
			options:         makeOptionsWithCIDRs("10.0.0.0/16", "3000::/108"),
			enableDualStack: true,
		},
		{
			name:            "valid v6-v4 dual stack + gate on",
			expectErrors:    false,
			options:         makeOptionsWithCIDRs("3000::/108", "10.0.0.0/16"),
			enableDualStack: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.IPv6DualStack, tc.enableDualStack)()
			errs := validateClusterIPFlags(tc.options)
			if len(errs) > 0 && !tc.expectErrors {
				t.Errorf("expected no errors, errors found %+v", errs)
			}

			if len(errs) == 0 && tc.expectErrors {
				t.Errorf("expected errors, no errors found")
			}
		})
	}
}
