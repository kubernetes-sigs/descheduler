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

package strategies

import (
	"k8s.io/api/core/v1"
	"k8s.io/klog"

	"sigs.k8s.io/descheduler/cmd/descheduler/app/options"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
)

// RemovePodsHavingTooManyRestarts removes the pods that have too many restarts on node.
// There are too many cases leading this issue: Volume mount failed, app error due to nodes' different settings.
// As of now, this strategy won't evict daemonsets, mirror pods, critical pods and pods with local storages.
func RemovePodsHavingTooManyRestarts(ds *options.DeschedulerServer, strategy api.DeschedulerStrategy, nodes []*v1.Node, podEvictor *evictions.PodEvictor) {
	if !strategy.Enabled || strategy.Params.PodsHavingTooManyRestarts.PodRestartThreshold < 1 {
		return
	}
	removePodsHavingTooManyRestarts(ds, strategy, nodes, podEvictor, ds.EvictLocalStoragePods)
}

func removePodsHavingTooManyRestarts(ds *options.DeschedulerServer, strategy api.DeschedulerStrategy, nodes []*v1.Node, podEvictor *evictions.PodEvictor, evictLocalStoragePods bool) {
	for _, node := range nodes {
		klog.V(1).Infof("Processing node: %s", node.Name)
		pods, err := podutil.ListEvictablePodsOnNode(ds.Client, node, evictLocalStoragePods)
		if err != nil {
			klog.Errorf("Error when list pods at node %s", node.Name)
			continue
		}

		for i, pod := range pods {
			restarts, initRestarts := calcContainerRestarts(pod)
			if strategy.Params.PodsHavingTooManyRestarts.IncludingInitContainers {
				if restarts+initRestarts < strategy.Params.PodsHavingTooManyRestarts.PodRestartThreshold {
					continue
				}
			} else if restarts < strategy.Params.PodsHavingTooManyRestarts.PodRestartThreshold {
				continue
			}
			if _, err := podEvictor.EvictPod(pods[i], node); err != nil {
				break
			}
		}
	}
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
