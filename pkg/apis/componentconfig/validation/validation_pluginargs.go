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

package validation

import (
	"fmt"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/apis/componentconfig"
)

// ValidateRemovePodsViolatingNodeTaintsArgs validates RemovePodsViolatingNodeTaints arguments
func ValidateRemovePodsViolatingNodeTaintsArgs(args *componentconfig.RemovePodsViolatingNodeTaintsArgs) error {
	return errorsAggregate(
		validateNamespaceArgs(args.Namespaces),
		validateLabelSelectorArgs(args.LabelSelector),
	)
}

// ValidateRemoveFailedPodsArgs validates RemoveFailedPods arguments
func ValidateRemoveFailedPodsArgs(args *componentconfig.RemoveFailedPodsArgs) error {
	return errorsAggregate(
		validateNamespaceArgs(args.Namespaces),
		validateLabelSelectorArgs(args.LabelSelector),
	)
}

// errorsAggregate converts all arg validation errors to a single error interface.
// if no errors, it will return nil.
func errorsAggregate(errors ...error) error {
	return utilerrors.NewAggregate(errors)
}

func validateNamespaceArgs(namespaces *api.Namespaces) error {
	// At most one of include/exclude can be set
	if namespaces != nil && len(namespaces.Include) > 0 && len(namespaces.Exclude) > 0 {
		return fmt.Errorf("only one of Include/Exclude namespaces can be set")
	}

	return nil
}

func validateLabelSelectorArgs(labelSelector *metav1.LabelSelector) error {
	if labelSelector != nil {
		if _, err := metav1.LabelSelectorAsSelector(labelSelector); err != nil {
			return fmt.Errorf("failed to get label selectors from strategy's params: %+v", err)
		}
	}

	return nil
}

// ValidateRemovePodsViolatingNodeAffinityArgs validates RemovePodsViolatingNodeAffinity arguments
func ValidateRemovePodsViolatingNodeAffinityArgs(args *componentconfig.RemovePodsViolatingNodeAffinityArgs) error {
	if args == nil || len(args.NodeAffinityType) == 0 {
		return fmt.Errorf("nodeAffinityType needs to be set")
	}

	// At most one of include/exclude can be set
	if args.Namespaces != nil && len(args.Namespaces.Include) > 0 && len(args.Namespaces.Exclude) > 0 {
		return fmt.Errorf("only one of Include/Exclude namespaces can be set")
	}

	return nil
}
