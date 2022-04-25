package framework

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"

	"sigs.k8s.io/descheduler/pkg/api"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
)

type Handle interface {
	// ClientSet returns a kubernetes clientSet.
	ClientSet() clientset.Interface
	Evictor() Evictor
	GetPodsAssignedToNodeFunc() podutil.GetPodsAssignedToNodeFunc
	SharedInformerFactory() informers.SharedInformerFactory
}

type Evictor interface {
	// Sort pods from the most to the least suitable for eviction
	Sort([]*v1.Pod)
	// Filter checks if a pod can be evicted
	Filter(*v1.Pod) bool
	// Evict evicts a pod (no pre-check performed)
	Evict(context.Context, *v1.Pod) bool
}

type Evictable interface {
	Evict(context.Context, *v1.Pod) bool
}

type Status struct {
	Err error
}

// Plugin is the parent type for all the descheduling framework plugins.
type Plugin interface {
	Name() string
}

type DeschedulePlugin interface {
	Plugin
	Deschedule(ctx context.Context, nodes []*v1.Node) *Status
}

type BalancePlugin interface {
	Plugin
	Balance(ctx context.Context, nodes []*v1.Node) *Status
}

// Sort plugin sorts pods
type PreSortPlugin interface {
	Plugin
	PreLess(*v1.Pod, *v1.Pod) bool
}

type SortPlugin interface {
	Plugin
	Less(*v1.Pod, *v1.Pod) bool
}

type EvictPlugin interface {
	Plugin
	Filter(*v1.Pod) bool
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

// RemovePodsViolatingTopologySpreadConstraintArgs holds arguments used to configure the RemovePodsViolatingTopologySpreadConstraint plugin.
type RemovePodsViolatingTopologySpreadConstraintArgs struct {
	metav1.TypeMeta

	CommonArgs
	LabelSelector          *metav1.LabelSelector
	IncludeSoftConstraints bool
}

// TODO(jchaloup): have this go generated
func (in *RemovePodsViolatingTopologySpreadConstraintArgs) DeepCopyObject() runtime.Object {
	return nil
}

// LowNodeUtilizationArgs holds arguments used to configure the LowNodeUtilization plugin.
type LowNodeUtilizationArgs struct {
	metav1.TypeMeta

	PriorityThreshold      *api.PriorityThreshold
	NodeFit                bool
	UseDeviationThresholds bool
	Thresholds             api.ResourceThresholds
	TargetThresholds       api.ResourceThresholds
	NumberOfNodes          int
}

// TODO(jchaloup): have this go generated
func (in *LowNodeUtilizationArgs) DeepCopyObject() runtime.Object {
	return nil
}

// HighNodeUtilizationArgs holds arguments used to configure the HighNodeUtilization plugin.
type HighNodeUtilizationArgs struct {
	metav1.TypeMeta

	PriorityThreshold *api.PriorityThreshold
	NodeFit           bool
	Thresholds        api.ResourceThresholds
	TargetThresholds  api.ResourceThresholds
	NumberOfNodes     int
}

// TODO(jchaloup): have this go generated
func (in *HighNodeUtilizationArgs) DeepCopyObject() runtime.Object {
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

// DefaultEvictorArgs holds arguments used to configure the DefaultEvictor plugin.
type DefaultEvictorArgs struct {
	metav1.TypeMeta

	EvictFailedBarePods     bool
	EvictLocalStoragePods   bool
	EvictSystemCriticalPods bool
	IgnorePvcPods           bool
	PriorityThreshold       *api.PriorityThreshold
	NodeFit                 bool
	LabelSelector           *metav1.LabelSelector
	// TODO(jchaloup): turn it into *metav1.LabelSelector
	NodeSelector string
}

// TODO(jchaloup): have this go generated
func (in *DefaultEvictorArgs) DeepCopyObject() runtime.Object {
	return nil
}
