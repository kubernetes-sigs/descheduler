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

package removepodshavingtoomanyrestarts

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
)

// ValidateRemovePodsHavingTooManyRestartsArgs validates RemovePodsHavingTooManyRestarts arguments
func ValidateRemovePodsHavingTooManyRestartsArgs(_ klog.Logger, obj runtime.Object) error {
	args := obj.(*RemovePodsHavingTooManyRestartsArgs)
	// At most one of include/exclude can be set
	if args.Namespaces != nil && len(args.Namespaces.Include) > 0 && len(args.Namespaces.Exclude) > 0 {
		return fmt.Errorf("only one of Include/Exclude namespaces can be set")
	}

	if args.LabelSelector != nil {
		if _, err := metav1.LabelSelectorAsSelector(args.LabelSelector); err != nil {
			return fmt.Errorf("failed to get label selectors from strategy's params: %+v", err)
		}
	}

	if args.PodRestartThreshold < 1 {
		return fmt.Errorf("invalid PodsHavingTooManyRestarts threshold")
	}

	allowedStates := sets.New(
		// Pod phases:
		string(v1.PodRunning),

		// Container state reasons:
		"CrashLoopBackOff",
	)

	if !allowedStates.HasAll(args.States...) {
		return fmt.Errorf("states must be one of %v", allowedStates.UnsortedList())
	}

	return nil
}
