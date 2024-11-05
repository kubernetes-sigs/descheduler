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

// Package options provides the descheduler flags
package options

import (
	"time"

	"github.com/spf13/pflag"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiserveroptions "k8s.io/apiserver/pkg/server/options"
	clientset "k8s.io/client-go/kubernetes"
	componentbaseconfig "k8s.io/component-base/config"
	componentbaseoptions "k8s.io/component-base/config/options"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"

	"sigs.k8s.io/descheduler/pkg/apis/componentconfig"
	"sigs.k8s.io/descheduler/pkg/apis/componentconfig/v1alpha1"
	deschedulerscheme "sigs.k8s.io/descheduler/pkg/descheduler/scheme"
	"sigs.k8s.io/descheduler/pkg/tracing"
)

const (
	DefaultDeschedulerPort = 10258
)

// DeschedulerServer configuration
type DeschedulerServer struct {
	componentconfig.DeschedulerConfiguration

	Client         clientset.Interface
	EventClient    clientset.Interface
	MetricsClient  metricsclient.Interface
	SecureServing  *apiserveroptions.SecureServingOptionsWithLoopback
	DisableMetrics bool
	EnableHTTP2    bool
}

// NewDeschedulerServer creates a new DeschedulerServer with default parameters
func NewDeschedulerServer() (*DeschedulerServer, error) {
	cfg, err := newDefaultComponentConfig()
	if err != nil {
		return nil, err
	}

	secureServing := apiserveroptions.NewSecureServingOptions().WithLoopback()
	secureServing.BindPort = DefaultDeschedulerPort

	return &DeschedulerServer{
		DeschedulerConfiguration: *cfg,
		SecureServing:            secureServing,
	}, nil
}

func newDefaultComponentConfig() (*componentconfig.DeschedulerConfiguration, error) {
	versionedCfg := v1alpha1.DeschedulerConfiguration{
		LeaderElection: componentbaseconfig.LeaderElectionConfiguration{
			LeaderElect:       false,
			LeaseDuration:     metav1.Duration{Duration: 137 * time.Second},
			RenewDeadline:     metav1.Duration{Duration: 107 * time.Second},
			RetryPeriod:       metav1.Duration{Duration: 26 * time.Second},
			ResourceLock:      "leases",
			ResourceName:      "descheduler",
			ResourceNamespace: "kube-system",
		},
	}
	deschedulerscheme.Scheme.Default(&versionedCfg)
	cfg := componentconfig.DeschedulerConfiguration{
		Tracing: componentconfig.TracingConfiguration{},
	}
	if err := deschedulerscheme.Scheme.Convert(&versionedCfg, &cfg, nil); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// AddFlags adds flags for a specific SchedulerServer to the specified FlagSet
func (rs *DeschedulerServer) AddFlags(fs *pflag.FlagSet) {
	fs.DurationVar(&rs.DeschedulingInterval, "descheduling-interval", rs.DeschedulingInterval, "Time interval between two consecutive descheduler executions. Setting this value instructs the descheduler to run in a continuous loop at the interval specified.")
	fs.StringVar(&rs.ClientConnection.Kubeconfig, "kubeconfig", rs.ClientConnection.Kubeconfig, "File with kube configuration. Deprecated, use client-connection-kubeconfig instead.")
	fs.StringVar(&rs.ClientConnection.Kubeconfig, "client-connection-kubeconfig", rs.ClientConnection.Kubeconfig, "File path to kube configuration for interacting with kubernetes apiserver.")
	fs.Float32Var(&rs.ClientConnection.QPS, "client-connection-qps", rs.ClientConnection.QPS, "QPS to use for interacting with kubernetes apiserver.")
	fs.Int32Var(&rs.ClientConnection.Burst, "client-connection-burst", rs.ClientConnection.Burst, "Burst to use for interacting with kubernetes apiserver.")
	fs.StringVar(&rs.PolicyConfigFile, "policy-config-file", rs.PolicyConfigFile, "File with descheduler policy configuration.")
	fs.BoolVar(&rs.DryRun, "dry-run", rs.DryRun, "Execute descheduler in dry run mode.")
	fs.BoolVar(&rs.DisableMetrics, "disable-metrics", rs.DisableMetrics, "Disables metrics. The metrics are by default served through https://localhost:10258/metrics. Secure address, resp. port can be changed through --bind-address, resp. --secure-port flags.")
	fs.StringVar(&rs.Tracing.CollectorEndpoint, "otel-collector-endpoint", "", "Set this flag to the OpenTelemetry Collector Service Address")
	fs.StringVar(&rs.Tracing.TransportCert, "otel-transport-ca-cert", "", "Path of the CA Cert that can be used to generate the client Certificate for establishing secure connection to the OTEL in gRPC mode")
	fs.StringVar(&rs.Tracing.ServiceName, "otel-service-name", tracing.DefaultServiceName, "OTEL Trace name to be used with the resources")
	fs.StringVar(&rs.Tracing.ServiceNamespace, "otel-trace-namespace", "", "OTEL Trace namespace to be used with the resources")
	fs.Float64Var(&rs.Tracing.SampleRate, "otel-sample-rate", 1.0, "Sample rate to collect the Traces")
	fs.BoolVar(&rs.Tracing.FallbackToNoOpProviderOnError, "otel-fallback-no-op-on-error", false, "Fallback to NoOp Tracer in case of error")
	fs.BoolVar(&rs.EnableHTTP2, "enable-http2", false, "If http/2 should be enabled for the metrics and health check")

	componentbaseoptions.BindLeaderElectionFlags(&rs.LeaderElection, fs)

	rs.SecureServing.AddFlags(fs)
}
