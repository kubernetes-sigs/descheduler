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
	"context"
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
	"sigs.k8s.io/descheduler/pkg/utils"
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

	return decode(policyConfigFile, policy, client, registry)
}

func decode(policyConfigFile string, policy []byte, client clientset.Interface, registry pluginregistry.Registry) (*api.DeschedulerPolicy, error) {
	internalPolicy := &api.DeschedulerPolicy{}
	var err error

	decoder := scheme.Codecs.UniversalDecoder(v1alpha1.SchemeGroupVersion, v1alpha2.SchemeGroupVersion, api.SchemeGroupVersion)
	if err := runtime.DecodeInto(decoder, policy, internalPolicy); err != nil {
		return nil, fmt.Errorf("failed decoding descheduler's policy config %q: %v", policyConfigFile, err)
	}

	err = validateDeschedulerConfiguration(*internalPolicy, registry)
	if err != nil {
		return nil, err
	}

	setDefaults(*internalPolicy, registry, client)

	return internalPolicy, nil
}

func setDefaults(in api.DeschedulerPolicy, registry pluginregistry.Registry, client clientset.Interface) *api.DeschedulerPolicy {
	for idx, profile := range in.Profiles {
		// If we need to set defaults coming from loadtime in each profile we do it here
		in.Profiles[idx] = setDefaultEvictor(profile, client)
		for _, pluginConfig := range profile.PluginConfigs {
			setDefaultsPluginConfig(&pluginConfig, registry)
		}
	}
	return &in
}

func setDefaultsPluginConfig(pluginConfig *api.PluginConfig, registry pluginregistry.Registry) {
	if _, ok := registry[pluginConfig.Name]; ok {
		pluginUtilities := registry[pluginConfig.Name]
		if pluginUtilities.PluginArgDefaulter != nil {
			pluginUtilities.PluginArgDefaulter(pluginConfig.Args)
		}
	}
}

func setDefaultEvictor(profile api.DeschedulerProfile, client clientset.Interface) api.DeschedulerProfile {
	newPluginConfig := api.PluginConfig{
		Name: defaultevictor.PluginName,
		Args: &defaultevictor.DefaultEvictorArgs{
			EvictLocalStoragePods:   false,
			EvictSystemCriticalPods: false,
			IgnorePvcPods:           false,
			EvictFailedBarePods:     false,
		},
	}
	if len(profile.Plugins.Evict.Enabled) == 0 && !hasPluginConfigsWithSameName(newPluginConfig, profile.PluginConfigs) {
		profile.Plugins.Evict.Enabled = append(profile.Plugins.Evict.Enabled, defaultevictor.PluginName)
		profile.PluginConfigs = append(profile.PluginConfigs, newPluginConfig)
	}
	var thresholdPriority int32
	var err error
	defaultevictorPluginConfig, idx := GetPluginConfig(defaultevictor.PluginName, profile.PluginConfigs)
	if defaultevictorPluginConfig != nil {
		thresholdPriority, err = utils.GetPriorityFromStrategyParams(context.TODO(), client, defaultevictorPluginConfig.Args.(*defaultevictor.DefaultEvictorArgs).PriorityThreshold)
	}
	if err != nil {
		klog.Error(err, "Failed to get threshold priority from args")
	}
	profile.PluginConfigs[idx].Args.(*defaultevictor.DefaultEvictorArgs).PriorityThreshold = &api.PriorityThreshold{}
	profile.PluginConfigs[idx].Args.(*defaultevictor.DefaultEvictorArgs).PriorityThreshold.Value = &thresholdPriority
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
				if pluginUtilities.PluginArgValidator != nil {
					err := pluginUtilities.PluginArgValidator(pluginConfig.Args)
					errorsInProfiles = setErrorsInProfiles(err, profile.Name, errorsInProfiles)
				}
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
