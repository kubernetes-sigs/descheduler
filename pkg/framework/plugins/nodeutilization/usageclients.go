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
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog/v2"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
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
	metricsClientset      metricsclient.Interface

	_nodes           []*v1.Node
	_pods            map[string][]*v1.Pod
	_nodeUtilization map[string]map[v1.ResourceName]*resource.Quantity
}

var _ usageClient = &actualUsageClient{}

func newActualUsageSnapshot(
	resourceNames []v1.ResourceName,
	getPodsAssignedToNode podutil.GetPodsAssignedToNodeFunc,
	metricsCollector *metricscollector.MetricsCollector,
	metricsClientset metricsclient.Interface,
) *actualUsageClient {
	return &actualUsageClient{
		resourceNames:         resourceNames,
		getPodsAssignedToNode: getPodsAssignedToNode,
		metricsCollector:      metricsCollector,
		metricsClientset:      metricsClientset,
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

		// store the snapshot of pods from the same (or the closest) node utilization computation
		client._pods[node.Name] = pods
		client._nodeUtilization[node.Name] = nodeUsage
    capturedNodes = append(capturedNodes, node)
	}

  client._nodes = capturedNodes

	return nil
}
