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
	"flag"
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/uuid"
	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/deviceplugin/v1alpha"
	"k8s.io/kubernetes/pkg/kubelet/lifecycle"
	"k8s.io/kubernetes/plugin/pkg/scheduler/schedulercache"
)

const (
	socketName       = "/tmp/device_plugin/server.sock"
	pluginSocketName = "/tmp/device_plugin/device-plugin.sock"
	testResourceName = "fake-domain/resource"
)

func TestNewManagerImpl(t *testing.T) {
	_, err := newManagerImpl(socketName)
	require.NoError(t, err)
}

func TestNewManagerImplStart(t *testing.T) {
	m, p := setup(t, []*pluginapi.Device{}, func(n string, a, u, r []pluginapi.Device) {})
	cleanup(t, m, p)
}

// Tests that the device plugin manager correctly handles registration and re-registration by
// making sure that after registration, devices are correctly updated and if a re-registration
// happens, we will NOT delete devices; and no orphaned devices left.
func TestDevicePluginReRegistration(t *testing.T) {
	devs := []*pluginapi.Device{
		{ID: "Dev1", Health: pluginapi.Healthy},
		{ID: "Dev2", Health: pluginapi.Healthy},
	}
	devsForRegistration := []*pluginapi.Device{
		{ID: "Dev3", Health: pluginapi.Healthy},
	}

	callbackCount := 0
	callbackChan := make(chan int)
	var stopping int32
	stopping = 0
	callback := func(n string, a, u, r []pluginapi.Device) {
		// Should be called three times, one for each plugin registration, till we are stopping.
		if callbackCount > 2 && atomic.LoadInt32(&stopping) <= 0 {
			t.FailNow()
		}
		callbackCount++
		callbackChan <- callbackCount
	}
	m, p1 := setup(t, devs, callback)
	p1.Register(socketName, testResourceName)
	// Wait for the first callback to be issued.

	<-callbackChan
	// Wait till the endpoint is added to the manager.
	for i := 0; i < 20; i++ {
		if len(m.Devices()) > 0 {
			break
		}
		time.Sleep(1)
	}
	devices := m.Devices()
	require.Equal(t, 2, len(devices[testResourceName]), "Devices are not updated.")

	p2 := NewDevicePluginStub(devs, pluginSocketName+".new")
	err := p2.Start()
	require.NoError(t, err)
	p2.Register(socketName, testResourceName)
	// Wait for the second callback to be issued.
	<-callbackChan

	devices2 := m.Devices()
	require.Equal(t, 2, len(devices2[testResourceName]), "Devices shouldn't change.")

	// Test the scenario that a plugin re-registers with different devices.
	p3 := NewDevicePluginStub(devsForRegistration, pluginSocketName+".third")
	err = p3.Start()
	require.NoError(t, err)
	p3.Register(socketName, testResourceName)
	// Wait for the second callback to be issued.
	<-callbackChan

	devices3 := m.Devices()
	require.Equal(t, 1, len(devices3[testResourceName]), "Devices of plugin previously registered should be removed.")
	// Wait long enough to catch unexpected callbacks.
	time.Sleep(5 * time.Second)

	atomic.StoreInt32(&stopping, 1)
	p2.Stop()
	p3.Stop()
	cleanup(t, m, p1)

}

func setup(t *testing.T, devs []*pluginapi.Device, callback monitorCallback) (Manager, *Stub) {
	m, err := newManagerImpl(socketName)
	require.NoError(t, err)

	m.callback = callback

	activePods := func() []*v1.Pod {
		return []*v1.Pod{}
	}
	err = m.Start(activePods, &sourcesReadyStub{})
	require.NoError(t, err)

	p := NewDevicePluginStub(devs, pluginSocketName)
	err = p.Start()
	require.NoError(t, err)

	return m, p
}

func cleanup(t *testing.T, m Manager, p *Stub) {
	p.Stop()
	m.Stop()
}

func TestUpdateCapacity(t *testing.T) {
	testManager, err := newManagerImpl(socketName)
	as := assert.New(t)
	as.NotNil(testManager)
	as.Nil(err)

	devs := []pluginapi.Device{
		{ID: "Device1", Health: pluginapi.Healthy},
		{ID: "Device2", Health: pluginapi.Healthy},
		{ID: "Device3", Health: pluginapi.Unhealthy},
	}
	callback := testManager.genericDeviceUpdateCallback

	// Adds three devices for resource1, two healthy and one unhealthy.
	// Expects capacity for resource1 to be 2.
	resourceName1 := "domain1.com/resource1"
	testManager.endpoints[resourceName1] = &endpointImpl{devices: make(map[string]pluginapi.Device)}
	callback(resourceName1, devs, []pluginapi.Device{}, []pluginapi.Device{})
	capacity, removedResources := testManager.GetCapacity()
	resource1Capacity, ok := capacity[v1.ResourceName(resourceName1)]
	as.True(ok)
	as.Equal(int64(2), resource1Capacity.Value())
	as.Equal(0, len(removedResources))

	// Deletes an unhealthy device should NOT change capacity.
	callback(resourceName1, []pluginapi.Device{}, []pluginapi.Device{}, []pluginapi.Device{devs[2]})
	capacity, removedResources = testManager.GetCapacity()
	resource1Capacity, ok = capacity[v1.ResourceName(resourceName1)]
	as.True(ok)
	as.Equal(int64(2), resource1Capacity.Value())
	as.Equal(0, len(removedResources))

	// Updates a healthy device to unhealthy should reduce capacity by 1.
	dev2 := devs[1]
	dev2.Health = pluginapi.Unhealthy
	callback(resourceName1, []pluginapi.Device{}, []pluginapi.Device{dev2}, []pluginapi.Device{})
	capacity, removedResources = testManager.GetCapacity()
	resource1Capacity, ok = capacity[v1.ResourceName(resourceName1)]
	as.True(ok)
	as.Equal(int64(1), resource1Capacity.Value())
	as.Equal(0, len(removedResources))

	// Deletes a healthy device should reduce capacity by 1.
	callback(resourceName1, []pluginapi.Device{}, []pluginapi.Device{}, []pluginapi.Device{devs[0]})
	capacity, removedResources = testManager.GetCapacity()
	resource1Capacity, ok = capacity[v1.ResourceName(resourceName1)]
	as.True(ok)
	as.Equal(int64(0), resource1Capacity.Value())
	as.Equal(0, len(removedResources))

	// Tests adding another resource.
	resourceName2 := "resource2"
	testManager.endpoints[resourceName2] = &endpointImpl{devices: make(map[string]pluginapi.Device)}
	callback(resourceName2, devs, []pluginapi.Device{}, []pluginapi.Device{})
	capacity, removedResources = testManager.GetCapacity()
	as.Equal(2, len(capacity))
	resource2Capacity, ok := capacity[v1.ResourceName(resourceName2)]
	as.True(ok)
	as.Equal(int64(2), resource2Capacity.Value())
	as.Equal(0, len(removedResources))

	// Removes resourceName1 endpoint. Verifies testManager.GetCapacity() reports that resourceName1
	// is removed from capacity and it no longer exists in allDevices after the call.
	delete(testManager.endpoints, resourceName1)
	capacity, removed := testManager.GetCapacity()
	as.Equal([]string{resourceName1}, removed)
	_, ok = capacity[v1.ResourceName(resourceName1)]
	as.False(ok)
	val, ok := capacity[v1.ResourceName(resourceName2)]
	as.True(ok)
	as.Equal(int64(2), val.Value())
	_, ok = testManager.allDevices[resourceName1]
	as.False(ok)
}

type stringPairType struct {
	value1 string
	value2 string
}

func constructDevices(devices []string) sets.String {
	ret := sets.NewString()
	for _, dev := range devices {
		ret.Insert(dev)
	}
	return ret
}

func constructAllocResp(devices, mounts, envs map[string]string) *pluginapi.AllocateResponse {
	resp := &pluginapi.AllocateResponse{}
	for k, v := range devices {
		resp.Devices = append(resp.Devices, &pluginapi.DeviceSpec{
			HostPath:      k,
			ContainerPath: v,
			Permissions:   "mrw",
		})
	}
	for k, v := range mounts {
		resp.Mounts = append(resp.Mounts, &pluginapi.Mount{
			ContainerPath: k,
			HostPath:      v,
			ReadOnly:      true,
		})
	}
	resp.Envs = make(map[string]string)
	for k, v := range envs {
		resp.Envs[k] = v
	}
	return resp
}

func TestCheckpoint(t *testing.T) {
	resourceName1 := "domain1.com/resource1"
	resourceName2 := "domain2.com/resource2"

	testManager := &ManagerImpl{
		allDevices:       make(map[string]sets.String),
		allocatedDevices: make(map[string]sets.String),
		podDevices:       make(podDevices),
	}

	testManager.podDevices.insert("pod1", "con1", resourceName1,
		constructDevices([]string{"dev1", "dev2"}),
		constructAllocResp(map[string]string{"/dev/r1dev1": "/dev/r1dev1", "/dev/r1dev2": "/dev/r1dev2"},
			map[string]string{"/home/r1lib1": "/usr/r1lib1"}, map[string]string{}))
	testManager.podDevices.insert("pod1", "con1", resourceName2,
		constructDevices([]string{"dev1", "dev2"}),
		constructAllocResp(map[string]string{"/dev/r2dev1": "/dev/r2dev1", "/dev/r2dev2": "/dev/r2dev2"},
			map[string]string{"/home/r2lib1": "/usr/r2lib1"},
			map[string]string{"r2devices": "dev1 dev2"}))
	testManager.podDevices.insert("pod1", "con2", resourceName1,
		constructDevices([]string{"dev3"}),
		constructAllocResp(map[string]string{"/dev/r1dev3": "/dev/r1dev3"},
			map[string]string{"/home/r1lib1": "/usr/r1lib1"}, map[string]string{}))
	testManager.podDevices.insert("pod2", "con1", resourceName1,
		constructDevices([]string{"dev4"}),
		constructAllocResp(map[string]string{"/dev/r1dev4": "/dev/r1dev4"},
			map[string]string{"/home/r1lib1": "/usr/r1lib1"}, map[string]string{}))

	testManager.allDevices[resourceName1] = sets.NewString()
	testManager.allDevices[resourceName1].Insert("dev1")
	testManager.allDevices[resourceName1].Insert("dev2")
	testManager.allDevices[resourceName1].Insert("dev3")
	testManager.allDevices[resourceName1].Insert("dev4")
	testManager.allDevices[resourceName1].Insert("dev5")
	testManager.allDevices[resourceName2] = sets.NewString()
	testManager.allDevices[resourceName2].Insert("dev1")
	testManager.allDevices[resourceName2].Insert("dev2")

	expectedPodDevices := testManager.podDevices
	expectedAllocatedDevices := testManager.podDevices.devices()
	expectedAllDevices := testManager.allDevices

	err := testManager.writeCheckpoint()
	as := assert.New(t)

	as.Nil(err)
	testManager.podDevices = make(podDevices)
	err = testManager.readCheckpoint()
	as.Nil(err)

	as.Equal(len(expectedPodDevices), len(testManager.podDevices))
	for podUID, containerDevices := range expectedPodDevices {
		for conName, resources := range containerDevices {
			for resource := range resources {
				as.True(reflect.DeepEqual(
					expectedPodDevices.containerDevices(podUID, conName, resource),
					testManager.podDevices.containerDevices(podUID, conName, resource)))
				opts1 := expectedPodDevices.deviceRunContainerOptions(podUID, conName)
				opts2 := testManager.podDevices.deviceRunContainerOptions(podUID, conName)
				as.Equal(len(opts1.Envs), len(opts2.Envs))
				as.Equal(len(opts1.Mounts), len(opts2.Mounts))
				as.Equal(len(opts1.Devices), len(opts2.Devices))
			}
		}
	}
	as.True(reflect.DeepEqual(expectedAllocatedDevices, testManager.allocatedDevices))
	as.True(reflect.DeepEqual(expectedAllDevices, testManager.allDevices))
}

type activePodsStub struct {
	activePods []*v1.Pod
}

func (a *activePodsStub) getActivePods() []*v1.Pod {
	return a.activePods
}

func (a *activePodsStub) updateActivePods(newPods []*v1.Pod) {
	a.activePods = newPods
}

type MockEndpoint struct {
	allocateFunc func(devs []string) (*pluginapi.AllocateResponse, error)
}

func (m *MockEndpoint) stop() {}
func (m *MockEndpoint) run()  {}

func (m *MockEndpoint) getDevices() []pluginapi.Device {
	return []pluginapi.Device{}
}

func (m *MockEndpoint) callback(resourceName string, added, updated, deleted []pluginapi.Device) {}

func (m *MockEndpoint) allocate(devs []string) (*pluginapi.AllocateResponse, error) {
	if m.allocateFunc != nil {
		return m.allocateFunc(devs)
	}
	return nil, nil
}

func TestPodContainerDeviceAllocation(t *testing.T) {
	flag.Set("alsologtostderr", fmt.Sprintf("%t", true))
	var logLevel string
	flag.StringVar(&logLevel, "logLevel", "4", "test")
	flag.Lookup("v").Value.Set(logLevel)

	resourceName1 := "domain1.com/resource1"
	resourceQuantity1 := *resource.NewQuantity(int64(2), resource.DecimalSI)
	devID1 := "dev1"
	devID2 := "dev2"
	resourceName2 := "domain2.com/resource2"
	resourceQuantity2 := *resource.NewQuantity(int64(1), resource.DecimalSI)
	devID3 := "dev3"
	devID4 := "dev4"

	as := require.New(t)
	monitorCallback := func(resourceName string, added, updated, deleted []pluginapi.Device) {}
	podsStub := activePodsStub{
		activePods: []*v1.Pod{},
	}
	cachedNode := &v1.Node{
		Status: v1.NodeStatus{
			Allocatable: v1.ResourceList{},
		},
	}
	nodeInfo := &schedulercache.NodeInfo{}
	nodeInfo.SetNode(cachedNode)

	testManager := &ManagerImpl{
		callback:         monitorCallback,
		allDevices:       make(map[string]sets.String),
		allocatedDevices: make(map[string]sets.String),
		endpoints:        make(map[string]endpoint),
		podDevices:       make(podDevices),
		activePods:       podsStub.getActivePods,
		sourcesReady:     &sourcesReadyStub{},
	}

	testManager.allDevices[resourceName1] = sets.NewString()
	testManager.allDevices[resourceName1].Insert(devID1)
	testManager.allDevices[resourceName1].Insert(devID2)
	testManager.allDevices[resourceName2] = sets.NewString()
	testManager.allDevices[resourceName2].Insert(devID3)
	testManager.allDevices[resourceName2].Insert(devID4)

	testManager.endpoints[resourceName1] = &MockEndpoint{
		allocateFunc: func(devs []string) (*pluginapi.AllocateResponse, error) {
			resp := new(pluginapi.AllocateResponse)
			resp.Envs = make(map[string]string)
			for _, dev := range devs {
				switch dev {
				case "dev1":
					resp.Devices = append(resp.Devices, &pluginapi.DeviceSpec{
						ContainerPath: "/dev/aaa",
						HostPath:      "/dev/aaa",
						Permissions:   "mrw",
					})

					resp.Devices = append(resp.Devices, &pluginapi.DeviceSpec{
						ContainerPath: "/dev/bbb",
						HostPath:      "/dev/bbb",
						Permissions:   "mrw",
					})

					resp.Mounts = append(resp.Mounts, &pluginapi.Mount{
						ContainerPath: "/container_dir1/file1",
						HostPath:      "host_dir1/file1",
						ReadOnly:      true,
					})

				case "dev2":
					resp.Devices = append(resp.Devices, &pluginapi.DeviceSpec{
						ContainerPath: "/dev/ccc",
						HostPath:      "/dev/ccc",
						Permissions:   "mrw",
					})

					resp.Mounts = append(resp.Mounts, &pluginapi.Mount{
						ContainerPath: "/container_dir1/file2",
						HostPath:      "host_dir1/file2",
						ReadOnly:      true,
					})

					resp.Envs["key1"] = "val1"
				}
			}
			return resp, nil
		},
	}

	testManager.endpoints[resourceName2] = &MockEndpoint{
		allocateFunc: func(devs []string) (*pluginapi.AllocateResponse, error) {
			resp := new(pluginapi.AllocateResponse)
			resp.Envs = make(map[string]string)
			for _, dev := range devs {
				switch dev {
				case "dev3":
					resp.Envs["key2"] = "val2"

				case "dev4":
					resp.Envs["key2"] = "val3"
				}
			}
			return resp, nil
		},
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID: uuid.NewUUID(),
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name: string(uuid.NewUUID()),
					Resources: v1.ResourceRequirements{
						Limits: v1.ResourceList{
							v1.ResourceName(resourceName1): resourceQuantity1,
							v1.ResourceName("cpu"):         resourceQuantity1,
							v1.ResourceName(resourceName2): resourceQuantity2,
						},
					},
				},
			},
		},
	}

	podsStub.updateActivePods([]*v1.Pod{pod})
	err := testManager.Allocate(nodeInfo, &lifecycle.PodAdmitAttributes{Pod: pod})
	as.Nil(err)
	runContainerOpts := testManager.GetDeviceRunContainerOptions(pod, &pod.Spec.Containers[0])
	as.NotNil(runContainerOpts)
	as.Equal(len(runContainerOpts.Devices), 3)
	as.Equal(len(runContainerOpts.Mounts), 2)
	as.Equal(len(runContainerOpts.Envs), 2)

	// Requesting to create a pod without enough resources should fail.
	as.Equal(2, testManager.allocatedDevices[resourceName1].Len())
	failPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID: uuid.NewUUID(),
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name: string(uuid.NewUUID()),
					Resources: v1.ResourceRequirements{
						Limits: v1.ResourceList{
							v1.ResourceName(resourceName1): resourceQuantity2,
						},
					},
				},
			},
		},
	}
	err = testManager.Allocate(nodeInfo, &lifecycle.PodAdmitAttributes{Pod: failPod})
	as.NotNil(err)
	runContainerOpts2 := testManager.GetDeviceRunContainerOptions(failPod, &failPod.Spec.Containers[0])
	as.Nil(runContainerOpts2)

	// Requesting to create a new pod with a single resourceName2 should succeed.
	newPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID: uuid.NewUUID(),
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name: string(uuid.NewUUID()),
					Resources: v1.ResourceRequirements{
						Limits: v1.ResourceList{
							v1.ResourceName(resourceName2): resourceQuantity2,
						},
					},
				},
			},
		},
	}
	err = testManager.Allocate(nodeInfo, &lifecycle.PodAdmitAttributes{Pod: newPod})
	as.Nil(err)
	runContainerOpts3 := testManager.GetDeviceRunContainerOptions(newPod, &newPod.Spec.Containers[0])
	as.Equal(1, len(runContainerOpts3.Envs))

	// Requesting to create a pod that requests resourceName1 in init containers and normal containers
	// should succeed with devices allocated to init containers reallocated to normal containers.
	podWithPluginResourcesInInitContainers := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID: uuid.NewUUID(),
		},
		Spec: v1.PodSpec{
			InitContainers: []v1.Container{
				{
					Name: string(uuid.NewUUID()),
					Resources: v1.ResourceRequirements{
						Limits: v1.ResourceList{
							v1.ResourceName(resourceName1): resourceQuantity2,
						},
					},
				},
				{
					Name: string(uuid.NewUUID()),
					Resources: v1.ResourceRequirements{
						Limits: v1.ResourceList{
							v1.ResourceName(resourceName1): resourceQuantity1,
						},
					},
				},
			},
			Containers: []v1.Container{
				{
					Name: string(uuid.NewUUID()),
					Resources: v1.ResourceRequirements{
						Limits: v1.ResourceList{
							v1.ResourceName(resourceName1): resourceQuantity2,
							v1.ResourceName(resourceName2): resourceQuantity2,
						},
					},
				},
				{
					Name: string(uuid.NewUUID()),
					Resources: v1.ResourceRequirements{
						Limits: v1.ResourceList{
							v1.ResourceName(resourceName1): resourceQuantity2,
							v1.ResourceName(resourceName2): resourceQuantity2,
						},
					},
				},
			},
		},
	}
	podsStub.updateActivePods([]*v1.Pod{podWithPluginResourcesInInitContainers})
	err = testManager.Allocate(nodeInfo, &lifecycle.PodAdmitAttributes{Pod: podWithPluginResourcesInInitContainers})
	as.Nil(err)
	podUID := string(podWithPluginResourcesInInitContainers.UID)
	initCont1 := podWithPluginResourcesInInitContainers.Spec.InitContainers[0].Name
	initCont2 := podWithPluginResourcesInInitContainers.Spec.InitContainers[1].Name
	normalCont1 := podWithPluginResourcesInInitContainers.Spec.Containers[0].Name
	normalCont2 := podWithPluginResourcesInInitContainers.Spec.Containers[1].Name
	initCont1Devices := testManager.podDevices.containerDevices(podUID, initCont1, resourceName1)
	initCont2Devices := testManager.podDevices.containerDevices(podUID, initCont2, resourceName1)
	normalCont1Devices := testManager.podDevices.containerDevices(podUID, normalCont1, resourceName1)
	normalCont2Devices := testManager.podDevices.containerDevices(podUID, normalCont2, resourceName1)
	as.True(initCont2Devices.IsSuperset(initCont1Devices))
	as.True(initCont2Devices.IsSuperset(normalCont1Devices))
	as.True(initCont2Devices.IsSuperset(normalCont2Devices))
	as.Equal(0, normalCont1Devices.Intersection(normalCont2Devices).Len())
}

func TestSanitizeNodeAllocatable(t *testing.T) {
	resourceName1 := "domain1.com/resource1"
	devID1 := "dev1"

	resourceName2 := "domain2.com/resource2"
	devID2 := "dev2"

	as := assert.New(t)
	monitorCallback := func(resourceName string, added, updated, deleted []pluginapi.Device) {}

	testManager := &ManagerImpl{
		callback:         monitorCallback,
		allDevices:       make(map[string]sets.String),
		allocatedDevices: make(map[string]sets.String),
		podDevices:       make(podDevices),
	}
	// require one of resource1 and one of resource2
	testManager.allocatedDevices[resourceName1] = sets.NewString()
	testManager.allocatedDevices[resourceName1].Insert(devID1)
	testManager.allocatedDevices[resourceName2] = sets.NewString()
	testManager.allocatedDevices[resourceName2].Insert(devID2)

	cachedNode := &v1.Node{
		Status: v1.NodeStatus{
			Allocatable: v1.ResourceList{
				// has no resource1 and two of resource2
				v1.ResourceName(resourceName2): *resource.NewQuantity(int64(2), resource.DecimalSI),
			},
		},
	}
	nodeInfo := &schedulercache.NodeInfo{}
	nodeInfo.SetNode(cachedNode)

	testManager.sanitizeNodeAllocatable(nodeInfo)

	allocatableScalarResources := nodeInfo.AllocatableResource().ScalarResources
	// allocatable in nodeInfo is less than needed, should update
	as.Equal(1, int(allocatableScalarResources[v1.ResourceName(resourceName1)]))
	// allocatable in nodeInfo is more than needed, should skip updating
	as.Equal(2, int(allocatableScalarResources[v1.ResourceName(resourceName2)]))
}
