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
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	evictionutils "sigs.k8s.io/descheduler/pkg/descheduler/evictions/utils"
	nodeutil "sigs.k8s.io/descheduler/pkg/descheduler/node"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
	"sigs.k8s.io/descheduler/pkg/utils"
)

const (
	PluginName                 = "DefaultEvictor"
	evictPodAnnotationKey      = "descheduler.alpha.kubernetes.io/evict"
	namespaceWithLabelSelector = "namespaceWithLabelSelector-"

	// pdbDisruptionsAllowedTimeout bounds how long a relaxed PDB is given to have
	// its status recomputed by the PDB controller before the drain moves on.
	pdbDisruptionsAllowedTimeout = 5 * time.Second
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

	// relaxedPDBs tracks the PDBs this process has relaxed, as namespace/name,
	// so they can be restored once their drain finishes and so PDBs still
	// carrying the relaxation annotation from an earlier, interrupted run can be
	// told apart from the ones in flight now.
	relaxedPDBsMu sync.Mutex
	relaxedPDBs   map[string]bool
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

	if ev.args.NamespaceLabelSelector != nil && (len(ev.args.NamespaceLabelSelector.MatchLabels) > 0 || len(ev.args.NamespaceLabelSelector.MatchExpressions) > 0) {
		selector, nslErr := metav1.LabelSelectorAsSelector(ev.args.NamespaceLabelSelector)
		if nslErr != nil {
			return nil, fmt.Errorf("unable to convert namespaceLabelSelector to label selector: %w", nslErr)
		}
		indexName := namespaceWithLabelSelector + ev.handle.PluginInstanceID()
		if nslErr := addNamespaceLabelSelectorIndexer(ev.handle.SharedInformerFactory().Core().V1().Namespaces().Informer(), indexName, selector); nslErr != nil {
			return nil, fmt.Errorf("failed to add namespace label selector indexer: %w", nslErr)
		}
	}

	if ev.pdbRelaxationEnabled() {
		// Request the informers the PDB relaxation path reads from while the
		// plugin is still being constructed. The shared factory only starts the
		// informers that exist when Start is called, so asking for a lister later
		// would hand back one backed by a cache that is never populated.
		factory := ev.handle.SharedInformerFactory()
		factory.Policy().V1().PodDisruptionBudgets().Informer()
		factory.Core().V1().Pods().Informer()
		factory.Apps().V1().Deployments().Informer()
		factory.Apps().V1().ReplicaSets().Informer()
		factory.Apps().V1().StatefulSets().Informer()
	}

	return ev, nil
}

func addNamespaceLabelSelectorIndexer(informer cache.SharedIndexInformer, indexName string, selector labels.Selector) error {
	indexer := informer.GetIndexer()
	for name := range indexer.GetIndexers() {
		if name == indexName {
			return nil
		}
	}
	return informer.AddIndexers(cache.Indexers{
		indexName: func(obj interface{}) ([]string, error) {
			ns, ok := obj.(*v1.Namespace)
			if !ok {
				return []string{}, errors.New("unexpected object")
			}
			if !selector.Empty() {
				if !selector.Matches(labels.Set(ns.Labels)) {
					return []string{}, nil
				}
			}
			return []string{ns.GetName()}, nil
		},
	})
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
	applyPVCPodsProtection(d, protectionMap)
	applyPodsWithoutPDBProtection(d, protectionMap, handle)
	applyPodsWithResourceClaimsProtection(d, protectionMap)

	return nil
}

// protectedPVCStorageClasses returns the list of storage classes that should
// be protected from eviction. If the list is empty or nil then all storage
// classes are protected (assuming PodsWithPVC protection is enabled).
func protectedPVCStorageClasses(d *DefaultEvictor) []ProtectedStorageClass {
	protcfg := d.args.PodProtections.Config
	if protcfg == nil {
		return nil
	}
	scconfig := protcfg.PodsWithPVC
	if scconfig == nil {
		return nil
	}
	return scconfig.ProtectedStorageClasses
}

// podStorageClasses returns a list of storage classes referred by a pod. We
// need this when assessing if a pod should be protected because it refers to a
// protected storage class.
func podStorageClasses(inf informers.SharedInformerFactory, pod *v1.Pod) ([]string, error) {
	lister := inf.Core().V1().PersistentVolumeClaims().Lister().PersistentVolumeClaims(
		pod.Namespace,
	)

	referred := map[string]bool{}
	for _, vol := range pod.Spec.Volumes {
		if vol.PersistentVolumeClaim == nil {
			continue
		}

		claim, err := lister.Get(vol.PersistentVolumeClaim.ClaimName)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to get persistent volume claim %q/%q: %w",
				pod.Namespace, vol.PersistentVolumeClaim.ClaimName, err,
			)
		}

		// this should never happen as once a pvc is created with a nil
		// storageClass it is automatically picked up by the default
		// storage class. By returning an error here we make the pod
		// protected from eviction.
		if claim.Spec.StorageClassName == nil || *claim.Spec.StorageClassName == "" {
			return nil, fmt.Errorf(
				"failed to resolve storage class for pod %q/%q",
				pod.Namespace, claim.Name,
			)
		}

		referred[*claim.Spec.StorageClassName] = true
	}

	return slices.Collect(maps.Keys(referred)), nil
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

// applyPVCPodsProtection protects pods that refer to a PVC from eviction. If
// the user has specified a list of storage classes to protect then only pods
// referring to PVCs of those storage classes are protected.
func applyPVCPodsProtection(d *DefaultEvictor, enabledProtections map[PodProtection]bool) {
	if !enabledProtections[PodsWithPVC] {
		return
	}

	// if the user isn't filtering by storage classes we protect all pods
	// referring to a PVC.
	protected := protectedPVCStorageClasses(d)
	if len(protected) == 0 {
		d.constraints = append(
			d.constraints,
			func(pod *v1.Pod) error {
				if utils.IsPodWithPVC(pod) {
					return fmt.Errorf("pod with PVC is protected against eviction")
				}
				return nil
			},
		)
		return
	}

	protectedsc := map[string]bool{}
	for _, class := range protected {
		protectedsc[class.Name] = true
	}

	d.constraints = append(
		d.constraints, func(pod *v1.Pod) error {
			classes, err := podStorageClasses(d.handle.SharedInformerFactory(), pod)
			if err != nil {
				return err
			}
			for _, class := range classes {
				if !protectedsc[class] {
					continue
				}
				return fmt.Errorf("pod using protected storage class %q", class)
			}
			return nil
		},
	)
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
		// Skip nodeFit check for pods in excluded namespaces
		if d.isNamespaceExcludedFromNodeFit(pod.Namespace) {
			logger.V(3).Info("pod is in excluded namespace, skipping nodeFit check", "pod", klog.KObj(pod), "namespace", pod.Namespace)
			return true
		}

		// Skip nodeFit check for pods matching the label selector (they are exempt from fit checking)
		if d.args.LabelSelector != nil {
			selector, err := metav1.LabelSelectorAsSelector(d.args.LabelSelector)
			if err == nil && selector.Matches(labels.Set(pod.Labels)) {
				logger.V(3).Info("pod matches nodeFit exemption label selector, skipping nodeFit check", "pod", klog.KObj(pod))
				return true
			}
		}

		nodes, err := nodeutil.ReadyNodes(context.TODO(), d.handle.ClientSet(), d.handle.SharedInformerFactory().Core().V1().Nodes().Lister(), d.args.NodeSelector)
		if err != nil {
			logger.Error(err, "unable to list ready nodes", "pod", klog.KObj(pod))
			return false
		}
		if !nodeutil.PodFitsAnyOtherNode(d.handle.GetPodsAssignedToNodeFunc(), pod, nodes) {
			logger.V(3).Info("pod does not fit on any other node because of nodeSelector(s), Taint(s), or nodes marked as unschedulable", "pod", klog.KObj(pod))
			return false
		}
	}

	if d.args.NamespaceLabelSelector == nil || (len(d.args.NamespaceLabelSelector.MatchLabels) == 0 && len(d.args.NamespaceLabelSelector.MatchExpressions) == 0) {
		return true
	}
	indexName := namespaceWithLabelSelector + d.handle.PluginInstanceID()
	objs, err := d.handle.SharedInformerFactory().Core().V1().Namespaces().Informer().GetIndexer().ByIndex(indexName, pod.Namespace)
	if err != nil {
		logger.Error(err, "unable to list namespaces for namespaceLabelSelector filter in the policy parameter", "pod", klog.KObj(pod))
		return false
	}
	if len(objs) == 0 {
		logger.Info("pod namespace do not match the namespaceLabelSelector filter in the policy parameter", "pod", klog.KObj(pod))
		return false
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

// PDB action types
const (
	PDBActionModify = "modify"
	PDBActionNone   = "none"
)

const (
	// pdbOriginalSpecAnnotationKey holds the PDB's disruption settings as they
	// were before the descheduler relaxed them, so they can be put back once the
	// drain is over. It doubles as the marker used to find PDBs left relaxed by a
	// descheduler process that died mid-drain.
	pdbOriginalSpecAnnotationKey = "descheduler.alpha.kubernetes.io/original-pdb-spec"
	// pdbRelaxedForNodeAnnotationKey records which node the PDB was relaxed for.
	pdbRelaxedForNodeAnnotationKey = "descheduler.alpha.kubernetes.io/relaxed-for-node"
)

// pdbDisruptionSettings captures the parts of a PDB spec that relaxation
// overwrites. Storing them lets the original budget be restored exactly.
type pdbDisruptionSettings struct {
	MinAvailable   *intstr.IntOrString `json:"minAvailable,omitempty"`
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`
}

// pdbRelaxationEnabled reports whether either PDB relaxation behaviour is on.
func (d *DefaultEvictor) pdbRelaxationEnabled() bool {
	return d.args.DeletePDBsForSingleReplicaDeployments || d.args.DeletePDBsForUnderreplicatedDeployments
}

// shouldHandlePDB determines what action to take with a PDB to allow evictions during node drain.
// It returns (action, reason) where action is one of: "modify" or "none".
//
// Relaxation is always a modification, never a deletion: a modified PDB records
// its original settings and can be restored when the drain ends, whereas a
// deleted one is gone for good unless some operator happens to own it.
func (d *DefaultEvictor) shouldHandlePDB(pdb *policyv1.PodDisruptionBudget, allPods, podsOnNode []*v1.Pod, logger klog.Logger) (string, string) {
	if len(allPods) == 0 {
		return PDBActionNone, "PDB has no pods"
	}

	// Scenario 1: every pod the PDB covers belongs to a single-replica workload.
	// Such a workload can never satisfy a disruption budget and still be evicted,
	// so its PDB blocks the drain indefinitely.
	if d.args.DeletePDBsForSingleReplicaDeployments {
		allAreSingleReplica := true
		for _, pod := range allPods {
			if !d.isSingleReplicaOwnedPod(pod, logger) {
				allAreSingleReplica = false
				break
			}
		}
		if allAreSingleReplica {
			return PDBActionModify, "single-replica-workload"
		}
	}

	// The remaining scenarios relax budgets for multi-replica workloads, which
	// trades real availability for drain progress. They are deliberately behind
	// their own flag so that enabling single-replica handling does not silently
	// opt a cluster into relaxing healthy multi-replica workloads as well.
	if !d.args.DeletePDBsForUnderreplicatedDeployments {
		return PDBActionNone, "no single-replica match and multi-replica relaxation is disabled"
	}

	// Scenario 2: every pod the PDB covers is on the node being drained, so there
	// is nowhere for the budget to draw availability from. Single-replica cases
	// are already handled above.
	if len(podsOnNode) == len(allPods) && len(podsOnNode) > 1 {
		return PDBActionModify, "all-pods-on-target-node: PDB has no pods on other nodes"
	}

	// Scenario 3: the owning workload is already running below its desired
	// replica count, so the budget has no slack and the drain cannot proceed.
	if desired, ok := d.desiredReplicasForPods(allPods, logger); ok && len(allPods) < desired {
		return PDBActionModify, fmt.Sprintf("underreplicated-workload: %d healthy pods of %d desired", len(allPods), desired)
	}

	// Scenario 4: the workload is already sitting at the minimum the budget
	// permits, so no further pod can be disrupted through the normal path.
	minRequired := calculateMinRequiredPods(pdb, len(allPods))
	if len(allPods) <= minRequired {
		return PDBActionModify, fmt.Sprintf("at-minimum-replicas: %d pods meet minimum requirement of %d, allowing eviction", len(allPods), minRequired)
	}

	// Scenario 5: the pods are concentrated on one or two nodes, so the budget is
	// not buying the distribution it appears to. This covers zone-pinned gateways
	// whose replicas all landed in the same place.
	nodeDistribution := make(map[string]int)
	for _, pod := range allPods {
		nodeDistribution[pod.Spec.NodeName]++
	}
	nodesWithPods := len(nodeDistribution)
	if nodesWithPods <= 2 && len(allPods) > 2 {
		maxPodsOnSingleNode := 0
		for _, count := range nodeDistribution {
			if count > maxPodsOnSingleNode {
				maxPodsOnSingleNode = count
			}
		}
		// If >=80% of pods are on a single node, the PDB is failing to distribute.
		if (maxPodsOnSingleNode * 100) >= (len(allPods) * 80) {
			return PDBActionModify, fmt.Sprintf("poorly-distributed-across-nodes: %d/%d pods on %d nodes", len(podsOnNode), len(allPods), nodesWithPods)
		}
	}

	return PDBActionNone, fmt.Sprintf("adequate-distribution: %d pods across %d nodes", len(allPods), nodesWithPods)
}

// isNamespaceExcludedFromNodeFit checks if a namespace is in the nodeFitExcludedNamespaces list
func (d *DefaultEvictor) isNamespaceExcludedFromNodeFit(namespace string) bool {
	if len(d.args.NodeFitExcludedNamespaces) == 0 {
		return false
	}
	for _, excludedNs := range d.args.NodeFitExcludedNamespaces {
		if excludedNs == namespace {
			return true
		}
	}
	return false
}

// calculateMinRequiredPods calculates the minimum number of pods that must remain available
// based on the PDB's minAvailable and maxUnavailable constraints.
//
// Both fields may be a percentage, so they are scaled against the pod count
// rather than read as plain integers. If a value cannot be interpreted the
// function fails closed by demanding every pod stay available, which leaves the
// budget alone rather than relaxing it on the strength of a bad parse.
func calculateMinRequiredPods(pdb *policyv1.PodDisruptionBudget, totalPods int) int {
	if pdb.Spec.MaxUnavailable != nil {
		maxUnavailable, err := intstr.GetScaledValueFromIntOrPercent(pdb.Spec.MaxUnavailable, totalPods, true)
		if err != nil {
			return totalPods
		}
		return totalPods - maxUnavailable
	}

	if pdb.Spec.MinAvailable != nil {
		minAvailable, err := intstr.GetScaledValueFromIntOrPercent(pdb.Spec.MinAvailable, totalPods, true)
		if err != nil {
			return totalPods
		}
		return minAvailable
	}

	// Default: at least 1 pod must remain (conservative default)
	return 1
}

// desiredReplicasForPods sums the configured replica count of every distinct
// workload owning the given pods. The second return value is false when any
// owner cannot be resolved, in which case the caller must not draw a conclusion
// about whether the workload is underreplicated.
func (d *DefaultEvictor) desiredReplicasForPods(pods []*v1.Pod, logger klog.Logger) (int, bool) {
	seen := make(map[string]bool)
	total := 0

	for _, pod := range pods {
		ownerRefs := podutil.OwnerRef(pod)
		if len(ownerRefs) == 0 {
			return 0, false
		}
		for _, ownerRef := range ownerRefs {
			key := pod.Namespace + "/" + ownerRef.Kind + "/" + ownerRef.Name
			if seen[key] {
				continue
			}
			replicas, ok := d.desiredReplicasForOwner(pod.Namespace, ownerRef, logger)
			if !ok {
				return 0, false
			}
			seen[key] = true
			total += replicas
		}
	}

	if total == 0 {
		return 0, false
	}
	return total, true
}

// desiredReplicasForOwner resolves the replica count configured on a single
// owning workload, following ReplicaSets up to their Deployment.
func (d *DefaultEvictor) desiredReplicasForOwner(namespace string, ownerRef metav1.OwnerReference, logger klog.Logger) (int, bool) {
	apps := d.handle.SharedInformerFactory().Apps().V1()

	switch ownerRef.Kind {
	case "Deployment":
		dep, err := apps.Deployments().Lister().Deployments(namespace).Get(ownerRef.Name)
		if err != nil {
			logger.V(2).Error(err, "unable to look up owning Deployment", "namespace", namespace, "deployment", ownerRef.Name)
			return 0, false
		}
		if dep.Spec.Replicas == nil {
			return 1, true
		}
		return int(*dep.Spec.Replicas), true

	case "StatefulSet":
		sts, err := apps.StatefulSets().Lister().StatefulSets(namespace).Get(ownerRef.Name)
		if err != nil {
			logger.V(2).Error(err, "unable to look up owning StatefulSet", "namespace", namespace, "statefulset", ownerRef.Name)
			return 0, false
		}
		if sts.Spec.Replicas == nil {
			return 1, true
		}
		return int(*sts.Spec.Replicas), true

	case "ReplicaSet":
		rs, err := apps.ReplicaSets().Lister().ReplicaSets(namespace).Get(ownerRef.Name)
		if err != nil {
			logger.V(2).Error(err, "unable to look up owning ReplicaSet", "namespace", namespace, "replicaset", ownerRef.Name)
			return 0, false
		}
		// A ReplicaSet owned by a Deployment is scaled by that Deployment, and
		// during a rollout the old ReplicaSet is on its way to zero. Ask the
		// Deployment instead so a mid-rollout workload is not mistaken for an
		// underreplicated one.
		for _, rsOwnerRef := range rs.GetOwnerReferences() {
			if rsOwnerRef.Kind == "Deployment" {
				return d.desiredReplicasForOwner(namespace, rsOwnerRef, logger)
			}
		}
		if rs.Spec.Replicas == nil {
			return 1, true
		}
		return int(*rs.Spec.Replicas), true
	}

	return 0, false
}

// RelaxPDBsForNode relaxes PodDisruptionBudgets that would otherwise block the
// drain of the given node, recording each budget's original settings so they can
// be restored by RestorePDBsForNode once the drain is over. Only PDBs with pods
// on the target node are considered.
func (d *DefaultEvictor) RelaxPDBsForNode(ctx context.Context, node *v1.Node, logger klog.Logger) {
	if !d.pdbRelaxationEnabled() {
		return
	}

	logger.V(1).Info("Starting PDB relaxation for node", "node", node.Name)

	pdbs, err := d.handle.SharedInformerFactory().Policy().V1().PodDisruptionBudgets().Lister().List(labels.Everything())
	if err != nil {
		logger.Error(err, "failed to list PodDisruptionBudgets")
		return
	}

	// A PDB still carrying the annotation that this process has no record of was
	// relaxed by a descheduler that died before restoring it. Put it back before
	// deciding anything else, so a crashed run cannot leave a workload
	// permanently unprotected.
	d.restoreOrphanedPDBs(ctx, pdbs, logger)

	if len(pdbs) == 0 {
		logger.V(2).Info("No PodDisruptionBudgets found")
		return
	}

	relaxedCount := 0
	for _, pdb := range pdbs {
		logger.V(3).Info("Checking PDB", "pdb", klog.KObj(pdb))

		pods, err := d.getPodsForPDB(pdb)
		if err != nil {
			logger.V(2).Error(err, "failed to get pods for PDB", "pdb", klog.KObj(pdb))
			continue
		}
		if len(pods) == 0 {
			logger.V(3).Info("PDB has no matching pods", "pdb", klog.KObj(pdb))
			continue
		}

		var podsOnNode []*v1.Pod
		for _, pod := range pods {
			if pod.Spec.NodeName == node.Name {
				podsOnNode = append(podsOnNode, pod)
			}
		}
		if len(podsOnNode) == 0 {
			logger.V(3).Info("PDB has no pods on target node", "pdb", klog.KObj(pdb), "node", node.Name)
			continue
		}

		action, reason := d.shouldHandlePDB(pdb, pods, podsOnNode, logger)
		if action != PDBActionModify {
			logger.V(2).Info("Skipping PDB action", "pdb", klog.KObj(pdb), "node", node.Name, "reason", reason, "podsOnNode", len(podsOnNode), "totalPods", len(pods))
			continue
		}

		logger.V(1).Info("Relaxing PDB to allow evictions", "pdb", klog.KObj(pdb), "node", node.Name, "reason", reason, "podCount", len(podsOnNode))
		if err := d.relaxPDB(ctx, pdb, node.Name, logger); err != nil {
			logger.Error(err, "failed to relax PDB", "pdb", klog.KObj(pdb))
			continue
		}
		relaxedCount++
	}

	if relaxedCount > 0 {
		logger.V(1).Info("Completed PDB relaxation", "node", node.Name, "relaxedCount", relaxedCount)
	}
}

// relaxPDB records a PDB's current disruption settings in an annotation and then
// widens it to allow every pod to be unavailable.
func (d *DefaultEvictor) relaxPDB(ctx context.Context, pdb *policyv1.PodDisruptionBudget, nodeName string, logger klog.Logger) error {
	client := d.handle.ClientSet().PolicyV1().PodDisruptionBudgets(pdb.Namespace)

	// Read through to the API server rather than the informer cache: the object
	// is about to be updated and a stale resourceVersion would just conflict.
	current, err := client.Get(ctx, pdb.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to read PDB before relaxing it: %w", err)
	}

	// Already relaxed, by this run or a previous one. Do not overwrite the stored
	// original with the relaxed values.
	if _, ok := current.Annotations[pdbOriginalSpecAnnotationKey]; ok {
		d.rememberRelaxedPDB(current.Namespace, current.Name)
		return nil
	}

	original, err := json.Marshal(pdbDisruptionSettings{
		MinAvailable:   current.Spec.MinAvailable,
		MaxUnavailable: current.Spec.MaxUnavailable,
	})
	if err != nil {
		return fmt.Errorf("failed to record original PDB settings: %w", err)
	}

	updated := current.DeepCopy()
	if updated.Annotations == nil {
		updated.Annotations = make(map[string]string, 2)
	}
	updated.Annotations[pdbOriginalSpecAnnotationKey] = string(original)
	updated.Annotations[pdbRelaxedForNodeAnnotationKey] = nodeName

	maxUnavailable := intstr.FromString("100%")
	updated.Spec.MaxUnavailable = &maxUnavailable
	updated.Spec.MinAvailable = nil

	if _, err := client.Update(ctx, updated, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("failed to relax PDB: %w", err)
	}

	d.rememberRelaxedPDB(updated.Namespace, updated.Name)
	logger.V(1).Info("Successfully relaxed PDB", "pdb", klog.KObj(updated), "maxUnavailable", "100%")

	// The eviction API consults status.disruptionsAllowed, which the PDB
	// controller recomputes asynchronously. Without this pause the evictions that
	// follow can still be rejected by the budget we just widened.
	d.waitForPDBDisruptionsAllowed(ctx, updated, logger)
	return nil
}

// RestorePDBsForNode puts back the disruption settings of every PDB this evictor
// relaxed for the given node. It is safe to call when nothing was relaxed.
func (d *DefaultEvictor) RestorePDBsForNode(ctx context.Context, node *v1.Node, logger klog.Logger) {
	if !d.pdbRelaxationEnabled() {
		return
	}

	d.relaxedPDBsMu.Lock()
	keys := slices.Collect(maps.Keys(d.relaxedPDBs))
	d.relaxedPDBsMu.Unlock()

	for _, key := range keys {
		namespace, name, ok := strings.Cut(key, "/")
		if !ok {
			continue
		}
		pdb, err := d.handle.ClientSet().PolicyV1().PodDisruptionBudgets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			logger.Error(err, "failed to read PDB for restoration", "namespace", namespace, "pdb", name)
			continue
		}
		if pdb.Annotations[pdbRelaxedForNodeAnnotationKey] != node.Name {
			continue
		}
		if err := d.restorePDB(ctx, pdb, logger); err != nil {
			logger.Error(err, "failed to restore PDB", "pdb", klog.KObj(pdb))
			continue
		}
		d.forgetRelaxedPDB(namespace, name)
	}
}

// restoreOrphanedPDBs restores PDBs that carry the relaxation annotation but are
// unknown to this process, which means a previous descheduler run was
// interrupted before it could restore them.
func (d *DefaultEvictor) restoreOrphanedPDBs(ctx context.Context, pdbs []*policyv1.PodDisruptionBudget, logger klog.Logger) {
	for _, pdb := range pdbs {
		if _, ok := pdb.Annotations[pdbOriginalSpecAnnotationKey]; !ok {
			continue
		}
		if d.isRelaxedByThisRun(pdb.Namespace, pdb.Name) {
			continue
		}
		logger.V(1).Info("Restoring PDB left relaxed by a previous descheduler run", "pdb", klog.KObj(pdb), "relaxedForNode", pdb.Annotations[pdbRelaxedForNodeAnnotationKey])
		if err := d.restorePDB(ctx, pdb, logger); err != nil {
			logger.Error(err, "failed to restore orphaned PDB", "pdb", klog.KObj(pdb))
		}
	}
}

// restorePDB writes a PDB's recorded disruption settings back and drops the
// bookkeeping annotations.
func (d *DefaultEvictor) restorePDB(ctx context.Context, pdb *policyv1.PodDisruptionBudget, logger klog.Logger) error {
	client := d.handle.ClientSet().PolicyV1().PodDisruptionBudgets(pdb.Namespace)

	current, err := client.Get(ctx, pdb.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to read PDB before restoring it: %w", err)
	}

	stored, ok := current.Annotations[pdbOriginalSpecAnnotationKey]
	if !ok {
		// Someone else already restored it.
		return nil
	}

	var original pdbDisruptionSettings
	if err := json.Unmarshal([]byte(stored), &original); err != nil {
		// Leave the annotation in place: it is the only remaining record of what
		// the budget used to be, and dropping it would strand the PDB wide open
		// with nothing to recover from.
		return fmt.Errorf("failed to parse recorded PDB settings %q: %w", stored, err)
	}

	updated := current.DeepCopy()
	updated.Spec.MinAvailable = original.MinAvailable
	updated.Spec.MaxUnavailable = original.MaxUnavailable
	delete(updated.Annotations, pdbOriginalSpecAnnotationKey)
	delete(updated.Annotations, pdbRelaxedForNodeAnnotationKey)

	if _, err := client.Update(ctx, updated, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("failed to restore PDB: %w", err)
	}

	logger.V(1).Info("Successfully restored PDB", "pdb", klog.KObj(updated))
	return nil
}

// waitForPDBDisruptionsAllowed waits briefly for the PDB controller to recompute
// status.disruptionsAllowed after a relaxation. Timing out is not fatal; the
// evictions that follow simply may not all get through on this pass.
func (d *DefaultEvictor) waitForPDBDisruptionsAllowed(ctx context.Context, pdb *policyv1.PodDisruptionBudget, logger klog.Logger) {
	client := d.handle.ClientSet().PolicyV1().PodDisruptionBudgets(pdb.Namespace)
	err := wait.PollUntilContextTimeout(ctx, 250*time.Millisecond, pdbDisruptionsAllowedTimeout, true, func(ctx context.Context) (bool, error) {
		current, err := client.Get(ctx, pdb.Name, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		return current.Status.DisruptionsAllowed > 0, nil
	})
	if err != nil {
		logger.V(2).Info("PDB did not report allowed disruptions after relaxation, continuing anyway", "pdb", klog.KObj(pdb))
	}
}

func relaxedPDBKey(namespace, name string) string {
	return namespace + "/" + name
}

func (d *DefaultEvictor) rememberRelaxedPDB(namespace, name string) {
	d.relaxedPDBsMu.Lock()
	defer d.relaxedPDBsMu.Unlock()
	if d.relaxedPDBs == nil {
		d.relaxedPDBs = make(map[string]bool)
	}
	d.relaxedPDBs[relaxedPDBKey(namespace, name)] = true
}

func (d *DefaultEvictor) forgetRelaxedPDB(namespace, name string) {
	d.relaxedPDBsMu.Lock()
	defer d.relaxedPDBsMu.Unlock()
	delete(d.relaxedPDBs, relaxedPDBKey(namespace, name))
}

func (d *DefaultEvictor) isRelaxedByThisRun(namespace, name string) bool {
	d.relaxedPDBsMu.Lock()
	defer d.relaxedPDBsMu.Unlock()
	return d.relaxedPDBs[relaxedPDBKey(namespace, name)]
}

// getPodsForPDB returns the pods a PDB covers, limited to those that actually
// count towards its budget: scheduled and not terminal. Terminal pods consume no
// budget and unscheduled pods belong to no node, so counting either one skews
// every distribution check in shouldHandlePDB.
func (d *DefaultEvictor) getPodsForPDB(pdb *policyv1.PodDisruptionBudget) ([]*v1.Pod, error) {
	// A PDB with no selector covers nothing.
	if pdb.Spec.Selector == nil {
		return nil, nil
	}

	selector, err := metav1.LabelSelectorAsSelector(pdb.Spec.Selector)
	if err != nil {
		return nil, fmt.Errorf("failed to parse PDB label selector: %w", err)
	}

	matched, err := d.handle.SharedInformerFactory().Core().V1().Pods().Lister().Pods(pdb.Namespace).List(selector)
	if err != nil {
		return nil, fmt.Errorf("failed to list pods for PDB: %w", err)
	}

	pods := make([]*v1.Pod, 0, len(matched))
	for _, pod := range matched {
		if pod.Spec.NodeName == "" {
			continue
		}
		if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
			continue
		}
		pods = append(pods, pod)
	}

	return pods, nil
}

// isSingleReplicaOwnedPod checks if a pod belongs to a workload configured with a
// single replica. An owner that cannot be resolved is reported as not
// single-replica, so an unreadable workload never triggers relaxation.
func (d *DefaultEvictor) isSingleReplicaOwnedPod(pod *v1.Pod, logger klog.Logger) bool {
	apps := d.handle.SharedInformerFactory().Apps().V1()

	for _, ownerRef := range podutil.OwnerRef(pod) {
		switch ownerRef.Kind {
		case "Deployment":
			dep, err := apps.Deployments().Lister().Deployments(pod.Namespace).Get(ownerRef.Name)
			if err != nil {
				logger.V(2).Error(err, "unable to look up owning Deployment", "pod", klog.KObj(pod), "deployment", ownerRef.Name)
				continue
			}
			if utils.OwnerHasSingleReplica([]metav1.OwnerReference{ownerRef}, dep) {
				return true
			}

		case "StatefulSet":
			sts, err := apps.StatefulSets().Lister().StatefulSets(pod.Namespace).Get(ownerRef.Name)
			if err != nil {
				logger.V(2).Error(err, "unable to look up owning StatefulSet", "pod", klog.KObj(pod), "statefulset", ownerRef.Name)
				continue
			}
			if utils.OwnerHasSingleReplica([]metav1.OwnerReference{ownerRef}, sts) {
				return true
			}

		case "ReplicaSet":
			// A ReplicaSet is scaled by its Deployment, so follow the chain rather
			// than trusting the ReplicaSet's own count mid-rollout.
			rs, err := apps.ReplicaSets().Lister().ReplicaSets(pod.Namespace).Get(ownerRef.Name)
			if err != nil {
				logger.V(2).Error(err, "unable to look up owning ReplicaSet", "pod", klog.KObj(pod), "replicaset", ownerRef.Name)
				continue
			}
			for _, rsOwnerRef := range rs.GetOwnerReferences() {
				if rsOwnerRef.Kind != "Deployment" {
					continue
				}
				dep, err := apps.Deployments().Lister().Deployments(pod.Namespace).Get(rsOwnerRef.Name)
				if err != nil {
					logger.V(2).Error(err, "unable to look up owning Deployment", "pod", klog.KObj(pod), "deployment", rsOwnerRef.Name)
					continue
				}
				if utils.OwnerHasSingleReplica([]metav1.OwnerReference{rsOwnerRef}, dep) {
					return true
				}
			}
		}
	}

	return false
}
