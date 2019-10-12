/*
Copyright 2019 The Kubernetes Authors.

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

package nodeinfo

// Snapshot is a snapshot of cache NodeInfo. The scheduler takes a
// snapshot at the beginning of each scheduling cycle and uses it for its
// operations in that cycle.
type Snapshot struct {
	NodeInfoMap map[string]*NodeInfo
	Generation  int64
}

// NewSnapshot initializes a Snapshot struct and returns it.
func NewSnapshot() *Snapshot {
	return &Snapshot{
		NodeInfoMap: make(map[string]*NodeInfo),
	}
}
