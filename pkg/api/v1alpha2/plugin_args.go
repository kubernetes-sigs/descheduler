package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/descheduler/pkg/api"
)

// RemoveDuplicatePodsArgs holds arguments used to configure the RemoveDuplicatePods plugin.
type RemoveDuplicatePodsArgs struct {
	metav1.TypeMeta

	Namespaces        *api.Namespaces `json:"namespaces"`
	ExcludeOwnerKinds []string        `json:"excludeOwnerKinds"`
}

// RemoveFailedPodsArgs holds arguments used to configure the RemoveFailedPods plugin.
type RemoveFailedPodsArgs struct {
	metav1.TypeMeta

	Namespaces              *api.Namespaces       `json:"namespaces"`
	LabelSelector           *metav1.LabelSelector `json:"labelSelector"`
	MinPodLifetimeSeconds   *uint                 `json:"minPodLifetimeSeconds"`
	Reasons                 []string              `json:"reasons"`
	IncludingInitContainers bool                  `json:"includingInitContainers"`
	ExcludeOwnerKinds       []string              `json:"excludeOwnerKinds"`
}

// RemovePodsViolatingNodeAffinityArgs holds arguments used to configure the RemovePodsViolatingNodeAffinity plugin.
type RemovePodsViolatingNodeAffinityArgs struct {
	metav1.TypeMeta

	Namespaces       *api.Namespaces       `json:"namespaces"`
	LabelSelector    *metav1.LabelSelector `json:"labelSelector"`
	NodeAffinityType []string              `json:"nodeAffinityType"`
}

// RemovePodsViolatingNodeTaintsArgs holds arguments used to configure the RemovePodsViolatingNodeTaints plugin.
type RemovePodsViolatingNodeTaintsArgs struct {
	metav1.TypeMeta

	Namespaces              *api.Namespaces       `json:"namespaces"`
	LabelSelector           *metav1.LabelSelector `json:"labelSelector"`
	IncludePreferNoSchedule bool                  `json:"includePreferNoSchedule"`
	ExcludedTaints          []string              `json:"excludedTaints"`
}

// RemovePodsViolatingInterPodAntiAffinityArgs holds arguments used to configure the RemovePodsViolatingInterPodAntiAffinity plugin.
type RemovePodsViolatingInterPodAntiAffinityArgs struct {
	metav1.TypeMeta

	Namespaces    *api.Namespaces       `json:"namespaces"`
	LabelSelector *metav1.LabelSelector `json:"labelSelector"`
}

// PodLifeTimeArgs holds arguments used to configure the PodLifeTime plugin.
type PodLifeTimeArgs struct {
	metav1.TypeMeta

	Namespaces            *api.Namespaces       `json:"namespaces"`
	LabelSelector         *metav1.LabelSelector `json:"labelSelector"`
	MaxPodLifeTimeSeconds *uint                 `json:"maxPodLifeTimeSeconds"`
	PodStatusPhases       []string              `json:"podStatusPhases"`
}

// RemovePodsHavingTooManyRestartsArgs holds arguments used to configure the RemovePodsHavingTooManyRestarts plugin.
type RemovePodsHavingTooManyRestartsArgs struct {
	metav1.TypeMeta

	Namespaces              *api.Namespaces       `json:"namespaces"`
	LabelSelector           *metav1.LabelSelector `json:"labelSelector"`
	PodRestartThreshold     int32                 `json:"podRestartThreshold"`
	IncludingInitContainers bool                  `json:"includingInitContainers"`
}

// RemovePodsViolatingTopologySpreadConstraintArgs holds arguments used to configure the RemovePodsViolatingTopologySpreadConstraint plugin.
type RemovePodsViolatingTopologySpreadConstraintArgs struct {
	metav1.TypeMeta

	Namespaces             *api.Namespaces       `json:"namespaces"`
	LabelSelector          *metav1.LabelSelector `json:"labelSelector"`
	IncludeSoftConstraints bool                  `json:"includeSoftConstraints"`
}

// LowNodeUtilizationArgs holds arguments used to configure the LowNodeUtilization plugin.
type LowNodeUtilizationArgs struct {
	metav1.TypeMeta

	UseDeviationThresholds bool                   `json:"useDeviationThresholds"`
	Thresholds             api.ResourceThresholds `json:"thresholds"`
	TargetThresholds       api.ResourceThresholds `json:"targetThresholds"`
	NumberOfNodes          int                    `json:"numberOfNodes"`
}

// HighNodeUtilizationArgs holds arguments used to configure the HighNodeUtilization plugin.
type HighNodeUtilizationArgs struct {
	metav1.TypeMeta

	Thresholds api.ResourceThresholds `json:"thresholds"`
	// TODO(jchaloup): remove TargetThresholds
	TargetThresholds api.ResourceThresholds `json:"targetThresholds"`
	NumberOfNodes    int                    `json:"numberOfNodes"`
}

// DefaultEvictorArgs holds arguments used to configure the DefaultEvictor plugin.
type DefaultEvictorArgs struct {
	metav1.TypeMeta

	// EvictFailedBarePods allows pods without ownerReferences and in failed phase to be evicted.
	EvictFailedBarePods bool `json:"evictFailedBarePods"`

	// EvictLocalStoragePods allows pods using local storage to be evicted.
	EvictLocalStoragePods bool `json:"evictLocalStoragePods"`

	// EvictSystemCriticalPods allows eviction of pods of any priority (including Kubernetes system pods)
	EvictSystemCriticalPods bool `json:"evictSystemCriticalPods"`

	// IgnorePVCPods prevents pods with PVCs from being evicted.
	IgnorePvcPods bool `json:"ignorePvcPods"`

	PriorityThreshold *api.PriorityThreshold `json:"priorityThreshold"`
	NodeFit           bool                   `json:"nodeFit"`
	LabelSelector     *metav1.LabelSelector  `json:"labelSelector"`
	// TODO(jchaloup): turn it into *metav1.LabelSelector `json:"labelSelector"`
	NodeSelector string `json:"nodeSelector"`
}
