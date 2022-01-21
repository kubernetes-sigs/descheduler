package validation

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	clientset "k8s.io/client-go/kubernetes"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/utils"
)

// ValidatedStrategyParams contains validated common strategy parameters
type ValidatedStrategyParams struct {
	ThresholdPriority  int32
	IncludedNamespaces sets.String
	ExcludedNamespaces sets.String
	LabelSelector      labels.Selector
	NodeFit            bool
}

func DefaultValidatedStrategyParams() ValidatedStrategyParams {
	return ValidatedStrategyParams{ThresholdPriority: utils.SystemCriticalPriority}
}

func ValidateAndParseStrategyParams(
	ctx context.Context,
	client clientset.Interface,
	params *api.StrategyParameters,
) (*ValidatedStrategyParams, error) {
	if params == nil {
		defaultValidatedStrategyParams := DefaultValidatedStrategyParams()
		return &defaultValidatedStrategyParams, nil
	}

	// At most one of include/exclude can be set
	var includedNamespaces, excludedNamespaces sets.String
	if params.Namespaces != nil && len(params.Namespaces.Include) > 0 && len(params.Namespaces.Exclude) > 0 {
		return nil, fmt.Errorf("only one of Include/Exclude namespaces can be set")
	}
	if params.ThresholdPriority != nil && params.ThresholdPriorityClassName != "" {
		return nil, fmt.Errorf("only one of ThresholdPriority and thresholdPriorityClassName can be set")
	}

	thresholdPriority, err := utils.GetPriorityFromStrategyParams(ctx, client, params)
	if err != nil {
		return nil, fmt.Errorf("failed to get threshold priority from strategy's params: %+v", err)
	}
	if params.Namespaces != nil {
		includedNamespaces = sets.NewString(params.Namespaces.Include...)
		excludedNamespaces = sets.NewString(params.Namespaces.Exclude...)
	}
	var selector labels.Selector
	if params.LabelSelector != nil {
		selector, err = metav1.LabelSelectorAsSelector(params.LabelSelector)
		if err != nil {
			return nil, fmt.Errorf("failed to get label selectors from strategy's params: %+v", err)
		}
	}

	return &ValidatedStrategyParams{
		ThresholdPriority:  thresholdPriority,
		IncludedNamespaces: includedNamespaces,
		ExcludedNamespaces: excludedNamespaces,
		LabelSelector:      selector,
		NodeFit:            params.NodeFit,
	}, nil
}
