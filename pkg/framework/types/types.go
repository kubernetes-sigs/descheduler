/*
Copyright 2022 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package types

import (
	"context"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	"sigs.k8s.io/descheduler/pkg/descheduler/metricscollector"

	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
)

// Handle provides handles used by plugins to retrieve a kubernetes client set,
// evictor interface, shared informer factory and other instruments shared
// across plugins.
type Handle interface {
	// ClientSet returns a kubernetes clientSet.
	ClientSet() clientset.Interface
	Evictor() Evictor
	GetPodsAssignedToNodeFunc() podutil.GetPodsAssignedToNodeFunc
	SharedInformerFactory() informers.SharedInformerFactory
	MetricsCollector() *metricscollector.MetricsCollector
}

// Evictor defines an interface for filtering and evicting pods
// while abstracting away the specific pod evictor/evictor filter.
type Evictor interface {
	// Filter checks if a pod can be evicted
	Filter(*v1.Pod) bool
	// PreEvictionFilter checks if pod can be evicted right before eviction
	PreEvictionFilter(*v1.Pod) bool
	// Evict evicts a pod (no pre-check performed)
	Evict(context.Context, *v1.Pod, evictions.EvictOptions) error
}

// Status describes result of an extension point invocation
type Status struct {
	Err error
}

// Plugin is the parent type for all the descheduling framework plugins.
type Plugin interface {
	Name() string
}

// DeschedulePlugin defines an extension point for a general descheduling operation
type DeschedulePlugin interface {
	Plugin
	Deschedule(ctx context.Context, nodes []*v1.Node) *Status
}

// BalancePlugin defines an extension point for balancing pods across a cluster
type BalancePlugin interface {
	Plugin
	Balance(ctx context.Context, nodes []*v1.Node) *Status
}

// EvictorPlugin defines extension points for a general evictor behavior
// Even though we name this plugin interface EvictorPlugin, it does not actually evict anything,
// This plugin is only meant to customize other actions (extension points) of the evictor,
// like filtering, sorting, and other ones that might be relevant in the future
type EvictorPlugin interface {
	Plugin
	Filter(pod *v1.Pod) bool
	PreEvictionFilter(pod *v1.Pod) bool
}

type ExtensionPoint string

const (
	DescheduleExtensionPoint        ExtensionPoint = "Deschedule"
	BalanceExtensionPoint           ExtensionPoint = "Balance"
	FilterExtensionPoint            ExtensionPoint = "Filter"
	PreEvictionFilterExtensionPoint ExtensionPoint = "PreEvictionFilter"
)
