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

package rescheduler

import (
	"fmt"
	"io/ioutil"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/aveshagarwal/rescheduler/pkg/api"
	_ "github.com/aveshagarwal/rescheduler/pkg/api/install"
	"github.com/aveshagarwal/rescheduler/pkg/api/v1alpha1"
	"github.com/aveshagarwal/rescheduler/pkg/rescheduler/scheme"
)

func LoadPolicyConfig(policyConfigFile string) (*api.ReschedulerPolicy, error) {
	if policyConfigFile == "" {
		fmt.Printf("policy config file not specified")
		return nil, nil
	}

	policy, err := ioutil.ReadFile(policyConfigFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read policy config file %q: %+v", policyConfigFile, err)
	}

	versionedPolicy := &v1alpha1.ReschedulerPolicy{}

	decoder := scheme.Codecs.UniversalDecoder(v1alpha1.SchemeGroupVersion)
	if err := runtime.DecodeInto(decoder, policy, versionedPolicy); err != nil {
		return nil, fmt.Errorf("failed decoding rescheduler's policy config %q: %v", policyConfigFile, err)
	}

	internalPolicy := &api.ReschedulerPolicy{}
	if err := scheme.Scheme.Convert(versionedPolicy, internalPolicy, nil); err != nil {
		return nil, fmt.Errorf("failed converting versioned policy to internal policy version: %v", err)
	}

	return internalPolicy, nil
}
