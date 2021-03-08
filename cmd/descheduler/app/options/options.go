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
	"github.com/spf13/pflag"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	apiserveroptions "k8s.io/apiserver/pkg/server/options"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/component-base/logs"

	"sigs.k8s.io/descheduler/pkg/apis/componentconfig"
	"sigs.k8s.io/descheduler/pkg/apis/componentconfig/v1alpha1"
	deschedulerscheme "sigs.k8s.io/descheduler/pkg/descheduler/scheme"
)

const (
	DefaultDeschedulerPort = 10258
)

// DeschedulerServer configuration
type DeschedulerServer struct {
	componentconfig.DeschedulerConfiguration

	Client         clientset.Interface
	Logs           *logs.Options
	SecureServing  *apiserveroptions.SecureServingOptionsWithLoopback
	DisableMetrics bool
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
		Logs:                     logs.NewOptions(),
		SecureServing:            secureServing,
	}, nil
}

// Validation checks for DeschedulerServer.
func (s *DeschedulerServer) Validate() error {
	var errs []error
	errs = append(errs, s.Logs.Validate()...)
	return utilerrors.NewAggregate(errs)
}

func newDefaultComponentConfig() (*componentconfig.DeschedulerConfiguration, error) {
	versionedCfg := v1alpha1.DeschedulerConfiguration{}
	deschedulerscheme.Scheme.Default(&versionedCfg)
	cfg := componentconfig.DeschedulerConfiguration{}
	if err := deschedulerscheme.Scheme.Convert(&versionedCfg, &cfg, nil); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// AddFlags adds flags for a specific SchedulerServer to the specified FlagSet
func (rs *DeschedulerServer) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&rs.Logging.Format, "logging-format", "text", `Sets the log format. Permitted formats: "text", "json". Non-default formats don't honor these flags: --add-dir-header, --alsologtostderr, --log-backtrace-at, --log-dir, --log-file, --log-file-max-size, --logtostderr, --skip-headers, --skip-log-headers, --stderrthreshold, --log-flush-frequency.\nNon-default choices are currently alpha and subject to change without warning.`)
	fs.DurationVar(&rs.DeschedulingInterval, "descheduling-interval", rs.DeschedulingInterval, "Time interval between two consecutive descheduler executions. Setting this value instructs the descheduler to run in a continuous loop at the interval specified.")
	fs.StringVar(&rs.KubeconfigFile, "kubeconfig", rs.KubeconfigFile, "File with  kube configuration.")
	fs.StringVar(&rs.PolicyConfigFile, "policy-config-file", rs.PolicyConfigFile, "File with descheduler policy configuration.")
	fs.BoolVar(&rs.DryRun, "dry-run", rs.DryRun, "execute descheduler in dry run mode.")
	// node-selector query causes descheduler to run only on nodes that matches the node labels in the query
	fs.StringVar(&rs.NodeSelector, "node-selector", rs.NodeSelector, "DEPRECATED: selector (label query) to filter on, supports '=', '==', and '!='.(e.g. -l key1=value1,key2=value2)")
	// max-no-pods-to-evict limits the maximum number of pods to be evicted per node by descheduler.
	fs.IntVar(&rs.MaxNoOfPodsToEvictPerNode, "max-pods-to-evict-per-node", rs.MaxNoOfPodsToEvictPerNode, "DEPRECATED: limits the maximum number of pods to be evicted per node by descheduler")
	// evict-local-storage-pods allows eviction of pods that are using local storage. This is false by default.
	fs.BoolVar(&rs.EvictLocalStoragePods, "evict-local-storage-pods", rs.EvictLocalStoragePods, "DEPRECATED: enables evicting pods using local storage by descheduler")
	fs.BoolVar(&rs.DisableMetrics, "disable-metrics", rs.DisableMetrics, "Disables metrics. The metrics are by default served through https://localhost:10258/metrics. Secure address, resp. port can be changed through --bind-address, resp. --secure-port flags.")

	rs.SecureServing.AddFlags(fs)
}
