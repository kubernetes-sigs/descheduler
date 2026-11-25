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
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
)

// nodeScopedCluster is a fake API server for pods that honours the
// spec.nodeName field selector, which the fake clientset ignores.
type nodeScopedCluster struct {
	mu       sync.Mutex
	pods     map[string]*v1.Pod
	watchers map[string][]*watch.FakeWatcher
	lists    []string
}

func nodeOf(selector string) string {
	return strings.TrimPrefix(selector, "spec.nodeName=")
}

func (c *nodeScopedCluster) list(action core.Action) (bool, runtime.Object, error) {
	selector := action.(core.ListAction).GetListRestrictions().Fields.String()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lists = append(c.lists, selector)
	list := &v1.PodList{ListMeta: metav1.ListMeta{ResourceVersion: "1"}}
	for _, pod := range c.pods {
		if pod.Spec.NodeName == nodeOf(selector) {
			list.Items = append(list.Items, *pod)
		}
	}
	return true, list, nil
}

func (c *nodeScopedCluster) watch(action core.Action) (bool, watch.Interface, error) {
	selector := action.(core.WatchAction).GetWatchRestrictions().Fields.String()
	c.mu.Lock()
	defer c.mu.Unlock()
	w := watch.NewFake()
	c.watchers[nodeOf(selector)] = append(c.watchers[nodeOf(selector)], w)
	return true, w, nil
}

// add stores the pod and sends it to the newest watch of its node.
func (c *nodeScopedCluster) add(pod *v1.Pod) {
	c.mu.Lock()
	c.pods[pod.Name] = pod
	watchers := c.watchers[pod.Spec.NodeName]
	c.mu.Unlock()
	if len(watchers) > 0 {
		watchers[len(watchers)-1].Add(pod)
	}
}

func nodeScopedTestPod(name, nodeName string) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", ResourceVersion: "2"},
		Spec:       v1.PodSpec{NodeName: nodeName},
	}
}

func TestNodeScopedPodInformer(t *testing.T) {
	cluster := &nodeScopedCluster{
		pods: map[string]*v1.Pod{
			"a1": nodeScopedTestPod("a1", "node-a"),
			"b1": nodeScopedTestPod("b1", "node-b"),
			"c1": nodeScopedTestPod("c1", "node-c"),
		},
		watchers: map[string][]*watch.FakeWatcher{},
	}
	client := fake.NewSimpleClientset()
	client.PrependReactor("list", "pods", cluster.list)
	client.PrependWatchReactor("pods", cluster.watch)

	var nodesMu sync.Mutex
	nodes := []string{"node-a", "node-b"}
	nodeNames := func() ([]string, error) {
		nodesMu.Lock()
		defer nodesMu.Unlock()
		return append([]string{}, nodes...), nil
	}

	lw := NewNodeScopedListerWatcher(client, nodeNames, func() bool { return true })
	lw.nodePollInterval = 10 * time.Millisecond
	informer := NewNodeScopedPodInformer(lw, 0)

	ctx := t.Context()
	go informer.Run(ctx.Done())

	cached := func() []string {
		names := []string{}
		for _, obj := range informer.GetStore().List() {
			names = append(names, obj.(*v1.Pod).Name)
		}
		sort.Strings(names)
		return names
	}
	expect := func(step string, want ...string) {
		t.Helper()
		err := wait.PollUntilContextTimeout(ctx, 10*time.Millisecond, 10*time.Second, true, func(context.Context) (bool, error) {
			return informer.HasSynced() && strings.Join(cached(), ",") == strings.Join(want, ","), nil
		})
		if err != nil {
			t.Fatalf("%s: cached pods %v, want %v", step, cached(), want)
		}
	}

	expect("initial list", "a1", "b1")

	cluster.mu.Lock()
	for _, selector := range cluster.lists {
		if nodeOf(selector) != "node-a" && nodeOf(selector) != "node-b" {
			t.Errorf("unexpected list with field selector %q", selector)
		}
	}
	cluster.mu.Unlock()

	// a pod on a selected node arrives through that node's watch
	if err := wait.PollUntilContextTimeout(ctx, 10*time.Millisecond, 10*time.Second, true, func(context.Context) (bool, error) {
		cluster.mu.Lock()
		defer cluster.mu.Unlock()
		return len(cluster.watchers["node-a"]) > 0 && len(cluster.watchers["node-b"]) > 0, nil
	}); err != nil {
		t.Fatalf("watches were not started: %v", err)
	}
	cluster.add(nodeScopedTestPod("a2", "node-a"))
	expect("watch event", "a1", "a2", "b1")

	// a pod on an unselected node stays out
	cluster.add(nodeScopedTestPod("c2", "node-c"))

	// node-b leaves the selection, node-c joins it: the pods are listed again
	nodesMu.Lock()
	nodes = []string{"node-a", "node-c"}
	nodesMu.Unlock()
	expect("node set change", "a1", "a2", "c1", "c2")
}
