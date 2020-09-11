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

package e2e

import (
	"context"
	"math"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/cmd/descheduler/app/options"
	"sigs.k8s.io/descheduler/pkg/api"
	deschedulerapi "sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler"
	"sigs.k8s.io/descheduler/pkg/descheduler/client"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	eutils "sigs.k8s.io/descheduler/pkg/descheduler/evictions/utils"
	nodeutil "sigs.k8s.io/descheduler/pkg/descheduler/node"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/descheduler/strategies"
)

func MakePodSpec(priorityClassName string) v1.PodSpec {
	return v1.PodSpec{
		Containers: []v1.Container{{
			Name:            "pause",
			ImagePullPolicy: "Never",
			Image:           "kubernetes/pause",
			Ports:           []v1.ContainerPort{{ContainerPort: 80}},
			Resources: v1.ResourceRequirements{
				Limits: v1.ResourceList{
					v1.ResourceCPU:    resource.MustParse("100m"),
					v1.ResourceMemory: resource.MustParse("1000Mi"),
				},
				Requests: v1.ResourceList{
					v1.ResourceCPU:    resource.MustParse("100m"),
					v1.ResourceMemory: resource.MustParse("800Mi"),
				},
			},
		}},
		PriorityClassName: priorityClassName,
	}
}

// RcByNameContainer returns a ReplicationControoler with specified name and container
func RcByNameContainer(name, namespace string, replicas int32, labels map[string]string, gracePeriod *int64, priorityClassName string) *v1.ReplicationController {
	zeroGracePeriod := int64(0)

	// Add "name": name to the labels, overwriting if it exists.
	labels["name"] = name
	if gracePeriod == nil {
		gracePeriod = &zeroGracePeriod
	}
	return &v1.ReplicationController{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ReplicationController",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1.ReplicationControllerSpec{
			Replicas: func(i int32) *int32 { return &i }(replicas),
			Selector: map[string]string{
				"name": name,
			},
			Template: &v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: MakePodSpec(priorityClassName),
			},
		},
	}
}

// startEndToEndForLowNodeUtilization tests the lownode utilization strategy.
func startEndToEndForLowNodeUtilization(ctx context.Context, clientset clientset.Interface, nodeInformer coreinformers.NodeInformer, podEvictor *evictions.PodEvictor) {
	// Run descheduler.
	nodes, err := nodeutil.ReadyNodes(ctx, clientset, nodeInformer, "", nil)
	if err != nil {
		klog.Fatalf("%v", err)
	}

	strategies.LowNodeUtilization(
		ctx,
		clientset,
		deschedulerapi.DeschedulerStrategy{
			Enabled: true,
			Params: &deschedulerapi.StrategyParameters{
				NodeResourceUtilizationThresholds: &deschedulerapi.NodeResourceUtilizationThresholds{
					Thresholds: deschedulerapi.ResourceThresholds{
						v1.ResourceMemory: 20,
						v1.ResourcePods:   20,
						v1.ResourceCPU:    85,
					},
					TargetThresholds: deschedulerapi.ResourceThresholds{
						v1.ResourceMemory: 20,
						v1.ResourcePods:   20,
						v1.ResourceCPU:    90,
					},
				},
			},
		},
		nodes,
		podEvictor,
	)

	time.Sleep(10 * time.Second)
}

func initializeClient(t *testing.T) (clientset.Interface, coreinformers.NodeInformer, chan struct{}) {
	clientSet, err := client.CreateClient(os.Getenv("KUBECONFIG"))
	if err != nil {
		t.Errorf("Error during client creation with %v", err)
	}

	stopChannel := make(chan struct{})

	sharedInformerFactory := informers.NewSharedInformerFactory(clientSet, 0)
	sharedInformerFactory.Start(stopChannel)
	sharedInformerFactory.WaitForCacheSync(stopChannel)

	nodeInformer := sharedInformerFactory.Core().V1().Nodes()

	return clientSet, nodeInformer, stopChannel
}

func TestLowNodeUtilization(t *testing.T) {
	ctx := context.Background()

	clientSet, nodeInformer, stopCh := initializeClient(t)
	defer close(stopCh)

	nodeList, err := clientSet.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Errorf("Error listing node with %v", err)
	}

	var nodes []*v1.Node
	for i := range nodeList.Items {
		node := nodeList.Items[i]
		nodes = append(nodes, &node)
	}

	testNamespace := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "e2e-" + strings.ToLower(t.Name())}}
	if _, err := clientSet.CoreV1().Namespaces().Create(ctx, testNamespace, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Unable to create ns %v", testNamespace.Name)
	}
	defer clientSet.CoreV1().Namespaces().Delete(ctx, testNamespace.Name, metav1.DeleteOptions{})

	rc := RcByNameContainer("test-rc-node-utilization", testNamespace.Name, int32(15), map[string]string{"test": "node-utilization"}, nil, "")
	if _, err := clientSet.CoreV1().ReplicationControllers(rc.Namespace).Create(ctx, rc, metav1.CreateOptions{}); err != nil {
		t.Errorf("Error creating deployment %v", err)
	}

	evictPods(ctx, t, clientSet, nodeInformer, nodes, rc)
	deleteRC(ctx, t, clientSet, rc)
}

func runPodLifetimeStrategy(ctx context.Context, clientset clientset.Interface, nodeInformer coreinformers.NodeInformer, namespaces *deschedulerapi.Namespaces, priorityClass string, priority *int32) {
	// Run descheduler.
	evictionPolicyGroupVersion, err := eutils.SupportEviction(clientset)
	if err != nil || len(evictionPolicyGroupVersion) == 0 {
		klog.Fatalf("%v", err)
	}

	nodes, err := nodeutil.ReadyNodes(ctx, clientset, nodeInformer, "", nil)
	if err != nil {
		klog.Fatalf("%v", err)
	}

	maxPodLifeTimeSeconds := uint(1)
	strategies.PodLifeTime(
		ctx,
		clientset,
		deschedulerapi.DeschedulerStrategy{
			Enabled: true,
			Params: &deschedulerapi.StrategyParameters{
				PodLifeTime:                &deschedulerapi.PodLifeTime{MaxPodLifeTimeSeconds: &maxPodLifeTimeSeconds},
				Namespaces:                 namespaces,
				ThresholdPriority:          priority,
				ThresholdPriorityClassName: priorityClass,
			},
		},
		nodes,
		evictions.NewPodEvictor(
			clientset,
			evictionPolicyGroupVersion,
			false,
			0,
			nodes,
			false,
		),
	)
}

func getPodNames(pods []v1.Pod) []string {
	names := []string{}
	for _, pod := range pods {
		names = append(names, pod.Name)
	}
	return names
}

func intersectStrings(lista, listb []string) []string {
	commonNames := []string{}

	for _, stra := range lista {
		for _, strb := range listb {
			if stra == strb {
				commonNames = append(commonNames, stra)
				break
			}
		}
	}

	return commonNames
}

// TODO(jchaloup): add testcases for two included/excluded namespaces

func TestNamespaceConstraintsInclude(t *testing.T) {
	ctx := context.Background()

	clientSet, nodeInformer, stopCh := initializeClient(t)
	defer close(stopCh)

	testNamespace := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "e2e-" + strings.ToLower(t.Name())}}
	if _, err := clientSet.CoreV1().Namespaces().Create(ctx, testNamespace, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Unable to create ns %v", testNamespace.Name)
	}
	defer clientSet.CoreV1().Namespaces().Delete(ctx, testNamespace.Name, metav1.DeleteOptions{})

	rc := RcByNameContainer("test-rc-podlifetime", testNamespace.Name, 5, map[string]string{"test": "podlifetime-include"}, nil, "")
	if _, err := clientSet.CoreV1().ReplicationControllers(rc.Namespace).Create(ctx, rc, metav1.CreateOptions{}); err != nil {
		t.Errorf("Error creating deployment %v", err)
	}
	defer deleteRC(ctx, t, clientSet, rc)

	// wait for a while so all the pods are at least few seconds older
	time.Sleep(5 * time.Second)

	// it's assumed all new pods are named differently from currently running -> no name collision
	podList, err := clientSet.CoreV1().Pods(rc.Namespace).List(ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(rc.Spec.Template.Labels).String()})
	if err != nil {
		t.Fatalf("Unable to list pods: %v", err)
	}

	if len(podList.Items) != 5 {
		t.Fatalf("Expected 5 replicas, got %v instead", len(podList.Items))
	}

	initialPodNames := getPodNames(podList.Items)
	sort.Strings(initialPodNames)
	t.Logf("Existing pods: %v", initialPodNames)

	t.Logf("set the strategy to delete pods from %v namespace", rc.Namespace)
	runPodLifetimeStrategy(ctx, clientSet, nodeInformer, &deschedulerapi.Namespaces{
		Include: []string{rc.Namespace},
	}, "", nil)

	// All pods are supposed to be deleted, wait until all the old pods are deleted
	if err := wait.PollImmediate(time.Second, 20*time.Second, func() (bool, error) {
		podList, err := clientSet.CoreV1().Pods(rc.Namespace).List(ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(rc.Spec.Template.Labels).String()})
		if err != nil {
			return false, nil
		}

		includePodNames := getPodNames(podList.Items)
		// validate all pod were deleted
		if len(intersectStrings(initialPodNames, includePodNames)) > 0 {
			t.Logf("Waiting until %v pods get deleted", intersectStrings(initialPodNames, includePodNames))
			// check if there's at least one pod not in Terminating state
			for _, pod := range podList.Items {
				// In case podList contains newly created pods
				if len(intersectStrings(initialPodNames, []string{pod.Name})) == 0 {
					continue
				}
				if pod.DeletionTimestamp == nil {
					t.Logf("Pod %v not in terminating state", pod.Name)
					return false, nil
				}
			}
			t.Logf("All %v pods are terminating", intersectStrings(initialPodNames, includePodNames))
		}

		return true, nil
	}); err != nil {
		t.Fatalf("Error waiting for pods to be deleted: %v", err)
	}
}

func TestNamespaceConstraintsExclude(t *testing.T) {
	ctx := context.Background()

	clientSet, nodeInformer, stopCh := initializeClient(t)
	defer close(stopCh)

	testNamespace := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "e2e-" + strings.ToLower(t.Name())}}
	if _, err := clientSet.CoreV1().Namespaces().Create(ctx, testNamespace, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Unable to create ns %v", testNamespace.Name)
	}
	defer clientSet.CoreV1().Namespaces().Delete(ctx, testNamespace.Name, metav1.DeleteOptions{})

	rc := RcByNameContainer("test-rc-podlifetime", testNamespace.Name, 5, map[string]string{"test": "podlifetime-exclude"}, nil, "")
	if _, err := clientSet.CoreV1().ReplicationControllers(rc.Namespace).Create(ctx, rc, metav1.CreateOptions{}); err != nil {
		t.Errorf("Error creating deployment %v", err)
	}
	defer deleteRC(ctx, t, clientSet, rc)

	// wait for a while so all the pods are at least few seconds older
	time.Sleep(5 * time.Second)

	// it's assumed all new pods are named differently from currently running -> no name collision
	podList, err := clientSet.CoreV1().Pods(rc.Namespace).List(ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(rc.Spec.Template.Labels).String()})
	if err != nil {
		t.Fatalf("Unable to list pods: %v", err)
	}

	if len(podList.Items) != 5 {
		t.Fatalf("Expected 5 replicas, got %v instead", len(podList.Items))
	}

	initialPodNames := getPodNames(podList.Items)
	sort.Strings(initialPodNames)
	t.Logf("Existing pods: %v", initialPodNames)

	t.Logf("set the strategy to delete pods from namespaces except the %v namespace", rc.Namespace)
	runPodLifetimeStrategy(ctx, clientSet, nodeInformer, &deschedulerapi.Namespaces{
		Exclude: []string{rc.Namespace},
	}, "", nil)

	t.Logf("Waiting 10s")
	time.Sleep(10 * time.Second)
	podList, err = clientSet.CoreV1().Pods(rc.Namespace).List(ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(rc.Spec.Template.Labels).String()})
	if err != nil {
		t.Fatalf("Unable to list pods after running strategy: %v", err)
	}

	excludePodNames := getPodNames(podList.Items)
	sort.Strings(excludePodNames)
	t.Logf("Existing pods: %v", excludePodNames)

	// validate no pods were deleted
	if len(intersectStrings(initialPodNames, excludePodNames)) != 5 {
		t.Fatalf("None of %v pods are expected to be deleted", initialPodNames)
	}
}

func TestThresholdPriority(t *testing.T) {
	testPriority(t, false)
}

func TestThresholdPriorityClass(t *testing.T) {
	testPriority(t, true)
}

func testPriority(t *testing.T, isPriorityClass bool) {
	var highPriority = int32(1000)
	var lowPriority = int32(500)
	ctx := context.Background()

	clientSet, nodeInformer, stopCh := initializeClient(t)
	defer close(stopCh)

	testNamespace := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "e2e-" + strings.ToLower(t.Name())}}
	if _, err := clientSet.CoreV1().Namespaces().Create(ctx, testNamespace, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Unable to create ns %v", testNamespace.Name)
	}
	defer clientSet.CoreV1().Namespaces().Delete(ctx, testNamespace.Name, metav1.DeleteOptions{})

	// create two priority classes
	highPriorityClass := &schedulingv1.PriorityClass{
		ObjectMeta: metav1.ObjectMeta{Name: "e2e-" + strings.ToLower(t.Name()) + "-highpriority"},
		Value:      highPriority,
	}
	if _, err := clientSet.SchedulingV1().PriorityClasses().Create(ctx, highPriorityClass, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Error creating priorityclass %s: %v", highPriorityClass.Name, err)
	}
	defer clientSet.SchedulingV1().PriorityClasses().Delete(ctx, highPriorityClass.Name, metav1.DeleteOptions{})

	lowPriorityClass := &schedulingv1.PriorityClass{
		ObjectMeta: metav1.ObjectMeta{Name: "e2e-" + strings.ToLower(t.Name()) + "-lowpriority"},
		Value:      lowPriority,
	}
	if _, err := clientSet.SchedulingV1().PriorityClasses().Create(ctx, lowPriorityClass, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Error creating priorityclass %s: %v", lowPriorityClass.Name, err)
	}
	defer clientSet.SchedulingV1().PriorityClasses().Delete(ctx, lowPriorityClass.Name, metav1.DeleteOptions{})

	// create two RCs with different priority classes in the same namespace
	rcHighPriority := RcByNameContainer("test-rc-podlifetime-highpriority", testNamespace.Name, 5,
		map[string]string{"test": "podlifetime-highpriority"}, nil, highPriorityClass.Name)
	if _, err := clientSet.CoreV1().ReplicationControllers(rcHighPriority.Namespace).Create(ctx, rcHighPriority, metav1.CreateOptions{}); err != nil {
		t.Errorf("Error creating rc %s: %v", rcHighPriority.Name, err)
	}
	defer deleteRC(ctx, t, clientSet, rcHighPriority)

	rcLowPriority := RcByNameContainer("test-rc-podlifetime-lowpriority", testNamespace.Name, 5,
		map[string]string{"test": "podlifetime-lowpriority"}, nil, lowPriorityClass.Name)
	if _, err := clientSet.CoreV1().ReplicationControllers(rcLowPriority.Namespace).Create(ctx, rcLowPriority, metav1.CreateOptions{}); err != nil {
		t.Errorf("Error creating rc %s: %v", rcLowPriority.Name, err)
	}
	defer deleteRC(ctx, t, clientSet, rcLowPriority)

	// wait for a while so all the pods are at least few seconds older
	time.Sleep(5 * time.Second)

	// it's assumed all new pods are named differently from currently running -> no name collision
	podListHighPriority, err := clientSet.CoreV1().Pods(rcHighPriority.Namespace).List(
		ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(rcHighPriority.Spec.Template.Labels).String()})
	if err != nil {
		t.Fatalf("Unable to list pods: %v", err)
	}
	podListLowPriority, err := clientSet.CoreV1().Pods(rcLowPriority.Namespace).List(
		ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(rcLowPriority.Spec.Template.Labels).String()})
	if err != nil {
		t.Fatalf("Unable to list pods: %v", err)
	}

	if len(podListHighPriority.Items)+len(podListLowPriority.Items) != 10 {
		t.Fatalf("Expected 10 replicas, got %v instead", len(podListHighPriority.Items)+len(podListLowPriority.Items))
	}

	expectReservePodNames := getPodNames(podListHighPriority.Items)
	expectEvictPodNames := getPodNames(podListLowPriority.Items)
	sort.Strings(expectReservePodNames)
	sort.Strings(expectEvictPodNames)
	t.Logf("Pods not expect to be evicted: %v, pods expect to be evicted: %v", expectReservePodNames, expectEvictPodNames)

	if isPriorityClass {
		t.Logf("set the strategy to delete pods with priority lower than priority class %s", highPriorityClass.Name)
		runPodLifetimeStrategy(ctx, clientSet, nodeInformer, nil, highPriorityClass.Name, nil)
	} else {
		t.Logf("set the strategy to delete pods with priority lower than %d", highPriority)
		runPodLifetimeStrategy(ctx, clientSet, nodeInformer, nil, "", &highPriority)
	}

	t.Logf("Waiting 10s")
	time.Sleep(10 * time.Second)
	// check if all pods with high priority class are not evicted
	podListHighPriority, err = clientSet.CoreV1().Pods(rcHighPriority.Namespace).List(
		ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(rcHighPriority.Spec.Template.Labels).String()})
	if err != nil {
		t.Fatalf("Unable to list pods after running strategy: %v", err)
	}

	excludePodNames := getPodNames(podListHighPriority.Items)
	sort.Strings(excludePodNames)
	t.Logf("Existing high priority pods: %v", excludePodNames)

	// validate no pods were deleted
	if len(intersectStrings(expectReservePodNames, excludePodNames)) != 5 {
		t.Fatalf("None of %v high priority pods are expected to be deleted", expectReservePodNames)
	}

	//check if all pods with low priority class are evicted
	if err := wait.PollImmediate(time.Second, 20*time.Second, func() (bool, error) {
		podListLowPriority, err := clientSet.CoreV1().Pods(rcLowPriority.Namespace).List(
			ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(rcLowPriority.Spec.Template.Labels).String()})
		if err != nil {
			return false, nil
		}

		includePodNames := getPodNames(podListLowPriority.Items)
		// validate all pod were deleted
		if len(intersectStrings(expectEvictPodNames, includePodNames)) > 0 {
			t.Logf("Waiting until %v low priority pods get deleted", intersectStrings(expectEvictPodNames, includePodNames))
			// check if there's at least one pod not in Terminating state
			for _, pod := range podListLowPriority.Items {
				// In case podList contains newly created pods
				if len(intersectStrings(expectEvictPodNames, []string{pod.Name})) == 0 {
					continue
				}
				if pod.DeletionTimestamp == nil {
					t.Logf("Pod %v not in terminating state", pod.Name)
					return false, nil
				}
			}
			t.Logf("All %v pods are terminating", intersectStrings(expectEvictPodNames, includePodNames))
		}

		return true, nil
	}); err != nil {
		t.Fatalf("Error waiting for pods to be deleted: %v", err)
	}
}

func TestEvictAnnotation(t *testing.T) {
	ctx := context.Background()

	clientSet, nodeInformer, stopCh := initializeClient(t)
	defer close(stopCh)

	nodeList, err := clientSet.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Errorf("Error listing node with %v", err)
	}

	var nodes []*v1.Node
	for i := range nodeList.Items {
		node := nodeList.Items[i]
		nodes = append(nodes, &node)
	}

	testNamespace := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "e2e-" + strings.ToLower(t.Name())}}
	if _, err := clientSet.CoreV1().Namespaces().Create(ctx, testNamespace, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Unable to create ns %v", testNamespace.Name)
	}
	defer clientSet.CoreV1().Namespaces().Delete(ctx, testNamespace.Name, metav1.DeleteOptions{})

	rc := RcByNameContainer("test-rc-evict-annotation", testNamespace.Name, int32(15), map[string]string{"test": "annotation"}, nil, "")
	rc.Spec.Template.Annotations = map[string]string{"descheduler.alpha.kubernetes.io/evict": "true"}
	rc.Spec.Template.Spec.Volumes = []v1.Volume{
		{
			Name: "sample",
			VolumeSource: v1.VolumeSource{
				EmptyDir: &v1.EmptyDirVolumeSource{
					SizeLimit: resource.NewQuantity(int64(10), resource.BinarySI)},
			},
		},
	}

	if _, err := clientSet.CoreV1().ReplicationControllers(rc.Namespace).Create(ctx, rc, metav1.CreateOptions{}); err != nil {
		t.Errorf("Error creating deployment %v", err)
	}

	evictPods(ctx, t, clientSet, nodeInformer, nodes, rc)
	deleteRC(ctx, t, clientSet, rc)
}

func TestDeschedulingInterval(t *testing.T) {
	ctx := context.Background()
	clientSet, err := client.CreateClient(os.Getenv("KUBECONFIG"))
	if err != nil {
		t.Errorf("Error during client creation with %v", err)
	}

	// By default, the DeschedulingInterval param should be set to 0, meaning Descheduler only runs once then exits
	s := options.NewDeschedulerServer()
	s.Client = clientSet

	deschedulerPolicy := &api.DeschedulerPolicy{}

	c := make(chan bool)
	go func() {
		evictionPolicyGroupVersion, err := eutils.SupportEviction(s.Client)
		if err != nil || len(evictionPolicyGroupVersion) == 0 {
			t.Errorf("Error when checking support for eviction: %v", err)
		}

		stopChannel := make(chan struct{})
		if err := descheduler.RunDeschedulerStrategies(ctx, s, deschedulerPolicy, evictionPolicyGroupVersion, stopChannel); err != nil {
			t.Errorf("Error running descheduler strategies: %+v", err)
		}
		c <- true
	}()

	select {
	case <-c:
		// successfully returned
	case <-time.After(3 * time.Minute):
		t.Errorf("descheduler.Run timed out even without descheduling-interval set")
	}
}

func deleteRC(ctx context.Context, t *testing.T, clientSet clientset.Interface, rc *v1.ReplicationController) {
	//set number of replicas to 0
	rcdeepcopy := rc.DeepCopy()
	rcdeepcopy.Spec.Replicas = func(i int32) *int32 { return &i }(0)
	if _, err := clientSet.CoreV1().ReplicationControllers(rcdeepcopy.Namespace).Update(ctx, rcdeepcopy, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("Error updating replica controller %v", err)
	}
	allPodsDeleted := false
	//wait 30 seconds until all pods are deleted
	for i := 0; i < 6; i++ {
		scale, _ := clientSet.CoreV1().ReplicationControllers(rc.Namespace).GetScale(ctx, rc.Name, metav1.GetOptions{})
		if scale.Spec.Replicas == 0 {
			allPodsDeleted = true
			break
		}
		time.Sleep(5 * time.Second)
	}

	if !allPodsDeleted {
		t.Errorf("Deleting of rc pods took too long")
	}

	if err := clientSet.CoreV1().ReplicationControllers(rc.Namespace).Delete(ctx, rc.Name, metav1.DeleteOptions{}); err != nil {
		t.Fatalf("Error deleting rc %v", err)
	}

	if err := wait.PollImmediate(time.Second, 10*time.Second, func() (bool, error) {
		_, err := clientSet.CoreV1().ReplicationControllers(rc.Namespace).Get(ctx, rc.Name, metav1.GetOptions{})
		if err != nil && strings.Contains(err.Error(), "not found") {
			return true, nil
		}
		return false, nil
	}); err != nil {
		t.Fatalf("Error deleting rc %v", err)
	}
}

func evictPods(ctx context.Context, t *testing.T, clientSet clientset.Interface, nodeInformer coreinformers.NodeInformer, nodeList []*v1.Node, rc *v1.ReplicationController) {
	var leastLoadedNode *v1.Node
	podsBefore := math.MaxInt16
	evictionPolicyGroupVersion, err := eutils.SupportEviction(clientSet)
	if err != nil || len(evictionPolicyGroupVersion) == 0 {
		klog.Fatalf("%v", err)
	}
	podEvictor := evictions.NewPodEvictor(
		clientSet,
		evictionPolicyGroupVersion,
		false,
		0,
		nodeList,
		true,
	)
	for _, node := range nodeList {
		// Skip the Master Node
		if _, exist := node.Labels["node-role.kubernetes.io/master"]; exist {
			continue
		}
		// List all the pods on the current Node
		podsOnANode, err := podutil.ListPodsOnANode(ctx, clientSet, node, podutil.WithFilter(podEvictor.Evictable().IsEvictable))
		if err != nil {
			t.Errorf("Error listing pods on a node %v", err)
		}
		// Update leastLoadedNode if necessary
		if tmpLoads := len(podsOnANode); tmpLoads < podsBefore {
			leastLoadedNode = node
			podsBefore = tmpLoads
		}
	}
	t.Log("Eviction of pods starting")
	startEndToEndForLowNodeUtilization(ctx, clientSet, nodeInformer, podEvictor)
	podsOnleastUtilizedNode, err := podutil.ListPodsOnANode(ctx, clientSet, leastLoadedNode, podutil.WithFilter(podEvictor.Evictable().IsEvictable))
	if err != nil {
		t.Errorf("Error listing pods on a node %v", err)
	}
	podsAfter := len(podsOnleastUtilizedNode)
	if podsBefore > podsAfter {
		t.Fatalf("We should have see more pods on this node as per kubeadm's way of installing %v, %v", podsBefore, podsAfter)
	}
}
