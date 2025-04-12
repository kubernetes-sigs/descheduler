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

package defaultevictor

import (
	"fmt"
	"slices"

	"k8s.io/klog/v2"

	"k8s.io/apimachinery/pkg/runtime"
)

func ValidateDefaultEvictorArgs(obj runtime.Object) error {
	args := obj.(*DefaultEvictorArgs)

	if args.PriorityThreshold != nil && args.PriorityThreshold.Value != nil && len(args.PriorityThreshold.Name) > 0 {
		return fmt.Errorf("priority threshold misconfigured, only one of priorityThreshold fields can be set")
	}

	if args.MinReplicas == 1 {
		klog.V(4).Info("DefaultEvictor minReplicas must be greater than 1 to check for min pods during eviction. This check will be ignored during eviction.")
	}

	if args.NoEvictionPolicy != "" {
		if args.NoEvictionPolicy != PreferredNoEvictionPolicy && args.NoEvictionPolicy != MandatoryNoEvictionPolicy {
			return fmt.Errorf("noEvictionPolicy accepts only %q values", []NoEvictionPolicy{PreferredNoEvictionPolicy, MandatoryNoEvictionPolicy})
		}
	}

	// check if any deprecated fields are set to true
	hasDeprecatedFields := args.EvictLocalStoragePods || args.EvictDaemonSetPods ||
		args.EvictSystemCriticalPods || args.IgnorePvcPods ||
		args.EvictFailedBarePods || args.IgnorePodsWithoutPDB

	// disallow mixing deprecated fields with PodProtections.ExtraEnabled and PodProtections.DefaultDisabled
	if hasDeprecatedFields && (len(args.PodProtections.ExtraEnabled) > 0 || len(args.PodProtections.DefaultDisabled) > 0) {
		return fmt.Errorf("cannot use Deprecated fields alongside PodProtections.ExtraEnabled or PodProtections.DefaultDisabled")
	}

	if len(args.PodProtections.ExtraEnabled) > 0 || len(args.PodProtections.DefaultDisabled) > 0 {

		for _, policy := range args.PodProtections.ExtraEnabled {
			if !slices.Contains(ExtraPodProtections, policy) {
				return fmt.Errorf("invalid pod protection policy in ExtraEnabled: %q. Valid options are: %v",
					string(policy), ExtraPodProtections)
			}
		}

		for _, policy := range args.PodProtections.DefaultDisabled {
			if !slices.Contains(DefaultPodProtections, policy) {
				return fmt.Errorf("invalid pod protection policy in DefaultDisabled: %q. Valid options are: %v",
					string(policy), DefaultPodProtections)
			}
		}
	}

	return nil
}
