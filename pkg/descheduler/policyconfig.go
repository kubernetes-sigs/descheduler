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
	"sigs.k8s.io/descheduler/pkg/framework/plugins/nodeutilization"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/podlifetime"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removeduplicates"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removefailedpods"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodshavingtoomanyrestarts"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodsviolatinginterpodantiaffinity"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodsviolatingnodeaffinity"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodsviolatingnodetaints"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodsviolatingtopologyspreadconstraint"
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
	var errorsInProfiles error
	for _, profile := range in.Profiles {
		// v1alpha2.DeschedulerPolicy needs only 1 evictor plugin enabled
		if len(profile.Plugins.Evict.Enabled) != 1 {
			errTooManyEvictors := fmt.Errorf("profile with invalid number of evictor plugins enabled found. Please enable a single evictor plugin.")
			errorsInProfiles = setErrorsInProfiles(errTooManyEvictors, profile.Name, errorsInProfiles)
		}
		for _, pluginConfig := range profile.PluginConfig {
			switch pluginConfig.Name {
			case removeduplicates.PluginName:
				err := removeduplicates.ValidateRemoveDuplicatesArgs(pluginConfig.Args.Object.(*removeduplicates.RemoveDuplicatesArgs))
				errorsInProfiles = setErrorsInProfiles(err, profile.Name, errorsInProfiles)
			case nodeutilization.LowNodeUtilizationPluginName:
				err := nodeutilization.ValidateLowNodeUtilizationArgs(pluginConfig.Args.Object.(*nodeutilization.LowNodeUtilizationArgs))
				errorsInProfiles = setErrorsInProfiles(err, profile.Name, errorsInProfiles)
			case nodeutilization.HighNodeUtilizationPluginName:
				err := nodeutilization.ValidateHighNodeUtilizationArgs(pluginConfig.Args.Object.(*nodeutilization.HighNodeUtilizationArgs))
				errorsInProfiles = setErrorsInProfiles(err, profile.Name, errorsInProfiles)
			case removepodsviolatinginterpodantiaffinity.PluginName:
				err := removepodsviolatinginterpodantiaffinity.ValidateRemovePodsViolatingInterPodAntiAffinityArgs(pluginConfig.Args.Object.(*removepodsviolatinginterpodantiaffinity.RemovePodsViolatingInterPodAntiAffinityArgs))
				errorsInProfiles = setErrorsInProfiles(err, profile.Name, errorsInProfiles)
			case removepodsviolatingnodeaffinity.PluginName:
				err := removepodsviolatingnodeaffinity.ValidateRemovePodsViolatingNodeAffinityArgs(pluginConfig.Args.Object.(*removepodsviolatingnodeaffinity.RemovePodsViolatingNodeAffinityArgs))
				errorsInProfiles = setErrorsInProfiles(err, profile.Name, errorsInProfiles)
			case removepodsviolatingnodetaints.PluginName:
				err := removepodsviolatingnodetaints.ValidateRemovePodsViolatingNodeTaintsArgs(pluginConfig.Args.Object.(*removepodsviolatingnodetaints.RemovePodsViolatingNodeTaintsArgs))
				errorsInProfiles = setErrorsInProfiles(err, profile.Name, errorsInProfiles)
			case removepodsviolatingtopologyspreadconstraint.PluginName:
				err := removepodsviolatingtopologyspreadconstraint.ValidateRemovePodsViolatingTopologySpreadConstraintArgs(pluginConfig.Args.Object.(*removepodsviolatingtopologyspreadconstraint.RemovePodsViolatingTopologySpreadConstraintArgs))
				errorsInProfiles = setErrorsInProfiles(err, profile.Name, errorsInProfiles)
			case removepodshavingtoomanyrestarts.PluginName:
				err := removepodshavingtoomanyrestarts.ValidateRemovePodsHavingTooManyRestartsArgs(pluginConfig.Args.Object.(*removepodshavingtoomanyrestarts.RemovePodsHavingTooManyRestartsArgs))
				errorsInProfiles = setErrorsInProfiles(err, profile.Name, errorsInProfiles)
			case podlifetime.PluginName:
				err := podlifetime.ValidatePodLifeTimeArgs(pluginConfig.Args.Object.(*podlifetime.PodLifeTimeArgs))
				errorsInProfiles = setErrorsInProfiles(err, profile.Name, errorsInProfiles)
			case removefailedpods.PluginName:
				err := removefailedpods.ValidateRemoveFailedPodsArgs(pluginConfig.Args.Object.(*removefailedpods.RemoveFailedPodsArgs))
				errorsInProfiles = setErrorsInProfiles(err, profile.Name, errorsInProfiles)
			case defaultevictor.PluginName:
				_, err := defaultevictor.ValidateDefaultEvictorArgs(pluginConfig.Args.Object.(*defaultevictor.DefaultEvictorArgs))
				errorsInProfiles = setErrorsInProfiles(err, profile.Name, errorsInProfiles)
			default:
				// For now erroing out on unexpected plugin names,
				// TODO: call validations for any registered plugin,
				// including out-of-tree plugins
				err := fmt.Errorf("unexpected plugin name")
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

func strategiesToProfiles(strategies v1alpha1.StrategyList) (*[]v1alpha2.Profile, error) {
	var profiles []v1alpha2.Profile
	for name, strategy := range strategies {
		switch name {
		case removeduplicates.PluginName:
			removeduplicatesArgs := convertRemoveDuplicatesArgs(strategy.Params)
			profile := strategyToProfileWithBalancePlugin(removeduplicatesArgs, name, strategy)
			profile.Name = removeduplicates.PluginName
			if len(profile.PluginConfig) > 0 {
				profiles = append(profiles, profile)
			}
		case nodeutilization.LowNodeUtilizationPluginName:
			lowNodeUtilizationArgs := convertLowNodeUtilizationArgs(strategy.Params)
			profile := strategyToProfileWithBalancePlugin(lowNodeUtilizationArgs, name, strategy)
			profile.Name = nodeutilization.LowNodeUtilizationPluginName
			if len(profile.PluginConfig) > 0 {
				profiles = append(profiles, profile)
			}
		case nodeutilization.HighNodeUtilizationPluginName:
			highNodeUtilizationArgs := convertHighNodeUtilizationArgs(strategy.Params)
			profile := strategyToProfileWithBalancePlugin(highNodeUtilizationArgs, name, strategy)
			profile.Name = nodeutilization.HighNodeUtilizationPluginName
			if len(profile.PluginConfig) > 0 {
				profiles = append(profiles, profile)
			}
		case removepodsviolatinginterpodantiaffinity.PluginName:
			removePodsViolatingInterPodAntiAffinityArgs := convertRemovePodsViolatingInterPodAntiAffinityArgs(strategy.Params)
			profile := strategyToProfileWithDeschedulePlugin(removePodsViolatingInterPodAntiAffinityArgs, name, strategy)
			profile.Name = removepodsviolatinginterpodantiaffinity.PluginName
			if len(profile.PluginConfig) > 0 {
				profiles = append(profiles, profile)
			}
		case removepodsviolatingnodeaffinity.PluginName:
			removePodsViolatingNodeAffinityArgs := convertRemovePodsViolatingNodeAffinityArgs(strategy.Params)
			profile := strategyToProfileWithDeschedulePlugin(removePodsViolatingNodeAffinityArgs, name, strategy)
			profile.Name = removepodsviolatingnodeaffinity.PluginName
			if len(profile.PluginConfig) > 0 {
				profiles = append(profiles, profile)
			}
		case removepodsviolatingnodetaints.PluginName:
			removePodsViolatingNodeTaintsArgs := convertRemovePodsViolatingNodeTaintsArgs(strategy.Params)
			profile := strategyToProfileWithDeschedulePlugin(removePodsViolatingNodeTaintsArgs, name, strategy)
			profile.Name = removepodsviolatingnodetaints.PluginName
			if len(profile.PluginConfig) > 0 {
				profiles = append(profiles, profile)
			}
		case removepodsviolatingtopologyspreadconstraint.PluginName:
			removePodsViolatingTopologySpreadConstraintArgs := convertRemovePodsViolatingTopologySpreadConstraintArgs(strategy.Params)
			profile := strategyToProfileWithBalancePlugin(removePodsViolatingTopologySpreadConstraintArgs, name, strategy)
			profile.Name = removepodsviolatingtopologyspreadconstraint.PluginName
			if len(profile.PluginConfig) > 0 {
				profiles = append(profiles, profile)
			}
		case removepodshavingtoomanyrestarts.PluginName:
			removePodsHavingTooManyRestartsArgs := convertRemovePodsHavingTooManyRestartsArgs(strategy.Params)
			profile := strategyToProfileWithDeschedulePlugin(removePodsHavingTooManyRestartsArgs, name, strategy)
			profile.Name = removepodshavingtoomanyrestarts.PluginName
			if len(profile.PluginConfig) > 0 {
				profiles = append(profiles, profile)
			}
		case podlifetime.PluginName:
			podLifeTimeArgs := convertPodLifeTimeArgs(strategy.Params)
			profile := strategyToProfileWithDeschedulePlugin(podLifeTimeArgs, name, strategy)
			profile.Name = podlifetime.PluginName
			if len(profile.PluginConfig) > 0 {
				profiles = append(profiles, profile)
			}
		case removefailedpods.PluginName:
			RemoveFailedPodsArgs := convertRemoveFailedPodsArgs(strategy.Params)
			profile := strategyToProfileWithDeschedulePlugin(RemoveFailedPodsArgs, name, strategy)
			profile.Name = removefailedpods.PluginName
			if len(profile.PluginConfig) > 0 {
				profiles = append(profiles, profile)
			}
		default:
			return nil, fmt.Errorf("could not process strategy: %s", string(name))
		}
	}
	// easier to test and to know what to expect if it is sorted
	// (accessing the map with 'for key, val := range map' can start with any of the keys)
	profiles = v1alpha2.SortProfilesByName(profiles)
	return &profiles, nil
}

func strategyToProfileWithBalancePlugin(args runtime.Object, name v1alpha1.StrategyName, strategy v1alpha1.DeschedulerStrategy) v1alpha2.Profile {
	var profile v1alpha2.Profile
	newPluginConfig := configurePlugin(args, string(name))
	profile.PluginConfig = append(profile.PluginConfig, newPluginConfig)
	if strategy.Enabled {
		profile.Plugins.Balance.Enabled = append(profile.Plugins.Balance.Enabled, string(name))
	} else {
		profile.Plugins.Balance.Disabled = append(profile.Plugins.Balance.Enabled, string(name))
	}
	return profile
}

func strategyToProfileWithDeschedulePlugin(args runtime.Object, name v1alpha1.StrategyName, strategy v1alpha1.DeschedulerStrategy) v1alpha2.Profile {
	var profile v1alpha2.Profile
	newPluginConfig := configurePlugin(args, string(name))
	profile.PluginConfig = append(profile.PluginConfig, newPluginConfig)
	if strategy.Enabled {
		profile.Plugins.Deschedule.Enabled = append(profile.Plugins.Deschedule.Enabled, string(name))
	} else {
		profile.Plugins.Deschedule.Disabled = append(profile.Plugins.Deschedule.Enabled, string(name))
	}
	return profile
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

func convertRemoveDuplicatesArgs(params *v1alpha1.StrategyParameters) *removeduplicates.RemoveDuplicatesArgs {
	removeduplicatesArgs := &removeduplicates.RemoveDuplicatesArgs{}
	if params.RemoveDuplicates != nil {
		removeduplicatesArgs.ExcludeOwnerKinds = params.RemoveDuplicates.ExcludeOwnerKinds
	}
	if params.Namespaces != nil {
		removeduplicatesArgs.Namespaces = &api.Namespaces{
			Include: params.Namespaces.Include,
			Exclude: params.Namespaces.Exclude,
		}
	}
	return removeduplicatesArgs
}

func convertLowNodeUtilizationArgs(params *v1alpha1.StrategyParameters) *nodeutilization.LowNodeUtilizationArgs {
	lowNodeUtilizationArgs := &nodeutilization.LowNodeUtilizationArgs{}
	if params.NodeResourceUtilizationThresholds != nil {
		lowNodeUtilizationArgs.TargetThresholds = params.NodeResourceUtilizationThresholds.TargetThresholds
		lowNodeUtilizationArgs.Thresholds = params.NodeResourceUtilizationThresholds.Thresholds
		lowNodeUtilizationArgs.UseDeviationThresholds = params.NodeResourceUtilizationThresholds.UseDeviationThresholds
		lowNodeUtilizationArgs.NumberOfNodes = params.NodeResourceUtilizationThresholds.NumberOfNodes
	}
	return lowNodeUtilizationArgs
}

func convertHighNodeUtilizationArgs(params *v1alpha1.StrategyParameters) *nodeutilization.HighNodeUtilizationArgs {
	highNodeUtilizationArgs := &nodeutilization.HighNodeUtilizationArgs{}
	if params.NodeResourceUtilizationThresholds != nil {
		highNodeUtilizationArgs.NumberOfNodes = params.NodeResourceUtilizationThresholds.NumberOfNodes
	}
	return highNodeUtilizationArgs
}

func convertRemovePodsViolatingInterPodAntiAffinityArgs(params *v1alpha1.StrategyParameters) *removepodsviolatinginterpodantiaffinity.RemovePodsViolatingInterPodAntiAffinityArgs {
	removePodsViolatingInterPodAntiAffinityArgs := &removepodsviolatinginterpodantiaffinity.RemovePodsViolatingInterPodAntiAffinityArgs{}
	if params.Namespaces != nil {
		removePodsViolatingInterPodAntiAffinityArgs.Namespaces = &api.Namespaces{
			Include: params.Namespaces.Include,
			Exclude: params.Namespaces.Exclude,
		}
	}
	removePodsViolatingInterPodAntiAffinityArgs.LabelSelector = params.LabelSelector
	return removePodsViolatingInterPodAntiAffinityArgs
}

func convertRemovePodsViolatingNodeAffinityArgs(params *v1alpha1.StrategyParameters) *removepodsviolatingnodeaffinity.RemovePodsViolatingNodeAffinityArgs {
	removePodsViolatingNodeAffinityArgs := &removepodsviolatingnodeaffinity.RemovePodsViolatingNodeAffinityArgs{}
	if params.Namespaces != nil {
		removePodsViolatingNodeAffinityArgs.Namespaces = &api.Namespaces{
			Include: params.Namespaces.Include,
			Exclude: params.Namespaces.Exclude,
		}
	}
	removePodsViolatingNodeAffinityArgs.LabelSelector = params.LabelSelector
	return removePodsViolatingNodeAffinityArgs
}

func convertRemovePodsViolatingNodeTaintsArgs(params *v1alpha1.StrategyParameters) *removepodsviolatingnodetaints.RemovePodsViolatingNodeTaintsArgs {
	removePodsViolatingNodeTaintsArgs := &removepodsviolatingnodetaints.RemovePodsViolatingNodeTaintsArgs{}
	if params.Namespaces != nil {
		removePodsViolatingNodeTaintsArgs.Namespaces = &api.Namespaces{
			Include: params.Namespaces.Include,
			Exclude: params.Namespaces.Exclude,
		}
	}
	removePodsViolatingNodeTaintsArgs.LabelSelector = params.LabelSelector
	removePodsViolatingNodeTaintsArgs.IncludePreferNoSchedule = params.IncludePreferNoSchedule
	removePodsViolatingNodeTaintsArgs.ExcludedTaints = params.ExcludedTaints
	return removePodsViolatingNodeTaintsArgs
}

func convertRemovePodsViolatingTopologySpreadConstraintArgs(params *v1alpha1.StrategyParameters) *removepodsviolatingtopologyspreadconstraint.RemovePodsViolatingTopologySpreadConstraintArgs {
	removePodsViolatingTopologySpreadConstraintArgs := &removepodsviolatingtopologyspreadconstraint.RemovePodsViolatingTopologySpreadConstraintArgs{}
	if params.Namespaces != nil {
		removePodsViolatingTopologySpreadConstraintArgs.Namespaces = &api.Namespaces{
			Include: params.Namespaces.Include,
			Exclude: params.Namespaces.Exclude,
		}
	}
	removePodsViolatingTopologySpreadConstraintArgs.LabelSelector = params.LabelSelector
	removePodsViolatingTopologySpreadConstraintArgs.IncludeSoftConstraints = params.IncludeSoftConstraints
	return removePodsViolatingTopologySpreadConstraintArgs
}

func convertRemovePodsHavingTooManyRestartsArgs(params *v1alpha1.StrategyParameters) *removepodshavingtoomanyrestarts.RemovePodsHavingTooManyRestartsArgs {
	removePodsHavingTooManyRestartsArgs := &removepodshavingtoomanyrestarts.RemovePodsHavingTooManyRestartsArgs{}
	if params.Namespaces != nil {
		removePodsHavingTooManyRestartsArgs.Namespaces = &api.Namespaces{
			Include: params.Namespaces.Include,
			Exclude: params.Namespaces.Exclude,
		}
	}
	removePodsHavingTooManyRestartsArgs.LabelSelector = params.LabelSelector
	if params.PodsHavingTooManyRestarts != nil {
		removePodsHavingTooManyRestartsArgs.PodRestartThreshold = params.PodsHavingTooManyRestarts.PodRestartThreshold
		removePodsHavingTooManyRestartsArgs.IncludingInitContainers = params.PodsHavingTooManyRestarts.IncludingInitContainers
	}
	return removePodsHavingTooManyRestartsArgs
}

func convertPodLifeTimeArgs(params *v1alpha1.StrategyParameters) *podlifetime.PodLifeTimeArgs {
	podLifeTimeArgs := &podlifetime.PodLifeTimeArgs{}
	if params.Namespaces != nil {
		podLifeTimeArgs.Namespaces = &api.Namespaces{
			Include: params.Namespaces.Include,
			Exclude: params.Namespaces.Exclude,
		}
	}
	podLifeTimeArgs.LabelSelector = params.LabelSelector
	if params.PodLifeTime != nil {
		podLifeTimeArgs.MaxPodLifeTimeSeconds = params.PodLifeTime.MaxPodLifeTimeSeconds
		podLifeTimeArgs.States = params.PodLifeTime.States
	}
	return podLifeTimeArgs
}

func convertRemoveFailedPodsArgs(params *v1alpha1.StrategyParameters) *removefailedpods.RemoveFailedPodsArgs {
	removeFailedPodsArgs := &removefailedpods.RemoveFailedPodsArgs{}
	if params.Namespaces != nil {
		removeFailedPodsArgs.Namespaces = &api.Namespaces{
			Include: params.Namespaces.Include,
			Exclude: params.Namespaces.Exclude,
		}
	}
	removeFailedPodsArgs.LabelSelector = params.LabelSelector
	if params.FailedPods != nil {
		removeFailedPodsArgs.ExcludeOwnerKinds = params.FailedPods.ExcludeOwnerKinds
		removeFailedPodsArgs.MinPodLifetimeSeconds = params.FailedPods.MinPodLifetimeSeconds
		removeFailedPodsArgs.Reasons = params.FailedPods.Reasons
		removeFailedPodsArgs.IncludingInitContainers = params.FailedPods.IncludingInitContainers
	}
	return removeFailedPodsArgs
}
