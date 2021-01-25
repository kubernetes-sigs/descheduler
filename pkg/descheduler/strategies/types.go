package strategies

import (
	"context"
	v1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	"time"
)

var StrategyFuncs = map[string]struct {
	StrategyFunc           StrategyFunction
	StrategyControllerFunc StrategyControllerFunction
}{
	RemoveDuplicatesName:     {StrategyFunc: RemoveDuplicatePods},
	LowNodeUtilizationName:   {StrategyFunc: LowNodeUtilization},
	InterPodAntiAffinityName: {StrategyFunc: RemovePodsViolatingInterPodAntiAffinity},
	NodeAffinityName: {
		StrategyFunc:           RemovePodsViolatingNodeAffinity,
		StrategyControllerFunc: NewRemovePodsViolatingNodeAffinity},
	NodeTaintsName: {
		StrategyFunc:           RemovePodsViolatingNodeTaints,
		StrategyControllerFunc: NewRemovePodsViolatingNodeTaints},
	TooManyRestartsName: {StrategyFunc: RemovePodsHavingTooManyRestarts},
	PodLifeTimeName:     {StrategyFunc: PodLifeTime},
	TopologySpreadName:  {StrategyFunc: RemovePodsViolatingTopologySpreadConstraint},
}

// StrategyFunction defines the function signature for each strategy's main function
type StrategyFunction func(
	ctx context.Context,
	client clientset.Interface,
	strategy api.DeschedulerStrategy,
	nodes []*v1.Node,
	podEvictor *evictions.PodEvictor,
)

// StrategyController is a controller responsible for running an individual strategy, used with informed strategies
type StrategyController struct {
	ctx                   context.Context
	client                clientset.Interface
	queue                 workqueue.RateLimitingInterface
	sharedInformerFactory informers.SharedInformerFactory
	stopChannel           chan struct{}

	f            StrategyFunction
	name         string
	strategy     api.DeschedulerStrategy
	nodes        []*v1.Node
	podEvictor   *evictions.PodEvictor
	nodeSelector string
}

// StrategyControllerFunction defines the function signature to return a StrategyController
type StrategyControllerFunction func(
	ctx context.Context,
	client clientset.Interface,
	sharedInformerFactory informers.SharedInformerFactory,
	strategy api.DeschedulerStrategy,
	podEvictor *evictions.PodEvictor,
	nodeSelector string,
	stopChannel chan struct{},
) *StrategyController

const (
	workQueueKey = "key"
)

func (c *StrategyController) Run(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()
	klog.InfoS("Starting strategy with informers", "strategy", c.name)
	defer klog.InfoS("Shutting down strategy with informers", "strategy", c.name)

	go wait.Until(c.runWorker, time.Second, stopCh)

	<-stopCh
}

func (c *StrategyController) runWorker() {
	for c.processNextWorkItem() {
	}
}

func (c *StrategyController) processNextWorkItem() bool {
	dsKey, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(dsKey)

	c.f(c.ctx, c.client, c.strategy, c.nodes, c.podEvictor)

	c.queue.Forget(dsKey)
	return true
}
