package nodeutilization

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNodeMatchesSelector(t *testing.T) {
	tests := []struct {
		name     string
		node     *v1.Node
		selector map[string]string
		expected bool
	}{
		{
			name: "node matches single label",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
					Labels: map[string]string{
						"env": "prod",
					},
				},
			},
			selector: map[string]string{
				"env": "prod",
			},
			expected: true,
		},
		{
			name: "node matches multiple labels",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
					Labels: map[string]string{
						"env":  "prod",
						"zone": "us-west",
						"type": "worker",
					},
				},
			},
			selector: map[string]string{
				"env":  "prod",
				"zone": "us-west",
			},
			expected: true,
		},
		{
			name: "node does not match label value",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
					Labels: map[string]string{
						"env": "dev",
					},
				},
			},
			selector: map[string]string{
				"env": "prod",
			},
			expected: false,
		},
		{
			name: "node missing required label",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
					Labels: map[string]string{
						"zone": "us-west",
					},
				},
			},
			selector: map[string]string{
				"env": "prod",
			},
			expected: false,
		},
		{
			name: "empty selector matches all nodes",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
					Labels: map[string]string{
						"env": "prod",
					},
				},
			},
			selector: map[string]string{},
			expected: true,
		},
		{
			name: "node with no labels and non-empty selector",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-node",
					Labels: map[string]string{},
				},
			},
			selector: map[string]string{
				"env": "prod",
			},
			expected: false,
		},
		{
			name: "cluster-autoscaler scale-down disabled label",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
					Labels: map[string]string{
						"cluster-autoscaler.kubernetes.io/scale-down-disabled": "true",
						"node.kubernetes.io/instance-type":                    "m5.large",
					},
				},
			},
			selector: map[string]string{
				"cluster-autoscaler.kubernetes.io/scale-down-disabled": "true",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := nodeMatchesSelector(tt.node, tt.selector)
			if result != tt.expected {
				t.Errorf("nodeMatchesSelector() = %v, want %v", result, tt.expected)
			}
		})
	}
}