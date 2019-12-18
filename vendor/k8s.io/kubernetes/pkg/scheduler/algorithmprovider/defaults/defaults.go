/*
Copyright 2014 The Kubernetes Authors.

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

package defaults

import (
	"k8s.io/klog"

	"k8s.io/apimachinery/pkg/util/sets"
	utilfeature "k8s.io/apiserver/pkg/util/feature"

	"k8s.io/kubernetes/pkg/features"
	"k8s.io/kubernetes/pkg/scheduler"
	"k8s.io/kubernetes/pkg/scheduler/algorithm/predicates"
	"k8s.io/kubernetes/pkg/scheduler/algorithm/priorities"
)

const (
	// ClusterAutoscalerProvider defines the default autoscaler provider
	ClusterAutoscalerProvider = "ClusterAutoscalerProvider"
)

func init() {
	registerAlgorithmProvider(defaultPredicates(), defaultPriorities())
}

func defaultPredicates() sets.String {
	return sets.NewString(
		predicates.NoVolumeZoneConflictPred,
		predicates.MaxEBSVolumeCountPred,
		predicates.MaxGCEPDVolumeCountPred,
		predicates.MaxAzureDiskVolumeCountPred,
		predicates.MaxCSIVolumeCountPred,
		predicates.MatchInterPodAffinityPred,
		predicates.NoDiskConflictPred,
		predicates.GeneralPred,
		predicates.PodToleratesNodeTaintsPred,
		predicates.CheckVolumeBindingPred,
		predicates.CheckNodeUnschedulablePred,
	)
}

// ApplyFeatureGates applies algorithm by feature gates.
// The returned function is used to restore the state of registered predicates/priorities
// when this function is called, and should be called in tests which may modify the value
// of a feature gate temporarily.
func ApplyFeatureGates() (restore func()) {
	snapshot := scheduler.RegisteredPredicatesAndPrioritiesSnapshot()

	// Only register EvenPodsSpread predicate & priority if the feature is enabled
	if utilfeature.DefaultFeatureGate.Enabled(features.EvenPodsSpread) {
		klog.Infof("Registering EvenPodsSpread predicate and priority function")
		// register predicate
		scheduler.InsertPredicateKeyToAlgorithmProviderMap(predicates.EvenPodsSpreadPred)
		scheduler.RegisterFitPredicate(predicates.EvenPodsSpreadPred, predicates.EvenPodsSpreadPredicate)
		// register priority
		scheduler.InsertPriorityKeyToAlgorithmProviderMap(priorities.EvenPodsSpreadPriority)
		scheduler.RegisterPriorityMapReduceFunction(
			priorities.EvenPodsSpreadPriority,
			priorities.CalculateEvenPodsSpreadPriorityMap,
			priorities.CalculateEvenPodsSpreadPriorityReduce,
			1,
		)
	}

	// Prioritizes nodes that satisfy pod's resource limits
	if utilfeature.DefaultFeatureGate.Enabled(features.ResourceLimitsPriorityFunction) {
		klog.Infof("Registering resourcelimits priority function")
		scheduler.RegisterPriorityMapReduceFunction(priorities.ResourceLimitsPriority, priorities.ResourceLimitsPriorityMap, nil, 1)
		// Register the priority function to specific provider too.
		scheduler.InsertPriorityKeyToAlgorithmProviderMap(scheduler.RegisterPriorityMapReduceFunction(priorities.ResourceLimitsPriority, priorities.ResourceLimitsPriorityMap, nil, 1))
	}

	restore = func() {
		scheduler.ApplyPredicatesAndPriorities(snapshot)
	}
	return
}

func registerAlgorithmProvider(predSet, priSet sets.String) {
	// Registers algorithm providers. By default we use 'DefaultProvider', but user can specify one to be used
	// by specifying flag.
	scheduler.RegisterAlgorithmProvider(scheduler.DefaultProvider, predSet, priSet)
	// Cluster autoscaler friendly scheduling algorithm.
	scheduler.RegisterAlgorithmProvider(ClusterAutoscalerProvider, predSet,
		copyAndReplace(priSet, priorities.LeastRequestedPriority, priorities.MostRequestedPriority))
}

func defaultPriorities() sets.String {
	return sets.NewString(
		priorities.SelectorSpreadPriority,
		priorities.InterPodAffinityPriority,
		priorities.LeastRequestedPriority,
		priorities.BalancedResourceAllocation,
		priorities.NodePreferAvoidPodsPriority,
		priorities.NodeAffinityPriority,
		priorities.TaintTolerationPriority,
		priorities.ImageLocalityPriority,
	)
}

func copyAndReplace(set sets.String, replaceWhat, replaceWith string) sets.String {
	result := sets.NewString(set.List()...)
	if result.Has(replaceWhat) {
		result.Delete(replaceWhat)
		result.Insert(replaceWith)
	}
	return result
}
