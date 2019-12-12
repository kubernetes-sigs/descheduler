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

package scheduler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apiserver/pkg/admission"
	cacheddiscovery "k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/scale"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/events"
	"k8s.io/kubernetes/pkg/api/legacyscheme"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	"k8s.io/kubernetes/pkg/controller/disruption"
	"k8s.io/kubernetes/pkg/scheduler"
	schedulerapi "k8s.io/kubernetes/pkg/scheduler/apis/config"
	"k8s.io/kubernetes/pkg/scheduler/apis/config/scheme"
	schedulerapiv1 "k8s.io/kubernetes/pkg/scheduler/apis/config/v1"

	// Register defaults in pkg/scheduler/algorithmprovider.
	_ "k8s.io/kubernetes/pkg/scheduler/algorithmprovider"
	taintutils "k8s.io/kubernetes/pkg/util/taints"
	"k8s.io/kubernetes/test/integration/framework"
	imageutils "k8s.io/kubernetes/test/utils/image"
)

type testContext struct {
	closeFn         framework.CloseFunc
	httpServer      *httptest.Server
	ns              *v1.Namespace
	clientSet       *clientset.Clientset
	informerFactory informers.SharedInformerFactory
	scheduler       *scheduler.Scheduler
	ctx             context.Context
	cancelFn        context.CancelFunc
}

func createAlgorithmSourceFromPolicy(policy *schedulerapi.Policy, clientSet clientset.Interface) schedulerapi.SchedulerAlgorithmSource {
	// Serialize the Policy object into a ConfigMap later.
	info, ok := runtime.SerializerInfoForMediaType(scheme.Codecs.SupportedMediaTypes(), runtime.ContentTypeJSON)
	if !ok {
		panic("could not find json serializer")
	}
	encoder := scheme.Codecs.EncoderForVersion(info.Serializer, schedulerapiv1.SchemeGroupVersion)
	policyString := runtime.EncodeOrDie(encoder, policy)
	configPolicyName := "scheduler-custom-policy-config"
	policyConfigMap := v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceSystem, Name: configPolicyName},
		Data:       map[string]string{schedulerapi.SchedulerPolicyConfigMapKey: policyString},
	}
	policyConfigMap.APIVersion = "v1"
	clientSet.CoreV1().ConfigMaps(metav1.NamespaceSystem).Create(&policyConfigMap)

	return schedulerapi.SchedulerAlgorithmSource{
		Policy: &schedulerapi.SchedulerPolicySource{
			ConfigMap: &schedulerapi.SchedulerPolicyConfigMapSource{
				Namespace: policyConfigMap.Namespace,
				Name:      policyConfigMap.Name,
			},
		},
	}
}

// initTestMasterAndScheduler initializes a test environment and creates a master with default
// configuration.
func initTestMaster(t *testing.T, nsPrefix string, admission admission.Interface) *testContext {
	ctx, cancelFunc := context.WithCancel(context.Background())
	context := testContext{
		ctx:      ctx,
		cancelFn: cancelFunc,
	}

	// 1. Create master
	h := &framework.MasterHolder{Initialized: make(chan struct{})}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		<-h.Initialized
		h.M.GenericAPIServer.Handler.ServeHTTP(w, req)
	}))

	masterConfig := framework.NewIntegrationTestMasterConfig()

	if admission != nil {
		masterConfig.GenericConfig.AdmissionControl = admission
	}

	_, context.httpServer, context.closeFn = framework.RunAMasterUsingServer(masterConfig, s, h)

	if nsPrefix != "default" {
		context.ns = framework.CreateTestingNamespace(nsPrefix+string(uuid.NewUUID()), s, t)
	} else {
		context.ns = framework.CreateTestingNamespace("default", s, t)
	}

	// 2. Create kubeclient
	context.clientSet = clientset.NewForConfigOrDie(
		&restclient.Config{
			QPS: -1, Host: s.URL,
			ContentConfig: restclient.ContentConfig{
				GroupVersion: &schema.GroupVersion{Group: "", Version: "v1"},
			},
		},
	)
	return &context
}

// initTestScheduler initializes a test environment and creates a scheduler with default
// configuration.
func initTestScheduler(
	t *testing.T,
	context *testContext,
	setPodInformer bool,
	policy *schedulerapi.Policy,
) *testContext {
	// Pod preemption is enabled by default scheduler configuration.
	return initTestSchedulerWithOptions(t, context, setPodInformer, policy, time.Second)
}

// initTestSchedulerWithOptions initializes a test environment and creates a scheduler with default
// configuration and other options.
func initTestSchedulerWithOptions(
	t *testing.T,
	context *testContext,
	setPodInformer bool,
	policy *schedulerapi.Policy,
	resyncPeriod time.Duration,
	opts ...scheduler.Option,
) *testContext {
	// 1. Create scheduler
	context.informerFactory = informers.NewSharedInformerFactory(context.clientSet, resyncPeriod)

	var podInformer coreinformers.PodInformer

	// create independent pod informer if required
	if setPodInformer {
		podInformer = scheduler.NewPodInformer(context.clientSet, 12*time.Hour)
	} else {
		podInformer = context.informerFactory.Core().V1().Pods()
	}
	var err error
	eventBroadcaster := events.NewBroadcaster(&events.EventSinkImpl{
		Interface: context.clientSet.EventsV1beta1().Events(""),
	})
	recorder := eventBroadcaster.NewRecorder(
		legacyscheme.Scheme,
		v1.DefaultSchedulerName,
	)
	if policy != nil {
		opts = append(opts, scheduler.WithAlgorithmSource(createAlgorithmSourceFromPolicy(policy, context.clientSet)))
	}
	opts = append([]scheduler.Option{scheduler.WithBindTimeoutSeconds(600)}, opts...)
	context.scheduler, err = scheduler.New(
		context.clientSet,
		context.informerFactory,
		podInformer,
		recorder,
		context.ctx.Done(),
		opts...,
	)

	if err != nil {
		t.Fatalf("Couldn't create scheduler: %v", err)
	}

	// set setPodInformer if provided.
	if setPodInformer {
		go podInformer.Informer().Run(context.scheduler.StopEverything)
		cache.WaitForNamedCacheSync("scheduler", context.scheduler.StopEverything, podInformer.Informer().HasSynced)
	}

	stopCh := make(chan struct{})
	eventBroadcaster.StartRecordingToSink(stopCh)

	context.informerFactory.Start(context.scheduler.StopEverything)
	context.informerFactory.WaitForCacheSync(context.scheduler.StopEverything)

	go context.scheduler.Run(context.ctx)

	return context
}

// initDisruptionController initializes and runs a Disruption Controller to properly
// update PodDisuptionBudget objects.
func initDisruptionController(t *testing.T, context *testContext) *disruption.DisruptionController {
	informers := informers.NewSharedInformerFactory(context.clientSet, 12*time.Hour)

	discoveryClient := cacheddiscovery.NewMemCacheClient(context.clientSet.Discovery())
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(discoveryClient)

	config := restclient.Config{Host: context.httpServer.URL}
	scaleKindResolver := scale.NewDiscoveryScaleKindResolver(context.clientSet.Discovery())
	scaleClient, err := scale.NewForConfig(&config, mapper, dynamic.LegacyAPIPathResolverFunc, scaleKindResolver)
	if err != nil {
		t.Fatalf("Error in create scaleClient: %v", err)
	}

	dc := disruption.NewDisruptionController(
		informers.Core().V1().Pods(),
		informers.Policy().V1beta1().PodDisruptionBudgets(),
		informers.Core().V1().ReplicationControllers(),
		informers.Apps().V1().ReplicaSets(),
		informers.Apps().V1().Deployments(),
		informers.Apps().V1().StatefulSets(),
		context.clientSet,
		mapper,
		scaleClient)

	informers.Start(context.scheduler.StopEverything)
	informers.WaitForCacheSync(context.scheduler.StopEverything)
	go dc.Run(context.scheduler.StopEverything)
	return dc
}

// initTest initializes a test environment and creates master and scheduler with default
// configuration.
func initTest(t *testing.T, nsPrefix string) *testContext {
	return initTestScheduler(t, initTestMaster(t, nsPrefix, nil), true, nil)
}

// initTestDisablePreemption initializes a test environment and creates master and scheduler with default
// configuration but with pod preemption disabled.
func initTestDisablePreemption(t *testing.T, nsPrefix string) *testContext {
	return initTestSchedulerWithOptions(
		t, initTestMaster(t, nsPrefix, nil), true, nil,
		time.Second, scheduler.WithPreemptionDisabled(true))
}

// cleanupTest deletes the scheduler and the test namespace. It should be called
// at the end of a test.
func cleanupTest(t *testing.T, context *testContext) {
	// Kill the scheduler.
	context.cancelFn()
	// Cleanup nodes.
	context.clientSet.CoreV1().Nodes().DeleteCollection(nil, metav1.ListOptions{})
	framework.DeleteTestingNamespace(context.ns, context.httpServer, t)
	context.closeFn()
}

// waitForReflection waits till the passFunc confirms that the object it expects
// to see is in the store. Used to observe reflected events.
func waitForReflection(t *testing.T, nodeLister corelisters.NodeLister, key string,
	passFunc func(n interface{}) bool) error {
	nodes := []*v1.Node{}
	err := wait.Poll(time.Millisecond*100, wait.ForeverTestTimeout, func() (bool, error) {
		n, err := nodeLister.Get(key)

		switch {
		case err == nil && passFunc(n):
			return true, nil
		case errors.IsNotFound(err):
			nodes = append(nodes, nil)
		case err != nil:
			t.Errorf("Unexpected error: %v", err)
		default:
			nodes = append(nodes, n)
		}

		return false, nil
	})
	if err != nil {
		t.Logf("Logging consecutive node versions received from store:")
		for i, n := range nodes {
			t.Logf("%d: %#v", i, n)
		}
	}
	return err
}

// nodeHasLabels returns a function that checks if a node has all the given labels.
func nodeHasLabels(cs clientset.Interface, nodeName string, labels map[string]string) wait.ConditionFunc {
	return func() (bool, error) {
		node, err := cs.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
		if err != nil {
			// This could be a connection error so we want to retry.
			return false, nil
		}
		for k, v := range labels {
			if node.Labels == nil || node.Labels[k] != v {
				return false, nil
			}
		}
		return true, nil
	}
}

// waitForNodeLabels waits for the given node to have all the given labels.
func waitForNodeLabels(cs clientset.Interface, nodeName string, labels map[string]string) error {
	return wait.Poll(time.Millisecond*100, wait.ForeverTestTimeout, nodeHasLabels(cs, nodeName, labels))
}

// initNode returns a node with the given resource list and images. If 'res' is nil, a predefined amount of
// resource will be used.
func initNode(name string, res *v1.ResourceList, images []v1.ContainerImage) *v1.Node {
	// if resource is nil, we use a default amount of resources for the node.
	if res == nil {
		res = &v1.ResourceList{
			v1.ResourcePods: *resource.NewQuantity(32, resource.DecimalSI),
		}
	}

	n := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       v1.NodeSpec{Unschedulable: false},
		Status: v1.NodeStatus{
			Capacity: *res,
			Images:   images,
		},
	}
	return n
}

// createNode creates a node with the given resource list.
func createNode(cs clientset.Interface, name string, res *v1.ResourceList) (*v1.Node, error) {
	return cs.CoreV1().Nodes().Create(initNode(name, res, nil))
}

// createNodeWithImages creates a node with the given resource list and images.
func createNodeWithImages(cs clientset.Interface, name string, res *v1.ResourceList, images []v1.ContainerImage) (*v1.Node, error) {
	return cs.CoreV1().Nodes().Create(initNode(name, res, images))
}

// updateNodeStatus updates the status of node.
func updateNodeStatus(cs clientset.Interface, node *v1.Node) error {
	_, err := cs.CoreV1().Nodes().UpdateStatus(node)
	return err
}

// createNodes creates `numNodes` nodes. The created node names will be in the
// form of "`prefix`-X" where X is an ordinal.
func createNodes(cs clientset.Interface, prefix string, res *v1.ResourceList, numNodes int) ([]*v1.Node, error) {
	nodes := make([]*v1.Node, numNodes)
	for i := 0; i < numNodes; i++ {
		nodeName := fmt.Sprintf("%v-%d", prefix, i)
		node, err := createNode(cs, nodeName, res)
		if err != nil {
			return nodes[:], err
		}
		nodes[i] = node
	}
	return nodes[:], nil
}

// nodeTainted return a condition function that returns true if the given node contains
// the taints.
func nodeTainted(cs clientset.Interface, nodeName string, taints []v1.Taint) wait.ConditionFunc {
	return func() (bool, error) {
		node, err := cs.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		// node.Spec.Taints may have more taints
		if len(taints) > len(node.Spec.Taints) {
			return false, nil
		}

		for _, taint := range taints {
			if !taintutils.TaintExists(node.Spec.Taints, &taint) {
				return false, nil
			}
		}

		return true, nil
	}
}

func addTaintToNode(cs clientset.Interface, nodeName string, taint v1.Taint) error {
	node, err := cs.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	copy := node.DeepCopy()
	copy.Spec.Taints = append(copy.Spec.Taints, taint)
	_, err = cs.CoreV1().Nodes().Update(copy)
	return err
}

// waitForNodeTaints waits for a node to have the target taints and returns
// an error if it does not have taints within the given timeout.
func waitForNodeTaints(cs clientset.Interface, node *v1.Node, taints []v1.Taint) error {
	return wait.Poll(100*time.Millisecond, 30*time.Second, nodeTainted(cs, node.Name, taints))
}

// cleanupNodes deletes all nodes.
func cleanupNodes(cs clientset.Interface, t *testing.T) {
	err := cs.CoreV1().Nodes().DeleteCollection(
		metav1.NewDeleteOptions(0), metav1.ListOptions{})
	if err != nil {
		t.Errorf("error while deleting all nodes: %v", err)
	}
}

type pausePodConfig struct {
	Name                              string
	Namespace                         string
	Affinity                          *v1.Affinity
	Annotations, Labels, NodeSelector map[string]string
	Resources                         *v1.ResourceRequirements
	Tolerations                       []v1.Toleration
	NodeName                          string
	SchedulerName                     string
	Priority                          *int32
	PriorityClassName                 string
}

// initPausePod initializes a pod API object from the given config. It is used
// mainly in pod creation process.
func initPausePod(cs clientset.Interface, conf *pausePodConfig) *v1.Pod {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        conf.Name,
			Namespace:   conf.Namespace,
			Labels:      conf.Labels,
			Annotations: conf.Annotations,
		},
		Spec: v1.PodSpec{
			NodeSelector: conf.NodeSelector,
			Affinity:     conf.Affinity,
			Containers: []v1.Container{
				{
					Name:  conf.Name,
					Image: imageutils.GetPauseImageName(),
				},
			},
			Tolerations:       conf.Tolerations,
			NodeName:          conf.NodeName,
			SchedulerName:     conf.SchedulerName,
			Priority:          conf.Priority,
			PriorityClassName: conf.PriorityClassName,
		},
	}
	if conf.Resources != nil {
		pod.Spec.Containers[0].Resources = *conf.Resources
	}
	return pod
}

// createPausePod creates a pod with "Pause" image and the given config and
// return its pointer and error status.
func createPausePod(cs clientset.Interface, p *v1.Pod) (*v1.Pod, error) {
	return cs.CoreV1().Pods(p.Namespace).Create(p)
}

// createPausePodWithResource creates a pod with "Pause" image and the given
// resources and returns its pointer and error status. The resource list can be
// nil.
func createPausePodWithResource(cs clientset.Interface, podName string,
	nsName string, res *v1.ResourceList) (*v1.Pod, error) {
	var conf pausePodConfig
	if res == nil {
		conf = pausePodConfig{
			Name:      podName,
			Namespace: nsName,
		}
	} else {
		conf = pausePodConfig{
			Name:      podName,
			Namespace: nsName,
			Resources: &v1.ResourceRequirements{
				Requests: *res,
			},
		}
	}
	return createPausePod(cs, initPausePod(cs, &conf))
}

// runPausePod creates a pod with "Pause" image and the given config and waits
// until it is scheduled. It returns its pointer and error status.
func runPausePod(cs clientset.Interface, pod *v1.Pod) (*v1.Pod, error) {
	pod, err := cs.CoreV1().Pods(pod.Namespace).Create(pod)
	if err != nil {
		return nil, fmt.Errorf("Error creating pause pod: %v", err)
	}
	if err = waitForPodToSchedule(cs, pod); err != nil {
		return pod, fmt.Errorf("Pod %v/%v didn't schedule successfully. Error: %v", pod.Namespace, pod.Name, err)
	}
	if pod, err = cs.CoreV1().Pods(pod.Namespace).Get(pod.Name, metav1.GetOptions{}); err != nil {
		return pod, fmt.Errorf("Error getting pod %v/%v info: %v", pod.Namespace, pod.Name, err)
	}
	return pod, nil
}

type podWithContainersConfig struct {
	Name       string
	Namespace  string
	Containers []v1.Container
}

// initPodWithContainers initializes a pod API object from the given config. This is used primarily for generating
// pods with containers each having a specific image.
func initPodWithContainers(cs clientset.Interface, conf *podWithContainersConfig) *v1.Pod {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      conf.Name,
			Namespace: conf.Namespace,
		},
		Spec: v1.PodSpec{
			Containers: conf.Containers,
		},
	}
	return pod
}

// runPodWithContainers creates a pod with given config and containers and waits
// until it is scheduled. It returns its pointer and error status.
func runPodWithContainers(cs clientset.Interface, pod *v1.Pod) (*v1.Pod, error) {
	pod, err := cs.CoreV1().Pods(pod.Namespace).Create(pod)
	if err != nil {
		return nil, fmt.Errorf("Error creating pod-with-containers: %v", err)
	}
	if err = waitForPodToSchedule(cs, pod); err != nil {
		return pod, fmt.Errorf("Pod %v didn't schedule successfully. Error: %v", pod.Name, err)
	}
	if pod, err = cs.CoreV1().Pods(pod.Namespace).Get(pod.Name, metav1.GetOptions{}); err != nil {
		return pod, fmt.Errorf("Error getting pod %v info: %v", pod.Name, err)
	}
	return pod, nil
}

// podDeleted returns true if a pod is not found in the given namespace.
func podDeleted(c clientset.Interface, podNamespace, podName string) wait.ConditionFunc {
	return func() (bool, error) {
		pod, err := c.CoreV1().Pods(podNamespace).Get(podName, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			return true, nil
		}
		if pod.DeletionTimestamp != nil {
			return true, nil
		}
		return false, nil
	}
}

// podIsGettingEvicted returns true if the pod's deletion timestamp is set.
func podIsGettingEvicted(c clientset.Interface, podNamespace, podName string) wait.ConditionFunc {
	return func() (bool, error) {
		pod, err := c.CoreV1().Pods(podNamespace).Get(podName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if pod.DeletionTimestamp != nil {
			return true, nil
		}
		return false, nil
	}
}

// podScheduled returns true if a node is assigned to the given pod.
func podScheduled(c clientset.Interface, podNamespace, podName string) wait.ConditionFunc {
	return func() (bool, error) {
		pod, err := c.CoreV1().Pods(podNamespace).Get(podName, metav1.GetOptions{})
		if err != nil {
			// This could be a connection error so we want to retry.
			return false, nil
		}
		if pod.Spec.NodeName == "" {
			return false, nil
		}
		return true, nil
	}
}

// podScheduledIn returns true if a given pod is placed onto one of the expected nodes.
func podScheduledIn(c clientset.Interface, podNamespace, podName string, nodeNames []string) wait.ConditionFunc {
	return func() (bool, error) {
		pod, err := c.CoreV1().Pods(podNamespace).Get(podName, metav1.GetOptions{})
		if err != nil {
			// This could be a connection error so we want to retry.
			return false, nil
		}
		if pod.Spec.NodeName == "" {
			return false, nil
		}
		for _, nodeName := range nodeNames {
			if pod.Spec.NodeName == nodeName {
				return true, nil
			}
		}
		return false, nil
	}
}

// podUnschedulable returns a condition function that returns true if the given pod
// gets unschedulable status.
func podUnschedulable(c clientset.Interface, podNamespace, podName string) wait.ConditionFunc {
	return func() (bool, error) {
		pod, err := c.CoreV1().Pods(podNamespace).Get(podName, metav1.GetOptions{})
		if err != nil {
			// This could be a connection error so we want to retry.
			return false, nil
		}
		_, cond := podutil.GetPodCondition(&pod.Status, v1.PodScheduled)
		return cond != nil && cond.Status == v1.ConditionFalse &&
			cond.Reason == v1.PodReasonUnschedulable, nil
	}
}

// podSchedulingError returns a condition function that returns true if the given pod
// gets unschedulable status for reasons other than "Unschedulable". The scheduler
// records such reasons in case of error.
func podSchedulingError(c clientset.Interface, podNamespace, podName string) wait.ConditionFunc {
	return func() (bool, error) {
		pod, err := c.CoreV1().Pods(podNamespace).Get(podName, metav1.GetOptions{})
		if err != nil {
			// This could be a connection error so we want to retry.
			return false, nil
		}
		_, cond := podutil.GetPodCondition(&pod.Status, v1.PodScheduled)
		return cond != nil && cond.Status == v1.ConditionFalse &&
			cond.Reason != v1.PodReasonUnschedulable, nil
	}
}

// waitForPodToScheduleWithTimeout waits for a pod to get scheduled and returns
// an error if it does not scheduled within the given timeout.
func waitForPodToScheduleWithTimeout(cs clientset.Interface, pod *v1.Pod, timeout time.Duration) error {
	return wait.Poll(100*time.Millisecond, timeout, podScheduled(cs, pod.Namespace, pod.Name))
}

// waitForPodToSchedule waits for a pod to get scheduled and returns an error if
// it does not get scheduled within the timeout duration (30 seconds).
func waitForPodToSchedule(cs clientset.Interface, pod *v1.Pod) error {
	return waitForPodToScheduleWithTimeout(cs, pod, 30*time.Second)
}

// waitForPodUnscheduleWithTimeout waits for a pod to fail scheduling and returns
// an error if it does not become unschedulable within the given timeout.
func waitForPodUnschedulableWithTimeout(cs clientset.Interface, pod *v1.Pod, timeout time.Duration) error {
	return wait.Poll(100*time.Millisecond, timeout, podUnschedulable(cs, pod.Namespace, pod.Name))
}

// waitForPodUnschedule waits for a pod to fail scheduling and returns
// an error if it does not become unschedulable within the timeout duration (30 seconds).
func waitForPodUnschedulable(cs clientset.Interface, pod *v1.Pod) error {
	return waitForPodUnschedulableWithTimeout(cs, pod, 30*time.Second)
}

// waitForPDBsStable waits for PDBs to have "CurrentHealthy" status equal to
// the expected values.
func waitForPDBsStable(context *testContext, pdbs []*policy.PodDisruptionBudget, pdbPodNum []int32) error {
	return wait.Poll(time.Second, 60*time.Second, func() (bool, error) {
		pdbList, err := context.clientSet.PolicyV1beta1().PodDisruptionBudgets(context.ns.Name).List(metav1.ListOptions{})
		if err != nil {
			return false, err
		}
		if len(pdbList.Items) != len(pdbs) {
			return false, nil
		}
		for i, pdb := range pdbs {
			found := false
			for _, cpdb := range pdbList.Items {
				if pdb.Name == cpdb.Name && pdb.Namespace == cpdb.Namespace {
					found = true
					if cpdb.Status.CurrentHealthy != pdbPodNum[i] {
						return false, nil
					}
				}
			}
			if !found {
				return false, nil
			}
		}
		return true, nil
	})
}

// waitCachedPodsStable waits until scheduler cache has the given pods.
func waitCachedPodsStable(context *testContext, pods []*v1.Pod) error {
	return wait.Poll(time.Second, 30*time.Second, func() (bool, error) {
		cachedPods, err := context.scheduler.SchedulerCache.List(labels.Everything())
		if err != nil {
			return false, err
		}
		if len(pods) != len(cachedPods) {
			return false, nil
		}
		for _, p := range pods {
			actualPod, err1 := context.clientSet.CoreV1().Pods(p.Namespace).Get(p.Name, metav1.GetOptions{})
			if err1 != nil {
				return false, err1
			}
			cachedPod, err2 := context.scheduler.SchedulerCache.GetPod(actualPod)
			if err2 != nil || cachedPod == nil {
				return false, err2
			}
		}
		return true, nil
	})
}

// deletePod deletes the given pod in the given namespace.
func deletePod(cs clientset.Interface, podName string, nsName string) error {
	return cs.CoreV1().Pods(nsName).Delete(podName, metav1.NewDeleteOptions(0))
}

func getPod(cs clientset.Interface, podName string, podNamespace string) (*v1.Pod, error) {
	return cs.CoreV1().Pods(podNamespace).Get(podName, metav1.GetOptions{})
}

// cleanupPods deletes the given pods and waits for them to be actually deleted.
func cleanupPods(cs clientset.Interface, t *testing.T, pods []*v1.Pod) {
	for _, p := range pods {
		err := cs.CoreV1().Pods(p.Namespace).Delete(p.Name, metav1.NewDeleteOptions(0))
		if err != nil && !errors.IsNotFound(err) {
			t.Errorf("error while deleting pod %v/%v: %v", p.Namespace, p.Name, err)
		}
	}
	for _, p := range pods {
		if err := wait.Poll(time.Millisecond, wait.ForeverTestTimeout,
			podDeleted(cs, p.Namespace, p.Name)); err != nil {
			t.Errorf("error while waiting for pod  %v/%v to get deleted: %v", p.Namespace, p.Name, err)
		}
	}
}

// noPodsInNamespace returns true if no pods in the given namespace.
func noPodsInNamespace(c clientset.Interface, podNamespace string) wait.ConditionFunc {
	return func() (bool, error) {
		pods, err := c.CoreV1().Pods(podNamespace).List(metav1.ListOptions{})
		if err != nil {
			return false, err
		}

		return len(pods.Items) == 0, nil
	}
}

// cleanupPodsInNamespace deletes the pods in the given namespace and waits for them to
// be actually deleted.
func cleanupPodsInNamespace(cs clientset.Interface, t *testing.T, ns string) {
	if err := cs.CoreV1().Pods(ns).DeleteCollection(nil, metav1.ListOptions{}); err != nil {
		t.Errorf("error while listing pod in namespace %v: %v", ns, err)
		return
	}

	if err := wait.Poll(time.Second, wait.ForeverTestTimeout,
		noPodsInNamespace(cs, ns)); err != nil {
		t.Errorf("error while waiting for pods in namespace %v: %v", ns, err)
	}
}

func waitForSchedulerCacheCleanup(sched *scheduler.Scheduler, t *testing.T) {
	schedulerCacheIsEmpty := func() (bool, error) {
		snapshot := sched.Cache().Snapshot()

		return len(snapshot.Nodes) == 0 && len(snapshot.AssumedPods) == 0, nil
	}

	if err := wait.Poll(time.Second, wait.ForeverTestTimeout, schedulerCacheIsEmpty); err != nil {
		t.Errorf("Failed to wait for scheduler cache cleanup: %v", err)
	}
}
