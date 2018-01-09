/*
Copyright 2014 The Kubernetes Authors.

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

//
// utility functions to setup rbd volume
// mainly implement diskManager interface
//

package rbd

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	fileutil "k8s.io/kubernetes/pkg/util/file"
	"k8s.io/kubernetes/pkg/util/node"
	"k8s.io/kubernetes/pkg/volume"
	volutil "k8s.io/kubernetes/pkg/volume/util"
)

const (
	imageWatcherStr = "watcher="
	imageSizeStr    = "size "
	sizeDivStr      = " MB in"
	kubeLockMagic   = "kubelet_lock_magic_"
)

var (
	clientKubeLockMagicRe = regexp.MustCompile("client.* " + kubeLockMagic + ".*")
)

// search /sys/bus for rbd device that matches given pool and image
func getDevFromImageAndPool(pool, image string) (string, bool) {
	// /sys/bus/rbd/devices/X/name and /sys/bus/rbd/devices/X/pool
	sys_path := "/sys/bus/rbd/devices"
	if dirs, err := ioutil.ReadDir(sys_path); err == nil {
		for _, f := range dirs {
			// pool and name format:
			// see rbd_pool_show() and rbd_name_show() at
			// https://github.com/torvalds/linux/blob/master/drivers/block/rbd.c
			name := f.Name()
			// first match pool, then match name
			poolFile := path.Join(sys_path, name, "pool")
			poolBytes, err := ioutil.ReadFile(poolFile)
			if err != nil {
				glog.V(4).Infof("Error reading %s: %v", poolFile, err)
				continue
			}
			if strings.TrimSpace(string(poolBytes)) != pool {
				glog.V(4).Infof("Device %s is not %q: %q", name, pool, string(poolBytes))
				continue
			}
			imgFile := path.Join(sys_path, name, "name")
			imgBytes, err := ioutil.ReadFile(imgFile)
			if err != nil {
				glog.V(4).Infof("Error reading %s: %v", imgFile, err)
				continue
			}
			if strings.TrimSpace(string(imgBytes)) != image {
				glog.V(4).Infof("Device %s is not %q: %q", name, image, string(imgBytes))
				continue
			}
			// found a match, check if device exists
			devicePath := "/dev/rbd" + name
			if _, err := os.Lstat(devicePath); err == nil {
				return devicePath, true
			}
		}
	}
	return "", false
}

// stat a path, if not exists, retry maxRetries times
func waitForPath(pool, image string, maxRetries int) (string, bool) {
	for i := 0; i < maxRetries; i++ {
		devicePath, found := getDevFromImageAndPool(pool, image)
		if found {
			return devicePath, true
		}
		if i == maxRetries-1 {
			break
		}
		time.Sleep(time.Second)
	}
	return "", false
}

// make a directory like /var/lib/kubelet/plugins/kubernetes.io/pod/rbd/pool-image-image
func makePDNameInternal(host volume.VolumeHost, pool string, image string) string {
	return path.Join(host.GetPluginDir(rbdPluginName), "rbd", pool+"-image-"+image)
}

// RBDUtil implements diskManager interface.
type RBDUtil struct{}

var _ diskManager = &RBDUtil{}

func (util *RBDUtil) MakeGlobalPDName(rbd rbd) string {
	return makePDNameInternal(rbd.plugin.host, rbd.Pool, rbd.Image)
}

func rbdErrors(runErr, resultErr error) error {
	if err, ok := runErr.(*exec.Error); ok {
		if err.Err == exec.ErrNotFound {
			return fmt.Errorf("rbd: rbd cmd not found")
		}
	}
	return resultErr
}

// rbdLock acquires a lock on image if lock is true, otherwise releases if a
// lock is found on image.
func (util *RBDUtil) rbdLock(b rbdMounter, lock bool) error {
	var err error
	var output, locker string
	var cmd []byte
	var secret_opt []string

	if b.Secret != "" {
		secret_opt = []string{"--key=" + b.Secret}
	} else {
		secret_opt = []string{"-k", b.Keyring}
	}
	if len(b.adminId) == 0 {
		b.adminId = b.Id
	}
	if len(b.adminSecret) == 0 {
		b.adminSecret = b.Secret
	}

	// construct lock id using host name and a magic prefix
	lock_id := kubeLockMagic + node.GetHostname("")

	l := len(b.Mon)
	// avoid mount storm, pick a host randomly
	start := rand.Int() % l
	// iterate all hosts until mount succeeds.
	for i := start; i < start+l; i++ {
		mon := b.Mon[i%l]
		// cmd "rbd lock list" serves two purposes:
		// for fencing, check if lock already held for this host
		// this edge case happens if host crashes in the middle of acquiring lock and mounting rbd
		// for defencing, get the locker name, something like "client.1234"
		args := []string{"lock", "list", b.Image, "--pool", b.Pool, "--id", b.Id, "-m", mon}
		args = append(args, secret_opt...)
		cmd, err = b.exec.Run("rbd", args...)
		output = string(cmd)
		glog.Infof("lock list output %q", output)
		if err != nil {
			continue
		}

		if lock {
			// check if lock is already held for this host by matching lock_id and rbd lock id
			if strings.Contains(output, lock_id) {
				// this host already holds the lock, exit
				glog.V(1).Infof("rbd: lock already held for %s", lock_id)
				return nil
			}
			// clean up orphaned lock if no watcher on the image
			used, rbdOutput, statusErr := util.rbdStatus(&b)
			if statusErr != nil {
				return fmt.Errorf("rbdStatus failed error %v, rbd output: %v", statusErr, rbdOutput)
			}
			if used {
				// this image is already used by a node other than this node
				return fmt.Errorf("rbd image: %s/%s is already used by a node other than this node, rbd output: %v", b.Image, b.Pool, output)
			}

			// best effort clean up orphaned locked if not used
			locks := clientKubeLockMagicRe.FindAllStringSubmatch(output, -1)
			for _, v := range locks {
				if len(v) > 0 {
					lockInfo := strings.Split(v[0], " ")
					if len(lockInfo) > 2 {
						args := []string{"lock", "remove", b.Image, lockInfo[1], lockInfo[0], "--pool", b.Pool, "--id", b.Id, "-m", mon}
						args = append(args, secret_opt...)
						cmd, err = b.exec.Run("rbd", args...)
						glog.Infof("remove orphaned locker %s from client %s: err %v, rbd output: %s", lockInfo[1], lockInfo[0], err, string(cmd))
					}
				}
			}

			// hold a lock: rbd lock add
			args := []string{"lock", "add", b.Image, lock_id, "--pool", b.Pool, "--id", b.Id, "-m", mon}
			args = append(args, secret_opt...)
			cmd, err = b.exec.Run("rbd", args...)
			if err == nil {
				glog.V(4).Infof("rbd: successfully add lock (locker_id: %s) on image: %s/%s with id %s mon %s", lock_id, b.Pool, b.Image, b.Id, mon)
			}
		} else {
			// defencing, find locker name
			ind := strings.LastIndex(output, lock_id) - 1
			for i := ind; i >= 0; i-- {
				if output[i] == '\n' {
					locker = output[(i + 1):ind]
					break
				}
			}
			// remove a lock if found: rbd lock remove
			if len(locker) > 0 {
				args := []string{"lock", "remove", b.Image, lock_id, locker, "--pool", b.Pool, "--id", b.Id, "-m", mon}
				args = append(args, secret_opt...)
				cmd, err = b.exec.Run("rbd", args...)
				if err == nil {
					glog.V(4).Infof("rbd: successfully remove lock (locker_id: %s) on image: %s/%s with id %s mon %s", lock_id, b.Pool, b.Image, b.Id, mon)
				}
			}
		}

		if err == nil {
			// break if operation succeeds
			break
		}
	}
	return err
}

// AttachDisk attaches the disk on the node.
// If Volume is not read-only, acquire a lock on image first.
func (util *RBDUtil) AttachDisk(b rbdMounter) (string, error) {
	var err error
	var output []byte

	globalPDPath := util.MakeGlobalPDName(*b.rbd)
	if pathExists, pathErr := volutil.PathExists(globalPDPath); pathErr != nil {
		return "", fmt.Errorf("Error checking if path exists: %v", pathErr)
	} else if !pathExists {
		if err := os.MkdirAll(globalPDPath, 0750); err != nil {
			return "", err
		}
	}

	devicePath, found := waitForPath(b.Pool, b.Image, 1)
	if !found {
		_, err = b.exec.Run("modprobe", "rbd")
		if err != nil {
			glog.Warningf("rbd: failed to load rbd kernel module:%v", err)
		}

		// Currently, we don't acquire advisory lock on image, but for backward
		// compatibility, we need to check if the image is being used by nodes running old kubelet.
		found, rbdOutput, err := util.rbdStatus(&b)
		if err != nil {
			return "", fmt.Errorf("error: %v, rbd output: %v", err, rbdOutput)
		}
		if found {
			glog.Infof("rbd image %s/%s is still being used ", b.Pool, b.Image)
			return "", fmt.Errorf("rbd image %s/%s is still being used. rbd output: %s", b.Pool, b.Image, rbdOutput)
		}

		// rbd map
		l := len(b.Mon)
		// avoid mount storm, pick a host randomly
		start := rand.Int() % l
		// iterate all hosts until mount succeeds.
		for i := start; i < start+l; i++ {
			mon := b.Mon[i%l]
			glog.V(1).Infof("rbd: map mon %s", mon)
			if b.Secret != "" {
				output, err = b.exec.Run("rbd",
					"map", b.Image, "--pool", b.Pool, "--id", b.Id, "-m", mon, "--key="+b.Secret)
			} else {
				output, err = b.exec.Run("rbd",
					"map", b.Image, "--pool", b.Pool, "--id", b.Id, "-m", mon, "-k", b.Keyring)
			}
			if err == nil {
				break
			}
			glog.V(1).Infof("rbd: map error %v, rbd output: %s", err, string(output))
		}
		if err != nil {
			return "", fmt.Errorf("rbd: map failed %v, rbd output: %s", err, string(output))
		}
		devicePath, found = waitForPath(b.Pool, b.Image, 10)
		if !found {
			return "", fmt.Errorf("Could not map image %s/%s, Timeout after 10s", b.Pool, b.Image)
		}
	}
	return devicePath, nil
}

// DetachDisk detaches the disk from the node.
// It detaches device from the node if device is provided, and removes the lock
// if there is persisted RBD info under deviceMountPath.
func (util *RBDUtil) DetachDisk(plugin *rbdPlugin, deviceMountPath string, device string) error {
	if len(device) == 0 {
		return fmt.Errorf("DetachDisk failed , device is empty")
	}
	// rbd unmap
	exec := plugin.host.GetExec(plugin.GetPluginName())
	output, err := exec.Run("rbd", "unmap", device)
	if err != nil {
		return rbdErrors(err, fmt.Errorf("rbd: failed to unmap device %s, error %v, rbd output: %v", device, err, output))
	}
	glog.V(3).Infof("rbd: successfully unmap device %s", device)

	// Currently, we don't persist rbd info on the disk, but for backward
	// compatbility, we need to clean it if found.
	rbdFile := path.Join(deviceMountPath, "rbd.json")
	exists, err := fileutil.FileExists(rbdFile)
	if err != nil {
		return err
	}
	if exists {
		glog.V(3).Infof("rbd: old rbd.json is found under %s, cleaning it", deviceMountPath)
		err = util.cleanOldRBDFile(plugin, rbdFile)
		if err != nil {
			glog.Errorf("rbd: failed to clean %s", rbdFile)
			return err
		}
		glog.V(3).Infof("rbd: successfully remove %s", rbdFile)
	}
	return nil
}

// cleanOldRBDFile read rbd info from rbd.json file and removes lock if found.
// At last, it removes rbd.json file.
func (util *RBDUtil) cleanOldRBDFile(plugin *rbdPlugin, rbdFile string) error {
	mounter := &rbdMounter{
		// util.rbdLock needs it to run command.
		rbd: newRBD("", "", "", "", false, plugin, util),
	}
	fp, err := os.Open(rbdFile)
	if err != nil {
		return fmt.Errorf("rbd: open err %s/%s", rbdFile, err)
	}
	defer fp.Close()

	decoder := json.NewDecoder(fp)
	if err = decoder.Decode(mounter); err != nil {
		return fmt.Errorf("rbd: decode err: %v.", err)
	}

	if err != nil {
		glog.Errorf("failed to load rbd info from %s: %v", rbdFile, err)
		return err
	}
	// remove rbd lock if found
	// the disk is not attached to this node anymore, so the lock on image
	// for this node can be removed safely
	err = util.rbdLock(*mounter, false)
	if err == nil {
		os.Remove(rbdFile)
	}
	return err
}

func (util *RBDUtil) CreateImage(p *rbdVolumeProvisioner) (r *v1.RBDPersistentVolumeSource, size int, err error) {
	var output []byte
	capacity := p.options.PVC.Spec.Resources.Requests[v1.ResourceName(v1.ResourceStorage)]
	volSizeBytes := capacity.Value()
	// convert to MB that rbd defaults on
	sz := int(volume.RoundUpSize(volSizeBytes, 1024*1024))
	volSz := fmt.Sprintf("%d", sz)
	// rbd create
	l := len(p.rbdMounter.Mon)
	// pick a mon randomly
	start := rand.Int() % l
	// iterate all monitors until create succeeds.
	for i := start; i < start+l; i++ {
		mon := p.Mon[i%l]
		if p.rbdMounter.imageFormat == rbdImageFormat2 {
			glog.V(4).Infof("rbd: create %s size %s format %s (features: %s) using mon %s, pool %s id %s key %s", p.rbdMounter.Image, volSz, p.rbdMounter.imageFormat, p.rbdMounter.imageFeatures, mon, p.rbdMounter.Pool, p.rbdMounter.adminId, p.rbdMounter.adminSecret)
		} else {
			glog.V(4).Infof("rbd: create %s size %s format %s using mon %s, pool %s id %s key %s", p.rbdMounter.Image, volSz, p.rbdMounter.imageFormat, mon, p.rbdMounter.Pool, p.rbdMounter.adminId, p.rbdMounter.adminSecret)
		}
		args := []string{"create", p.rbdMounter.Image, "--size", volSz, "--pool", p.rbdMounter.Pool, "--id", p.rbdMounter.adminId, "-m", mon, "--key=" + p.rbdMounter.adminSecret, "--image-format", p.rbdMounter.imageFormat}
		if p.rbdMounter.imageFormat == rbdImageFormat2 {
			// if no image features is provided, it results in empty string
			// which disable all RBD image format 2 features as we expected
			features := strings.Join(p.rbdMounter.imageFeatures, ",")
			args = append(args, "--image-feature", features)
		}
		output, err = p.exec.Run("rbd", args...)
		if err == nil {
			break
		} else {
			glog.Warningf("failed to create rbd image, output %v", string(output))
		}
	}

	if err != nil {
		return nil, 0, fmt.Errorf("failed to create rbd image: %v, command output: %s", err, string(output))
	}

	return &v1.RBDPersistentVolumeSource{
		CephMonitors: p.rbdMounter.Mon,
		RBDImage:     p.rbdMounter.Image,
		RBDPool:      p.rbdMounter.Pool,
	}, sz, nil
}

func (util *RBDUtil) DeleteImage(p *rbdVolumeDeleter) error {
	var output []byte
	found, rbdOutput, err := util.rbdStatus(p.rbdMounter)
	if err != nil {
		return fmt.Errorf("error %v, rbd output: %v", err, rbdOutput)
	}
	if found {
		glog.Info("rbd is still being used ", p.rbdMounter.Image)
		return fmt.Errorf("rbd image %s/%s is still being used, rbd output: %v", p.rbdMounter.Pool, p.rbdMounter.Image, rbdOutput)
	}
	// rbd rm
	l := len(p.rbdMounter.Mon)
	// pick a mon randomly
	start := rand.Int() % l
	// iterate all monitors until rm succeeds.
	for i := start; i < start+l; i++ {
		mon := p.rbdMounter.Mon[i%l]
		glog.V(4).Infof("rbd: rm %s using mon %s, pool %s id %s key %s", p.rbdMounter.Image, mon, p.rbdMounter.Pool, p.rbdMounter.adminId, p.rbdMounter.adminSecret)
		output, err = p.exec.Run("rbd",
			"rm", p.rbdMounter.Image, "--pool", p.rbdMounter.Pool, "--id", p.rbdMounter.adminId, "-m", mon, "--key="+p.rbdMounter.adminSecret)
		if err == nil {
			return nil
		} else {
			glog.Errorf("failed to delete rbd image: %v, command output: %s", err, string(output))
		}
	}
	return fmt.Errorf("error %v, rbd output: %v", err, string(output))
}

// ExpandImage runs rbd resize command to resize the specified image
func (util *RBDUtil) ExpandImage(rbdExpander *rbdVolumeExpander, oldSize resource.Quantity, newSize resource.Quantity) (resource.Quantity, error) {
	var output []byte
	var err error
	volSizeBytes := newSize.Value()
	// convert to MB that rbd defaults on
	sz := int(volume.RoundUpSize(volSizeBytes, 1024*1024))
	newVolSz := fmt.Sprintf("%d", sz)
	newSizeQuant := resource.MustParse(fmt.Sprintf("%dMi", sz))

	// check the current size of rbd image, if equals to or greater that the new request size, do nothing
	curSize, infoErr := util.rbdInfo(rbdExpander.rbdMounter)
	if infoErr != nil {
		return oldSize, fmt.Errorf("rbd info failed, error: %v", infoErr)
	}
	if curSize >= sz {
		return newSizeQuant, nil
	}

	// rbd resize
	l := len(rbdExpander.rbdMounter.Mon)
	// pick a mon randomly
	start := rand.Int() % l
	// iterate all monitors until resize succeeds.
	for i := start; i < start+l; i++ {
		mon := rbdExpander.rbdMounter.Mon[i%l]
		glog.V(4).Infof("rbd: resize %s using mon %s, pool %s id %s key %s", rbdExpander.rbdMounter.Image, mon, rbdExpander.rbdMounter.Pool, rbdExpander.rbdMounter.adminId, rbdExpander.rbdMounter.adminSecret)
		output, err = rbdExpander.exec.Run("rbd",
			"resize", rbdExpander.rbdMounter.Image, "--size", newVolSz, "--pool", rbdExpander.rbdMounter.Pool, "--id", rbdExpander.rbdMounter.adminId, "-m", mon, "--key="+rbdExpander.rbdMounter.adminSecret)
		if err == nil {
			return newSizeQuant, nil
		} else {
			glog.Errorf("failed to resize rbd image: %v, command output: %s", err, string(output))
		}
	}
	return oldSize, err
}

// rbdInfo runs `rbd info` command to get the current image size in MB
func (util *RBDUtil) rbdInfo(b *rbdMounter) (int, error) {
	var err error
	var output string
	var cmd []byte

	// If we don't have admin id/secret (e.g. attaching), fallback to user id/secret.
	id := b.adminId
	secret := b.adminSecret
	if id == "" {
		id = b.Id
		secret = b.Secret
	}

	l := len(b.Mon)
	start := rand.Int() % l
	// iterate all hosts until rbd command succeeds.
	for i := start; i < start+l; i++ {
		mon := b.Mon[i%l]
		// cmd "rbd info" get the image info with the following output:
		//
		// # image exists (exit=0)
		// rbd info volume-4a5bcc8b-2b55-46da-ba04-0d3dc5227f08
		//    size 1024 MB in 256 objects
		//    order 22 (4096 kB objects)
		// 	  block_name_prefix: rbd_data.1253ac238e1f29
		//    format: 2
		//    ...
		//
		//  rbd info volume-4a5bcc8b-2b55-46da-ba04-0d3dc5227f08 --format json
		// {"name":"volume-4a5bcc8b-2b55-46da-ba04-0d3dc5227f08","size":1073741824,"objects":256,"order":22,"object_size":4194304,"block_name_prefix":"rbd_data.1253ac238e1f29","format":2,"features":["layering","exclusive-lock","object-map","fast-diff","deep-flatten"],"flags":[]}
		//
		//
		// # image does not exist (exit=2)
		// rbd: error opening image 1234: (2) No such file or directory
		//
		glog.V(4).Infof("rbd: info %s using mon %s, pool %s id %s key %s", b.Image, mon, b.Pool, id, secret)
		cmd, err = b.exec.Run("rbd",
			"info", b.Image, "--pool", b.Pool, "-m", mon, "--id", id, "--key="+secret)
		output = string(cmd)

		// break if command succeeds
		if err == nil {
			break
		}

		if err, ok := err.(*exec.Error); ok {
			if err.Err == exec.ErrNotFound {
				glog.Errorf("rbd cmd not found")
				// fail fast if command not found
				return 0, err
			}
		}
	}

	// If command never succeed, returns its last error.
	if err != nil {
		return 0, err
	}

	if len(output) == 0 {
		return 0, fmt.Errorf("can not get image size info %s: %s", b.Image, output)
	}

	// get the size value string, just between `size ` and ` MB in`, such as `size 1024 MB in 256 objects`
	sizeIndex := strings.Index(output, imageSizeStr)
	divIndex := strings.Index(output, sizeDivStr)
	if sizeIndex == -1 || divIndex == -1 || divIndex <= sizeIndex+5 {
		return 0, fmt.Errorf("can not get image size info %s: %s", b.Image, output)
	}
	rbdSizeStr := output[sizeIndex+5 : divIndex]
	rbdSize, err := strconv.Atoi(rbdSizeStr)
	if err != nil {
		return 0, fmt.Errorf("can not convert size str: %s to int", rbdSizeStr)
	}

	return rbdSize, nil
}

// rbdStatus runs `rbd status` command to check if there is watcher on the image.
func (util *RBDUtil) rbdStatus(b *rbdMounter) (bool, string, error) {
	var err error
	var output string
	var cmd []byte

	// If we don't have admin id/secret (e.g. attaching), fallback to user id/secret.
	id := b.adminId
	secret := b.adminSecret
	if id == "" {
		id = b.Id
		secret = b.Secret
	}

	l := len(b.Mon)
	start := rand.Int() % l
	// iterate all hosts until rbd command succeeds.
	for i := start; i < start+l; i++ {
		mon := b.Mon[i%l]
		// cmd "rbd status" list the rbd client watch with the following output:
		//
		// # there is a watcher (exit=0)
		// Watchers:
		//   watcher=10.16.153.105:0/710245699 client.14163 cookie=1
		//
		// # there is no watcher (exit=0)
		// Watchers: none
		//
		// Otherwise, exit is non-zero, for example:
		//
		// # image does not exist (exit=2)
		// rbd: error opening image kubernetes-dynamic-pvc-<UUID>: (2) No such file or directory
		//
		glog.V(4).Infof("rbd: status %s using mon %s, pool %s id %s key %s", b.Image, mon, b.Pool, id, secret)
		cmd, err = b.exec.Run("rbd",
			"status", b.Image, "--pool", b.Pool, "-m", mon, "--id", id, "--key="+secret)
		output = string(cmd)

		// break if command succeeds
		if err == nil {
			break
		}

		if err, ok := err.(*exec.Error); ok {
			if err.Err == exec.ErrNotFound {
				glog.Errorf("rbd cmd not found")
				// fail fast if command not found
				return false, output, err
			}
		}
	}

	// If command never succeed, returns its last error.
	if err != nil {
		return false, output, err
	}

	if strings.Contains(output, imageWatcherStr) {
		glog.V(4).Infof("rbd: watchers on %s: %s", b.Image, output)
		return true, output, nil
	} else {
		glog.Warningf("rbd: no watchers on %s", b.Image)
		return false, output, nil
	}
}
