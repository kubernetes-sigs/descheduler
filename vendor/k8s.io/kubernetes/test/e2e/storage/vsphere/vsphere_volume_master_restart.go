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
	"fmt"
	"strconv"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/pkg/master/ports"
	"k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
	e2essh "k8s.io/kubernetes/test/e2e/framework/ssh"
	"k8s.io/kubernetes/test/e2e/storage/utils"
)

// waitForKubeletUp waits for the kubelet on the given host to be up.
func waitForKubeletUp(host string) error {
	cmd := "curl http://localhost:" + strconv.Itoa(ports.KubeletReadOnlyPort) + "/healthz"
	for start := time.Now(); time.Since(start) < time.Minute; time.Sleep(5 * time.Second) {
		result, err := e2essh.SSH(cmd, host, framework.TestContext.Provider)
		if err != nil || result.Code != 0 {
			e2essh.LogResult(result)
		}
		if result.Stdout == "ok" {
			return nil
		}
	}
	return fmt.Errorf("waiting for kubelet timed out")
}

/*
	Test to verify volume remains attached after kubelet restart on master node
	For the number of schedulable nodes,
	1. Create a volume with default volume options
	2. Create a Pod
	3. Verify the volume is attached
	4. Restart the kubelet on master node
	5. Verify again that the volume is attached
	6. Delete the pod and wait for the volume to be detached
	7. Delete the volume
*/
var _ = utils.SIGDescribe("Volume Attach Verify [Feature:vsphere][Serial][Disruptive]", func() {
	f := framework.NewDefaultFramework("restart-master")

	const labelKey = "vsphere_e2e_label"
	var (
		client                clientset.Interface
		namespace             string
		volumePaths           []string
		pods                  []*v1.Pod
		numNodes              int
		nodeKeyValueLabelList []map[string]string
		nodeNameList          []string
		nodeInfo              *NodeInfo
	)
	ginkgo.BeforeEach(func() {
		framework.SkipUnlessProviderIs("vsphere")
		Bootstrap(f)
		client = f.ClientSet
		namespace = f.Namespace.Name
		framework.ExpectNoError(framework.WaitForAllNodesSchedulable(client, framework.TestContext.NodeSchedulableTimeout))

		nodes, err := e2enode.GetReadySchedulableNodes(client)
		framework.ExpectNoError(err)
		numNodes = len(nodes.Items)
		if numNodes < 2 {
			framework.Skipf("Requires at least %d nodes (not %d)", 2, len(nodes.Items))
		}
		nodeInfo = TestContext.NodeMapper.GetNodeInfo(nodes.Items[0].Name)
		for i := 0; i < numNodes; i++ {
			nodeName := nodes.Items[i].Name
			nodeNameList = append(nodeNameList, nodeName)
			nodeLabelValue := "vsphere_e2e_" + string(uuid.NewUUID())
			nodeKeyValueLabel := make(map[string]string)
			nodeKeyValueLabel[labelKey] = nodeLabelValue
			nodeKeyValueLabelList = append(nodeKeyValueLabelList, nodeKeyValueLabel)
			framework.AddOrUpdateLabelOnNode(client, nodeName, labelKey, nodeLabelValue)
		}
	})

	ginkgo.It("verify volume remains attached after master kubelet restart", func() {
		framework.SkipUnlessSSHKeyPresent()

		// Create pod on each node
		for i := 0; i < numNodes; i++ {
			ginkgo.By(fmt.Sprintf("%d: Creating a test vsphere volume", i))
			volumePath, err := nodeInfo.VSphere.CreateVolume(&VolumeOptions{}, nodeInfo.DataCenterRef)
			framework.ExpectNoError(err)
			volumePaths = append(volumePaths, volumePath)

			ginkgo.By(fmt.Sprintf("Creating pod %d on node %v", i, nodeNameList[i]))
			podspec := getVSpherePodSpecWithVolumePaths([]string{volumePath}, nodeKeyValueLabelList[i], nil)
			pod, err := client.CoreV1().Pods(namespace).Create(podspec)
			framework.ExpectNoError(err)
			defer e2epod.DeletePodWithWait(client, pod)

			ginkgo.By("Waiting for pod to be ready")
			gomega.Expect(e2epod.WaitForPodNameRunningInNamespace(client, pod.Name, namespace)).To(gomega.Succeed())

			pod, err = client.CoreV1().Pods(namespace).Get(pod.Name, metav1.GetOptions{})
			framework.ExpectNoError(err)

			pods = append(pods, pod)

			nodeName := pod.Spec.NodeName
			ginkgo.By(fmt.Sprintf("Verify volume %s is attached to the node %s", volumePath, nodeName))
			expectVolumeToBeAttached(nodeName, volumePath)
		}

		ginkgo.By("Restarting kubelet on master node")
		masterAddress := framework.GetMasterHost() + ":22"
		err := framework.RestartKubelet(masterAddress)
		framework.ExpectNoError(err, "Unable to restart kubelet on master node")

		ginkgo.By("Verifying the kubelet on master node is up")
		err = waitForKubeletUp(masterAddress)
		framework.ExpectNoError(err)

		for i, pod := range pods {
			volumePath := volumePaths[i]
			nodeName := pod.Spec.NodeName

			ginkgo.By(fmt.Sprintf("After master restart, verify volume %v is attached to the node %v", volumePath, nodeName))
			expectVolumeToBeAttached(nodeName, volumePath)

			ginkgo.By(fmt.Sprintf("Deleting pod on node %s", nodeName))
			err = e2epod.DeletePodWithWait(client, pod)
			framework.ExpectNoError(err)

			ginkgo.By(fmt.Sprintf("Waiting for volume %s to be detached from the node %s", volumePath, nodeName))
			err = waitForVSphereDiskToDetach(volumePath, nodeName)
			framework.ExpectNoError(err)

			ginkgo.By(fmt.Sprintf("Deleting volume %s", volumePath))
			err = nodeInfo.VSphere.DeleteVolume(volumePath, nodeInfo.DataCenterRef)
			framework.ExpectNoError(err)
		}
	})
})
