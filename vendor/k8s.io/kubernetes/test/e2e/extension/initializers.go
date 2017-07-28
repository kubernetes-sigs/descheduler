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

package extension

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/kubernetes/pkg/api/v1"
	"k8s.io/kubernetes/pkg/apis/admissionregistration/v1alpha1"
	"k8s.io/kubernetes/pkg/client/clientset_generated/clientset"
	clientretry "k8s.io/kubernetes/pkg/client/retry"
	"k8s.io/kubernetes/test/e2e/framework"
)

var _ = framework.KubeDescribe("Initializers", func() {
	f := framework.NewDefaultFramework("initializers")

	// TODO: Add failure traps once we have JustAfterEach
	// See https://github.com/onsi/ginkgo/issues/303

	It("should be invisible to controllers by default", func() {
		ns := f.Namespace.Name
		c := f.ClientSet

		podName := "uninitialized-pod"
		framework.Logf("Creating pod %s", podName)

		ch := make(chan struct{})
		go func() {
			_, err := c.Core().Pods(ns).Create(newUninitializedPod(podName))
			Expect(err).NotTo(HaveOccurred())
			close(ch)
		}()

		// wait to ensure the scheduler does not act on an uninitialized pod
		err := wait.PollImmediate(2*time.Second, 15*time.Second, func() (bool, error) {
			p, err := c.Core().Pods(ns).Get(podName, metav1.GetOptions{})
			if err != nil {
				if errors.IsNotFound(err) {
					return false, nil
				}
				return false, err
			}
			return len(p.Spec.NodeName) > 0, nil
		})
		Expect(err).To(Equal(wait.ErrWaitTimeout))

		// verify that we can update an initializing pod
		pod, err := c.Core().Pods(ns).Get(podName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		pod.Annotations = map[string]string{"update-1": "test"}
		pod, err = c.Core().Pods(ns).Update(pod)
		Expect(err).NotTo(HaveOccurred())

		// verify the list call filters out uninitialized pods
		pods, err := c.Core().Pods(ns).List(metav1.ListOptions{IncludeUninitialized: true})
		Expect(err).NotTo(HaveOccurred())
		Expect(pods.Items).To(HaveLen(1))
		pods, err = c.Core().Pods(ns).List(metav1.ListOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(pods.Items).To(HaveLen(0))

		// clear initializers
		pod.Initializers = nil
		pod, err = c.Core().Pods(ns).Update(pod)
		Expect(err).NotTo(HaveOccurred())

		// pod should now start running
		err = framework.WaitForPodRunningInNamespace(c, pod)
		Expect(err).NotTo(HaveOccurred())

		// ensure create call returns
		<-ch

		// verify that we cannot start the pod initializing again
		pod, err = c.Core().Pods(ns).Get(podName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		pod.Initializers = &metav1.Initializers{
			Pending: []metav1.Initializer{{Name: "Other"}},
		}
		_, err = c.Core().Pods(ns).Update(pod)
		if !errors.IsInvalid(err) || !strings.Contains(err.Error(), "immutable") {
			Fail(fmt.Sprintf("expected invalid error: %v", err))
		}
	})

	It("should dynamically register and apply initializers to pods [Serial]", func() {
		ns := f.Namespace.Name
		c := f.ClientSet

		podName := "uninitialized-pod"
		framework.Logf("Creating pod %s", podName)

		// create and register an initializer
		initializerName := "pod.test.e2e.kubernetes.io"
		initializerConfigName := "e2e-test-initializer"
		_, err := c.AdmissionregistrationV1alpha1().InitializerConfigurations().Create(&v1alpha1.InitializerConfiguration{
			ObjectMeta: metav1.ObjectMeta{Name: initializerConfigName},
			Initializers: []v1alpha1.Initializer{
				{
					Name: initializerName,
					Rules: []v1alpha1.Rule{
						{APIGroups: []string{""}, APIVersions: []string{"*"}, Resources: []string{"pods"}},
					},
				},
			},
		})
		if errors.IsNotFound(err) {
			framework.Skipf("dynamic configuration of initializers requires the alpha admissionregistration.k8s.io group to be enabled")
		}
		Expect(err).NotTo(HaveOccurred())

		// we must remove the initializer when the test is complete and ensure no pods are pending for that initializer
		defer func() {
			if err := c.AdmissionregistrationV1alpha1().InitializerConfigurations().Delete(initializerConfigName, nil); err != nil && !errors.IsNotFound(err) {
				framework.Logf("got error on deleting %s", initializerConfigName)
			}
			// poller configuration is 1 second, wait at least that long
			time.Sleep(3 * time.Second)
			// clear our initializer from anyone who got it
			removeInitializersFromAllPods(c, initializerName)
		}()

		// poller configuration is 1 second, wait at least that long
		time.Sleep(3 * time.Second)

		// run create that blocks
		ch := make(chan struct{})
		go func() {
			defer close(ch)
			_, err := c.Core().Pods(ns).Create(newPod(podName))
			Expect(err).NotTo(HaveOccurred())
		}()

		// wait until the pod shows up uninitialized
		By("Waiting until the pod is visible to a client")
		var pod *v1.Pod
		err = wait.PollImmediate(2*time.Second, 15*time.Second, func() (bool, error) {
			pod, err = c.Core().Pods(ns).Get(podName, metav1.GetOptions{IncludeUninitialized: true})
			if errors.IsNotFound(err) {
				return false, nil
			}
			if err != nil {
				return false, err
			}
			return true, nil
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(pod.Initializers).NotTo(BeNil())
		Expect(pod.Initializers.Pending).To(HaveLen(1))
		Expect(pod.Initializers.Pending[0].Name).To(Equal(initializerName))

		// pretend we are an initializer
		By("Completing initialization")
		pod.Initializers = nil
		pod, err = c.Core().Pods(ns).Update(pod)
		Expect(err).NotTo(HaveOccurred())

		// ensure create call returns
		<-ch

		// pod should now start running
		err = framework.WaitForPodRunningInNamespace(c, pod)
		Expect(err).NotTo(HaveOccurred())

		// bypass initialization by explicitly passing an empty pending list
		By("Setting an empty initializer as an admin to bypass initialization")
		podName = "preinitialized-pod"
		pod = newUninitializedPod(podName)
		pod.Initializers.Pending = nil
		pod, err = c.Core().Pods(ns).Create(pod)
		Expect(err).NotTo(HaveOccurred())
		Expect(pod.Initializers).To(BeNil())

		// bypass initialization for mirror pods
		By("Creating a mirror pod that bypasses initialization")
		podName = "mirror-pod"
		pod = newPod(podName)
		pod.Annotations = map[string]string{
			v1.MirrorPodAnnotationKey: "true",
		}
		pod.Spec.NodeName = "node-does-not-yet-exist"
		pod, err = c.Core().Pods(ns).Create(pod)
		Expect(err).NotTo(HaveOccurred())
		Expect(pod.Initializers).To(BeNil())
		Expect(pod.Annotations[v1.MirrorPodAnnotationKey]).To(Equal("true"))
	})
})

func newUninitializedPod(podName string) *v1.Pod {
	pod := newPod(podName)
	pod.Initializers = &metav1.Initializers{
		Pending: []metav1.Initializer{{Name: "Test"}},
	}
	return pod
}

func newPod(podName string) *v1.Pod {
	containerName := fmt.Sprintf("%s-container", podName)
	port := 8080
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: podName,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  containerName,
					Image: "gcr.io/google_containers/porter:4524579c0eb935c056c8e75563b4e1eda31587e0",
					Env:   []v1.EnvVar{{Name: fmt.Sprintf("SERVE_PORT_%d", port), Value: "foo"}},
					Ports: []v1.ContainerPort{{ContainerPort: int32(port)}},
				},
			},
			RestartPolicy: v1.RestartPolicyNever,
		},
	}
	return pod
}

// removeInitializersFromAllPods walks all pods and ensures they don't have the provided initializer,
// to guarantee completing the test doesn't block the entire cluster.
func removeInitializersFromAllPods(c clientset.Interface, initializerName string) {
	pods, err := c.Core().Pods("").List(metav1.ListOptions{IncludeUninitialized: true})
	if err != nil {
		return
	}
	for _, p := range pods.Items {
		if p.Initializers == nil {
			continue
		}
		err := clientretry.RetryOnConflict(clientretry.DefaultRetry, func() error {
			pod, err := c.Core().Pods(p.Namespace).Get(p.Name, metav1.GetOptions{IncludeUninitialized: true})
			if err != nil {
				if errors.IsNotFound(err) {
					return nil
				}
				return err
			}
			if pod.Initializers == nil {
				return nil
			}
			var updated []metav1.Initializer
			for _, pending := range pod.Initializers.Pending {
				if pending.Name != initializerName {
					updated = append(updated, pending)
				}
			}
			if len(updated) == len(pod.Initializers.Pending) {
				return nil
			}
			pod.Initializers.Pending = updated
			if len(updated) == 0 {
				pod.Initializers = nil
			}
			framework.Logf("Found initializer on pod %s in ns %s", pod.Name, pod.Namespace)
			_, err = c.Core().Pods(p.Namespace).Update(pod)
			return err
		})
		if err != nil {
			framework.Logf("Unable to remove initializer from pod %s in ns %s: %v", p.Name, p.Namespace, err)
		}
	}
}
