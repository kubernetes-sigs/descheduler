package validation

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"sigs.k8s.io/descheduler/pkg/api"
)

var (
	thresholdPriority int32 = 1000
)

func TestValidStrategyParams(t *testing.T) {
	ctx := context.Background()
	fakeClient := &fake.Clientset{}
	testCases := []struct {
		name   string
		params *api.StrategyParameters
	}{
		{name: "validate nil params", params: nil},
		{name: "validate empty params", params: &api.StrategyParameters{}},
		{name: "validate params with NodeFit", params: &api.StrategyParameters{NodeFit: true}},
		{name: "validate params with ThresholdPriority", params: &api.StrategyParameters{ThresholdPriority: &thresholdPriority}},
		{name: "validate params with priorityClassName", params: &api.StrategyParameters{ThresholdPriorityClassName: "high-priority"}},
		{name: "validate params with excluded namespace", params: &api.StrategyParameters{Namespaces: &api.Namespaces{Exclude: []string{"excluded-ns"}}}},
		{name: "validate params with included namespace", params: &api.StrategyParameters{Namespaces: &api.Namespaces{Include: []string{"include-ns"}}}},
		{name: "validate params with empty label selector", params: &api.StrategyParameters{LabelSelector: &metav1.LabelSelector{}}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			params, err := ValidateAndParseStrategyParams(ctx, fakeClient, tc.params)
			if err != nil {
				t.Errorf("strategy params should be valid but got err: %v", err.Error())
			}
			if params == nil {
				t.Errorf("strategy params should return a strategyParams but got nil")
			}
		})
	}
}

func TestInvalidStrategyParams(t *testing.T) {
	ctx := context.Background()
	fakeClient := &fake.Clientset{}
	testCases := []struct {
		name   string
		params *api.StrategyParameters
	}{
		{
			name:   "invalid params with both included and excluded namespaces nil params",
			params: &api.StrategyParameters{Namespaces: &api.Namespaces{Include: []string{"include-ns"}, Exclude: []string{"exclude-ns"}}},
		},
		{
			name:   "invalid params with both threshold priority and priority class name",
			params: &api.StrategyParameters{ThresholdPriorityClassName: "high-priority", ThresholdPriority: &thresholdPriority},
		},
		{
			name:   "invalid params with bad label selector",
			params: &api.StrategyParameters{LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"": "missing-label"}}},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			params, err := ValidateAndParseStrategyParams(ctx, fakeClient, tc.params)
			if err == nil {
				t.Errorf("strategy params should be invalid but did not get err")
			}
			if params != nil {
				t.Errorf("strategy params should return a nil strategyParams but got %v", params)
			}
		})
	}
}
