/*
Copyright 2018 The Kubernetes Authors.

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
	"errors"
	"fmt"

	v1 "k8s.io/api/core/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	componentbasevalidation "k8s.io/component-base/config/validation"
	v1helper "k8s.io/kubernetes/pkg/apis/core/v1/helper"
	"k8s.io/kubernetes/pkg/scheduler/apis/config"
)

// ValidateKubeSchedulerConfiguration ensures validation of the KubeSchedulerConfiguration struct
func ValidateKubeSchedulerConfiguration(cc *config.KubeSchedulerConfiguration) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, componentbasevalidation.ValidateClientConnectionConfiguration(&cc.ClientConnection, field.NewPath("clientConnection"))...)
	allErrs = append(allErrs, ValidateKubeSchedulerLeaderElectionConfiguration(&cc.LeaderElection, field.NewPath("leaderElection"))...)
	if len(cc.SchedulerName) == 0 {
		allErrs = append(allErrs, field.Required(field.NewPath("schedulerName"), ""))
	}
	for _, msg := range validation.IsValidSocketAddr(cc.HealthzBindAddress) {
		allErrs = append(allErrs, field.Invalid(field.NewPath("healthzBindAddress"), cc.HealthzBindAddress, msg))
	}
	for _, msg := range validation.IsValidSocketAddr(cc.MetricsBindAddress) {
		allErrs = append(allErrs, field.Invalid(field.NewPath("metricsBindAddress"), cc.MetricsBindAddress, msg))
	}
	if cc.HardPodAffinitySymmetricWeight < 0 || cc.HardPodAffinitySymmetricWeight > 100 {
		allErrs = append(allErrs, field.Invalid(field.NewPath("hardPodAffinitySymmetricWeight"), cc.HardPodAffinitySymmetricWeight, "not in valid range [0-100]"))
	}
	if cc.PercentageOfNodesToScore < 0 || cc.PercentageOfNodesToScore > 100 {
		allErrs = append(allErrs, field.Invalid(field.NewPath("percentageOfNodesToScore"),
			cc.PercentageOfNodesToScore, "not in valid range [0-100]"))
	}
	if cc.PodInitialBackoffSeconds <= 0 {
		allErrs = append(allErrs, field.Invalid(field.NewPath("podInitialBackoffSeconds"),
			cc.PodInitialBackoffSeconds, "must be greater than 0"))
	}
	if cc.PodMaxBackoffSeconds < cc.PodInitialBackoffSeconds {
		allErrs = append(allErrs, field.Invalid(field.NewPath("podMaxBackoffSeconds"),
			cc.PodMaxBackoffSeconds, "must be greater than or equal to PodInitialBackoffSeconds"))
	}
	return allErrs
}

// ValidateKubeSchedulerLeaderElectionConfiguration ensures validation of the KubeSchedulerLeaderElectionConfiguration struct
func ValidateKubeSchedulerLeaderElectionConfiguration(cc *config.KubeSchedulerLeaderElectionConfiguration, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if !cc.LeaderElectionConfiguration.LeaderElect {
		return allErrs
	}
	allErrs = append(allErrs, componentbasevalidation.ValidateLeaderElectionConfiguration(&cc.LeaderElectionConfiguration, field.NewPath("leaderElectionConfiguration"))...)
	return allErrs
}

// ValidatePolicy checks for errors in the Config
// It does not return early so that it can find as many errors as possible
func ValidatePolicy(policy config.Policy) error {
	var validationErrors []error

	priorities := make(map[string]config.PriorityPolicy, len(policy.Priorities))
	for _, priority := range policy.Priorities {
		if priority.Weight <= 0 || priority.Weight >= config.MaxWeight {
			validationErrors = append(validationErrors, fmt.Errorf("Priority %s should have a positive weight applied to it or it has overflown", priority.Name))
		}
		validationErrors = append(validationErrors, validateCustomPriorities(priorities, priority))
	}

	binders := 0
	extenderManagedResources := sets.NewString()
	for _, extender := range policy.Extenders {
		if len(extender.PrioritizeVerb) > 0 && extender.Weight <= 0 {
			validationErrors = append(validationErrors, fmt.Errorf("Priority for extender %s should have a positive weight applied to it", extender.URLPrefix))
		}
		if extender.BindVerb != "" {
			binders++
		}
		for _, resource := range extender.ManagedResources {
			errs := validateExtendedResourceName(v1.ResourceName(resource.Name))
			if len(errs) != 0 {
				validationErrors = append(validationErrors, errs...)
			}
			if extenderManagedResources.Has(resource.Name) {
				validationErrors = append(validationErrors, fmt.Errorf("Duplicate extender managed resource name %s", string(resource.Name)))
			}
			extenderManagedResources.Insert(resource.Name)
		}
	}
	if binders > 1 {
		validationErrors = append(validationErrors, fmt.Errorf("Only one extender can implement bind, found %v", binders))
	}
	return utilerrors.NewAggregate(validationErrors)
}

// validateCustomPriorities validates that:
// 1. RequestedToCapacityRatioRedeclared custom priority cannot be declared multiple times,
// 2. LabelPreference/ServiceAntiAffinity custom priorities can be declared multiple times,
// however the weights for each custom priority type should be the same.
func validateCustomPriorities(priorities map[string]config.PriorityPolicy, priority config.PriorityPolicy) error {
	verifyRedeclaration := func(priorityType string) error {
		if existing, alreadyDeclared := priorities[priorityType]; alreadyDeclared {
			return fmt.Errorf("Priority %q redeclares custom priority %q, from:%q", priority.Name, priorityType, existing.Name)
		}
		priorities[priorityType] = priority
		return nil
	}
	verifyDifferentWeights := func(priorityType string) error {
		if existing, alreadyDeclared := priorities[priorityType]; alreadyDeclared {
			if existing.Weight != priority.Weight {
				return fmt.Errorf("%s  priority %q has a different weight with %q", priorityType, priority.Name, existing.Name)
			}
		}
		priorities[priorityType] = priority
		return nil
	}
	if priority.Argument != nil {
		if priority.Argument.LabelPreference != nil {
			if err := verifyDifferentWeights("LabelPreference"); err != nil {
				return err
			}
		} else if priority.Argument.ServiceAntiAffinity != nil {
			if err := verifyDifferentWeights("ServiceAntiAffinity"); err != nil {
				return err
			}
		} else if priority.Argument.RequestedToCapacityRatioArguments != nil {
			if err := verifyRedeclaration("RequestedToCapacityRatio"); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("No priority arguments set for priority %s", priority.Name)
		}
	}
	return nil
}

// validateExtendedResourceName checks whether the specified name is a valid
// extended resource name.
func validateExtendedResourceName(name v1.ResourceName) []error {
	var validationErrors []error
	for _, msg := range validation.IsQualifiedName(string(name)) {
		validationErrors = append(validationErrors, errors.New(msg))
	}
	if len(validationErrors) != 0 {
		return validationErrors
	}
	if !v1helper.IsExtendedResourceName(name) {
		validationErrors = append(validationErrors, fmt.Errorf("%s is an invalid extended resource name", name))
	}
	return validationErrors
}
