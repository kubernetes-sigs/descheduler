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

package scaledowndeploymenthavingtoomanypodrestarts

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
)

func addDefaultingFuncs(scheme *runtime.Scheme) error {
	return RegisterDefaults(scheme)
}

func SetDefaults_ScaleDownDeploymentHavingTooManyPodRestartsArgs(obj runtime.Object) {
	args := obj.(*ScaleDownDeploymentHavingTooManyPodRestartsArgs)
	if args.Namespaces == nil {
		args.Namespaces = nil
	}
	if args.LabelSelector == nil {
		args.LabelSelector = nil
	}
	if args.ReplicasThreshold == nil {
		args.ReplicasThreshold = pointer.Int32(1)
	}
	if args.PodRestartThreshold == 0 {
		args.PodRestartThreshold = 0
	}
	if !args.IncludingInitContainers {
		args.IncludingInitContainers = false
	}
}
