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

// RemoveDuplicatePodsArgs holds arguments used to configure the RemoveDuplicatePods plugin.
type RemoveDuplicatePodsArgs struct {
	metav1.TypeMeta

	CommonArgs
	ExcludeOwnerKinds []string
}

// TODO(jchaloup): have this go generated
func (in *RemoveDuplicatePodsArgs) DeepCopyObject() runtime.Object {
	return nil
}

// RemoveFailedPodsArgs holds arguments used to configure the RemoveFailedPods plugin.
type RemoveFailedPodsArgs struct {
	metav1.TypeMeta

	CommonArgs
	LabelSelector           *metav1.LabelSelector
	MinPodLifetimeSeconds   *uint
	Reasons                 []string
	IncludingInitContainers bool
	ExcludeOwnerKinds       []string
}

// TODO(jchaloup): have this go generated
func (in *RemoveFailedPodsArgs) DeepCopyObject() runtime.Object {
	return nil
}

// RemovePodsViolatingNodeAffinityArgs holds arguments used to configure the RemovePodsViolatingNodeAffinity plugin.
type RemovePodsViolatingNodeAffinityArgs struct {
	metav1.TypeMeta

	CommonArgs
	LabelSelector    *metav1.LabelSelector
	NodeAffinityType []string
}

// TODO(jchaloup): have this go generated
func (in *RemovePodsViolatingNodeAffinityArgs) DeepCopyObject() runtime.Object {
	return nil
}

// RemovePodsViolatingNodeTaintsArgs holds arguments used to configure the RemovePodsViolatingNodeTaints plugin.
type RemovePodsViolatingNodeTaintsArgs struct {
	metav1.TypeMeta

	CommonArgs
	LabelSelector           *metav1.LabelSelector
	IncludePreferNoSchedule bool
	ExcludedTaints          []string
}

// TODO(jchaloup): have this go generated
func (in *RemovePodsViolatingNodeTaintsArgs) DeepCopyObject() runtime.Object {
	return nil
}

// RemovePodsViolatingInterPodAntiAffinityArgs holds arguments used to configure the RemovePodsViolatingInterPodAntiAffinity plugin.
type RemovePodsViolatingInterPodAntiAffinityArgs struct {
	metav1.TypeMeta

	CommonArgs
	LabelSelector *metav1.LabelSelector
}

// TODO(jchaloup): have this go generated
func (in *RemovePodsViolatingInterPodAntiAffinityArgs) DeepCopyObject() runtime.Object {
	return nil
}

// PodLifeTimeArgs holds arguments used to configure the PodLifeTime plugin.
type PodLifeTimeArgs struct {
	metav1.TypeMeta

	CommonArgs
	LabelSelector         *metav1.LabelSelector
	MaxPodLifeTimeSeconds *uint
	PodStatusPhases       []string
}

// TODO(jchaloup): have this go generated
func (in *PodLifeTimeArgs) DeepCopyObject() runtime.Object {
	return nil
}

// RemovePodsHavingTooManyRestartsArgs holds arguments used to configure the RemovePodsHavingTooManyRestarts plugin.
type RemovePodsHavingTooManyRestartsArgs struct {
	metav1.TypeMeta

	CommonArgs
	LabelSelector           *metav1.LabelSelector
	PodRestartThreshold     int32
	IncludingInitContainers bool
}

// TODO(jchaloup): have this go generated
func (in *RemovePodsHavingTooManyRestartsArgs) DeepCopyObject() runtime.Object {
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
