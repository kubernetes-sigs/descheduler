/*
Copyright 2017 The Kubernetes Authors.

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

package util

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestGetNonZeroRequest(t *testing.T) {
	tests := []struct {
		name           string
		requests       v1.ResourceList
		expectedCPU    int64
		expectedMemory int64
	}{
		{
			"cpu_and_memory_not_found",
			v1.ResourceList{},
			DefaultMilliCPURequest,
			DefaultMemoryRequest,
		},
		{
			"only_cpu_exist",
			v1.ResourceList{
				v1.ResourceCPU: resource.MustParse("200m"),
			},
			200,
			DefaultMemoryRequest,
		},
		{
			"only_memory_exist",
			v1.ResourceList{
				v1.ResourceMemory: resource.MustParse("400Mi"),
			},
			DefaultMilliCPURequest,
			400 * 1024 * 1024,
		},
		{
			"cpu_memory_exist",
			v1.ResourceList{
				v1.ResourceCPU:    resource.MustParse("200m"),
				v1.ResourceMemory: resource.MustParse("400Mi"),
			},
			200,
			400 * 1024 * 1024,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			realCPU, realMemory := GetNonzeroRequests(&test.requests)
			assert.EqualValuesf(t, test.expectedCPU, realCPU, "Failed to test: %s", test.name)
			assert.EqualValuesf(t, test.expectedMemory, realMemory, "Failed to test: %s", test.name)
		})
	}
}

func TestGetLeastRequestResource(t *testing.T) {
	tests := []struct {
		name             string
		requests         v1.ResourceList
		resource         v1.ResourceName
		expectedQuantity int64
	}{
		{
			"extended_resource_not_found",
			v1.ResourceList{},
			v1.ResourceName("intel.com/foo"),
			0,
		},
		{
			"extended_resource_found",
			v1.ResourceList{
				v1.ResourceName("intel.com/foo"): resource.MustParse("4"),
			},
			v1.ResourceName("intel.com/foo"),
			4,
		},
		{
			"cpu_not_found",
			v1.ResourceList{},
			v1.ResourceCPU,
			DefaultMilliCPURequest,
		},
		{
			"memory_not_found",
			v1.ResourceList{},
			v1.ResourceMemory,
			DefaultMemoryRequest,
		},
		{
			"cpu_exist",
			v1.ResourceList{
				v1.ResourceCPU: resource.MustParse("200m"),
			},
			v1.ResourceCPU,
			200,
		},
		{
			"memory_exist",
			v1.ResourceList{
				v1.ResourceMemory: resource.MustParse("400Mi"),
			},
			v1.ResourceMemory,
			400 * 1024 * 1024,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			realQuantity := GetNonzeroRequestForResource(test.resource, &test.requests)
			assert.EqualValuesf(t, test.expectedQuantity, realQuantity, "Failed to test: %s", test.name)
		})
	}
}
