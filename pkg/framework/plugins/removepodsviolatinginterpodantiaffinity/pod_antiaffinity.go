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

package removepodsviolatinginterpodantiaffinity

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
	"sigs.k8s.io/descheduler/pkg/utils"

	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

const PluginName = "RemovePodsViolatingInterPodAntiAffinity"

// RemovePodsViolatingInterPodAntiAffinity evicts pods on the node which violate inter pod anti affinity
type RemovePodsViolatingInterPodAntiAffinity struct {
	logger    klog.Logger
	handle    frameworktypes.Handle
	args      *RemovePodsViolatingInterPodAntiAffinityArgs
	podFilter podutil.FilterFunc
}

var _ frameworktypes.DeschedulePlugin = &RemovePodsViolatingInterPodAntiAffinity{}

// New builds plugin from its arguments while passing a handle
func New(ctx context.Context, args runtime.Object, handle frameworktypes.Handle) (frameworktypes.Plugin, error) {
	interPodAntiAffinityArgs, ok := args.(*RemovePodsViolatingInterPodAntiAffinityArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type RemovePodsViolatingInterPodAntiAffinityArgs, got %T", args)
	}
	logger := klog.FromContext(ctx).WithValues("plugin", PluginName)

	var includedNamespaces, excludedNamespaces sets.Set[string]
	if interPodAntiAffinityArgs.Namespaces != nil {
		includedNamespaces = sets.New(interPodAntiAffinityArgs.Namespaces.Include...)
		excludedNamespaces = sets.New(interPodAntiAffinityArgs.Namespaces.Exclude...)
	}

	podFilter, err := podutil.NewOptions().
		WithNamespaces(includedNamespaces).
		WithoutNamespaces(excludedNamespaces).
		WithLabelSelector(interPodAntiAffinityArgs.LabelSelector).
		BuildFilterFunc()
	if err != nil {
		return nil, fmt.Errorf("error initializing pod filter function: %v", err)
	}

	return &RemovePodsViolatingInterPodAntiAffinity{
		logger:    logger,
		handle:    handle,
		podFilter: podFilter,
		args:      interPodAntiAffinityArgs,
	}, nil
}

// Name retrieves the plugin name
func (d *RemovePodsViolatingInterPodAntiAffinity) Name() string {
	return PluginName
}

func (d *RemovePodsViolatingInterPodAntiAffinity) Deschedule(ctx context.Context, nodes []*v1.Node) *frameworktypes.Status {
	logger := klog.FromContext(klog.NewContext(ctx, d.logger)).WithValues("ExtensionPoint", frameworktypes.DescheduleExtensionPoint)
	pods, err := podutil.ListPodsOnNodes(nodes, d.handle.GetPodsAssignedToNodeFunc(), d.podFilter)
	if err != nil {
		return &frameworktypes.Status{
			Err: fmt.Errorf("error listing all pods: %v", err),
		}
	}

	podsInANamespace := podutil.GroupByNamespace(pods)
	podsOnANode := podutil.GroupByNodeName(pods)
	nodeMap := utils.CreateNodeMap(nodes)

loop:
	for _, node := range nodes {
		logger.V(2).Info("Processing node", "node", klog.KObj(node))
		pods := podsOnANode[node.Name]
		// sort the evict-able Pods based on priority, if there are multiple pods with same priority, they are sorted based on QoS tiers.
		podutil.SortPodsBasedOnPriorityLowToHigh(pods)
		totalPods := len(pods)
		for i := 0; i < totalPods; i++ {
			if utils.CheckPodsWithAntiAffinityExist(pods[i], podsInANamespace, nodeMap) {
				if d.handle.Evictor().Filter(pods[i]) && d.handle.Evictor().PreEvictionFilter(pods[i]) {
					err := d.handle.Evictor().Evict(ctx, pods[i], evictions.EvictOptions{StrategyName: PluginName})
					if err == nil {
						// Since the current pod is evicted all other pods which have anti-affinity with this
						// pod need not be evicted.
						// Update allPods.
						podsInANamespace = removePodFromNamespaceMap(pods[i], podsInANamespace)
						pods = append(pods[:i], pods[i+1:]...)
						i--
						totalPods--
						continue
					}
					switch err.(type) {
					case *evictions.EvictionNodeLimitError:
						continue loop
					case *evictions.EvictionTotalLimitError:
						return nil
					default:
						logger.Error(err, "eviction failed")
					}
				}
			}
		}
	}
	return nil
}

func removePodFromNamespaceMap(podToRemove *v1.Pod, podMap map[string][]*v1.Pod) map[string][]*v1.Pod {
	podList, ok := podMap[podToRemove.Namespace]
	if !ok {
		return podMap
	}
	for i := 0; i < len(podList); i++ {
		podToCheck := podList[i]
		if podToRemove.Name == podToCheck.Name {
			podMap[podToRemove.Namespace] = append(podList[:i], podList[i+1:]...)
			return podMap
		}
	}
	return podMap
}
