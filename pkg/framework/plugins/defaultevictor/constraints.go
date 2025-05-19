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

func evictionConstraintsForFailedBarePods(evictFailedBarePods bool) []constraint {
	if evictFailedBarePods {
		klog.V(1).InfoS("Warning: EvictFailedBarePods is set to True. This could cause eviction of pods without ownerReferences.")
		return []constraint{
			func(pod *v1.Pod) error {
				ownerRefList := podutil.OwnerRef(pod)
				if len(ownerRefList) == 0 && pod.Status.Phase != v1.PodFailed {
					return fmt.Errorf("pod does not have any ownerRefs and is not in failed phase")
				}
				return nil
			},
		}
	}
	return []constraint{
		func(pod *v1.Pod) error {
			if len(podutil.OwnerRef(pod)) == 0 {
				return fmt.Errorf("pod does not have any ownerRefs")
			}
			return nil
		},
	}
}

func evictionConstraintsForSystemCriticalPods(evictSystemCriticalPods bool, priorityThreshold *api.PriorityThreshold, handle frameworktypes.Handle) ([]constraint, error) {
	var constraints []constraint

	if !evictSystemCriticalPods {
		constraints = append(constraints, func(pod *v1.Pod) error {
			if utils.IsCriticalPriorityPod(pod) {
				return fmt.Errorf("pod has system critical priority")
			}
			return nil
		})

		if priorityThreshold != nil && (priorityThreshold.Value != nil || len(priorityThreshold.Name) > 0) {
			thresholdPriority, err := utils.GetPriorityValueFromPriorityThreshold(context.TODO(), handle.ClientSet(), priorityThreshold)
			if err != nil {
				klog.Errorf("failed to get priority threshold: %v", err)
				return nil, err
			}
			constraints = append(constraints, func(pod *v1.Pod) error {
				if !IsPodEvictableBasedOnPriority(pod, thresholdPriority) {
					return fmt.Errorf("pod has higher priority than specified priority class threshold")
				}
				return nil
			})
		}
	} else {
		klog.V(1).InfoS("Warning: EvictSystemCriticalPods is set to True. This could cause eviction of Kubernetes system pods.")
	}

	return constraints, nil
}

func evictionConstraintsForLocalStoragePods(evictLocalStoragePods bool) []constraint {
	if !evictLocalStoragePods {
		return []constraint{
			func(pod *v1.Pod) error {
				if utils.IsPodWithLocalStorage(pod) {
					return fmt.Errorf("pod has local storage and descheduler is not configured with evictLocalStoragePods")
				}
				return nil
			},
		}
	}
	return nil
}

func evictionConstraintsForDaemonSetPods(evictDaemonSetPods bool) []constraint {
	if !evictDaemonSetPods {
		return []constraint{
			func(pod *v1.Pod) error {
				ownerRefList := podutil.OwnerRef(pod)
				if utils.IsDaemonsetPod(ownerRefList) {
					return fmt.Errorf("pod is related to daemonset and descheduler is not configured with evictDaemonSetPods")
				}
				return nil
			},
		}
	}
	return nil
}

func evictionConstraintsForPvcPods(ignorePvcPods bool) []constraint {
	if ignorePvcPods {
		return []constraint{
			func(pod *v1.Pod) error {
				if utils.IsPodWithPVC(pod) {
					return fmt.Errorf("pod has a PVC and descheduler is configured to ignore PVC pods")
				}
				return nil
			},
		}
	}
	return nil
}

func evictionConstraintsForLabelSelector(labelSelector *metav1.LabelSelector) ([]constraint, error) {
	if labelSelector != nil {
		selector, err := metav1.LabelSelectorAsSelector(labelSelector)
		if err != nil {
			klog.Error(err, "could not get selector from label selector")
			return nil, err
		}
		if !selector.Empty() {
			return []constraint{
				func(pod *v1.Pod) error {
					if !selector.Matches(labels.Set(pod.Labels)) {
						return fmt.Errorf("pod labels do not match the labelSelector filter in the policy parameter")
					}
					return nil
				},
			}, nil
		}
	}
	return nil, nil
}

func evictionConstraintsForMinReplicas(minReplicas uint, handle frameworktypes.Handle) ([]constraint, error) {
	if minReplicas > 1 {
		indexName := "metadata.ownerReferences"
		indexer, err := getPodIndexerByOwnerRefs(indexName, handle)
		if err != nil {
			klog.Error(err, "could not get pod indexer by ownerRefs")
			return nil, err
		}
		return []constraint{
			func(pod *v1.Pod) error {
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
				if uint(len(objs)) < minReplicas {
					return fmt.Errorf("owner has %d replicas which is less than minReplicas of %d", len(objs), minReplicas)
				}
				return nil
			},
		}, nil

	}
	return nil, nil
}

func evictionConstraintsForMinPodAge(minPodAge *metav1.Duration) []constraint {
	if minPodAge != nil {
		return []constraint{
			func(pod *v1.Pod) error {
				if pod.Status.StartTime == nil || time.Since(pod.Status.StartTime.Time) < minPodAge.Duration {
					return fmt.Errorf("pod age is not older than MinPodAge: %s seconds", minPodAge.String())
				}
				return nil
			},
		}
	}
	return nil
}

func evictionConstraintsForIgnorePodsWithoutPDB(ignorePodsWithoutPDB bool, handle frameworktypes.Handle) []constraint {
	if ignorePodsWithoutPDB {
		return []constraint{
			func(pod *v1.Pod) error {
				hasPdb, err := utils.IsPodCoveredByPDB(pod, handle.SharedInformerFactory().Policy().V1().PodDisruptionBudgets().Lister())
				if err != nil {
					return fmt.Errorf("unable to check if pod is covered by PodDisruptionBudget: %w", err)
				}
				if !hasPdb {
					return fmt.Errorf("no PodDisruptionBudget found for pod")
				}
				return nil
			},
		}
	}
	return nil
}
