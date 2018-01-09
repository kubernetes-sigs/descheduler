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

package deviceplugin

import (
	"k8s.io/api/core/v1"
	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/deviceplugin/v1alpha"
	"k8s.io/kubernetes/pkg/kubelet/config"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
	"k8s.io/kubernetes/pkg/kubelet/lifecycle"
	"k8s.io/kubernetes/plugin/pkg/scheduler/schedulercache"
)

// Manager manages all the Device Plugins running on a node.
type Manager interface {
	// Start starts device plugin registration service.
	Start(activePods ActivePodsFunc, sourcesReady config.SourcesReady) error

	// Devices is the map of devices that have registered themselves
	// against the manager.
	// The map key is the ResourceName of the device plugins.
	Devices() map[string][]pluginapi.Device

	// Allocate configures and assigns devices to pods. The pods are provided
	// through the pod admission attributes in the attrs argument. From the
	// requested device resources, Allocate will communicate with the owning
	// device plugin to allow setup procedures to take place, and for the
	// device plugin to provide runtime settings to use the device (environment
	// variables, mount points and device files). The node object is provided
	// for the device manager to update the node capacity to reflect the
	// currently available devices.
	Allocate(node *schedulercache.NodeInfo, attrs *lifecycle.PodAdmitAttributes) error

	// Stop stops the manager.
	Stop() error

	// GetDeviceRunContainerOptions checks whether we have cached containerDevices
	// for the passed-in <pod, container> and returns its DeviceRunContainerOptions
	// for the found one. An empty struct is returned in case no cached state is found.
	GetDeviceRunContainerOptions(pod *v1.Pod, container *v1.Container) *DeviceRunContainerOptions

	// GetCapacity returns the amount of available device plugin resource capacity
	// and inactive device plugin resources previously registered on the node.
	GetCapacity() (v1.ResourceList, []string)
}

// DeviceRunContainerOptions contains the combined container runtime settings to consume its allocated devices.
type DeviceRunContainerOptions struct {
	// The environment variables list.
	Envs []kubecontainer.EnvVar
	// The mounts for the container.
	Mounts []kubecontainer.Mount
	// The host devices mapped into the container.
	Devices []kubecontainer.DeviceInfo
}

// TODO: evaluate whether we need these error definitions.
const (
	// errFailedToDialDevicePlugin is the error raised when the device plugin could not be
	// reached on the registered socket
	errFailedToDialDevicePlugin = "failed to dial device plugin:"
	// errUnsuportedVersion is the error raised when the device plugin uses an API version not
	// supported by the Kubelet registry
	errUnsuportedVersion = "requested API version %q is not supported by kubelet. Supported version is %q"
	// errDevicePluginAlreadyExists is the error raised when a device plugin with the
	// same Resource Name tries to register itself
	errDevicePluginAlreadyExists = "another device plugin already registered this Resource Name"
	// errInvalidResourceName is the error raised when a device plugin is registering
	// itself with an invalid ResourceName
	errInvalidResourceName = "the ResourceName %q is invalid"
	// errEmptyResourceName is the error raised when the resource name field is empty
	errEmptyResourceName = "invalid Empty ResourceName"

	// errBadSocket is the error raised when the registry socket path is not absolute
	errBadSocket = "bad socketPath, must be an absolute path:"
	// errRemoveSocket is the error raised when the registry could not remove the existing socket
	errRemoveSocket = "failed to remove socket while starting device plugin registry, with error"
	// errListenSocket is the error raised when the registry could not listen on the socket
	errListenSocket = "failed to listen to socket while starting device plugin registry, with error"
	// errListAndWatch is the error raised when ListAndWatch ended unsuccessfully
	errListAndWatch = "listAndWatch ended unexpectedly for device plugin %s with error %v"
)
