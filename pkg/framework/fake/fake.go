package fake

import (
	"context"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"

	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
)

type HandleImpl struct {
	ClientsetImpl                 clientset.Interface
	GetPodsAssignedToNodeFuncImpl podutil.GetPodsAssignedToNodeFunc
	SharedInformerFactoryImpl     informers.SharedInformerFactory
	EvictorFilterImpl             frameworktypes.EvictorPlugin
	PodEvictorImpl                *evictions.PodEvictor
}

var _ frameworktypes.Handle = &HandleImpl{}

func (hi *HandleImpl) ClientSet() clientset.Interface {
	return hi.ClientsetImpl
}

func (hi *HandleImpl) GetPodsAssignedToNodeFunc() podutil.GetPodsAssignedToNodeFunc {
	return hi.GetPodsAssignedToNodeFuncImpl
}

func (hi *HandleImpl) SharedInformerFactory() informers.SharedInformerFactory {
	return hi.SharedInformerFactoryImpl
}

func (hi *HandleImpl) Evictor() frameworktypes.Evictor {
	return hi
}

func (hi *HandleImpl) Filter(pod *v1.Pod) bool {
	return hi.EvictorFilterImpl.Filter(pod)
}

func (hi *HandleImpl) PreEvictionFilter(pod *v1.Pod) bool {
	return hi.EvictorFilterImpl.PreEvictionFilter(pod)
}

func (hi *HandleImpl) Evict(ctx context.Context, pod *v1.Pod, opts evictions.EvictOptions) bool {
	return hi.PodEvictorImpl.EvictPod(ctx, pod, opts)
}

func (hi *HandleImpl) NodeLimitExceeded(node *v1.Node) bool {
	return hi.PodEvictorImpl.NodeLimitExceeded(node)
}
