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

package descheduler

import (
	"fmt"
	"io/ioutil"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/api/v1alpha1"
	"sigs.k8s.io/descheduler/pkg/descheduler/scheme"
)

func LoadPolicyConfig(policyConfigFile string) (*api.DeschedulerPolicy, error) {
	if policyConfigFile == "" {
		klog.V(1).Infof("policy config file not specified")
		return nil, nil
	}

	policy, err := ioutil.ReadFile(policyConfigFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read policy config file %q: %+v", policyConfigFile, err)
	}

	versionedPolicy := &v1alpha1.DeschedulerPolicy{}

	decoder := scheme.Codecs.UniversalDecoder(v1alpha1.SchemeGroupVersion)
	if err := runtime.DecodeInto(decoder, policy, versionedPolicy); err != nil {
		return nil, fmt.Errorf("failed decoding descheduler's policy config %q: %v", policyConfigFile, err)
	}

	internalPolicy := &api.DeschedulerPolicy{}
	if err := scheme.Scheme.Convert(versionedPolicy, internalPolicy, nil); err != nil {
		return nil, fmt.Errorf("failed converting versioned policy to internal policy version: %v", err)
	}

	return internalPolicy, nil
}
