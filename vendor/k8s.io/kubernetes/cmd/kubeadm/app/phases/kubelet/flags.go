/*
Copyright 2018 The Kubernetes Authors.

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

package kubelet

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/klog"
	kubeadmapi "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm"
	"k8s.io/kubernetes/cmd/kubeadm/app/constants"
	"k8s.io/kubernetes/cmd/kubeadm/app/features"
	"k8s.io/kubernetes/cmd/kubeadm/app/images"
	kubeadmutil "k8s.io/kubernetes/cmd/kubeadm/app/util"
	"k8s.io/kubernetes/cmd/kubeadm/app/util/initsystem"
	utilsexec "k8s.io/utils/exec"
)

type kubeletFlagsOpts struct {
	nodeRegOpts              *kubeadmapi.NodeRegistrationOptions
	featureGates             map[string]bool
	pauseImage               string
	registerTaintsUsingFlags bool
	execer                   utilsexec.Interface
	isServiceActiveFunc      func(string) (bool, error)
	defaultHostname          string
}

// WriteKubeletDynamicEnvFile writes an environment file with dynamic flags to the kubelet.
// Used at "kubeadm init" and "kubeadm join" time.
func WriteKubeletDynamicEnvFile(cfg *kubeadmapi.ClusterConfiguration, nodeReg *kubeadmapi.NodeRegistrationOptions, registerTaintsUsingFlags bool, kubeletDir string) error {
	hostName, err := kubeadmutil.GetHostname("")
	if err != nil {
		return err
	}

	flagOpts := kubeletFlagsOpts{
		nodeRegOpts:              nodeReg,
		featureGates:             cfg.FeatureGates,
		pauseImage:               images.GetPauseImage(cfg),
		registerTaintsUsingFlags: registerTaintsUsingFlags,
		execer:                   utilsexec.New(),
		isServiceActiveFunc: func(name string) (bool, error) {
			initSystem, err := initsystem.GetInitSystem()
			if err != nil {
				return false, err
			}
			return initSystem.ServiceIsActive(name), nil
		},
		defaultHostname: hostName,
	}
	stringMap := buildKubeletArgMap(flagOpts)
	argList := kubeadmutil.BuildArgumentListFromMap(stringMap, nodeReg.KubeletExtraArgs)
	envFileContent := fmt.Sprintf("%s=%q\n", constants.KubeletEnvFileVariableName, strings.Join(argList, " "))

	return writeKubeletFlagBytesToDisk([]byte(envFileContent), kubeletDir)
}

// buildKubeletArgMap takes a kubeletFlagsOpts object and builds based on that a string-string map with flags
// that should be given to the local kubelet daemon.
func buildKubeletArgMap(opts kubeletFlagsOpts) map[string]string {
	kubeletFlags := map[string]string{}

	if opts.nodeRegOpts.CRISocket == constants.DefaultDockerCRISocket {
		// These flags should only be set when running docker
		kubeletFlags["network-plugin"] = "cni"
		driver, err := kubeadmutil.GetCgroupDriverDocker(opts.execer)
		if err != nil {
			klog.Warningf("cannot automatically assign a '--cgroup-driver' value when starting the Kubelet: %v\n", err)
		} else {
			kubeletFlags["cgroup-driver"] = driver
		}
		if opts.pauseImage != "" {
			kubeletFlags["pod-infra-container-image"] = opts.pauseImage
		}
	} else {
		kubeletFlags["container-runtime"] = "remote"
		kubeletFlags["container-runtime-endpoint"] = opts.nodeRegOpts.CRISocket
	}

	if opts.registerTaintsUsingFlags && opts.nodeRegOpts.Taints != nil && len(opts.nodeRegOpts.Taints) > 0 {
		taintStrs := []string{}
		for _, taint := range opts.nodeRegOpts.Taints {
			taintStrs = append(taintStrs, taint.ToString())
		}

		kubeletFlags["register-with-taints"] = strings.Join(taintStrs, ",")
	}

	ok, err := opts.isServiceActiveFunc("systemd-resolved")
	if err != nil {
		klog.Warningf("cannot determine if systemd-resolved is active: %v\n", err)
	}
	if ok {
		kubeletFlags["resolv-conf"] = "/run/systemd/resolve/resolv.conf"
	}

	// Make sure the node name we're passed will work with Kubelet
	if opts.nodeRegOpts.Name != "" && opts.nodeRegOpts.Name != opts.defaultHostname {
		klog.V(1).Infof("setting kubelet hostname-override to %q", opts.nodeRegOpts.Name)
		kubeletFlags["hostname-override"] = opts.nodeRegOpts.Name
	}

	// TODO: Conditionally set `--cgroup-driver` to either `systemd` or `cgroupfs` for CRI other than Docker

	// TODO: The following code should be remvoved after dual-stack is GA.
	// Note: The user still retains the ability to explicitly set feature-gates and that value will overwrite this base value.
	if enabled, present := opts.featureGates[features.IPv6DualStack]; present {
		kubeletFlags["feature-gates"] = fmt.Sprintf("%s=%t", features.IPv6DualStack, enabled)
	}

	return kubeletFlags
}

// writeKubeletFlagBytesToDisk writes a byte slice down to disk at the specific location of the kubelet flag overrides file
func writeKubeletFlagBytesToDisk(b []byte, kubeletDir string) error {
	kubeletEnvFilePath := filepath.Join(kubeletDir, constants.KubeletEnvFileName)
	fmt.Printf("[kubelet-start] Writing kubelet environment file with flags to file %q\n", kubeletEnvFilePath)

	// creates target folder if not already exists
	if err := os.MkdirAll(kubeletDir, 0700); err != nil {
		return errors.Wrapf(err, "failed to create directory %q", kubeletDir)
	}
	if err := ioutil.WriteFile(kubeletEnvFilePath, b, 0644); err != nil {
		return errors.Wrapf(err, "failed to write kubelet configuration to the file %q", kubeletEnvFilePath)
	}
	return nil
}
