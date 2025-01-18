package removepodsviolatinginterpodantiaffinity

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/pkg/api"
)

func TestValidateRemovePodsViolatingInterPodAntiAffinityArgs(t *testing.T) {
	testCases := []struct {
		description string
		args        *RemovePodsViolatingInterPodAntiAffinityArgs
		expectError bool
	}{
		{
			description: "valid namespace args, no errors",
			args: &RemovePodsViolatingInterPodAntiAffinityArgs{
				Namespaces: &api.Namespaces{
					Include: []string{"default"},
				},
			},
			expectError: false,
		},
		{
			description: "invalid namespaces args, expects error",
			args: &RemovePodsViolatingInterPodAntiAffinityArgs{
				Namespaces: &api.Namespaces{
					Include: []string{"default"},
					Exclude: []string{"kube-system"},
				},
			},
			expectError: true,
		},
		{
			description: "valid label selector args, no errors",
			args: &RemovePodsViolatingInterPodAntiAffinityArgs{
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"role.kubernetes.io/node": ""},
				},
			},
			expectError: false,
		},
		{
			description: "invalid label selector args, expects errors",
			args: &RemovePodsViolatingInterPodAntiAffinityArgs{
				LabelSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Operator: metav1.LabelSelectorOpIn,
						},
					},
				},
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			err := ValidateRemovePodsViolatingInterPodAntiAffinityArgs(klog.FromContext(context.Background()), tc.args)
			hasError := err != nil
			if tc.expectError != hasError {
				t.Error("unexpected arg validation behavior")
			}
		})
	}
}
