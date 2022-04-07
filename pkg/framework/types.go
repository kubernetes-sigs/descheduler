package framework

import (
	"context"
	"fmt"

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
	Err error
}

// Plugin is the parent type for all the descheduling framework plugins.
type Plugin interface {
	Name() string
}

type DeschedulePlugin interface {
	Deschedule(ctx context.Context, nodes []*v1.Node) *Status
}

type CommonArgs struct {
	Namespaces        *api.Namespaces
	PriorityThreshold *api.PriorityThreshold
	NodeFit           bool
}

// RemoveDuplicatePodsArg holds arguments used to configure the RemoveDuplicatePods plugin.
type RemoveDuplicatePodsArg struct {
	metav1.TypeMeta

	CommonArgs
	ExcludeOwnerKinds []string
}

// TODO(jchaloup): have this go generated
func (in *RemoveDuplicatePodsArg) DeepCopyObject() runtime.Object {
	return nil
}

// RemoveFailedPodsArg holds arguments used to configure the RemoveFailedPods plugin.
type RemoveFailedPodsArg struct {
	metav1.TypeMeta

	CommonArgs
	LabelSelector           *metav1.LabelSelector
	MinPodLifetimeSeconds   *uint
	Reasons                 []string
	IncludingInitContainers bool
	ExcludeOwnerKinds       []string
}

// TODO(jchaloup): have this go generated
func (in *RemoveFailedPodsArg) DeepCopyObject() runtime.Object {
	return nil
}

// RemovePodsViolatingNodeAffinityArg holds arguments used to configure the RemovePodsViolatingNodeAffinity plugin.
type RemovePodsViolatingNodeAffinityArg struct {
	metav1.TypeMeta

	CommonArgs
	LabelSelector    *metav1.LabelSelector
	NodeAffinityType []string
}

// TODO(jchaloup): have this go generated
func (in *RemovePodsViolatingNodeAffinityArg) DeepCopyObject() runtime.Object {
	return nil
}

// RemovePodsViolatingNodeTaintsArg holds arguments used to configure the RemovePodsViolatingNodeTaints plugin.
type RemovePodsViolatingNodeTaintsArg struct {
	metav1.TypeMeta

	CommonArgs
	LabelSelector           *metav1.LabelSelector
	IncludePreferNoSchedule bool
	ExcludedTaints          []string
}

// TODO(jchaloup): have this go generated
func (in *RemovePodsViolatingNodeTaintsArg) DeepCopyObject() runtime.Object {
	return nil
}

func ValidateCommonArgs(args CommonArgs) error {
	// At most one of include/exclude can be set
	if args.Namespaces != nil && len(args.Namespaces.Include) > 0 && len(args.Namespaces.Exclude) > 0 {
		return fmt.Errorf("only one of Include/Exclude namespaces can be set")
	}
	if args.PriorityThreshold != nil && args.PriorityThreshold.Value != nil && args.PriorityThreshold.Name != "" {
		return fmt.Errorf("only one of priorityThreshold fields can be set")
	}

	return nil
}
