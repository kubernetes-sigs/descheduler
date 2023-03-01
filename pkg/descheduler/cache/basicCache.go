package cache

import (
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/descheduler/pkg/api"
	"time"
)

type NodeInfoShadow struct {
	Node *v1.Node
	Pods []*v1.Pod
}

type NodeUsageMap struct {
	Node         *v1.Node          `json:"-"`
	UsageList    []v1.ResourceList `json:"usageList"`
	AllPods      []*PodUsageMap    `json:"-"`
	CurrentUsage v1.ResourceList
}
type PodUsageMap struct {
	Pod       *v1.Pod           `json:"-"`
	UsageList []v1.ResourceList `json:"-"`
}

type QueryCacheOption struct {
	NodeSelector     string
	Thresholds       api.ResourceThresholds
	TargetThresholds api.ResourceThresholds
}

type BasicCache interface {
	Run(period time.Duration)
	GetReadyNodeUsage(option *QueryCacheOption) map[string]*NodeUsageMap
}

var innerCache BasicCache

func GetCache() BasicCache {
	return innerCache
}
