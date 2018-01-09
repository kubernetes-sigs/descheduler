/*
Copyright 2016 The Kubernetes Authors.

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

package openstack

import (
	"fmt"
	"io/ioutil"
	"path"
	"path/filepath"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	k8s_volume "k8s.io/kubernetes/pkg/volume"

	"github.com/gophercloud/gophercloud"
	volumeexpand "github.com/gophercloud/gophercloud/openstack/blockstorage/extensions/volumeactions"
	volumes_v1 "github.com/gophercloud/gophercloud/openstack/blockstorage/v1/volumes"
	volumes_v2 "github.com/gophercloud/gophercloud/openstack/blockstorage/v2/volumes"
	volumes_v3 "github.com/gophercloud/gophercloud/openstack/blockstorage/v3/volumes"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/volumeattach"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/golang/glog"
)

type volumeService interface {
	createVolume(opts VolumeCreateOpts) (string, string, error)
	getVolume(volumeID string) (Volume, error)
	deleteVolume(volumeName string) error
	expandVolume(volumeID string, newSize int) error
}

// Volumes implementation for v1
type VolumesV1 struct {
	blockstorage *gophercloud.ServiceClient
	opts         BlockStorageOpts
}

// Volumes implementation for v2
type VolumesV2 struct {
	blockstorage *gophercloud.ServiceClient
	opts         BlockStorageOpts
}

// Volumes implementation for v3
type VolumesV3 struct {
	blockstorage *gophercloud.ServiceClient
	opts         BlockStorageOpts
}

type Volume struct {
	// ID of the instance, to which this volume is attached. "" if not attached
	AttachedServerId string
	// Device file path
	AttachedDevice string
	// Unique identifier for the volume.
	ID string
	// Human-readable display name for the volume.
	Name string
	// Current status of the volume.
	Status string
	// Volume size in GB
	Size int
}

type VolumeCreateOpts struct {
	Size         int
	Availability string
	Name         string
	VolumeType   string
	Metadata     map[string]string
}

const (
	VolumeAvailableStatus = "available"
	VolumeInUseStatus     = "in-use"
	VolumeDeletedStatus   = "deleted"
	VolumeErrorStatus     = "error"

	// On some environments, we need to query the metadata service in order
	// to locate disks. We'll use the Newton version, which includes device
	// metadata.
	NewtonMetadataVersion = "2016-06-30"
)

func (volumes *VolumesV1) createVolume(opts VolumeCreateOpts) (string, string, error) {
	startTime := time.Now()

	create_opts := volumes_v1.CreateOpts{
		Name:             opts.Name,
		Size:             opts.Size,
		VolumeType:       opts.VolumeType,
		AvailabilityZone: opts.Availability,
		Metadata:         opts.Metadata,
	}

	vol, err := volumes_v1.Create(volumes.blockstorage, create_opts).Extract()
	timeTaken := time.Since(startTime).Seconds()
	recordOpenstackOperationMetric("create_v1_volume", timeTaken, err)
	if err != nil {
		return "", "", err
	}
	return vol.ID, vol.AvailabilityZone, nil
}

func (volumes *VolumesV2) createVolume(opts VolumeCreateOpts) (string, string, error) {
	startTime := time.Now()

	create_opts := volumes_v2.CreateOpts{
		Name:             opts.Name,
		Size:             opts.Size,
		VolumeType:       opts.VolumeType,
		AvailabilityZone: opts.Availability,
		Metadata:         opts.Metadata,
	}

	vol, err := volumes_v2.Create(volumes.blockstorage, create_opts).Extract()
	timeTaken := time.Since(startTime).Seconds()
	recordOpenstackOperationMetric("create_v2_volume", timeTaken, err)
	if err != nil {
		return "", "", err
	}
	return vol.ID, vol.AvailabilityZone, nil
}

func (volumes *VolumesV3) createVolume(opts VolumeCreateOpts) (string, string, error) {
	startTime := time.Now()

	create_opts := volumes_v3.CreateOpts{
		Name:             opts.Name,
		Size:             opts.Size,
		VolumeType:       opts.VolumeType,
		AvailabilityZone: opts.Availability,
		Metadata:         opts.Metadata,
	}

	vol, err := volumes_v3.Create(volumes.blockstorage, create_opts).Extract()
	timeTaken := time.Since(startTime).Seconds()
	recordOpenstackOperationMetric("create_v3_volume", timeTaken, err)
	if err != nil {
		return "", "", err
	}
	return vol.ID, vol.AvailabilityZone, nil
}

func (volumes *VolumesV1) getVolume(volumeID string) (Volume, error) {
	startTime := time.Now()
	volumeV1, err := volumes_v1.Get(volumes.blockstorage, volumeID).Extract()
	timeTaken := time.Since(startTime).Seconds()
	recordOpenstackOperationMetric("get_v1_volume", timeTaken, err)
	if err != nil {
		return Volume{}, fmt.Errorf("error occurred getting volume by ID: %s, err: %v", volumeID, err)
	}

	volume := Volume{
		ID:     volumeV1.ID,
		Name:   volumeV1.Name,
		Status: volumeV1.Status,
		Size:   volumeV1.Size,
	}

	if len(volumeV1.Attachments) > 0 && volumeV1.Attachments[0]["server_id"] != nil {
		volume.AttachedServerId = volumeV1.Attachments[0]["server_id"].(string)
		volume.AttachedDevice = volumeV1.Attachments[0]["device"].(string)
	}

	return volume, nil
}

func (volumes *VolumesV2) getVolume(volumeID string) (Volume, error) {
	startTime := time.Now()
	volumeV2, err := volumes_v2.Get(volumes.blockstorage, volumeID).Extract()
	timeTaken := time.Since(startTime).Seconds()
	recordOpenstackOperationMetric("get_v2_volume", timeTaken, err)
	if err != nil {
		return Volume{}, fmt.Errorf("error occurred getting volume by ID: %s, err: %v", volumeID, err)
	}

	volume := Volume{
		ID:     volumeV2.ID,
		Name:   volumeV2.Name,
		Status: volumeV2.Status,
		Size:   volumeV2.Size,
	}

	if len(volumeV2.Attachments) > 0 {
		volume.AttachedServerId = volumeV2.Attachments[0].ServerID
		volume.AttachedDevice = volumeV2.Attachments[0].Device
	}

	return volume, nil
}

func (volumes *VolumesV3) getVolume(volumeID string) (Volume, error) {
	startTime := time.Now()
	volumeV3, err := volumes_v3.Get(volumes.blockstorage, volumeID).Extract()
	timeTaken := time.Since(startTime).Seconds()
	recordOpenstackOperationMetric("get_v3_volume", timeTaken, err)
	if err != nil {
		return Volume{}, fmt.Errorf("error occurred getting volume by ID: %s, err: %v", volumeID, err)
	}

	volume := Volume{
		ID:     volumeV3.ID,
		Name:   volumeV3.Name,
		Status: volumeV3.Status,
	}

	if len(volumeV3.Attachments) > 0 {
		volume.AttachedServerId = volumeV3.Attachments[0].ServerID
		volume.AttachedDevice = volumeV3.Attachments[0].Device
	}

	return volume, nil
}

func (volumes *VolumesV1) deleteVolume(volumeID string) error {
	startTime := time.Now()
	err := volumes_v1.Delete(volumes.blockstorage, volumeID).ExtractErr()
	timeTaken := time.Since(startTime).Seconds()
	recordOpenstackOperationMetric("delete_v1_volume", timeTaken, err)
	return err
}

func (volumes *VolumesV2) deleteVolume(volumeID string) error {
	startTime := time.Now()
	err := volumes_v2.Delete(volumes.blockstorage, volumeID).ExtractErr()
	timeTaken := time.Since(startTime).Seconds()
	recordOpenstackOperationMetric("delete_v2_volume", timeTaken, err)
	return err
}

func (volumes *VolumesV3) deleteVolume(volumeID string) error {
	startTime := time.Now()
	err := volumes_v3.Delete(volumes.blockstorage, volumeID).ExtractErr()
	timeTaken := time.Since(startTime).Seconds()
	recordOpenstackOperationMetric("delete_v3_volume", timeTaken, err)
	return err
}

func (volumes *VolumesV1) expandVolume(volumeID string, newSize int) error {
	startTime := time.Now()
	create_opts := volumeexpand.ExtendSizeOpts{
		NewSize: newSize,
	}
	err := volumeexpand.ExtendSize(volumes.blockstorage, volumeID, create_opts).ExtractErr()
	timeTaken := time.Since(startTime).Seconds()
	recordOpenstackOperationMetric("expand_volume", timeTaken, err)
	return err
}

func (volumes *VolumesV2) expandVolume(volumeID string, newSize int) error {
	startTime := time.Now()
	create_opts := volumeexpand.ExtendSizeOpts{
		NewSize: newSize,
	}
	err := volumeexpand.ExtendSize(volumes.blockstorage, volumeID, create_opts).ExtractErr()
	timeTaken := time.Since(startTime).Seconds()
	recordOpenstackOperationMetric("expand_volume", timeTaken, err)
	return err
}

func (volumes *VolumesV3) expandVolume(volumeID string, newSize int) error {
	startTime := time.Now()
	create_opts := volumeexpand.ExtendSizeOpts{
		NewSize: newSize,
	}
	err := volumeexpand.ExtendSize(volumes.blockstorage, volumeID, create_opts).ExtractErr()
	timeTaken := time.Since(startTime).Seconds()
	recordOpenstackOperationMetric("expand_volume", timeTaken, err)
	return err
}

func (os *OpenStack) OperationPending(diskName string) (bool, string, error) {
	volume, err := os.getVolume(diskName)
	if err != nil {
		return false, "", err
	}
	volumeStatus := volume.Status
	if volumeStatus == VolumeErrorStatus {
		return false, volumeStatus, nil
	}
	if volumeStatus == VolumeAvailableStatus || volumeStatus == VolumeInUseStatus || volumeStatus == VolumeDeletedStatus {
		return false, volume.Status, nil
	}
	return true, volumeStatus, nil
}

// AttachDisk attaches given cinder volume to the compute running kubelet
func (os *OpenStack) AttachDisk(instanceID, volumeID string) (string, error) {
	volume, err := os.getVolume(volumeID)
	if err != nil {
		return "", err
	}

	cClient, err := os.NewComputeV2()
	if err != nil {
		return "", err
	}

	if volume.AttachedServerId != "" {
		if instanceID == volume.AttachedServerId {
			glog.V(4).Infof("Disk %s is already attached to instance %s", volumeID, instanceID)
			return volume.ID, nil
		}
		return "", fmt.Errorf("disk %s is attached to a different instance (%s)", volumeID, volume.AttachedServerId)
	}

	startTime := time.Now()
	// add read only flag here if possible spothanis
	_, err = volumeattach.Create(cClient, instanceID, &volumeattach.CreateOpts{
		VolumeID: volume.ID,
	}).Extract()
	timeTaken := time.Since(startTime).Seconds()
	recordOpenstackOperationMetric("attach_disk", timeTaken, err)
	if err != nil {
		return "", fmt.Errorf("failed to attach %s volume to %s compute: %v", volumeID, instanceID, err)
	}
	glog.V(2).Infof("Successfully attached %s volume to %s compute", volumeID, instanceID)
	return volume.ID, nil
}

// DetachDisk detaches given cinder volume from the compute running kubelet
func (os *OpenStack) DetachDisk(instanceID, volumeID string) error {
	volume, err := os.getVolume(volumeID)
	if err != nil {
		return err
	}
	if volume.Status == VolumeAvailableStatus {
		// "available" is fine since that means the volume is detached from instance already.
		glog.V(2).Infof("volume: %s has been detached from compute: %s ", volume.ID, instanceID)
		return nil
	}

	if volume.Status != VolumeInUseStatus {
		return fmt.Errorf("can not detach volume %s, its status is %s", volume.Name, volume.Status)
	}
	cClient, err := os.NewComputeV2()
	if err != nil {
		return err
	}
	if volume.AttachedServerId != instanceID {
		return fmt.Errorf("disk: %s has no attachments or is not attached to compute: %s", volume.Name, instanceID)
	} else {
		startTime := time.Now()
		// This is a blocking call and effects kubelet's performance directly.
		// We should consider kicking it out into a separate routine, if it is bad.
		err = volumeattach.Delete(cClient, instanceID, volume.ID).ExtractErr()
		timeTaken := time.Since(startTime).Seconds()
		recordOpenstackOperationMetric("detach_disk", timeTaken, err)
		if err != nil {
			return fmt.Errorf("failed to delete volume %s from compute %s attached %v", volume.ID, instanceID, err)
		}
		glog.V(2).Infof("Successfully detached volume: %s from compute: %s", volume.ID, instanceID)
	}

	return nil
}

// ExpandVolume expands the size of specific cinder volume (in GiB)
func (os *OpenStack) ExpandVolume(volumeID string, oldSize resource.Quantity, newSize resource.Quantity) (resource.Quantity, error) {
	volume, err := os.getVolume(volumeID)
	if err != nil {
		return oldSize, err
	}
	if volume.Status != VolumeAvailableStatus {
		// cinder volume can not be expanded if its status is not available
		return oldSize, fmt.Errorf("volume status is not available")
	}

	volSizeBytes := newSize.Value()
	// Cinder works with gigabytes, convert to GiB with rounding up
	volSizeGB := int(k8s_volume.RoundUpSize(volSizeBytes, 1024*1024*1024))
	newSizeQuant := resource.MustParse(fmt.Sprintf("%dGi", volSizeGB))

	// if volume size equals to or greater than the newSize, return nil
	if volume.Size >= volSizeGB {
		return newSizeQuant, nil
	}

	volumes, err := os.volumeService("")
	if err != nil {
		return oldSize, err
	}

	err = volumes.expandVolume(volumeID, volSizeGB)
	if err != nil {
		return oldSize, err
	}
	return newSizeQuant, nil
}

// getVolume retrieves Volume by its ID.
func (os *OpenStack) getVolume(volumeID string) (Volume, error) {
	volumes, err := os.volumeService("")
	if err != nil {
		return Volume{}, fmt.Errorf("unable to initialize cinder client for region: %s, err: %v", os.region, err)
	}
	return volumes.getVolume(volumeID)
}

// CreateVolume creates a volume of given size (in GiB)
func (os *OpenStack) CreateVolume(name string, size int, vtype, availability string, tags *map[string]string) (string, string, bool, error) {
	volumes, err := os.volumeService("")
	if err != nil {
		return "", "", os.bsOpts.IgnoreVolumeAZ, fmt.Errorf("unable to initialize cinder client for region: %s, err: %v", os.region, err)
	}

	opts := VolumeCreateOpts{
		Name:         name,
		Size:         size,
		VolumeType:   vtype,
		Availability: availability,
	}
	if tags != nil {
		opts.Metadata = *tags
	}

	volumeID, volumeAZ, err := volumes.createVolume(opts)

	if err != nil {
		return "", "", os.bsOpts.IgnoreVolumeAZ, fmt.Errorf("failed to create a %d GB volume: %v", size, err)
	}

	glog.Infof("Created volume %v in Availability Zone: %v Ignore volume AZ: %v", volumeID, volumeAZ, os.bsOpts.IgnoreVolumeAZ)
	return volumeID, volumeAZ, os.bsOpts.IgnoreVolumeAZ, nil
}

// GetDevicePath returns the path of an attached block storage volume, specified by its id.
func (os *OpenStack) GetDevicePathBySerialId(volumeID string) string {
	// Build a list of candidate device paths.
	// Certain Nova drivers will set the disk serial ID, including the Cinder volume id.
	candidateDeviceNodes := []string{
		// KVM
		fmt.Sprintf("virtio-%s", volumeID[:20]),
		// KVM virtio-scsi
		fmt.Sprintf("scsi-0QEMU_QEMU_HARDDISK_%s", volumeID[:20]),
		// ESXi
		fmt.Sprintf("wwn-0x%s", strings.Replace(volumeID, "-", "", -1)),
	}

	files, _ := ioutil.ReadDir("/dev/disk/by-id/")

	for _, f := range files {
		for _, c := range candidateDeviceNodes {
			if c == f.Name() {
				glog.V(4).Infof("Found disk attached as %q; full devicepath: %s\n", f.Name(), path.Join("/dev/disk/by-id/", f.Name()))
				return path.Join("/dev/disk/by-id/", f.Name())
			}
		}
	}

	glog.V(4).Infof("Failed to find device for the volumeID: %q by serial ID", volumeID)
	return ""
}

func (os *OpenStack) GetDevicePathFromInstanceMetadata(volumeID string) string {
	// Nova Hyper-V hosts cannot override disk SCSI IDs. In order to locate
	// volumes, we're querying the metadata service. Note that the Hyper-V
	// driver will include device metadata for untagged volumes as well.
	//
	// We're avoiding using cached metadata (or the configdrive),
	// relying on the metadata service.
	instanceMetadata, err := getMetadataFromMetadataService(
		NewtonMetadataVersion)

	if err != nil {
		glog.V(4).Infof(
			"Could not retrieve instance metadata. Error: %v", err)
		return ""
	}

	for _, device := range instanceMetadata.Devices {
		if device.Type == "disk" && device.Serial == volumeID {
			glog.V(4).Infof(
				"Found disk metadata for volumeID %q. Bus: %q, Address: %q",
				volumeID, device.Bus, device.Address)

			diskPattern := fmt.Sprintf(
				"/dev/disk/by-path/*-%s-%s",
				device.Bus, device.Address)
			diskPaths, err := filepath.Glob(diskPattern)
			if err != nil {
				glog.Errorf(
					"could not retrieve disk path for volumeID: %q. Error filepath.Glob(%q): %v",
					volumeID, diskPattern, err)
				return ""
			}

			if len(diskPaths) == 1 {
				return diskPaths[0]
			}

			glog.Errorf(
				"expecting to find one disk path for volumeID %q, found %d: %v",
				volumeID, len(diskPaths), diskPaths)
			return ""
		}
	}

	glog.V(4).Infof(
		"Could not retrieve device metadata for volumeID: %q", volumeID)
	return ""
}

// GetDevicePath returns the path of an attached block storage volume, specified by its id.
func (os *OpenStack) GetDevicePath(volumeID string) string {
	devicePath := os.GetDevicePathBySerialId(volumeID)

	if devicePath == "" {
		devicePath = os.GetDevicePathFromInstanceMetadata(volumeID)
	}

	if devicePath == "" {
		glog.Warningf("Failed to find device for the volumeID: %q", volumeID)
	}

	return devicePath
}

func (os *OpenStack) DeleteVolume(volumeID string) error {
	used, err := os.diskIsUsed(volumeID)
	if err != nil {
		return err
	}
	if used {
		msg := fmt.Sprintf("Cannot delete the volume %q, it's still attached to a node", volumeID)
		return k8s_volume.NewDeletedVolumeInUseError(msg)
	}

	volumes, err := os.volumeService("")
	if err != nil {
		return fmt.Errorf("unable to initialize cinder client for region: %s, err: %v", os.region, err)
	}

	err = volumes.deleteVolume(volumeID)
	return err

}

// GetAttachmentDiskPath gets device path of attached volume to the compute running kubelet, as known by cinder
func (os *OpenStack) GetAttachmentDiskPath(instanceID, volumeID string) (string, error) {
	// See issue #33128 - Cinder does not always tell you the right device path, as such
	// we must only use this value as a last resort.
	volume, err := os.getVolume(volumeID)
	if err != nil {
		return "", err
	}
	if volume.Status != VolumeInUseStatus {
		return "", fmt.Errorf("can not get device path of volume %s, its status is %s ", volume.Name, volume.Status)
	}
	if volume.AttachedServerId != "" {
		if instanceID == volume.AttachedServerId {
			// Attachment[0]["device"] points to the device path
			// see http://developer.openstack.org/api-ref-blockstorage-v1.html
			return volume.AttachedDevice, nil
		} else {
			return "", fmt.Errorf("disk %q is attached to a different compute: %q, should be detached before proceeding", volumeID, volume.AttachedServerId)
		}
	}
	return "", fmt.Errorf("volume %s has no ServerId", volumeID)
}

// DiskIsAttached queries if a volume is attached to a compute instance
func (os *OpenStack) DiskIsAttached(instanceID, volumeID string) (bool, error) {
	volume, err := os.getVolume(volumeID)
	if err != nil {
		return false, err
	}

	return instanceID == volume.AttachedServerId, nil
}

// DisksAreAttached queries if a list of volumes are attached to a compute instance
func (os *OpenStack) DisksAreAttached(instanceID string, volumeIDs []string) (map[string]bool, error) {
	attached := make(map[string]bool)
	for _, volumeID := range volumeIDs {
		isAttached, _ := os.DiskIsAttached(instanceID, volumeID)
		attached[volumeID] = isAttached
	}
	return attached, nil
}

// diskIsUsed returns true a disk is attached to any node.
func (os *OpenStack) diskIsUsed(volumeID string) (bool, error) {
	volume, err := os.getVolume(volumeID)
	if err != nil {
		return false, err
	}
	return volume.AttachedServerId != "", nil
}

// ShouldTrustDevicePath queries if we should trust the cinder provide deviceName, See issue #33128
func (os *OpenStack) ShouldTrustDevicePath() bool {
	return os.bsOpts.TrustDevicePath
}

// recordOpenstackOperationMetric records openstack operation metrics
func recordOpenstackOperationMetric(operation string, timeTaken float64, err error) {
	if err != nil {
		OpenstackApiRequestErrors.With(prometheus.Labels{"request": operation}).Inc()
	} else {
		OpenstackOperationsLatency.With(prometheus.Labels{"request": operation}).Observe(timeTaken)
	}
}
