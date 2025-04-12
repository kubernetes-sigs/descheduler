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

package nodeutilization

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/descheduler/pkg/api"
)

func ValidateHighNodeUtilizationArgs(obj runtime.Object) error {
	args := obj.(*HighNodeUtilizationArgs)
	// only exclude can be set, or not at all
	if args.EvictableNamespaces != nil && len(args.EvictableNamespaces.Include) > 0 {
		return fmt.Errorf("only Exclude namespaces can be set, inclusion is not supported")
	}
	err := validateThresholds(args.Thresholds)
	if err != nil {
		return err
	}
	// make sure we know about the eviction modes defined by the user.
	return validateEvictionModes(args.EvictionModes)
}

// validateEvictionModes checks if the eviction modes are valid/known
// to the descheduler.
func validateEvictionModes(modes []EvictionMode) error {
	// we are using this approach to make the code more extensible
	// in the future.
	validModes := map[EvictionMode]bool{
		EvictionModeOnlyThresholdingResources: true,
	}

	for _, mode := range modes {
		if validModes[mode] {
			continue
		}
		return fmt.Errorf("invalid eviction mode %s", mode)
	}
	return nil
}

func ValidateLowNodeUtilizationArgs(obj runtime.Object) error {
	args := obj.(*LowNodeUtilizationArgs)
	// only exclude can be set, or not at all
	if args.EvictableNamespaces != nil && len(args.EvictableNamespaces.Include) > 0 {
		return fmt.Errorf("only Exclude namespaces can be set, inclusion is not supported")
	}
	err := validateLowNodeUtilizationThresholds(args.Thresholds, args.TargetThresholds, args.UseDeviationThresholds)
	if err != nil {
		return err
	}
	if args.MetricsUtilization != nil {
		if args.MetricsUtilization.Source == api.KubernetesMetrics && args.MetricsUtilization.MetricsServer {
			return fmt.Errorf("it is not allowed to set both %q source and metricsServer", api.KubernetesMetrics)
		}
		if args.MetricsUtilization.Source == api.KubernetesMetrics && args.MetricsUtilization.Prometheus != nil {
			return fmt.Errorf("prometheus configuration is not allowed to set when source is set to %q", api.KubernetesMetrics)
		}
		if args.MetricsUtilization.Source == api.PrometheusMetrics && (args.MetricsUtilization.Prometheus == nil || args.MetricsUtilization.Prometheus.Query == "") {
			return fmt.Errorf("prometheus query is required when metrics source is set to %q", api.PrometheusMetrics)
		}
	}
	return nil
}

func validateLowNodeUtilizationThresholds(thresholds, targetThresholds api.ResourceThresholds, useDeviationThresholds bool) error {
	// validate thresholds and targetThresholds config
	if err := validateThresholds(thresholds); err != nil {
		return fmt.Errorf("thresholds config is not valid: %v", err)
	}
	if err := validateThresholds(targetThresholds); err != nil {
		return fmt.Errorf("targetThresholds config is not valid: %v", err)
	}

	// validate if thresholds and targetThresholds have same resources configured
	if len(thresholds) != len(targetThresholds) {
		return fmt.Errorf("thresholds and targetThresholds configured different resources")
	}
	for resourceName, value := range thresholds {
		if targetValue, ok := targetThresholds[resourceName]; !ok {
			return fmt.Errorf("thresholds and targetThresholds configured different resources")
		} else if value > targetValue && !useDeviationThresholds {
			return fmt.Errorf("thresholds' %v percentage is greater than targetThresholds'", resourceName)
		}
	}
	return nil
}

// validateThresholds checks if thresholds have valid resource name and resource percentage configured
func validateThresholds(thresholds api.ResourceThresholds) error {
	if len(thresholds) == 0 {
		return fmt.Errorf("no resource threshold is configured")
	}
	for name, percent := range thresholds {
		if percent < MinResourcePercentage || percent > MaxResourcePercentage {
			return fmt.Errorf("%v threshold not in [%v, %v] range", name, MinResourcePercentage, MaxResourcePercentage)
		}
	}
	return nil
}
