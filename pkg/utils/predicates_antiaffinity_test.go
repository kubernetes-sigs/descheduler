/*
Copyright 2026 The Kubernetes Authors.

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
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetPodAntiAffinityTerms(t *testing.T) {
	required := v1.PodAffinityTerm{
		TopologyKey: "topology.kubernetes.io/zone",
		LabelSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"app": "required"},
		},
	}
	preferredTerm := v1.PodAffinityTerm{
		TopologyKey: "kubernetes.io/hostname",
		LabelSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"app": "preferred"},
		},
	}
	affinity := &v1.PodAntiAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{required},
		PreferredDuringSchedulingIgnoredDuringExecution: []v1.WeightedPodAffinityTerm{
			{Weight: 100, PodAffinityTerm: preferredTerm},
		},
	}

	gotRequired := GetPodAntiAffinityTerms(affinity)
	if len(gotRequired) != 1 || gotRequired[0].TopologyKey != required.TopologyKey {
		t.Fatalf("GetPodAntiAffinityTerms() = %#v, want required-only", gotRequired)
	}

	gotAll := GetPodAntiAffinityTermsWithPreferred(affinity)
	if len(gotAll) != 2 {
		t.Fatalf("GetPodAntiAffinityTermsWithPreferred() len = %d, want 2", len(gotAll))
	}
	if gotAll[0].TopologyKey != required.TopologyKey || gotAll[1].TopologyKey != preferredTerm.TopologyKey {
		t.Fatalf("GetPodAntiAffinityTermsWithPreferred() = %#v", gotAll)
	}

	if terms := GetPodAntiAffinityTerms(nil); terms != nil {
		t.Fatalf("GetPodAntiAffinityTerms(nil) = %#v, want nil", terms)
	}
	if terms := GetPodAntiAffinityTermsWithPreferred(nil); terms != nil {
		t.Fatalf("GetPodAntiAffinityTermsWithPreferred(nil) = %#v, want nil", terms)
	}
}
