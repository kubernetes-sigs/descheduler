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

package metrics

import (
	"testing"
)

func TestNormalizePluginName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"LowNodeUtilization", "low_node_utilization"},
		{"RemovePodsViolatingNodeTaints", "remove_pods_violating_node_taints"},
		{"HighNodeUtilization", "high_node_utilization"},
		{"RemoveDuplicates", "remove_duplicates"},
		{"simple", "simple"},
		{"SimplePlugin", "simple_plugin"},
		{"ABC", "a_b_c"},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			result := normalizePluginName(test.input)
			if result != test.expected {
				t.Errorf("normalizePluginName(%q) = %q, expected %q", test.input, result, test.expected)
			}
		})
	}
}

func TestPluginMetricsRegistry_GetPluginSubsystem(t *testing.T) {
	registry := NewPluginMetricsRegistry()

	tests := []struct {
		name        string
		profileName string
		pluginName  string
		expected    string
	}{
		{
			name:        "plugin without a profile",
			profileName: "",
			pluginName:  "LowNodeUtilization",
			expected:    "descheduler_low_node_utilization",
		},
		{
			name:        "well known profile with a plugin",
			profileName: "ProfileA",
			pluginName:  "LowNodeUtilization",
			expected:    "descheduler_profile_a_low_node_utilization",
		},
		{
			name:        "same plugin registered by a different profile",
			profileName: "CustomProfile",
			pluginName:  "LowNodeUtilization",
			expected:    "descheduler_custom_profile_low_node_utilization",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.GetPluginSubsystem(tt.profileName, tt.pluginName)
			if result != tt.expected {
				t.Errorf("GetPluginSubsystem(%s, %s) = %q, expected %q", tt.profileName, tt.pluginName, result, tt.expected)
			}
		})
	}
}
