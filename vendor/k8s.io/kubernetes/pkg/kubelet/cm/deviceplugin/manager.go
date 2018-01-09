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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"
	v1helper "k8s.io/kubernetes/pkg/apis/core/v1/helper"
	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/deviceplugin/v1alpha"
	"k8s.io/kubernetes/pkg/kubelet/config"
	"k8s.io/kubernetes/pkg/kubelet/lifecycle"
	"k8s.io/kubernetes/pkg/kubelet/metrics"
	"k8s.io/kubernetes/plugin/pkg/scheduler/schedulercache"
)

// ActivePodsFunc is a function that returns a list of pods to reconcile.
type ActivePodsFunc func() []*v1.Pod

// monitorCallback is the function called when a device's health state changes,
// or new devices are reported, or old devices are deleted.
// Updated contains the most recent state of the Device.
type monitorCallback func(resourceName string, added, updated, deleted []pluginapi.Device)

// ManagerImpl is the structure in charge of managing Device Plugins.
type ManagerImpl struct {
	socketname string
	socketdir  string

	endpoints map[string]endpoint // Key is ResourceName
	mutex     sync.Mutex

	server *grpc.Server

	// activePods is a method for listing active pods on the node
	// so the amount of pluginResources requested by existing pods
	// could be counted when updating allocated devices
	activePods ActivePodsFunc

	// sourcesReady provides the readiness of kubelet configuration sources such as apiserver update readiness.
	// We use it to determine when we can purge inactive pods from checkpointed state.
	sourcesReady config.SourcesReady

	// callback is used for updating devices' states in one time call.
	// e.g. a new device is advertised, two old devices are deleted and a running device fails.
	callback monitorCallback

	// allDevices contains all of registered resourceNames and their exported device IDs.
	allDevices map[string]sets.String

	// allocatedDevices contains allocated deviceIds, keyed by resourceName.
	allocatedDevices map[string]sets.String

	// podDevices contains pod to allocated device mapping.
	podDevices podDevices
}

type sourcesReadyStub struct{}

func (s *sourcesReadyStub) AddSource(source string) {}
func (s *sourcesReadyStub) AllReady() bool          { return true }

// NewManagerImpl creates a new manager.
func NewManagerImpl() (*ManagerImpl, error) {
	return newManagerImpl(pluginapi.KubeletSocket)
}

func newManagerImpl(socketPath string) (*ManagerImpl, error) {
	glog.V(2).Infof("Creating Device Plugin manager at %s", socketPath)

	if socketPath == "" || !filepath.IsAbs(socketPath) {
		return nil, fmt.Errorf(errBadSocket+" %v", socketPath)
	}

	dir, file := filepath.Split(socketPath)
	manager := &ManagerImpl{
		endpoints:        make(map[string]endpoint),
		socketname:       file,
		socketdir:        dir,
		allDevices:       make(map[string]sets.String),
		allocatedDevices: make(map[string]sets.String),
		podDevices:       make(podDevices),
	}
	manager.callback = manager.genericDeviceUpdateCallback

	// The following structs are populated with real implementations in manager.Start()
	// Before that, initializes them to perform no-op operations.
	manager.activePods = func() []*v1.Pod { return []*v1.Pod{} }
	manager.sourcesReady = &sourcesReadyStub{}

	return manager, nil
}

func (m *ManagerImpl) genericDeviceUpdateCallback(resourceName string, added, updated, deleted []pluginapi.Device) {
	kept := append(updated, added...)
	m.mutex.Lock()
	if _, ok := m.allDevices[resourceName]; !ok {
		m.allDevices[resourceName] = sets.NewString()
	}
	// For now, Manager only keeps track of healthy devices.
	// TODO: adds support to track unhealthy devices.
	for _, dev := range kept {
		if dev.Health == pluginapi.Healthy {
			m.allDevices[resourceName].Insert(dev.ID)
		} else {
			m.allDevices[resourceName].Delete(dev.ID)
		}
	}
	for _, dev := range deleted {
		m.allDevices[resourceName].Delete(dev.ID)
	}
	m.mutex.Unlock()
	m.writeCheckpoint()
}

func (m *ManagerImpl) removeContents(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	names, err := d.Readdirnames(-1)
	if err != nil {
		return err
	}
	for _, name := range names {
		filePath := filepath.Join(dir, name)
		if filePath == m.checkpointFile() {
			continue
		}
		stat, err := os.Stat(filePath)
		if err != nil {
			glog.Errorf("Failed to stat file %v: %v", filePath, err)
			continue
		}
		if stat.IsDir() {
			continue
		}
		err = os.RemoveAll(filePath)
		if err != nil {
			return err
		}
	}
	return nil
}

const (
	// kubeletDevicePluginCheckpoint is the file name of device plugin checkpoint
	kubeletDevicePluginCheckpoint = "kubelet_internal_checkpoint"
)

// checkpointFile returns device plugin checkpoint file path.
func (m *ManagerImpl) checkpointFile() string {
	return filepath.Join(m.socketdir, kubeletDevicePluginCheckpoint)
}

// Start starts the Device Plugin Manager amd start initialization of
// podDevices and allocatedDevices information from checkpoint-ed state and
// starts device plugin registration service.
func (m *ManagerImpl) Start(activePods ActivePodsFunc, sourcesReady config.SourcesReady) error {
	glog.V(2).Infof("Starting Device Plugin manager")

	m.activePods = activePods
	m.sourcesReady = sourcesReady

	// Loads in allocatedDevices information from disk.
	err := m.readCheckpoint()
	if err != nil {
		glog.Warningf("Continue after failing to read checkpoint file. Device allocation info may NOT be up-to-date. Err: %v", err)
	}

	socketPath := filepath.Join(m.socketdir, m.socketname)
	os.MkdirAll(m.socketdir, 0755)

	// Removes all stale sockets in m.socketdir. Device plugins can monitor
	// this and use it as a signal to re-register with the new Kubelet.
	if err := m.removeContents(m.socketdir); err != nil {
		glog.Errorf("Fail to clean up stale contents under %s: %+v", m.socketdir, err)
	}

	s, err := net.Listen("unix", socketPath)
	if err != nil {
		glog.Errorf(errListenSocket+" %+v", err)
		return err
	}

	m.server = grpc.NewServer([]grpc.ServerOption{}...)

	pluginapi.RegisterRegistrationServer(m.server, m)
	go m.server.Serve(s)

	glog.V(2).Infof("Serving device plugin registration server on %q", socketPath)

	return nil
}

// Devices is the map of devices that are known by the Device
// Plugin manager with the kind of the devices as key
func (m *ManagerImpl) Devices() map[string][]pluginapi.Device {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	devs := make(map[string][]pluginapi.Device)
	for k, e := range m.endpoints {
		glog.V(3).Infof("Endpoint: %+v: %p", k, e)
		devs[k] = e.getDevices()
	}

	return devs
}

// Allocate is the call that you can use to allocate a set of devices
// from the registered device plugins.
func (m *ManagerImpl) Allocate(node *schedulercache.NodeInfo, attrs *lifecycle.PodAdmitAttributes) error {
	pod := attrs.Pod
	devicesToReuse := make(map[string]sets.String)
	// TODO: Reuse devices between init containers and regular containers.
	for _, container := range pod.Spec.InitContainers {
		if err := m.allocateContainerResources(pod, &container, devicesToReuse); err != nil {
			return err
		}
		m.podDevices.addContainerAllocatedResources(string(pod.UID), container.Name, devicesToReuse)
	}
	for _, container := range pod.Spec.Containers {
		if err := m.allocateContainerResources(pod, &container, devicesToReuse); err != nil {
			return err
		}
		m.podDevices.removeContainerAllocatedResources(string(pod.UID), container.Name, devicesToReuse)
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	// quick return if no pluginResources requested
	if _, podRequireDevicePluginResource := m.podDevices[string(pod.UID)]; !podRequireDevicePluginResource {
		return nil
	}

	m.sanitizeNodeAllocatable(node)
	return nil
}

// Register registers a device plugin.
func (m *ManagerImpl) Register(ctx context.Context, r *pluginapi.RegisterRequest) (*pluginapi.Empty, error) {
	glog.Infof("Got registration request from device plugin with resource name %q", r.ResourceName)
	metrics.DevicePluginRegistrationCount.WithLabelValues(r.ResourceName).Inc()
	if r.Version != pluginapi.Version {
		errorString := fmt.Sprintf(errUnsuportedVersion, r.Version, pluginapi.Version)
		glog.Infof("Bad registration request from device plugin with resource name %q: %v", r.ResourceName, errorString)
		return &pluginapi.Empty{}, fmt.Errorf(errorString)
	}

	if !v1helper.IsExtendedResourceName(v1.ResourceName(r.ResourceName)) {
		errorString := fmt.Sprintf(errInvalidResourceName, r.ResourceName)
		glog.Infof("Bad registration request from device plugin: %v", errorString)
		return &pluginapi.Empty{}, fmt.Errorf(errorString)
	}

	// TODO: for now, always accepts newest device plugin. Later may consider to
	// add some policies here, e.g., verify whether an old device plugin with the
	// same resource name is still alive to determine whether we want to accept
	// the new registration.
	go m.addEndpoint(r)

	return &pluginapi.Empty{}, nil
}

// Stop is the function that can stop the gRPC server.
func (m *ManagerImpl) Stop() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	for _, e := range m.endpoints {
		e.stop()
	}

	m.server.Stop()
	return nil
}

func (m *ManagerImpl) addEndpoint(r *pluginapi.RegisterRequest) {
	existingDevs := make(map[string]pluginapi.Device)
	m.mutex.Lock()
	old, ok := m.endpoints[r.ResourceName]
	if ok && old != nil {
		// Pass devices of previous endpoint into re-registered one,
		// to avoid potential orphaned devices upon re-registration
		devices := make(map[string]pluginapi.Device)
		for _, device := range old.getDevices() {
			devices[device.ID] = device
		}
		existingDevs = devices
	}
	m.mutex.Unlock()

	socketPath := filepath.Join(m.socketdir, r.Endpoint)
	e, err := newEndpointImpl(socketPath, r.ResourceName, existingDevs, m.callback)
	if err != nil {
		glog.Errorf("Failed to dial device plugin with request %v: %v", r, err)
		return
	}

	m.mutex.Lock()
	// Check for potential re-registration during the initialization of new endpoint,
	// and skip updating if re-registration happens.
	// TODO: simplify the part once we have a better way to handle registered devices
	ext := m.endpoints[r.ResourceName]
	if ext != old {
		glog.Warningf("Some other endpoint %v is added while endpoint %v is initialized", ext, e)
		m.mutex.Unlock()
		e.stop()
		return
	}
	// Associates the newly created endpoint with the corresponding resource name.
	// Stops existing endpoint if there is any.
	m.endpoints[r.ResourceName] = e
	glog.V(2).Infof("Registered endpoint %v", e)
	m.mutex.Unlock()

	if old != nil {
		old.stop()
	}

	go func() {
		e.run()
		e.stop()

		m.mutex.Lock()
		if old, ok := m.endpoints[r.ResourceName]; ok && old == e {
			glog.V(2).Infof("Delete resource for endpoint %v", e)
			delete(m.endpoints, r.ResourceName)
		}

		glog.V(2).Infof("Unregistered endpoint %v", e)
		m.mutex.Unlock()
	}()
}

// GetCapacity is expected to be called when Kubelet updates its node status.
// The first returned variable contains the registered device plugin resource capacity.
// The second returned variable contains previously registered resources that are no longer active.
// Kubelet uses this information to update resource capacity/allocatable in its node status.
// After the call, device plugin can remove the inactive resources from its internal list as the
// change is already reflected in Kubelet node status.
// Note in the special case after Kubelet restarts, device plugin resource capacities can
// temporarily drop to zero till corresponding device plugins re-register. This is OK because
// cm.UpdatePluginResource() run during predicate Admit guarantees we adjust nodeinfo
// capacity for already allocated pods so that they can continue to run. However, new pods
// requiring device plugin resources will not be scheduled till device plugin re-registers.
func (m *ManagerImpl) GetCapacity() (v1.ResourceList, []string) {
	needsUpdateCheckpoint := false
	var capacity = v1.ResourceList{}
	var deletedResources []string
	m.mutex.Lock()
	for resourceName, devices := range m.allDevices {
		if _, ok := m.endpoints[resourceName]; !ok {
			delete(m.allDevices, resourceName)
			deletedResources = append(deletedResources, resourceName)
			needsUpdateCheckpoint = true
		} else {
			capacity[v1.ResourceName(resourceName)] = *resource.NewQuantity(int64(devices.Len()), resource.DecimalSI)
		}
	}
	m.mutex.Unlock()
	if needsUpdateCheckpoint {
		m.writeCheckpoint()
	}
	return capacity, deletedResources
}

// checkpointData struct is used to store pod to device allocation information
// and registered device information in a checkpoint file.
// TODO: add version control when we need to change checkpoint format.
type checkpointData struct {
	PodDeviceEntries  []podDevicesCheckpointEntry
	RegisteredDevices map[string][]string
}

// Checkpoints device to container allocation information to disk.
func (m *ManagerImpl) writeCheckpoint() error {
	m.mutex.Lock()
	data := checkpointData{
		PodDeviceEntries:  m.podDevices.toCheckpointData(),
		RegisteredDevices: make(map[string][]string),
	}
	for resource, devices := range m.allDevices {
		data.RegisteredDevices[resource] = devices.UnsortedList()
	}
	m.mutex.Unlock()

	dataJSON, err := json.Marshal(data)
	if err != nil {
		return err
	}
	filepath := m.checkpointFile()
	return ioutil.WriteFile(filepath, dataJSON, 0644)
}

// Reads device to container allocation information from disk, and populates
// m.allocatedDevices accordingly.
func (m *ManagerImpl) readCheckpoint() error {
	filepath := m.checkpointFile()
	content, err := ioutil.ReadFile(filepath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read checkpoint file %q: %v", filepath, err)
	}
	glog.V(2).Infof("Read checkpoint file %s\n", filepath)
	var data checkpointData
	if err := json.Unmarshal(content, &data); err != nil {
		return fmt.Errorf("failed to unmarshal checkpoint data: %v", err)
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.podDevices.fromCheckpointData(data.PodDeviceEntries)
	m.allocatedDevices = m.podDevices.devices()
	for resource, devices := range data.RegisteredDevices {
		m.allDevices[resource] = sets.NewString()
		for _, dev := range devices {
			m.allDevices[resource].Insert(dev)
		}
	}
	return nil
}

// updateAllocatedDevices gets a list of active pods and then frees any Devices that are bound to
// terminated pods. Returns error on failure.
func (m *ManagerImpl) updateAllocatedDevices(activePods []*v1.Pod) {
	if !m.sourcesReady.AllReady() {
		return
	}
	m.mutex.Lock()
	defer m.mutex.Unlock()
	activePodUids := sets.NewString()
	for _, pod := range activePods {
		activePodUids.Insert(string(pod.UID))
	}
	allocatedPodUids := m.podDevices.pods()
	podsToBeRemoved := allocatedPodUids.Difference(activePodUids)
	if len(podsToBeRemoved) <= 0 {
		return
	}
	glog.V(3).Infof("pods to be removed: %v", podsToBeRemoved.List())
	m.podDevices.delete(podsToBeRemoved.List())
	// Regenerated allocatedDevices after we update pod allocation information.
	m.allocatedDevices = m.podDevices.devices()
}

// Returns list of device Ids we need to allocate with Allocate rpc call.
// Returns empty list in case we don't need to issue the Allocate rpc call.
func (m *ManagerImpl) devicesToAllocate(podUID, contName, resource string, required int, reusableDevices sets.String) (sets.String, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	needed := required
	// Gets list of devices that have already been allocated.
	// This can happen if a container restarts for example.
	devices := m.podDevices.containerDevices(podUID, contName, resource)
	if devices != nil {
		glog.V(3).Infof("Found pre-allocated devices for resource %s container %q in Pod %q: %v", resource, contName, podUID, devices.List())
		needed = needed - devices.Len()
		// A pod's resource is not expected to change once admitted by the API server,
		// so just fail loudly here. We can revisit this part if this no longer holds.
		if needed != 0 {
			return nil, fmt.Errorf("pod %v container %v changed request for resource %v from %v to %v", podUID, contName, resource, devices.Len(), required)
		}
	}
	if needed == 0 {
		// No change, no work.
		return nil, nil
	}
	glog.V(3).Infof("Needs to allocate %v %v for pod %q container %q", needed, resource, podUID, contName)
	// Needs to allocate additional devices.
	if _, ok := m.allDevices[resource]; !ok {
		return nil, fmt.Errorf("can't allocate unregistered device %v", resource)
	}
	devices = sets.NewString()
	// Allocates from reusableDevices list first.
	for device := range reusableDevices {
		devices.Insert(device)
		needed--
		if needed == 0 {
			return devices, nil
		}
	}
	// Needs to allocate additional devices.
	if m.allocatedDevices[resource] == nil {
		m.allocatedDevices[resource] = sets.NewString()
	}
	// Gets Devices in use.
	devicesInUse := m.allocatedDevices[resource]
	// Gets a list of available devices.
	available := m.allDevices[resource].Difference(devicesInUse)
	if int(available.Len()) < needed {
		return nil, fmt.Errorf("requested number of devices unavailable for %s. Requested: %d, Available: %d", resource, needed, available.Len())
	}
	allocated := available.UnsortedList()[:needed]
	// Updates m.allocatedDevices with allocated devices to prevent them
	// from being allocated to other pods/containers, given that we are
	// not holding lock during the rpc call.
	for _, device := range allocated {
		m.allocatedDevices[resource].Insert(device)
		devices.Insert(device)
	}
	return devices, nil
}

// allocateContainerResources attempts to allocate all of required device
// plugin resources for the input container, issues an Allocate rpc request
// for each new device resource requirement, processes their AllocateResponses,
// and updates the cached containerDevices on success.
func (m *ManagerImpl) allocateContainerResources(pod *v1.Pod, container *v1.Container, devicesToReuse map[string]sets.String) error {
	podUID := string(pod.UID)
	contName := container.Name
	allocatedDevicesUpdated := false
	for k, v := range container.Resources.Limits {
		resource := string(k)
		needed := int(v.Value())
		glog.V(3).Infof("needs %d %s", needed, resource)
		_, registeredResource := m.allDevices[resource]
		_, allocatedResource := m.allocatedDevices[resource]
		// Continues if this is neither an active device plugin resource nor
		// a resource we have previously allocated.
		if !registeredResource && !allocatedResource {
			continue
		}
		// Updates allocatedDevices to garbage collect any stranded resources
		// before doing the device plugin allocation.
		if !allocatedDevicesUpdated {
			m.updateAllocatedDevices(m.activePods())
			allocatedDevicesUpdated = true
		}
		allocDevices, err := m.devicesToAllocate(podUID, contName, resource, needed, devicesToReuse[resource])
		if err != nil {
			return err
		}
		if allocDevices == nil || len(allocDevices) <= 0 {
			continue
		}
		startRPCTime := time.Now()
		// devicePluginManager.Allocate involves RPC calls to device plugin, which
		// could be heavy-weight. Therefore we want to perform this operation outside
		// mutex lock. Note if Allocate call fails, we may leave container resources
		// partially allocated for the failed container. We rely on updateAllocatedDevices()
		// to garbage collect these resources later. Another side effect is that if
		// we have X resource A and Y resource B in total, and two containers, container1
		// and container2 both require X resource A and Y resource B. Both allocation
		// requests may fail if we serve them in mixed order.
		// TODO: may revisit this part later if we see inefficient resource allocation
		// in real use as the result of this. Should also consider to parallize device
		// plugin Allocate grpc calls if it becomes common that a container may require
		// resources from multiple device plugins.
		m.mutex.Lock()
		e, ok := m.endpoints[resource]
		m.mutex.Unlock()
		if !ok {
			m.mutex.Lock()
			m.allocatedDevices = m.podDevices.devices()
			m.mutex.Unlock()
			return fmt.Errorf("Unknown Device Plugin %s", resource)
		}

		devs := allocDevices.UnsortedList()
		glog.V(3).Infof("Making allocation request for devices %v for device plugin %s", devs, resource)
		resp, err := e.allocate(devs)
		metrics.DevicePluginAllocationLatency.WithLabelValues(resource).Observe(metrics.SinceInMicroseconds(startRPCTime))
		if err != nil {
			// In case of allocation failure, we want to restore m.allocatedDevices
			// to the actual allocated state from m.podDevices.
			m.mutex.Lock()
			m.allocatedDevices = m.podDevices.devices()
			m.mutex.Unlock()
			return err
		}

		// Update internal cached podDevices state.
		m.mutex.Lock()
		m.podDevices.insert(podUID, contName, resource, allocDevices, resp)
		m.mutex.Unlock()
	}

	// Checkpoints device to container allocation information.
	return m.writeCheckpoint()
}

// GetDeviceRunContainerOptions checks whether we have cached containerDevices
// for the passed-in <pod, container> and returns its DeviceRunContainerOptions
// for the found one. An empty struct is returned in case no cached state is found.
func (m *ManagerImpl) GetDeviceRunContainerOptions(pod *v1.Pod, container *v1.Container) *DeviceRunContainerOptions {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.podDevices.deviceRunContainerOptions(string(pod.UID), container.Name)
}

// sanitizeNodeAllocatable scans through allocatedDevices in the device manager
// and if necessary, updates allocatableResource in nodeInfo to at least equal to
// the allocated capacity. This allows pods that have already been scheduled on
// the node to pass GeneralPredicates admission checking even upon device plugin failure.
func (m *ManagerImpl) sanitizeNodeAllocatable(node *schedulercache.NodeInfo) {
	var newAllocatableResource *schedulercache.Resource
	allocatableResource := node.AllocatableResource()
	if allocatableResource.ScalarResources == nil {
		allocatableResource.ScalarResources = make(map[v1.ResourceName]int64)
	}
	for resource, devices := range m.allocatedDevices {
		needed := devices.Len()
		quant, ok := allocatableResource.ScalarResources[v1.ResourceName(resource)]
		if ok && int(quant) >= needed {
			continue
		}
		// Needs to update nodeInfo.AllocatableResource to make sure
		// NodeInfo.allocatableResource at least equal to the capacity already allocated.
		if newAllocatableResource == nil {
			newAllocatableResource = allocatableResource.Clone()
		}
		newAllocatableResource.ScalarResources[v1.ResourceName(resource)] = int64(needed)
	}
	if newAllocatableResource != nil {
		node.SetAllocatableResource(newAllocatableResource)
	}
}
