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

package deploymentevictionlimit

import (
	"context"
	"fmt"
	"strings"
	"sync"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"

	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
)

const PluginName = "DeploymentEvictionLimit"

var (
	_ frameworktypes.EvictorPlugin    = &DeploymentEvictionLimit{}
	_ frameworktypes.DeschedulePlugin = &DeploymentEvictionLimit{}
)

type deploymentKey struct {
	namespace string
	name      string
}

func (k deploymentKey) String() string { return k.namespace + "/" + k.name }

// DeploymentEvictionLimit caps how many distinct pods of each configured
// Deployment may be cleared for eviction in a single descheduling cycle.
//
// It implements two kinds of extension points:
//
//   - Filter / PreEvictionFilter (EvictorPlugin): enforce the budget.
//     PreEvictionFilter clears a pod for eviction, consuming one unit of its
//     Deployment's budget the first time it sees that pod. Repeated calls for
//     the same pod are free, because several strategies call PreEvictionFilter
//     both while listing candidates and again right before evicting. Filter is
//     read-only: it passes pods that are already cleared and pods whose
//     Deployment still has budget.
//
//   - Deschedule (DeschedulePlugin): evicts nothing. It only resets the
//     per-cycle state. The framework runs every Deschedule plugin of a profile
//     before any of its Balance plugins, so listing this plugin FIRST under
//     deschedule.enabled resets the budget at the start of every cycle.
//
// The Deployment is resolved from the pod alone, with no API access: the
// Deployment controller names each ReplicaSet "<deployment>-<pod-template-hash>"
// and stamps the same hash on every pod as the pod-template-hash label, so the
// Deployment name is the ReplicaSet name with that suffix removed. This works in
// dry-run mode and needs no extra RBAC.
//
// The budget counts pods cleared for eviction, not confirmed evictions. If the
// API server rejects an eviction afterwards (for example a PodDisruptionBudget
// returns 429), or a strategy clears a pod while listing and then decides not
// to evict it, that unit of budget is still spent. The plugin therefore never
// lets more than the limit through, but may let fewer.
type DeploymentEvictionLimit struct {
	logger klog.Logger
	limits map[deploymentKey]uint

	mu sync.Mutex
	// approved holds, per Deployment, the UIDs of pods cleared for eviction in
	// the current cycle. Its size is the budget consumed so far.
	approved map[deploymentKey]map[types.UID]struct{}
	// resetSeen becomes true the first time Deschedule runs. It is used to warn
	// once when the plugin is not wired into the deschedule extension point.
	resetSeen bool
	warned    bool
}

// New builds the plugin from its arguments while passing a handle.
func New(ctx context.Context, args runtime.Object, _ frameworktypes.Handle) (frameworktypes.Plugin, error) {
	limitArgs, ok := args.(*DeploymentEvictionLimitArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type DeploymentEvictionLimitArgs, got %T", args)
	}

	limits := make(map[deploymentKey]uint, len(limitArgs.Deployments))
	for _, d := range limitArgs.Deployments {
		limits[deploymentKey{namespace: d.Namespace, name: d.Name}] = d.MaxPodsToEvict
	}

	return &DeploymentEvictionLimit{
		logger:   klog.FromContext(ctx).WithValues("plugin", PluginName),
		limits:   limits,
		approved: map[deploymentKey]map[types.UID]struct{}{},
	}, nil
}

// Name retrieves the plugin name.
func (d *DeploymentEvictionLimit) Name() string {
	return PluginName
}

// Deschedule evicts nothing. It marks the start of a new descheduling cycle and
// forgets every pod cleared in the previous one.
func (d *DeploymentEvictionLimit) Deschedule(ctx context.Context, nodes []*v1.Node) *frameworktypes.Status {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.approved = make(map[deploymentKey]map[types.UID]struct{}, len(d.limits))
	d.resetSeen = true
	return &frameworktypes.Status{}
}

// Filter reports whether the pod may be considered for eviction: it is either
// already cleared, or its Deployment still has budget. It consumes nothing.
func (d *DeploymentEvictionLimit) Filter(pod *v1.Pod) bool {
	key, limit, limited := d.limitFor(pod)
	if !limited {
		return true
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, cleared := d.approved[key][pod.UID]; cleared {
		return true
	}
	return uint(len(d.approved[key])) < limit
}

// PreEvictionFilter clears the pod for eviction, consuming one unit of its
// Deployment's budget the first time it sees the pod. It rejects the pod when
// the budget is exhausted.
func (d *DeploymentEvictionLimit) PreEvictionFilter(pod *v1.Pod) bool {
	key, limit, limited := d.limitFor(pod)
	if !limited {
		return true
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.resetSeen && !d.warned {
		d.warned = true
		d.logger.Info("Deschedule extension point has not run; per-cycle counters will never reset. List the plugin under deschedule.enabled to fix this.")
	}

	cleared := d.approved[key]
	if _, ok := cleared[pod.UID]; ok {
		return true
	}
	if uint(len(cleared)) >= limit {
		d.logger.V(1).Info("Deployment eviction limit reached, skipping pod", "deployment", key.String(), "limit", limit, "pod", klog.KObj(pod))
		return false
	}
	if cleared == nil {
		cleared = map[types.UID]struct{}{}
		d.approved[key] = cleared
	}
	cleared[pod.UID] = struct{}{}
	d.logger.V(4).Info("Cleared pod for eviction", "deployment", key.String(), "used", len(cleared), "limit", limit, "pod", klog.KObj(pod))
	return true
}

// limitFor resolves the pod's Deployment and looks up its budget. limited is
// false when the pod is not owned by a configured Deployment.
func (d *DeploymentEvictionLimit) limitFor(pod *v1.Pod) (key deploymentKey, limit uint, limited bool) {
	name, ok := deploymentNameOf(pod)
	if !ok {
		return deploymentKey{}, 0, false
	}
	key = deploymentKey{namespace: pod.Namespace, name: name}
	limit, limited = d.limits[key]
	return key, limit, limited
}

// deploymentNameOf returns the name of the Deployment that manages the pod, if
// any. It relies on the Deployment controller's naming contract: the owning
// ReplicaSet is called "<deployment>-<hash>" and the pod carries the same hash
// in its pod-template-hash label.
func deploymentNameOf(pod *v1.Pod) (string, bool) {
	ctrl := metav1.GetControllerOf(pod)
	if ctrl == nil || ctrl.Kind != "ReplicaSet" || !isAppsGroup(ctrl.APIVersion) {
		return "", false
	}
	hash := pod.Labels[appsv1.DefaultDeploymentUniqueLabelKey]
	if hash == "" {
		return "", false
	}
	suffix := "-" + hash
	if len(ctrl.Name) <= len(suffix) || !strings.HasSuffix(ctrl.Name, suffix) {
		return "", false
	}
	return strings.TrimSuffix(ctrl.Name, suffix), true
}

func isAppsGroup(apiVersion string) bool {
	gv, err := schema.ParseGroupVersion(apiVersion)
	return err == nil && gv.Group == appsv1.GroupName
}
