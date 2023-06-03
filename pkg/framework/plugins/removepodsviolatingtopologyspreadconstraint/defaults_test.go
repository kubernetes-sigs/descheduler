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

package removepodsviolatingtopologyspreadconstraint

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	utilpointer "k8s.io/utils/pointer"
	"sigs.k8s.io/descheduler/pkg/api"
)

var scheme *runtime.Scheme

func init() {
	scheme = runtime.NewScheme()
	scheme.AddTypeDefaultingFunc(&RemovePodsViolatingTopologySpreadConstraintArgs{}, func(obj interface{}) {
		SetDefaults_RemovePodsViolatingTopologySpreadConstraintArgs(obj.(*RemovePodsViolatingTopologySpreadConstraintArgs))
	})
	utilruntime.Must(AddToScheme(scheme))
}

func TestSetDefaults_RemovePodsViolatingTopologySpreadConstraintArgs(t *testing.T) {
	tests := []struct {
		name string
		in   runtime.Object
		want runtime.Object
	}{
		{
			name: "RemovePodsViolatingTopologySpreadConstraintArgs empty",
			in:   &RemovePodsViolatingTopologySpreadConstraintArgs{},
			want: &RemovePodsViolatingTopologySpreadConstraintArgs{
				Namespaces:             nil,
				LabelSelector:          nil,
				IncludeSoftConstraints: false,
				TopologyBalanceNodeFit: utilpointer.Bool(true),
			},
		},
		{
			name: "RemovePodsViolatingTopologySpreadConstraintArgs with value",
			in: &RemovePodsViolatingTopologySpreadConstraintArgs{
				Namespaces:             &api.Namespaces{},
				LabelSelector:          &metav1.LabelSelector{},
				IncludeSoftConstraints: true,
			},
			want: &RemovePodsViolatingTopologySpreadConstraintArgs{
				Namespaces:             &api.Namespaces{},
				LabelSelector:          &metav1.LabelSelector{},
				IncludeSoftConstraints: true,
				TopologyBalanceNodeFit: utilpointer.Bool(true),
			},
		},
		{
			name: "RemovePodsViolatingTopologySpreadConstraintArgs without TopologyBalanceNodeFit",
			in:   &RemovePodsViolatingTopologySpreadConstraintArgs{},
			want: &RemovePodsViolatingTopologySpreadConstraintArgs{
				TopologyBalanceNodeFit: utilpointer.Bool(true),
			},
		},
		{
			name: "RemovePodsViolatingTopologySpreadConstraintArgs with TopologyBalanceNodeFit=false",
			in: &RemovePodsViolatingTopologySpreadConstraintArgs{
				TopologyBalanceNodeFit: utilpointer.Bool(false),
			},
			want: &RemovePodsViolatingTopologySpreadConstraintArgs{
				TopologyBalanceNodeFit: utilpointer.Bool(false),
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scheme.Default(tc.in)
			if diff := cmp.Diff(tc.in, tc.want); diff != "" {
				t.Errorf("Got unexpected defaults (-want, +got):\n%s", diff)
			}
		})
	}
}
