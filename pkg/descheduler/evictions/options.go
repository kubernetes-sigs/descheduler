package evictions

import (
	"time"

	policy "k8s.io/api/policy/v1"
)

type Options struct {
	policyGroupVersion         string
	dryRun                     bool
	maxPodsToEvictPerNode      *uint
	maxPodsToEvictPerNamespace *uint
	maxPodsToEvictTotal        *uint
	metricsEnabled             bool

	assumedRequestTimeout             uint
	evictionRequestsCacheResyncPeriod time.Duration
}

// NewOptions returns an Options with default values.
func NewOptions() *Options {
	return &Options{
		policyGroupVersion:                policy.SchemeGroupVersion.String(),
		assumedRequestTimeout:             AssumedEvictionRequestTimeoutSeconds,
		evictionRequestsCacheResyncPeriod: EvictionRequestsCacheResyncPeriod,
	}
}

func (o *Options) WithPolicyGroupVersion(policyGroupVersion string) *Options {
	o.policyGroupVersion = policyGroupVersion
	return o
}

func (o *Options) WithDryRun(dryRun bool) *Options {
	o.dryRun = dryRun
	return o
}

func (o *Options) WithMaxPodsToEvictPerNode(maxPodsToEvictPerNode *uint) *Options {
	o.maxPodsToEvictPerNode = maxPodsToEvictPerNode
	return o
}

func (o *Options) WithMaxPodsToEvictPerNamespace(maxPodsToEvictPerNamespace *uint) *Options {
	o.maxPodsToEvictPerNamespace = maxPodsToEvictPerNamespace
	return o
}

func (o *Options) WithMaxPodsToEvictTotal(maxPodsToEvictTotal *uint) *Options {
	o.maxPodsToEvictTotal = maxPodsToEvictTotal
	return o
}

func (o *Options) WithMetricsEnabled(metricsEnabled bool) *Options {
	o.metricsEnabled = metricsEnabled
	return o
}

func (o *Options) WithAssumedRequestTimeout(assumedRequestTimeout uint) *Options {
	o.assumedRequestTimeout = assumedRequestTimeout
	return o
}

func (o *Options) WithEvictionRequestsCacheResyncPeriod(evictionRequestsCacheResyncPeriod time.Duration) *Options {
	o.evictionRequestsCacheResyncPeriod = evictionRequestsCacheResyncPeriod
	return o
}
