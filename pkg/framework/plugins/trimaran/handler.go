/*
Copyright 2023 The Kubernetes Authors.

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

/*
Package Trimaran provides common code for plugins developed for real load aware scheduling like TargetLoadPacking etc.
*/

package trimaran

import (
	"fmt"
	"sort"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	clientcache "k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
)

const (
	// This is the maximum staleness of metrics possible by load watcher
	cacheCleanupIntervalMinutes = 5
	// Time interval in seconds for each metrics agent ingestion.
	metricsAgentReportingIntervalSeconds = 60
)

// PodAssignEventHandler watches assigned pods and caches them locally.
type PodAssignEventHandler interface {
	RangePodInfos(string, func(*PodInfo))
}

var (
	_ clientcache.ResourceEventHandler = &podAssignEventHandler{}
	_ PodAssignEventHandler            = &podAssignEventHandler{}
)

type podAssignEventHandler struct {
	// Maintains the node-name to PodInfo mapping for pods successfully bound to nodes
	scheduledPodsCache map[string][]*PodInfo
	sync.RWMutex
}

// Stores Timestamp and Pod spec info object
type PodInfo struct {
	// This timestamp is initialised when adding it to scheduledPodsCache after successful binding
	Timestamp time.Time
	Pod       *v1.Pod
}

// Returns a new instance of PodAssignEventHandler, after starting a background go routine for cache cleanup
func NewHandler(handle frameworktypes.Handle) PodAssignEventHandler {
	p := podAssignEventHandler{scheduledPodsCache: make(map[string][]*PodInfo)}
	go wait.Until(p.cleanupCache, time.Minute*cacheCleanupIntervalMinutes, wait.NeverStop)
	p.addToHandle(handle)
	return &p
}

// addToHandle : add event handler to framework handle
func (p *podAssignEventHandler) addToHandle(handle frameworktypes.Handle) {
	if handle == nil {
		return
	}
	handle.SharedInformerFactory().Core().V1().Pods().Informer().AddEventHandler(
		clientcache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				switch t := obj.(type) {
				case *v1.Pod:
					return isAssigned(t)
				case clientcache.DeletedFinalStateUnknown:
					if pod, ok := t.Obj.(*v1.Pod); ok {
						return isAssigned(pod)
					}
					utilruntime.HandleError(fmt.Errorf("unable to convert object %T to *v1.Pod", obj))
					return false
				default:
					utilruntime.HandleError(fmt.Errorf("unable to handle object: %T", obj))
					return false
				}
			},
			Handler: p,
		},
	)
}

func (p *podAssignEventHandler) OnAdd(obj interface{}, _ bool) {
	pod := obj.(*v1.Pod)
	p.Lock()
	defer p.Unlock()
	p.updateCache(pod)
}

func (p *podAssignEventHandler) OnUpdate(oldObj, newObj interface{}) {
	oldPod, newPod := oldObj.(*v1.Pod), newObj.(*v1.Pod)
	if oldPod.Spec.NodeName == newPod.Spec.NodeName {
		return
	}
	p.Lock()
	defer p.Unlock()
	p.updateCache(newPod)
}

func (p *podAssignEventHandler) OnDelete(obj interface{}) {
	pod := obj.(*v1.Pod)
	nodeName := pod.Spec.NodeName
	p.Lock()
	defer p.Unlock()
	cache, ok := p.scheduledPodsCache[nodeName]
	if !ok {
		return
	}
	for i, v := range cache {
		if pod.ObjectMeta.UID == v.Pod.ObjectMeta.UID {
			klog.V(10).InfoS("Deleting pod", "pod", klog.KObj(v.Pod))
			n := len(cache)
			copy(cache[i:], cache[i+1:])
			cache[n-1] = nil
			p.scheduledPodsCache[nodeName] = cache[:n-1]
			return
		}
	}
}

func (p *podAssignEventHandler) RangePodInfos(nodeName string, rangeFunc func(*PodInfo)) {
	p.RLock()
	defer p.RUnlock()
	cache, ok := p.scheduledPodsCache[nodeName]
	if !ok {
		return
	}
	for _, pi := range cache {
		rangeFunc(pi)
	}
}

func (p *podAssignEventHandler) updateCache(pod *v1.Pod) {
	p.scheduledPodsCache[pod.Spec.NodeName] = append(
		p.scheduledPodsCache[pod.Spec.NodeName],
		&PodInfo{Timestamp: time.Now(), Pod: pod})
}

// Deletes PodInfo entries that are older than metricsAgentReportingIntervalSeconds. Also deletes node entry if empty
func (p *podAssignEventHandler) cleanupCache() {
	p.Lock()
	defer p.Unlock()
	for nodeName := range p.scheduledPodsCache {
		cache := p.scheduledPodsCache[nodeName]
		expireTime := time.Now().Add(-metricsAgentReportingIntervalSeconds * time.Second)
		idx := sort.Search(len(cache), func(i int) bool {
			return cache[i].Timestamp.After(expireTime)
		})
		if idx == len(cache) {
			continue
		}
		n := copy(cache, cache[idx:])
		for j := n; j < len(cache); j++ {
			cache[j] = nil
		}
		cache = cache[:n]

		if len(cache) == 0 {
			delete(p.scheduledPodsCache, nodeName)
		} else {
			p.scheduledPodsCache[nodeName] = cache
		}
	}
}

// Checks and returns true if the pod is assigned to a node
func isAssigned(pod *v1.Pod) bool {
	return len(pod.Spec.NodeName) != 0
}
