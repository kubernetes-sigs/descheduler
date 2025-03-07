//go:build !ignore_autogenerated
// +build !ignore_autogenerated

/*
Copyright 2025 The Kubernetes Authors.

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

package nodeutilization

import (
	runtime "k8s.io/apimachinery/pkg/runtime"
	api "sigs.k8s.io/descheduler/pkg/api"
)

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
	out.MetricsUtilization = in.MetricsUtilization
	if in.EvictableNamespaces != nil {
		in, out := &in.EvictableNamespaces, &out.EvictableNamespaces
		*out = new(api.Namespaces)
		(*in).DeepCopyInto(*out)
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

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *HighNodeUtilizationArgs) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
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
	out.MetricsUtilization = in.MetricsUtilization
	if in.EvictableNamespaces != nil {
		in, out := &in.EvictableNamespaces, &out.EvictableNamespaces
		*out = new(api.Namespaces)
		(*in).DeepCopyInto(*out)
	}
	if in.EvictionLimits != nil {
		in, out := &in.EvictionLimits, &out.EvictionLimits
		*out = new(api.EvictionLimits)
		(*in).DeepCopyInto(*out)
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

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *LowNodeUtilizationArgs) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}
