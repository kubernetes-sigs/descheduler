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
	"time"

	promapi "github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	utilptr "k8s.io/utils/ptr"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/metricscollector"
	nodeutil "sigs.k8s.io/descheduler/pkg/descheduler/node"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/utils"
)

type UsageClientType int

const (
	requestedUsageClientType UsageClientType = iota
	actualUsageClientType
	prometheusUsageClientType
)

type notSupportedError struct {
	usageClientType UsageClientType
}

func (e notSupportedError) Error() string {
	return "maximum number of evicted pods per node reached"
}

func newNotSupportedError(usageClientType UsageClientType) *notSupportedError {
	return &notSupportedError{
		usageClientType: usageClientType,
	}
}

type usageClient interface {
	// Both low/high node utilization plugins are expected to invoke sync right
	// after Balance method is invoked. There's no cache invalidation so each
	// Balance is expected to get the latest data by invoking sync.
	sync(ctx context.Context, nodes []*v1.Node) error
	nodeUtilization(node string) api.ReferencedResourceList
	pods(node string) []*v1.Pod
	podUsage(pod *v1.Pod) (api.ReferencedResourceList, error)
}

type requestedUsageClient struct {
	resourceNames         []v1.ResourceName
	getPodsAssignedToNode podutil.GetPodsAssignedToNodeFunc

	_pods            map[string][]*v1.Pod
	_nodeUtilization map[string]api.ReferencedResourceList
}

var _ usageClient = &requestedUsageClient{}

func newRequestedUsageClient(
	resourceNames []v1.ResourceName,
	getPodsAssignedToNode podutil.GetPodsAssignedToNodeFunc,
) *requestedUsageClient {
	return &requestedUsageClient{
		resourceNames:         resourceNames,
		getPodsAssignedToNode: getPodsAssignedToNode,
	}
}

func (s *requestedUsageClient) nodeUtilization(node string) api.ReferencedResourceList {
	return s._nodeUtilization[node]
}

func (s *requestedUsageClient) pods(node string) []*v1.Pod {
	return s._pods[node]
}

func (s *requestedUsageClient) podUsage(pod *v1.Pod) (api.ReferencedResourceList, error) {
	usage := make(api.ReferencedResourceList)
	for _, resourceName := range s.resourceNames {
		usage[resourceName] = utilptr.To[resource.Quantity](utils.GetResourceRequestQuantity(pod, resourceName).DeepCopy())
	}
	return usage, nil
}

func (s *requestedUsageClient) sync(ctx context.Context, nodes []*v1.Node) error {
	s._nodeUtilization = make(map[string]api.ReferencedResourceList)
	s._pods = make(map[string][]*v1.Pod)

	for _, node := range nodes {
		pods, err := podutil.ListPodsOnANode(node.Name, s.getPodsAssignedToNode, nil)
		if err != nil {
			klog.V(2).InfoS("Node will not be processed, error accessing its pods", "node", klog.KObj(node), "err", err)
			return fmt.Errorf("error accessing %q node's pods: %v", node.Name, err)
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
	}

	return nil
}

type actualUsageClient struct {
	resourceNames         []v1.ResourceName
	getPodsAssignedToNode podutil.GetPodsAssignedToNodeFunc
	metricsCollector      *metricscollector.MetricsCollector

	_pods            map[string][]*v1.Pod
	_nodeUtilization map[string]api.ReferencedResourceList
}

var _ usageClient = &actualUsageClient{}

func newActualUsageClient(
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

func (client *actualUsageClient) nodeUtilization(node string) api.ReferencedResourceList {
	return client._nodeUtilization[node]
}

func (client *actualUsageClient) pods(node string) []*v1.Pod {
	return client._pods[node]
}

func (client *actualUsageClient) podUsage(pod *v1.Pod) (api.ReferencedResourceList, error) {
	// It's not efficient to keep track of all pods in a cluster when only their fractions is evicted.
	// Thus, take the current pod metrics without computing any softening (like e.g. EWMA).
	podMetrics, err := client.metricsCollector.MetricsClient().MetricsV1beta1().PodMetricses(pod.Namespace).Get(context.TODO(), pod.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("unable to get podmetrics for %q/%q: %v", pod.Namespace, pod.Name, err)
	}

	totalUsage := make(api.ReferencedResourceList)
	for _, container := range podMetrics.Containers {
		for _, resourceName := range client.resourceNames {
			if resourceName == v1.ResourcePods {
				continue
			}
			if _, exists := container.Usage[resourceName]; !exists {
				return nil, fmt.Errorf("pod %v/%v: container %q is missing %q resource", pod.Namespace, pod.Name, container.Name, resourceName)
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

func (client *actualUsageClient) sync(ctx context.Context, nodes []*v1.Node) error {
	client._nodeUtilization = make(map[string]api.ReferencedResourceList)
	client._pods = make(map[string][]*v1.Pod)

	nodesUsage, err := client.metricsCollector.AllNodesUsage()
	if err != nil {
		return err
	}

	for _, node := range nodes {
		pods, err := podutil.ListPodsOnANode(node.Name, client.getPodsAssignedToNode, nil)
		if err != nil {
			klog.V(2).InfoS("Node will not be processed, error accessing its pods", "node", klog.KObj(node), "err", err)
			return fmt.Errorf("error accessing %q node's pods: %v", node.Name, err)
		}

		collectedNodeUsage, ok := nodesUsage[node.Name]
		if !ok {
			return fmt.Errorf("unable to find node %q in the collected metrics", node.Name)
		}
		collectedNodeUsage[v1.ResourcePods] = resource.NewQuantity(int64(len(pods)), resource.DecimalSI)

		nodeUsage := api.ReferencedResourceList{}
		for _, resourceName := range client.resourceNames {
			if _, exists := collectedNodeUsage[resourceName]; !exists {
				return fmt.Errorf("unable to find %q resource for collected %q node metric", resourceName, node.Name)
			}
			nodeUsage[resourceName] = collectedNodeUsage[resourceName]
		}
		// store the snapshot of pods from the same (or the closest) node utilization computation
		client._pods[node.Name] = pods
		client._nodeUtilization[node.Name] = nodeUsage
	}

	return nil
}

type prometheusUsageClient struct {
	getPodsAssignedToNode podutil.GetPodsAssignedToNodeFunc
	promClient            promapi.Client
	promQuery             string

	_pods            map[string][]*v1.Pod
	_nodeUtilization map[string]map[v1.ResourceName]*resource.Quantity
}

var _ usageClient = &actualUsageClient{}

func newPrometheusUsageClient(
	getPodsAssignedToNode podutil.GetPodsAssignedToNodeFunc,
	promClient promapi.Client,
	promQuery string,
) *prometheusUsageClient {
	return &prometheusUsageClient{
		getPodsAssignedToNode: getPodsAssignedToNode,
		promClient:            promClient,
		promQuery:             promQuery,
	}
}

func (client *prometheusUsageClient) nodeUtilization(node string) map[v1.ResourceName]*resource.Quantity {
	return client._nodeUtilization[node]
}

func (client *prometheusUsageClient) pods(node string) []*v1.Pod {
	return client._pods[node]
}

func (client *prometheusUsageClient) podUsage(pod *v1.Pod) (map[v1.ResourceName]*resource.Quantity, error) {
	return nil, newNotSupportedError(prometheusUsageClientType)
}

func NodeUsageFromPrometheusMetrics(ctx context.Context, promClient promapi.Client, promQuery string) (map[string]map[v1.ResourceName]*resource.Quantity, error) {
	results, warnings, err := promv1.NewAPI(promClient).Query(ctx, promQuery, time.Now())
	if err != nil {
		return nil, fmt.Errorf("unable to capture prometheus metrics: %v", err)
	}
	if len(warnings) > 0 {
		klog.Infof("prometheus metrics warnings: %v", warnings)
	}

	if results.Type() != model.ValVector {
		return nil, fmt.Errorf("expected query results to be of type %q, got %q instead", model.ValVector, results.Type())
	}

	nodeUsages := make(map[string]map[v1.ResourceName]*resource.Quantity)
	for _, sample := range results.(model.Vector) {
		nodeName, exists := sample.Metric["instance"]
		if !exists {
			return nil, fmt.Errorf("The collected metrics sample is missing 'instance' key")
		}
		if sample.Value < 0 || sample.Value > 1 {
			return nil, fmt.Errorf("The collected metrics sample for %q has value %v outside of <0; 1> interval", string(nodeName), sample.Value)
		}
		nodeUsages[string(nodeName)] = map[v1.ResourceName]*resource.Quantity{
			MetricResource: resource.NewQuantity(int64(sample.Value*100), resource.DecimalSI),
		}
	}

	return nodeUsages, nil
}

func (client *prometheusUsageClient) sync(ctx context.Context, nodes []*v1.Node) error {
	client._nodeUtilization = make(map[string]map[v1.ResourceName]*resource.Quantity)
	client._pods = make(map[string][]*v1.Pod)

	nodeUsages, err := NodeUsageFromPrometheusMetrics(ctx, client.promClient, client.promQuery)
	if err != nil {
		return err
	}

	for _, node := range nodes {
		if _, exists := nodeUsages[node.Name]; !exists {
			return fmt.Errorf("unable to find metric entry for %v", node.Name)
		}
		pods, err := podutil.ListPodsOnANode(node.Name, client.getPodsAssignedToNode, nil)
		if err != nil {
			klog.V(2).InfoS("Node will not be processed, error accessing its pods", "node", klog.KObj(node), "err", err)
			return fmt.Errorf("error accessing %q node's pods: %v", node.Name, err)
		}

		// store the snapshot of pods from the same (or the closest) node utilization computation
		client._pods[node.Name] = pods
		client._nodeUtilization[node.Name] = nodeUsages[node.Name]
	}

	return nil
}
