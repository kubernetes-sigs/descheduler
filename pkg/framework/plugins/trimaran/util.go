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

package trimaran

import v1 "k8s.io/api/core/v1"

const (
	MinNodeScore int64 = 0
	MaxNodeScore int64 = 0
)

type NodeInfo struct {
	Node     *v1.Node
	Usage    float64
	Capacity float64
	Score    float64
}
