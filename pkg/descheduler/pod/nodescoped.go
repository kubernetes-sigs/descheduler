/*
Copyright 2026 The Kubernetes Authors.

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

package pod

import (
	"context"
	"maps"
	"strconv"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/watch"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

const (
	nodeScopedSyncPollInterval = 100 * time.Millisecond
	nodeScopedNodePollInterval = 5 * time.Second
)

// NodeNamesFunc returns the names of the nodes whose pods are to be cached.
type NodeNamesFunc func() ([]string, error)

// NodeScopedListerWatcher lists and watches only the pods assigned to a set of
// nodes, with one spec.nodeName field selector per node. When the node set
// changes, the watch stops and the reflector relists.
type NodeScopedListerWatcher struct {
	client           clientset.Interface
	nodeNames        NodeNamesFunc
	nodesSynced      func() bool
	nodePollInterval time.Duration

	mu sync.Mutex
	// resourceVersions holds, per listed node, the version to resume its watch from.
	resourceVersions map[string]string
	// generation counts the lists, a watch of an older list must not touch resourceVersions.
	generation int
}

var (
	_ cache.ListerWatcher            = &NodeScopedListerWatcher{}
	_ cache.ListerWatcherWithContext = &NodeScopedListerWatcher{}
)

// NewNodeScopedListerWatcher returns a lister/watcher for the pods assigned to
// the nodes returned by nodeNames. nodesSynced reports whether nodeNames is
// backed by a synced cache; listing blocks until it is.
func NewNodeScopedListerWatcher(client clientset.Interface, nodeNames NodeNamesFunc, nodesSynced func() bool) *NodeScopedListerWatcher {
	return &NodeScopedListerWatcher{
		client:           client,
		nodeNames:        nodeNames,
		nodesSynced:      nodesSynced,
		nodePollInterval: nodeScopedNodePollInterval,
	}
}

// NewNodeScopedPodInformer returns a pod informer backed by lw.
func NewNodeScopedPodInformer(lw *NodeScopedListerWatcher, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(lw, &v1.Pod{}, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
}

// IsWatchListSemanticsUnSupported makes the reflector use list+watch: a
// streamed initial list cannot be merged across several watches.
func (lw *NodeScopedListerWatcher) IsWatchListSemanticsUnSupported() bool {
	return true
}

func (lw *NodeScopedListerWatcher) List(options metav1.ListOptions) (runtime.Object, error) {
	return lw.ListWithContext(context.Background(), options)
}

func (lw *NodeScopedListerWatcher) Watch(options metav1.ListOptions) (watch.Interface, error) {
	return lw.WatchWithContext(context.Background(), options)
}

func nodeNameSelector(nodeName string) string {
	return fields.OneTermEqualSelector("spec.nodeName", nodeName).String()
}

func (lw *NodeScopedListerWatcher) ListWithContext(ctx context.Context, options metav1.ListOptions) (runtime.Object, error) {
	for !lw.nodesSynced() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(nodeScopedSyncPollInterval):
		}
	}
	nodeNames, err := lw.nodeNames()
	if err != nil {
		return nil, err
	}

	// each per-node list is complete, the reflector's paging does not apply
	options.Limit = 0
	options.Continue = ""

	result := &v1.PodList{}
	resourceVersions := make(map[string]string, len(nodeNames))
	for _, nodeName := range nodeNames {
		options.FieldSelector = nodeNameSelector(nodeName)
		list, err := lw.client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, options)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, list.Items...)
		resourceVersions[nodeName] = list.ResourceVersion
		result.ResourceVersion = newerResourceVersion(result.ResourceVersion, list.ResourceVersion)
	}
	if len(nodeNames) == 0 {
		// no node selected: the reflector still needs a resource version
		options.FieldSelector = ""
		options.Limit = 1
		list, err := lw.client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, options)
		if err != nil {
			return nil, err
		}
		result.ResourceVersion = list.ResourceVersion
	}

	lw.mu.Lock()
	lw.resourceVersions = resourceVersions
	lw.generation++
	lw.mu.Unlock()
	return result, nil
}

// newerResourceVersion returns the larger of two resource versions, or b when
// they are not comparable.
func newerResourceVersion(a, b string) string {
	av, aErr := strconv.ParseUint(a, 10, 64)
	bv, bErr := strconv.ParseUint(b, 10, 64)
	if aErr == nil && bErr == nil && av > bv {
		return a
	}
	return b
}

func (lw *NodeScopedListerWatcher) WatchWithContext(ctx context.Context, options metav1.ListOptions) (watch.Interface, error) {
	nodeNames, err := lw.nodeNames()
	if err != nil {
		return nil, err
	}
	listed := sets.New(nodeNames...)

	lw.mu.Lock()
	resourceVersions := lw.resourceVersions
	if resourceVersions == nil || !listed.Equal(sets.KeySet(resourceVersions)) {
		lw.resourceVersions = nil
		lw.mu.Unlock()
		return nil, apierrors.NewResourceExpired("the set of selected nodes changed, pods need to be listed again")
	}
	// copy: the watches below update the versions while they run
	startVersions := maps.Clone(resourceVersions)
	generation := lw.generation
	lw.mu.Unlock()

	ctx, cancel := context.WithCancel(ctx)
	merged := &mergedWatch{
		result: make(chan watch.Event),
		cancel: cancel,
	}

	watches := make(map[string]watch.Interface, len(startVersions))
	for nodeName, rv := range startVersions {
		nodeOptions := options
		nodeOptions.FieldSelector = nodeNameSelector(nodeName)
		nodeOptions.ResourceVersion = rv
		nodeOptions.AllowWatchBookmarks = true
		w, err := lw.client.CoreV1().Pods(metav1.NamespaceAll).Watch(ctx, nodeOptions)
		if err != nil {
			cancel()
			for _, started := range watches {
				started.Stop()
			}
			return nil, err
		}
		watches[nodeName] = w
	}

	for nodeName, w := range watches {
		merged.wg.Go(func() {
			// one watch ending ends all of them, the reflector then starts over
			defer cancel()
			defer w.Stop()
			lw.forward(ctx, generation, nodeName, w, merged.result)
		})
	}

	merged.wg.Go(func() {
		defer cancel()
		lw.waitForNodeChange(ctx, listed)
	})

	go func() {
		merged.wg.Wait()
		close(merged.result)
	}()

	return merged, nil
}

// forward copies the events of one node's watch to result and records the
// version to resume that watch from.
func (lw *NodeScopedListerWatcher) forward(ctx context.Context, generation int, nodeName string, w watch.Interface, result chan<- watch.Event) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-w.ResultChan():
			if !ok {
				return
			}
			// a bookmark covers one node only, the reflector must not take it for all of them
			if event.Type != watch.Bookmark {
				select {
				case <-ctx.Done():
					return
				case result <- event:
				}
			}
			if event.Type == watch.Error {
				return
			}
			// only after the event is delivered, a resumed watch must not skip it
			if accessor, err := meta.Accessor(event.Object); err == nil {
				lw.setResourceVersion(generation, nodeName, accessor.GetResourceVersion())
			}
		}
	}
}

func (lw *NodeScopedListerWatcher) setResourceVersion(generation int, nodeName, resourceVersion string) {
	if resourceVersion == "" {
		return
	}
	lw.mu.Lock()
	defer lw.mu.Unlock()
	if lw.generation != generation {
		return
	}
	if _, ok := lw.resourceVersions[nodeName]; ok {
		lw.resourceVersions[nodeName] = resourceVersion
	}
}

// waitForNodeChange returns once the selected nodes differ from listed.
func (lw *NodeScopedListerWatcher) waitForNodeChange(ctx context.Context, listed sets.Set[string]) {
	ticker := time.NewTicker(lw.nodePollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			nodeNames, err := lw.nodeNames()
			if err != nil || !listed.Equal(sets.New(nodeNames...)) {
				return
			}
		}
	}
}

type mergedWatch struct {
	result chan watch.Event
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func (m *mergedWatch) Stop() {
	m.cancel()
}

func (m *mergedWatch) ResultChan() <-chan watch.Event {
	return m.result
}
