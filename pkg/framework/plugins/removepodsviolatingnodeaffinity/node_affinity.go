/*
Copyright 2022 The Kubernetes Authors.
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

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"

	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	nodeutil "sigs.k8s.io/descheduler/pkg/descheduler/node"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/framework"
	"sigs.k8s.io/descheduler/pkg/tracing"
)

const PluginName = "RemovePodsViolatingNodeAffinity"

// RemovePodsViolatingNodeAffinity evicts pods on the node which violate node affinity
type RemovePodsViolatingNodeAffinity struct {
	handle    framework.Handle
	args      *RemovePodsViolatingNodeAffinityArgs
	podFilter podutil.FilterFunc
}

var _ framework.DeschedulePlugin = &RemovePodsViolatingNodeAffinity{}

// New builds plugin from its arguments while passing a handle
func New(args runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	nodeAffinityArgs, ok := args.(*RemovePodsViolatingNodeAffinityArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type RemovePodsViolatingNodeAffinityArgs, got %T", args)
	}

	var includedNamespaces, excludedNamespaces sets.String
	if nodeAffinityArgs.Namespaces != nil {
		includedNamespaces = sets.NewString(nodeAffinityArgs.Namespaces.Include...)
		excludedNamespaces = sets.NewString(nodeAffinityArgs.Namespaces.Exclude...)
	}

	// We can combine Filter and PreEvictionFilter since for this strategy it does not matter where we run PreEvictionFilter
	podFilter, err := podutil.NewOptions().
		WithFilter(podutil.WrapFilterFuncs(handle.Evictor().Filter, handle.Evictor().PreEvictionFilter)).
		WithNamespaces(includedNamespaces).
		WithoutNamespaces(excludedNamespaces).
		WithLabelSelector(nodeAffinityArgs.LabelSelector).
		BuildFilterFunc()
	if err != nil {
		return nil, fmt.Errorf("error initializing pod filter function: %v", err)
	}

	return &RemovePodsViolatingNodeAffinity{
		handle:    handle,
		podFilter: podFilter,
		args:      nodeAffinityArgs,
	}, nil
}

// Name retrieves the plugin name
func (d *RemovePodsViolatingNodeAffinity) Name() string {
	return PluginName
}

func (d *RemovePodsViolatingNodeAffinity) Deschedule(ctx context.Context, nodes []*v1.Node) (status *framework.Status) {
	ctx, span, spanCloser := tracing.StartSpan(ctx, tracing.DescheduleOperation, d.Name())
	defer func() {
		if status != nil && status.Err != nil {
			span.RecordError(status.Err)
		}
		spanCloser()
	}()
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
						return d.handle.Evictor().Filter(pod) &&
							!nodeutil.PodFitsCurrentNode(d.handle.GetPodsAssignedToNodeFunc(), pod, node) &&
							nodeutil.PodFitsAnyNode(d.handle.GetPodsAssignedToNodeFunc(), pod, nodes)
					}),
				)
				if err != nil {
					status = &framework.Status{
						Err: fmt.Errorf("error listing pods on a node: %v", err),
					}
					return status
				}

				for _, pod := range pods {
					if pod.Spec.Affinity != nil && pod.Spec.Affinity.NodeAffinity != nil && pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
						klog.V(1).InfoS("Evicting pod", "pod", klog.KObj(pod))
						d.handle.Evictor().Evict(ctx, pod, evictions.EvictOptions{})
						if d.handle.Evictor().NodeLimitExceeded(node) {
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
