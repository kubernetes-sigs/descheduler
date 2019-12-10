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
	clientset "k8s.io/client-go/kubernetes"

	// install the componentconfig api so we get its defaulting and conversion functions
	"sigs.k8s.io/descheduler/pkg/apis/componentconfig"
	"sigs.k8s.io/descheduler/pkg/apis/componentconfig/v1alpha1"
	deschedulerscheme "sigs.k8s.io/descheduler/pkg/descheduler/scheme"

	"github.com/spf13/pflag"
)

// DeschedulerServer configuration
type DeschedulerServer struct {
	componentconfig.DeschedulerConfiguration
	Client clientset.Interface
}

// NewDeschedulerServer creates a new DeschedulerServer with default parameters
func NewDeschedulerServer() *DeschedulerServer {
	versioned := v1alpha1.DeschedulerConfiguration{}
	deschedulerscheme.Scheme.Default(&versioned)
	cfg := componentconfig.DeschedulerConfiguration{}
	deschedulerscheme.Scheme.Convert(versioned, &cfg, nil)
	s := DeschedulerServer{
		DeschedulerConfiguration: cfg,
	}
	return &s
}

// AddFlags adds flags for a specific SchedulerServer to the specified FlagSet
func (rs *DeschedulerServer) AddFlags(fs *pflag.FlagSet) {
	fs.DurationVar(&rs.DeschedulingInterval, "descheduling-interval", rs.DeschedulingInterval, "time interval between two consecutive descheduler executions")
	fs.StringVar(&rs.KubeconfigFile, "kubeconfig", rs.KubeconfigFile, "File with  kube configuration.")
	fs.StringVar(&rs.PolicyConfigFile, "policy-config-file", rs.PolicyConfigFile, "File with descheduler policy configuration.")
	fs.BoolVar(&rs.DryRun, "dry-run", rs.DryRun, "execute descheduler in dry run mode.")
	// node-selector query causes descheduler to run only on nodes that matches the node labels in the query
	fs.StringVar(&rs.NodeSelector, "node-selector", rs.NodeSelector, "Selector (label query) to filter on, supports '=', '==', and '!='.(e.g. -l key1=value1,key2=value2)")
	// pod-selector query causes descheduler to only evict pods that match the pod labels in the query
	fs.StringVar(&rs.PodSelector, "pod-selector", rs.PodSelector, "Selector (label query) to filter on, supports '=', '==', and '!='.(e.g. -l key1=value1,key2=value2)")

	// max-no-pods-to-evict limits the maximum number of pods to be evicted per node by descheduler.
	fs.IntVar(&rs.MaxNoOfPodsToEvictPerNode, "max-pods-to-evict-per-node", rs.MaxNoOfPodsToEvictPerNode, "Limits the maximum number of pods to be evicted per node by descheduler")
	// evict-local-storage-pods allows eviction of pods that are using local storage. This is false by default.
	fs.BoolVar(&rs.EvictLocalStoragePods, "evict-local-storage-pods", rs.EvictLocalStoragePods, "Enables evicting pods using local storage by descheduler")
}
