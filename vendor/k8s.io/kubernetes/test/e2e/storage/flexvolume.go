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

package storage

import (
	"fmt"
	"net"
	"path"

	"time"

	"github.com/onsi/ginkgo"
	v1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
	e2essh "k8s.io/kubernetes/test/e2e/framework/ssh"
	"k8s.io/kubernetes/test/e2e/framework/testfiles"
	"k8s.io/kubernetes/test/e2e/framework/volume"
	"k8s.io/kubernetes/test/e2e/storage/utils"
)

const (
	sshPort                = "22"
	driverDir              = "test/e2e/testing-manifests/flexvolume/"
	defaultVolumePluginDir = "/usr/libexec/kubernetes/kubelet-plugins/volume/exec"
	// TODO: change this and config-test.sh when default flex volume install path is changed for GCI
	// On gci, root is read-only and controller-manager containerized. Assume
	// controller-manager has started with --flex-volume-plugin-dir equal to this
	// (see cluster/gce/config-test.sh)
	gciVolumePluginDir = "/home/kubernetes/flexvolume"
	detachTimeout      = 10 * time.Second
)

// testFlexVolume tests that a client pod using a given flexvolume driver
// successfully mounts it and runs
func testFlexVolume(driver string, config volume.TestConfig, f *framework.Framework) {
	tests := []volume.Test{
		{
			Volume: v1.VolumeSource{
				FlexVolume: &v1.FlexVolumeSource{
					Driver: "k8s/" + driver,
				},
			},
			File: "index.html",
			// Must match content of examples/volumes/flexvolume/dummy(-attachable) domount
			ExpectedContent: "Hello from flexvolume!",
		},
	}
	volume.TestVolumeClient(f, config, nil, "" /* fsType */, tests)

	volume.TestCleanup(f, config)
}

// installFlex installs the driver found at filePath on the node, and restarts
// kubelet if 'restart' is true. If node is nil, installs on the master, and restarts
// controller-manager if 'restart' is true.
func installFlex(c clientset.Interface, node *v1.Node, vendor, driver, filePath string) {
	flexDir := getFlexDir(c, node, vendor, driver)
	flexFile := path.Join(flexDir, driver)

	host := ""
	var err error
	if node != nil {
		host, err = e2enode.GetExternalIP(node)
		if err != nil {
			host, err = e2enode.GetInternalIP(node)
		}
	} else {
		masterHostWithPort := framework.GetMasterHost()
		hostName := getHostFromHostPort(masterHostWithPort)
		host = net.JoinHostPort(hostName, sshPort)
	}

	framework.ExpectNoError(err)

	cmd := fmt.Sprintf("sudo mkdir -p %s", flexDir)
	sshAndLog(cmd, host, true /*failOnError*/)

	data := testfiles.ReadOrDie(filePath)
	cmd = fmt.Sprintf("sudo tee <<'EOF' %s\n%s\nEOF", flexFile, string(data))
	sshAndLog(cmd, host, true /*failOnError*/)

	cmd = fmt.Sprintf("sudo chmod +x %s", flexFile)
	sshAndLog(cmd, host, true /*failOnError*/)
}

func uninstallFlex(c clientset.Interface, node *v1.Node, vendor, driver string) {
	flexDir := getFlexDir(c, node, vendor, driver)

	host := ""
	var err error
	if node != nil {
		host, err = e2enode.GetExternalIP(node)
		if err != nil {
			host, err = e2enode.GetInternalIP(node)
		}
	} else {
		masterHostWithPort := framework.GetMasterHost()
		hostName := getHostFromHostPort(masterHostWithPort)
		host = net.JoinHostPort(hostName, sshPort)
	}

	if host == "" {
		framework.Failf("Error getting node ip : %v", err)
	}

	cmd := fmt.Sprintf("sudo rm -r %s", flexDir)
	sshAndLog(cmd, host, false /*failOnError*/)
}

func getFlexDir(c clientset.Interface, node *v1.Node, vendor, driver string) string {
	volumePluginDir := defaultVolumePluginDir
	if framework.ProviderIs("gce") {
		volumePluginDir = gciVolumePluginDir
	}
	flexDir := path.Join(volumePluginDir, fmt.Sprintf("/%s~%s/", vendor, driver))
	return flexDir
}

func sshAndLog(cmd, host string, failOnError bool) {
	result, err := e2essh.SSH(cmd, host, framework.TestContext.Provider)
	e2essh.LogResult(result)
	framework.ExpectNoError(err)
	if result.Code != 0 && failOnError {
		framework.Failf("%s returned non-zero, stderr: %s", cmd, result.Stderr)
	}
}

func getHostFromHostPort(hostPort string) string {
	// try to split host and port
	var host string
	var err error
	if host, _, err = net.SplitHostPort(hostPort); err != nil {
		// if SplitHostPort returns an error, the entire hostport is considered as host
		host = hostPort
	}
	return host
}

var _ = utils.SIGDescribe("Flexvolumes", func() {
	f := framework.NewDefaultFramework("flexvolume")

	// note that namespace deletion is handled by delete-namespace flag

	var cs clientset.Interface
	var ns *v1.Namespace
	var node *v1.Node
	var config volume.TestConfig
	var suffix string

	ginkgo.BeforeEach(func() {
		framework.SkipUnlessProviderIs("gce", "local")
		framework.SkipUnlessMasterOSDistroIs("debian", "ubuntu", "gci", "custom")
		framework.SkipUnlessNodeOSDistroIs("debian", "ubuntu", "gci", "custom")
		framework.SkipUnlessSSHKeyPresent()

		cs = f.ClientSet
		ns = f.Namespace
		var err error
		node, err = e2enode.GetRandomReadySchedulableNode(f.ClientSet)
		framework.ExpectNoError(err)
		config = volume.TestConfig{
			Namespace:      ns.Name,
			Prefix:         "flex",
			ClientNodeName: node.Name,
		}
		suffix = ns.Name
	})

	ginkgo.It("should be mountable when non-attachable", func() {
		driver := "dummy"
		driverInstallAs := driver + "-" + suffix

		ginkgo.By(fmt.Sprintf("installing flexvolume %s on node %s as %s", path.Join(driverDir, driver), node.Name, driverInstallAs))
		installFlex(cs, node, "k8s", driverInstallAs, path.Join(driverDir, driver))

		testFlexVolume(driverInstallAs, config, f)

		ginkgo.By("waiting for flex client pod to terminate")
		if err := f.WaitForPodTerminated(config.Prefix+"-client", ""); !apierrs.IsNotFound(err) {
			framework.ExpectNoError(err, "Failed to wait client pod terminated: %v", err)
		}

		ginkgo.By(fmt.Sprintf("uninstalling flexvolume %s from node %s", driverInstallAs, node.Name))
		uninstallFlex(cs, node, "k8s", driverInstallAs)
	})

	ginkgo.It("should be mountable when attachable", func() {
		driver := "dummy-attachable"
		driverInstallAs := driver + "-" + suffix

		ginkgo.By(fmt.Sprintf("installing flexvolume %s on node %s as %s", path.Join(driverDir, driver), node.Name, driverInstallAs))
		installFlex(cs, node, "k8s", driverInstallAs, path.Join(driverDir, driver))
		ginkgo.By(fmt.Sprintf("installing flexvolume %s on master as %s", path.Join(driverDir, driver), driverInstallAs))
		installFlex(cs, nil, "k8s", driverInstallAs, path.Join(driverDir, driver))

		testFlexVolume(driverInstallAs, config, f)

		ginkgo.By("waiting for flex client pod to terminate")
		if err := f.WaitForPodTerminated(config.Prefix+"-client", ""); !apierrs.IsNotFound(err) {
			framework.ExpectNoError(err, "Failed to wait client pod terminated: %v", err)
		}

		// Detach might occur after pod deletion. Wait before deleting driver.
		time.Sleep(detachTimeout)

		ginkgo.By(fmt.Sprintf("uninstalling flexvolume %s from node %s", driverInstallAs, node.Name))
		uninstallFlex(cs, node, "k8s", driverInstallAs)
		ginkgo.By(fmt.Sprintf("uninstalling flexvolume %s from master", driverInstallAs))
		uninstallFlex(cs, nil, "k8s", driverInstallAs)
	})
})
