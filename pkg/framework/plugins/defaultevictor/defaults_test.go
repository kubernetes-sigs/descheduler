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

package defaultevictor

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/descheduler/pkg/api"
)

func TestSetDefaults_DefaultEvictorArgs(t *testing.T) {
	tests := []struct {
		name string
		in   runtime.Object
		want runtime.Object
	}{
		{
			name: "DefaultEvictorArgs empty",
			in:   &DefaultEvictorArgs{},
			want: &DefaultEvictorArgs{
				NodeSelector:            "",
				EvictLocalStoragePods:   false,
				EvictSystemCriticalPods: false,
				IgnorePvcPods:           false,
				EvictFailedBarePods:     false,
				LabelSelector:           nil,
				PriorityThreshold:       nil,
				NodeFit:                 false,
			},
		},
		{
			name: "DefaultEvictorArgs with value",
			in: &DefaultEvictorArgs{
				NodeSelector:            "NodeSelector",
				EvictLocalStoragePods:   true,
				EvictSystemCriticalPods: true,
				IgnorePvcPods:           true,
				EvictFailedBarePods:     true,
				LabelSelector:           nil,
				PriorityThreshold: &api.PriorityThreshold{
					Value: pointer.Int32(800),
				},
				NodeFit: true,
			},
			want: &DefaultEvictorArgs{
				NodeSelector:            "NodeSelector",
				EvictLocalStoragePods:   true,
				EvictSystemCriticalPods: true,
				IgnorePvcPods:           true,
				EvictFailedBarePods:     true,
				LabelSelector:           nil,
				PriorityThreshold: &api.PriorityThreshold{
					Value: pointer.Int32(800),
				},
				NodeFit: true,
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
