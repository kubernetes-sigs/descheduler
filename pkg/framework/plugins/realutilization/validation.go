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

package realutilization

import (
	"fmt"

	v1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/descheduler/pkg/api"
)

func ValidateLowNodeRealUtilizationArgs(obj runtime.Object) error {
	args := obj.(*LowNodeRealUtilizationArgs)
	// only exclude can be set, or not at all
	if args.EvictableNamespaces != nil && len(args.EvictableNamespaces.Include) > 0 {
		return fmt.Errorf("only Exclude namespaces can be set, inclusion is not supported")
	}
	err := validateLowNodeRealUtilizationThresholds(args.Thresholds, args.TargetThresholds)
	if err != nil {
		return err
	}
	return nil
}

func validateLowNodeRealUtilizationThresholds(thresholds, targetThresholds api.ResourceThresholds) error {
	// validate thresholds and targetThresholds config
	if err := validateRealThresholds(thresholds); err != nil {
		return fmt.Errorf("thresholds config is not valid: %v", err)
	}
	if err := validateRealThresholds(targetThresholds); err != nil {
		return fmt.Errorf("targetThresholds config is not valid: %v", err)
	}

	// validate if thresholds and targetThresholds have same resources configured
	if len(thresholds) != len(targetThresholds) {
		return fmt.Errorf("thresholds and targetThresholds configured different resources")
	}
	for resourceName, value := range thresholds {
		if targetValue, ok := targetThresholds[resourceName]; !ok {
			return fmt.Errorf("thresholds and targetThresholds configured different resources")
		} else if value > targetValue {
			return fmt.Errorf("thresholds' %v percentage is greater than targetThresholds'", resourceName)
		}
	}
	return nil
}

var validResource = map[v1.ResourceName]bool{v1.ResourceCPU: true, v1.ResourceMemory: true}

// validateThresholds checks if thresholds have valid resource name and resource percentage configured
func validateRealThresholds(thresholds api.ResourceThresholds) error {
	if len(thresholds) == 0 {
		return fmt.Errorf("no resource threshold is configured")
	}
	for name, percent := range thresholds {
		if !validResource[name] {
			return fmt.Errorf("resource %v is invalid", name)
		}
		if percent < MinResourcePercentage || percent > MaxResourcePercentage {
			return fmt.Errorf("%v threshold not in [%v, %v] range", name, MinResourcePercentage, MaxResourcePercentage)
		}
	}
	return nil
}
