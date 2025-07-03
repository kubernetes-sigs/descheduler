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
	d.constraints = append(d.constraints, evictionConstraintsForFailedBarePods(logger, args.EvictFailedBarePods)...)
	if constraints, err := evictionConstraintsForSystemCriticalPods(logger, args.EvictSystemCriticalPods, args.PriorityThreshold, handle); err != nil {
		return err
	} else {
		d.constraints = append(d.constraints, constraints...)
	}
	d.constraints = append(d.constraints, evictionConstraintsForLocalStoragePods(args.EvictLocalStoragePods)...)
	d.constraints = append(d.constraints, evictionConstraintsForDaemonSetPods(args.EvictDaemonSetPods)...)
	d.constraints = append(d.constraints, evictionConstraintsForPvcPods(args.IgnorePvcPods)...)
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
	d.constraints = append(d.constraints, evictionConstraintsForIgnorePodsWithoutPDB(args.IgnorePodsWithoutPDB, handle)...)
	return nil
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
	}

	if d.args.NamespaceLabelSelector == nil {
		return true
	}

	// check pod by namespace label filter
	indexName := "namespaceWithLabelSelector"
	indexer, err := getNamespacesListByLabelSelector(indexName, d.args.NamespaceLabelSelector, d.handle)
	if err != nil {
		klog.ErrorS(err, "unable to list namespaces", "pod", klog.KObj(pod))
		return false
	}
	objs, err := indexer.ByIndex(indexName, pod.Namespace)
	if err != nil {
		klog.ErrorS(err, "unable to list namespaces for namespaceLabelSelector filter in the policy parameter", "pod", klog.KObj(pod))
		return false
	}
	if len(objs) == 0 {
		klog.InfoS("pod namespace do not match the namespaceLabelSelector filter in the policy parameter", "pod", klog.KObj(pod))
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

func getNamespacesListByLabelSelector(indexName string, labelSelector *metav1.LabelSelector, handle frameworktypes.Handle) (cache.Indexer, error) {
	nsInformer := handle.SharedInformerFactory().Core().V1().Namespaces().Informer()
	indexer := nsInformer.GetIndexer()

	// do not reinitialize the indexer, if it's been defined already
	for name := range indexer.GetIndexers() {
		if name == indexName {
			return indexer, nil
		}
	}

	if err := nsInformer.AddIndexers(cache.Indexers{
		indexName: func(obj interface{}) ([]string, error) {
			ns, ok := obj.(*v1.Namespace)
			if !ok {
				return []string{}, errors.New("unexpected object")
			}

			selector, err := metav1.LabelSelectorAsSelector(labelSelector)
			if err != nil {
				return []string{}, errors.New("could not get selector from label selector")
			}
			if labelSelector != nil && !selector.Empty() {
				if !selector.Matches(labels.Set(ns.Labels)) {
					return []string{}, nil
				}
			}
			return []string{ns.GetName()}, nil
		},
	}); err != nil {
		return nil, err
	}

	return indexer, nil
}
