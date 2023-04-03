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
	"k8s.io/apimachinery/pkg/runtime"
)

func addDefaultingFuncs(scheme *runtime.Scheme) error {
	return RegisterDefaults(scheme)
}

// SetDefaults_DefaultEvictorArgs
// TODO: the final default values would be discussed in community
func SetDefaults_DefaultEvictorArgs(obj *DefaultEvictorArgs) {
	if obj.NodeSelector == "" {
		obj.NodeSelector = ""
	}
	if !obj.EvictLocalStoragePods {
		obj.EvictLocalStoragePods = false
	}
	if !obj.EvictSystemCriticalPods {
		obj.EvictSystemCriticalPods = false
	}
	if !obj.IgnorePvcPods {
		obj.IgnorePvcPods = false
	}
	if !obj.EvictFailedBarePods {
		obj.EvictFailedBarePods = false
	}
	if obj.LabelSelector == nil {
		obj.LabelSelector = nil
	}
	if obj.PriorityThreshold == nil {
		obj.PriorityThreshold = nil
	}
	if !obj.NodeFit {
		obj.NodeFit = false
	}
}
