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

package evictions

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDefaultWorkloadResolver(t *testing.T) {
	isController := true
	tests := []struct {
		name         string
		pod          *v1.Pod
		expectedKind string
		expectedName string
	}{
		{
			name: "controller owner is reported directly",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "db-0",
					OwnerReferences: []metav1.OwnerReference{
						{APIVersion: "apps/v1", Kind: "StatefulSet", Name: "db", Controller: &isController},
					},
				},
			},
			expectedKind: "StatefulSet",
			expectedName: "db",
		},
		{
			name: "bare pod resolves to empty workload",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "lonely-pod"},
			},
			expectedKind: "",
			expectedName: "",
		},
		{
			name: "non-controller owner is ignored",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "owned-pod",
					OwnerReferences: []metav1.OwnerReference{
						{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "web-abc123"},
					},
				},
			},
			expectedKind: "",
			expectedName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, name := defaultWorkloadResolver(tt.pod)
			if kind != tt.expectedKind || name != tt.expectedName {
				t.Errorf("defaultWorkloadResolver() = (%q, %q), want (%q, %q)", kind, name, tt.expectedKind, tt.expectedName)
			}
		})
	}
}

func TestWorkloadLabelValues(t *testing.T) {
	tests := []struct {
		name         string
		kind, rsName string
		expectedKind string
		expectedName string
	}{
		{
			name:         "resolved workload is passed through",
			kind:         "Deployment",
			rsName:       "web",
			expectedKind: "Deployment",
			expectedName: "web",
		},
		{
			name:         "absent workload maps to the placeholder",
			kind:         "",
			rsName:       "",
			expectedKind: workloadNone,
			expectedName: workloadNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, name := workloadLabelValues(tt.kind, tt.rsName)
			if kind != tt.expectedKind || name != tt.expectedName {
				t.Errorf("workloadLabelValues() = (%q, %q), want (%q, %q)", kind, name, tt.expectedKind, tt.expectedName)
			}
		})
	}
}
