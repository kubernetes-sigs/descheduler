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
	// "context"
	"context"
	"errors"
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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
	args        runtime.Object
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
func New(args runtime.Object, handle frameworktypes.Handle) (frameworktypes.Plugin, error) {
	defaultEvictorArgs, ok := args.(*DefaultEvictorArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type defaultEvictorFilterArgs, got %T", args)
	}

	ev := &DefaultEvictor{}
	ev.handle = handle
	ev.args = defaultEvictorArgs

	if defaultEvictorArgs.EvictFailedBarePods {
		klog.V(1).InfoS("Warning: EvictFailedBarePods is set to True. This could cause eviction of pods without ownerReferences.")
		ev.constraints = append(ev.constraints, func(pod *v1.Pod) error {
			ownerRefList := podutil.OwnerRef(pod)
			// Enable evictFailedBarePods to evict bare pods in failed phase
			if len(ownerRefList) == 0 && pod.Status.Phase != v1.PodFailed {
				return fmt.Errorf("pod does not have any ownerRefs and is not in failed phase")
			}
			return nil
		})
	} else {
		ev.constraints = append(ev.constraints, func(pod *v1.Pod) error {
			ownerRefList := podutil.OwnerRef(pod)
			if len(ownerRefList) == 0 {
				return fmt.Errorf("pod does not have any ownerRefs")
			}
			return nil
		})
	}
	if !defaultEvictorArgs.EvictSystemCriticalPods {
		ev.constraints = append(ev.constraints, func(pod *v1.Pod) error {
			if utils.IsCriticalPriorityPod(pod) {
				return fmt.Errorf("pod has system critical priority")
			}
			return nil
		})

		if defaultEvictorArgs.PriorityThreshold != nil && (defaultEvictorArgs.PriorityThreshold.Value != nil || len(defaultEvictorArgs.PriorityThreshold.Name) > 0) {
			thresholdPriority, err := utils.GetPriorityValueFromPriorityThreshold(context.TODO(), handle.ClientSet(), defaultEvictorArgs.PriorityThreshold)
			if err != nil {
				return nil, fmt.Errorf("failed to get priority threshold: %v", err)
			}
			ev.constraints = append(ev.constraints, func(pod *v1.Pod) error {
				if IsPodEvictableBasedOnPriority(pod, thresholdPriority) {
					return nil
				}
				return fmt.Errorf("pod has higher priority than specified priority class threshold")
			})
		}
	} else {
		klog.V(1).InfoS("Warning: EvictSystemCriticalPods is set to True. This could cause eviction of Kubernetes system pods.")
	}
	if !defaultEvictorArgs.EvictLocalStoragePods {
		ev.constraints = append(ev.constraints, func(pod *v1.Pod) error {
			if utils.IsPodWithLocalStorage(pod) {
				return fmt.Errorf("pod has local storage and descheduler is not configured with evictLocalStoragePods")
			}
			return nil
		})
	}
	if !defaultEvictorArgs.EvictDaemonSetPods {
		ev.constraints = append(ev.constraints, func(pod *v1.Pod) error {
			ownerRefList := podutil.OwnerRef(pod)
			if utils.IsDaemonsetPod(ownerRefList) {
				return fmt.Errorf("pod is related to daemonset and descheduler is not configured with evictDaemonSetPods")
			}
			return nil
		})
	}
	if defaultEvictorArgs.IgnorePvcPods {
		ev.constraints = append(ev.constraints, func(pod *v1.Pod) error {
			if utils.IsPodWithPVC(pod) {
				return fmt.Errorf("pod has a PVC and descheduler is configured to ignore PVC pods")
			}
			return nil
		})
	}
	selector, err := metav1.LabelSelectorAsSelector(defaultEvictorArgs.LabelSelector)
	if err != nil {
		return nil, fmt.Errorf("could not get selector from label selector")
	}
	if defaultEvictorArgs.LabelSelector != nil && !selector.Empty() {
		ev.constraints = append(ev.constraints, func(pod *v1.Pod) error {
			if !selector.Matches(labels.Set(pod.Labels)) {
				return fmt.Errorf("pod labels do not match the labelSelector filter in the policy parameter")
			}
			return nil
		})
	}

	if defaultEvictorArgs.MinReplicas > 1 {
		indexName := "metadata.ownerReferences"
		indexer, err := getPodIndexerByOwnerRefs(indexName, handle)
		if err != nil {
			return nil, err
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

			if uint(len(objs)) < defaultEvictorArgs.MinReplicas {
				return fmt.Errorf("owner has %d replicas which is less than minReplicas of %d", len(objs), defaultEvictorArgs.MinReplicas)
			}

			return nil
		})
	}

	return ev, nil
}

// Name retrieves the plugin name
func (d *DefaultEvictor) Name() string {
	return PluginName
}

func (d *DefaultEvictor) PreEvictionFilter(pod *v1.Pod) bool {
	defaultEvictorArgs := d.args.(*DefaultEvictorArgs)
	if defaultEvictorArgs.NodeFit {
		nodes, err := nodeutil.ReadyNodes(context.TODO(), d.handle.ClientSet(), d.handle.SharedInformerFactory().Core().V1().Nodes().Lister(), defaultEvictorArgs.NodeSelector)
		if err != nil {
			klog.ErrorS(err, "unable to list ready nodes", "pod", klog.KObj(pod))
			return false
		}
		if !nodeutil.PodFitsAnyOtherNode(d.handle.GetPodsAssignedToNodeFunc(), pod, nodes) {
			klog.InfoS("pod does not fit on any other node because of nodeSelector(s), Taint(s), or nodes marked as unschedulable", "pod", klog.KObj(pod))
			return false
		}
		return true
	}
	return true
}

func (d *DefaultEvictor) Filter(pod *v1.Pod) bool {
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
		klog.V(4).InfoS("Pod fails the following checks", "pod", klog.KObj(pod), "checks", utilerrors.NewAggregate(checkErrs).Error())
		return false
	}

	return true
}

func getPodIndexerByOwnerRefs(indexName string, handle frameworktypes.Handle) (cache.Indexer, error) {
	podInformer := handle.SharedInformerFactory().Core().V1().Pods().Informer()
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

	indexer := podInformer.GetIndexer()
	return indexer, nil
}
