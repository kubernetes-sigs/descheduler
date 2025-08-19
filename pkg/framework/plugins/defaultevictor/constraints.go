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
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"

	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
)

func evictionConstraintsForLabelSelector(logger klog.Logger, labelSelector *metav1.LabelSelector) ([]constraint, error) {
	if labelSelector != nil {
		selector, err := metav1.LabelSelectorAsSelector(labelSelector)
		if err != nil {
			logger.Error(err, "could not get selector from label selector")
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

func evictionConstraintsForMinReplicas(logger klog.Logger, minReplicas uint, handle frameworktypes.Handle) ([]constraint, error) {
	if minReplicas > 1 {
		indexName := "metadata.ownerReferences"
		indexer, err := getPodIndexerByOwnerRefs(indexName, handle)
		if err != nil {
			logger.Error(err, "could not get pod indexer by ownerRefs")
			return nil, err
		}
		return []constraint{
			func(pod *v1.Pod) error {
				if len(pod.OwnerReferences) == 0 {
					return nil
				}
				if len(pod.OwnerReferences) > 1 {
					logger.V(5).Info("pod has multiple owner references which is not supported for minReplicas check", "size", len(pod.OwnerReferences), "pod", klog.KObj(pod))
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
