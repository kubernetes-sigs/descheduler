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
	"slices"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	evictionutils "sigs.k8s.io/descheduler/pkg/descheduler/evictions/utils"
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
	// Determine effective protected policies based on the provided arguments.
	effectivePodProtections := getEffectivePodProtections(args)

	if err := applyEffectivePodProtections(d, effectivePodProtections, handle); err != nil {
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

// applyEffectivePodProtections configures the evictor with specified Pod protection.
func applyEffectivePodProtections(d *DefaultEvictor, podProtections []PodProtection, handle frameworktypes.Handle) error {
	protectionMap := make(map[PodProtection]bool, len(podProtections))
	for _, protection := range podProtections {
		protectionMap[protection] = true
	}

	// Apply protections
	if err := applySystemCriticalPodsProtection(d, protectionMap, handle); err != nil {
		return err
	}
	applyFailedBarePodsProtection(d, protectionMap)
	applyLocalStoragePodsProtection(d, protectionMap)
	applyDaemonSetPodsProtection(d, protectionMap)
	applyPvcPodsProtection(d, protectionMap)
	applyPodsWithoutPDBProtection(d, protectionMap, handle)
	applyPodsWithResourceClaimsProtection(d, protectionMap)

	return nil
}

func applyFailedBarePodsProtection(d *DefaultEvictor, protectionMap map[PodProtection]bool) {
	isProtectionEnabled := protectionMap[FailedBarePods]
	if !isProtectionEnabled {
		d.logger.V(1).Info("Warning: EvictFailedBarePods is set to True. This could cause eviction of pods without ownerReferences.")
		d.constraints = append(d.constraints, func(pod *v1.Pod) error {
			ownerRefList := podutil.OwnerRef(pod)
			if len(ownerRefList) == 0 && pod.Status.Phase != v1.PodFailed {
				return fmt.Errorf("pod does not have any ownerRefs and is not in failed phase")
			}
			return nil
		})
	} else {
		d.constraints = append(d.constraints, func(pod *v1.Pod) error {
			if len(podutil.OwnerRef(pod)) == 0 {
				return fmt.Errorf("pod does not have any ownerRefs")
			}
			return nil
		})
	}
}

func applySystemCriticalPodsProtection(d *DefaultEvictor, protectionMap map[PodProtection]bool, handle frameworktypes.Handle) error {
	isProtectionEnabled := protectionMap[SystemCriticalPods]
	if !isProtectionEnabled {
		d.logger.V(1).Info("Warning: System critical pod protection is disabled. This could cause eviction of Kubernetes system pods.")
		return nil
	}

	d.constraints = append(d.constraints, func(pod *v1.Pod) error {
		if utils.IsCriticalPriorityPod(pod) {
			return fmt.Errorf("pod has system critical priority and is protected against eviction")
		}
		return nil
	})

	priorityThreshold := d.args.PriorityThreshold
	if priorityThreshold != nil && (priorityThreshold.Value != nil || len(priorityThreshold.Name) > 0) {
		thresholdPriority, err := utils.GetPriorityValueFromPriorityThreshold(context.TODO(), handle.ClientSet(), priorityThreshold)
		if err != nil {
			d.logger.Error(err, "failed to get priority threshold")
			return err
		}
		d.constraints = append(d.constraints, func(pod *v1.Pod) error {
			if !IsPodEvictableBasedOnPriority(pod, thresholdPriority) {
				return fmt.Errorf("pod has higher priority than specified priority class threshold")
			}
			return nil
		})
	}
	return nil
}

func applyLocalStoragePodsProtection(d *DefaultEvictor, protectionMap map[PodProtection]bool) {
	isProtectionEnabled := protectionMap[PodsWithLocalStorage]
	if isProtectionEnabled {
		d.constraints = append(d.constraints, func(pod *v1.Pod) error {
			if utils.IsPodWithLocalStorage(pod) {
				return fmt.Errorf("pod has local storage and is protected against eviction")
			}
			return nil
		})
	}
}

func applyDaemonSetPodsProtection(d *DefaultEvictor, protectionMap map[PodProtection]bool) {
	isProtectionEnabled := protectionMap[DaemonSetPods]
	if isProtectionEnabled {
		d.constraints = append(d.constraints, func(pod *v1.Pod) error {
			ownerRefList := podutil.OwnerRef(pod)
			if utils.IsDaemonsetPod(ownerRefList) {
				return fmt.Errorf("daemonset pods are protected against eviction")
			}
			return nil
		})
	}
}

func applyPvcPodsProtection(d *DefaultEvictor, protectionMap map[PodProtection]bool) {
	isProtectionEnabled := protectionMap[PodsWithPVC]
	if isProtectionEnabled {
		d.constraints = append(d.constraints, func(pod *v1.Pod) error {
			if utils.IsPodWithPVC(pod) {
				return fmt.Errorf("pod with PVC is protected against eviction")
			}
			return nil
		})
	}
}

func applyPodsWithoutPDBProtection(d *DefaultEvictor, protectionMap map[PodProtection]bool, handle frameworktypes.Handle) {
	isProtectionEnabled := protectionMap[PodsWithoutPDB]
	if isProtectionEnabled {
		d.constraints = append(d.constraints, func(pod *v1.Pod) error {
			hasPdb, err := utils.IsPodCoveredByPDB(pod, handle.SharedInformerFactory().Policy().V1().PodDisruptionBudgets().Lister())
			if err != nil {
				return fmt.Errorf("unable to check if pod is covered by PodDisruptionBudget: %w", err)
			}
			if !hasPdb {
				return fmt.Errorf("pod does not have a PodDisruptionBudget and is protected against eviction")
			}
			return nil
		})
	}
}

func applyPodsWithResourceClaimsProtection(d *DefaultEvictor, protectionMap map[PodProtection]bool) {
	isProtectionEnabled := protectionMap[PodsWithResourceClaims]
	if isProtectionEnabled {
		d.constraints = append(d.constraints, func(pod *v1.Pod) error {
			if utils.IsPodWithResourceClaims(pod) {
				return fmt.Errorf("pod has ResourceClaims and descheduler is configured to protect ResourceClaims pods")
			}
			return nil
		})
	}
}

// getEffectivePodProtections determines which policies are currently active.
// It supports both new-style (PodProtections) and legacy-style flags.
func getEffectivePodProtections(args *DefaultEvictorArgs) []PodProtection {
	// determine whether to use PodProtections config
	useNewConfig := len(args.PodProtections.DefaultDisabled) > 0 || len(args.PodProtections.ExtraEnabled) > 0

	if !useNewConfig {
		// fall back to the Deprecated config
		return legacyGetPodProtections(args)
	}

	// effective is the final list of active protection.
	effective := make([]PodProtection, 0)
	effective = append(effective, defaultPodProtections...)

	// Remove PodProtections that are in the DefaultDisabled list.
	effective = slices.DeleteFunc(effective, func(protection PodProtection) bool {
		return slices.Contains(args.PodProtections.DefaultDisabled, protection)
	})

	// Add extra enabled in PodProtections
	effective = append(effective, args.PodProtections.ExtraEnabled...)

	return effective
}

// legacyGetPodProtections returns protections using deprecated boolean flags.
func legacyGetPodProtections(args *DefaultEvictorArgs) []PodProtection {
	var protections []PodProtection

	// defaultDisabled
	if !args.EvictLocalStoragePods {
		protections = append(protections, PodsWithLocalStorage)
	}
	if !args.EvictDaemonSetPods {
		protections = append(protections, DaemonSetPods)
	}
	if !args.EvictSystemCriticalPods {
		protections = append(protections, SystemCriticalPods)
	}
	if !args.EvictFailedBarePods {
		protections = append(protections, FailedBarePods)
	}

	// extraEnabled
	if args.IgnorePvcPods {
		protections = append(protections, PodsWithPVC)
	}
	if args.IgnorePodsWithoutPDB {
		protections = append(protections, PodsWithoutPDB)
	}
	return protections
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

	if d.args.NoEvictionPolicy == MandatoryNoEvictionPolicy && evictionutils.HaveNoEvictionAnnotation(pod) {
		return false
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
