/*
Copyright 2024 The Kubernetes Authors.

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

package nodeutilization

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	utilptr "k8s.io/utils/ptr"
	"sigs.k8s.io/descheduler/pkg/descheduler/metricscollector"
	nodeutil "sigs.k8s.io/descheduler/pkg/descheduler/node"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/utils"
)

type usageClient interface {
	nodeUtilization(node string) map[v1.ResourceName]*resource.Quantity
	nodes() []*v1.Node
	pods(node string) []*v1.Pod
	capture(nodes []*v1.Node) error
	podUsage(pod *v1.Pod) (map[v1.ResourceName]*resource.Quantity, error)
}

type requestedUsageClient struct {
	resourceNames         []v1.ResourceName
	getPodsAssignedToNode podutil.GetPodsAssignedToNodeFunc

	_nodes           []*v1.Node
	_pods            map[string][]*v1.Pod
	_nodeUtilization map[string]map[v1.ResourceName]*resource.Quantity
}

var _ usageClient = &requestedUsageClient{}

func newRequestedUsageSnapshot(
	resourceNames []v1.ResourceName,
	getPodsAssignedToNode podutil.GetPodsAssignedToNodeFunc,
) *requestedUsageClient {
	return &requestedUsageClient{
		resourceNames:         resourceNames,
		getPodsAssignedToNode: getPodsAssignedToNode,
	}
}

func (s *requestedUsageClient) nodeUtilization(node string) map[v1.ResourceName]*resource.Quantity {
	return s._nodeUtilization[node]
}

func (s *requestedUsageClient) nodes() []*v1.Node {
	return s._nodes
}

func (s *requestedUsageClient) pods(node string) []*v1.Pod {
	return s._pods[node]
}

func (s *requestedUsageClient) podUsage(pod *v1.Pod) (map[v1.ResourceName]*resource.Quantity, error) {
	usage := make(map[v1.ResourceName]*resource.Quantity)
	for _, resourceName := range s.resourceNames {
		usage[resourceName] = utilptr.To[resource.Quantity](utils.GetResourceRequestQuantity(pod, resourceName).DeepCopy())
	}
	return usage, nil
}

func (s *requestedUsageClient) capture(nodes []*v1.Node) error {
	s._nodeUtilization = make(map[string]map[v1.ResourceName]*resource.Quantity)
	s._pods = make(map[string][]*v1.Pod)
	capturedNodes := []*v1.Node{}

	for _, node := range nodes {
		pods, err := podutil.ListPodsOnANode(node.Name, s.getPodsAssignedToNode, nil)
		if err != nil {
			klog.V(2).InfoS("Node will not be processed, error accessing its pods", "node", klog.KObj(node), "err", err)
			continue
		}

		nodeUsage, err := nodeutil.NodeUtilization(pods, s.resourceNames, func(pod *v1.Pod) (v1.ResourceList, error) {
			req, _ := utils.PodRequestsAndLimits(pod)
			return req, nil
		})
		if err != nil {
			return err
		}

		// store the snapshot of pods from the same (or the closest) node utilization computation
		s._pods[node.Name] = pods
		s._nodeUtilization[node.Name] = nodeUsage
		capturedNodes = append(capturedNodes, node)
	}

	s._nodes = capturedNodes

	return nil
}

type actualUsageClient struct {
	resourceNames         []v1.ResourceName
	getPodsAssignedToNode podutil.GetPodsAssignedToNodeFunc
	metricsCollector      *metricscollector.MetricsCollector

	_nodes           []*v1.Node
	_pods            map[string][]*v1.Pod
	_nodeUtilization map[string]map[v1.ResourceName]*resource.Quantity
}

var _ usageClient = &actualUsageClient{}

func newActualUsageSnapshot(
	resourceNames []v1.ResourceName,
	getPodsAssignedToNode podutil.GetPodsAssignedToNodeFunc,
	metricsCollector *metricscollector.MetricsCollector,
) *actualUsageClient {
	return &actualUsageClient{
		resourceNames:         resourceNames,
		getPodsAssignedToNode: getPodsAssignedToNode,
		metricsCollector:      metricsCollector,
	}
}

func (client *actualUsageClient) nodeUtilization(node string) map[v1.ResourceName]*resource.Quantity {
	return client._nodeUtilization[node]
}

func (client *actualUsageClient) nodes() []*v1.Node {
	return client._nodes
}

func (client *actualUsageClient) pods(node string) []*v1.Pod {
	return client._pods[node]
}

func (client *actualUsageClient) podUsage(pod *v1.Pod) (map[v1.ResourceName]*resource.Quantity, error) {
	// It's not efficient to keep track of all pods in a cluster when only their fractions is evicted.
	// Thus, take the current pod metrics without computing any softening (like e.g. EWMA).
	podMetrics, err := client.metricsCollector.MetricsClient().MetricsV1beta1().PodMetricses(pod.Namespace).Get(context.TODO(), pod.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("unable to get podmetrics for %q/%q: %v", pod.Namespace, pod.Name, err)
	}

	totalUsage := make(map[v1.ResourceName]*resource.Quantity)
	for _, container := range podMetrics.Containers {
		for _, resourceName := range client.resourceNames {
			if _, exists := container.Usage[resourceName]; !exists {
				continue
			}
			if totalUsage[resourceName] == nil {
				totalUsage[resourceName] = utilptr.To[resource.Quantity](container.Usage[resourceName].DeepCopy())
			} else {
				totalUsage[resourceName].Add(container.Usage[resourceName])
			}
		}
	}

	return totalUsage, nil
}

func (client *actualUsageClient) capture(nodes []*v1.Node) error {
	client._nodeUtilization = make(map[string]map[v1.ResourceName]*resource.Quantity)
	client._pods = make(map[string][]*v1.Pod)
	capturedNodes := []*v1.Node{}

	for _, node := range nodes {
		pods, err := podutil.ListPodsOnANode(node.Name, client.getPodsAssignedToNode, nil)
		if err != nil {
			klog.V(2).InfoS("Node will not be processed, error accessing its pods", "node", klog.KObj(node), "err", err)
			continue
		}

		nodeUsage, err := client.metricsCollector.NodeUsage(node)
		if err != nil {
			return err
		}
		nodeUsage[v1.ResourcePods] = resource.NewQuantity(int64(len(pods)), resource.DecimalSI)

		// store the snapshot of pods from the same (or the closest) node utilization computation
		client._pods[node.Name] = pods
		client._nodeUtilization[node.Name] = nodeUsage
		capturedNodes = append(capturedNodes, node)
	}

	client._nodes = capturedNodes

	return nil
}
