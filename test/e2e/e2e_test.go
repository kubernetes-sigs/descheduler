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
	"fmt"
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
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	v1qos "k8s.io/kubectl/pkg/util/qos"

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
	"sigs.k8s.io/descheduler/pkg/descheduler/strategies/nodeutilization"
	"sigs.k8s.io/descheduler/pkg/utils"
)

func MakePodSpec(priorityClassName string, gracePeriod *int64) v1.PodSpec {
	return v1.PodSpec{
		Containers: []v1.Container{{
			Name:            "pause",
			ImagePullPolicy: "Never",
			Image:           "kubernetes/pause",
			Ports:           []v1.ContainerPort{{ContainerPort: 80}},
			Resources: v1.ResourceRequirements{
				Limits: v1.ResourceList{
					v1.ResourceCPU:    resource.MustParse("100m"),
					v1.ResourceMemory: resource.MustParse("200Mi"),
				},
				Requests: v1.ResourceList{
					v1.ResourceCPU:    resource.MustParse("100m"),
					v1.ResourceMemory: resource.MustParse("100Mi"),
				},
			},
		}},
		PriorityClassName:             priorityClassName,
		TerminationGracePeriodSeconds: gracePeriod,
	}
}

// RcByNameContainer returns a ReplicationController with specified name and container
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
				Spec: MakePodSpec(priorityClassName, gracePeriod),
			},
		},
	}
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

func runPodLifetimeStrategy(
	ctx context.Context,
	t *testing.T,
	clientset clientset.Interface,
	nodeInformer coreinformers.NodeInformer,
	namespaces *deschedulerapi.Namespaces,
	priorityClass string,
	priority *int32,
	evictCritical bool,
	labelSelector *metav1.LabelSelector,
) {
	// Run descheduler.
	evictionPolicyGroupVersion, err := eutils.SupportEviction(clientset)
	if err != nil || len(evictionPolicyGroupVersion) == 0 {
		t.Fatalf("%v", err)
	}

	nodes, err := nodeutil.ReadyNodes(ctx, clientset, nodeInformer, "")
	if err != nil {
		t.Fatalf("%v", err)
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
				LabelSelector:              labelSelector,
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
			evictCritical,
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

func TestLowNodeUtilization(t *testing.T) {
	ctx := context.Background()

	clientSet, _, stopCh := initializeClient(t)
	defer close(stopCh)

	nodeList, err := clientSet.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Errorf("Error listing node with %v", err)
	}

	nodes, workerNodes := splitNodesAndWorkerNodes(nodeList.Items)

	t.Log("Creating testing namespace")
	testNamespace := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "e2e-" + strings.ToLower(t.Name())}}
	if _, err := clientSet.CoreV1().Namespaces().Create(ctx, testNamespace, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Unable to create ns %v", testNamespace.Name)
	}
	defer clientSet.CoreV1().Namespaces().Delete(ctx, testNamespace.Name, metav1.DeleteOptions{})

	// Make all worker nodes resource balanced
	cleanUp, err := createBalancedPodForNodes(t, ctx, clientSet, testNamespace.Name, workerNodes, 0.5)
	if err != nil {
		t.Fatalf("Unable to create load balancing pods: %v", err)
	}
	defer cleanUp()

	t.Log("Creating pods each consuming 10% of node's allocatable")
	nodeCpu := workerNodes[0].Status.Allocatable[v1.ResourceCPU]
	tenthOfCpu := int64(float64((&nodeCpu).MilliValue()) * 0.1)

	t.Log("Creating pods all bound to a single node")
	for i := 0; i < 4; i++ {
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("lnu-pod-%v", i),
				Namespace: testNamespace.Name,
				Labels:    map[string]string{"test": "node-utilization", "name": "test-rc-node-utilization"},
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{{
					Name:            "pause",
					ImagePullPolicy: "Never",
					Image:           "kubernetes/pause",
					Ports:           []v1.ContainerPort{{ContainerPort: 80}},
					Resources: v1.ResourceRequirements{
						Limits: v1.ResourceList{
							v1.ResourceCPU: *resource.NewMilliQuantity(tenthOfCpu, resource.DecimalSI),
						},
						Requests: v1.ResourceList{
							v1.ResourceCPU: *resource.NewMilliQuantity(tenthOfCpu, resource.DecimalSI),
						},
					},
				}},
				Affinity: &v1.Affinity{
					NodeAffinity: &v1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
							NodeSelectorTerms: []v1.NodeSelectorTerm{
								{
									MatchFields: []v1.NodeSelectorRequirement{
										{Key: "metadata.name", Operator: v1.NodeSelectorOpIn, Values: []string{workerNodes[0].Name}},
									},
								},
							},
						},
					},
				},
			},
		}

		t.Logf("Creating pod %v in %v namespace for node %v", pod.Name, pod.Namespace, workerNodes[0].Name)
		_, err := clientSet.CoreV1().Pods(pod.Namespace).Create(ctx, pod, metav1.CreateOptions{})
		if err != nil {
			t.Logf("Error creating LNU pods: %v", err)
			if err = clientSet.CoreV1().Pods(pod.Namespace).DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{
				LabelSelector: labels.SelectorFromSet(labels.Set(map[string]string{"test": "node-utilization", "name": "test-rc-node-utilization"})).String(),
			}); err != nil {
				t.Fatalf("Unable to delete LNU pods: %v", err)
			}
			return
		}
	}

	t.Log("Creating RC with 4 replicas owning the created pods")
	rc := RcByNameContainer("test-rc-node-utilization", testNamespace.Name, int32(4), map[string]string{"test": "node-utilization"}, nil, "")
	if _, err := clientSet.CoreV1().ReplicationControllers(rc.Namespace).Create(ctx, rc, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Error creating RC %v", err)
	}
	defer deleteRC(ctx, t, clientSet, rc)
	waitForRCPodsRunning(ctx, t, clientSet, rc)

	// Run LowNodeUtilization strategy
	podEvictor := initPodEvictorOrFail(t, clientSet, nodes)

	podsOnMosttUtilizedNode, err := podutil.ListPodsOnANode(ctx, clientSet, workerNodes[0], podutil.WithFilter(podEvictor.Evictable().IsEvictable))
	if err != nil {
		t.Errorf("Error listing pods on a node %v", err)
	}
	podsBefore := len(podsOnMosttUtilizedNode)

	t.Log("Running LowNodeUtilization strategy")
	nodeutilization.LowNodeUtilization(
		ctx,
		clientSet,
		deschedulerapi.DeschedulerStrategy{
			Enabled: true,
			Params: &deschedulerapi.StrategyParameters{
				NodeResourceUtilizationThresholds: &deschedulerapi.NodeResourceUtilizationThresholds{
					Thresholds: deschedulerapi.ResourceThresholds{
						v1.ResourceCPU: 70,
					},
					TargetThresholds: deschedulerapi.ResourceThresholds{
						v1.ResourceCPU: 80,
					},
				},
			},
		},
		workerNodes,
		podEvictor,
	)

	waitForTerminatingPodsToDisappear(ctx, t, clientSet, rc.Namespace)

	podsOnMosttUtilizedNode, err = podutil.ListPodsOnANode(ctx, clientSet, workerNodes[0], podutil.WithFilter(podEvictor.Evictable().IsEvictable))
	if err != nil {
		t.Errorf("Error listing pods on a node %v", err)
	}
	podsAfter := len(podsOnMosttUtilizedNode)

	if podsAfter >= podsBefore {
		t.Fatalf("No pod has been evicted from %v node", workerNodes[0].Name)
	}
	t.Logf("Number of pods on node %v changed from %v to %v", workerNodes[0].Name, podsBefore, podsAfter)
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
	runPodLifetimeStrategy(ctx, t, clientSet, nodeInformer, &deschedulerapi.Namespaces{
		Include: []string{rc.Namespace},
	}, "", nil, false, nil)

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
	runPodLifetimeStrategy(ctx, t, clientSet, nodeInformer, &deschedulerapi.Namespaces{
		Exclude: []string{rc.Namespace},
	}, "", nil, false, nil)

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

func TestEvictSystemCriticalPriority(t *testing.T) {
	testEvictSystemCritical(t, false)
}

func TestEvictSystemCriticalPriorityClass(t *testing.T) {
	testEvictSystemCritical(t, true)
}

func testEvictSystemCritical(t *testing.T, isPriorityClass bool) {
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

	// Create a replication controller with the "system-node-critical" priority class (this gives the pods a priority of 2000001000)
	rcCriticalPriority := RcByNameContainer("test-rc-podlifetime-criticalpriority", testNamespace.Name, 3,
		map[string]string{"test": "podlifetime-criticalpriority"}, nil, "system-node-critical")
	if _, err := clientSet.CoreV1().ReplicationControllers(rcCriticalPriority.Namespace).Create(ctx, rcCriticalPriority, metav1.CreateOptions{}); err != nil {
		t.Errorf("Error creating rc %s: %v", rcCriticalPriority.Name, err)
	}
	defer deleteRC(ctx, t, clientSet, rcCriticalPriority)

	// create two RCs with different priority classes in the same namespace
	rcHighPriority := RcByNameContainer("test-rc-podlifetime-highpriority", testNamespace.Name, 3,
		map[string]string{"test": "podlifetime-highpriority"}, nil, highPriorityClass.Name)
	if _, err := clientSet.CoreV1().ReplicationControllers(rcHighPriority.Namespace).Create(ctx, rcHighPriority, metav1.CreateOptions{}); err != nil {
		t.Errorf("Error creating rc %s: %v", rcHighPriority.Name, err)
	}
	defer deleteRC(ctx, t, clientSet, rcHighPriority)

	rcLowPriority := RcByNameContainer("test-rc-podlifetime-lowpriority", testNamespace.Name, 3,
		map[string]string{"test": "podlifetime-lowpriority"}, nil, lowPriorityClass.Name)
	if _, err := clientSet.CoreV1().ReplicationControllers(rcLowPriority.Namespace).Create(ctx, rcLowPriority, metav1.CreateOptions{}); err != nil {
		t.Errorf("Error creating rc %s: %v", rcLowPriority.Name, err)
	}
	defer deleteRC(ctx, t, clientSet, rcLowPriority)

	// wait for a while so all the pods are at least few seconds older
	time.Sleep(5 * time.Second)

	// it's assumed all new pods are named differently from currently running -> no name collision
	podListCriticalPriority, err := clientSet.CoreV1().Pods(rcCriticalPriority.Namespace).List(ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(rcCriticalPriority.Spec.Template.Labels).String()})
	if err != nil {
		t.Fatalf("Unable to list pods: %v", err)
	}

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

	if (len(podListHighPriority.Items) + len(podListLowPriority.Items) + len(podListCriticalPriority.Items)) != 9 {
		t.Fatalf("Expected 9 replicas, got %v instead", len(podListHighPriority.Items)+len(podListLowPriority.Items)+len(podListCriticalPriority.Items))
	}

	initialPodNames := append(getPodNames(podListHighPriority.Items), getPodNames(podListLowPriority.Items)...)
	initialPodNames = append(initialPodNames, getPodNames(podListCriticalPriority.Items)...)
	sort.Strings(initialPodNames)
	t.Logf("Existing pods: %v", initialPodNames)

	if isPriorityClass {
		runPodLifetimeStrategy(ctx, t, clientSet, nodeInformer, nil, highPriorityClass.Name, nil, true, nil)
	} else {
		runPodLifetimeStrategy(ctx, t, clientSet, nodeInformer, nil, "", &highPriority, true, nil)
	}

	// All pods are supposed to be deleted, wait until all pods in the test namespace are terminating
	t.Logf("All pods in the test namespace, no matter their priority (including system-node-critical and system-cluster-critical), will be deleted")
	if err := wait.PollImmediate(time.Second, 30*time.Second, func() (bool, error) {
		podList, err := clientSet.CoreV1().Pods(testNamespace.Name).List(ctx, metav1.ListOptions{})
		if err != nil {
			return false, nil
		}
		currentPodNames := getPodNames(podList.Items)
		// validate all pod were deleted
		if len(intersectStrings(initialPodNames, currentPodNames)) > 0 {
			t.Logf("Waiting until %v pods get deleted", intersectStrings(initialPodNames, currentPodNames))
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
			t.Logf("All %v pods are terminating", intersectStrings(initialPodNames, currentPodNames))
		}

		return true, nil
	}); err != nil {
		t.Fatalf("Error waiting for pods to be deleted: %v", err)
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
	t.Logf("Pods not expected to be evicted: %v, pods expected to be evicted: %v", expectReservePodNames, expectEvictPodNames)

	if isPriorityClass {
		t.Logf("set the strategy to delete pods with priority lower than priority class %s", highPriorityClass.Name)
		runPodLifetimeStrategy(ctx, t, clientSet, nodeInformer, nil, highPriorityClass.Name, nil, false, nil)
	} else {
		t.Logf("set the strategy to delete pods with priority lower than %d", highPriority)
		runPodLifetimeStrategy(ctx, t, clientSet, nodeInformer, nil, "", &highPriority, false, nil)
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
	if err := wait.PollImmediate(time.Second, 30*time.Second, func() (bool, error) {
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

func TestPodLabelSelector(t *testing.T) {
	ctx := context.Background()

	clientSet, nodeInformer, stopCh := initializeClient(t)
	defer close(stopCh)

	testNamespace := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "e2e-" + strings.ToLower(t.Name())}}
	if _, err := clientSet.CoreV1().Namespaces().Create(ctx, testNamespace, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Unable to create ns %v", testNamespace.Name)
	}
	defer clientSet.CoreV1().Namespaces().Delete(ctx, testNamespace.Name, metav1.DeleteOptions{})

	// create two replicationControllers with different labels
	rcEvict := RcByNameContainer("test-rc-podlifetime-evict", testNamespace.Name, 5, map[string]string{"test": "podlifetime-evict"}, nil, "")
	if _, err := clientSet.CoreV1().ReplicationControllers(rcEvict.Namespace).Create(ctx, rcEvict, metav1.CreateOptions{}); err != nil {
		t.Errorf("Error creating rc %v", err)
	}
	defer deleteRC(ctx, t, clientSet, rcEvict)

	rcReserve := RcByNameContainer("test-rc-podlifetime-reserve", testNamespace.Name, 5, map[string]string{"test": "podlifetime-reserve"}, nil, "")
	if _, err := clientSet.CoreV1().ReplicationControllers(rcReserve.Namespace).Create(ctx, rcReserve, metav1.CreateOptions{}); err != nil {
		t.Errorf("Error creating rc %v", err)
	}
	defer deleteRC(ctx, t, clientSet, rcReserve)

	// wait for a while so all the pods are at least few seconds older
	time.Sleep(5 * time.Second)

	// it's assumed all new pods are named differently from currently running -> no name collision
	podListEvict, err := clientSet.CoreV1().Pods(rcEvict.Namespace).List(
		ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(rcEvict.Spec.Template.Labels).String()})
	if err != nil {
		t.Fatalf("Unable to list pods: %v", err)
	}
	podListReserve, err := clientSet.CoreV1().Pods(rcReserve.Namespace).List(
		ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(rcReserve.Spec.Template.Labels).String()})
	if err != nil {
		t.Fatalf("Unable to list pods: %v", err)
	}

	if len(podListEvict.Items)+len(podListReserve.Items) != 10 {
		t.Fatalf("Expected 10 replicas, got %v instead", len(podListEvict.Items)+len(podListReserve.Items))
	}

	expectReservePodNames := getPodNames(podListReserve.Items)
	expectEvictPodNames := getPodNames(podListEvict.Items)
	sort.Strings(expectReservePodNames)
	sort.Strings(expectEvictPodNames)
	t.Logf("Pods not expected to be evicted: %v, pods expected to be evicted: %v", expectReservePodNames, expectEvictPodNames)

	t.Logf("set the strategy to delete pods with label test:podlifetime-evict")
	runPodLifetimeStrategy(ctx, t, clientSet, nodeInformer, nil, "", nil, false, &metav1.LabelSelector{MatchLabels: map[string]string{"test": "podlifetime-evict"}})

	t.Logf("Waiting 10s")
	time.Sleep(10 * time.Second)
	// check if all pods without target label are not evicted
	podListReserve, err = clientSet.CoreV1().Pods(rcReserve.Namespace).List(
		ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(rcReserve.Spec.Template.Labels).String()})
	if err != nil {
		t.Fatalf("Unable to list pods after running strategy: %v", err)
	}

	reservedPodNames := getPodNames(podListReserve.Items)
	sort.Strings(reservedPodNames)
	t.Logf("Existing reserved pods: %v", reservedPodNames)

	// validate no pods were deleted
	if len(intersectStrings(expectReservePodNames, reservedPodNames)) != 5 {
		t.Fatalf("None of %v unevictable pods are expected to be deleted", expectReservePodNames)
	}

	//check if all selected pods are evicted
	if err := wait.PollImmediate(time.Second, 30*time.Second, func() (bool, error) {
		podListEvict, err := clientSet.CoreV1().Pods(rcEvict.Namespace).List(
			ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(rcEvict.Spec.Template.Labels).String()})
		if err != nil {
			return false, nil
		}

		newPodNames := getPodNames(podListEvict.Items)
		// validate all pod were deleted
		if len(intersectStrings(expectEvictPodNames, newPodNames)) > 0 {
			t.Logf("Waiting until %v selected pods get deleted", intersectStrings(expectEvictPodNames, newPodNames))
			// check if there's at least one pod not in Terminating state
			for _, pod := range podListEvict.Items {
				// In case podList contains newly created pods
				if len(intersectStrings(expectEvictPodNames, []string{pod.Name})) == 0 {
					continue
				}
				if pod.DeletionTimestamp == nil {
					t.Logf("Pod %v not in terminating state", pod.Name)
					return false, nil
				}
			}
			t.Logf("All %v pods are terminating", intersectStrings(expectEvictPodNames, newPodNames))
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

	t.Log("Create RC with pods with local storage which require descheduler.alpha.kubernetes.io/evict annotation to be set for eviction")
	rc := RcByNameContainer("test-rc-evict-annotation", testNamespace.Name, int32(5), map[string]string{"test": "annotation"}, nil, "")
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
	defer deleteRC(ctx, t, clientSet, rc)

	t.Logf("Waiting 10s to make pods 10s old")
	time.Sleep(10 * time.Second)

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

	t.Log("Running PodLifetime strategy")
	runPodLifetimeStrategy(ctx, t, clientSet, nodeInformer, nil, "", nil, false, nil)

	if err := wait.PollImmediate(5*time.Second, time.Minute, func() (bool, error) {
		podList, err = clientSet.CoreV1().Pods(rc.Namespace).List(ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(rc.Spec.Template.Labels).String()})
		if err != nil {
			return false, fmt.Errorf("Unable to list pods after running strategy: %v", err)
		}

		excludePodNames := getPodNames(podList.Items)
		sort.Strings(excludePodNames)
		t.Logf("Existing pods: %v", excludePodNames)

		// validate no pods were deleted
		if len(intersectStrings(initialPodNames, excludePodNames)) > 0 {
			t.Logf("Not every pods was evicted")
			return false, nil
		}

		return true, nil
	}); err != nil {
		t.Fatalf("Error waiting for pods to be deleted: %v", err)
	}
}

func TestDeschedulingInterval(t *testing.T) {
	ctx := context.Background()
	clientSet, err := client.CreateClient(os.Getenv("KUBECONFIG"))
	if err != nil {
		t.Errorf("Error during client creation with %v", err)
	}

	// By default, the DeschedulingInterval param should be set to 0, meaning Descheduler only runs once then exits
	s, err := options.NewDeschedulerServer()
	if err != nil {
		t.Fatalf("Unable to initialize server: %v", err)
	}
	s.Client = clientSet

	deschedulerPolicy := &api.DeschedulerPolicy{}

	c := make(chan bool, 1)
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

func waitForRCPodsRunning(ctx context.Context, t *testing.T, clientSet clientset.Interface, rc *v1.ReplicationController) {
	if err := wait.PollImmediate(5*time.Second, 30*time.Second, func() (bool, error) {
		podList, err := clientSet.CoreV1().Pods(rc.Namespace).List(ctx, metav1.ListOptions{
			LabelSelector: labels.SelectorFromSet(labels.Set(rc.Spec.Template.ObjectMeta.Labels)).String(),
		})
		if err != nil {
			return false, err
		}
		if len(podList.Items) != int(*rc.Spec.Replicas) {
			t.Logf("Waiting for %v pods to be created, got %v instead", *rc.Spec.Replicas, len(podList.Items))
			return false, nil
		}
		for _, pod := range podList.Items {
			if pod.Status.Phase != v1.PodRunning {
				t.Logf("Pod %v not running yet, is %v instead", pod.Name, pod.Status.Phase)
				return false, nil
			}
		}
		return true, nil
	}); err != nil {
		t.Fatalf("Error waiting for pods running: %v", err)
	}
}

func waitForTerminatingPodsToDisappear(ctx context.Context, t *testing.T, clientSet clientset.Interface, namespace string) {
	if err := wait.PollImmediate(5*time.Second, 30*time.Second, func() (bool, error) {
		podList, err := clientSet.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return false, err
		}
		for _, pod := range podList.Items {
			if pod.DeletionTimestamp != nil {
				t.Logf("Pod %v still terminating", pod.Name)
				return false, nil
			}
		}
		return true, nil
	}); err != nil {
		t.Fatalf("Error waiting for terminating pods to disappear: %v", err)
	}
}

func deleteRC(ctx context.Context, t *testing.T, clientSet clientset.Interface, rc *v1.ReplicationController) {
	// set number of replicas to 0
	rcdeepcopy := rc.DeepCopy()
	rcdeepcopy.Spec.Replicas = func(i int32) *int32 { return &i }(0)
	if _, err := clientSet.CoreV1().ReplicationControllers(rcdeepcopy.Namespace).Update(ctx, rcdeepcopy, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("Error updating replica controller %v", err)
	}

	// wait 30 seconds until all pods are deleted
	if err := wait.PollImmediate(5*time.Second, 30*time.Second, func() (bool, error) {
		scale, err := clientSet.CoreV1().ReplicationControllers(rc.Namespace).GetScale(ctx, rc.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		return scale.Spec.Replicas == 0, nil
	}); err != nil {
		t.Fatalf("Error deleting rc pods %v", err)
	}

	if err := wait.PollImmediate(5*time.Second, time.Minute, func() (bool, error) {
		podList, _ := clientSet.CoreV1().Pods(rc.Namespace).List(ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(rc.Spec.Template.Labels).String()})
		t.Logf("Waiting for %v RC pods to disappear, still %v remaining", rc.Name, len(podList.Items))
		if len(podList.Items) > 0 {
			return false, nil
		}
		return true, nil
	}); err != nil {
		t.Fatalf("Error waiting for rc pods to disappear: %v", err)
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

var balancePodLabel = map[string]string{"podname": "priority-balanced-memory"}

// track min memory limit based on crio minimum. pods cannot set a limit lower than this
// see: https://github.com/cri-o/cri-o/blob/29805b13e9a43d9d22628553db337ce1c1bec0a8/internal/config/cgmgr/cgmgr.go#L23
// see: https://bugzilla.redhat.com/show_bug.cgi?id=1595256
var crioMinMemLimit = 12 * 1024 * 1024

var podRequestedResource = &v1.ResourceRequirements{
	Limits: v1.ResourceList{
		v1.ResourceMemory: resource.MustParse("100Mi"),
		v1.ResourceCPU:    resource.MustParse("100m"),
	},
	Requests: v1.ResourceList{
		v1.ResourceMemory: resource.MustParse("100Mi"),
		v1.ResourceCPU:    resource.MustParse("100m"),
	},
}

// createBalancedPodForNodes creates a pod per node that asks for enough resources to make all nodes have the same mem/cpu usage ratio.
// TODO(jchaloup): The function is updated version of what under https://github.com/kubernetes/kubernetes/blob/84483a5/test/e2e/scheduling/priorities.go#L478.
//                 Import it once the function is moved under k8s.io/components-helper repo and modified to work for both priority and predicates cases.
func createBalancedPodForNodes(
	t *testing.T,
	ctx context.Context,
	cs clientset.Interface,
	ns string,
	nodes []*v1.Node,
	ratio float64,
) (func(), error) {
	cleanUp := func() {
		// Delete all remaining pods
		err := cs.CoreV1().Pods(ns).DeleteCollection(context.TODO(), metav1.DeleteOptions{}, metav1.ListOptions{
			LabelSelector: labels.SelectorFromSet(labels.Set(balancePodLabel)).String(),
		})
		if err != nil {
			t.Logf("Failed to delete memory balanced pods: %v.", err)
		} else {
			err := wait.PollImmediate(2*time.Second, time.Minute, func() (bool, error) {
				podList, err := cs.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{
					LabelSelector: labels.SelectorFromSet(labels.Set(balancePodLabel)).String(),
				})
				if err != nil {
					t.Logf("Failed to list memory balanced pods: %v.", err)
					return false, nil
				}
				if len(podList.Items) > 0 {
					return false, nil
				}
				return true, nil
			})
			if err != nil {
				t.Logf("Failed to wait until all memory balanced pods are deleted: %v.", err)
			}
		}
	}

	// find the max, if the node has the max,use the one, if not,use the ratio parameter
	var maxCPUFraction, maxMemFraction float64 = ratio, ratio
	var cpuFractionMap = make(map[string]float64)
	var memFractionMap = make(map[string]float64)

	for _, node := range nodes {
		cpuFraction, memFraction, _, _ := computeCPUMemFraction(t, cs, node, podRequestedResource)
		cpuFractionMap[node.Name] = cpuFraction
		memFractionMap[node.Name] = memFraction
		if cpuFraction > maxCPUFraction {
			maxCPUFraction = cpuFraction
		}
		if memFraction > maxMemFraction {
			maxMemFraction = memFraction
		}
	}

	// we need the max one to keep the same cpu/mem use rate
	ratio = math.Max(maxCPUFraction, maxMemFraction)
	for _, node := range nodes {
		memAllocatable, found := node.Status.Allocatable[v1.ResourceMemory]
		if !found {
			t.Fatalf("Failed to get node's Allocatable[v1.ResourceMemory]")
		}
		memAllocatableVal := memAllocatable.Value()

		cpuAllocatable, found := node.Status.Allocatable[v1.ResourceCPU]
		if !found {
			t.Fatalf("Failed to get node's Allocatable[v1.ResourceCPU]")
		}
		cpuAllocatableMil := cpuAllocatable.MilliValue()

		needCreateResource := v1.ResourceList{}
		cpuFraction := cpuFractionMap[node.Name]
		memFraction := memFractionMap[node.Name]
		needCreateResource[v1.ResourceCPU] = *resource.NewMilliQuantity(int64((ratio-cpuFraction)*float64(cpuAllocatableMil)), resource.DecimalSI)

		// add crioMinMemLimit to ensure that all pods are setting at least that much for a limit, while keeping the same ratios
		needCreateResource[v1.ResourceMemory] = *resource.NewQuantity(int64((ratio-memFraction)*float64(memAllocatableVal)+float64(crioMinMemLimit)), resource.BinarySI)

		var gracePeriod = int64(1)
		// Don't set OwnerReferences to avoid pod eviction
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "filler-pod-" + string(uuid.NewUUID()),
				Namespace: ns,
				Labels:    balancePodLabel,
			},
			Spec: v1.PodSpec{
				Affinity: &v1.Affinity{
					NodeAffinity: &v1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
							NodeSelectorTerms: []v1.NodeSelectorTerm{
								{
									MatchFields: []v1.NodeSelectorRequirement{
										{Key: "metadata.name", Operator: v1.NodeSelectorOpIn, Values: []string{node.Name}},
									},
								},
							},
						},
					},
				},
				Containers: []v1.Container{
					{
						Name:  "pause",
						Image: "kubernetes/pause",
						Resources: v1.ResourceRequirements{
							Limits:   needCreateResource,
							Requests: needCreateResource,
						},
					},
				},
				// PriorityClassName:             conf.PriorityClassName,
				TerminationGracePeriodSeconds: &gracePeriod,
			},
		}

		t.Logf("Creating pod %v in %v namespace for node %v", pod.Name, pod.Namespace, node.Name)
		_, err := cs.CoreV1().Pods(pod.Namespace).Create(ctx, pod, metav1.CreateOptions{})
		if err != nil {
			t.Logf("Error creating filler pod: %v", err)
			return cleanUp, err
		}

		waitForPodRunning(ctx, t, cs, pod)
	}

	for _, node := range nodes {
		t.Log("Compute Cpu, Mem Fraction after create balanced pods.")
		computeCPUMemFraction(t, cs, node, podRequestedResource)
	}

	return cleanUp, nil
}

func computeCPUMemFraction(t *testing.T, cs clientset.Interface, node *v1.Node, resourceReq *v1.ResourceRequirements) (float64, float64, int64, int64) {
	t.Logf("ComputeCPUMemFraction for node: %v", node.Name)
	totalRequestedCPUResource := resourceReq.Requests.Cpu().MilliValue()
	totalRequestedMemResource := resourceReq.Requests.Memory().Value()
	allpods, err := cs.CoreV1().Pods(metav1.NamespaceAll).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("Expect error of invalid, got : %v", err)
	}
	for _, pod := range allpods.Items {
		if pod.Spec.NodeName == node.Name {
			req, _ := utils.PodRequestsAndLimits(&pod)
			if _, ok := req[v1.ResourceCPU]; !ok {
				req[v1.ResourceCPU] = *resource.NewMilliQuantity(0, resource.DecimalSI)
			}
			if _, ok := req[v1.ResourceMemory]; !ok {
				req[v1.ResourceMemory] = *resource.NewQuantity(0, resource.BinarySI)
			}

			cpuRequest := req[v1.ResourceCPU]
			memoryRequest := req[v1.ResourceMemory]

			t.Logf("Pod for on the node: %v, Cpu: %v, Mem: %v", pod.Name, (&cpuRequest).MilliValue(), (&memoryRequest).Value())
			// Ignore best effort pods while computing fractions as they won't be taken in account by scheduler.
			if v1qos.GetPodQOS(&pod) == v1.PodQOSBestEffort {
				continue
			}
			totalRequestedCPUResource += (&cpuRequest).MilliValue()
			totalRequestedMemResource += (&memoryRequest).Value()
		}
	}
	cpuAllocatable, found := node.Status.Allocatable[v1.ResourceCPU]
	if !found {
		t.Fatalf("Failed to get node's Allocatable[v1.ResourceCPU]")
	}
	cpuAllocatableMil := cpuAllocatable.MilliValue()

	floatOne := float64(1)
	cpuFraction := float64(totalRequestedCPUResource) / float64(cpuAllocatableMil)
	if cpuFraction > floatOne {
		cpuFraction = floatOne
	}
	memAllocatable, found := node.Status.Allocatable[v1.ResourceMemory]
	if !found {
		t.Fatalf("Failed to get node's Allocatable[v1.ResourceMemory]")
	}
	memAllocatableVal := memAllocatable.Value()
	memFraction := float64(totalRequestedMemResource) / float64(memAllocatableVal)
	if memFraction > floatOne {
		memFraction = floatOne
	}

	t.Logf("Node: %v, totalRequestedCPUResource: %v, cpuAllocatableMil: %v, cpuFraction: %v", node.Name, totalRequestedCPUResource, cpuAllocatableMil, cpuFraction)
	t.Logf("Node: %v, totalRequestedMemResource: %v, memAllocatableVal: %v, memFraction: %v", node.Name, totalRequestedMemResource, memAllocatableVal, memFraction)

	return cpuFraction, memFraction, cpuAllocatableMil, memAllocatableVal
}

func waitForPodRunning(ctx context.Context, t *testing.T, clientSet clientset.Interface, pod *v1.Pod) {
	if err := wait.PollImmediate(5*time.Second, 30*time.Second, func() (bool, error) {
		podItem, err := clientSet.CoreV1().Pods(pod.Namespace).Get(ctx, pod.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		if podItem.Status.Phase != v1.PodRunning {
			t.Logf("Pod %v not running yet, is %v instead", podItem.Name, podItem.Status.Phase)
			return false, nil
		}

		return true, nil
	}); err != nil {
		t.Fatalf("Error waiting for pod running: %v", err)
	}
}

func splitNodesAndWorkerNodes(nodes []v1.Node) ([]*v1.Node, []*v1.Node) {
	var allNodes []*v1.Node
	var workerNodes []*v1.Node
	for i := range nodes {
		node := nodes[i]
		allNodes = append(allNodes, &node)
		if _, exists := node.Labels["node-role.kubernetes.io/master"]; !exists {
			workerNodes = append(workerNodes, &node)
		}
	}
	return allNodes, workerNodes
}

func initPodEvictorOrFail(t *testing.T, clientSet clientset.Interface, nodes []*v1.Node) *evictions.PodEvictor {
	evictionPolicyGroupVersion, err := eutils.SupportEviction(clientSet)
	if err != nil || len(evictionPolicyGroupVersion) == 0 {
		t.Fatalf("Error creating eviction policy group: %v", err)
	}
	return evictions.NewPodEvictor(
		clientSet,
		evictionPolicyGroupVersion,
		false,
		0,
		nodes,
		true,
		false,
		false,
	)
}
