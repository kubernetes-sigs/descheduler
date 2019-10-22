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

package common

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/kubernetes/test/e2e/framework"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

var _ = framework.KubeDescribe("Kubelet", func() {
	f := framework.NewDefaultFramework("kubelet-test")
	var podClient *framework.PodClient
	ginkgo.BeforeEach(func() {
		podClient = f.PodClient()
	})
	ginkgo.Context("when scheduling a busybox command in a pod", func() {
		podName := "busybox-scheduling-" + string(uuid.NewUUID())

		/*
			Release : v1.13
			Testname: Kubelet, log output, default
			Description: By default the stdout and stderr from the process being executed in a pod MUST be sent to the pod's logs.
		*/
		framework.ConformanceIt("should print the output to logs [NodeConformance]", func() {
			podClient.CreateSync(&v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: podName,
				},
				Spec: v1.PodSpec{
					// Don't restart the Pod since it is expected to exit
					RestartPolicy: v1.RestartPolicyNever,
					Containers: []v1.Container{
						{
							Image:   framework.BusyBoxImage,
							Name:    podName,
							Command: []string{"sh", "-c", "echo 'Hello World' ; sleep 240"},
						},
					},
				},
			})
			gomega.Eventually(func() string {
				sinceTime := metav1.NewTime(time.Now().Add(time.Duration(-1 * time.Hour)))
				rc, err := podClient.GetLogs(podName, &v1.PodLogOptions{SinceTime: &sinceTime}).Stream()
				if err != nil {
					return ""
				}
				defer rc.Close()
				buf := new(bytes.Buffer)
				buf.ReadFrom(rc)
				return buf.String()
			}, time.Minute, time.Second*4).Should(gomega.Equal("Hello World\n"))
		})
	})
	ginkgo.Context("when scheduling a busybox command that always fails in a pod", func() {
		var podName string

		ginkgo.BeforeEach(func() {
			podName = "bin-false" + string(uuid.NewUUID())
			podClient.Create(&v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: podName,
				},
				Spec: v1.PodSpec{
					// Don't restart the Pod since it is expected to exit
					RestartPolicy: v1.RestartPolicyNever,
					Containers: []v1.Container{
						{
							Image:   framework.BusyBoxImage,
							Name:    podName,
							Command: []string{"/bin/false"},
						},
					},
				},
			})
		})

		/*
			Release : v1.13
			Testname: Kubelet, failed pod, terminated reason
			Description: Create a Pod with terminated state. Pod MUST have only one container. Container MUST be in terminated state and MUST have an terminated reason.
		*/
		framework.ConformanceIt("should have an terminated reason [NodeConformance]", func() {
			gomega.Eventually(func() error {
				podData, err := podClient.Get(podName, metav1.GetOptions{})
				if err != nil {
					return err
				}
				if len(podData.Status.ContainerStatuses) != 1 {
					return fmt.Errorf("expected only one container in the pod %q", podName)
				}
				contTerminatedState := podData.Status.ContainerStatuses[0].State.Terminated
				if contTerminatedState == nil {
					return fmt.Errorf("expected state to be terminated. Got pod status: %+v", podData.Status)
				}
				if contTerminatedState.ExitCode == 0 || contTerminatedState.Reason == "" {
					return fmt.Errorf("expected non-zero exitCode and non-empty terminated state reason. Got exitCode: %+v and terminated state reason: %+v", contTerminatedState.ExitCode, contTerminatedState.Reason)
				}
				return nil
			}, time.Minute, time.Second*4).Should(gomega.BeNil())
		})

		/*
			Release : v1.13
			Testname: Kubelet, failed pod, delete
			Description: Create a Pod with terminated state. This terminated pod MUST be able to be deleted.
		*/
		framework.ConformanceIt("should be possible to delete [NodeConformance]", func() {
			err := podClient.Delete(podName, &metav1.DeleteOptions{})
			gomega.Expect(err).To(gomega.BeNil(), fmt.Sprintf("Error deleting Pod %v", err))
		})
	})
	ginkgo.Context("when scheduling a busybox Pod with hostAliases", func() {
		podName := "busybox-host-aliases" + string(uuid.NewUUID())

		/*
			Release : v1.13
			Testname: Kubelet, hostAliases
			Description: Create a Pod with hostAliases and a container with command to output /etc/hosts entries. Pod's logs MUST have matching entries of specified hostAliases to the output of /etc/hosts entries.
			Kubernetes mounts the /etc/hosts file into its containers, however, mounting individual files is not supported on Windows Containers. For this reason, this test is marked LinuxOnly.
		*/
		framework.ConformanceIt("should write entries to /etc/hosts [LinuxOnly] [NodeConformance]", func() {
			podClient.CreateSync(&v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: podName,
				},
				Spec: v1.PodSpec{
					// Don't restart the Pod since it is expected to exit
					RestartPolicy: v1.RestartPolicyNever,
					Containers: []v1.Container{
						{
							Image:   framework.BusyBoxImage,
							Name:    podName,
							Command: []string{"/bin/sh", "-c", "cat /etc/hosts; sleep 6000"},
						},
					},
					HostAliases: []v1.HostAlias{
						{
							IP:        "123.45.67.89",
							Hostnames: []string{"foo", "bar"},
						},
					},
				},
			})

			gomega.Eventually(func() error {
				rc, err := podClient.GetLogs(podName, &v1.PodLogOptions{}).Stream()
				if err != nil {
					return err
				}
				defer rc.Close()
				buf := new(bytes.Buffer)
				buf.ReadFrom(rc)
				hostsFileContent := buf.String()

				if !strings.Contains(hostsFileContent, "123.45.67.89\tfoo\tbar") {
					return fmt.Errorf("expected hosts file to contain entries from HostAliases. Got:\n%+v", hostsFileContent)
				}

				return nil
			}, time.Minute, time.Second*4).Should(gomega.BeNil())
		})
	})
	ginkgo.Context("when scheduling a read only busybox container", func() {
		podName := "busybox-readonly-fs" + string(uuid.NewUUID())

		/*
			Release : v1.13
			Testname: Kubelet, pod with read only root file system
			Description: Create a Pod with security context set with ReadOnlyRootFileSystem set to true. The Pod then tries to write to the /file on the root, write operation to the root filesystem MUST fail as expected.
			This test is marked LinuxOnly since Windows does not support creating containers with read-only access.
		*/
		framework.ConformanceIt("should not write to root filesystem [LinuxOnly] [NodeConformance]", func() {
			isReadOnly := true
			podClient.CreateSync(&v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: podName,
				},
				Spec: v1.PodSpec{
					// Don't restart the Pod since it is expected to exit
					RestartPolicy: v1.RestartPolicyNever,
					Containers: []v1.Container{
						{
							Image:   framework.BusyBoxImage,
							Name:    podName,
							Command: []string{"/bin/sh", "-c", "echo test > /file; sleep 240"},
							SecurityContext: &v1.SecurityContext{
								ReadOnlyRootFilesystem: &isReadOnly,
							},
						},
					},
				},
			})
			gomega.Eventually(func() string {
				rc, err := podClient.GetLogs(podName, &v1.PodLogOptions{}).Stream()
				if err != nil {
					return ""
				}
				defer rc.Close()
				buf := new(bytes.Buffer)
				buf.ReadFrom(rc)
				return buf.String()
			}, time.Minute, time.Second*4).Should(gomega.Equal("/bin/sh: can't create /file: Read-only file system\n"))
		})
	})
})
