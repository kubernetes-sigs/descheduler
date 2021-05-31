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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	"sigs.k8s.io/descheduler/pkg/api/v1alpha1"
	"unsafe"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/api/v1alpha2"
	"sigs.k8s.io/descheduler/pkg/descheduler/scheme"
)

func LoadPolicyConfig(policyConfigFile string) (*api.DeschedulerPolicy, error) {
	if policyConfigFile == "" {
		klog.V(1).InfoS("Policy config file not specified")
		return nil, nil
	}

	policy, err := ioutil.ReadFile(policyConfigFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read policy config file %q: %+v", policyConfigFile, err)
	}

	decoder := scheme.Codecs.UniversalDecoder(v1alpha2.SchemeGroupVersion, v1alpha1.SchemeGroupVersion)
	obj, err := runtime.Decode(decoder, policy)
	if err != nil {
		return nil, fmt.Errorf("failed decoding descheduler's policy config %q: %v", policyConfigFile, err)
	}
	versionedPolicy, err := decodeVersionedPolicy(obj.GetObjectKind(), decoder, policy)
	if err != nil {
		return nil, fmt.Errorf("failed decoding descheduler's policy config %q: %v", policyConfigFile, err)
	}

	internalPolicy := &api.DeschedulerPolicy{}
	if err := scheme.Scheme.Convert(versionedPolicy, internalPolicy, nil); err != nil {
		return nil, fmt.Errorf("failed converting versioned policy to internal policy version: %v", err)
	}

	return internalPolicy, nil
}

func decodeVersionedPolicy(kind schema.ObjectKind, decoder runtime.Decoder, policy []byte) (*v1alpha2.DeschedulerPolicy, error) {
	v2Policy := &v1alpha2.DeschedulerPolicy{}
	if kind.GroupVersionKind().Version == "v1alpha1" {
		v1Policy := &v1alpha1.DeschedulerPolicy{}
		if err := runtime.DecodeInto(decoder, policy, v1Policy); err != nil {
			return nil, err
		}
		v2Policy = convertV1ToV2Policy(v1Policy)
	} else {
		if err := runtime.DecodeInto(decoder, policy, v2Policy); err != nil {
			return nil, err
		}
	}
	return v2Policy, nil
}

func convertV1ToV2Policy(in *v1alpha1.DeschedulerPolicy) *v1alpha2.DeschedulerPolicy {
	t := true
	profiles := []v1alpha2.DeschedulerProfile{
		{
			Name:       "Default",
			Enabled:    &t,
			Strategies: *(*v1alpha2.StrategyList)(unsafe.Pointer(&in.Strategies)),
		},
	}
	return &v1alpha2.DeschedulerPolicy{
		TypeMeta:                  *&in.TypeMeta,
		Profiles:                  profiles,
		NodeSelector:              in.NodeSelector,
		EvictLocalStoragePods:     in.EvictLocalStoragePods,
		EvictSystemCriticalPods:   in.EvictSystemCriticalPods,
		IgnorePVCPods:             in.IgnorePVCPods,
		MaxNoOfPodsToEvictPerNode: in.MaxNoOfPodsToEvictPerNode,
	}
}
