package strategies

import (
	"context"
	v1 "k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
)

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
	ctx    context.Context
	client clientset.Interface
	queue  workqueue.RateLimitingInterface
	f      StrategyFunction
}

// StrategyControllerFunction defines the function signature to return a StrategyController
type StrategyControllerFunction func(
	ctx context.Context,
	client clientset.Interface,
	f StrategyFunction,
) *StrategyController