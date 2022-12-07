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
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/api/v1alpha1"
	"sigs.k8s.io/descheduler/pkg/api/v1alpha2"
	"sigs.k8s.io/descheduler/pkg/descheduler/scheme"
	"sigs.k8s.io/descheduler/pkg/framework/pluginregistry"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
)

func LoadPolicyConfig(policyConfigFile string, client clientset.Interface, registry pluginregistry.Registry) (*api.DeschedulerPolicy, error) {
	if policyConfigFile == "" {
		klog.V(1).InfoS("Policy config file not specified")
		return nil, nil
	}

	policy, err := ioutil.ReadFile(policyConfigFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read policy config file %q: %+v", policyConfigFile, err)
	}

	internalPolicy, err := decode(policyConfigFile, policy, client, registry)
	if err != nil {
		return nil, err
	}
	return internalPolicy, nil
}

func decode(policyConfigFile string, policy []byte, client clientset.Interface, registry pluginregistry.Registry) (*api.DeschedulerPolicy, error) {
	versionedPolicy := &v1alpha1.DeschedulerPolicy{}
	internalPolicy := &api.DeschedulerPolicy{}
	var err error

	decoder := scheme.Codecs.UniversalDecoder(v1alpha1.SchemeGroupVersion, v1alpha2.SchemeGroupVersion, api.SchemeGroupVersion)
	if err := runtime.DecodeInto(decoder, policy, versionedPolicy); err != nil {
		if err.Error() == "converting (v1alpha2.DeschedulerPolicy) to (v1alpha1.DeschedulerPolicy): unknown conversion" {
			klog.V(1).InfoS("Tried reading v1alpha2.DeschedulerPolicy and failed. Trying legacy conversion now.")
		} else {
			return nil, fmt.Errorf("failed decoding descheduler's policy config %q: %v", policyConfigFile, err)
		}
	}
	if versionedPolicy.APIVersion == "descheduler/v1alpha1" {
		// Build profiles
		internalPolicy, err = v1alpha1.V1alpha1ToInternal(client, versionedPolicy, registry)
		if err != nil {
			return nil, fmt.Errorf("failed converting versioned policy to internal policy version: %v", err)
		}
	} else {
		if err := runtime.DecodeInto(decoder, policy, internalPolicy); err != nil {
			return nil, fmt.Errorf("failed decoding descheduler's policy config %q: %v", policyConfigFile, err)
		}
	}

	err = validateDeschedulerConfiguration(*internalPolicy, registry)
	if err != nil {
		return nil, err
	}

	setDefaults(*internalPolicy, registry)

	return internalPolicy, nil
}

func setDefaults(in api.DeschedulerPolicy, registry pluginregistry.Registry) *api.DeschedulerPolicy {
	for idx, profile := range in.Profiles {
		// If we need to set defaults coming from loadtime in each profile we do it here
		in.Profiles[idx] = setDefaultEvictor(profile)
		for _, pluginConfig := range profile.PluginConfigs {
			setDefaultsPluginConfig(&pluginConfig, registry)
		}
	}
	return &in
}

func setDefaultsPluginConfig(pluginConfig *api.PluginConfig, registry pluginregistry.Registry) {
	if _, ok := registry[pluginConfig.Name]; ok {
		pluginUtilities := registry[pluginConfig.Name]
		pluginUtilities.PluginArgDefaulter(pluginConfig.Args)
	}
}

func setDefaultEvictor(profile api.Profile) api.Profile {
	if len(profile.Plugins.Evict.Enabled) == 0 {
		profile.Plugins.Evict.Enabled = append(profile.Plugins.Evict.Enabled, defaultevictor.PluginName)
		newPluginConfig := api.PluginConfig{
			Name: defaultevictor.PluginName,
			Args: &defaultevictor.DefaultEvictorArgs{
				EvictLocalStoragePods:   false,
				EvictSystemCriticalPods: false,
				IgnorePvcPods:           false,
				EvictFailedBarePods:     false,
			},
		}
		if !hasPluginConfigsWithSameName(newPluginConfig, profile.PluginConfigs) {
			profile.PluginConfigs = append(profile.PluginConfigs, newPluginConfig)
		}
	}
	return profile
}

func validateDeschedulerConfiguration(in api.DeschedulerPolicy, registry pluginregistry.Registry) error {
	var errorsInProfiles error
	for _, profile := range in.Profiles {
		// api.DeschedulerPolicy needs only 1 evictor plugin enabled
		if len(profile.Plugins.Evict.Enabled) != 1 {
			errTooManyEvictors := fmt.Errorf("profile with invalid number of evictor plugins enabled found. Please enable a single evictor plugin.")
			errorsInProfiles = setErrorsInProfiles(errTooManyEvictors, profile.Name, errorsInProfiles)
		}
		for _, pluginConfig := range profile.PluginConfigs {
			if _, ok := registry[pluginConfig.Name]; ok {
				pluginUtilities := registry[pluginConfig.Name]
				err := pluginUtilities.PluginArgValidator(pluginConfig.Args)
				errorsInProfiles = setErrorsInProfiles(err, profile.Name, errorsInProfiles)
			}
		}
	}
	if errorsInProfiles != nil {
		return errorsInProfiles
	}
	return nil
}

func setErrorsInProfiles(err error, profileName string, errorsInProfiles error) error {
	if err != nil {
		if errorsInProfiles == nil {
			errorsInProfiles = fmt.Errorf("in profile %s: %s", profileName, err.Error())
		} else {
			errorsInProfiles = fmt.Errorf("%w: %s", errorsInProfiles, fmt.Sprintf("in profile %s: %s", profileName, err.Error()))
		}
	}
	return errorsInProfiles
}

func hasPluginConfigsWithSameName(newPluginConfig api.PluginConfig, pluginConfigs []api.PluginConfig) bool {
	for _, pluginConfig := range pluginConfigs {
		if newPluginConfig.Name == pluginConfig.Name {
			return true
		}
	}
	return false
}
