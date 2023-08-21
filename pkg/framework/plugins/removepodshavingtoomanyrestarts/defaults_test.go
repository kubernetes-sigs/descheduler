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

package removepodshavingtoomanyrestarts

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/descheduler/pkg/api"
)

func TestSetDefaults_RemovePodsHavingTooManyRestartsArgs(t *testing.T) {
	tests := []struct {
		name string
		in   runtime.Object
		want runtime.Object
	}{
		{
			name: "RemovePodsHavingTooManyRestartsArgs empty",
			in:   &RemovePodsHavingTooManyRestartsArgs{},
			want: &RemovePodsHavingTooManyRestartsArgs{
				Namespaces:              nil,
				LabelSelector:           nil,
				PodRestartThreshold:     0,
				IncludingInitContainers: false,
				States:                  nil,
			},
		},
		{
			name: "RemovePodsHavingTooManyRestartsArgs with value",
			in: &RemovePodsHavingTooManyRestartsArgs{
				Namespaces:              &api.Namespaces{},
				LabelSelector:           &metav1.LabelSelector{},
				PodRestartThreshold:     10,
				IncludingInitContainers: true,
				States:                  []string{string(v1.PodRunning)},
			},
			want: &RemovePodsHavingTooManyRestartsArgs{
				Namespaces:              &api.Namespaces{},
				LabelSelector:           &metav1.LabelSelector{},
				PodRestartThreshold:     10,
				IncludingInitContainers: true,
				States:                  []string{string(v1.PodRunning)},
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
