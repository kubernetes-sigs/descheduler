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

package strategies

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	v1meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
)

func validatePodLifeTimeParams(params *api.StrategyParameters) error {
	if params == nil || params.MaxPodLifeTimeSeconds == nil {
		return fmt.Errorf("MaxPodLifeTimeSeconds not set")
	}

	// At most one of include/exclude can be set
	if len(params.Namespaces.Include) > 0 && len(params.Namespaces.Exclude) > 0 {
		return fmt.Errorf("only one of Include/Exclude namespaces can be set")
	}

	return nil
}

// PodLifeTime evicts pods on nodes that were created more than strategy.Params.MaxPodLifeTimeSeconds seconds ago.
func PodLifeTime(ctx context.Context, client clientset.Interface, strategy api.DeschedulerStrategy, nodes []*v1.Node, podEvictor *evictions.PodEvictor) {
	if err := validatePodLifeTimeParams(strategy.Params); err != nil {
		klog.V(1).Info(err)
		return
	}

	for _, node := range nodes {
		klog.V(1).Infof("Processing node: %#v", node.Name)

		pods := listOldPodsOnNode(ctx, client, node, strategy.Params, podEvictor)
		for _, pod := range pods {
			success, err := podEvictor.EvictPod(ctx, pod, node, "PodLifeTime")
			if success {
				klog.V(1).Infof("Evicted pod: %#v because it was created more than %v seconds ago", pod.Name, *strategy.Params.MaxPodLifeTimeSeconds)
			}

			if err != nil {
				klog.Errorf("Error evicting pod: (%#v)", err)
				break
			}
		}

	}
}

func listOldPodsOnNode(ctx context.Context, client clientset.Interface, node *v1.Node, params *api.StrategyParameters, evictor *evictions.PodEvictor) []*v1.Pod {
	pods, err := podutil.ListPodsOnANode(
		ctx,
		client,
		node,
		podutil.WithFilter(evictor.IsEvictable),
		podutil.WithNamespaces(params.Namespaces.Include),
		podutil.WithoutNamespaces(params.Namespaces.Exclude),
	)
	if err != nil {
		return nil
	}

	var oldPods []*v1.Pod
	for _, pod := range pods {
		podAgeSeconds := uint(v1meta.Now().Sub(pod.GetCreationTimestamp().Local()).Seconds())
		if podAgeSeconds > *params.MaxPodLifeTimeSeconds {
			oldPods = append(oldPods, pod)
		}
	}

	return oldPods
}
