package testing

import (
	"context"
	"sort"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"

	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/framework"
)

type FrameworkHandle struct {
	ClientsetImpl                 clientset.Interface
	EvictorImpl                   *framework.PodEvictor
	GetPodsAssignedToNodeFuncImpl podutil.GetPodsAssignedToNodeFunc
	SharedInformerFactoryImpl     informers.SharedInformerFactory
	EvictPlugin                   framework.EvictPlugin
	SortPlugin                    framework.SortPlugin
}

var _ framework.Handle = &FrameworkHandle{}
var _ framework.Evictor = &FrameworkHandle{}

func (f *FrameworkHandle) ClientSet() clientset.Interface {
	return f.ClientsetImpl
}

func (f *FrameworkHandle) Evictor() framework.Evictor {
	return f
}

func (f *FrameworkHandle) GetPodsAssignedToNodeFunc() podutil.GetPodsAssignedToNodeFunc {
	return f.GetPodsAssignedToNodeFuncImpl
}

func (f *FrameworkHandle) SharedInformerFactory() informers.SharedInformerFactory {
	return f.SharedInformerFactoryImpl
}

// Sort pods from the most to the least suitable for eviction
func (f *FrameworkHandle) Sort(pods []*v1.Pod) {
	sort.Slice(pods, func(i int, j int) bool {
		return f.SortPlugin.Less(pods[i], pods[j])
	})
}

// Filter checks if a pod can be evicted
func (f *FrameworkHandle) Filter(pod *v1.Pod) bool {
	return f.EvictPlugin.Filter(pod)
}

// Evict evicts a pod (no pre-check performed)
func (f *FrameworkHandle) Evict(ctx context.Context, pod *v1.Pod) bool {
	return f.EvictorImpl.Evict(ctx, pod)
}
