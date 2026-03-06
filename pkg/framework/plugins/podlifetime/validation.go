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

package podlifetime

import (
	"fmt"
	"sort"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
)

var podLifeTimeAllowedStates = sets.New(
	// Pod Status Phase
	string(v1.PodRunning),
	string(v1.PodPending),
	string(v1.PodSucceeded),
	string(v1.PodFailed),
	string(v1.PodUnknown),

	// Pod Status Reasons
	"NodeAffinity",
	"NodeLost",
	"Shutdown",
	"UnexpectedAdmissionError",

	// Container State Waiting Reasons
	"PodInitializing",
	"ContainerCreating",
	"ImagePullBackOff",
	"CrashLoopBackOff",
	"CreateContainerConfigError",
	"ErrImagePull",
	"CreateContainerError",
	"InvalidImageName",

	// Container State Terminated Reasons
	"OOMKilled",
	"Error",
	"Completed",
	"DeadlineExceeded",
	"Evicted",
	"ContainerCannotRun",
	"StartError",
)

// ValidatePodLifeTimeArgs validates PodLifeTime arguments
func ValidatePodLifeTimeArgs(obj runtime.Object) error {
	args := obj.(*PodLifeTimeArgs)
	var allErrs []error

	if args.Namespaces != nil && len(args.Namespaces.Include) > 0 && len(args.Namespaces.Exclude) > 0 {
		allErrs = append(allErrs, fmt.Errorf("only one of Include/Exclude namespaces can be set"))
	}

	if args.OwnerKinds != nil && len(args.OwnerKinds.Include) > 0 && len(args.OwnerKinds.Exclude) > 0 {
		allErrs = append(allErrs, fmt.Errorf("only one of Include/Exclude ownerKinds can be set"))
	}

	if args.LabelSelector != nil {
		if _, err := metav1.LabelSelectorAsSelector(args.LabelSelector); err != nil {
			allErrs = append(allErrs, fmt.Errorf("failed to get label selectors from strategy's params: %+v", err))
		}
	}

	if len(args.States) > 0 && !podLifeTimeAllowedStates.HasAll(args.States...) {
		allowed := podLifeTimeAllowedStates.UnsortedList()
		sort.Strings(allowed)
		allErrs = append(allErrs, fmt.Errorf("states must be one of %v", allowed))
	}

	for i, c := range args.Conditions {
		if c.Type == "" && c.Status == "" && c.Reason == "" && c.MinTimeSinceLastTransitionSeconds == nil {
			allErrs = append(allErrs, fmt.Errorf("conditions[%d]: at least one of type, status, reason, or minTimeSinceLastTransitionSeconds must be set", i))
		}
	}

	hasFilter := args.MaxPodLifeTimeSeconds != nil ||
		len(args.States) > 0 ||
		len(args.Conditions) > 0 ||
		len(args.ExitCodes) > 0
	if !hasFilter {
		allErrs = append(allErrs, fmt.Errorf("at least one filtering criterion must be specified (maxPodLifeTimeSeconds, states, conditions, or exitCodes)"))
	}

	return utilerrors.NewAggregate(allErrs)
}
