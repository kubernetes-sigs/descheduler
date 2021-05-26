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

package scheme

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/api/v1alpha1"
	"sigs.k8s.io/descheduler/pkg/api/v1alpha2"
	"sigs.k8s.io/descheduler/pkg/apis/componentconfig"
	componentconfigv1alpha1 "sigs.k8s.io/descheduler/pkg/apis/componentconfig/v1alpha1"
)

var (
	Scheme = runtime.NewScheme()
	Codecs = serializer.NewCodecFactory(Scheme)
)

func init() {
	utilruntime.Must(api.AddToScheme(Scheme))
	utilruntime.Must(v1alpha1.AddToScheme(Scheme))
	utilruntime.Must(v1alpha2.AddToScheme(Scheme))

	utilruntime.Must(componentconfig.AddToScheme(Scheme))
	utilruntime.Must(componentconfigv1alpha1.AddToScheme(Scheme))
}
