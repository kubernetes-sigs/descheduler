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
	"github.com/golang/glog"

	"k8s.io/api/core/v1"

	"github.com/kubernetes-incubator/descheduler/cmd/descheduler/app/options"
	"github.com/kubernetes-incubator/descheduler/pkg/api"
	"github.com/kubernetes-incubator/descheduler/pkg/descheduler/evictions"
	podutil "github.com/kubernetes-incubator/descheduler/pkg/descheduler/pod"
)

// RemovePodsHavingTooManyRestarts removes the pods that have too many restarts on node.
// There are too many cases leading this issue: Volume mount failed, app error due to nodes' different settings.
// As of now, this strategy won't evict daemonsets, mirror pods, critical pods and pods with local storages.
func RemovePodsHavingTooManyRestarts(ds *options.DeschedulerServer, strategy api.DeschedulerStrategy, policyGroupVersion string, nodes []*v1.Node, nodePodCount nodePodEvictedCount) {
	evictionCount := removePodsHavingTooManyRestarts(ds, strategy, policyGroupVersion, nodes, nodePodCount)
	glog.V(1).Infof("RemovePodsHavingTooManyRestarts evicted %v pods", evictionCount)
}

func removePodsHavingTooManyRestarts(ds *options.DeschedulerServer, strategy api.DeschedulerStrategy, policyGroupVersion string, nodes []*v1.Node, nodePodCount nodePodEvictedCount) int {
	podsEvicted := 0

	if !strategy.Enabled || strategy.Params.PodsHavingTooManyRestarts.PodeRestartThresholds < 1 {
		glog.Info("RemovePodsHavingTooManyRestarts policy is disabled.")
		return podsEvicted
	}

	for _, node := range nodes {
		glog.V(1).Infof("RemovePodsHavingTooManyRestarts Processing node: %#v", node.Name)
		pods, err := podutil.ListEvictablePodsOnNode(ds.Client, node)

		if err != nil {
			glog.Errorf("RemovePodsHavingTooManyRestarts Error when list pods at node %s", node.Name)
			continue
		}
		for _, pod := range pods {
			if ds.MaxNoOfPodsToEvictPerNode > 0 && nodePodCount[node]+1 > ds.MaxNoOfPodsToEvictPerNode {
				break
			}

			restarts, initRestarts := calcContainerRestarts(pod)
			if strategy.Params.PodsHavingTooManyRestarts.IncludingInitContainers {
				if restarts+initRestarts <= strategy.Params.PodsHavingTooManyRestarts.PodeRestartThresholds {
					continue
				}
			} else if restarts <= strategy.Params.PodsHavingTooManyRestarts.PodeRestartThresholds {
				continue
			}

			glog.V(1).Infof("RemovePodsHavingTooManyRestarts will evicted pod: %#v, container restarts: %d, initContainer restarts: %d", pod.Name, restarts, initRestarts)
			success, err := evictions.EvictPod(ds.Client, pod, policyGroupVersion, ds.DryRun)
			if !success {
				glog.Infof("RemovePodsHavingTooManyRestarts Error when evicting pod: %#v (%#v)", pod.Name, err)
			} else {
				nodePodCount[node]++
				glog.V(1).Infof("RemovePodsHavingTooManyRestarts Evicted pod: %#v (%#v)", pod.Name, err)
			}
		}
		podsEvicted += nodePodCount[node]
	}
	return podsEvicted
}

// calcContainerRestarts get container restarts and init container restarts.
func calcContainerRestarts(pod *v1.Pod) (int32, int32) {
	var (
		restarts     int32 = 0
		initRestarts int32 = 0
	)

	for _, cs := range pod.Status.ContainerStatuses {
		restarts += cs.RestartCount
	}

	for _, cs := range pod.Status.InitContainerStatuses {
		initRestarts += cs.RestartCount
	}

	return restarts, initRestarts
}
