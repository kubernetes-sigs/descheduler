// +build !ignore_autogenerated

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

// This file was autogenerated by conversion-gen. Do not edit it manually!

package v1alpha1

import (
	componentconfig "github.com/aveshagarwal/rescheduler/pkg/apis/componentconfig"
	conversion "k8s.io/apimachinery/pkg/conversion"
	runtime "k8s.io/apimachinery/pkg/runtime"
	time "time"
)

func init() {
	SchemeBuilder.Register(RegisterConversions)
}

// RegisterConversions adds conversion functions to the given scheme.
// Public to allow building arbitrary schemes.
func RegisterConversions(scheme *runtime.Scheme) error {
	return scheme.AddGeneratedConversionFuncs(
		Convert_v1alpha1_ReschedulerConfiguration_To_componentconfig_ReschedulerConfiguration,
		Convert_componentconfig_ReschedulerConfiguration_To_v1alpha1_ReschedulerConfiguration,
	)
}

func autoConvert_v1alpha1_ReschedulerConfiguration_To_componentconfig_ReschedulerConfiguration(in *ReschedulerConfiguration, out *componentconfig.ReschedulerConfiguration, s conversion.Scope) error {
	out.ReschedulingInterval = time.Duration(in.ReschedulingInterval)
	out.KubeconfigFile = in.KubeconfigFile
	out.PolicyConfigFile = in.PolicyConfigFile
	return nil
}

// Convert_v1alpha1_ReschedulerConfiguration_To_componentconfig_ReschedulerConfiguration is an autogenerated conversion function.
func Convert_v1alpha1_ReschedulerConfiguration_To_componentconfig_ReschedulerConfiguration(in *ReschedulerConfiguration, out *componentconfig.ReschedulerConfiguration, s conversion.Scope) error {
	return autoConvert_v1alpha1_ReschedulerConfiguration_To_componentconfig_ReschedulerConfiguration(in, out, s)
}

func autoConvert_componentconfig_ReschedulerConfiguration_To_v1alpha1_ReschedulerConfiguration(in *componentconfig.ReschedulerConfiguration, out *ReschedulerConfiguration, s conversion.Scope) error {
	out.ReschedulingInterval = time.Duration(in.ReschedulingInterval)
	out.KubeconfigFile = in.KubeconfigFile
	out.PolicyConfigFile = in.PolicyConfigFile
	return nil
}

// Convert_componentconfig_ReschedulerConfiguration_To_v1alpha1_ReschedulerConfiguration is an autogenerated conversion function.
func Convert_componentconfig_ReschedulerConfiguration_To_v1alpha1_ReschedulerConfiguration(in *componentconfig.ReschedulerConfiguration, out *ReschedulerConfiguration, s conversion.Scope) error {
	return autoConvert_componentconfig_ReschedulerConfiguration_To_v1alpha1_ReschedulerConfiguration(in, out, s)
}
