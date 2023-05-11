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

package scaledowndeploymenthavingtoomanypodrestarts

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/utils/pointer"

	"sigs.k8s.io/descheduler/pkg/api"
)

func TestSetDefaults_ScaleDownDeploymentHavingTooManyPodRestartsArgs(t *testing.T) {
	tests := []struct {
		name string
		in   runtime.Object
		want runtime.Object
	}{
		{
			name: "ScaleDownDeploymentHavingTooManyPodRestartsArgs empty",
			in:   &ScaleDownDeploymentHavingTooManyPodRestartsArgs{},
			want: &ScaleDownDeploymentHavingTooManyPodRestartsArgs{
				Namespaces:              nil,
				LabelSelector:           nil,
				ReplicasThreshold:       pointer.Int32(1),
				PodRestartThreshold:     0,
				IncludingInitContainers: false,
			},
		},
		{
			name: "ScaleDownDeploymentHavingTooManyPodRestartsArgs with value",
			in: &ScaleDownDeploymentHavingTooManyPodRestartsArgs{
				Namespaces:              &api.Namespaces{},
				LabelSelector:           &metav1.LabelSelector{},
				ReplicasThreshold:       pointer.Int32(0),
				PodRestartThreshold:     10,
				IncludingInitContainers: true,
			},
			want: &ScaleDownDeploymentHavingTooManyPodRestartsArgs{
				Namespaces:              &api.Namespaces{},
				LabelSelector:           &metav1.LabelSelector{},
				ReplicasThreshold:       pointer.Int32(0),
				PodRestartThreshold:     10,
				IncludingInitContainers: true,
			},
		},
	}
	for _, tc := range tests {
		scheme := runtime.NewScheme()
		utilruntime.Must(AddToScheme(scheme))
		t.Run(tc.name, func(t *testing.T) {
			scheme.Default(tc.in)
			SetDefaults_ScaleDownDeploymentHavingTooManyPodRestartsArgs(tc.in)
			if diff := cmp.Diff(tc.in, tc.want); diff != "" {
				t.Errorf("Got unexpected defaults (-want, +got):\n%s", diff)
			}
		})
	}
}
