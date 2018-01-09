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

package vsphere

import (
	"context"
	"errors"
	"io/ioutil"
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/vim25"

	"fmt"

	"github.com/vmware/govmomi/vim25/mo"
	"k8s.io/api/core/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/kubernetes/pkg/cloudprovider/providers/vsphere/vclib"
	"k8s.io/kubernetes/pkg/cloudprovider/providers/vsphere/vclib/diskmanagers"
	"path/filepath"
)

const (
	DatastoreProperty     = "datastore"
	DatastoreInfoProperty = "info"
	Folder                = "Folder"
	VirtualMachine        = "VirtualMachine"
	DummyDiskName         = "kube-dummyDisk.vmdk"
)

// GetVSphere reads vSphere configuration from system environment and construct vSphere object
func GetVSphere() (*VSphere, error) {
	cfg := getVSphereConfig()
	vSphereConn := getVSphereConn(cfg)
	client, err := GetgovmomiClient(vSphereConn)
	if err != nil {
		return nil, err
	}
	vSphereConn.GoVmomiClient = client
	vsphereIns := &VSphereInstance{
		conn: vSphereConn,
		cfg: &VirtualCenterConfig{
			User:              cfg.Global.User,
			Password:          cfg.Global.Password,
			VCenterPort:       cfg.Global.VCenterPort,
			Datacenters:       cfg.Global.Datacenters,
			RoundTripperCount: cfg.Global.RoundTripperCount,
		},
	}
	vsphereInsMap := make(map[string]*VSphereInstance)
	vsphereInsMap[cfg.Global.VCenterIP] = vsphereIns
	// TODO: Initialize nodeManager and set it in VSphere.
	vs := &VSphere{
		vsphereInstanceMap: vsphereInsMap,
		hostName:           "",
		cfg:                cfg,
		nodeManager: &NodeManager{
			vsphereInstanceMap: vsphereInsMap,
			nodeInfoMap:        make(map[string]*NodeInfo),
			registeredNodes:    make(map[string]*v1.Node),
		},
	}
	runtime.SetFinalizer(vs, logout)
	return vs, nil
}

func getVSphereConfig() *VSphereConfig {
	var cfg VSphereConfig
	cfg.Global.VCenterIP = os.Getenv("VSPHERE_VCENTER")
	cfg.Global.VCenterPort = os.Getenv("VSPHERE_VCENTER_PORT")
	cfg.Global.User = os.Getenv("VSPHERE_USER")
	cfg.Global.Password = os.Getenv("VSPHERE_PASSWORD")
	cfg.Global.Datacenters = os.Getenv("VSPHERE_DATACENTER")
	cfg.Global.DefaultDatastore = os.Getenv("VSPHERE_DATASTORE")
	cfg.Global.WorkingDir = os.Getenv("VSPHERE_WORKING_DIR")
	cfg.Global.VMName = os.Getenv("VSPHERE_VM_NAME")
	cfg.Global.InsecureFlag = false
	if strings.ToLower(os.Getenv("VSPHERE_INSECURE")) == "true" {
		cfg.Global.InsecureFlag = true
	}
	cfg.Workspace.VCenterIP = cfg.Global.VCenterIP
	cfg.Workspace.Datacenter = cfg.Global.Datacenters
	cfg.Workspace.DefaultDatastore = cfg.Global.DefaultDatastore
	cfg.Workspace.Folder = cfg.Global.WorkingDir
	return &cfg
}

func getVSphereConn(cfg *VSphereConfig) *vclib.VSphereConnection {
	vSphereConn := &vclib.VSphereConnection{
		Username:          cfg.Global.User,
		Password:          cfg.Global.Password,
		Hostname:          cfg.Global.VCenterIP,
		Insecure:          cfg.Global.InsecureFlag,
		RoundTripperCount: cfg.Global.RoundTripperCount,
		Port:              cfg.Global.VCenterPort,
	}
	return vSphereConn
}

// GetgovmomiClient gets the goVMOMI client for the vsphere connection object
func GetgovmomiClient(conn *vclib.VSphereConnection) (*govmomi.Client, error) {
	if conn == nil {
		cfg := getVSphereConfig()
		conn = getVSphereConn(cfg)
	}
	client, err := conn.NewClient(context.TODO())
	return client, err
}

// getvmUUID gets the BIOS UUID via the sys interface.  This UUID is known by vsphere
func getvmUUID() (string, error) {
	id, err := ioutil.ReadFile(UUIDPath)
	if err != nil {
		return "", fmt.Errorf("error retrieving vm uuid: %s", err)
	}
	uuidFromFile := string(id[:])
	//strip leading and trailing white space and new line char
	uuid := strings.TrimSpace(uuidFromFile)
	// check the uuid starts with "VMware-"
	if !strings.HasPrefix(uuid, UUIDPrefix) {
		return "", fmt.Errorf("Failed to match Prefix, UUID read from the file is %v", uuidFromFile)
	}
	// Strip the prefix and while spaces and -
	uuid = strings.Replace(uuid[len(UUIDPrefix):(len(uuid))], " ", "", -1)
	uuid = strings.Replace(uuid, "-", "", -1)
	if len(uuid) != 32 {
		return "", fmt.Errorf("Length check failed, UUID read from the file is %v", uuidFromFile)
	}
	// need to add dashes, e.g. "564d395e-d807-e18a-cb25-b79f65eb2b9f"
	uuid = fmt.Sprintf("%s-%s-%s-%s-%s", uuid[0:8], uuid[8:12], uuid[12:16], uuid[16:20], uuid[20:32])
	return uuid, nil
}

// Returns the accessible datastores for the given node VM.
func getAccessibleDatastores(ctx context.Context, nodeVmDetail *NodeDetails, nodeManager *NodeManager) ([]*vclib.DatastoreInfo, error) {
	accessibleDatastores, err := nodeVmDetail.vm.GetAllAccessibleDatastores(ctx)
	if err != nil {
		// Check if the node VM is not found which indicates that the node info in the node manager is stale.
		// If so, rediscover the node and retry.
		if vclib.IsManagedObjectNotFoundError(err) {
			glog.V(4).Infof("error %q ManagedObjectNotFound for node %q. Rediscovering...", err, nodeVmDetail.NodeName)
			err = nodeManager.RediscoverNode(convertToK8sType(nodeVmDetail.NodeName))
			if err == nil {
				glog.V(4).Infof("Discovered node %s successfully", nodeVmDetail.NodeName)
				nodeInfo, err := nodeManager.GetNodeInfo(convertToK8sType(nodeVmDetail.NodeName))
				if err != nil {
					glog.V(4).Infof("error %q getting node info for node %+v", err, nodeVmDetail)
					return nil, err
				}

				accessibleDatastores, err = nodeInfo.vm.GetAllAccessibleDatastores(ctx)
				if err != nil {
					glog.V(4).Infof("error %q getting accessible datastores for node %+v", err, nodeVmDetail)
					return nil, err
				}
			} else {
				glog.V(4).Infof("error %q rediscovering node %+v", err, nodeVmDetail)
				return nil, err
			}
		} else {
			glog.V(4).Infof("error %q getting accessible datastores for node %+v", err, nodeVmDetail)
			return nil, err
		}
	}
	return accessibleDatastores, nil
}

// Get all datastores accessible for the virtual machine object.
func getSharedDatastoresInK8SCluster(ctx context.Context, dc *vclib.Datacenter, nodeManager *NodeManager) ([]*vclib.DatastoreInfo, error) {
	nodeVmDetails := nodeManager.GetNodeDetails()
	if nodeVmDetails == nil || len(nodeVmDetails) == 0 {
		msg := fmt.Sprintf("Kubernetes node nodeVmDetail details is empty. nodeVmDetails : %+v", nodeVmDetails)
		glog.Error(msg)
		return nil, fmt.Errorf(msg)
	}
	var sharedDatastores []*vclib.DatastoreInfo
	for index, nodeVmDetail := range nodeVmDetails {
		glog.V(9).Infof("Getting accessible datastores for node %s", nodeVmDetail.NodeName)
		accessibleDatastores, err := getAccessibleDatastores(ctx, &nodeVmDetail, nodeManager)
		if err != nil {
			return nil, err
		}
		if index == 0 {
			sharedDatastores = accessibleDatastores
		} else {
			sharedDatastores = intersect(sharedDatastores, accessibleDatastores)
			if len(sharedDatastores) == 0 {
				return nil, fmt.Errorf("No shared datastores found in the Kubernetes cluster for nodeVmDetails: %+v", nodeVmDetails)
			}
		}
	}
	glog.V(9).Infof("sharedDatastores : %+v", sharedDatastores)
	sharedDatastores, err := getDatastoresForEndpointVC(ctx, dc, sharedDatastores)
	if err != nil {
		glog.Errorf("Failed to get shared datastores from endpoint VC. err: %+v", err)
		return nil, err
	}
	glog.V(9).Infof("sharedDatastores at endpoint VC: %+v", sharedDatastores)
	return sharedDatastores, nil
}

func intersect(list1 []*vclib.DatastoreInfo, list2 []*vclib.DatastoreInfo) []*vclib.DatastoreInfo {
	glog.V(9).Infof("list1: %+v", list1)
	glog.V(9).Infof("list2: %+v", list2)
	var sharedDs []*vclib.DatastoreInfo
	for _, val1 := range list1 {
		// Check if val1 is found in list2
		for _, val2 := range list2 {
			// Intersection is performed based on the datastoreUrl as this uniquely identifies the datastore.
			if val1.Info.Url == val2.Info.Url {
				sharedDs = append(sharedDs, val1)
				break
			}
		}
	}
	return sharedDs
}

// getMostFreeDatastore gets the best fit compatible datastore by free space.
func getMostFreeDatastoreName(ctx context.Context, client *vim25.Client, dsInfoList []*vclib.DatastoreInfo) (string, error) {
	var curMax int64
	curMax = -1
	var index int
	for i, dsInfo := range dsInfoList {
		dsFreeSpace := dsInfo.Info.GetDatastoreInfo().FreeSpace
		if dsFreeSpace > curMax {
			curMax = dsFreeSpace
			index = i
		}
	}
	return dsInfoList[index].Info.GetDatastoreInfo().Name, nil
}

// Returns the datastores in the given datacenter by performing lookup based on datastore URL.
func getDatastoresForEndpointVC(ctx context.Context, dc *vclib.Datacenter, sharedDsInfos []*vclib.DatastoreInfo) ([]*vclib.DatastoreInfo, error) {
	var datastores []*vclib.DatastoreInfo
	allDsInfoMap, err := dc.GetAllDatastores(ctx)
	if err != nil {
		return nil, err
	}
	for _, sharedDsInfo := range sharedDsInfos {
		dsInfo, ok := allDsInfoMap[sharedDsInfo.Info.Url]
		if ok {
			datastores = append(datastores, dsInfo)
		} else {
			glog.V(4).Infof("Warning: Shared datastore with URL %s does not exist in endpoint VC", sharedDsInfo.Info.Url)
		}
	}
	glog.V(9).Infof("Datastore from endpoint VC: %+v", datastores)
	return datastores, nil
}

func getPbmCompatibleDatastore(ctx context.Context, dc *vclib.Datacenter, storagePolicyName string, nodeManager *NodeManager) (string, error) {
	pbmClient, err := vclib.NewPbmClient(ctx, dc.Client())
	if err != nil {
		return "", err
	}
	storagePolicyID, err := pbmClient.ProfileIDByName(ctx, storagePolicyName)
	if err != nil {
		glog.Errorf("Failed to get Profile ID by name: %s. err: %+v", storagePolicyName, err)
		return "", err
	}
	sharedDs, err := getSharedDatastoresInK8SCluster(ctx, dc, nodeManager)
	if err != nil {
		glog.Errorf("Failed to get shared datastores. err: %+v", err)
		return "", err
	}
	if len(sharedDs) == 0 {
		msg := "No shared datastores found in the endpoint virtual center"
		glog.Errorf(msg)
		return "", errors.New(msg)
	}
	compatibleDatastores, _, err := pbmClient.GetCompatibleDatastores(ctx, dc, storagePolicyID, sharedDs)
	if err != nil {
		glog.Errorf("Failed to get compatible datastores from datastores : %+v with storagePolicy: %s. err: %+v",
			sharedDs, storagePolicyID, err)
		return "", err
	}
	glog.V(9).Infof("compatibleDatastores : %+v", compatibleDatastores)
	datastore, err := getMostFreeDatastoreName(ctx, dc.Client(), compatibleDatastores)
	if err != nil {
		glog.Errorf("Failed to get most free datastore from compatible datastores: %+v. err: %+v", compatibleDatastores, err)
		return "", err
	}
	glog.V(4).Infof("Most free datastore : %+s", datastore)
	return datastore, err
}

func (vs *VSphere) setVMOptions(ctx context.Context, dc *vclib.Datacenter, resourcePoolPath string) (*vclib.VMOptions, error) {
	var vmOptions vclib.VMOptions
	resourcePool, err := dc.GetResourcePool(ctx, resourcePoolPath)
	if err != nil {
		return nil, err
	}
	glog.V(9).Infof("Resource pool path %s, resourcePool %+v", resourcePoolPath, resourcePool)
	folder, err := dc.GetFolderByPath(ctx, vs.cfg.Workspace.Folder)
	if err != nil {
		return nil, err
	}
	vmOptions.VMFolder = folder
	vmOptions.VMResourcePool = resourcePool
	return &vmOptions, nil
}

// A background routine which will be responsible for deleting stale dummy VM's.
func (vs *VSphere) cleanUpDummyVMs(dummyVMPrefix string) {
	// Create context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for {
		time.Sleep(CleanUpDummyVMRoutineInterval * time.Minute)
		vsi, err := vs.getVSphereInstanceForServer(vs.cfg.Workspace.VCenterIP, ctx)
		if err != nil {
			glog.V(4).Infof("Failed to get VSphere instance with err: %+v. Retrying again...", err)
			continue
		}
		dc, err := vclib.GetDatacenter(ctx, vsi.conn, vs.cfg.Workspace.Datacenter)
		if err != nil {
			glog.V(4).Infof("Failed to get the datacenter: %s from VC. err: %+v", vs.cfg.Workspace.Datacenter, err)
			continue
		}
		// Get the folder reference for global working directory where the dummy VM needs to be created.
		vmFolder, err := dc.GetFolderByPath(ctx, vs.cfg.Workspace.Folder)
		if err != nil {
			glog.V(4).Infof("Unable to get the kubernetes folder: %q reference. err: %+v", vs.cfg.Workspace.Folder, err)
			continue
		}
		// A write lock is acquired to make sure the cleanUp routine doesn't delete any VM's created by ongoing PVC requests.
		defer cleanUpDummyVMLock.Lock()
		err = diskmanagers.CleanUpDummyVMs(ctx, vmFolder, dc)
		if err != nil {
			glog.V(4).Infof("Unable to clean up dummy VM's in the kubernetes cluster: %q. err: %+v", vs.cfg.Workspace.Folder, err)
		}
	}
}

// Get canonical volume path for volume Path.
// Example1: The canonical path for volume path - [vsanDatastore] kubevols/volume.vmdk will be [vsanDatastore] 25d8b159-948c-4b73-e499-02001ad1b044/volume.vmdk
// Example2: The canonical path for volume path - [vsanDatastore] 25d8b159-948c-4b73-e499-02001ad1b044/volume.vmdk will be same as volume Path.
func getcanonicalVolumePath(ctx context.Context, dc *vclib.Datacenter, volumePath string) (string, error) {
	var folderID string
	var folderExists bool
	canonicalVolumePath := volumePath
	dsPathObj, err := vclib.GetDatastorePathObjFromVMDiskPath(volumePath)
	if err != nil {
		return "", err
	}
	dsPath := strings.Split(strings.TrimSpace(dsPathObj.Path), "/")
	if len(dsPath) <= 1 {
		return canonicalVolumePath, nil
	}
	datastore := dsPathObj.Datastore
	dsFolder := dsPath[0]
	folderNameIDMap, datastoreExists := datastoreFolderIDMap[datastore]
	if datastoreExists {
		folderID, folderExists = folderNameIDMap[dsFolder]
	}
	// Get the datastore folder ID if datastore or folder doesn't exist in datastoreFolderIDMap
	if !datastoreExists || !folderExists {
		if !vclib.IsValidUUID(dsFolder) {
			dummyDiskVolPath := "[" + datastore + "] " + dsFolder + "/" + DummyDiskName
			// Querying a non-existent dummy disk on the datastore folder.
			// It would fail and return an folder ID in the error message.
			_, err := dc.GetVirtualDiskPage83Data(ctx, dummyDiskVolPath)
			if err != nil {
				re := regexp.MustCompile("File (.*?) was not found")
				match := re.FindStringSubmatch(err.Error())
				canonicalVolumePath = match[1]
			}
		}
		diskPath := vclib.GetPathFromVMDiskPath(canonicalVolumePath)
		if diskPath == "" {
			return "", fmt.Errorf("Failed to parse canonicalVolumePath: %s in getcanonicalVolumePath method", canonicalVolumePath)
		}
		folderID = strings.Split(strings.TrimSpace(diskPath), "/")[0]
		setdatastoreFolderIDMap(datastoreFolderIDMap, datastore, dsFolder, folderID)
	}
	canonicalVolumePath = strings.Replace(volumePath, dsFolder, folderID, 1)
	return canonicalVolumePath, nil
}

func setdatastoreFolderIDMap(
	datastoreFolderIDMap map[string]map[string]string,
	datastore string,
	folderName string,
	folderID string) {
	folderNameIDMap := datastoreFolderIDMap[datastore]
	if folderNameIDMap == nil {
		folderNameIDMap = make(map[string]string)
		datastoreFolderIDMap[datastore] = folderNameIDMap
	}
	folderNameIDMap[folderName] = folderID
}

func convertVolPathToDevicePath(ctx context.Context, dc *vclib.Datacenter, volPath string) (string, error) {
	volPath = vclib.RemoveStorageClusterORFolderNameFromVDiskPath(volPath)
	// Get the canonical volume path for volPath.
	canonicalVolumePath, err := getcanonicalVolumePath(ctx, dc, volPath)
	if err != nil {
		glog.Errorf("Failed to get canonical vsphere volume path for volume: %s. err: %+v", volPath, err)
		return "", err
	}
	// Check if the volume path contains .vmdk extension. If not, add the extension and update the nodeVolumes Map
	if len(canonicalVolumePath) > 0 && filepath.Ext(canonicalVolumePath) != ".vmdk" {
		canonicalVolumePath += ".vmdk"
	}
	return canonicalVolumePath, nil
}

// convertVolPathsToDevicePaths removes cluster or folder path from volPaths and convert to canonicalPath
func (vs *VSphere) convertVolPathsToDevicePaths(ctx context.Context, nodeVolumes map[k8stypes.NodeName][]string) (map[k8stypes.NodeName][]string, error) {
	vmVolumes := make(map[k8stypes.NodeName][]string)
	for nodeName, volPaths := range nodeVolumes {
		nodeInfo, err := vs.nodeManager.GetNodeInfo(nodeName)
		if err != nil {
			return nil, err
		}

		_, err = vs.getVSphereInstanceForServer(nodeInfo.vcServer, ctx)
		if err != nil {
			return nil, err
		}

		for i, volPath := range volPaths {
			deviceVolPath, err := convertVolPathToDevicePath(ctx, nodeInfo.dataCenter, volPath)
			if err != nil {
				glog.Errorf("Failed to convert vsphere volume path %s to device path for volume %s. err: %+v", volPath, deviceVolPath, err)
				return nil, err
			}
			volPaths[i] = deviceVolPath
		}
		vmVolumes[nodeName] = volPaths
	}
	return vmVolumes, nil
}

// checkDiskAttached verifies volumes are attached to the VMs which are in same vCenter and Datacenter
// Returns nodes if exist any for which VM is not found in that vCenter and Datacenter
func (vs *VSphere) checkDiskAttached(ctx context.Context, nodes []k8stypes.NodeName, nodeVolumes map[k8stypes.NodeName][]string, attached map[string]map[string]bool, retry bool) ([]k8stypes.NodeName, error) {
	var nodesToRetry []k8stypes.NodeName
	var vmList []*vclib.VirtualMachine
	var nodeInfo NodeInfo
	var err error

	for _, nodeName := range nodes {
		nodeInfo, err = vs.nodeManager.GetNodeInfo(nodeName)
		if err != nil {
			return nodesToRetry, err
		}
		vmList = append(vmList, nodeInfo.vm)
	}

	// Making sure session is valid
	_, err = vs.getVSphereInstanceForServer(nodeInfo.vcServer, ctx)
	if err != nil {
		return nodesToRetry, err
	}

	// If any of the nodes are not present property collector query will fail for entire operation
	vmMoList, err := nodeInfo.dataCenter.GetVMMoList(ctx, vmList, []string{"config.hardware.device", "name", "config.uuid"})
	if err != nil {
		if vclib.IsManagedObjectNotFoundError(err) && !retry {
			glog.V(4).Infof("checkDiskAttached: ManagedObjectNotFound for property collector query for nodes: %+v vms: %+v", nodes, vmList)
			// Property Collector Query failed
			// VerifyVolumePaths per VM
			for _, nodeName := range nodes {
				nodeInfo, err := vs.nodeManager.GetNodeInfo(nodeName)
				if err != nil {
					return nodesToRetry, err
				}
				devices, err := nodeInfo.vm.VirtualMachine.Device(ctx)
				if err != nil {
					if vclib.IsManagedObjectNotFoundError(err) {
						glog.V(4).Infof("checkDiskAttached: ManagedObjectNotFound for Kubernetes node: %s with vSphere Virtual Machine reference: %v", nodeName, nodeInfo.vm)
						nodesToRetry = append(nodesToRetry, nodeName)
						continue
					}
					return nodesToRetry, err
				}
				glog.V(4).Infof("Verifying Volume Paths by devices for node %s and VM %s", nodeName, nodeInfo.vm)
				vclib.VerifyVolumePathsForVMDevices(devices, nodeVolumes[nodeName], convertToString(nodeName), attached)
			}
		}
		return nodesToRetry, err
	}

	vmMoMap := make(map[string]mo.VirtualMachine)
	for _, vmMo := range vmMoList {
		if vmMo.Config == nil {
			glog.Errorf("Config is not available for VM: %q", vmMo.Name)
			continue
		}
		glog.V(9).Infof("vmMoMap vmname: %q vmuuid: %s", vmMo.Name, strings.ToLower(vmMo.Config.Uuid))
		vmMoMap[strings.ToLower(vmMo.Config.Uuid)] = vmMo
	}

	glog.V(9).Infof("vmMoMap: +%v", vmMoMap)

	for _, nodeName := range nodes {
		node, err := vs.nodeManager.GetNode(nodeName)
		if err != nil {
			return nodesToRetry, err
		}
		glog.V(9).Infof("Verifying volume for nodeName: %q with nodeuuid: %s", nodeName, node.Status.NodeInfo.SystemUUID, vmMoMap)
		vclib.VerifyVolumePathsForVM(vmMoMap[strings.ToLower(node.Status.NodeInfo.SystemUUID)], nodeVolumes[nodeName], convertToString(nodeName), attached)
	}
	return nodesToRetry, nil
}
