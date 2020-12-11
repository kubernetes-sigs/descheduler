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
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"sigs.k8s.io/descheduler/cmd/descheduler/app/options"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/client"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	eutils "sigs.k8s.io/descheduler/pkg/descheduler/evictions/utils"
	nodeutil "sigs.k8s.io/descheduler/pkg/descheduler/node"
	"sigs.k8s.io/descheduler/pkg/descheduler/strategies"
)

func Run(rs *options.DeschedulerServer) error {
	ctx := context.Background()
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

	evictionPolicyGroupVersion, err := eutils.SupportEviction(rs.Client)
	if err != nil || len(evictionPolicyGroupVersion) == 0 {
		return err
	}

	stopChannel := make(chan struct{})
	return RunDeschedulerStrategies(ctx, rs, deschedulerPolicy, evictionPolicyGroupVersion, stopChannel)
}

type strategyFunction func(ctx context.Context, client clientset.Interface, strategy api.DeschedulerStrategy, nodes []*v1.Node, podEvictor *evictions.PodEvictor)

func RunDeschedulerStrategies(ctx context.Context, rs *options.DeschedulerServer, deschedulerPolicy *api.DeschedulerPolicy, evictionPolicyGroupVersion string, stopChannel chan struct{}) error {
	sharedInformerFactory := informers.NewSharedInformerFactory(rs.Client, 0)
	nodeInformer := sharedInformerFactory.Core().V1().Nodes()

	sharedInformerFactory.Start(stopChannel)
	sharedInformerFactory.WaitForCacheSync(stopChannel)

	strategyFuncs := map[string]strategyFunction{
		"RemoveDuplicates":                            strategies.RemoveDuplicatePods,
		"LowNodeUtilization":                          strategies.LowNodeUtilization,
		"RemovePodsViolatingInterPodAntiAffinity":     strategies.RemovePodsViolatingInterPodAntiAffinity,
		"RemovePodsViolatingNodeAffinity":             strategies.RemovePodsViolatingNodeAffinity,
		"RemovePodsViolatingNodeTaints":               strategies.RemovePodsViolatingNodeTaints,
		"RemovePodsHavingTooManyRestarts":             strategies.RemovePodsHavingTooManyRestarts,
		"PodLifeTime":                                 strategies.PodLifeTime,
		"RemovePodsViolatingTopologySpreadConstraint": strategies.RemovePodsViolatingTopologySpreadConstraint,
	}

	nodeSelector := rs.NodeSelector
	if deschedulerPolicy.NodeSelector != nil {
		nodeSelector = *deschedulerPolicy.NodeSelector
	}

	evictLocalStoragePods := rs.EvictLocalStoragePods
	if deschedulerPolicy.EvictLocalStoragePods != nil {
		evictLocalStoragePods = *deschedulerPolicy.EvictLocalStoragePods
	}

	maxNoOfPodsToEvictPerNode := rs.MaxNoOfPodsToEvictPerNode
	if deschedulerPolicy.MaxNoOfPodsToEvictPerNode != nil {
		maxNoOfPodsToEvictPerNode = *deschedulerPolicy.MaxNoOfPodsToEvictPerNode
	}

	wait.Until(func() {
		nodes, totalReadyNodeNum, err := nodeutil.ReadyNodes(ctx, rs.Client, nodeInformer, nodeSelector)
		if err != nil {
			klog.V(1).InfoS("Unable to get ready nodes", "err", err)
			close(stopChannel)
			return
		}

		if len(nodes) == 0 || totalReadyNodeNum <= 1 {
			klog.V(1).InfoS("Aborting given there's no enough ready node for eviction",
				"total ready node num", totalReadyNodeNum, "selected node num", len(nodes))
			close(stopChannel)
			return
		}

		podEvictor := evictions.NewPodEvictor(
			rs.Client,
			evictionPolicyGroupVersion,
			rs.DryRun,
			maxNoOfPodsToEvictPerNode,
			nodes,
			evictLocalStoragePods,
		)

		for name, f := range strategyFuncs {
			if strategy := deschedulerPolicy.Strategies[api.StrategyName(name)]; strategy.Enabled {
				f(ctx, rs.Client, strategy, nodes, podEvictor)
			}
		}

		// If there was no interval specified, send a signal to the stopChannel to end the wait.Until loop after 1 iteration
		if rs.DeschedulingInterval.Seconds() == 0 {
			close(stopChannel)
		}
	}, rs.DeschedulingInterval, stopChannel)

	return nil
}
