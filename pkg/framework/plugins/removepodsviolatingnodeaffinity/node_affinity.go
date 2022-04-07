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

package removepodsviolatingnodeaffinity

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	nodeutil "sigs.k8s.io/descheduler/pkg/descheduler/node"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/framework"
	"sigs.k8s.io/descheduler/pkg/utils"
)

const PluginName = "RemovePodsViolatingNodeAffinity"

// RemovePodsViolatingNodeAffinity evicts pods on nodes which violate node affinity
type RemovePodsViolatingNodeAffinity struct {
	handle    framework.Handle
	args      *framework.RemovePodsViolatingNodeAffinityArg
	podFilter podutil.FilterFunc
}

var _ framework.Plugin = &RemovePodsViolatingNodeAffinity{}
var _ framework.DeschedulePlugin = &RemovePodsViolatingNodeAffinity{}

func New(args runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	nodeAffinityArg, ok := args.(*framework.RemovePodsViolatingNodeAffinityArg)
	if !ok {
		return nil, fmt.Errorf("want args to be of type RemovePodsViolatingNodeAffinityArg, got %T", args)
	}

	if err := framework.ValidateCommonArgs(nodeAffinityArg.CommonArgs); err != nil {
		return nil, err
	}

	if len(nodeAffinityArg.NodeAffinityType) == 0 {
		return nil, fmt.Errorf("NodeAffinityType is empty")
	}

	thresholdPriority, err := utils.GetPriorityValueFromPriorityThreshold(context.TODO(), handle.ClientSet(), nodeAffinityArg.PriorityThreshold)
	if err != nil {
		return nil, fmt.Errorf("failed to get priority threshold: %v", err)
	}

	evictable := handle.PodEvictor().Evictable(
		evictions.WithPriorityThreshold(thresholdPriority),
		evictions.WithNodeFit(nodeAffinityArg.NodeFit),
	)

	var includedNamespaces, excludedNamespaces sets.String
	if nodeAffinityArg.Namespaces != nil {
		includedNamespaces = sets.NewString(nodeAffinityArg.Namespaces.Include...)
		excludedNamespaces = sets.NewString(nodeAffinityArg.Namespaces.Exclude...)
	}

	podFilter, err := podutil.NewOptions().
		WithFilter(evictable.IsEvictable).
		WithNamespaces(includedNamespaces).
		WithoutNamespaces(excludedNamespaces).
		WithLabelSelector(nodeAffinityArg.LabelSelector).
		BuildFilterFunc()
	if err != nil {
		return nil, fmt.Errorf("error initializing pod filter function: %v", err)
	}

	return &RemovePodsViolatingNodeAffinity{
		handle:    handle,
		podFilter: podFilter,
		args:      nodeAffinityArg,
	}, nil
}

func (d *RemovePodsViolatingNodeAffinity) Name() string {
	return PluginName
}

func (d *RemovePodsViolatingNodeAffinity) Deschedule(ctx context.Context, nodes []*v1.Node) *framework.Status {
	for _, nodeAffinity := range d.args.NodeAffinityType {
		klog.V(2).InfoS("Executing for nodeAffinityType", "nodeAffinity", nodeAffinity)

		switch nodeAffinity {
		case "requiredDuringSchedulingIgnoredDuringExecution":
			for _, node := range nodes {
				klog.V(1).InfoS("Processing node", "node", klog.KObj(node))

				pods, err := podutil.ListPodsOnANode(
					node.Name,
					d.handle.GetPodsAssignedToNodeFunc(),
					podutil.WrapFilterFuncs(d.podFilter, func(pod *v1.Pod) bool {
						return !nodeutil.PodFitsCurrentNode(pod, node) &&
							nodeutil.PodFitsAnyNode(pod, nodes)
					}),
				)
				if err != nil {
					klog.ErrorS(err, "Failed to get pods", "node", klog.KObj(node))
				}

				for _, pod := range pods {
					if pod.Spec.Affinity != nil && pod.Spec.Affinity.NodeAffinity != nil && pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
						klog.V(1).InfoS("Evicting pod", "pod", klog.KObj(pod))
						if _, err := d.handle.PodEvictor().EvictPod(ctx, pod, node, "NodeAffinity"); err != nil {
							klog.ErrorS(err, "Error evicting pod")
							break
						}
					}
				}
			}
		default:
			klog.ErrorS(nil, "Invalid nodeAffinityType", "nodeAffinity", nodeAffinity)
		}
	}
	return nil
}
