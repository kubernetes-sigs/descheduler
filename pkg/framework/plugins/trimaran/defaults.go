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
	"strconv"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	// Defaults for TargetLoadPacking plugin

	// Default 1 core CPU usage for containers without requests and limits i.e. Best Effort QoS.
	DefaultRequestsMilliCores int64 = 1000
	// DefaultRequestsMultiplier for containers without limits predicted as 1.5*requests i.e. Burstable QoS class
	DefaultRequestsMultiplier = "1.5"
	// DefaultTargetUtilizationPercent Recommended to keep -10 than desired limit.
	DefaultTargetUtilizationPercent int64 = 40

	// DefaultMetricProviderType is the Kubernetes metrics server
	DefaultMetricProviderType = KubernetesMetricsServer
	// DefaultInsecureSkipVerify is whether to skip the certificate verification
	DefaultInsecureSkipVerify = true
)

func addDefaultingFuncs(scheme *runtime.Scheme) error {
	return RegisterDefaults(scheme)
}

// SetDefaultTrimaranSpec sets the default parameters for common Trimaran plugins
func SetDefaultTrimaranSpec(args *TrimaranSpec) {
	if args.WatcherAddress == nil && args.MetricProvider.Type == "" {
		args.MetricProvider.Type = MetricProviderType(DefaultMetricProviderType)
	}
	if args.MetricProvider.Type == Prometheus && args.MetricProvider.InsecureSkipVerify == nil {
		args.MetricProvider.InsecureSkipVerify = &DefaultInsecureSkipVerify
	}
}

// SetDefaults_TargetLoadPackingArgs sets the default parameters for TargetLoadPacking plugin
func SetDefaults_TargetLoadPackingArgs(obj runtime.Object) {
	args := obj.(*TargetLoadPackingArgs)
	SetDefaultTrimaranSpec(&args.TrimaranSpec)
	if args.DefaultRequests == nil {
		args.DefaultRequests = v1.ResourceList{v1.ResourceCPU: resource.MustParse(
			strconv.FormatInt(DefaultRequestsMilliCores, 10) + "m")}
	}
	if args.DefaultRequestsMultiplier == nil {
		args.DefaultRequestsMultiplier = &DefaultRequestsMultiplier
	}
	if args.TargetUtilization == nil || *args.TargetUtilization <= 0 {
		args.TargetUtilization = &DefaultTargetUtilizationPercent
	}
}
