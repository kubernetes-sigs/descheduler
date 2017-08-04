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

package strategies

import (
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/kubernetes/pkg/api/v1"
	"k8s.io/kubernetes/pkg/api/v1/resource"
	"k8s.io/kubernetes/pkg/client/clientset_generated/clientset"

	"github.com/aveshagarwal/rescheduler/pkg/api"
	podutil "github.com/aveshagarwal/rescheduler/pkg/rescheduler/pod"
)

func LowNodeUtilization(client clientset.Interface, policyGroupVersion string, nodes []*v1.Node) {
	for _, node := range nodes {
		fmt.Printf("Node %#v usage: %#v\n", node.Name, NodeUtilization(client, node))
	}
}

func NodeUtilization(client clientset.Interface, node *v1.Node) api.ResourceThresholds {
	pods, err := podutil.ListPodsOnANode(client, node)
	if err != nil {
		return nil
	}

	totalReqs := map[v1.ResourceName]resource.Quantity{}
	for pod := range pods {
		if podutil.IsBestEffortPod(pod) {
			continue
		}
		req, _, err := resource.PodRequestsAndLimits(pod)
		if err != nil {
			fmt.Printf("Error computing resource usage of pod, ignoring: %#v\n", pod.Name)
			continue
		}
		for name, quantity := range req {
			if name == v1.ResourceCPU || name == v1.ResourceMemory {
				if value, ok := totalReqs[name]; !ok {
					totalReqs[name] = *quantity.Copy()
				} else {
					value.Add(quantity)
					totalReqs[name] = value
				}
			}
		}
	}

	allocatable := node.Status.Capacity
	if len(node.Status.Allocatable) > 0 {
		allocatable = node.Status.Allocatable
	}

	rt := api.ResourceThresholds{}
	totalCPUReq := totalReq[v1.ResourceCPU]
	totalMemReq := totalReq[v1.ResourceMemory]
	totalPods := len(pods)
	rt[v1.ResourceCPU] = (float64(totalCPUReq.MilliValue()) * 100) / float64(allocatable.Cpu().MilliValue())
	rt[v1.ResourceMmeory] = float64(totalMemReq.Value()) / float64(allocatable.Memory().Value()) * 100
	rt[v1.ResourcePods] = (float64(totalPods) * 100) / float64(allocatable.Pods().Value())
	return rt
}
