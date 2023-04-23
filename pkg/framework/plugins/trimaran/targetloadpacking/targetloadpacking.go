/*
Copyright 2023 The Kubernetes Authors.

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

/*
targetloadpacking package provides K8s scheduler plugin for best-fit variant of bin packing based on CPU utilization around a target load
It contains plugin for Score extension point.
*/

package targetloadpacking

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"

	"github.com/paypal/load-watcher/pkg/watcher"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	nodeutil "sigs.k8s.io/descheduler/pkg/descheduler/node"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/trimaran"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
	"sigs.k8s.io/descheduler/pkg/utils"
)

const (
	Name = "TargetLoadPacking"
	// Time interval in seconds for each metrics agent ingestion.
	metricsAgentReportingIntervalSeconds = 60
)

type TargetLoadPacking struct {
	handle               frameworktypes.Handle
	eventHandler         trimaran.PodAssignEventHandler
	collector            trimaran.Collector
	args                 *trimaran.TargetLoadPackingArgs
	podEvictionFilter    func(pod *v1.Pod) bool
	podPreEvictionFilter func(pod *v1.Pod) bool

	requestsMilliCores int64
	targetUtilization  float64
	requestsMultiplier float64
}

var _ frameworktypes.BalancePlugin = &TargetLoadPacking{}

func New(obj runtime.Object, handle frameworktypes.Handle) (frameworktypes.Plugin, error) {
	// cast object into plugin arguments object
	args, ok := obj.(*trimaran.TargetLoadPackingArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type TargetLoadPackingArgs, got %T", obj)
	}
	requestsMultiplier, err := strconv.ParseFloat(*args.DefaultRequestsMultiplier, 64)
	if err != nil {
		return nil, fmt.Errorf("unable to parse DefaultRequestsMultiplier: %v", err)
	}

	podEvictionFilter, err := podutil.NewOptions().
		WithFilter(handle.Evictor().Filter).
		BuildFilterFunc()
	if err != nil {
		return nil, fmt.Errorf("error initializing pod eviction filter function: %v", err)
	}
	podPreEvictionFilter, err := podutil.NewOptions().
		WithFilter(handle.Evictor().PreEvictionFilter).
		BuildFilterFunc()

	pl := &TargetLoadPacking{
		handle:               handle,
		eventHandler:         trimaran.NewHandler(handle),
		collector:            trimaran.NewCollector(&args.TrimaranSpec),
		args:                 args,
		podEvictionFilter:    podEvictionFilter,
		podPreEvictionFilter: podPreEvictionFilter,

		requestsMilliCores: args.DefaultRequests.Cpu().MilliValue(),
		targetUtilization:  float64(*args.TargetUtilization),
		requestsMultiplier: requestsMultiplier,
	}
	return pl, nil
}

func (pl *TargetLoadPacking) Name() string {
	return Name
}

func (pl *TargetLoadPacking) Balance(ctx context.Context, nodes []*v1.Node) *frameworktypes.Status {
	// 1. Classify all nodes.
	sourceNodes, targetNodes := make([]*trimaran.NodeInfo, 0), make([]*trimaran.NodeInfo, 0)
	for _, node := range nodes {
		predictedCPUUsage, err := pl.getPredictedCPUUsage(node)
		if err != nil {
			klog.V(5).ErrorS(err, "Failed to get Predicted CPU Usage", "node", node.Name)
			continue
		}

		nodeInfo := &trimaran.NodeInfo{
			Node:     node,
			Usage:    predictedCPUUsage,
			Capacity: float64(node.Status.Capacity.Cpu().MilliValue()),
		}
		nodeInfo.Score = calculateScore(nodeInfo.Usage*100/nodeInfo.Capacity, pl.targetUtilization)

		if nodeInfo.Score < pl.targetUtilization {
			sourceNodes = append(sourceNodes, nodeInfo)
		} else {
			// Ignore any unschedulable nodes.
			if nodeutil.IsNodeUnschedulable(nodeInfo.Node) {
				klog.V(2).InfoS("Node is unschedulable, thus not considered as underutilized", "node", klog.KObj(nodeInfo.Node))
				continue
			}
			targetNodes = append(targetNodes, nodeInfo)
		}
	}

	if len(sourceNodes) == 0 {
		klog.V(1).InfoS("All nodes are underutilized, nothing to do here")
		return nil
	}
	if len(targetNodes) == 0 {
		klog.V(1).InfoS("No node is underutilized, nothing to do here, you might tune your thresholds further")
		return nil
	}

	// 2. Sorting nodes with low score (overutilized).
	sort.Slice(sourceNodes, func(i, j int) bool { return sourceNodes[i].Score < sourceNodes[j].Score })

	// 3. Construct evict functions.
	var nodeEvictFunc func(*trimaran.NodeInfo) bool
	var podEvictFunc func(*trimaran.NodeInfo, *v1.Pod) bool
	{
		// In TargetLoadPacking, we focus only on the CPU for now.
		availableCPUOfTargetNodes := &resource.Quantity{}
		taintsOfTargetNodes := map[string][]v1.Taint{}
		for _, nodeInfo := range targetNodes {
			taintsOfTargetNodes[nodeInfo.Node.Name] = nodeInfo.Node.Spec.Taints
			avaliableCPU := nodeInfo.Capacity*pl.targetUtilization/100 - nodeInfo.Usage
			availableCPUOfTargetNodes.Add(*resource.NewMilliQuantity(int64(avaliableCPU), resource.DecimalSI))
		}

		// nodeEvictFunc specifies whether the current node should attempt attempt to evict some pods.
		nodeEvictFunc = func(nodeInfo *trimaran.NodeInfo) bool {
			if availableCPUOfTargetNodes.CmpInt64(0) < 1 {
				return false
			}
			if calculateScore(nodeInfo.Usage*100/nodeInfo.Capacity, pl.targetUtilization) >= pl.targetUtilization {
				return false
			}
			if pl.handle.Evictor().NodeLimitExceeded(nodeInfo.Node) {
				return false
			}
			return true
		}
		// podEvictFunc specifies whether the current pod should be evicted, and if so, directly evicts it
		// and updates the resource usage of the node it is on.
		podEvictFunc = func(nodeInfo *trimaran.NodeInfo, pod *v1.Pod) bool {
			if !utils.PodToleratesTaints(pod, taintsOfTargetNodes) {
				return false
			}
			if !pl.podPreEvictionFilter(pod) {
				return false
			}
			if !pl.handle.Evictor().Evict(ctx, pod, evictions.EvictOptions{}) {
				return false
			}
			request := utils.GetResourceRequestQuantity(pod, v1.ResourceCPU)
			availableCPUOfTargetNodes.Sub(request)
			nodeInfo.Usage -= float64(request.MilliValue())
			return true
		}
	}

	pl.evictPodsFromNodes(
		ctx,
		sourceNodes,
		nodeEvictFunc,
		podEvictFunc,
	)

	return nil
}

func (pl *TargetLoadPacking) evictPodsFromNodes(
	ctx context.Context,
	sourceNodes []*trimaran.NodeInfo,
	nodeEvictFunc func(*trimaran.NodeInfo) bool,
	podEvictFunc func(nodeInfo *trimaran.NodeInfo, pod *v1.Pod) bool,
) {
	for _, sourceNode := range sourceNodes {
		// Perform a check on the node at the beginning.
		if !nodeEvictFunc(sourceNode) {
			continue
		}

		// We only care about the pods that can pass the podFilter.
		pods, err := podutil.ListPodsOnANode(sourceNode.Node.Name, pl.handle.GetPodsAssignedToNodeFunc(), pl.podEvictionFilter)
		if err != nil {
			klog.V(2).InfoS("Node will not be processed, error accessing its pods", "node", sourceNode.Node.Name, "err", err)
			continue
		}
		// Sort the evictable Pods based on priority. This also sorts them based on QoS.
		// If there are multiple pods with same priority, they are sorted based on QoS tiers.
		podutil.SortPodsBasedOnPriorityLowToHigh(pods)

		for _, pod := range pods {
			if podEvictFunc(sourceNode, pod) {
				klog.V(3).InfoS("Evicted pods", "pod", klog.KObj(pod))

				// Perform a check on the node every time a Pod is evicted.
				if !nodeEvictFunc(sourceNode) {
					break
				}
			}
		}
	}
}

func (pl *TargetLoadPacking) getPredictedCPUUsage(node *v1.Node) (float64, error) {
	if node.Status.Capacity.Cpu().MilliValue() == 0 {
		return 0, fmt.Errorf("Node CPU capacity is zero")
	}
	metrics, window := pl.collector.GetNodeMetrics(node.Name)
	if metrics == nil {
		return 0, fmt.Errorf("Failed to get metrics for node")
		// TODO(aqadeer): If this happens for a long time, fall back to allocation based packing.
		// This could mean maintaining failure state across cycles if scheduler doesn't provide this state.
	}

	nodeCPUUtilPercent, cpuMetricFound := getNodeCPUUtilPercent(metrics)
	if !cpuMetricFound {
		return 0, fmt.Errorf("Failed to get CPU metric in node metrics")
	}

	var missingCPUUtilMillis int64 = 0
	pl.eventHandler.RangePodInfos(node.Name, func(pi *trimaran.PodInfo) {
		// If the time stamp of the scheduled pod is outside fetched metrics window, or it is within metrics reporting interval seconds, we predict util.
		// Note that the second condition doesn't guarantee metrics for that pod are not reported yet as the 0 <= t <= 2*metricsAgentReportingIntervalSeconds
		// t = metricsAgentReportingIntervalSeconds is taken as average case and it doesn't hurt us much if we are
		// counting metrics twice in case actual t is less than metricsAgentReportingIntervalSeconds
		if pi.Timestamp.Unix() > window.End ||
			pi.Timestamp.Unix() <= window.End && (window.End-pi.Timestamp.Unix()) < metricsAgentReportingIntervalSeconds {
			for _, container := range pi.Pod.Spec.Containers {
				if _, ok := container.Resources.Limits[v1.ResourceCPU]; ok {
					missingCPUUtilMillis += container.Resources.Limits.Cpu().MilliValue()
				} else if _, ok := container.Resources.Requests[v1.ResourceCPU]; ok {
					missingCPUUtilMillis += int64(math.Round(float64(container.Resources.Requests.Cpu().MilliValue()) * pl.requestsMultiplier))
				} else {
					missingCPUUtilMillis += pl.requestsMilliCores
				}
			}
			missingCPUUtilMillis += pi.Pod.Spec.Overhead.Cpu().MilliValue()
		}
	})

	return (nodeCPUUtilPercent*float64(node.Status.Capacity.Cpu().MilliValue()) + float64(missingCPUUtilMillis)) / 100, nil
}

func calculateScore(U, X float64) float64 {
	// To avoid the interference of small deviations, math.Round is not used。
	switch {
	case U >= 0 && U <= X:
		return (100-X)*U/X + X
	case U > X && U <= 100:
		return X * (100 - U) / (100 - X)
	default:
		return float64(trimaran.MinNodeScore)
	}
}

func getNodeCPUUtilPercent(metrics []watcher.Metric) (value float64, found bool) {
	for _, metric := range metrics {
		if metric.Type == watcher.CPU {
			if metric.Operator == watcher.Average || metric.Operator == watcher.Latest {
				value = metric.Value
				found = true
			}
		}
	}
	return
}
