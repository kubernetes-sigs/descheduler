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

// Package options provides the rescheduler flags
package options

import (
	//"fmt"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/clientset_generated/clientset"

	// install the componentconfig api so we get its defaulting and conversion functions
	"github.com/aveshagarwal/rescheduler/pkg/apis/componentconfig"
	_ "github.com/aveshagarwal/rescheduler/pkg/apis/componentconfig/install"
	"github.com/aveshagarwal/rescheduler/pkg/apis/componentconfig/v1alpha1"

	"github.com/spf13/pflag"
)

// ReschedulerServer configuration
type ReschedulerServer struct {
	componentconfig.ReschedulerConfiguration
	Client clientset.Interface
}

// NewReschedulerServer creates a new ReschedulerServer with default parameters
func NewReschedulerServer() *ReschedulerServer {
	versioned := v1alpha1.ReschedulerConfiguration{}
	api.Scheme.Default(&versioned)
	cfg := componentconfig.ReschedulerConfiguration{}
	api.Scheme.Convert(versioned, &cfg, nil)
	s := ReschedulerServer{
		ReschedulerConfiguration: cfg,
	}
	return &s
}

// AddFlags adds flags for a specific SchedulerServer to the specified FlagSet
func (rs *ReschedulerServer) AddFlags(fs *pflag.FlagSet) {
	fs.DurationVar(&rs.ReschedulingInterval, "rescheduling-interval", rs.ReschedulingInterval, "time interval between two consecutive rescheduler executions")
	fs.StringVar(&rs.KubeconfigFile, "kubeconfig-file", rs.KubeconfigFile, "File with  kube configuration.")
	fs.StringVar(&rs.PolicyConfigFile, "policy-config-file", rs.PolicyConfigFile, "File with rescheduler policy configuration.")
}
