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

	// "sort"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/api/v1alpha1"
	"sigs.k8s.io/descheduler/pkg/api/v1alpha2"
	"sigs.k8s.io/descheduler/pkg/descheduler/scheme"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
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

	internalPolicy, err := decode(policy, policyConfigFile)
	if err != nil {
		return nil, fmt.Errorf("failed decoding %q: %+v", policyConfigFile, err)
	}

	return internalPolicy, nil
}

func decode(policy []byte, policyConfigFile string) (*api.DeschedulerPolicy, error) {
	// if we get v1alpha1, easier to work with it like this
	decoder := scheme.Codecs.UniversalDecoder(v1alpha1.SchemeGroupVersion, v1alpha2.SchemeGroupVersion, api.SchemeGroupVersion)
	decodedWithVersion, err := runtime.Decode(decoder, policy)
	if err != nil {
		return nil, fmt.Errorf("failed decoding decodedWithVersion with descheduler's policy config %q: %v", policyConfigFile, err)
	}

	// if we get v1alpha2, easier to use the default conersion. Will need to set apiVersion and kind
	obj, gvk, err := scheme.Codecs.UniversalDecoder().Decode(policy, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed decoding with universal decoder and descheduler's policy config %q: %v", policyConfigFile, err)
	}

	var internalPolicy *api.DeschedulerPolicy

	versionedPolicy, err := decodePolicy(decodedWithVersion.GetObjectKind(), decoder, policy, decodedWithVersion)
	if err != nil {
		return nil, fmt.Errorf("failed decoding policy config %q: %v", policyConfigFile, err)
	}

	// we only populate versionedPolicy if we got v1alpha1
	if versionedPolicy != nil {
		internalPolicy = &api.DeschedulerPolicy{}
		if err := scheme.Scheme.Convert(versionedPolicy, internalPolicy, nil); err != nil {
			return nil, fmt.Errorf("failed converting versioned policy to internal policy version: %v", err)
		}
	} else {
		cfgObj, err := loadConfig(obj, gvk)
		if err != nil {
			return nil, fmt.Errorf("failed decoding universal decoder obj for %q: %v", policyConfigFile, err)
		}
		internalPolicy = cfgObj
	}
	internalPolicy = setDefaults(*internalPolicy)

	return internalPolicy, nil
}

func loadConfig(obj runtime.Object, gvk *schema.GroupVersionKind) (*api.DeschedulerPolicy, error) {
	if cfgObj, ok := obj.(*api.DeschedulerPolicy); ok {
		cfgObj.TypeMeta.APIVersion = v1alpha2.SchemeGroupVersion.String()
		cfgObj.TypeMeta.Kind = "DeschedulerPolicy"
		cfgObj.Profiles = api.SortProfilesByName(cfgObj.Profiles)
		return cfgObj, nil
	}
	return nil, fmt.Errorf("couldn't decode as DeschedulerPolicy, got %s: ", gvk)
}

func decodePolicy(kind schema.ObjectKind, decoder runtime.Decoder, policy []byte, decodedWithVersion runtime.Object) (*v1alpha2.DeschedulerPolicy, error) {
	v2Policy := &v1alpha2.DeschedulerPolicy{}
	var err error
	if kind.GroupVersionKind().Version == "v1alpha1" || kind.GroupVersionKind().Version == runtime.APIVersionInternal {
		v1Policy := &v1alpha1.DeschedulerPolicy{}
		if err := runtime.DecodeInto(decoder, policy, v1Policy); err != nil {
			return nil, err
		}
		v2Policy, err = convertV1ToV2Policy(v1Policy)
		if err != nil {
			return nil, err
		}
		err = validateDeschedulerConfiguration(*v2Policy)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, nil
	}
	return v2Policy, nil
}

func convertV1ToV2Policy(in *v1alpha1.DeschedulerPolicy) (*v1alpha2.DeschedulerPolicy, error) {
	profiles, err := strategiesToProfiles(in.Strategies)
	if err != nil {
		return nil, err
	}

	profilesWithDefaultEvictor := policyToDefaultEvictor(in, *profiles)

	return &v1alpha2.DeschedulerPolicy{
		TypeMeta: v1.TypeMeta{
			Kind:       "DeschedulerPolicy",
			APIVersion: "descheduler/v1alpha2",
		},
		Profiles:                       *profilesWithDefaultEvictor,
		NodeSelector:                   in.NodeSelector,
		MaxNoOfPodsToEvictPerNode:      in.MaxNoOfPodsToEvictPerNode,
		MaxNoOfPodsToEvictPerNamespace: in.MaxNoOfPodsToEvictPerNamespace,
	}, nil
}

func policyToDefaultEvictor(in *v1alpha1.DeschedulerPolicy, profiles []v1alpha2.Profile) *[]v1alpha2.Profile {
	defaultEvictorArgs := &defaultevictor.DefaultEvictorArgs{}
	if in.NodeSelector != nil {
		defaultEvictorArgs.NodeSelector = *in.NodeSelector
	}
	if in.EvictLocalStoragePods != nil {
		defaultEvictorArgs.EvictLocalStoragePods = *in.EvictLocalStoragePods
	}
	if in.EvictSystemCriticalPods != nil {
		defaultEvictorArgs.EvictSystemCriticalPods = *in.EvictSystemCriticalPods
	}
	if in.IgnorePVCPods != nil {
		defaultEvictorArgs.IgnorePvcPods = *in.IgnorePVCPods
	}
	if in.EvictFailedBarePods != nil {
		defaultEvictorArgs.EvictFailedBarePods = *in.EvictFailedBarePods
	}
	for idx, profile := range profiles {
		profile.PluginConfig = append(profile.PluginConfig, configurePlugin(defaultEvictorArgs, defaultevictor.PluginName))
		profile.Plugins.Filter.Enabled = append(profile.Plugins.Filter.Enabled, defaultevictor.PluginName)
		profile.Plugins.PreEvictionFilter.Enabled = append(profile.Plugins.PreEvictionFilter.Enabled, defaultevictor.PluginName)
		profile.Plugins.Evict.Enabled = append(profile.Plugins.Evict.Enabled, defaultevictor.PluginName)
		profiles[idx] = profile
	}
	return &profiles
}

func setDefaults(in api.DeschedulerPolicy) *api.DeschedulerPolicy {
	for idx, profile := range in.Profiles {
		// Most defaults are being set, for example in pkg/framework/plugins/nodeutilization/defaults.go
		// If we need to set defaults coming from loadtime in each profile we do it here
		in.Profiles[idx] = setDefaultEvictor(profile)
	}
	return &in
}

func setDefaultEvictor(profile api.Profile) api.Profile {
	if len(profile.Plugins.Filter.Enabled) == 0 {
		profile.Plugins.Filter.Enabled = append(profile.Plugins.Filter.Enabled, defaultevictor.PluginName)
		newPluginConfig := api.PluginConfig{
			Name: defaultevictor.PluginName,
			Args: &defaultevictor.DefaultEvictorArgs{
				EvictLocalStoragePods:   false,
				EvictSystemCriticalPods: false,
				IgnorePvcPods:           false,
				EvictFailedBarePods:     false,
			},
		}
		if !hasPluginConfigsWithSameName(newPluginConfig, profile.PluginConfig) {
			profile.PluginConfig = append(profile.PluginConfig, newPluginConfig)

		}
	}
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
		if !hasPluginConfigsWithSameName(newPluginConfig, profile.PluginConfig) {
			profile.PluginConfig = append(profile.PluginConfig, newPluginConfig)

		}
	}
	if len(profile.Plugins.PreEvictionFilter.Enabled) == 0 {
		profile.Plugins.PreEvictionFilter.Enabled = append(profile.Plugins.PreEvictionFilter.Enabled, defaultevictor.PluginName)
		newPluginConfig := api.PluginConfig{
			Name: defaultevictor.PluginName,
			Args: &defaultevictor.DefaultEvictorArgs{
				EvictLocalStoragePods:   false,
				EvictSystemCriticalPods: false,
				IgnorePvcPods:           false,
				EvictFailedBarePods:     false,
			},
		}
		if !hasPluginConfigsWithSameName(newPluginConfig, profile.PluginConfig) {
			profile.PluginConfig = append(profile.PluginConfig, newPluginConfig)

		}
	}
	return profile
}

func validateDeschedulerConfiguration(in v1alpha2.DeschedulerPolicy) error {
	return nil
}

func strategiesToProfiles(strategies v1alpha1.StrategyList) (*[]v1alpha2.Profile, error) {
	var profiles []v1alpha2.Profile
	return &profiles, nil
}

func hasPlugin(newPluginName string, pluginSet []string) bool {
	for _, pluginName := range pluginSet {
		if newPluginName == pluginName {
			return true
		}
	}
	return false
}

func hasPluginConfigsWithSameName(newPluginConfig api.PluginConfig, pluginConfigs []api.PluginConfig) bool {
	for _, pluginConfig := range pluginConfigs {
		if newPluginConfig.Name == pluginConfig.Name {
			return true
		}
	}
	return false
}

func configurePlugin(args runtime.Object, name string) v1alpha2.PluginConfig {
	var pluginConfig v1alpha2.PluginConfig

	// runtime.Convert_runtime_Object_To_runtime_RawExtension(&args, pluginConfig.Args)
	pluginConfig.Args.Object = args
	// pluginConfig.Args.Raw = []byte(args)
	pluginConfig.Name = name
	return pluginConfig
}
