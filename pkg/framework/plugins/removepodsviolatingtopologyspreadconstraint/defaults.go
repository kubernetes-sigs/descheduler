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

package removepodsviolatingtopologyspreadconstraint

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilpointer "k8s.io/utils/pointer"
)

func addDefaultingFuncs(scheme *runtime.Scheme) error {
	return RegisterDefaults(scheme)
}

// SetDefaults_RemovePodsViolatingTopologySpreadConstraintArgs
// TODO: the final default values would be discussed in community
func SetDefaults_RemovePodsViolatingTopologySpreadConstraintArgs(obj runtime.Object) {
	args := obj.(*RemovePodsViolatingTopologySpreadConstraintArgs)
	if args.Namespaces == nil {
		args.Namespaces = nil
	}
	if args.LabelSelector == nil {
		args.LabelSelector = nil
	}
	if args.TopologyBalanceNodeFit == nil {
		args.TopologyBalanceNodeFit = utilpointer.Bool(true)
	}
	if len(args.Constraints) == 0 {
		args.Constraints = append(args.Constraints, v1.DoNotSchedule)
	}
}
