package framework

import (
	"context"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientset "k8s.io/client-go/kubernetes"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
)

type Handle interface {
	// ClientSet returns a kubernetes clientSet.
	ClientSet() clientset.Interface

	PodEvictor() *evictions.PodEvictor
	GetPodsAssignedToNodeFunc() podutil.GetPodsAssignedToNodeFunc
}

type Status struct {
	err error
}

// Plugin is the parent type for all the descheduling framework plugins.
type Plugin interface {
	Name() string
}

type DeschedulePlugin interface {
	Deschedule(ctx context.Context, nodes []*v1.Node) *Status
}

// RemoveDuplicatePodsArg holds arguments used to configure the RemoveDuplicatePods plugin.
type RemoveDuplicatePodsArg struct {
	metav1.TypeMeta

	Namespaces        *api.Namespaces
	PriorityThreshold *api.PriorityThreshold
	NodeFit           bool
	ExcludeOwnerKinds []string
}

// TODO(jchaloup): have this go generated
func (in *RemoveDuplicatePodsArg) DeepCopyObject() runtime.Object {
	return nil
}
