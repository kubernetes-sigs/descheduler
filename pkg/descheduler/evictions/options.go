package evictions

import (
	policy "k8s.io/api/policy/v1"
)

type Options struct {
	policyGroupVersion               string
	dryRun                           bool
	maxPodsToEvictPerNode            *uint
	maxPodsToEvictPerNamespace       *uint
	maxPodsToEvictTotal              *uint
	evictionFailureEventNotification bool
	metricsEnabled                   bool
	gracePeriodSeconds               *int64
}

// NewOptions returns an Options with default values.
func NewOptions() *Options {
	return &Options{
		policyGroupVersion: policy.SchemeGroupVersion.String(),
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

func (o *Options) WithGracePeriodSeconds(gracePeriodSeconds *int64) *Options {
	o.gracePeriodSeconds = gracePeriodSeconds
	return o
}

func (o *Options) WithMetricsEnabled(metricsEnabled bool) *Options {
	o.metricsEnabled = metricsEnabled
	return o
}

func (o *Options) WithEvictionFailureEventNotification(evictionFailureEventNotification *bool) *Options {
	if evictionFailureEventNotification != nil {
		o.evictionFailureEventNotification = *evictionFailureEventNotification
	}
	return o
}
