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

package removefailedpods

import (
	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/descheduler/pkg/api"
	"testing"
)

func TestSetDefaults_RemoveFailedPodsArgs(t *testing.T) {
	tests := []struct {
		name string
		in   runtime.Object
		want runtime.Object
	}{
		{
			name: "RemoveFailedPodsArgs empty",
			in:   &RemoveFailedPodsArgs{},
			want: &RemoveFailedPodsArgs{
				Namespaces:              nil,
				LabelSelector:           nil,
				ExcludeOwnerKinds:       nil,
				MinPodLifetimeSeconds:   nil,
				Reasons:                 nil,
				IncludingInitContainers: false,
			},
		},
		{
			name: "RemoveFailedPodsArgs with value",
			in: &RemoveFailedPodsArgs{
				Namespaces:              &api.Namespaces{},
				LabelSelector:           &metav1.LabelSelector{},
				ExcludeOwnerKinds:       []string{"ReplicaSet"},
				MinPodLifetimeSeconds:   pointer.Uint(0),
				Reasons:                 []string{"reason"},
				IncludingInitContainers: true,
			},
			want: &RemoveFailedPodsArgs{
				Namespaces:              &api.Namespaces{},
				LabelSelector:           &metav1.LabelSelector{},
				ExcludeOwnerKinds:       []string{"ReplicaSet"},
				MinPodLifetimeSeconds:   pointer.Uint(0),
				Reasons:                 []string{"reason"},
				IncludingInitContainers: true,
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
