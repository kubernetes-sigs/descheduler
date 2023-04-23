/*
Copyright 2023 The Kubernetes Authors.
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

package trimaran

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
)

// ValidateTargetLoadPackingArgs validates TargetLoadPacking arguments
func ValidateTargetLoadPackingArgs(obj runtime.Object) error {
	args := obj.(*TargetLoadPackingArgs)
	if err := ValidateTrimaranSpec(args.TrimaranSpec); err != nil {
		return err
	}
	return nil
}

// ValidateMetricProviderSpec validates ValidateMetricProvider
func ValidateMetricProviderSpec(metricsProviderSpec MetricProviderSpec) error {
	metricProviderType := metricsProviderSpec.Type
	if validMetricProviderType := metricProviderType == KubernetesMetricsServer || metricProviderType == Prometheus || metricProviderType == SignalFx; !validMetricProviderType {
		return fmt.Errorf("invalid MetricProvider.Type, got %T", metricProviderType)
	}
	return nil
}

// ValidateTrimaranSpec validates TrimaranSpec
func ValidateTrimaranSpec(trimaranSpec TrimaranSpec) error {
	if trimaranSpec.WatcherAddress != nil && *trimaranSpec.WatcherAddress != "" {
		return nil
	}
	return ValidateMetricProviderSpec(trimaranSpec.MetricProvider)
}
