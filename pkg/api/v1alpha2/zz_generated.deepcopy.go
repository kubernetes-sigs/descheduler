//go:build !ignore_autogenerated
// +build !ignore_autogenerated

/*
Copyright 2022 The Kubernetes Authors.

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

// Code generated by deepcopy-gen. DO NOT EDIT.

package v1alpha2

import (
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	api "sigs.k8s.io/descheduler/pkg/api"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DefaultEvictorArgs) DeepCopyInto(out *DefaultEvictorArgs) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	if in.PriorityThreshold != nil {
		in, out := &in.PriorityThreshold, &out.PriorityThreshold
		*out = new(api.PriorityThreshold)
		(*in).DeepCopyInto(*out)
	}
	if in.LabelSelector != nil {
		in, out := &in.LabelSelector, &out.LabelSelector
		*out = new(v1.LabelSelector)
		(*in).DeepCopyInto(*out)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DefaultEvictorArgs.
func (in *DefaultEvictorArgs) DeepCopy() *DefaultEvictorArgs {
	if in == nil {
		return nil
	}
	out := new(DefaultEvictorArgs)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DeschedulerConfiguration) DeepCopyInto(out *DeschedulerConfiguration) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	if in.Profiles != nil {
		in, out := &in.Profiles, &out.Profiles
		*out = make([]Profile, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.NodeSelector != nil {
		in, out := &in.NodeSelector, &out.NodeSelector
		*out = new(string)
		**out = **in
	}
	if in.MaxNoOfPodsToEvictPerNode != nil {
		in, out := &in.MaxNoOfPodsToEvictPerNode, &out.MaxNoOfPodsToEvictPerNode
		*out = new(int)
		**out = **in
	}
	if in.MaxNoOfPodsToEvictPerNamespace != nil {
		in, out := &in.MaxNoOfPodsToEvictPerNamespace, &out.MaxNoOfPodsToEvictPerNamespace
		*out = new(int)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DeschedulerConfiguration.
func (in *DeschedulerConfiguration) DeepCopy() *DeschedulerConfiguration {
	if in == nil {
		return nil
	}
	out := new(DeschedulerConfiguration)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DeschedulerConfiguration) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HighNodeUtilizationArgs) DeepCopyInto(out *HighNodeUtilizationArgs) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	if in.Thresholds != nil {
		in, out := &in.Thresholds, &out.Thresholds
		*out = make(api.ResourceThresholds, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.TargetThresholds != nil {
		in, out := &in.TargetThresholds, &out.TargetThresholds
		*out = make(api.ResourceThresholds, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HighNodeUtilizationArgs.
func (in *HighNodeUtilizationArgs) DeepCopy() *HighNodeUtilizationArgs {
	if in == nil {
		return nil
	}
	out := new(HighNodeUtilizationArgs)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LowNodeUtilizationArgs) DeepCopyInto(out *LowNodeUtilizationArgs) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	if in.Thresholds != nil {
		in, out := &in.Thresholds, &out.Thresholds
		*out = make(api.ResourceThresholds, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.TargetThresholds != nil {
		in, out := &in.TargetThresholds, &out.TargetThresholds
		*out = make(api.ResourceThresholds, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LowNodeUtilizationArgs.
func (in *LowNodeUtilizationArgs) DeepCopy() *LowNodeUtilizationArgs {
	if in == nil {
		return nil
	}
	out := new(LowNodeUtilizationArgs)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Namespaces) DeepCopyInto(out *Namespaces) {
	*out = *in
	if in.Include != nil {
		in, out := &in.Include, &out.Include
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.Exclude != nil {
		in, out := &in.Exclude, &out.Exclude
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Namespaces.
func (in *Namespaces) DeepCopy() *Namespaces {
	if in == nil {
		return nil
	}
	out := new(Namespaces)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Plugin) DeepCopyInto(out *Plugin) {
	*out = *in
	if in.Enabled != nil {
		in, out := &in.Enabled, &out.Enabled
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.Disabled != nil {
		in, out := &in.Disabled, &out.Disabled
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Plugin.
func (in *Plugin) DeepCopy() *Plugin {
	if in == nil {
		return nil
	}
	out := new(Plugin)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PluginConfig) DeepCopyInto(out *PluginConfig) {
	*out = *in
	if in.Args != nil {
		out.Args = in.Args.DeepCopyObject()
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PluginConfig.
func (in *PluginConfig) DeepCopy() *PluginConfig {
	if in == nil {
		return nil
	}
	out := new(PluginConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Plugins) DeepCopyInto(out *Plugins) {
	*out = *in
	in.PreSort.DeepCopyInto(&out.PreSort)
	in.Sort.DeepCopyInto(&out.Sort)
	in.Deschedule.DeepCopyInto(&out.Deschedule)
	in.Balance.DeepCopyInto(&out.Balance)
	in.Evict.DeepCopyInto(&out.Evict)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Plugins.
func (in *Plugins) DeepCopy() *Plugins {
	if in == nil {
		return nil
	}
	out := new(Plugins)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PodLifeTimeArgs) DeepCopyInto(out *PodLifeTimeArgs) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	if in.Namespaces != nil {
		in, out := &in.Namespaces, &out.Namespaces
		*out = new(api.Namespaces)
		(*in).DeepCopyInto(*out)
	}
	if in.LabelSelector != nil {
		in, out := &in.LabelSelector, &out.LabelSelector
		*out = new(v1.LabelSelector)
		(*in).DeepCopyInto(*out)
	}
	if in.MaxPodLifeTimeSeconds != nil {
		in, out := &in.MaxPodLifeTimeSeconds, &out.MaxPodLifeTimeSeconds
		*out = new(uint)
		**out = **in
	}
	if in.PodStatusPhases != nil {
		in, out := &in.PodStatusPhases, &out.PodStatusPhases
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PodLifeTimeArgs.
func (in *PodLifeTimeArgs) DeepCopy() *PodLifeTimeArgs {
	if in == nil {
		return nil
	}
	out := new(PodLifeTimeArgs)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Profile) DeepCopyInto(out *Profile) {
	*out = *in
	if in.PluginConfig != nil {
		in, out := &in.PluginConfig, &out.PluginConfig
		*out = make([]PluginConfig, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	in.Plugins.DeepCopyInto(&out.Plugins)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Profile.
func (in *Profile) DeepCopy() *Profile {
	if in == nil {
		return nil
	}
	out := new(Profile)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RemoveDuplicatePodsArgs) DeepCopyInto(out *RemoveDuplicatePodsArgs) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	if in.Namespaces != nil {
		in, out := &in.Namespaces, &out.Namespaces
		*out = new(api.Namespaces)
		(*in).DeepCopyInto(*out)
	}
	if in.ExcludeOwnerKinds != nil {
		in, out := &in.ExcludeOwnerKinds, &out.ExcludeOwnerKinds
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RemoveDuplicatePodsArgs.
func (in *RemoveDuplicatePodsArgs) DeepCopy() *RemoveDuplicatePodsArgs {
	if in == nil {
		return nil
	}
	out := new(RemoveDuplicatePodsArgs)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RemoveFailedPodsArgs) DeepCopyInto(out *RemoveFailedPodsArgs) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	if in.Namespaces != nil {
		in, out := &in.Namespaces, &out.Namespaces
		*out = new(api.Namespaces)
		(*in).DeepCopyInto(*out)
	}
	if in.LabelSelector != nil {
		in, out := &in.LabelSelector, &out.LabelSelector
		*out = new(v1.LabelSelector)
		(*in).DeepCopyInto(*out)
	}
	if in.MinPodLifetimeSeconds != nil {
		in, out := &in.MinPodLifetimeSeconds, &out.MinPodLifetimeSeconds
		*out = new(uint)
		**out = **in
	}
	if in.Reasons != nil {
		in, out := &in.Reasons, &out.Reasons
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.ExcludeOwnerKinds != nil {
		in, out := &in.ExcludeOwnerKinds, &out.ExcludeOwnerKinds
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RemoveFailedPodsArgs.
func (in *RemoveFailedPodsArgs) DeepCopy() *RemoveFailedPodsArgs {
	if in == nil {
		return nil
	}
	out := new(RemoveFailedPodsArgs)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RemovePodsHavingTooManyRestartsArgs) DeepCopyInto(out *RemovePodsHavingTooManyRestartsArgs) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	if in.Namespaces != nil {
		in, out := &in.Namespaces, &out.Namespaces
		*out = new(api.Namespaces)
		(*in).DeepCopyInto(*out)
	}
	if in.LabelSelector != nil {
		in, out := &in.LabelSelector, &out.LabelSelector
		*out = new(v1.LabelSelector)
		(*in).DeepCopyInto(*out)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RemovePodsHavingTooManyRestartsArgs.
func (in *RemovePodsHavingTooManyRestartsArgs) DeepCopy() *RemovePodsHavingTooManyRestartsArgs {
	if in == nil {
		return nil
	}
	out := new(RemovePodsHavingTooManyRestartsArgs)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RemovePodsViolatingInterPodAntiAffinityArgs) DeepCopyInto(out *RemovePodsViolatingInterPodAntiAffinityArgs) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	if in.Namespaces != nil {
		in, out := &in.Namespaces, &out.Namespaces
		*out = new(api.Namespaces)
		(*in).DeepCopyInto(*out)
	}
	if in.LabelSelector != nil {
		in, out := &in.LabelSelector, &out.LabelSelector
		*out = new(v1.LabelSelector)
		(*in).DeepCopyInto(*out)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RemovePodsViolatingInterPodAntiAffinityArgs.
func (in *RemovePodsViolatingInterPodAntiAffinityArgs) DeepCopy() *RemovePodsViolatingInterPodAntiAffinityArgs {
	if in == nil {
		return nil
	}
	out := new(RemovePodsViolatingInterPodAntiAffinityArgs)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RemovePodsViolatingNodeAffinityArgs) DeepCopyInto(out *RemovePodsViolatingNodeAffinityArgs) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	if in.Namespaces != nil {
		in, out := &in.Namespaces, &out.Namespaces
		*out = new(api.Namespaces)
		(*in).DeepCopyInto(*out)
	}
	if in.LabelSelector != nil {
		in, out := &in.LabelSelector, &out.LabelSelector
		*out = new(v1.LabelSelector)
		(*in).DeepCopyInto(*out)
	}
	if in.NodeAffinityType != nil {
		in, out := &in.NodeAffinityType, &out.NodeAffinityType
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RemovePodsViolatingNodeAffinityArgs.
func (in *RemovePodsViolatingNodeAffinityArgs) DeepCopy() *RemovePodsViolatingNodeAffinityArgs {
	if in == nil {
		return nil
	}
	out := new(RemovePodsViolatingNodeAffinityArgs)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RemovePodsViolatingNodeTaintsArgs) DeepCopyInto(out *RemovePodsViolatingNodeTaintsArgs) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	if in.Namespaces != nil {
		in, out := &in.Namespaces, &out.Namespaces
		*out = new(api.Namespaces)
		(*in).DeepCopyInto(*out)
	}
	if in.LabelSelector != nil {
		in, out := &in.LabelSelector, &out.LabelSelector
		*out = new(v1.LabelSelector)
		(*in).DeepCopyInto(*out)
	}
	if in.ExcludedTaints != nil {
		in, out := &in.ExcludedTaints, &out.ExcludedTaints
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RemovePodsViolatingNodeTaintsArgs.
func (in *RemovePodsViolatingNodeTaintsArgs) DeepCopy() *RemovePodsViolatingNodeTaintsArgs {
	if in == nil {
		return nil
	}
	out := new(RemovePodsViolatingNodeTaintsArgs)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RemovePodsViolatingTopologySpreadConstraintArgs) DeepCopyInto(out *RemovePodsViolatingTopologySpreadConstraintArgs) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	if in.Namespaces != nil {
		in, out := &in.Namespaces, &out.Namespaces
		*out = new(api.Namespaces)
		(*in).DeepCopyInto(*out)
	}
	if in.LabelSelector != nil {
		in, out := &in.LabelSelector, &out.LabelSelector
		*out = new(v1.LabelSelector)
		(*in).DeepCopyInto(*out)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RemovePodsViolatingTopologySpreadConstraintArgs.
func (in *RemovePodsViolatingTopologySpreadConstraintArgs) DeepCopy() *RemovePodsViolatingTopologySpreadConstraintArgs {
	if in == nil {
		return nil
	}
	out := new(RemovePodsViolatingTopologySpreadConstraintArgs)
	in.DeepCopyInto(out)
	return out
}
