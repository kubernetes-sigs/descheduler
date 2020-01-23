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

import v1 "k8s.io/api/core/v1"

// This file contains the datastructures, types & functions needed by all the strategies so that we don't have
// to compute them again in each strategy.

// nodePodEvictedCount keeps count of pods evicted on node. This is used in conjunction with strategies to
type nodePodEvictedCount map[*v1.Node]int

// InitializeNodePodCount initializes the nodePodCount.
func InitializeNodePodCount(nodeList []*v1.Node) nodePodEvictedCount {
	var nodePodCount = make(nodePodEvictedCount)
	for _, node := range nodeList {
		// Initialize podsEvicted till now with 0.
		nodePodCount[node] = 0
	}
	return nodePodCount
}
