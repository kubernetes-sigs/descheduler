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

package nodeutilization

import (
	"k8s.io/apimachinery/pkg/runtime"
)

func addDefaultingFuncs(scheme *runtime.Scheme) error {
	return RegisterDefaults(scheme)
}

// SetDefaults_LowNodeUtilizationArgs
// TODO: the final default values would be discussed in community
func SetDefaults_LowNodeUtilizationArgs(obj *LowNodeUtilizationArgs) {
	if obj.UseDeviationThresholds == false {
		obj.UseDeviationThresholds = false
	}
	if obj.Thresholds == nil {
		obj.Thresholds = nil
	}
	if obj.TargetThresholds == nil {
		obj.TargetThresholds = nil
	}
	if obj.NumberOfNodes == 0 {
		obj.NumberOfNodes = 0
	}
}

// SetDefaults_HighNodeUtilizationArgs
// TODO: the final default values would be discussed in community
func SetDefaults_HighNodeUtilizationArgs(obj *HighNodeUtilizationArgs) {
	if obj.Thresholds == nil {
		obj.Thresholds = nil
	}
	if obj.NumberOfNodes == 0 {
		obj.NumberOfNodes = 0
	}
}
