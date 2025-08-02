package extender

import (
	"context"
	"fmt"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
)

const PluginName = "ExternalDecision"

// ExternalDecision
type ExternalDecision struct {
	logger    klog.Logger
	handle    frameworktypes.Handle
	args      *ExternalDecisionArgs
	podFilter podutil.FilterFunc
	client    *ExtenderClient
}

func (d *ExternalDecision) Name() string {
	return PluginName
}

var _ frameworktypes.DeschedulePlugin = &ExternalDecision{}

// New builds plugin from its arguments while passing a handle
func New(ctx context.Context, args runtime.Object, handle frameworktypes.Handle) (frameworktypes.Plugin, error) {
	extArgs, ok := args.(*ExternalDecisionArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type RemoveFailedPodsArgs, got %T", args)
	}

	if extArgs.Extender == nil {
		return nil, nil
	}
	if extArgs.Extender.URLPrefix == "" {
		return nil, nil
	}
	if extArgs.Extender.HTTPTimeout.Duration <= 0 {
		return nil, nil
	}

	if extArgs.Extender.FailPolicy == "" {
		extArgs.Extender.FailPolicy = "Deny"
	}

	if extArgs.Extender == nil {
		return nil, nil
	}

	client, err := NewExtenderClient(extArgs.Extender, klog.FromContext(ctx))
	if err != nil {
		return nil, nil
	}

	logger := klog.FromContext(ctx).WithValues("plugin", PluginName)

	podFilter, err := podutil.NewOptions().
		WithFilter(podutil.WrapFilterFuncs(handle.Evictor().Filter, handle.Evictor().PreEvictionFilter)).
		BuildFilterFunc()
	if err != nil {
		return nil, fmt.Errorf("error initializing pod filter function: %v", err)
	}

	return &ExternalDecision{
		logger:    logger,
		handle:    handle,
		podFilter: podFilter,
		client:    client,
	}, nil
}

// Deschedule extension point implementation for the ExternalDecision plugin
func (d *ExternalDecision) Deschedule(ctx context.Context, nodes []*v1.Node) *frameworktypes.Status {
	podsToEvict := make([]*v1.Pod, 0)
	nodeMap := make(map[string]*v1.Node, len(nodes))
	logger := klog.FromContext(klog.NewContext(ctx, d.logger)).WithValues("ExtensionPoint", frameworktypes.DescheduleExtensionPoint)

	// Build node map and collect candidate pods
	for _, node := range nodes {
		logger.V(2).Info("Processing node", "node", klog.KObj(node))
		pods, err := podutil.ListAllPodsOnANode(node.Name, d.handle.GetPodsAssignedToNodeFunc(), d.podFilter)
		if err != nil {
			return &frameworktypes.Status{
				Err: fmt.Errorf("error listing pods on node %s: %v", node.Name, err),
			}
		}

		nodeMap[node.Name] = node
		podsToEvict = append(podsToEvict, pods...)
	}

	// Evaluate each pod
	for i := range podsToEvict {
		pod := podsToEvict[i]
		node, exists := nodeMap[pod.Spec.NodeName]
		if !exists {
			logger.V(4).Info("Node not found in map, skipping pod", "pod", klog.KObj(pod), "node", pod.Spec.NodeName)
			continue
		}

		shouldEvict, reason, err := d.evaluatePod(ctx, pod, node)
		if err != nil {
			d.handleError(logger, pod, err)
			continue
		}

		if !shouldEvict {
			logger.V(3).Info("Eviction denied by external decision", "pod", klog.KObj(pod), "reason", reason)
			continue
		}

	loop:
		for _, pod := range podsToEvict {
			err := d.handle.Evictor().Evict(ctx, pod, evictions.EvictOptions{StrategyName: PluginName})
			if err == nil {
				continue
			}
			switch err.(type) {
			case *evictions.EvictionNodeLimitError:
				continue loop
			case *evictions.EvictionTotalLimitError:
				return nil
			default:
				logger.Error(err, "eviction failed")
			}
		}

	}

	return nil
}

// evaluatePod calls the external service to decide if a pod can be evicted.
func (d *ExternalDecision) evaluatePod(ctx context.Context, pod *v1.Pod, node *v1.Node) (bool, string, error) {
	allow, reason, err := d.client.DecideEviction(ctx, pod, node)
	if err != nil {
		return false, "", fmt.Errorf("extender call failed: %v", err)
	}
	return allow, reason, nil
}

// handleError logs and handles errors from external decision service based on FailPolicy.
func (d *ExternalDecision) handleError(logger klog.Logger, pod *v1.Pod, err error) {
	logger.Error(err, "External decision service error", "pod", klog.KObj(pod))

	switch d.args.Extender.FailPolicy {
	case "Allow":
		logger.V(2).Info("failPolicy=Allow, proceeding with eviction", "pod", klog.KObj(pod))
	case "Ignore":
		logger.V(2).Info("failPolicy=Ignore, skipping pod", "pod", klog.KObj(pod))
	case "Deny", "":
		logger.V(2).Info("failPolicy=Deny, blocking eviction", "pod", klog.KObj(pod))
	default:
		logger.Error(nil, "Unknown failPolicy, defaulting to Deny", "failPolicy", d.args.Extender.FailPolicy, "pod", klog.KObj(pod))
	}
}
