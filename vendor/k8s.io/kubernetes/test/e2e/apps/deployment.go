/*
Copyright 2015 The Kubernetes Authors.

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

package apps

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/api/core/v1"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	clientset "k8s.io/client-go/kubernetes"
	appsinternal "k8s.io/kubernetes/pkg/apis/apps"
	deploymentutil "k8s.io/kubernetes/pkg/controller/deployment/util"
	"k8s.io/kubernetes/test/e2e/framework"
	e2edeploy "k8s.io/kubernetes/test/e2e/framework/deployment"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
	"k8s.io/kubernetes/test/e2e/framework/replicaset"
	e2eservice "k8s.io/kubernetes/test/e2e/framework/service"
	testutil "k8s.io/kubernetes/test/utils"
	utilpointer "k8s.io/utils/pointer"
)

const (
	dRetryPeriod  = 2 * time.Second
	dRetryTimeout = 5 * time.Minute
)

var (
	nilRs *appsv1.ReplicaSet
)

var _ = SIGDescribe("Deployment", func() {
	var ns string
	var c clientset.Interface

	ginkgo.AfterEach(func() {
		failureTrap(c, ns)
	})

	f := framework.NewDefaultFramework("deployment")

	ginkgo.BeforeEach(func() {
		c = f.ClientSet
		ns = f.Namespace.Name
	})

	ginkgo.It("deployment reaping should cascade to its replica sets and pods", func() {
		testDeleteDeployment(f)
	})
	/*
	  Testname: Deployment RollingUpdate
	  Description: A conformant Kubernetes distribution MUST support the Deployment with RollingUpdate strategy.
	*/
	framework.ConformanceIt("RollingUpdateDeployment should delete old pods and create new ones", func() {
		testRollingUpdateDeployment(f)
	})
	/*
	  Testname: Deployment Recreate
	  Description: A conformant Kubernetes distribution MUST support the Deployment with Recreate strategy.
	*/
	framework.ConformanceIt("RecreateDeployment should delete old pods and create new ones", func() {
		testRecreateDeployment(f)
	})
	/*
	  Testname: Deployment RevisionHistoryLimit
	  Description: A conformant Kubernetes distribution MUST clean up Deployment's ReplicaSets based on
	  the Deployment's `.spec.revisionHistoryLimit`.
	*/
	framework.ConformanceIt("deployment should delete old replica sets", func() {
		testDeploymentCleanUpPolicy(f)
	})
	/*
	  Testname: Deployment Rollover
	  Description: A conformant Kubernetes distribution MUST support Deployment rollover,
	    i.e. allow arbitrary number of changes to desired state during rolling update
	    before the rollout finishes.
	*/
	framework.ConformanceIt("deployment should support rollover", func() {
		testRolloverDeployment(f)
	})
	ginkgo.It("iterative rollouts should eventually progress", func() {
		testIterativeDeployments(f)
	})
	ginkgo.It("test Deployment ReplicaSet orphaning and adoption regarding controllerRef", func() {
		testDeploymentsControllerRef(f)
	})
	/*
	  Testname: Deployment Proportional Scaling
	  Description: A conformant Kubernetes distribution MUST support Deployment
	    proportional scaling, i.e. proportionally scale a Deployment's ReplicaSets
	    when a Deployment is scaled.
	*/
	framework.ConformanceIt("deployment should support proportional scaling", func() {
		testProportionalScalingDeployment(f)
	})
	ginkgo.It("should not disrupt a cloud load-balancer's connectivity during rollout", func() {
		framework.SkipUnlessProviderIs("aws", "azure", "gce", "gke")
		testRollingUpdateDeploymentWithLocalTrafficLoadBalancer(f)
	})
	// TODO: add tests that cover deployment.Spec.MinReadySeconds once we solved clock-skew issues
	// See https://github.com/kubernetes/kubernetes/issues/29229
})

func failureTrap(c clientset.Interface, ns string) {
	deployments, err := c.AppsV1().Deployments(ns).List(metav1.ListOptions{LabelSelector: labels.Everything().String()})
	if err != nil {
		framework.Logf("Could not list Deployments in namespace %q: %v", ns, err)
		return
	}
	for i := range deployments.Items {
		d := deployments.Items[i]

		framework.Logf(spew.Sprintf("Deployment %q:\n%+v\n", d.Name, d))
		_, allOldRSs, newRS, err := deploymentutil.GetAllReplicaSets(&d, c.AppsV1())
		if err != nil {
			framework.Logf("Could not list ReplicaSets for Deployment %q: %v", d.Name, err)
			return
		}
		testutil.LogReplicaSetsOfDeployment(&d, allOldRSs, newRS, framework.Logf)
		rsList := allOldRSs
		if newRS != nil {
			rsList = append(rsList, newRS)
		}
		testutil.LogPodsOfDeployment(c, &d, rsList, framework.Logf)
	}
	// We need print all the ReplicaSets if there are no Deployment object created
	if len(deployments.Items) != 0 {
		return
	}
	framework.Logf("Log out all the ReplicaSets if there is no deployment created")
	rss, err := c.AppsV1().ReplicaSets(ns).List(metav1.ListOptions{LabelSelector: labels.Everything().String()})
	if err != nil {
		framework.Logf("Could not list ReplicaSets in namespace %q: %v", ns, err)
		return
	}
	for _, rs := range rss.Items {
		framework.Logf(spew.Sprintf("ReplicaSet %q:\n%+v\n", rs.Name, rs))
		selector, err := metav1.LabelSelectorAsSelector(rs.Spec.Selector)
		if err != nil {
			framework.Logf("failed to get selector of ReplicaSet %s: %v", rs.Name, err)
		}
		options := metav1.ListOptions{LabelSelector: selector.String()}
		podList, err := c.CoreV1().Pods(rs.Namespace).List(options)
		if err != nil {
			framework.Logf("Failed to list Pods in namespace %s: %v", rs.Namespace, err)
			continue
		}
		for _, pod := range podList.Items {
			framework.Logf(spew.Sprintf("pod: %q:\n%+v\n", pod.Name, pod))
		}
	}
}

func intOrStrP(num int) *intstr.IntOrString {
	intstr := intstr.FromInt(num)
	return &intstr
}

func newDeploymentRollback(name string, annotations map[string]string, revision int64) *extensionsv1beta1.DeploymentRollback {
	return &extensionsv1beta1.DeploymentRollback{
		Name:               name,
		UpdatedAnnotations: annotations,
		RollbackTo:         extensionsv1beta1.RollbackConfig{Revision: revision},
	}
}

func stopDeployment(c clientset.Interface, ns, deploymentName string) {
	deployment, err := c.AppsV1().Deployments(ns).Get(deploymentName, metav1.GetOptions{})
	framework.ExpectNoError(err)

	framework.Logf("Deleting deployment %s", deploymentName)
	err = framework.DeleteResourceAndWaitForGC(c, appsinternal.Kind("Deployment"), ns, deployment.Name)
	framework.ExpectNoError(err)

	framework.Logf("Ensuring deployment %s was deleted", deploymentName)
	_, err = c.AppsV1().Deployments(ns).Get(deployment.Name, metav1.GetOptions{})
	framework.ExpectError(err)
	gomega.Expect(errors.IsNotFound(err)).To(gomega.BeTrue())
	framework.Logf("Ensuring deployment %s's RSes were deleted", deploymentName)
	selector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
	framework.ExpectNoError(err)
	options := metav1.ListOptions{LabelSelector: selector.String()}
	rss, err := c.AppsV1().ReplicaSets(ns).List(options)
	framework.ExpectNoError(err)
	gomega.Expect(rss.Items).Should(gomega.HaveLen(0))
	framework.Logf("Ensuring deployment %s's Pods were deleted", deploymentName)
	var pods *v1.PodList
	if err := wait.PollImmediate(time.Second, timeout, func() (bool, error) {
		pods, err = c.CoreV1().Pods(ns).List(options)
		if err != nil {
			return false, err
		}
		// Pods may be created by overlapping deployments right after this deployment is deleted, ignore them
		if len(pods.Items) == 0 {
			return true, nil
		}
		return false, nil
	}); err != nil {
		framework.Failf("Err : %s\n. Failed to remove deployment %s pods : %+v", err, deploymentName, pods)
	}
}

func testDeleteDeployment(f *framework.Framework) {
	ns := f.Namespace.Name
	c := f.ClientSet

	deploymentName := "test-new-deployment"
	podLabels := map[string]string{"name": WebserverImageName}
	replicas := int32(1)
	framework.Logf("Creating simple deployment %s", deploymentName)
	d := e2edeploy.NewDeployment(deploymentName, replicas, podLabels, WebserverImageName, WebserverImage, appsv1.RollingUpdateDeploymentStrategyType)
	d.Annotations = map[string]string{"test": "should-copy-to-replica-set", v1.LastAppliedConfigAnnotation: "should-not-copy-to-replica-set"}
	deploy, err := c.AppsV1().Deployments(ns).Create(d)
	framework.ExpectNoError(err)

	// Wait for it to be updated to revision 1
	err = e2edeploy.WaitForDeploymentRevisionAndImage(c, ns, deploymentName, "1", WebserverImage)
	framework.ExpectNoError(err)

	err = e2edeploy.WaitForDeploymentComplete(c, deploy)
	framework.ExpectNoError(err)

	deployment, err := c.AppsV1().Deployments(ns).Get(deploymentName, metav1.GetOptions{})
	framework.ExpectNoError(err)
	newRS, err := deploymentutil.GetNewReplicaSet(deployment, c.AppsV1())
	framework.ExpectNoError(err)
	framework.ExpectNotEqual(newRS, nilRs)
	stopDeployment(c, ns, deploymentName)
}

func testRollingUpdateDeployment(f *framework.Framework) {
	ns := f.Namespace.Name
	c := f.ClientSet
	// Create webserver pods.
	deploymentPodLabels := map[string]string{"name": "sample-pod"}
	rsPodLabels := map[string]string{
		"name": "sample-pod",
		"pod":  WebserverImageName,
	}

	rsName := "test-rolling-update-controller"
	replicas := int32(1)
	rsRevision := "3546343826724305832"
	annotations := make(map[string]string)
	annotations[deploymentutil.RevisionAnnotation] = rsRevision
	rs := newRS(rsName, replicas, rsPodLabels, WebserverImageName, WebserverImage, nil)
	rs.Annotations = annotations
	framework.Logf("Creating replica set %q (going to be adopted)", rs.Name)
	_, err := c.AppsV1().ReplicaSets(ns).Create(rs)
	framework.ExpectNoError(err)
	// Verify that the required pods have come up.
	err = e2epod.VerifyPodsRunning(c, ns, "sample-pod", false, replicas)
	framework.ExpectNoError(err, "error in waiting for pods to come up: %s", err)

	// Create a deployment to delete webserver pods and instead bring up agnhost pods.
	deploymentName := "test-rolling-update-deployment"
	framework.Logf("Creating deployment %q", deploymentName)
	d := e2edeploy.NewDeployment(deploymentName, replicas, deploymentPodLabels, AgnhostImageName, AgnhostImage, appsv1.RollingUpdateDeploymentStrategyType)
	deploy, err := c.AppsV1().Deployments(ns).Create(d)
	framework.ExpectNoError(err)

	// Wait for it to be updated to revision 3546343826724305833.
	framework.Logf("Ensuring deployment %q gets the next revision from the one the adopted replica set %q has", deploy.Name, rs.Name)
	err = e2edeploy.WaitForDeploymentRevisionAndImage(c, ns, deploymentName, "3546343826724305833", AgnhostImage)
	framework.ExpectNoError(err)

	framework.Logf("Ensuring status for deployment %q is the expected", deploy.Name)
	err = e2edeploy.WaitForDeploymentComplete(c, deploy)
	framework.ExpectNoError(err)

	// There should be 1 old RS (webserver-controller, which is adopted)
	framework.Logf("Ensuring deployment %q has one old replica set (the one it adopted)", deploy.Name)
	deployment, err := c.AppsV1().Deployments(ns).Get(deploymentName, metav1.GetOptions{})
	framework.ExpectNoError(err)
	_, allOldRSs, err := deploymentutil.GetOldReplicaSets(deployment, c.AppsV1())
	framework.ExpectNoError(err)
	framework.ExpectEqual(len(allOldRSs), 1)
}

func testRecreateDeployment(f *framework.Framework) {
	ns := f.Namespace.Name
	c := f.ClientSet

	// Create a deployment that brings up agnhost pods.
	deploymentName := "test-recreate-deployment"
	framework.Logf("Creating deployment %q", deploymentName)
	d := e2edeploy.NewDeployment(deploymentName, int32(1), map[string]string{"name": "sample-pod-3"}, AgnhostImageName, AgnhostImage, appsv1.RecreateDeploymentStrategyType)
	deployment, err := c.AppsV1().Deployments(ns).Create(d)
	framework.ExpectNoError(err)

	// Wait for it to be updated to revision 1
	framework.Logf("Waiting deployment %q to be updated to revision 1", deploymentName)
	err = e2edeploy.WaitForDeploymentRevisionAndImage(c, ns, deploymentName, "1", AgnhostImage)
	framework.ExpectNoError(err)

	framework.Logf("Waiting deployment %q to complete", deploymentName)
	err = e2edeploy.WaitForDeploymentComplete(c, deployment)
	framework.ExpectNoError(err)

	// Update deployment to delete agnhost pods and bring up webserver pods.
	framework.Logf("Triggering a new rollout for deployment %q", deploymentName)
	deployment, err = e2edeploy.UpdateDeploymentWithRetries(c, ns, deploymentName, func(update *appsv1.Deployment) {
		update.Spec.Template.Spec.Containers[0].Name = WebserverImageName
		update.Spec.Template.Spec.Containers[0].Image = WebserverImage
	})
	framework.ExpectNoError(err)

	framework.Logf("Watching deployment %q to verify that new pods will not run with olds pods", deploymentName)
	err = e2edeploy.WatchRecreateDeployment(c, deployment)
	framework.ExpectNoError(err)
}

// testDeploymentCleanUpPolicy tests that deployment supports cleanup policy
func testDeploymentCleanUpPolicy(f *framework.Framework) {
	ns := f.Namespace.Name
	c := f.ClientSet
	// Create webserver pods.
	deploymentPodLabels := map[string]string{"name": "cleanup-pod"}
	rsPodLabels := map[string]string{
		"name": "cleanup-pod",
		"pod":  WebserverImageName,
	}
	rsName := "test-cleanup-controller"
	replicas := int32(1)
	revisionHistoryLimit := utilpointer.Int32Ptr(0)
	_, err := c.AppsV1().ReplicaSets(ns).Create(newRS(rsName, replicas, rsPodLabels, WebserverImageName, WebserverImage, nil))
	framework.ExpectNoError(err)

	// Verify that the required pods have come up.
	err = e2epod.VerifyPodsRunning(c, ns, "cleanup-pod", false, replicas)
	framework.ExpectNoError(err, "error in waiting for pods to come up: %v", err)

	// Create a deployment to delete webserver pods and instead bring up agnhost pods.
	deploymentName := "test-cleanup-deployment"
	framework.Logf("Creating deployment %s", deploymentName)

	pods, err := c.CoreV1().Pods(ns).List(metav1.ListOptions{LabelSelector: labels.Everything().String()})
	framework.ExpectNoError(err, "Failed to query for pods: %v", err)

	options := metav1.ListOptions{
		ResourceVersion: pods.ListMeta.ResourceVersion,
	}
	stopCh := make(chan struct{})
	defer close(stopCh)
	w, err := c.CoreV1().Pods(ns).Watch(options)
	framework.ExpectNoError(err)
	go func() {
		// There should be only one pod being created, which is the pod with the agnhost image.
		// The old RS shouldn't create new pod when deployment controller adding pod template hash label to its selector.
		numPodCreation := 1
		for {
			select {
			case event, _ := <-w.ResultChan():
				if event.Type != watch.Added {
					continue
				}
				numPodCreation--
				if numPodCreation < 0 {
					framework.Failf("Expect only one pod creation, the second creation event: %#v\n", event)
				}
				pod, ok := event.Object.(*v1.Pod)
				if !ok {
					framework.Failf("Expect event Object to be a pod")
				}
				if pod.Spec.Containers[0].Name != AgnhostImageName {
					framework.Failf("Expect the created pod to have container name %s, got pod %#v\n", AgnhostImageName, pod)
				}
			case <-stopCh:
				return
			}
		}
	}()
	d := e2edeploy.NewDeployment(deploymentName, replicas, deploymentPodLabels, AgnhostImageName, AgnhostImage, appsv1.RollingUpdateDeploymentStrategyType)
	d.Spec.RevisionHistoryLimit = revisionHistoryLimit
	_, err = c.AppsV1().Deployments(ns).Create(d)
	framework.ExpectNoError(err)

	ginkgo.By(fmt.Sprintf("Waiting for deployment %s history to be cleaned up", deploymentName))
	err = e2edeploy.WaitForDeploymentOldRSsNum(c, ns, deploymentName, int(*revisionHistoryLimit))
	framework.ExpectNoError(err)
}

// testRolloverDeployment tests that deployment supports rollover.
// i.e. we can change desired state and kick off rolling update, then change desired state again before it finishes.
func testRolloverDeployment(f *framework.Framework) {
	ns := f.Namespace.Name
	c := f.ClientSet
	podName := "rollover-pod"
	deploymentPodLabels := map[string]string{"name": podName}
	rsPodLabels := map[string]string{
		"name": podName,
		"pod":  WebserverImageName,
	}

	rsName := "test-rollover-controller"
	rsReplicas := int32(1)
	_, err := c.AppsV1().ReplicaSets(ns).Create(newRS(rsName, rsReplicas, rsPodLabels, WebserverImageName, WebserverImage, nil))
	framework.ExpectNoError(err)
	// Verify that the required pods have come up.
	err = e2epod.VerifyPodsRunning(c, ns, podName, false, rsReplicas)
	framework.ExpectNoError(err, "error in waiting for pods to come up: %v", err)

	// Wait for replica set to become ready before adopting it.
	framework.Logf("Waiting for pods owned by replica set %q to become ready", rsName)
	err = replicaset.WaitForReadyReplicaSet(c, ns, rsName)
	framework.ExpectNoError(err)

	// Create a deployment to delete webserver pods and instead bring up redis-slave pods.
	// We use a nonexistent image here, so that we make sure it won't finish
	deploymentName, deploymentImageName := "test-rollover-deployment", "redis-slave"
	deploymentReplicas := int32(1)
	deploymentImage := "gcr.io/google_samples/gb-redisslave:nonexistent"
	deploymentStrategyType := appsv1.RollingUpdateDeploymentStrategyType
	framework.Logf("Creating deployment %q", deploymentName)
	newDeployment := e2edeploy.NewDeployment(deploymentName, deploymentReplicas, deploymentPodLabels, deploymentImageName, deploymentImage, deploymentStrategyType)
	newDeployment.Spec.Strategy.RollingUpdate = &appsv1.RollingUpdateDeployment{
		MaxUnavailable: intOrStrP(0),
		MaxSurge:       intOrStrP(1),
	}
	newDeployment.Spec.MinReadySeconds = int32(10)
	_, err = c.AppsV1().Deployments(ns).Create(newDeployment)
	framework.ExpectNoError(err)

	// Verify that the pods were scaled up and down as expected.
	deployment, err := c.AppsV1().Deployments(ns).Get(deploymentName, metav1.GetOptions{})
	framework.ExpectNoError(err)
	framework.Logf("Make sure deployment %q performs scaling operations", deploymentName)
	// Make sure the deployment starts to scale up and down replica sets by checking if its updated replicas >= 1
	err = e2edeploy.WaitForDeploymentUpdatedReplicasGTE(c, ns, deploymentName, deploymentReplicas, deployment.Generation)
	// Check if it's updated to revision 1 correctly
	framework.Logf("Check revision of new replica set for deployment %q", deploymentName)
	err = e2edeploy.CheckDeploymentRevisionAndImage(c, ns, deploymentName, "1", deploymentImage)
	framework.ExpectNoError(err)

	framework.Logf("Ensure that both replica sets have 1 created replica")
	oldRS, err := c.AppsV1().ReplicaSets(ns).Get(rsName, metav1.GetOptions{})
	framework.ExpectNoError(err)
	ensureReplicas(oldRS, int32(1))
	newRS, err := deploymentutil.GetNewReplicaSet(deployment, c.AppsV1())
	framework.ExpectNoError(err)
	ensureReplicas(newRS, int32(1))

	// The deployment is stuck, update it to rollover the above 2 ReplicaSets and bring up agnhost pods.
	framework.Logf("Rollover old replica sets for deployment %q with new image update", deploymentName)
	updatedDeploymentImageName, updatedDeploymentImage := AgnhostImageName, AgnhostImage
	deployment, err = e2edeploy.UpdateDeploymentWithRetries(c, ns, newDeployment.Name, func(update *appsv1.Deployment) {
		update.Spec.Template.Spec.Containers[0].Name = updatedDeploymentImageName
		update.Spec.Template.Spec.Containers[0].Image = updatedDeploymentImage
	})
	framework.ExpectNoError(err)

	// Use observedGeneration to determine if the controller noticed the pod template update.
	framework.Logf("Wait deployment %q to be observed by the deployment controller", deploymentName)
	err = e2edeploy.WaitForObservedDeployment(c, ns, deploymentName, deployment.Generation)
	framework.ExpectNoError(err)

	// Wait for it to be updated to revision 2
	framework.Logf("Wait for revision update of deployment %q to 2", deploymentName)
	err = e2edeploy.WaitForDeploymentRevisionAndImage(c, ns, deploymentName, "2", updatedDeploymentImage)
	framework.ExpectNoError(err)

	framework.Logf("Make sure deployment %q is complete", deploymentName)
	err = e2edeploy.WaitForDeploymentCompleteAndCheckRolling(c, deployment)
	framework.ExpectNoError(err)

	framework.Logf("Ensure that both old replica sets have no replicas")
	oldRS, err = c.AppsV1().ReplicaSets(ns).Get(rsName, metav1.GetOptions{})
	framework.ExpectNoError(err)
	ensureReplicas(oldRS, int32(0))
	// Not really the new replica set anymore but we GET by name so that's fine.
	newRS, err = c.AppsV1().ReplicaSets(ns).Get(newRS.Name, metav1.GetOptions{})
	framework.ExpectNoError(err)
	ensureReplicas(newRS, int32(0))
}

func ensureReplicas(rs *appsv1.ReplicaSet, replicas int32) {
	framework.ExpectEqual(*rs.Spec.Replicas, replicas)
	framework.ExpectEqual(rs.Status.Replicas, replicas)
}

func randomScale(d *appsv1.Deployment, i int) {
	switch r := rand.Float32(); {
	case r < 0.3:
		framework.Logf("%02d: scaling up", i)
		*(d.Spec.Replicas)++
	case r < 0.6:
		if *(d.Spec.Replicas) > 1 {
			framework.Logf("%02d: scaling down", i)
			*(d.Spec.Replicas)--
		}
	}
}

func testIterativeDeployments(f *framework.Framework) {
	ns := f.Namespace.Name
	c := f.ClientSet

	podLabels := map[string]string{"name": WebserverImageName}
	replicas := int32(6)
	zero := int64(0)
	two := int32(2)

	// Create a webserver deployment.
	deploymentName := "webserver"
	thirty := int32(30)
	d := e2edeploy.NewDeployment(deploymentName, replicas, podLabels, WebserverImageName, WebserverImage, appsv1.RollingUpdateDeploymentStrategyType)
	d.Spec.ProgressDeadlineSeconds = &thirty
	d.Spec.RevisionHistoryLimit = &two
	d.Spec.Template.Spec.TerminationGracePeriodSeconds = &zero
	framework.Logf("Creating deployment %q", deploymentName)
	deployment, err := c.AppsV1().Deployments(ns).Create(d)
	framework.ExpectNoError(err)

	iterations := 20
	for i := 0; i < iterations; i++ {
		if r := rand.Float32(); r < 0.6 {
			time.Sleep(time.Duration(float32(i) * r * float32(time.Second)))
		}

		switch n := rand.Float32(); {
		case n < 0.2:
			// trigger a new deployment
			framework.Logf("%02d: triggering a new rollout for deployment %q", i, deployment.Name)
			deployment, err = e2edeploy.UpdateDeploymentWithRetries(c, ns, deployment.Name, func(update *appsv1.Deployment) {
				newEnv := v1.EnvVar{Name: "A", Value: fmt.Sprintf("%d", i)}
				update.Spec.Template.Spec.Containers[0].Env = append(update.Spec.Template.Spec.Containers[0].Env, newEnv)
				randomScale(update, i)
			})
			framework.ExpectNoError(err)

		case n < 0.4:
			// rollback to the previous version
			framework.Logf("%02d: rolling back a rollout for deployment %q", i, deployment.Name)
			deployment, err = e2edeploy.UpdateDeploymentWithRetries(c, ns, deployment.Name, func(update *appsv1.Deployment) {
				if update.Annotations == nil {
					update.Annotations = make(map[string]string)
				}
				update.Annotations[appsv1.DeprecatedRollbackTo] = "0"
			})
			framework.ExpectNoError(err)

		case n < 0.6:
			// just scaling
			framework.Logf("%02d: scaling deployment %q", i, deployment.Name)
			deployment, err = e2edeploy.UpdateDeploymentWithRetries(c, ns, deployment.Name, func(update *appsv1.Deployment) {
				randomScale(update, i)
			})
			framework.ExpectNoError(err)

		case n < 0.8:
			// toggling the deployment
			if deployment.Spec.Paused {
				framework.Logf("%02d: pausing deployment %q", i, deployment.Name)
				deployment, err = e2edeploy.UpdateDeploymentWithRetries(c, ns, deployment.Name, func(update *appsv1.Deployment) {
					update.Spec.Paused = true
					randomScale(update, i)
				})
				framework.ExpectNoError(err)
			} else {
				framework.Logf("%02d: resuming deployment %q", i, deployment.Name)
				deployment, err = e2edeploy.UpdateDeploymentWithRetries(c, ns, deployment.Name, func(update *appsv1.Deployment) {
					update.Spec.Paused = false
					randomScale(update, i)
				})
				framework.ExpectNoError(err)
			}

		default:
			// arbitrarily delete deployment pods
			framework.Logf("%02d: arbitrarily deleting one or more deployment pods for deployment %q", i, deployment.Name)
			selector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
			framework.ExpectNoError(err)
			opts := metav1.ListOptions{LabelSelector: selector.String()}
			podList, err := c.CoreV1().Pods(ns).List(opts)
			framework.ExpectNoError(err)
			if len(podList.Items) == 0 {
				framework.Logf("%02d: no deployment pods to delete", i)
				continue
			}
			for p := range podList.Items {
				if rand.Float32() < 0.5 {
					continue
				}
				name := podList.Items[p].Name
				framework.Logf("%02d: deleting deployment pod %q", i, name)
				err := c.CoreV1().Pods(ns).Delete(name, nil)
				if err != nil && !errors.IsNotFound(err) {
					framework.ExpectNoError(err)
				}
			}
		}
	}

	// unpause the deployment if we end up pausing it
	deployment, err = c.AppsV1().Deployments(ns).Get(deployment.Name, metav1.GetOptions{})
	framework.ExpectNoError(err)
	if deployment.Spec.Paused {
		deployment, err = e2edeploy.UpdateDeploymentWithRetries(c, ns, deployment.Name, func(update *appsv1.Deployment) {
			update.Spec.Paused = false
		})
	}

	framework.Logf("Waiting for deployment %q to be observed by the controller", deploymentName)
	err = e2edeploy.WaitForObservedDeployment(c, ns, deploymentName, deployment.Generation)
	framework.ExpectNoError(err)

	framework.Logf("Waiting for deployment %q status", deploymentName)
	err = e2edeploy.WaitForDeploymentComplete(c, deployment)
	framework.ExpectNoError(err)

	framework.Logf("Checking deployment %q for a complete condition", deploymentName)
	err = e2edeploy.WaitForDeploymentWithCondition(c, ns, deploymentName, deploymentutil.NewRSAvailableReason, appsv1.DeploymentProgressing)
	framework.ExpectNoError(err)
}

func testDeploymentsControllerRef(f *framework.Framework) {
	ns := f.Namespace.Name
	c := f.ClientSet

	deploymentName := "test-orphan-deployment"
	framework.Logf("Creating Deployment %q", deploymentName)
	podLabels := map[string]string{"name": WebserverImageName}
	replicas := int32(1)
	d := e2edeploy.NewDeployment(deploymentName, replicas, podLabels, WebserverImageName, WebserverImage, appsv1.RollingUpdateDeploymentStrategyType)
	deploy, err := c.AppsV1().Deployments(ns).Create(d)
	framework.ExpectNoError(err)
	err = e2edeploy.WaitForDeploymentComplete(c, deploy)
	framework.ExpectNoError(err)

	framework.Logf("Verifying Deployment %q has only one ReplicaSet", deploymentName)
	rsList := listDeploymentReplicaSets(c, ns, podLabels)
	framework.ExpectEqual(len(rsList.Items), 1)

	framework.Logf("Obtaining the ReplicaSet's UID")
	orphanedRSUID := rsList.Items[0].UID

	framework.Logf("Checking the ReplicaSet has the right controllerRef")
	err = checkDeploymentReplicaSetsControllerRef(c, ns, deploy.UID, podLabels)
	framework.ExpectNoError(err)

	framework.Logf("Deleting Deployment %q and orphaning its ReplicaSet", deploymentName)
	err = orphanDeploymentReplicaSets(c, deploy)
	framework.ExpectNoError(err)

	ginkgo.By("Wait for the ReplicaSet to be orphaned")
	err = wait.Poll(dRetryPeriod, dRetryTimeout, waitDeploymentReplicaSetsOrphaned(c, ns, podLabels))
	framework.ExpectNoError(err, "error waiting for Deployment ReplicaSet to be orphaned")

	deploymentName = "test-adopt-deployment"
	framework.Logf("Creating Deployment %q to adopt the ReplicaSet", deploymentName)
	d = e2edeploy.NewDeployment(deploymentName, replicas, podLabels, WebserverImageName, WebserverImage, appsv1.RollingUpdateDeploymentStrategyType)
	deploy, err = c.AppsV1().Deployments(ns).Create(d)
	framework.ExpectNoError(err)
	err = e2edeploy.WaitForDeploymentComplete(c, deploy)
	framework.ExpectNoError(err)

	framework.Logf("Waiting for the ReplicaSet to have the right controllerRef")
	err = checkDeploymentReplicaSetsControllerRef(c, ns, deploy.UID, podLabels)
	framework.ExpectNoError(err)

	framework.Logf("Verifying no extra ReplicaSet is created (Deployment %q still has only one ReplicaSet after adoption)", deploymentName)
	rsList = listDeploymentReplicaSets(c, ns, podLabels)
	framework.ExpectEqual(len(rsList.Items), 1)

	framework.Logf("Verifying the ReplicaSet has the same UID as the orphaned ReplicaSet")
	framework.ExpectEqual(rsList.Items[0].UID, orphanedRSUID)
}

// testProportionalScalingDeployment tests that when a RollingUpdate Deployment is scaled in the middle
// of a rollout (either in progress or paused), then the Deployment will balance additional replicas
// in existing active ReplicaSets (ReplicaSets with more than 0 replica) in order to mitigate risk.
func testProportionalScalingDeployment(f *framework.Framework) {
	ns := f.Namespace.Name
	c := f.ClientSet

	podLabels := map[string]string{"name": WebserverImageName}
	replicas := int32(10)

	// Create a webserver deployment.
	deploymentName := "webserver-deployment"
	d := e2edeploy.NewDeployment(deploymentName, replicas, podLabels, WebserverImageName, WebserverImage, appsv1.RollingUpdateDeploymentStrategyType)
	d.Spec.Strategy.RollingUpdate = new(appsv1.RollingUpdateDeployment)
	d.Spec.Strategy.RollingUpdate.MaxSurge = intOrStrP(3)
	d.Spec.Strategy.RollingUpdate.MaxUnavailable = intOrStrP(2)

	framework.Logf("Creating deployment %q", deploymentName)
	deployment, err := c.AppsV1().Deployments(ns).Create(d)
	framework.ExpectNoError(err)

	framework.Logf("Waiting for observed generation %d", deployment.Generation)
	err = e2edeploy.WaitForObservedDeployment(c, ns, deploymentName, deployment.Generation)
	framework.ExpectNoError(err)

	// Verify that the required pods have come up.
	framework.Logf("Waiting for all required pods to come up")
	err = e2epod.VerifyPodsRunning(c, ns, WebserverImageName, false, *(deployment.Spec.Replicas))
	framework.ExpectNoError(err, "error in waiting for pods to come up: %v", err)

	framework.Logf("Waiting for deployment %q to complete", deployment.Name)
	err = e2edeploy.WaitForDeploymentComplete(c, deployment)
	framework.ExpectNoError(err)

	firstRS, err := deploymentutil.GetNewReplicaSet(deployment, c.AppsV1())
	framework.ExpectNoError(err)

	// Update the deployment with a non-existent image so that the new replica set
	// will be blocked to simulate a partial rollout.
	framework.Logf("Updating deployment %q with a non-existent image", deploymentName)
	deployment, err = e2edeploy.UpdateDeploymentWithRetries(c, ns, d.Name, func(update *appsv1.Deployment) {
		update.Spec.Template.Spec.Containers[0].Image = "webserver:404"
	})
	framework.ExpectNoError(err)

	framework.Logf("Waiting for observed generation %d", deployment.Generation)
	err = e2edeploy.WaitForObservedDeployment(c, ns, deploymentName, deployment.Generation)
	framework.ExpectNoError(err)

	// Checking state of first rollout's replicaset.
	maxUnavailable, err := intstr.GetValueFromIntOrPercent(deployment.Spec.Strategy.RollingUpdate.MaxUnavailable, int(*(deployment.Spec.Replicas)), false)
	framework.ExpectNoError(err)

	// First rollout's replicaset should have Deployment's (replicas - maxUnavailable) = 10 - 2 = 8 available replicas.
	minAvailableReplicas := replicas - int32(maxUnavailable)
	framework.Logf("Waiting for the first rollout's replicaset to have .status.availableReplicas = %d", minAvailableReplicas)
	err = replicaset.WaitForReplicaSetTargetAvailableReplicas(c, firstRS, minAvailableReplicas)
	framework.ExpectNoError(err)

	// First rollout's replicaset should have .spec.replicas = 8 too.
	framework.Logf("Waiting for the first rollout's replicaset to have .spec.replicas = %d", minAvailableReplicas)
	err = replicaset.WaitForReplicaSetTargetSpecReplicas(c, firstRS, minAvailableReplicas)
	framework.ExpectNoError(err)

	// The desired replicas wait makes sure that the RS controller has created expected number of pods.
	framework.Logf("Waiting for the first rollout's replicaset of deployment %q to have desired number of replicas", deploymentName)
	firstRS, err = c.AppsV1().ReplicaSets(ns).Get(firstRS.Name, metav1.GetOptions{})
	framework.ExpectNoError(err)
	err = replicaset.WaitForReplicaSetDesiredReplicas(c.AppsV1(), firstRS)
	framework.ExpectNoError(err)

	// Checking state of second rollout's replicaset.
	secondRS, err := deploymentutil.GetNewReplicaSet(deployment, c.AppsV1())
	framework.ExpectNoError(err)

	maxSurge, err := intstr.GetValueFromIntOrPercent(deployment.Spec.Strategy.RollingUpdate.MaxSurge, int(*(deployment.Spec.Replicas)), false)
	framework.ExpectNoError(err)

	// Second rollout's replicaset should have 0 available replicas.
	framework.Logf("Verifying that the second rollout's replicaset has .status.availableReplicas = 0")
	framework.ExpectEqual(secondRS.Status.AvailableReplicas, int32(0))

	// Second rollout's replicaset should have Deployment's (replicas + maxSurge - first RS's replicas) = 10 + 3 - 8 = 5 for .spec.replicas.
	newReplicas := replicas + int32(maxSurge) - minAvailableReplicas
	framework.Logf("Waiting for the second rollout's replicaset to have .spec.replicas = %d", newReplicas)
	err = replicaset.WaitForReplicaSetTargetSpecReplicas(c, secondRS, newReplicas)
	framework.ExpectNoError(err)

	// The desired replicas wait makes sure that the RS controller has created expected number of pods.
	framework.Logf("Waiting for the second rollout's replicaset of deployment %q to have desired number of replicas", deploymentName)
	secondRS, err = c.AppsV1().ReplicaSets(ns).Get(secondRS.Name, metav1.GetOptions{})
	framework.ExpectNoError(err)
	err = replicaset.WaitForReplicaSetDesiredReplicas(c.AppsV1(), secondRS)
	framework.ExpectNoError(err)

	// Check the deployment's minimum availability.
	framework.Logf("Verifying that deployment %q has minimum required number of available replicas", deploymentName)
	if deployment.Status.AvailableReplicas < minAvailableReplicas {
		err = fmt.Errorf("observed %d available replicas, less than min required %d", deployment.Status.AvailableReplicas, minAvailableReplicas)
		framework.ExpectNoError(err)
	}

	// Scale the deployment to 30 replicas.
	newReplicas = int32(30)
	framework.Logf("Scaling up the deployment %q from %d to %d", deploymentName, replicas, newReplicas)
	deployment, err = e2edeploy.UpdateDeploymentWithRetries(c, ns, deployment.Name, func(update *appsv1.Deployment) {
		update.Spec.Replicas = &newReplicas
	})
	framework.ExpectNoError(err)

	framework.Logf("Waiting for the replicasets of deployment %q to have desired number of replicas", deploymentName)
	firstRS, err = c.AppsV1().ReplicaSets(ns).Get(firstRS.Name, metav1.GetOptions{})
	framework.ExpectNoError(err)
	secondRS, err = c.AppsV1().ReplicaSets(ns).Get(secondRS.Name, metav1.GetOptions{})
	framework.ExpectNoError(err)

	// First rollout's replicaset should have .spec.replicas = 8 + (30-10)*(8/13) = 8 + 12 = 20 replicas.
	// Note that 12 comes from rounding (30-10)*(8/13) to nearest integer.
	framework.Logf("Verifying that first rollout's replicaset has .spec.replicas = 20")
	err = replicaset.WaitForReplicaSetTargetSpecReplicas(c, firstRS, 20)
	framework.ExpectNoError(err)

	// Second rollout's replicaset should have .spec.replicas = 5 + (30-10)*(5/13) = 5 + 8 = 13 replicas.
	// Note that 8 comes from rounding (30-10)*(5/13) to nearest integer.
	framework.Logf("Verifying that second rollout's replicaset has .spec.replicas = 13")
	err = replicaset.WaitForReplicaSetTargetSpecReplicas(c, secondRS, 13)
	framework.ExpectNoError(err)
}

func checkDeploymentReplicaSetsControllerRef(c clientset.Interface, ns string, uid types.UID, label map[string]string) error {
	rsList := listDeploymentReplicaSets(c, ns, label)
	for _, rs := range rsList.Items {
		// This rs is adopted only when its controller ref is update
		if controllerRef := metav1.GetControllerOf(&rs); controllerRef == nil || controllerRef.UID != uid {
			return fmt.Errorf("ReplicaSet %s has unexpected controllerRef %v", rs.Name, controllerRef)
		}
	}
	return nil
}

func waitDeploymentReplicaSetsOrphaned(c clientset.Interface, ns string, label map[string]string) func() (bool, error) {
	return func() (bool, error) {
		rsList := listDeploymentReplicaSets(c, ns, label)
		for _, rs := range rsList.Items {
			// This rs is orphaned only when controller ref is cleared
			if controllerRef := metav1.GetControllerOf(&rs); controllerRef != nil {
				return false, nil
			}
		}
		return true, nil
	}
}

func listDeploymentReplicaSets(c clientset.Interface, ns string, label map[string]string) *appsv1.ReplicaSetList {
	selector := labels.Set(label).AsSelector()
	options := metav1.ListOptions{LabelSelector: selector.String()}
	rsList, err := c.AppsV1().ReplicaSets(ns).List(options)
	framework.ExpectNoError(err)
	gomega.Expect(len(rsList.Items)).To(gomega.BeNumerically(">", 0))
	return rsList
}

func orphanDeploymentReplicaSets(c clientset.Interface, d *appsv1.Deployment) error {
	trueVar := true
	deleteOptions := &metav1.DeleteOptions{OrphanDependents: &trueVar}
	deleteOptions.Preconditions = metav1.NewUIDPreconditions(string(d.UID))
	return c.AppsV1().Deployments(d.Namespace).Delete(d.Name, deleteOptions)
}

func testRollingUpdateDeploymentWithLocalTrafficLoadBalancer(f *framework.Framework) {
	ns := f.Namespace.Name
	c := f.ClientSet

	name := "test-rolling-update-with-lb"
	framework.Logf("Creating Deployment %q", name)
	podLabels := map[string]string{"name": name}
	replicas := int32(3)
	d := e2edeploy.NewDeployment(name, replicas, podLabels, AgnhostImageName, AgnhostImage, appsv1.RollingUpdateDeploymentStrategyType)
	// NewDeployment assigned the same value to both d.Spec.Selector and
	// d.Spec.Template.Labels, so mutating the one would mutate the other.
	// Thus we need to set d.Spec.Template.Labels to a new value if we want
	// to mutate it alone.
	d.Spec.Template.Labels = map[string]string{
		"iteration": "0",
		"name":      name,
	}
	d.Spec.Template.Spec.Containers[0].Args = []string{"netexec", "--http-port=80", "--udp-port=80"}
	// To ensure that a node that had a local endpoint prior to a rolling
	// update continues to have a local endpoint throughout the rollout, we
	// need an affinity policy that will cause pods to be scheduled on the
	// same nodes as old pods, and we need the deployment to scale up a new
	// pod before deleting an old pod.  This affinity policy will define
	// inter-pod affinity for pods of different rollouts and anti-affinity
	// for pods of the same rollout, so it will need to be updated when
	// performing a rollout.
	setAffinities(d, false)
	d.Spec.Strategy.RollingUpdate = &appsv1.RollingUpdateDeployment{
		MaxSurge:       intOrStrP(1),
		MaxUnavailable: intOrStrP(0),
	}
	deployment, err := c.AppsV1().Deployments(ns).Create(d)
	framework.ExpectNoError(err)
	err = e2edeploy.WaitForDeploymentComplete(c, deployment)
	framework.ExpectNoError(err)

	framework.Logf("Creating a service %s with type=LoadBalancer and externalTrafficPolicy=Local in namespace %s", name, ns)
	jig := e2eservice.NewTestJig(c, ns, name)
	jig.Labels = podLabels
	service, err := jig.CreateLoadBalancerService(e2eservice.GetServiceLoadBalancerCreationTimeout(c), func(svc *v1.Service) {
		svc.Spec.ExternalTrafficPolicy = v1.ServiceExternalTrafficPolicyTypeLocal
	})
	framework.ExpectNoError(err)

	lbNameOrAddress := e2eservice.GetIngressPoint(&service.Status.LoadBalancer.Ingress[0])
	svcPort := int(service.Spec.Ports[0].Port)

	framework.Logf("Hitting the replica set's pods through the service's load balancer")
	timeout := e2eservice.LoadBalancerLagTimeoutDefault
	if framework.ProviderIs("aws") {
		timeout = e2eservice.LoadBalancerLagTimeoutAWS
	}
	e2eservice.TestReachableHTTP(lbNameOrAddress, svcPort, timeout)

	framework.Logf("Starting a goroutine to watch the service's endpoints in the background")
	done := make(chan struct{})
	failed := make(chan struct{})
	defer close(done)
	go func() {
		defer ginkgo.GinkgoRecover()
		expectedNodes, err := jig.GetEndpointNodeNames()
		framework.ExpectNoError(err)
		// The affinity policy should ensure that before an old pod is
		// deleted, a new pod will have been created on the same node.
		// Thus the set of nodes with local endpoints for the service
		// should remain unchanged.
		wait.Until(func() {
			actualNodes, err := jig.GetEndpointNodeNames()
			framework.ExpectNoError(err)
			if !actualNodes.Equal(expectedNodes) {
				framework.Logf("The set of nodes with local endpoints changed; started with %v, now have %v", expectedNodes.List(), actualNodes.List())
				failed <- struct{}{}
			}
		}, framework.Poll, done)
	}()

	framework.Logf("Triggering a rolling deployment several times")
	for i := 1; i <= 3; i++ {
		framework.Logf("Updating label deployment %q pod spec (iteration #%d)", name, i)
		deployment, err = e2edeploy.UpdateDeploymentWithRetries(c, ns, d.Name, func(update *appsv1.Deployment) {
			update.Spec.Template.Labels["iteration"] = fmt.Sprintf("%d", i)
			setAffinities(update, true)
		})
		framework.ExpectNoError(err)

		framework.Logf("Waiting for observed generation %d", deployment.Generation)
		err = e2edeploy.WaitForObservedDeployment(c, ns, name, deployment.Generation)
		framework.ExpectNoError(err)

		framework.Logf("Make sure deployment %q is complete", name)
		err = e2edeploy.WaitForDeploymentCompleteAndCheckRolling(c, deployment)
		framework.ExpectNoError(err)
	}

	select {
	case <-failed:
		framework.Failf("Connectivity to the load balancer was interrupted")
	case <-time.After(1 * time.Minute):
	}
}

// setAffinities set PodAntiAffinity across pods from the same generation
// of Deployment and if, explicitly requested, also affinity with pods
// from other generations.
// It is required to make those "Required" so that in large clusters where
// scheduler may not score all nodes if a lot of them are feasible, the
// test will also have a chance to pass.
func setAffinities(d *appsv1.Deployment, setAffinity bool) {
	affinity := &v1.Affinity{
		PodAntiAffinity: &v1.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
				{
					TopologyKey: "kubernetes.io/hostname",
					LabelSelector: &metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "name",
								Operator: metav1.LabelSelectorOpIn,
								Values:   []string{d.Spec.Template.Labels["name"]},
							},
							{
								Key:      "iteration",
								Operator: metav1.LabelSelectorOpIn,
								Values:   []string{d.Spec.Template.Labels["iteration"]},
							},
						},
					},
				},
			},
		},
	}
	if setAffinity {
		affinity.PodAffinity = &v1.PodAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
				{
					TopologyKey: "kubernetes.io/hostname",
					LabelSelector: &metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "name",
								Operator: metav1.LabelSelectorOpIn,
								Values:   []string{d.Spec.Template.Labels["name"]},
							},
							{
								Key:      "iteration",
								Operator: metav1.LabelSelectorOpNotIn,
								Values:   []string{d.Spec.Template.Labels["iteration"]},
							},
						},
					},
				},
			},
		}
	}
	d.Spec.Template.Spec.Affinity = affinity
}
