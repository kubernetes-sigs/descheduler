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

package componentconfig

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ReschedulerConfiguration struct {
	metav1.TypeMeta

	// KubeconfigFile is path to kubeconfig file with authorization and master
	// location information.
	KubeconfigFile string

	// PolicyConfigFile is the filepath to the rescheduler policy configuration.
	PolicyConfigFile string
}
