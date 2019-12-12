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

package fuzzer

import (
	"time"

	fuzz "github.com/google/gofuzz"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	kubeadmapiv1beta2 "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/v1beta2"
	"k8s.io/kubernetes/cmd/kubeadm/app/apis/output"
)

// Funcs returns the fuzzer functions for the kubeadm apis.
func Funcs(codecs runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		fuzzBootstrapToken,
	}
}

func fuzzBootstrapToken(obj *output.BootstrapToken, c fuzz.Continue) {
	c.FuzzNoCustom(obj)

	obj.Token = &kubeadmapiv1beta2.BootstrapTokenString{ID: "uvxdac", Secret: "fq35fuyue3kd4gda"}
	obj.Description = ""
	obj.TTL = &metav1.Duration{Duration: time.Hour * 24}
	obj.Usages = []string{"authentication", "signing"}
	obj.Groups = []string{"system:bootstrappers:kubeadm:default-node-token"}
}
