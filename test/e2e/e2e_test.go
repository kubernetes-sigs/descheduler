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
	"bufio"
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	listersv1 "k8s.io/client-go/listers/core/v1"
	componentbaseconfig "k8s.io/component-base/config"
	"k8s.io/component-base/featuregate"
	"k8s.io/klog/v2"
	utilptr "k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"

	"sigs.k8s.io/descheduler/cmd/descheduler/app/options"
	"sigs.k8s.io/descheduler/pkg/api"
	deschedulerapi "sigs.k8s.io/descheduler/pkg/api"
	deschedulerapiv1alpha2 "sigs.k8s.io/descheduler/pkg/api/v1alpha2"
	"sigs.k8s.io/descheduler/pkg/descheduler"
	"sigs.k8s.io/descheduler/pkg/descheduler/client"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	eutils "sigs.k8s.io/descheduler/pkg/descheduler/evictions/utils"
	nodeutil "sigs.k8s.io/descheduler/pkg/descheduler/node"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/features"
	"sigs.k8s.io/descheduler/pkg/framework/pluginregistry"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/nodeutilization"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/podlifetime"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodshavingtoomanyrestarts"
	frameworktesting "sigs.k8s.io/descheduler/pkg/framework/testing"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
	"sigs.k8s.io/descheduler/pkg/utils"
)

func isClientRateLimiterError(err error) bool {
	return strings.Contains(err.Error(), "client rate limiter")
}

func initFeatureGates() featuregate.FeatureGate {
	featureGates := featuregate.NewFeatureGate()
	featureGates.Add(map[featuregate.Feature]featuregate.FeatureSpec{
		features.EvictionsInBackground: {Default: false, PreRelease: featuregate.Alpha},
	})
	return featureGates
}

func deschedulerPolicyConfigMap(policy *deschedulerapiv1alpha2.DeschedulerPolicy) (*v1.ConfigMap, error) {
	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "descheduler-policy-configmap",
			Namespace: "kube-system",
		},
	}
	policy.APIVersion = "descheduler/v1alpha2"
	policy.Kind = "DeschedulerPolicy"
	policyBytes, err := yaml.Marshal(policy)
	if err != nil {
		return nil, err
	}
	cm.Data = map[string]string{"policy.yaml": string(policyBytes)}
	return cm, nil
}

func deschedulerDeployment(testName string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "descheduler",
			Namespace: "kube-system",
			Labels:    map[string]string{"app": "descheduler", "test": testName},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: utilptr.To[int32](1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "descheduler", "test": testName},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "descheduler", "test": testName},
				},
				Spec: v1.PodSpec{
					PriorityClassName:  "system-cluster-critical",
					ServiceAccountName: "descheduler-sa",
					SecurityContext: &v1.PodSecurityContext{
						RunAsNonRoot: utilptr.To(true),
						RunAsUser:    utilptr.To[int64](1000),
						RunAsGroup:   utilptr.To[int64](1000),
						SeccompProfile: &v1.SeccompProfile{
							Type: v1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []v1.Container{
						{
							Name:            "descheduler",
							Image:           os.Getenv("DESCHEDULER_IMAGE"),
							ImagePullPolicy: "IfNotPresent",
							Command:         []string{"/bin/descheduler"},
							Args:            []string{"--policy-config-file", "/policy-dir/policy.yaml", "--descheduling-interval", "100m", "--v", "4"},
							Ports:           []v1.ContainerPort{{ContainerPort: 10258, Protocol: "TCP"}},
							LivenessProbe: &v1.Probe{
								FailureThreshold: 3,
								ProbeHandler: v1.ProbeHandler{
									HTTPGet: &v1.HTTPGetAction{
										Path:   "/healthz",
										Port:   intstr.FromInt(10258),
										Scheme: v1.URISchemeHTTPS,
									},
								},
								InitialDelaySeconds: 3,
								PeriodSeconds:       10,
							},
							Resources: v1.ResourceRequirements{
								Requests: v1.ResourceList{
									v1.ResourceCPU:    resource.MustParse("500m"),
									v1.ResourceMemory: resource.MustParse("256Mi"),
								},
							},
							SecurityContext: &v1.SecurityContext{
								AllowPrivilegeEscalation: utilptr.To(false),
								Capabilities: &v1.Capabilities{
									Drop: []v1.Capability{
										"ALL",
									},
								},
								Privileged:             utilptr.To[bool](false),
								ReadOnlyRootFilesystem: utilptr.To[bool](true),
								RunAsNonRoot:           utilptr.To[bool](true),
							},
							VolumeMounts: []v1.VolumeMount{
								{
									MountPath: "/policy-dir",
									Name:      "policy-volume",
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "policy-volume",
							VolumeSource: v1.VolumeSource{
								ConfigMap: &v1.ConfigMapVolumeSource{
									LocalObjectReference: v1.LocalObjectReference{
										Name: "descheduler-policy-configmap",
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func printPodLogs(ctx context.Context, t *testing.T, kubeClient clientset.Interface, podName string) {
	request := kubeClient.CoreV1().Pods("kube-system").GetLogs(podName, &v1.PodLogOptions{})
	readCloser, err := request.Stream(context.TODO())
	if err != nil {
		t.Logf("Unable to request stream: %v\n", err)
		return
	}

	defer readCloser.Close()
	scanner := bufio.NewScanner(readCloser)
	for scanner.Scan() {
		fmt.Println(string(scanner.Bytes()))
	}
	if err := scanner.Err(); err != nil {
		t.Logf("Unable to scan bytes: %v\n", err)
	}
}

func TestMain(m *testing.M) {
	if os.Getenv("DESCHEDULER_IMAGE") == "" {
		klog.Errorf("DESCHEDULER_IMAGE env is not set")
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func initPluginRegistry() {
	pluginregistry.PluginRegistry = pluginregistry.NewRegistry()
	pluginregistry.Register(defaultevictor.PluginName, defaultevictor.New, &defaultevictor.DefaultEvictor{}, &defaultevictor.DefaultEvictorArgs{}, defaultevictor.ValidateDefaultEvictorArgs, defaultevictor.SetDefaults_DefaultEvictorArgs, pluginregistry.PluginRegistry)
	pluginregistry.Register(podlifetime.PluginName, podlifetime.New, &podlifetime.PodLifeTime{}, &podlifetime.PodLifeTimeArgs{}, podlifetime.ValidatePodLifeTimeArgs, podlifetime.SetDefaults_PodLifeTimeArgs, pluginregistry.PluginRegistry)
	pluginregistry.Register(removepodshavingtoomanyrestarts.PluginName, removepodshavingtoomanyrestarts.New, &removepodshavingtoomanyrestarts.RemovePodsHavingTooManyRestarts{}, &removepodshavingtoomanyrestarts.RemovePodsHavingTooManyRestartsArgs{}, removepodshavingtoomanyrestarts.ValidateRemovePodsHavingTooManyRestartsArgs, removepodshavingtoomanyrestarts.SetDefaults_RemovePodsHavingTooManyRestartsArgs, pluginregistry.PluginRegistry)
}

// RcByNameContainer returns a ReplicationController with specified name and container
func RcByNameContainer(name, namespace string, replicas int32, labels map[string]string, gracePeriod *int64, priorityClassName string) *v1.ReplicationController {
	// Add "name": name to the labels, overwriting if it exists.
	labels["name"] = name
	if gracePeriod == nil {
		gracePeriod = utilptr.To[int64](0)
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
			Replicas: utilptr.To[int32](replicas),
			Selector: map[string]string{
				"name": name,
			},
			Template: &v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: makePodSpec(priorityClassName, gracePeriod),
			},
		},
	}
}

// DsByNameContainer returns a DaemonSet with specified name and container
func DsByNameContainer(name, namespace string, labels map[string]string, gracePeriod *int64) *appsv1.DaemonSet {
	// Add "name": name to the labels, overwriting if it exists.
	labels["name"] = name
	if gracePeriod == nil {
		gracePeriod = utilptr.To[int64](0)
	}
	return &appsv1.DaemonSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DaemonSet",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": name,
				},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: makePodSpec("", gracePeriod),
			},
		},
	}
}

func buildTestDeployment(name, namespace string, replicas int32, testLabel map[string]string, apply func(deployment *appsv1.Deployment)) *appsv1.Deployment {
	deployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    testLabel,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: utilptr.To[int32](replicas),
			Selector: &metav1.LabelSelector{
				MatchLabels: testLabel,
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: testLabel,
				},
				Spec: makePodSpec("", utilptr.To[int64](0)),
			},
		},
	}

	if apply != nil {
		apply(deployment)
	}

	return deployment
}

func makePodSpec(priorityClassName string, gracePeriod *int64) v1.PodSpec {
	return v1.PodSpec{
		SecurityContext: &v1.PodSecurityContext{
			RunAsNonRoot: utilptr.To(true),
			RunAsUser:    utilptr.To[int64](1000),
			RunAsGroup:   utilptr.To[int64](1000),
			SeccompProfile: &v1.SeccompProfile{
				Type: v1.SeccompProfileTypeRuntimeDefault,
			},
		},
		Containers: []v1.Container{{
			Name:            "pause",
			ImagePullPolicy: "IfNotPresent",
			Image:           "registry.k8s.io/pause",
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
			SecurityContext: &v1.SecurityContext{
				AllowPrivilegeEscalation: utilptr.To(false),
				Capabilities: &v1.Capabilities{
					Drop: []v1.Capability{
						"ALL",
					},
				},
			},
		}},
		PriorityClassName:             priorityClassName,
		TerminationGracePeriodSeconds: gracePeriod,
	}
}

func initializeClient(ctx context.Context, t *testing.T) (clientset.Interface, informers.SharedInformerFactory, listersv1.NodeLister, podutil.GetPodsAssignedToNodeFunc) {
	clientSet, err := client.CreateClient(componentbaseconfig.ClientConnectionConfiguration{Kubeconfig: os.Getenv("KUBECONFIG")}, "")
	if err != nil {
		t.Errorf("Error during client creation with %v", err)
	}

	sharedInformerFactory := informers.NewSharedInformerFactory(clientSet, 0)
	nodeLister := sharedInformerFactory.Core().V1().Nodes().Lister()
	podInformer := sharedInformerFactory.Core().V1().Pods().Informer()

	getPodsAssignedToNode, err := podutil.BuildGetPodsAssignedToNodeFunc(podInformer)
	if err != nil {
		t.Errorf("build get pods assigned to node function error: %v", err)
	}

	sharedInformerFactory.Start(ctx.Done())
	sharedInformerFactory.WaitForCacheSync(ctx.Done())

	waitForNodesReady(ctx, t, clientSet, nodeLister)
	return clientSet, sharedInformerFactory, nodeLister, getPodsAssignedToNode
}

func waitForNodesReady(ctx context.Context, t *testing.T, clientSet clientset.Interface, nodeLister listersv1.NodeLister) {
	if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
		nodeList, err := clientSet.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err != nil {
			return false, err
		}

		readyNodes, err := nodeutil.ReadyNodes(ctx, clientSet, nodeLister, "")
		if err != nil {
			return false, err
		}
		if len(nodeList.Items) != len(readyNodes) {
			t.Logf("%v/%v nodes are ready. Waiting for all nodes to be ready...", len(readyNodes), len(nodeList.Items))
			return false, nil
		}

		return true, nil
	}); err != nil {
		t.Fatalf("Error waiting for nodes to be ready: %v", err)
	}
}

func runPodLifetimePlugin(
	ctx context.Context,
	t *testing.T,
	clientset clientset.Interface,
	nodeLister listersv1.NodeLister,
	namespaces *deschedulerapi.Namespaces,
	priorityClass string,
	priority *int32,
	evictCritical bool,
	evictDaemonSet bool,
	maxPodsToEvictPerNamespace *uint,
	labelSelector *metav1.LabelSelector,
) {
	evictionPolicyGroupVersion, err := eutils.SupportEviction(clientset)
	if err != nil || len(evictionPolicyGroupVersion) == 0 {
		t.Fatalf("%v", err)
	}

	nodes, err := nodeutil.ReadyNodes(ctx, clientset, nodeLister, "")
	if err != nil {
		t.Fatalf("%v", err)
	}

	var thresholdPriority int32
	if priority != nil {
		thresholdPriority = *priority
	} else {
		thresholdPriority, err = utils.GetPriorityFromPriorityClass(ctx, clientset, priorityClass)
		if err != nil {
			t.Fatalf("Failed to get threshold priority from plugin arg params")
		}
	}

	handle, _, err := frameworktesting.InitFrameworkHandle(
		ctx,
		clientset,
		evictions.NewOptions().
			WithPolicyGroupVersion(evictionPolicyGroupVersion).
			WithMaxPodsToEvictPerNamespace(maxPodsToEvictPerNamespace),
		defaultevictor.DefaultEvictorArgs{
			EvictSystemCriticalPods: evictCritical,
			EvictDaemonSetPods:      evictDaemonSet,
			PriorityThreshold: &api.PriorityThreshold{
				Value: &thresholdPriority,
			},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("Unable to initialize a framework handle: %v", err)
	}

	maxPodLifeTimeSeconds := uint(1)

	plugin, err := podlifetime.New(ctx, &podlifetime.PodLifeTimeArgs{
		MaxPodLifeTimeSeconds: &maxPodLifeTimeSeconds,
		LabelSelector:         labelSelector,
		Namespaces:            namespaces,
	}, handle)
	if err != nil {
		t.Fatalf("Unable to initialize the plugin: %v", err)
	}
	t.Log("Running podlifetime plugin")
	plugin.(frameworktypes.DeschedulePlugin).Deschedule(ctx, nodes)
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

	clientSet, _, _, getPodsAssignedToNode := initializeClient(ctx, t)

	nodeList, err := clientSet.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Errorf("Error listing node with %v", err)
	}

	_, workerNodes := splitNodesAndWorkerNodes(nodeList.Items)

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
				SecurityContext: &v1.PodSecurityContext{
					RunAsNonRoot: utilptr.To(true),
					RunAsUser:    utilptr.To[int64](1000),
					RunAsGroup:   utilptr.To[int64](1000),
					SeccompProfile: &v1.SeccompProfile{
						Type: v1.SeccompProfileTypeRuntimeDefault,
					},
				},
				Containers: []v1.Container{{
					Name:            "pause",
					ImagePullPolicy: "Never",
					Image:           "registry.k8s.io/pause",
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

	evictionPolicyGroupVersion, err := eutils.SupportEviction(clientSet)
	if err != nil || len(evictionPolicyGroupVersion) == 0 {
		t.Fatalf("Error detecting eviction policy group: %v", err)
	}

	handle, _, err := frameworktesting.InitFrameworkHandle(
		ctx,
		clientSet,
		evictions.NewOptions().
			WithPolicyGroupVersion(evictionPolicyGroupVersion),
		defaultevictor.DefaultEvictorArgs{
			EvictLocalStoragePods: true,
		},
		nil,
	)
	if err != nil {
		t.Fatalf("Unable to initialize a framework handle: %v", err)
	}

	podFilter, err := podutil.NewOptions().WithFilter(handle.EvictorFilterImpl.Filter).BuildFilterFunc()
	if err != nil {
		t.Errorf("Error initializing pod filter function, %v", err)
	}

	podsOnMosttUtilizedNode, err := podutil.ListPodsOnANode(workerNodes[0].Name, getPodsAssignedToNode, podFilter)
	if err != nil {
		t.Errorf("Error listing pods on a node %v", err)
	}
	podsBefore := len(podsOnMosttUtilizedNode)

	t.Log("Running LowNodeUtilization plugin")
	plugin, err := nodeutilization.NewLowNodeUtilization(ctx, &nodeutilization.LowNodeUtilizationArgs{
		Thresholds: api.ResourceThresholds{
			v1.ResourceCPU: 70,
		},
		TargetThresholds: api.ResourceThresholds{
			v1.ResourceCPU: 80,
		},
	}, handle)
	if err != nil {
		t.Fatalf("Unable to initialize the plugin: %v", err)
	}
	plugin.(frameworktypes.BalancePlugin).Balance(ctx, workerNodes)

	waitForTerminatingPodsToDisappear(ctx, t, clientSet, rc.Namespace)

	podFilter, err = podutil.NewOptions().WithFilter(handle.EvictorFilterImpl.Filter).BuildFilterFunc()
	if err != nil {
		t.Errorf("Error initializing pod filter function, %v", err)
	}

	podsOnMosttUtilizedNode, err = podutil.ListPodsOnANode(workerNodes[0].Name, getPodsAssignedToNode, podFilter)
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

	clientSet, _, nodeInformer, _ := initializeClient(ctx, t)

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

	t.Logf("run the plugin to delete pods from %v namespace", rc.Namespace)
	runPodLifetimePlugin(ctx, t, clientSet, nodeInformer, &deschedulerapi.Namespaces{
		Include: []string{rc.Namespace},
	}, "", nil, false, false, nil, nil)

	// All pods are supposed to be deleted, wait until all the old pods are deleted
	if err := wait.PollUntilContextTimeout(ctx, time.Second, 20*time.Second, true, func(ctx context.Context) (bool, error) {
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

	clientSet, _, nodeInformer, _ := initializeClient(ctx, t)

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

	t.Logf("run the plugin to delete pods from namespaces except the %v namespace", rc.Namespace)
	runPodLifetimePlugin(ctx, t, clientSet, nodeInformer, &deschedulerapi.Namespaces{
		Exclude: []string{rc.Namespace},
	}, "", nil, false, false, nil, nil)

	t.Logf("Waiting 10s")
	time.Sleep(10 * time.Second)
	podList, err = clientSet.CoreV1().Pods(rc.Namespace).List(ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(rc.Spec.Template.Labels).String()})
	if err != nil {
		t.Fatalf("Unable to list pods after running PodLifeTime plugin: %v", err)
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
	highPriority := int32(1000)
	lowPriority := int32(500)
	ctx := context.Background()

	clientSet, _, nodeInformer, _ := initializeClient(ctx, t)

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
		runPodLifetimePlugin(ctx, t, clientSet, nodeInformer, nil, highPriorityClass.Name, nil, true, false, nil, nil)
	} else {
		runPodLifetimePlugin(ctx, t, clientSet, nodeInformer, nil, "", &highPriority, true, false, nil, nil)
	}

	// All pods are supposed to be deleted, wait until all pods in the test namespace are terminating
	t.Logf("All pods in the test namespace, no matter their priority (including system-node-critical and system-cluster-critical), will be deleted")
	if err := wait.PollUntilContextTimeout(ctx, time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
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

func TestEvictDaemonSetPod(t *testing.T) {
	testEvictDaemonSetPod(t, true)
}

func testEvictDaemonSetPod(t *testing.T, isDaemonSet bool) {
	ctx := context.Background()

	clientSet, _, nodeInformer, _ := initializeClient(ctx, t)

	testNamespace := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "e2e-" + strings.ToLower(t.Name())}}
	if _, err := clientSet.CoreV1().Namespaces().Create(ctx, testNamespace, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Unable to create ns %v", testNamespace.Name)
	}
	defer clientSet.CoreV1().Namespaces().Delete(ctx, testNamespace.Name, metav1.DeleteOptions{})

	daemonSet := DsByNameContainer("test-ds-evictdaemonsetpods", testNamespace.Name,
		map[string]string{"test": "evictdaemonsetpods"}, nil)
	if _, err := clientSet.AppsV1().DaemonSets(daemonSet.Namespace).Create(ctx, daemonSet, metav1.CreateOptions{}); err != nil {
		t.Errorf("Error creating ds %s: %v", daemonSet.Name, err)
	}
	defer deleteDS(ctx, t, clientSet, daemonSet)

	// wait for a while so all the pods are at least few seconds older
	time.Sleep(5 * time.Second)

	podListDaemonSet, err := clientSet.CoreV1().Pods(daemonSet.Namespace).List(ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(daemonSet.Spec.Template.Labels).String()})
	if err != nil {
		t.Fatalf("Unable to list pods: %v", err)
	}

	initialPodNames := getPodNames(podListDaemonSet.Items)
	sort.Strings(initialPodNames)
	t.Logf("Existing pods: %v", initialPodNames)

	runPodLifetimePlugin(ctx, t, clientSet, nodeInformer, nil, "", nil, false, isDaemonSet, nil, nil)

	// All pods are supposed to be deleted, wait until all pods in the test namespace are terminating
	t.Logf("All daemonset pods in the test namespace, will be deleted")
	if err := wait.PollUntilContextTimeout(ctx, time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
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
	highPriority := int32(1000)
	lowPriority := int32(500)
	ctx := context.Background()

	clientSet, _, nodeInformer, _ := initializeClient(ctx, t)

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
		t.Logf("run the plugin to delete pods with priority lower than priority class %s", highPriorityClass.Name)
		runPodLifetimePlugin(ctx, t, clientSet, nodeInformer, nil, highPriorityClass.Name, nil, false, false, nil, nil)
	} else {
		t.Logf("run the plugin to delete pods with priority lower than %d", highPriority)
		runPodLifetimePlugin(ctx, t, clientSet, nodeInformer, nil, "", &highPriority, false, false, nil, nil)
	}

	t.Logf("Waiting 10s")
	time.Sleep(10 * time.Second)
	// check if all pods with high priority class are not evicted
	podListHighPriority, err = clientSet.CoreV1().Pods(rcHighPriority.Namespace).List(
		ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(rcHighPriority.Spec.Template.Labels).String()})
	if err != nil {
		t.Fatalf("Unable to list pods after running plugin: %v", err)
	}

	excludePodNames := getPodNames(podListHighPriority.Items)
	sort.Strings(excludePodNames)
	t.Logf("Existing high priority pods: %v", excludePodNames)

	// validate no pods were deleted
	if len(intersectStrings(expectReservePodNames, excludePodNames)) != 5 {
		t.Fatalf("None of %v high priority pods are expected to be deleted", expectReservePodNames)
	}

	// check if all pods with low priority class are evicted
	if err := wait.PollUntilContextTimeout(ctx, time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
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

	clientSet, _, nodeInformer, _ := initializeClient(ctx, t)

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

	t.Logf("run the plugin to delete pods with label test:podlifetime-evict")
	runPodLifetimePlugin(ctx, t, clientSet, nodeInformer, nil, "", nil, false, false, nil, &metav1.LabelSelector{MatchLabels: map[string]string{"test": "podlifetime-evict"}})

	t.Logf("Waiting 10s")
	time.Sleep(10 * time.Second)
	// check if all pods without target label are not evicted
	podListReserve, err = clientSet.CoreV1().Pods(rcReserve.Namespace).List(
		ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(rcReserve.Spec.Template.Labels).String()})
	if err != nil {
		t.Fatalf("Unable to list pods after running plugin: %v", err)
	}

	reservedPodNames := getPodNames(podListReserve.Items)
	sort.Strings(reservedPodNames)
	t.Logf("Existing reserved pods: %v", reservedPodNames)

	// validate no pods were deleted
	if len(intersectStrings(expectReservePodNames, reservedPodNames)) != 5 {
		t.Fatalf("None of %v unevictable pods are expected to be deleted", expectReservePodNames)
	}

	// check if all selected pods are evicted
	if err := wait.PollUntilContextTimeout(ctx, time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
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

	clientSet, _, nodeLister, _ := initializeClient(ctx, t)

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
					SizeLimit: resource.NewQuantity(int64(10), resource.BinarySI),
				},
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

	t.Log("Running PodLifetime plugin")
	runPodLifetimePlugin(ctx, t, clientSet, nodeLister, nil, "", nil, false, false, nil, nil)

	if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, time.Minute, true, func(ctx context.Context) (bool, error) {
		podList, err = clientSet.CoreV1().Pods(rc.Namespace).List(ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(rc.Spec.Template.Labels).String()})
		if err != nil {
			return false, fmt.Errorf("unable to list pods after running plugin: %v", err)
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

func TestPodLifeTimeOldestEvicted(t *testing.T) {
	ctx := context.Background()

	clientSet, _, nodeLister, _ := initializeClient(ctx, t)

	testNamespace := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "e2e-" + strings.ToLower(t.Name())}}
	if _, err := clientSet.CoreV1().Namespaces().Create(ctx, testNamespace, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Unable to create ns %v", testNamespace.Name)
	}
	defer clientSet.CoreV1().Namespaces().Delete(ctx, testNamespace.Name, metav1.DeleteOptions{})

	t.Log("Create RC with 1 pod for testing oldest pod getting evicted")
	rc := RcByNameContainer("test-rc-pod-lifetime-oldest-evicted", testNamespace.Name, int32(1), map[string]string{"test": "oldest"}, nil, "")
	if _, err := clientSet.CoreV1().ReplicationControllers(rc.Namespace).Create(ctx, rc, metav1.CreateOptions{}); err != nil {
		t.Errorf("Error creating deployment %v", err)
	}
	defer deleteRC(ctx, t, clientSet, rc)

	waitForRCPodsRunning(ctx, t, clientSet, rc)

	podList, err := clientSet.CoreV1().Pods(rc.Namespace).List(ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(rc.Spec.Template.Labels).String()})
	if err != nil {
		t.Fatalf("Unable to list pods: %v", err)
	}
	oldestPod := podList.Items[0]

	t.Log("Scale the rs to 5 replicas with the 4 new pods having a more recent creation timestamp")
	rc.Spec.Replicas = utilptr.To[int32](5)
	rc, err = clientSet.CoreV1().ReplicationControllers(rc.Namespace).Update(ctx, rc, metav1.UpdateOptions{})
	if err != nil {
		t.Errorf("Error updating deployment %v", err)
	}
	waitForRCPodsRunning(ctx, t, clientSet, rc)

	podList, err = clientSet.CoreV1().Pods(rc.Namespace).List(ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(rc.Spec.Template.Labels).String()})
	if err != nil {
		t.Fatalf("Unable to list pods: %v", err)
	}

	t.Log("Running PodLifetime plugin with maxPodsToEvictPerNamespace=1 to ensure only the oldest pod is evicted")
	var maxPodsToEvictPerNamespace uint = 1
	runPodLifetimePlugin(ctx, t, clientSet, nodeLister, nil, "", nil, false, false, &maxPodsToEvictPerNamespace, nil)
	t.Log("Finished PodLifetime plugin")

	t.Logf("Wait for terminating pod to disappear")
	waitForTerminatingPodsToDisappear(ctx, t, clientSet, rc.Namespace)

	podList, err = clientSet.CoreV1().Pods(rc.Namespace).List(ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(rc.Spec.Template.Labels).String()})
	if err != nil {
		t.Fatalf("Unable to list pods: %v", err)
	}

	for _, pod := range podList.Items {
		if pod.GetName() == oldestPod.GetName() {
			t.Errorf("The oldest Pod %s was not evicted", oldestPod.GetName())
		}
	}
}

func TestDeschedulingInterval(t *testing.T) {
	ctx := context.Background()
	clientSet, err := client.CreateClient(componentbaseconfig.ClientConnectionConfiguration{Kubeconfig: os.Getenv("KUBECONFIG")}, "")
	if err != nil {
		t.Errorf("Error during client creation with %v", err)
	}

	// By default, the DeschedulingInterval param should be set to 0, meaning Descheduler only runs once then exits
	s, err := options.NewDeschedulerServer()
	if err != nil {
		t.Fatalf("Unable to initialize server: %v", err)
	}
	s.Client = clientSet
	s.DefaultFeatureGates = initFeatureGates()

	deschedulerPolicy := &deschedulerapi.DeschedulerPolicy{}

	c := make(chan bool, 1)
	go func() {
		evictionPolicyGroupVersion, err := eutils.SupportEviction(s.Client)
		if err != nil || len(evictionPolicyGroupVersion) == 0 {
			t.Errorf("Error when checking support for eviction: %v", err)
		}
		if err := descheduler.RunDeschedulerStrategies(ctx, s, deschedulerPolicy, evictionPolicyGroupVersion); err != nil {
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
	if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
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
	if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
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

func deleteDS(ctx context.Context, t *testing.T, clientSet clientset.Interface, ds *appsv1.DaemonSet) {
	// adds nodeselector to avoid any nodes by setting an unused label
	dsDeepCopy := ds.DeepCopy()
	dsDeepCopy.Spec.Template.Spec.NodeSelector = map[string]string{
		"avoid-all-nodes": "true",
	}

	if _, err := clientSet.AppsV1().DaemonSets(dsDeepCopy.Namespace).Update(ctx, dsDeepCopy, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("Error updating daemonset %v", err)
	}

	if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, time.Minute, true, func(ctx context.Context) (bool, error) {
		podList, _ := clientSet.CoreV1().Pods(ds.Namespace).List(ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(ds.Spec.Template.Labels).String()})
		t.Logf("Waiting for %v DS pods to disappear, still %v remaining", ds.Name, len(podList.Items))
		if len(podList.Items) > 0 {
			return false, nil
		}
		return true, nil
	}); err != nil {
		t.Fatalf("Error waiting for ds pods to disappear: %v", err)
	}

	if err := clientSet.AppsV1().DaemonSets(ds.Namespace).Delete(ctx, ds.Name, metav1.DeleteOptions{}); err != nil {
		t.Fatalf("Error deleting ds %v", err)
	}

	if err := wait.PollUntilContextTimeout(ctx, time.Second, 10*time.Second, true, func(ctx context.Context) (bool, error) {
		_, err := clientSet.AppsV1().DaemonSets(ds.Namespace).Get(ctx, ds.Name, metav1.GetOptions{})
		if err != nil && strings.Contains(err.Error(), "not found") {
			return true, nil
		}
		return false, nil
	}); err != nil {
		t.Fatalf("Error deleting ds %v", err)
	}
}

func deleteRC(ctx context.Context, t *testing.T, clientSet clientset.Interface, rc *v1.ReplicationController) {
	// set number of replicas to 0
	rcdeepcopy := rc.DeepCopy()
	rcdeepcopy.Spec.Replicas = utilptr.To[int32](0)
	if _, err := clientSet.CoreV1().ReplicationControllers(rcdeepcopy.Namespace).Update(ctx, rcdeepcopy, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("Error updating replica controller %v", err)
	}

	// wait 30 seconds until all pods are deleted
	if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
		scale, err := clientSet.CoreV1().ReplicationControllers(rc.Namespace).GetScale(ctx, rc.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		return scale.Spec.Replicas == 0, nil
	}); err != nil {
		t.Fatalf("Error deleting rc pods %v", err)
	}

	if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, time.Minute, true, func(ctx context.Context) (bool, error) {
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

	if err := wait.PollUntilContextTimeout(ctx, time.Second, 10*time.Second, true, func(ctx context.Context) (bool, error) {
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
//
//	Import it once the function is moved under k8s.io/components-helper repo and modified to work for both priority and predicates cases.
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
			err := wait.PollUntilContextTimeout(ctx, 2*time.Second, time.Minute, true, func(ctx context.Context) (bool, error) {
				podList, err := cs.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
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
	cpuFractionMap := make(map[string]float64)
	memFractionMap := make(map[string]float64)

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

		gracePeriod := int64(1)
		// Don't set OwnerReferences to avoid pod eviction
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "filler-pod-" + string(uuid.NewUUID()),
				Namespace: ns,
				Labels:    balancePodLabel,
			},
			Spec: v1.PodSpec{
				SecurityContext: &v1.PodSecurityContext{
					RunAsNonRoot: utilptr.To(true),
					RunAsUser:    utilptr.To[int64](1000),
					RunAsGroup:   utilptr.To[int64](1000),
					SeccompProfile: &v1.SeccompProfile{
						Type: v1.SeccompProfileTypeRuntimeDefault,
					},
				},
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
						Image: "registry.k8s.io/pause",
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
			// Ignore best effort pods while computing fractions as they won't be taken in account by scheduler.
			if utils.GetPodQOS(&pod) == v1.PodQOSBestEffort {
				t.Logf("Ignoring best-effort pod %s on node %s", pod.Name, pod.Spec.NodeName)
				continue
			}
			if _, ok := req[v1.ResourceCPU]; !ok {
				req[v1.ResourceCPU] = *resource.NewMilliQuantity(0, resource.DecimalSI)
			}
			if _, ok := req[v1.ResourceMemory]; !ok {
				req[v1.ResourceMemory] = *resource.NewQuantity(0, resource.BinarySI)
			}

			cpuRequest := req[v1.ResourceCPU]
			memoryRequest := req[v1.ResourceMemory]

			t.Logf("Pod %s on node %s, Cpu: %v, Mem: %v", pod.Name, pod.Spec.NodeName, (&cpuRequest).MilliValue(), (&memoryRequest).Value())
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
	if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
		podItem, err := clientSet.CoreV1().Pods(pod.Namespace).Get(ctx, pod.Name, metav1.GetOptions{})
		if err != nil {
			t.Logf("Unable to list pods: %v", err)
			if isClientRateLimiterError(err) {
				return false, nil
			}
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

func waitForPodsRunning(ctx context.Context, t *testing.T, clientSet clientset.Interface, labelMap map[string]string, desiredRunningPodNum int, namespace string) []*v1.Pod {
	desiredRunningPods := make([]*v1.Pod, desiredRunningPodNum)
	if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 60*time.Second, true, func(ctx context.Context) (bool, error) {
		podList, err := clientSet.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: labels.SelectorFromSet(labelMap).String(),
		})
		if err != nil {
			t.Logf("Unable to list pods: %v", err)
			if isClientRateLimiterError(err) {
				return false, nil
			}
			return false, err
		}
		runningPods := []*v1.Pod{}
		for _, item := range podList.Items {
			if item.Status.Phase != v1.PodRunning {
				continue
			}
			pod := item
			runningPods = append(runningPods, &pod)
		}

		if len(runningPods) != desiredRunningPodNum {
			t.Logf("Waiting for %v pods to be running, got %v instead", desiredRunningPodNum, len(runningPods))
			return false, nil
		}
		desiredRunningPods = runningPods

		return true, nil
	}); err != nil {
		t.Fatalf("Error waiting for pods running: %v", err)
	}
	return desiredRunningPods
}

func waitForPodsToDisappear(ctx context.Context, t *testing.T, clientSet clientset.Interface, labelMap map[string]string, namespace string) {
	if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 60*time.Second, true, func(ctx context.Context) (bool, error) {
		podList, err := clientSet.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: labels.SelectorFromSet(labelMap).String(),
		})
		if err != nil {
			t.Logf("Unable to list pods: %v", err)
			if isClientRateLimiterError(err) {
				return false, nil
			}
			return false, err
		}

		if len(podList.Items) > 0 {
			t.Logf("Found a existing pod. Waiting until it gets deleted")
			return false, nil
		}
		return true, nil
	}); err != nil {
		t.Fatalf("Error waiting for pods to disappear: %v", err)
	}
}

func splitNodesAndWorkerNodes(nodes []v1.Node) ([]*v1.Node, []*v1.Node) {
	var allNodes []*v1.Node
	var workerNodes []*v1.Node
	for i := range nodes {
		node := nodes[i]
		allNodes = append(allNodes, &node)
		if _, exists := node.Labels["node-role.kubernetes.io/control-plane"]; !exists {
			workerNodes = append(workerNodes, &node)
		}
	}
	return allNodes, workerNodes
}

func getCurrentPodNames(ctx context.Context, clientSet clientset.Interface, namespace string, t *testing.T) []string {
	podList, err := clientSet.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Logf("Unable to list pods: %v", err)
		return nil
	}

	names := []string{}
	for _, item := range podList.Items {
		names = append(names, item.Name)
	}
	return names
}
