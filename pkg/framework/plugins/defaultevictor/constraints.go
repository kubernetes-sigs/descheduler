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

package defaultevictor

import (
	"context"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/pkg/api"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
	"sigs.k8s.io/descheduler/pkg/utils"
)

func addDisabledDefaultPodProtectionsConstraints(ev *DefaultEvictor, handle frameworktypes.Handle, protections []DisabledDefaultPodProtection) error {
	if len(protections) == 0 {
		return nil
	}
	// helper function to check if a specific DisabledDefaultPodProtection is in the list.
	isDisabledDefaultPodProtection := func(action DisabledDefaultPodProtection) bool {
		for _, protection := range protections {
			if protection == action {
				return true
			}
		}
		return false
	}

	// handle WithLocalStorage
	if !isDisabledDefaultPodProtection(WithLocalStorage) {
		ev.constraints = append(ev.constraints, func(pod *v1.Pod) error {
			if utils.IsPodWithLocalStorage(pod) {
				return fmt.Errorf("pod has local storage and descheduler is not configured with evictLocalStoragePods")
			}
			return nil
		})
	} else {
		klog.V(1).InfoS("Warning: withLocalStorage is disabled. Pods with local storage will be evicted.")
	}

	// handle DaemonSetPods
	if !isDisabledDefaultPodProtection(DaemonSetPods) {
		ev.constraints = append(ev.constraints, func(pod *v1.Pod) error {
			ownerRefList := podutil.OwnerRef(pod)
			if utils.IsDaemonsetPod(ownerRefList) {
				return fmt.Errorf("pod is managed by a DaemonSet and descheduler is not configured with evictDaemonSetPods")
			}
			return nil
		})
	} else {
		klog.V(1).InfoS("Warning: daemonSetPods is disabled. DaemonSet pods will be evicted.")
	}

	// handle SystemCriticalPods
	if !isDisabledDefaultPodProtection(SystemCriticalPods) {
		ev.constraints = append(ev.constraints, func(pod *v1.Pod) error {
			if utils.IsCriticalPriorityPod(pod) {
				return fmt.Errorf("pod has system-critical priority and descheduler is not configured with systemCriticalPods")
			}
			return nil
		})

		if ev.args.PriorityThreshold != nil && (ev.args.PriorityThreshold.Value != nil || len(ev.args.PriorityThreshold.Name) > 0) {
			thresholdPriority, err := utils.GetPriorityValueFromPriorityThreshold(context.TODO(), handle.ClientSet(), ev.args.PriorityThreshold)
			if err != nil {
				return fmt.Errorf("failed to get priority class threshold: %v", err)
			}
			ev.constraints = append(ev.constraints, func(pod *v1.Pod) error {
				if IsPodEvictableBasedOnPriority(pod, thresholdPriority) {
					return nil
				}
				return fmt.Errorf("pod has higher priority than specified priority class threshold")
			})
		}
	} else {
		klog.V(1).InfoS("Warning: systemCriticalPods is set. This could cause eviction of Kubernetes system pods.")
	}

	// handle FailedBarePods
	if !isDisabledDefaultPodProtection(FailedBarePods) {
		ev.constraints = append(ev.constraints, func(pod *v1.Pod) error {
			ownerRefList := podutil.OwnerRef(pod)
			if len(ownerRefList) == 0 {
				return fmt.Errorf("pod does not have any ownerRefs")
			}
			return nil
		})
	} else {
		klog.V(1).InfoS("Warning: failedBarePods is set. This could cause eviction of pods without ownerReferences.")
		ev.constraints = append(ev.constraints, func(pod *v1.Pod) error {
			ownerRefList := podutil.OwnerRef(pod)
			// Enable evictFailedBarePods to evict bare pods in failed phase
			if len(ownerRefList) == 0 && pod.Status.Phase != v1.PodFailed {
				return fmt.Errorf("pod does not have any ownerRefs and is not in failed phase")
			}
			return nil
		})
	}
	return nil
}

func addExtraPodProtectionsConstraints(ev *DefaultEvictor, protections []ExtraPodProtection, handle frameworktypes.Handle) {
	for _, protection := range protections {
		switch protection {
		case WithPVC:
			ev.constraints = append(ev.constraints, func(pod *v1.Pod) error {
				if utils.IsPodWithPVC(pod) {
					return fmt.Errorf("pod has PVC and is protected by EvictionProtection: %s", WithPVC)
				}
				return nil
			})
		case WithoutPDB:
			ev.constraints = append(ev.constraints, func(pod *v1.Pod) error {
				hasPdb, err := utils.IsPodCoveredByPDB(pod, handle.SharedInformerFactory().Policy().V1().PodDisruptionBudgets().Lister())
				if err != nil {
					return fmt.Errorf("unable to check if pod is covered by PodDisruptionBudget: %w", err)
				}
				if !hasPdb {
					return fmt.Errorf("pod is not covered by a PodDisruptionBudget and is protected by EvictionProtection: %s", WithoutPDB)
				}
				return nil
			})
		default:
			klog.Warningf("Unknown ExtraPodProtection type: %s", protection)
		}
	}
}

func addEvictionConstraintsForFailedBarePods(ev *DefaultEvictor, evictFailedBarePods bool) {
	if evictFailedBarePods {
		klog.V(1).InfoS("Warning: EvictFailedBarePods is set to True. This could cause eviction of pods without ownerReferences.")
		ev.constraints = append(ev.constraints, func(pod *v1.Pod) error {
			ownerRefList := podutil.OwnerRef(pod)
			if len(ownerRefList) == 0 && pod.Status.Phase != v1.PodFailed {
				return fmt.Errorf("pod does not have any ownerRefs and is not in failed phase")
			}
			return nil
		})
	} else {
		ev.constraints = append(ev.constraints, func(pod *v1.Pod) error {
			if len(podutil.OwnerRef(pod)) == 0 {
				return fmt.Errorf("pod does not have any ownerRefs")
			}
			return nil
		})
	}
}

func addEvictionConstraintsForSystemCriticalPods(ev *DefaultEvictor, evictSystemCriticalPods bool, handle frameworktypes.Handle, priorityThreshold *api.PriorityThreshold) {
	if !evictSystemCriticalPods {
		ev.constraints = append(ev.constraints, func(pod *v1.Pod) error {
			if utils.IsCriticalPriorityPod(pod) {
				return fmt.Errorf("pod has system critical priority")
			}
			return nil
		})

		if priorityThreshold != nil && (priorityThreshold.Value != nil || len(priorityThreshold.Name) > 0) {
			thresholdPriority, err := utils.GetPriorityValueFromPriorityThreshold(context.TODO(), handle.ClientSet(), priorityThreshold)
			if err != nil {
				klog.Errorf("failed to get priority threshold: %v", err)
				return
			}
			ev.constraints = append(ev.constraints, func(pod *v1.Pod) error {
				if !IsPodEvictableBasedOnPriority(pod, thresholdPriority) {
					return fmt.Errorf("pod has higher priority than specified priority class threshold")
				}
				return nil
			})
		}
	} else {
		klog.V(1).InfoS("Warning: EvictSystemCriticalPods is set to True. This could cause eviction of Kubernetes system pods.")
	}
}

func addEvictionConstraintsForLocalStoragePods(ev *DefaultEvictor, evictLocalStoragePods bool) {
	if !evictLocalStoragePods {
		ev.constraints = append(ev.constraints, func(pod *v1.Pod) error {
			if utils.IsPodWithLocalStorage(pod) {
				return fmt.Errorf("pod has local storage and descheduler is not configured with evictLocalStoragePods")
			}
			return nil
		})
	}
}

func addEvictionConstraintsForDaemonSetPods(ev *DefaultEvictor, evictDaemonSetPods bool) {
	if !evictDaemonSetPods {
		ev.constraints = append(ev.constraints, func(pod *v1.Pod) error {
			if utils.IsDaemonsetPod(podutil.OwnerRef(pod)) {
				return fmt.Errorf("pod is related to daemonset and descheduler is not configured with evictDaemonSetPods")
			}
			return nil
		})
	}
}

func addEvictionConstraintsForPvcPods(ev *DefaultEvictor, ignorePvcPods bool) {
	if ignorePvcPods {
		ev.constraints = append(ev.constraints, func(pod *v1.Pod) error {
			if utils.IsPodWithPVC(pod) {
				return fmt.Errorf("pod has a PVC and descheduler is configured to ignore PVC pods")
			}
			return nil
		})
	}
}

func addEvictionConstraintsForPodsWithoutPDB(ev *DefaultEvictor, ignorePodsWithoutPDB bool, handle frameworktypes.Handle) {
	if ignorePodsWithoutPDB {
		ev.constraints = append(ev.constraints, func(pod *v1.Pod) error {
			hasPdb, err := utils.IsPodCoveredByPDB(pod, handle.SharedInformerFactory().Policy().V1().PodDisruptionBudgets().Lister())
			if err != nil {
				return fmt.Errorf("unable to check if pod is covered by PodDisruptionBudget: %w", err)
			}
			if !hasPdb {
				return fmt.Errorf("no PodDisruptionBudget found for pod")
			}
			return nil
		})
	}
}

func addLabelSelectorConstraint(ev *DefaultEvictor, labelSelector *metav1.LabelSelector) error {
	if labelSelector != nil {
		selector, err := metav1.LabelSelectorAsSelector(labelSelector)
		if err != nil {
			return fmt.Errorf("could not get selector from label selector: %v", err)
		}
		if !selector.Empty() {
			ev.constraints = append(ev.constraints, func(pod *v1.Pod) error {
				if !selector.Matches(labels.Set(pod.Labels)) {
					return fmt.Errorf("pod labels do not match the labelSelector filter in the policy parameter")
				}
				return nil
			})
		}
	}
	return nil
}

func addMinReplicasConstraint(ev *DefaultEvictor, minReplicas uint, handle frameworktypes.Handle) error {
	if minReplicas > 1 {
		indexName := "metadata.ownerReferences"
		indexer, err := getPodIndexerByOwnerRefs(indexName, handle)
		if err != nil {
			return err
		}
		ev.constraints = append(ev.constraints, func(pod *v1.Pod) error {
			if len(pod.OwnerReferences) == 0 {
				return nil
			}

			if len(pod.OwnerReferences) > 1 {
				klog.V(5).InfoS("pod has multiple owner references which is not supported for minReplicas check", "size", len(pod.OwnerReferences), "pod", klog.KObj(pod))
				return nil
			}

			ownerRef := pod.OwnerReferences[0]
			objs, err := indexer.ByIndex(indexName, string(ownerRef.UID))
			if err != nil {
				return fmt.Errorf("unable to list pods for minReplicas filter in the policy parameter")
			}

			if uint(len(objs)) < uint(minReplicas) {
				return fmt.Errorf("owner has %d replicas which is less than minReplicas of %d", len(objs), minReplicas)
			}

			return nil
		})
	}
	return nil
}

func addMinPodAgeConstraint(ev *DefaultEvictor, minPodAge *metav1.Duration) error {
	if minPodAge != nil {
		ev.constraints = append(ev.constraints, func(pod *v1.Pod) error {
			if pod.Status.StartTime == nil || time.Since(pod.Status.StartTime.Time) < minPodAge.Duration {
				return fmt.Errorf("pod age is not older than MinPodAge: %s seconds", minPodAge.String())
			}
			return nil
		})
	}
	return nil
}
