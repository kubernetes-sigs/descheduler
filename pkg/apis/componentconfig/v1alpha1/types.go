/*
Copyright 2017 The Kubernetes Authors.

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

package v1alpha1

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	componentbaseconfig "k8s.io/component-base/config"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type DeschedulerConfiguration struct {
	metav1.TypeMeta `json:",inline"`

	// Time interval for descheduler to run
	DeschedulingInterval time.Duration `json:"deschedulingInterval,omitempty"`

	// KubeconfigFile is path to kubeconfig file with authorization and master
	// location information.
	// Deprecated: Use clientConnection.kubeConfig instead.
	KubeconfigFile string `json:"kubeconfigFile"`

	// PolicyConfigFile is the filepath to the descheduler policy configuration.
	PolicyConfigFile string `json:"policyConfigFile,omitempty"`

	// Dry run
	DryRun bool `json:"dryRun,omitempty"`

	// Node selectors
	NodeSelector string `json:"nodeSelector,omitempty"`

	// MaxNoOfPodsToEvictPerNode restricts maximum of pods to be evicted per node.
	MaxNoOfPodsToEvictPerNode int `json:"maxNoOfPodsToEvictPerNode,omitempty"`

	// EvictLocalStoragePods allows pods using local storage to be evicted.
	EvictLocalStoragePods bool `json:"evictLocalStoragePods,omitempty"`

	// IgnorePVCPods sets whether PVC pods should be allowed to be evicted
	IgnorePVCPods bool `json:"ignorePvcPods,omitempty"`

	// Tracing is used to setup the required OTEL tracing configuration
	Tracing TracingConfiguration `json:"tracing,omitempty"`

	// LeaderElection starts Deployment using leader election loop
	LeaderElection componentbaseconfig.LeaderElectionConfiguration `json:"leaderElection,omitempty"`

	// ClientConnection specifies the kubeconfig file and client connection settings to use when communicating with the apiserver.
	// Refer to [ClientConnection](https://pkg.go.dev/k8s.io/kubernetes/pkg/apis/componentconfig#ClientConnectionConfiguration) for more information.
	ClientConnection componentbaseconfig.ClientConnectionConfiguration `json:"clientConnection,omitempty"`
}

type TracingConfiguration struct {
	// CollectorEndpoint is the address of the OpenTelemetry collector.
	// If not specified, tracing will be used NoopTraceProvider.
	CollectorEndpoint string `json:"collectorEndpoint"`
	// TransportCert is the path to the certificate file for the OpenTelemetry collector.
	// If not specified, provider will start in insecure mode.
	TransportCert string `json:"transportCert,omitempty"`
	// ServiceName is the name of the service to be used in the OpenTelemetry collector.
	// If not specified, the default value is "descheduler".
	ServiceName string `json:"serviceName,omitempty"`
	// ServiceNamespace is the namespace of the service to be used in the OpenTelemetry collector.
	// If not specified, tracing will be used default namespace.
	ServiceNamespace string `json:"serviceNamespace,omitempty"`
	// SampleRate is used to configure the sample rate of the OTEL trace collection. This value will
	// be used as the Base value with sample ratio. A value >= 1.0 will sample everything and < 0 will
	// not sample anything. Everything else is a percentage value.
	SampleRate float64 `json:"sampleRate"`
	// FallbackToNoOpProviderOnError can be set in case if you want your trace provider to fallback to
	// no op provider in case if the configured end point based provider can't be setup.
	FallbackToNoOpProviderOnError bool `json:"fallbackToNoOpProviderOnError"`
}
