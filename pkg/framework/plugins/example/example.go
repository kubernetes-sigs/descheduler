/*
Copyright 2025 The Kubernetes Authors.

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

package example

import (
	"context"
	"fmt"
	"regexp"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"

	fwtypes "sigs.k8s.io/descheduler/pkg/framework/types"

	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
)

// PluginName is used when registering the plugin. You need to choose a unique
// name across all plugins. This name is used to identify the plugin config in
// the descheduler policy.
const PluginName = "Example"

// We need to ensure that the plugin struct complies with the DeschedulePlugin
// interface. This prevent unexpected changes that may render this type
// incompatible.
var _ fwtypes.DeschedulePlugin = &Example{}

// Example is our plugin (implementing the DeschedulePlugin interface). This
// plugin will evict pods that match a regex and are older than a certain age.
type Example struct {
	handle    fwtypes.Handle
	args      *ExampleArgs
	podFilter podutil.FilterFunc
}

// New builds a plugin instance from its arguments. Arguments are passed in as
// a runtime.Object. Handle is used by plugins to retrieve a kubernetes client
// set, evictor interface, shared informer factory and other instruments shared
// across different plugins.
func New(args runtime.Object, handle fwtypes.Handle) (fwtypes.Plugin, error) {
	// make sure we are receiving the right argument type.
	exampleArgs, ok := args.(*ExampleArgs)
	if !ok {
		return nil, fmt.Errorf("args must be of type ExampleArgs, got %T", args)
	}

	// we can use the included and excluded namespaces to filter the pods we want
	// to evict.
	var includedNamespaces, excludedNamespaces sets.Set[string]
	if exampleArgs.Namespaces != nil {
		includedNamespaces = sets.New(exampleArgs.Namespaces.Include...)
		excludedNamespaces = sets.New(exampleArgs.Namespaces.Exclude...)
	}

	// here we create a pod filter that will return only pods that can be
	// evicted (according to the evictor and inside the namespaces we want).
	// NOTE: here we could also add a function to filter out by the regex and
	// age but for sake of the example we are keeping it simple and filtering
	// those out in the Deschedule() function.
	podFilter, err := podutil.NewOptions().
		WithNamespaces(includedNamespaces).
		WithoutNamespaces(excludedNamespaces).
		WithFilter(
			podutil.WrapFilterFuncs(
				handle.Evictor().Filter,
				handle.Evictor().PreEvictionFilter,
			),
		).
		BuildFilterFunc()
	if err != nil {
		return nil, fmt.Errorf("error initializing pod filter function: %v", err)
	}

	return &Example{
		handle:    handle,
		podFilter: podFilter,
		args:      exampleArgs,
	}, nil
}

// Name returns the plugin name.
func (d *Example) Name() string {
	return PluginName
}

// Deschedule is the function where most of the logic around eviction is laid
// down. Here we go through all pods in all nodes and evict the ones that match
// the regex and are older than the maximum age. This function receives a list
// of nodes we need to process.
func (d *Example) Deschedule(ctx context.Context, nodes []*v1.Node) *fwtypes.Status {
	var podsToEvict []*v1.Pod
	logger := klog.FromContext(ctx)
	logger.Info("Example plugin starting descheduling")

	re, err := regexp.Compile(d.args.Regex)
	if err != nil {
		err = fmt.Errorf("fail to compile regex: %w", err)
		return &fwtypes.Status{Err: err}
	}

	duration, err := time.ParseDuration(d.args.MaxAge)
	if err != nil {
		err = fmt.Errorf("fail to parse max age: %w", err)
		return &fwtypes.Status{Err: err}
	}

	// here we create an auxiliar filter to remove all pods that don't
	// match the provided regex or are still too young to be evicted.
	// This filter will be used when we list all pods on a node. This
	// filter here could have been part of the podFilter but we are
	// keeping it separate for the sake of the example.
	filter := func(pod *v1.Pod) bool {
		if !re.MatchString(pod.Name) {
			return false
		}
		deadline := pod.CreationTimestamp.Add(duration)
		return time.Now().After(deadline)
	}

	// go node by node getting all pods that we can evict.
	for _, node := range nodes {
		// ListAllPodsOnANode is a helper function that retrieves all
		// pods filtering out the ones we can't evict. We merge the
		// default filters with the one we created above.
		pods, err := podutil.ListPodsOnANode(
			node.Name,
			d.handle.GetPodsAssignedToNodeFunc(),
			podutil.WrapFilterFuncs(d.podFilter, filter),
		)
		if err != nil {
			err = fmt.Errorf("fail to list pods: %w", err)
			return &fwtypes.Status{Err: err}
		}

		// as we have already filtered out pods that don't match the
		// regex or are too young we can simply add them all to the
		// eviction list.
		podsToEvict = append(podsToEvict, pods...)
	}

	// evict all the pods.
	for _, pod := range podsToEvict {
		logger.Info("Example plugin evicting pod", "pod", klog.KObj(pod))
		opts := evictions.EvictOptions{StrategyName: PluginName}
		if err := d.handle.Evictor().Evict(ctx, pod, opts); err != nil {
			logger.Error(err, "unable to evict pod", "pod", klog.KObj(pod))
		}
	}

	logger.Info("Example plugin finished descheduling")
	return nil
}
