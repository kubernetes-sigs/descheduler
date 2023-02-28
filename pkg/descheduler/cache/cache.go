package cache

import (
	"context"
	"fmt"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
	"k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"
	"sigs.k8s.io/descheduler/pkg/utils"
	"sync"
	"time"
)

const LATEST_REAL_METRICS_STORE int = 6
const LATEST_HIGH_METRICS_STORE int = 3
const MAX_REAL_METRICS_STORE int = 15

type Cache struct {
	sync.RWMutex

	client     clientset.Interface
	stopChan   <-chan struct{}
	nodeLister corelisters.NodeLister
	mc         metricsclientset.Interface
	nodes      map[string]*nodeStats
}

type nodeStats struct {
	node       *v1.Node
	realUsed   []*v1beta1.NodeMetrics
	updateTime time.Time
	pods       map[string]*podStats
}

func (ns *nodeStats) getPodUsageByNode() []*PodUsageMap {
	podUsage := make([]*PodUsageMap, 0)
	for _, podStat := range ns.pods {
		usage := &PodUsageMap{
			Pod: podStat.pod,
		}
		var usageList []v1.ResourceList
		usageList = podStat.realUsed
		usage.UsageList = usageList
		podUsage = append(podUsage, usage)
	}
	return podUsage
}

func RealPodUsageWithIndex(podUsage *PodUsageMap, index int) (cUsage, mUsage int64) {
	if len(podUsage.UsageList) < 1 {
		return
	}
	if len(podUsage.UsageList) <= index {
		return
	}
	currentUsage := podUsage.UsageList[index]
	if currentUsage == nil {
		klog.Errorf("currentUsage is nil,podUsage:%+v", podUsage)
		return
	}
	addQuantity := resource.Quantity{Format: resource.DecimalSI}
	addQuantity.Add(*currentUsage.Cpu())
	cUsage = addQuantity.MilliValue()
	addQuantity = resource.Quantity{Format: resource.BinarySI}
	addQuantity.Add(*currentUsage.Memory())
	mUsage = addQuantity.Value()
	return
}

type podStats struct {
	pod        *v1.Pod
	used       v1.ResourceList
	limit      v1.ResourceList
	realUsed   []v1.ResourceList
	updateTime time.Time
}

func newPodStats(pod *v1.Pod) *podStats {
	req, limit := utils.PodRequestsAndLimits(pod)
	return &podStats{
		pod:      pod,
		used:     req,
		limit:    limit,
		realUsed: make([]v1.ResourceList, 0, MAX_REAL_METRICS_STORE),
	}
}

func InitCache(client clientset.Interface, nodeInformer cache.SharedIndexInformer, nodeLister corelisters.NodeLister, podInformer cache.SharedIndexInformer, mc metricsclientset.Interface, stopChan <-chan struct{}) (c BasicCache, err error) {
	iCache := &Cache{
		client:     client,
		nodeLister: nodeLister,
		stopChan:   stopChan,
		mc:         mc,
		nodes:      make(map[string]*nodeStats, 500),
	}
	_, err = nodeInformer.AddEventHandler(iCache.GetResourceNodeEventHandler())
	if err != nil {
		return iCache, err
	}
	_, err = podInformer.AddEventHandler(iCache.GetResourcePodEventHandler())
	if err != nil {
		return iCache, err
	}
	return iCache, nil
}

func (c *Cache) Run(period time.Duration) {
	go wait.Until(c.ClearStat, period, c.stopChan)
	go wait.Until(c.syncMetricsWorker, period, c.stopChan)
}
func (c *Cache) ClearStat() {
	c.Lock()
	defer c.Unlock()
	now := time.Now()
	canClear := func(updateTime time.Time) bool {
		if now.Sub(updateTime).Minutes() > 2 {
			return true
		}
		return false
	}
	for nodeName, nodeStats := range c.nodes {
		if canClear(nodeStats.updateTime) && len(nodeStats.realUsed) > 0 {
			klog.V(4).Infof("node %v last updateTime %v,will clear realUsed", nodeName, nodeStats.updateTime)
			nodeStats.realUsed = make([]*v1beta1.NodeMetrics, 0, MAX_REAL_METRICS_STORE)
		}
		for podName, podStats := range nodeStats.pods {
			if canClear(podStats.updateTime) && len(podStats.realUsed) > 0 {
				klog.V(4).Infof("pod %v last updateTime %v,will clear realUsed", podName, podStats.updateTime)
				podStats.realUsed = make([]v1.ResourceList, 0, MAX_REAL_METRICS_STORE)
			}
		}
	}
}
func (c *Cache) syncMetricsWorker() {
	klog.V(4).Infof("sync metrics start")
	defer func() {
		klog.V(4).Infof("sync metrics end")
	}()
	c.Lock()
	defer c.Unlock()

	ctx := context.Background()
	metricsList, err := c.mc.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
	if err != nil {
		klog.Errorf("Failed to get node metrics %v ", err)
		return
	}
	now := time.Now()
	for i := range metricsList.Items {
		item := metricsList.Items[i]
		if _, ok := c.nodes[item.Name]; ok {
			c.nodes[item.Name].updateTime = item.Timestamp.Time
			c.nodes[item.Name].realUsed = append(c.nodes[item.Name].realUsed, &item)
			//保留15分钟内的node利用率
			cutTime := time.Now().Add(-time.Duration(MAX_REAL_METRICS_STORE) * time.Minute)
			cutIndex := 0
			for j, metricsItem := range c.nodes[item.Name].realUsed {
				cutIndex = j
				if metricsItem.Timestamp.After(cutTime) {
					break
				}
			}
			c.nodes[item.Name].realUsed = c.nodes[item.Name].realUsed[cutIndex:]
		}
	}

	podMetricsList, err := c.mc.MetricsV1beta1().PodMetricses("").List(ctx, metav1.ListOptions{})

	if err != nil {
		klog.Errorf("Failed to get pod metric %v ", err)
		return
	}

	for i := range podMetricsList.Items {
		podMetricsItem := podMetricsList.Items[i]
		for name := range c.nodes {
			nodeStatus := c.nodes[name]
			if _, ok := nodeStatus.pods[podMetricsItem.Name]; ok {
				u := nodeStatus.pods[podMetricsItem.Name].realUsed
				podUsed := podResourceRealUsed(podMetricsItem)
				u = append(u, podUsed)
				l := len(u) - MAX_REAL_METRICS_STORE
				if l > 0 {
					u = u[l:]
				}
				nodeStatus.pods[podMetricsItem.Name].updateTime = now
				nodeStatus.pods[podMetricsItem.Name].realUsed = u
			}
		}
	}
}

func podResourceRealUsed(podMetrics v1beta1.PodMetrics) (used v1.ResourceList) {
	used = v1.ResourceList{}
	for _, container := range podMetrics.Containers {
		for name, value := range container.Usage {
			if q, ok := used[name]; !ok {
				used[name] = value.DeepCopy()
			} else {
				q.Add(value)
				used[name] = q
			}
		}
	}
	return
}

func (c *Cache) GetResourceNodeEventHandler() cache.ResourceEventHandler {
	return cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addNode,
		UpdateFunc: c.updateNode,
		DeleteFunc: c.deleteNode,
	}
}
func (c *Cache) addNode(obj interface{}) {
	c.Lock()
	defer c.Unlock()

	node, ok := obj.(*v1.Node)
	if !ok {
		klog.Errorf("Failed to convert %v to v1.Pod", obj)
		return
	}

	if _, ok := c.nodes[node.Name]; !ok {
		c.nodes[node.Name] = &nodeStats{
			node:     node,
			realUsed: make([]*v1beta1.NodeMetrics, 0, MAX_REAL_METRICS_STORE),
			pods:     make(map[string]*podStats),
		}
	}

}

func (c *Cache) updateNode(oldObj, newObj interface{}) {
	c.Lock()
	defer c.Unlock()

	// cpu, mem, allocated change -> update
	newNode, ok := newObj.(*v1.Node)
	if !ok {
		klog.Errorf("Failed to convert %v to v1.Pod", newObj)
		return
	}

	oldNode, ok := oldObj.(*v1.Node)
	if !ok {
		klog.Errorf("Failed to convert %v to v1.Pod", oldObj)
		return
	}
	if _, exists := c.nodes[oldNode.Name]; exists {
		c.nodes[oldNode.Name].node = newNode
	}
}

func (c *Cache) deleteNode(obj interface{}) {
	c.Lock()
	defer c.Unlock()
	fmt.Println("deleteNode")

	node, ok := obj.(*v1.Node)
	if !ok {
		klog.Errorf("Failed to convert %v to v1.Pod", obj)
		return
	}

	if _, ok := c.nodes[node.Name]; ok {
		delete(c.nodes, node.Name)
	}

}
func (c *Cache) GetResourcePodEventHandler() cache.ResourceEventHandler {
	return cache.FilteringResourceEventHandler{
		FilterFunc: func(obj interface{}) bool {
			po, ok := obj.(*v1.Pod)
			if !ok {
				return false
			}
			active := utils.IsPodActive(po)
			if !active {
				klog.V(7).Infof("pod %v is not active", po.Name)
			}
			return active
		},
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    c.addPod,
			UpdateFunc: c.updatePod,
			DeleteFunc: c.deletePod,
		},
	}
}

func (c *Cache) addPod(obj interface{}) {
	c.Lock()
	defer c.Unlock()

	pod, ok := obj.(*v1.Pod)
	if !ok {
		klog.Errorf("Failed to convert %v to v1.Pod", obj)
		return
	}
	klog.V(7).Infof("addPod:%v", pod.Name)

	// when pod create,  it is unbind , so ignore it
	// maybe we can store pending status pod
	if pod.Spec.NodeName == "" {
		return
	}

	// init pod ,, also do it in update pod
	if nodeStat, ok := c.nodes[pod.Spec.NodeName]; ok {
		if nodeStat.pods == nil {
			nodeStat.pods = make(map[string]*podStats)
		}
		nodeStat.pods[pod.Name] = newPodStats(pod)
	}
}

func (c *Cache) updatePod(oldObj, newObj interface{}) {
	c.Lock()
	defer c.Unlock()

	oldPod, ok := oldObj.(*v1.Pod)
	if !ok {
		klog.Errorf("Failed to convert %v to v1.Pod", oldObj)
		return
	}

	newPod, ok := newObj.(*v1.Pod)
	if !ok {
		klog.Errorf("Failed to convert %v to v1.Pod", newObj)
		return
	}

	// init new pod to node struct
	if newPod.Spec.NodeName != "" {
		if _, exists := c.nodes[newPod.Spec.NodeName]; !exists {
			return
		}
		nStats := c.nodes[newPod.Spec.NodeName]
		if _, exists := nStats.pods[newPod.Name]; exists {
			return
		}
		nStats.pods[newPod.Name] = newPodStats(newPod)
	}

	oldRestarts, oldInitRestarts := utils.CalcContainerRestarts(oldPod)
	newRestarts, newInitRestarts := utils.CalcContainerRestarts(newPod)
	if oldRestarts != newRestarts || oldInitRestarts != newInitRestarts {
		c.nodes[oldPod.Spec.NodeName].pods[oldPod.Name].pod = newPod
	}

	if oldPod.Status.Phase != newPod.Status.Phase {
		if _, ok := c.nodes[oldPod.Spec.NodeName].pods[oldPod.Name]; ok {
			c.nodes[oldPod.Spec.NodeName].pods[oldPod.Name].pod = newPod
		}
	}

}

func (c *Cache) deletePod(obj interface{}) {
	c.Lock()
	defer c.Unlock()

	pod, ok := obj.(*v1.Pod)
	if !ok {
		klog.Errorf("Failed to convert %v to v1.Pod", obj)
		return
	}

	if _, exists := c.nodes[pod.Spec.NodeName]; !exists {
		return
	}
	nodeStat := c.nodes[pod.Spec.NodeName]
	if _, ok := nodeStat.pods[pod.Name]; ok {
		delete(c.nodes[pod.Spec.NodeName].pods, pod.Name)
	}

}

func (c *Cache) GetReadyNodeUsage(option *QueryCacheOption) map[string]*NodeUsageMap {
	nodeSelector := option.NodeSelector
	c.RLock()
	defer c.RUnlock()
	selector, err := labels.Parse(nodeSelector)
	if err != nil {
		return nil
	}
	result := map[string]*NodeUsageMap{}
	for nodeName, nodeStat := range c.nodes {
		node := nodeStat.node
		if selector.Matches(labels.Set(node.GetLabels())) {
			for _, condition := range node.Status.Conditions {
				if condition.Type == v1.NodeReady && condition.Status == v1.ConditionTrue {
					ns := c.nodes[nodeName]
					nodeUsage := ns.getNodeUsageMap()
					result[nodeName] = nodeUsage
					break
				}
			}
		}
	}
	return result
}

func (ns *nodeStats) getNodeUsageMap() *NodeUsageMap {
	node := ns.node
	podUsageList := ns.getPodUsageByNode()
	resourceListArray := ns.realUsed
	allPods := make([]*v1.Pod, 0, len(ns.pods))
	for _, podStat := range ns.pods {
		allPods = append(allPods, podStat.pod)
	}
	usage := getResourceThresholds(resourceListArray)
	if len(usage) == 0 {
		klog.V(5).Infof("usage is empty,node:%s", node.Name)
	}
	var currentUsage v1.ResourceList
	//use latest usage
	if len(usage) > 0 {
		currentUsage = usage[len(usage)-1]
	}
	return &NodeUsageMap{
		Node:         node,
		UsageList:    usage,
		AllPods:      podUsageList,
		CurrentUsage: currentUsage,
	}
}

func getResourceThresholds(resourceListArray []*v1beta1.NodeMetrics) []v1.ResourceList {
	usageList := make([]v1.ResourceList, 0)
	for _, metrics := range resourceListArray {
		resourceList := metrics.Usage
		usageList = append(usageList, resourceList)
	}
	return usageList
}
