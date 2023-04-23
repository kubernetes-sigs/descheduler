/*
Copyright 2023 The Kubernetes Authors.
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

package trimaran

import (
	"strconv"
	"testing"

	"github.com/google/go-cmp/cmp"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/utils/pointer"
)

func TestSetDefaults_TrimaranArgs(t *testing.T) {
	tests := []struct {
		name string
		in   runtime.Object
		want runtime.Object
	}{
		{
			name: "empty config TargetLoadPackingArgs",
			in:   &TargetLoadPackingArgs{},
			want: &TargetLoadPackingArgs{
				TrimaranSpec: TrimaranSpec{
					MetricProvider: MetricProviderSpec{
						Type: "KubernetesMetricsServer",
					},
				},
				DefaultRequests: v1.ResourceList{v1.ResourceCPU: resource.MustParse(
					strconv.FormatInt(DefaultRequestsMilliCores, 10) + "m")},
				DefaultRequestsMultiplier: pointer.StringPtr("1.5"),
				TargetUtilization:         pointer.Int64Ptr(40),
			},
		},
		{
			name: "set non default TargetLoadPackingArgs",
			in: &TargetLoadPackingArgs{
				TrimaranSpec: TrimaranSpec{
					WatcherAddress: pointer.StringPtr("http://localhost:2020"),
				},
				DefaultRequests:           v1.ResourceList{v1.ResourceCPU: resource.MustParse("100m")},
				DefaultRequestsMultiplier: pointer.StringPtr("2.5"),
				TargetUtilization:         pointer.Int64Ptr(50),
			},
			want: &TargetLoadPackingArgs{
				TrimaranSpec: TrimaranSpec{
					WatcherAddress: pointer.StringPtr("http://localhost:2020"),
				},
				DefaultRequests:           v1.ResourceList{v1.ResourceCPU: resource.MustParse("100m")},
				DefaultRequestsMultiplier: pointer.StringPtr("2.5"),
				TargetUtilization:         pointer.Int64Ptr(50),
			},
		},
	}
	for _, tc := range tests {
		scheme := runtime.NewScheme()
		utilruntime.Must(AddToScheme(scheme))
		t.Run(tc.name, func(t *testing.T) {
			scheme.Default(tc.in)
			if diff := cmp.Diff(tc.in, tc.want); diff != "" {
				t.Errorf("Got unexpected defaults (-want, +got):\n%s", diff)
			}
		})
	}
}
