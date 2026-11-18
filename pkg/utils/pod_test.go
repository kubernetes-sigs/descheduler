/*
Copyright 2025 The Kubernetes Authors.

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

package utils

import (
	"context"
	"testing"

	v1 "k8s.io/api/core/v1"
)

func TestPodToleratesTaints(t *testing.T) {
	noScheduleTaint := func(key, value string) v1.Taint {
		return v1.Taint{Key: key, Value: value, Effect: v1.TaintEffectNoSchedule}
	}
	existsToleration := func(key string) v1.Toleration {
		return v1.Toleration{Key: key, Operator: v1.TolerationOpExists, Effect: v1.TaintEffectNoSchedule}
	}

	tests := []struct {
		name          string
		tolerations   []v1.Toleration
		taintsOfNodes map[string][]v1.Taint
		want          bool
	}{
		{
			// Regression test: a single wildcard Exists toleration tolerates
			// every taint, so it must tolerate a node carrying more taints than
			// the pod has tolerations.
			name:        "single wildcard Exists toleration tolerates a node with multiple taints",
			tolerations: []v1.Toleration{{Operator: v1.TolerationOpExists}},
			taintsOfNodes: map[string][]v1.Taint{
				"node1": {noScheduleTaint("gpu", "true"), noScheduleTaint("dedicated", "team-a")},
			},
			want: true,
		},
		{
			name:        "no tolerations does not tolerate a tainted node",
			tolerations: nil,
			taintsOfNodes: map[string][]v1.Taint{
				"node1": {noScheduleTaint("gpu", "true")},
			},
			want: false,
		},
		{
			name:        "explicit tolerations cover every taint",
			tolerations: []v1.Toleration{existsToleration("gpu"), existsToleration("dedicated")},
			taintsOfNodes: map[string][]v1.Taint{
				"node1": {noScheduleTaint("gpu", "true"), noScheduleTaint("dedicated", "team-a")},
			},
			want: true,
		},
		{
			name:        "a single untolerated taint fails the node",
			tolerations: []v1.Toleration{existsToleration("gpu")},
			taintsOfNodes: map[string][]v1.Taint{
				"node1": {noScheduleTaint("gpu", "true"), noScheduleTaint("dedicated", "team-a")},
			},
			want: false,
		},
		{
			name:        "a node without taints is always tolerated",
			tolerations: nil,
			taintsOfNodes: map[string][]v1.Taint{
				"node1": {},
			},
			want: true,
		},
		{
			name:        "tolerated if any node in the set is tolerated",
			tolerations: []v1.Toleration{existsToleration("gpu")},
			taintsOfNodes: map[string][]v1.Taint{
				"node1": {noScheduleTaint("gpu", "true"), noScheduleTaint("dedicated", "team-a")},
				"node2": {noScheduleTaint("gpu", "true")},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &v1.Pod{Spec: v1.PodSpec{Tolerations: tt.tolerations}}
			if got := PodToleratesTaints(context.Background(), pod, tt.taintsOfNodes); got != tt.want {
				t.Errorf("PodToleratesTaints() = %v, want %v", got, tt.want)
			}
		})
	}
}
