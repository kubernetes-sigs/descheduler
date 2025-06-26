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

package defaultevictor

import (
	"context"
	"errors"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	nodeutil "sigs.k8s.io/descheduler/pkg/descheduler/node"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
	"sigs.k8s.io/descheduler/pkg/utils"
)

const (
	PluginName            = "DefaultEvictor"
	evictPodAnnotationKey = "descheduler.alpha.kubernetes.io/evict"
)

var _ frameworktypes.EvictorPlugin = &DefaultEvictor{}

type constraint func(pod *v1.Pod) error

// DefaultEvictor is the first EvictorPlugin, which defines the default extension points of the
// pre-baked evictor that is shipped.
// Even though we name this plugin DefaultEvictor, it does not actually evict anything,
// This plugin is only meant to customize other actions (extension points) of the evictor,
// like filtering, sorting, and other ones that might be relevant in the future
type DefaultEvictor struct {
	logger      klog.Logger
	args        *DefaultEvictorArgs
	constraints []constraint
	handle      frameworktypes.Handle
}

// IsPodEvictableBasedOnPriority checks if the given pod is evictable based on priority resolved from pod Spec.
func IsPodEvictableBasedOnPriority(pod *v1.Pod, priority int32) bool {
	return pod.Spec.Priority == nil || *pod.Spec.Priority < priority
}

// HaveEvictAnnotation checks if the pod have evict annotation
func HaveEvictAnnotation(pod *v1.Pod) bool {
	_, found := pod.ObjectMeta.Annotations[evictPodAnnotationKey]
	return found
}

// New builds plugin from its arguments while passing a handle
// nolint: gocyclo
func New(ctx context.Context, args runtime.Object, handle frameworktypes.Handle) (frameworktypes.Plugin, error) {
	defaultEvictorArgs, ok := args.(*DefaultEvictorArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type defaultEvictorFilterArgs, got %T", args)
	}
	logger := klog.FromContext(ctx).WithValues("plugin", PluginName)

	ev := &DefaultEvictor{
		logger: logger,
		handle: handle,
		args:   defaultEvictorArgs,
	}
	// add constraints
	err := ev.addAllConstraints(logger, handle)
	if err != nil {
		return nil, err
	}
	return ev, nil
}

func (d *DefaultEvictor) addAllConstraints(logger klog.Logger, handle frameworktypes.Handle) error {
	args := d.args
	// Step 1: Determine effective protected policies based on the provided arguments.
	effectiveProtectedPolicies := getEffectiveProtectedPolicies(args)

	if err := applyEffectiveProtectedPolicies(d, effectiveProtectedPolicies, handle); err != nil {
		return fmt.Errorf("failed to apply effective protected policies: %w", err)
	}

	if constraints, err := evictionConstraintsForLabelSelector(logger, args.LabelSelector); err != nil {
		return err
	} else {
		d.constraints = append(d.constraints, constraints...)
	}
	if constraints, err := evictionConstraintsForMinReplicas(logger, args.MinReplicas, handle); err != nil {
		return err
	} else {
		d.constraints = append(d.constraints, constraints...)
	}
	d.constraints = append(d.constraints, evictionConstraintsForMinPodAge(args.MinPodAge)...)
	return nil
}

// applyEffectiveProtectedPolicies applies all relevant protection policies to the plugin.
// For each policy:
// - If it's in the list, we disallow eviction (shouldEvict = false).
// - If it's NOT in the list, we allow eviction (shouldEvict = true).
func applyEffectiveProtectedPolicies(d *DefaultEvictor, policies []PodProtectionPolicy, handle frameworktypes.Handle) error {
	policyMap := make(map[PodProtectionPolicy]bool)
	for _, policy := range policies {
		policyMap[policy] = true
	}

	d.constraints = append(d.constraints, evictionConstraintsForFailedBarePods(d.logger, !policyMap[FailedBarePods])...)
	if constraints, err := evictionConstraintsForSystemCriticalPods(d.logger, !policyMap[SystemCriticalPods], d.args.PriorityThreshold, handle); err != nil {
		return err
	} else {
		d.constraints = append(d.constraints, constraints...)
	}
	d.constraints = append(d.constraints, evictionConstraintsForLocalStoragePods(!policyMap[PodsWithLocalStorage])...)
	d.constraints = append(d.constraints, evictionConstraintsForDaemonSetPods(!policyMap[DaemonSetPods])...)
	d.constraints = append(d.constraints, evictionConstraintsForPvcPods(policyMap[PodsWithPVC])...)
	d.constraints = append(d.constraints, evictionConstraintsForIgnorePodsWithoutPDB(policyMap[PodsWithoutPDB], handle)...)
	return nil
}

// contains checks if a slice of PodProtectionPolicy contains a given policy.
func contains(list []PodProtectionPolicy, item PodProtectionPolicy) bool {
	for _, v := range list {
		if v == item {
			return true
		}
	}
	return false
}

// getEffectiveProtectedPolicies determines which policies are currently active.
// It supports both new-style (PodProtectionPolicies) and legacy-style flags.
func getEffectiveProtectedPolicies(args *DefaultEvictorArgs) []PodProtectionPolicy {
	// determine whether to use PodProtectionPolicies config
	useNewConfig := len(args.PodProtectionPolicies.Disabled) > 0 || len(args.PodProtectionPolicies.ExtraEnabled) > 0

	if !useNewConfig {
		// fall back to the Deprecated config
		return legacyGetProtectedPolicies(args)
	}

	effective := make([]PodProtectionPolicy, 0)

	// Add default policies that are not explicitly disabled
	for _, policy := range args.defaultPodProtectionPolicies {
		if !contains(args.PodProtectionPolicies.Disabled, policy) {
			// Only add if not in the Disabled list
			effective = append(effective, policy)
		}
	}

	// Add extra enabled policies if not already included
	for _, policy := range args.PodProtectionPolicies.ExtraEnabled {
		if !contains(effective, policy) {
			// Add to effective if not already present
			effective = append(effective, policy)
		}
	}

	return effective
}

// legacyGetProtectedPolicies returns protected policies using old-style boolean flags.
func legacyGetProtectedPolicies(args *DefaultEvictorArgs) []PodProtectionPolicy {
	var policies []PodProtectionPolicy

	if !args.EvictLocalStoragePods {
		policies = append(policies, PodsWithLocalStorage)
	}
	if !args.EvictDaemonSetPods {
		policies = append(policies, DaemonSetPods)
	}
	if !args.EvictSystemCriticalPods {
		policies = append(policies, SystemCriticalPods)
	}
	if !args.EvictFailedBarePods {
		policies = append(policies, FailedBarePods)
	}
	if args.IgnorePvcPods {
		policies = append(policies, PodsWithPVC)
	}
	if args.IgnorePodsWithoutPDB {
		policies = append(policies, PodsWithoutPDB)
	}
	return policies
}

// Name retrieves the plugin name
func (d *DefaultEvictor) Name() string {
	return PluginName
}

func (d *DefaultEvictor) PreEvictionFilter(pod *v1.Pod) bool {
	logger := d.logger.WithValues("ExtensionPoint", frameworktypes.PreEvictionFilterExtensionPoint)
	if d.args.NodeFit {
		nodes, err := nodeutil.ReadyNodes(context.TODO(), d.handle.ClientSet(), d.handle.SharedInformerFactory().Core().V1().Nodes().Lister(), d.args.NodeSelector)
		if err != nil {
			logger.Error(err, "unable to list ready nodes", "pod", klog.KObj(pod))
			return false
		}
		if !nodeutil.PodFitsAnyOtherNode(d.handle.GetPodsAssignedToNodeFunc(), pod, nodes) {
			logger.Info("pod does not fit on any other node because of nodeSelector(s), Taint(s), or nodes marked as unschedulable", "pod", klog.KObj(pod))
			return false
		}
		return true
	}
	return true
}

func (d *DefaultEvictor) Filter(pod *v1.Pod) bool {
	logger := d.logger.WithValues("ExtensionPoint", frameworktypes.FilterExtensionPoint)
	checkErrs := []error{}

	if HaveEvictAnnotation(pod) {
		return true
	}

	if utils.IsMirrorPod(pod) {
		checkErrs = append(checkErrs, fmt.Errorf("pod is a mirror pod"))
	}

	if utils.IsStaticPod(pod) {
		checkErrs = append(checkErrs, fmt.Errorf("pod is a static pod"))
	}

	if utils.IsPodTerminating(pod) {
		checkErrs = append(checkErrs, fmt.Errorf("pod is terminating"))
	}

	for _, c := range d.constraints {
		if err := c(pod); err != nil {
			checkErrs = append(checkErrs, err)
		}
	}

	if len(checkErrs) > 0 {
		logger.V(4).Info("Pod fails the following checks", "pod", klog.KObj(pod), "checks", utilerrors.NewAggregate(checkErrs).Error())
		return false
	}

	return true
}

func getPodIndexerByOwnerRefs(indexName string, handle frameworktypes.Handle) (cache.Indexer, error) {
	podInformer := handle.SharedInformerFactory().Core().V1().Pods().Informer()
	indexer := podInformer.GetIndexer()

	// do not reinitialize the indexer, if it's been defined already
	for name := range indexer.GetIndexers() {
		if name == indexName {
			return indexer, nil
		}
	}

	if err := podInformer.AddIndexers(cache.Indexers{
		indexName: func(obj interface{}) ([]string, error) {
			pod, ok := obj.(*v1.Pod)
			if !ok {
				return []string{}, errors.New("unexpected object")
			}

			return podutil.OwnerRefUIDs(pod), nil
		},
	}); err != nil {
		return nil, err
	}

	return indexer, nil
}
