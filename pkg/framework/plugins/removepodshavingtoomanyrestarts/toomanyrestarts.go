/*
Copyright 2018 The Kubernetes Authors.

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

package removepodshavingtoomanyrestarts

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"

	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/framework"
)

const PluginName = "RemovePodsHavingTooManyRestarts"

// RemovePodsHavingTooManyRestarts removes the pods that have too many restarts on node.
// There are too many cases leading this issue: Volume mount failed, app error due to nodes' different settings.
// As of now, this strategy won't evict daemonsets, mirror pods, critical pods and pods with local storages.
type RemovePodsHavingTooManyRestarts struct {
	handle            framework.Handle
	args              *framework.RemovePodsHavingTooManyRestartsArgs
	reasons           sets.String
	excludeOwnerKinds sets.String
	podFilter         podutil.FilterFunc
}

var _ framework.Plugin = &RemovePodsHavingTooManyRestarts{}
var _ framework.DeschedulePlugin = &RemovePodsHavingTooManyRestarts{}

func New(args runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	restartsArgs, ok := args.(*framework.RemovePodsHavingTooManyRestartsArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type RemovePodsHavingTooManyRestartsArgs, got %T", args)
	}

	if err := framework.ValidateCommonArgs(restartsArgs.CommonArgs); err != nil {
		return nil, err
	}

	if restartsArgs.PodRestartThreshold < 1 {
		return nil, fmt.Errorf("podsHavingTooManyRestarts threshold not set")
	}

	var includedNamespaces, excludedNamespaces sets.String
	if restartsArgs.Namespaces != nil {
		includedNamespaces = sets.NewString(restartsArgs.Namespaces.Include...)
		excludedNamespaces = sets.NewString(restartsArgs.Namespaces.Exclude...)
	}

	podFilter, err := podutil.NewOptions().
		WithFilter(handle.Evictor().Filter).
		WithNamespaces(includedNamespaces).
		WithoutNamespaces(excludedNamespaces).
		WithLabelSelector(restartsArgs.LabelSelector).
		BuildFilterFunc()
	if err != nil {
		return nil, fmt.Errorf("error initializing pod filter function: %v", err)
	}

	return &RemovePodsHavingTooManyRestarts{
		handle:    handle,
		args:      restartsArgs,
		podFilter: podFilter,
	}, nil
}

func (d *RemovePodsHavingTooManyRestarts) Name() string {
	return PluginName
}

func (d *RemovePodsHavingTooManyRestarts) Deschedule(ctx context.Context, nodes []*v1.Node) *framework.Status {
	for _, node := range nodes {
		klog.V(1).InfoS("Processing node", "node", klog.KObj(node))
		pods, err := podutil.ListPodsOnANode(node.Name, d.handle.GetPodsAssignedToNodeFunc(), d.podFilter)
		if err != nil {
			klog.ErrorS(err, "Error listing a nodes pods", "node", klog.KObj(node))
			continue
		}

		for i, pod := range pods {
			restarts, initRestarts := calcContainerRestarts(pod)
			if d.args.IncludingInitContainers {
				if restarts+initRestarts < d.args.PodRestartThreshold {
					continue
				}
			} else if restarts < d.args.PodRestartThreshold {
				continue
			}
			d.handle.Evictor().Evict(ctx, pods[i])
		}
	}
	return nil
}

// calcContainerRestarts get container restarts and init container restarts.
func calcContainerRestarts(pod *v1.Pod) (int32, int32) {
	var restarts, initRestarts int32

	for _, cs := range pod.Status.ContainerStatuses {
		restarts += cs.RestartCount
	}

	for _, cs := range pod.Status.InitContainerStatuses {
		initRestarts += cs.RestartCount
	}

	return restarts, initRestarts
}
