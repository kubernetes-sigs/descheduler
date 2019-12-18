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
	"io"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/websocket"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	watchtools "k8s.io/client-go/tools/watch"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	"k8s.io/kubernetes/pkg/kubelet"
	"k8s.io/kubernetes/test/e2e/framework"
	e2ekubelet "k8s.io/kubernetes/test/e2e/framework/kubelet"
	imageutils "k8s.io/kubernetes/test/utils/image"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

var (
	buildBackOffDuration = time.Minute
	syncLoopFrequency    = 10 * time.Second
	maxBackOffTolerance  = time.Duration(1.3 * float64(kubelet.MaxContainerBackOff))
)

// testHostIP tests that a pod gets a host IP
func testHostIP(podClient *framework.PodClient, pod *v1.Pod) {
	ginkgo.By("creating pod")
	podClient.CreateSync(pod)

	// Try to make sure we get a hostIP for each pod.
	hostIPTimeout := 2 * time.Minute
	t := time.Now()
	for {
		p, err := podClient.Get(pod.Name, metav1.GetOptions{})
		framework.ExpectNoError(err, "Failed to get pod %q", pod.Name)
		if p.Status.HostIP != "" {
			framework.Logf("Pod %s has hostIP: %s", p.Name, p.Status.HostIP)
			break
		}
		if time.Since(t) >= hostIPTimeout {
			framework.Failf("Gave up waiting for hostIP of pod %s after %v seconds",
				p.Name, time.Since(t).Seconds())
		}
		framework.Logf("Retrying to get the hostIP of pod %s", p.Name)
		time.Sleep(5 * time.Second)
	}
}

func startPodAndGetBackOffs(podClient *framework.PodClient, pod *v1.Pod, sleepAmount time.Duration) (time.Duration, time.Duration) {
	podClient.CreateSync(pod)
	time.Sleep(sleepAmount)
	gomega.Expect(pod.Spec.Containers).NotTo(gomega.BeEmpty())
	podName := pod.Name
	containerName := pod.Spec.Containers[0].Name

	ginkgo.By("getting restart delay-0")
	_, err := getRestartDelay(podClient, podName, containerName)
	if err != nil {
		framework.Failf("timed out waiting for container restart in pod=%s/%s", podName, containerName)
	}

	ginkgo.By("getting restart delay-1")
	delay1, err := getRestartDelay(podClient, podName, containerName)
	if err != nil {
		framework.Failf("timed out waiting for container restart in pod=%s/%s", podName, containerName)
	}

	ginkgo.By("getting restart delay-2")
	delay2, err := getRestartDelay(podClient, podName, containerName)
	if err != nil {
		framework.Failf("timed out waiting for container restart in pod=%s/%s", podName, containerName)
	}
	return delay1, delay2
}

func getRestartDelay(podClient *framework.PodClient, podName string, containerName string) (time.Duration, error) {
	beginTime := time.Now()
	var previousRestartCount int32 = -1
	var previousFinishedAt time.Time
	for time.Since(beginTime) < (2 * maxBackOffTolerance) { // may just miss the 1st MaxContainerBackOff delay
		time.Sleep(time.Second)
		pod, err := podClient.Get(podName, metav1.GetOptions{})
		framework.ExpectNoError(err, fmt.Sprintf("getting pod %s", podName))
		status, ok := podutil.GetContainerStatus(pod.Status.ContainerStatuses, containerName)
		if !ok {
			framework.Logf("getRestartDelay: status missing")
			continue
		}

		// the only case this happens is if this is the first time the Pod is running and there is no "Last State".
		if status.LastTerminationState.Terminated == nil {
			framework.Logf("Container's last state is not \"Terminated\".")
			continue
		}

		if previousRestartCount == -1 {
			if status.State.Running != nil {
				// container is still Running, there is no "FinishedAt" time.
				continue
			} else if status.State.Terminated != nil {
				previousFinishedAt = status.State.Terminated.FinishedAt.Time
			} else {
				previousFinishedAt = status.LastTerminationState.Terminated.FinishedAt.Time
			}
			previousRestartCount = status.RestartCount
		}

		// when the RestartCount is changed, the Containers will be in one of the following states:
		//Running, Terminated, Waiting (it already is waiting for the backoff period to expire, and the last state details have been stored into status.LastTerminationState).
		if status.RestartCount > previousRestartCount {
			var startedAt time.Time
			if status.State.Running != nil {
				startedAt = status.State.Running.StartedAt.Time
			} else if status.State.Terminated != nil {
				startedAt = status.State.Terminated.StartedAt.Time
			} else {
				startedAt = status.LastTerminationState.Terminated.StartedAt.Time
			}
			framework.Logf("getRestartDelay: restartCount = %d, finishedAt=%s restartedAt=%s (%s)", status.RestartCount, previousFinishedAt, startedAt, startedAt.Sub(previousFinishedAt))
			return startedAt.Sub(previousFinishedAt), nil
		}
	}
	return 0, fmt.Errorf("timeout getting pod restart delay")
}

// expectNoErrorWithRetries checks if an error occurs with the given retry count.
func expectNoErrorWithRetries(fn func() error, maxRetries int, explain ...interface{}) {
	var err error
	for i := 0; i < maxRetries; i++ {
		err = fn()
		if err == nil {
			return
		}
		framework.Logf("(Attempt %d of %d) Unexpected error occurred: %v", i+1, maxRetries, err)
	}
	if err != nil {
		debug.PrintStack()
	}
	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred(), explain...)
}

var _ = framework.KubeDescribe("Pods", func() {
	f := framework.NewDefaultFramework("pods")
	var podClient *framework.PodClient
	ginkgo.BeforeEach(func() {
		podClient = f.PodClient()
	})

	/*
		Release : v1.9
		Testname: Pods, assigned hostip
		Description: Create a Pod. Pod status MUST return successfully and contains a valid IP address.
	*/
	framework.ConformanceIt("should get a host IP [NodeConformance]", func() {
		name := "pod-hostip-" + string(uuid.NewUUID())
		testHostIP(podClient, &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name:  "test",
						Image: imageutils.GetPauseImageName(),
					},
				},
			},
		})
	})

	/*
		Release : v1.9
		Testname: Pods, lifecycle
		Description: A Pod is created with a unique label. Pod MUST be accessible when queried using the label selector upon creation. Add a watch, check if the Pod is running. Pod then deleted, The pod deletion timestamp is observed. The watch MUST return the pod deleted event. Query with the original selector for the Pod MUST return empty list.
	*/
	framework.ConformanceIt("should be submitted and removed [NodeConformance]", func() {
		ginkgo.By("creating the pod")
		name := "pod-submit-remove-" + string(uuid.NewUUID())
		value := strconv.Itoa(time.Now().Nanosecond())
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
				Labels: map[string]string{
					"name": "foo",
					"time": value,
				},
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name:  "nginx",
						Image: imageutils.GetE2EImage(imageutils.Nginx),
					},
				},
			},
		}

		ginkgo.By("setting up watch")
		selector := labels.SelectorFromSet(labels.Set(map[string]string{"time": value}))
		options := metav1.ListOptions{LabelSelector: selector.String()}
		pods, err := podClient.List(options)
		framework.ExpectNoError(err, "failed to query for pods")
		framework.ExpectEqual(len(pods.Items), 0)
		options = metav1.ListOptions{
			LabelSelector:   selector.String(),
			ResourceVersion: pods.ListMeta.ResourceVersion,
		}

		listCompleted := make(chan bool, 1)
		lw := &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				options.LabelSelector = selector.String()
				podList, err := podClient.List(options)
				if err == nil {
					select {
					case listCompleted <- true:
						framework.Logf("observed the pod list")
						return podList, err
					default:
						framework.Logf("channel blocked")
					}
				}
				return podList, err
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				options.LabelSelector = selector.String()
				return podClient.Watch(options)
			},
		}
		_, _, w, _ := watchtools.NewIndexerInformerWatcher(lw, &v1.Pod{})
		defer w.Stop()

		ginkgo.By("submitting the pod to kubernetes")
		podClient.Create(pod)

		ginkgo.By("verifying the pod is in kubernetes")
		selector = labels.SelectorFromSet(labels.Set(map[string]string{"time": value}))
		options = metav1.ListOptions{LabelSelector: selector.String()}
		pods, err = podClient.List(options)
		framework.ExpectNoError(err, "failed to query for pods")
		framework.ExpectEqual(len(pods.Items), 1)

		ginkgo.By("verifying pod creation was observed")
		select {
		case <-listCompleted:
			select {
			case event := <-w.ResultChan():
				if event.Type != watch.Added {
					framework.Failf("Failed to observe pod creation: %v", event)
				}
			case <-time.After(framework.PodStartTimeout):
				framework.Failf("Timeout while waiting for pod creation")
			}
		case <-time.After(10 * time.Second):
			framework.Failf("Timeout while waiting to observe pod list")
		}

		// We need to wait for the pod to be running, otherwise the deletion
		// may be carried out immediately rather than gracefully.
		framework.ExpectNoError(f.WaitForPodRunning(pod.Name))
		// save the running pod
		pod, err = podClient.Get(pod.Name, metav1.GetOptions{})
		framework.ExpectNoError(err, "failed to GET scheduled pod")

		ginkgo.By("deleting the pod gracefully")
		err = podClient.Delete(pod.Name, metav1.NewDeleteOptions(30))
		framework.ExpectNoError(err, "failed to delete pod")

		ginkgo.By("verifying the kubelet observed the termination notice")
		err = wait.Poll(time.Second*5, time.Second*30, func() (bool, error) {
			podList, err := e2ekubelet.GetKubeletPods(f.ClientSet, pod.Spec.NodeName)
			if err != nil {
				framework.Logf("Unable to retrieve kubelet pods for node %v: %v", pod.Spec.NodeName, err)
				return false, nil
			}
			for _, kubeletPod := range podList.Items {
				if pod.Name != kubeletPod.Name {
					continue
				}
				if kubeletPod.ObjectMeta.DeletionTimestamp == nil {
					framework.Logf("deletion has not yet been observed")
					return false, nil
				}
				return true, nil
			}
			framework.Logf("no pod exists with the name we were looking for, assuming the termination request was observed and completed")
			return true, nil
		})
		framework.ExpectNoError(err, "kubelet never observed the termination notice")

		ginkgo.By("verifying pod deletion was observed")
		deleted := false
		var lastPod *v1.Pod
		timer := time.After(framework.DefaultPodDeletionTimeout)
		for !deleted {
			select {
			case event := <-w.ResultChan():
				switch event.Type {
				case watch.Deleted:
					lastPod = event.Object.(*v1.Pod)
					deleted = true
				case watch.Error:
					framework.Logf("received a watch error: %v", event.Object)
					framework.Failf("watch closed with error")
				}
			case <-timer:
				framework.Failf("timed out waiting for pod deletion")
			}
		}
		if !deleted {
			framework.Failf("Failed to observe pod deletion")
		}

		gomega.Expect(lastPod.DeletionTimestamp).ToNot(gomega.BeNil())
		gomega.Expect(lastPod.Spec.TerminationGracePeriodSeconds).ToNot(gomega.BeZero())

		selector = labels.SelectorFromSet(labels.Set(map[string]string{"time": value}))
		options = metav1.ListOptions{LabelSelector: selector.String()}
		pods, err = podClient.List(options)
		framework.ExpectNoError(err, "failed to query for pods")
		framework.ExpectEqual(len(pods.Items), 0)
	})

	/*
		Release : v1.9
		Testname: Pods, update
		Description: Create a Pod with a unique label. Query for the Pod with the label as selector MUST be successful. Update the pod to change the value of the Label. Query for the Pod with the new value for the label MUST be successful.
	*/
	framework.ConformanceIt("should be updated [NodeConformance]", func() {
		ginkgo.By("creating the pod")
		name := "pod-update-" + string(uuid.NewUUID())
		value := strconv.Itoa(time.Now().Nanosecond())
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
				Labels: map[string]string{
					"name": "foo",
					"time": value,
				},
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name:  "nginx",
						Image: imageutils.GetE2EImage(imageutils.Nginx),
					},
				},
			},
		}

		ginkgo.By("submitting the pod to kubernetes")
		pod = podClient.CreateSync(pod)

		ginkgo.By("verifying the pod is in kubernetes")
		selector := labels.SelectorFromSet(labels.Set(map[string]string{"time": value}))
		options := metav1.ListOptions{LabelSelector: selector.String()}
		pods, err := podClient.List(options)
		framework.ExpectNoError(err, "failed to query for pods")
		framework.ExpectEqual(len(pods.Items), 1)

		ginkgo.By("updating the pod")
		podClient.Update(name, func(pod *v1.Pod) {
			value = strconv.Itoa(time.Now().Nanosecond())
			pod.Labels["time"] = value
		})

		framework.ExpectNoError(f.WaitForPodRunning(pod.Name))

		ginkgo.By("verifying the updated pod is in kubernetes")
		selector = labels.SelectorFromSet(labels.Set(map[string]string{"time": value}))
		options = metav1.ListOptions{LabelSelector: selector.String()}
		pods, err = podClient.List(options)
		framework.ExpectNoError(err, "failed to query for pods")
		framework.ExpectEqual(len(pods.Items), 1)
		framework.Logf("Pod update OK")
	})

	/*
		Release : v1.9
		Testname: Pods, ActiveDeadlineSeconds
		Description: Create a Pod with a unique label. Query for the Pod with the label as selector MUST be successful. The Pod is updated with ActiveDeadlineSeconds set on the Pod spec. Pod MUST terminate of the specified time elapses.
	*/
	framework.ConformanceIt("should allow activeDeadlineSeconds to be updated [NodeConformance]", func() {
		ginkgo.By("creating the pod")
		name := "pod-update-activedeadlineseconds-" + string(uuid.NewUUID())
		value := strconv.Itoa(time.Now().Nanosecond())
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
				Labels: map[string]string{
					"name": "foo",
					"time": value,
				},
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name:  "nginx",
						Image: imageutils.GetE2EImage(imageutils.Nginx),
					},
				},
			},
		}

		ginkgo.By("submitting the pod to kubernetes")
		podClient.CreateSync(pod)

		ginkgo.By("verifying the pod is in kubernetes")
		selector := labels.SelectorFromSet(labels.Set(map[string]string{"time": value}))
		options := metav1.ListOptions{LabelSelector: selector.String()}
		pods, err := podClient.List(options)
		framework.ExpectNoError(err, "failed to query for pods")
		framework.ExpectEqual(len(pods.Items), 1)

		ginkgo.By("updating the pod")
		podClient.Update(name, func(pod *v1.Pod) {
			newDeadline := int64(5)
			pod.Spec.ActiveDeadlineSeconds = &newDeadline
		})

		framework.ExpectNoError(f.WaitForPodTerminated(pod.Name, "DeadlineExceeded"))
	})

	/*
		Release : v1.9
		Testname: Pods, service environment variables
		Description: Create a server Pod listening on port 9376. A Service called fooservice is created for the server Pod listening on port 8765 targeting port 8080. If a new Pod is created in the cluster then the Pod MUST have the fooservice environment variables available from this new Pod. The new create Pod MUST have environment variables such as FOOSERVICE_SERVICE_HOST, FOOSERVICE_SERVICE_PORT, FOOSERVICE_PORT, FOOSERVICE_PORT_8765_TCP_PORT, FOOSERVICE_PORT_8765_TCP_PROTO, FOOSERVICE_PORT_8765_TCP and FOOSERVICE_PORT_8765_TCP_ADDR that are populated with proper values.
	*/
	framework.ConformanceIt("should contain environment variables for services [NodeConformance]", func() {
		// Make a pod that will be a service.
		// This pod serves its hostname via HTTP.
		serverName := "server-envvars-" + string(uuid.NewUUID())
		serverPod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:   serverName,
				Labels: map[string]string{"name": serverName},
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name:  "srv",
						Image: framework.ServeHostnameImage,
						Ports: []v1.ContainerPort{{ContainerPort: 9376}},
					},
				},
			},
		}
		podClient.CreateSync(serverPod)

		// This service exposes port 8080 of the test pod as a service on port 8765
		// TODO(filbranden): We would like to use a unique service name such as:
		//   svcName := "svc-envvars-" + randomSuffix()
		// However, that affects the name of the environment variables which are the capitalized
		// service name, so that breaks this test.  One possibility is to tweak the variable names
		// to match the service.  Another is to rethink environment variable names and possibly
		// allow overriding the prefix in the service manifest.
		svcName := "fooservice"
		svc := &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name: svcName,
				Labels: map[string]string{
					"name": svcName,
				},
			},
			Spec: v1.ServiceSpec{
				Ports: []v1.ServicePort{{
					Port:       8765,
					TargetPort: intstr.FromInt(8080),
				}},
				Selector: map[string]string{
					"name": serverName,
				},
			},
		}
		_, err := f.ClientSet.CoreV1().Services(f.Namespace.Name).Create(svc)
		framework.ExpectNoError(err, "failed to create service")

		// Make a client pod that verifies that it has the service environment variables.
		podName := "client-envvars-" + string(uuid.NewUUID())
		const containerName = "env3cont"
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:   podName,
				Labels: map[string]string{"name": podName},
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name:    containerName,
						Image:   imageutils.GetE2EImage(imageutils.BusyBox),
						Command: []string{"sh", "-c", "env"},
					},
				},
				RestartPolicy: v1.RestartPolicyNever,
			},
		}

		// It's possible for the Pod to be created before the Kubelet is updated with the new
		// service. In that case, we just retry.
		const maxRetries = 3
		expectedVars := []string{
			"FOOSERVICE_SERVICE_HOST=",
			"FOOSERVICE_SERVICE_PORT=",
			"FOOSERVICE_PORT=",
			"FOOSERVICE_PORT_8765_TCP_PORT=",
			"FOOSERVICE_PORT_8765_TCP_PROTO=",
			"FOOSERVICE_PORT_8765_TCP=",
			"FOOSERVICE_PORT_8765_TCP_ADDR=",
		}
		expectNoErrorWithRetries(func() error {
			return f.MatchContainerOutput(pod, containerName, expectedVars, gomega.ContainSubstring)
		}, maxRetries, "Container should have service environment variables set")
	})

	/*
		Release : v1.13
		Testname: Pods, remote command execution over websocket
		Description: A Pod is created. Websocket is created to retrieve exec command output from this pod.
		Message retrieved form Websocket MUST match with expected exec command output.
	*/
	framework.ConformanceIt("should support remote command execution over websockets [NodeConformance]", func() {
		config, err := framework.LoadConfig()
		framework.ExpectNoError(err, "unable to get base config")

		ginkgo.By("creating the pod")
		name := "pod-exec-websocket-" + string(uuid.NewUUID())
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name:    "main",
						Image:   imageutils.GetE2EImage(imageutils.BusyBox),
						Command: []string{"/bin/sh", "-c", "echo container is alive; sleep 600"},
					},
				},
			},
		}

		ginkgo.By("submitting the pod to kubernetes")
		pod = podClient.CreateSync(pod)

		req := f.ClientSet.CoreV1().RESTClient().Get().
			Namespace(f.Namespace.Name).
			Resource("pods").
			Name(pod.Name).
			Suffix("exec").
			Param("stderr", "1").
			Param("stdout", "1").
			Param("container", pod.Spec.Containers[0].Name).
			Param("command", "echo").
			Param("command", "remote execution test")

		url := req.URL()
		ws, err := framework.OpenWebSocketForURL(url, config, []string{"channel.k8s.io"})
		if err != nil {
			framework.Failf("Failed to open websocket to %s: %v", url.String(), err)
		}
		defer ws.Close()

		buf := &bytes.Buffer{}
		gomega.Eventually(func() error {
			for {
				var msg []byte
				if err := websocket.Message.Receive(ws, &msg); err != nil {
					if err == io.EOF {
						break
					}
					framework.Failf("Failed to read completely from websocket %s: %v", url.String(), err)
				}
				if len(msg) == 0 {
					continue
				}
				if msg[0] != 1 {
					if len(msg) == 1 {
						// skip an empty message on stream other than stdout
						continue
					} else {
						framework.Failf("Got message from server that didn't start with channel 1 (STDOUT): %v", msg)
					}

				}
				buf.Write(msg[1:])
			}
			if buf.Len() == 0 {
				return fmt.Errorf("unexpected output from server")
			}
			if !strings.Contains(buf.String(), "remote execution test") {
				return fmt.Errorf("expected to find 'remote execution test' in %q", buf.String())
			}
			return nil
		}, time.Minute, 10*time.Second).Should(gomega.BeNil())
	})

	/*
		Release : v1.13
		Testname: Pods, logs from websockets
		Description: A Pod is created. Websocket is created to retrieve log of a container from this pod.
		Message retrieved form Websocket MUST match with container's output.
	*/
	framework.ConformanceIt("should support retrieving logs from the container over websockets [NodeConformance]", func() {
		config, err := framework.LoadConfig()
		framework.ExpectNoError(err, "unable to get base config")

		ginkgo.By("creating the pod")
		name := "pod-logs-websocket-" + string(uuid.NewUUID())
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name:    "main",
						Image:   imageutils.GetE2EImage(imageutils.BusyBox),
						Command: []string{"/bin/sh", "-c", "echo container is alive; sleep 10000"},
					},
				},
			},
		}

		ginkgo.By("submitting the pod to kubernetes")
		podClient.CreateSync(pod)

		req := f.ClientSet.CoreV1().RESTClient().Get().
			Namespace(f.Namespace.Name).
			Resource("pods").
			Name(pod.Name).
			Suffix("log").
			Param("container", pod.Spec.Containers[0].Name)

		url := req.URL()

		ws, err := framework.OpenWebSocketForURL(url, config, []string{"binary.k8s.io"})
		if err != nil {
			framework.Failf("Failed to open websocket to %s: %v", url.String(), err)
		}
		defer ws.Close()
		buf := &bytes.Buffer{}
		for {
			var msg []byte
			if err := websocket.Message.Receive(ws, &msg); err != nil {
				if err == io.EOF {
					break
				}
				framework.Failf("Failed to read completely from websocket %s: %v", url.String(), err)
			}
			if len(strings.TrimSpace(string(msg))) == 0 {
				continue
			}
			buf.Write(msg)
		}
		if buf.String() != "container is alive\n" {
			framework.Failf("Unexpected websocket logs:\n%s", buf.String())
		}
	})

	// Slow (~7 mins)
	ginkgo.It("should have their auto-restart back-off timer reset on image update [Slow][NodeConformance]", func() {
		podName := "pod-back-off-image"
		containerName := "back-off"
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:   podName,
				Labels: map[string]string{"test": "back-off-image"},
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name:    containerName,
						Image:   imageutils.GetE2EImage(imageutils.BusyBox),
						Command: []string{"/bin/sh", "-c", "sleep 5", "/crash/missing"},
					},
				},
			},
		}

		delay1, delay2 := startPodAndGetBackOffs(podClient, pod, buildBackOffDuration)

		ginkgo.By("updating the image")
		podClient.Update(podName, func(pod *v1.Pod) {
			pod.Spec.Containers[0].Image = imageutils.GetE2EImage(imageutils.Nginx)
		})

		time.Sleep(syncLoopFrequency)
		framework.ExpectNoError(f.WaitForPodRunning(pod.Name))

		ginkgo.By("get restart delay after image update")
		delayAfterUpdate, err := getRestartDelay(podClient, podName, containerName)
		if err != nil {
			framework.Failf("timed out waiting for container restart in pod=%s/%s", podName, containerName)
		}

		if delayAfterUpdate > 2*delay2 || delayAfterUpdate > 2*delay1 {
			framework.Failf("updating image did not reset the back-off value in pod=%s/%s d3=%s d2=%s d1=%s", podName, containerName, delayAfterUpdate, delay1, delay2)
		}
	})

	// Slow by design (~27 mins) issue #19027
	ginkgo.It("should cap back-off at MaxContainerBackOff [Slow][NodeConformance]", func() {
		podName := "back-off-cap"
		containerName := "back-off-cap"
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:   podName,
				Labels: map[string]string{"test": "liveness"},
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name:    containerName,
						Image:   imageutils.GetE2EImage(imageutils.BusyBox),
						Command: []string{"/bin/sh", "-c", "sleep 5", "/crash/missing"},
					},
				},
			},
		}

		podClient.CreateSync(pod)
		time.Sleep(2 * kubelet.MaxContainerBackOff) // it takes slightly more than 2*x to get to a back-off of x

		// wait for a delay == capped delay of MaxContainerBackOff
		ginkgo.By("getting restart delay when capped")
		var (
			delay1 time.Duration
			err    error
		)
		for i := 0; i < 3; i++ {
			delay1, err = getRestartDelay(podClient, podName, containerName)
			if err != nil {
				framework.Failf("timed out waiting for container restart in pod=%s/%s", podName, containerName)
			}

			if delay1 < kubelet.MaxContainerBackOff {
				continue
			}
		}

		if (delay1 < kubelet.MaxContainerBackOff) || (delay1 > maxBackOffTolerance) {
			framework.Failf("expected %s back-off got=%s in delay1", kubelet.MaxContainerBackOff, delay1)
		}

		ginkgo.By("getting restart delay after a capped delay")
		delay2, err := getRestartDelay(podClient, podName, containerName)
		if err != nil {
			framework.Failf("timed out waiting for container restart in pod=%s/%s", podName, containerName)
		}

		if delay2 < kubelet.MaxContainerBackOff || delay2 > maxBackOffTolerance { // syncloop cumulative drift
			framework.Failf("expected %s back-off got=%s on delay2", kubelet.MaxContainerBackOff, delay2)
		}
	})

	// TODO(freehan): label the test to be [NodeConformance] after tests are proven to be stable.
	ginkgo.It("should support pod readiness gates [NodeFeature:PodReadinessGate]", func() {
		podName := "pod-ready"
		readinessGate1 := "k8s.io/test-condition1"
		readinessGate2 := "k8s.io/test-condition2"
		patchStatusFmt := `{"status":{"conditions":[{"type":%q, "status":%q}]}}`
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:   podName,
				Labels: map[string]string{"test": "pod-readiness-gate"},
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name:    "pod-readiness-gate",
						Image:   imageutils.GetE2EImage(imageutils.BusyBox),
						Command: []string{"/bin/sh", "-c", "echo container is alive; sleep 10000"},
					},
				},
				ReadinessGates: []v1.PodReadinessGate{
					{ConditionType: v1.PodConditionType(readinessGate1)},
					{ConditionType: v1.PodConditionType(readinessGate2)},
				},
			},
		}

		validatePodReadiness := func(expectReady bool) {
			err := wait.Poll(time.Second, wait.ForeverTestTimeout, func() (bool, error) {
				podReady := podClient.PodIsReady(podName)
				res := expectReady == podReady
				if !res {
					framework.Logf("Expect the Ready condition of pod %q to be %v, but got %v", podName, expectReady, podReady)
				}
				return res, nil
			})
			framework.ExpectNoError(err)
		}

		ginkgo.By("submitting the pod to kubernetes")
		podClient.CreateSync(pod)
		gomega.Expect(podClient.PodIsReady(podName)).To(gomega.BeFalse(), "Expect pod's Ready condition to be false initially.")

		ginkgo.By(fmt.Sprintf("patching pod status with condition %q to true", readinessGate1))
		_, err := podClient.Patch(podName, types.StrategicMergePatchType, []byte(fmt.Sprintf(patchStatusFmt, readinessGate1, "True")), "status")
		framework.ExpectNoError(err)
		// Sleep for 10 seconds.
		time.Sleep(syncLoopFrequency)
		// Verify the pod is still not ready
		gomega.Expect(podClient.PodIsReady(podName)).To(gomega.BeFalse(), "Expect pod's Ready condition to be false with only one condition in readinessGates equal to True")

		ginkgo.By(fmt.Sprintf("patching pod status with condition %q to true", readinessGate2))
		_, err = podClient.Patch(podName, types.StrategicMergePatchType, []byte(fmt.Sprintf(patchStatusFmt, readinessGate2, "True")), "status")
		framework.ExpectNoError(err)
		validatePodReadiness(true)

		ginkgo.By(fmt.Sprintf("patching pod status with condition %q to false", readinessGate1))
		_, err = podClient.Patch(podName, types.StrategicMergePatchType, []byte(fmt.Sprintf(patchStatusFmt, readinessGate1, "False")), "status")
		framework.ExpectNoError(err)
		validatePodReadiness(false)

	})
})
