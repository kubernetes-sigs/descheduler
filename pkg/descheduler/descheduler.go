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

package descheduler

import (
	"fmt"

	"github.com/kubernetes-incubator/descheduler/cmd/descheduler/app/options"
	"github.com/kubernetes-incubator/descheduler/pkg/descheduler/client"
	eutils "github.com/kubernetes-incubator/descheduler/pkg/descheduler/evictions/utils"
	nodeutil "github.com/kubernetes-incubator/descheduler/pkg/descheduler/node"
	"github.com/kubernetes-incubator/descheduler/pkg/descheduler/strategies"
)

func Run(rs *options.DeschedulerServer) error {

	rsclient, err := client.CreateClient(rs.KubeconfigFile)
	if err != nil {
		return err
	}
	rs.Client = rsclient

	deschedulerPolicy, err := LoadPolicyConfig(rs.PolicyConfigFile)
	if err != nil {
		return err
	}
	if deschedulerPolicy == nil {
		return fmt.Errorf("\ndeschedulerPolicy is nil\n")

	}
	evictionPolicyGroupVersion, err := eutils.SupportEviction(rs.Client)
	if err != nil || len(evictionPolicyGroupVersion) == 0 {
		return err
	}

	stopChannel := make(chan struct{})
	nodes, err := nodeutil.ReadyNodes(rs.Client, stopChannel)
	if err != nil {
		return err
	}

	strategies.RemoveDuplicatePods(rs, deschedulerPolicy.Strategies["RemoveDuplicates"], evictionPolicyGroupVersion, nodes)
	strategies.LowNodeUtilization(rs, deschedulerPolicy.Strategies["LowNodeUtilization"], evictionPolicyGroupVersion, nodes)
	strategies.RemovePodsViolatingInterPodAntiAffinity(rs, deschedulerPolicy.Strategies["RemovePodsViolatingInterPodAntiAffinity"], evictionPolicyGroupVersion, nodes)

	return nil
}
