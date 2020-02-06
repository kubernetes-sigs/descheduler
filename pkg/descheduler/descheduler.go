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

	"k8s.io/klog"

	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/descheduler/cmd/descheduler/app/options"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/client"
	eutils "sigs.k8s.io/descheduler/pkg/descheduler/evictions/utils"
	nodeutil "sigs.k8s.io/descheduler/pkg/descheduler/node"
	"sigs.k8s.io/descheduler/pkg/descheduler/strategies"
	"sigs.k8s.io/descheduler/pkg/utils"
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
		return fmt.Errorf("deschedulerPolicy is nil")
	}

	return RunDeschedulerStrategies(rs, deschedulerPolicy)
}

func RunDeschedulerStrategies(rs *options.DeschedulerServer, deschedulerPolicy *api.DeschedulerPolicy) error {
	evictionPolicyGroupVersion, err := eutils.SupportEviction(rs.Client)
	if err != nil || len(evictionPolicyGroupVersion) == 0 {
		return err
	}

	stopChannel := make(chan struct{})
	nodes, err := nodeutil.ReadyNodes(rs.Client, rs.NodeSelector, stopChannel)
	if err != nil {
		return err
	}

	if len(nodes) <= 1 {
		klog.V(1).Infof("The cluster size is 0 or 1 meaning eviction causes service disruption or degradation. So aborting..")
		return nil
	}

	nodePodCount := utils.InitializeNodePodCount(nodes)
	wait.Until(func() {
		strategies.RemoveDuplicatePods(rs, deschedulerPolicy.Strategies["RemoveDuplicates"], evictionPolicyGroupVersion, nodes, nodePodCount)
		strategies.LowNodeUtilization(rs, deschedulerPolicy.Strategies["LowNodeUtilization"], evictionPolicyGroupVersion, nodes, nodePodCount)
		strategies.RemovePodsViolatingInterPodAntiAffinity(rs, deschedulerPolicy.Strategies["RemovePodsViolatingInterPodAntiAffinity"], evictionPolicyGroupVersion, nodes, nodePodCount)
		strategies.RemovePodsViolatingNodeAffinity(rs, deschedulerPolicy.Strategies["RemovePodsViolatingNodeAffinity"], evictionPolicyGroupVersion, nodes, nodePodCount)
		strategies.RemovePodsViolatingNodeTaints(rs, deschedulerPolicy.Strategies["RemovePodsViolatingNodeTaints"], evictionPolicyGroupVersion, nodes, nodePodCount)

		// If there was no interval specified, send a signal to the stopChannel to end the wait.Until loop after 1 iteration
		if rs.DeschedulingInterval.Seconds() == 0 {
			close(stopChannel)
		}
	}, rs.DeschedulingInterval, stopChannel)

	return nil
}
