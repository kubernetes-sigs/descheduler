package strategies

import "sigs.k8s.io/descheduler/pkg/api"

const (
	RemoveDuplicates                            api.StrategyName = "RemoveDuplicates"
	RemoveFailedPods                            api.StrategyName = "RemoveFailedPods"
	RemovePodsViolatingNodeAffinity             api.StrategyName = "RemovePodsViolatingNodeAffinity"
	RemovePodsViolatingNodeTaints               api.StrategyName = "RemovePodsViolatingNodeTaints"
	RemovePodsViolatingInterPodAntiAffinity     api.StrategyName = "RemovePodsViolatingInterPodAntiAffinity"
	PodLifeTime                                 api.StrategyName = "PodLifeTime"
	RemovePodsHavingTooManyRestarts             api.StrategyName = "RemovePodsHavingTooManyRestarts"
	RemovePodsViolatingTopologySpreadConstraint api.StrategyName = "RemovePodsViolatingTopologySpreadConstraint"
)
