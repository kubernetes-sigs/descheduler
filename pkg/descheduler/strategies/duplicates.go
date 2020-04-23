/*
Copyright 2017 The Kubernetes Authors.

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

package strategies

import (
	"fmt"
	"strings"

	"k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/descheduler/cmd/descheduler/app/options"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/utils"
)

//type creator string
type DuplicatePodsMap map[string][]*v1.Pod

type creatorStatus struct {
	replicas  int32
	available int32
}

type creatorEvictedPods map[string]int

// RemoveDuplicatePods removes the duplicate pods on node. This strategy evicts all duplicate pods on node.
// A pod is said to be a duplicate of other if both of them are from same creator, kind and are within the same
// namespace. As of now, this strategy won't evict daemonsets, mirror pods, critical pods and pods with local storages.
func RemoveDuplicatePods(ds *options.DeschedulerServer, strategy api.DeschedulerStrategy, policyGroupVersion string, nodes []*v1.Node, nodepodCount utils.NodePodEvictedCount) {
	if !strategy.Enabled {
		return
	}
	deleteDuplicatePods(ds.Client, policyGroupVersion, nodes, ds.DryRun, nodepodCount, ds.MaxNoOfPodsToEvictPerNode, ds.EvictLocalStoragePods)
}

// deleteDuplicatePods evicts the pod from node and returns the count of evicted pods.
func deleteDuplicatePods(client clientset.Interface, policyGroupVersion string, nodes []*v1.Node, dryRun bool, nodepodCount utils.NodePodEvictedCount, maxPodsToEvict int, evictLocalStoragePods bool) int {
	podsEvicted := 0
	cepm := creatorEvictedPods{}
	for _, node := range nodes {
		klog.V(1).Infof("Processing node: %#v", node.Name)
		dpm := ListDuplicatePodsOnANode(client, node, evictLocalStoragePods)
		for creator, pods := range dpm {
			if _, found := cepm[creator]; !found {
				cepm[creator] = 0
			}
			status, err := getStatusForOwnerRef(client, creator)
			if err != nil {
				klog.Errorf("Error getting the number of replicas: %v", err)
				continue
			}
			// bestRatio := float64(status.replicas) / float64(len(nodes))
			bestRatio := (int(status.replicas) / len(nodes)) + 1
			// here we have to implement the logic to skip eviction
			// 1. too few available replicas
			klog.V(3).Infof("%#v with only %.2f%% of available pods, skipping?", creator, (float64(status.available)/float64(status.replicas))*100.0)
			if (float64(status.available) / float64(status.replicas)) < 0.6 {
				klog.V(1).Infof("%#v with only %.2f%% of available pods, skipping", creator, (float64(status.available)/float64(status.replicas))*100.0)
				continue
			}
			// 2. in case the replicas is greater than the number of nodes and we are enough close to the best balance
			klog.V(3).Infof("%#v on this node has close enough to the best ratio (%d) number of pods %d over replicas %d, skipping?", creator, bestRatio, len(pods), status.replicas)
			if len(pods) <= (bestRatio + 1) {
				klog.V(1).Infof("%#v on this node has close enough to the best ratio (%d) number of pods %d over replicas %d, skipping", creator, bestRatio, len(pods), status.replicas)
				continue
			}
			// 3. we already evicted some pods for this creator from other nodes
			klog.V(3).Infof("%#v with %d available replicas, we already evicted %d pods, skipping?", creator, status.available, cepm[creator])
			if (float64(cepm[creator]) / float64(status.available)) >= 0.1 {
				klog.V(1).Infof("%#v with %d available replicas, we already evicted %d pods, skipping", creator, status.available, cepm[creator])
				continue
			}

			if len(pods) > 1 {
				klog.V(1).Infof("%#v with replicas %d (%d)", creator, status.replicas, status.available)
				// i = 0 does not evict the first pod
				for i := 1; i < len(pods); i++ {
					if maxPodsToEvict > 0 && nodepodCount[node]+1 > maxPodsToEvict {
						break
					}
					success, err := evictions.EvictPod(client, pods[i], policyGroupVersion, dryRun)
					if !success {
						klog.Infof("Error when evicting pod: %#v (%#v)", pods[i].Name, err)
					} else {
						nodepodCount[node]++
						cepm[creator]++
						klog.V(1).Infof("Evicted pod: %#v (%#v)", pods[i].Name, err)
						break
					}
				}
			}
		}
		podsEvicted += nodepodCount[node]
	}
	return podsEvicted
}

// ListDuplicatePodsOnANode lists duplicate pods on a given node.
func ListDuplicatePodsOnANode(client clientset.Interface, node *v1.Node, evictLocalStoragePods bool) DuplicatePodsMap {
	pods, err := podutil.ListEvictablePodsOnNode(client, node, evictLocalStoragePods)
	if err != nil {
		return nil
	}
	return FindDuplicatePods(pods)
}

// FindDuplicatePods takes a list of pods and returns a duplicatePodsMap.
func FindDuplicatePods(pods []*v1.Pod) DuplicatePodsMap {
	dpm := DuplicatePodsMap{}
	// Ignoring the error here as in the ListDuplicatePodsOnNode function we call ListEvictablePodsOnNode which checks for error.
	for _, pod := range pods {
		ownerRefList := podutil.OwnerRef(pod)
		for _, ownerRef := range ownerRefList {
			// Namespace/Kind/Name should be unique for the cluster.
			s := strings.Join([]string{pod.ObjectMeta.Namespace, ownerRef.Kind, ownerRef.Name}, "/")
			dpm[s] = append(dpm[s], pod)
		}
	}
	return dpm
}

func getStatusForOwnerRef(client clientset.Interface, ownerRef string) (*creatorStatus, error) {
	parts := strings.Split(ownerRef, "/")
	ns := parts[0]
	kind := parts[1]
	name := parts[2]
	switch kind {
	case "ReplicaSet":
		if c, err := client.AppsV1().ReplicaSets(ns).Get(name, metav1.GetOptions{}); err == nil {
			return &creatorStatus{c.Status.Replicas, c.Status.AvailableReplicas}, nil
		} else {
			return nil, err
		}
	case "ReplicationController":
		if c, err := client.CoreV1().ReplicationControllers(ns).Get(name, metav1.GetOptions{}); err == nil {
			return &creatorStatus{c.Status.Replicas, c.Status.AvailableReplicas}, nil
		} else {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("pod owned by %s / %s - not managing", kind, name)
	}
}
