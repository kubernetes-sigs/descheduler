/*
Copyright 2019 The Kubernetes Authors.

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

package cloud

import (
	"time"

	v1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

var _ = SIGDescribe("[Feature:CloudProvider][Disruptive] Nodes", func() {
	f := framework.NewDefaultFramework("cloudprovider")
	var c clientset.Interface

	ginkgo.BeforeEach(func() {
		// Only supported in AWS/GCE because those are the only cloud providers
		// where E2E test are currently running.
		framework.SkipUnlessProviderIs("aws", "gce", "gke")
		c = f.ClientSet
	})

	ginkgo.It("should be deleted on API server if it doesn't exist in the cloud provider", func() {
		ginkgo.By("deleting a node on the cloud provider")

		nodeToDelete, err := e2enode.GetRandomReadySchedulableNode(c)
		framework.ExpectNoError(err)

		origNodes, err := e2enode.GetReadyNodesIncludingTainted(c)
		if err != nil {
			framework.Logf("Unexpected error occurred: %v", err)
		}
		// TODO: write a wrapper for ExpectNoErrorWithOffset()
		framework.ExpectNoErrorWithOffset(0, err)

		framework.Logf("Original number of ready nodes: %d", len(origNodes.Items))

		err = deleteNodeOnCloudProvider(nodeToDelete)
		if err != nil {
			framework.Failf("failed to delete node %q, err: %q", nodeToDelete.Name, err)
		}

		newNodes, err := e2enode.CheckReady(c, len(origNodes.Items)-1, 5*time.Minute)
		gomega.Expect(err).To(gomega.BeNil())
		framework.ExpectEqual(len(newNodes), len(origNodes.Items)-1)

		_, err = c.CoreV1().Nodes().Get(nodeToDelete.Name, metav1.GetOptions{})
		if err == nil {
			framework.Failf("node %q still exists when it should be deleted", nodeToDelete.Name)
		} else if !apierrs.IsNotFound(err) {
			framework.Failf("failed to get node %q err: %q", nodeToDelete.Name, err)
		}

	})
})

// DeleteNodeOnCloudProvider deletes the specified node.
func deleteNodeOnCloudProvider(node *v1.Node) error {
	return framework.TestContext.CloudConfig.Provider.DeleteNode(node)
}
