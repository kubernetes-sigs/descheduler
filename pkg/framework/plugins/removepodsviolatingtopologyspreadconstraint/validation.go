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
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
)

// ValidateRemovePodsViolatingTopologySpreadConstraintArgs validates RemovePodsViolatingTopologySpreadConstraint arguments
func ValidateRemovePodsViolatingTopologySpreadConstraintArgs(_ klog.Logger, obj runtime.Object) error {
	var errs []error

	args := obj.(*RemovePodsViolatingTopologySpreadConstraintArgs)
	// At most one of include/exclude can be set
	if args.Namespaces != nil && len(args.Namespaces.Include) > 0 && len(args.Namespaces.Exclude) > 0 {
		errs = append(errs, fmt.Errorf("only one of Include/Exclude namespaces can be set"))
	}

	if args.LabelSelector != nil {
		if _, err := metav1.LabelSelectorAsSelector(args.LabelSelector); err != nil {
			errs = append(errs, fmt.Errorf("failed to get label selectors from strategy's params: %+v", err))
		}
	}

	if len(args.Constraints) > 0 {
		supportedConstraints := sets.New(v1.DoNotSchedule, v1.ScheduleAnyway)
		for _, constraint := range args.Constraints {
			if !supportedConstraints.Has(constraint) {
				errs = append(errs, fmt.Errorf("constraint %s is not one of %v", constraint, supportedConstraints))
			}
		}
	}

	return errors.NewAggregate(errs)
}
