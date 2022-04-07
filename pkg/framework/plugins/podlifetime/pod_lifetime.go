/*
Copyright 2020 The Kubernetes Authors.

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

package podlifetime

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/framework"
	"sigs.k8s.io/descheduler/pkg/utils"
)

const PluginName = "PodLifeTime"

// PodLifeTime evicts pods on nodes that were created more than strategy.Params.MaxPodLifeTimeSeconds seconds ago.
type PodLifeTime struct {
	handle    framework.Handle
	args      *framework.PodLifeTimeArgs
	podFilter podutil.FilterFunc
}

var _ framework.Plugin = &PodLifeTime{}
var _ framework.DeschedulePlugin = &PodLifeTime{}

func New(args runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	lifetimeArgs, ok := args.(*framework.PodLifeTimeArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type PodLifeTimeArgs, got %T", args)
	}

	if err := framework.ValidateCommonArgs(lifetimeArgs.CommonArgs); err != nil {
		return nil, err
	}

	if lifetimeArgs.MaxPodLifeTimeSeconds == nil {
		return nil, fmt.Errorf("maxPodLifeTimeSeconds not set")
	}

	if lifetimeArgs.PodStatusPhases != nil {
		for _, phase := range lifetimeArgs.PodStatusPhases {
			if phase != string(v1.PodPending) && phase != string(v1.PodRunning) {
				return nil, fmt.Errorf("only Pending and Running phases are supported in PodLifeTime")
			}
		}
	}

	thresholdPriority, err := utils.GetPriorityValueFromPriorityThreshold(context.TODO(), handle.ClientSet(), lifetimeArgs.PriorityThreshold)
	if err != nil {
		return nil, fmt.Errorf("failed to get priority threshold: %v", err)
	}

	evictable := handle.PodEvictor().Evictable(
		evictions.WithPriorityThreshold(thresholdPriority),
	)

	filter := evictable.IsEvictable
	if lifetimeArgs.PodStatusPhases != nil {
		filter = func(pod *v1.Pod) bool {
			for _, phase := range lifetimeArgs.PodStatusPhases {
				if string(pod.Status.Phase) == phase {
					return evictable.IsEvictable(pod)
				}
			}
			return false
		}
	}

	var includedNamespaces, excludedNamespaces sets.String
	if lifetimeArgs.Namespaces != nil {
		includedNamespaces = sets.NewString(lifetimeArgs.Namespaces.Include...)
		excludedNamespaces = sets.NewString(lifetimeArgs.Namespaces.Exclude...)
	}

	podFilter, err := podutil.NewOptions().
		WithFilter(filter).
		WithNamespaces(includedNamespaces).
		WithoutNamespaces(excludedNamespaces).
		WithLabelSelector(lifetimeArgs.LabelSelector).
		BuildFilterFunc()
	if err != nil {
		return nil, fmt.Errorf("error initializing pod filter function: %v", err)
	}

	return &PodLifeTime{
		handle:    handle,
		args:      lifetimeArgs,
		podFilter: podFilter,
	}, nil
}

func (d *PodLifeTime) Name() string {
	return PluginName
}

func (d *PodLifeTime) Deschedule(ctx context.Context, nodes []*v1.Node) *framework.Status {
	for _, node := range nodes {
		klog.V(1).InfoS("Processing node", "node", klog.KObj(node))

		pods := listOldPodsOnNode(node.Name, d.handle.GetPodsAssignedToNodeFunc(), d.podFilter, *d.args.MaxPodLifeTimeSeconds)
		for _, pod := range pods {
			success, err := d.handle.PodEvictor().EvictPod(ctx, pod, node, "PodLifeTime")
			if success {
				klog.V(1).InfoS("Evicted pod because it exceeded its lifetime", "pod", klog.KObj(pod), "maxPodLifeTime", *d.args.MaxPodLifeTimeSeconds)
			}

			if err != nil {
				klog.ErrorS(err, "Error evicting pod", "pod", klog.KObj(pod))
				break
			}
		}

	}
	return nil
}

func listOldPodsOnNode(
	nodeName string,
	getPodsAssignedToNode podutil.GetPodsAssignedToNodeFunc,
	filter podutil.FilterFunc,
	maxPodLifeTimeSeconds uint,
) []*v1.Pod {
	pods, err := podutil.ListPodsOnANode(nodeName, getPodsAssignedToNode, filter)
	if err != nil {
		return nil
	}

	var oldPods []*v1.Pod
	for _, pod := range pods {
		podAgeSeconds := uint(metav1.Now().Sub(pod.GetCreationTimestamp().Local()).Seconds())
		if podAgeSeconds > maxPodLifeTimeSeconds {
			oldPods = append(oldPods, pod)
		}
	}

	return oldPods
}
