/*
Copyright 2014 The Kubernetes Authors.

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
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/apimachinery/pkg/util/sets"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	fakeV1 "k8s.io/client-go/kubernetes/typed/core/v1/fake"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	apitesting "k8s.io/kubernetes/pkg/api/testing"
	kubefeatures "k8s.io/kubernetes/pkg/features"
	"k8s.io/kubernetes/pkg/scheduler/algorithm"
	"k8s.io/kubernetes/pkg/scheduler/algorithm/predicates"
	schedulerapi "k8s.io/kubernetes/pkg/scheduler/apis/config"
	"k8s.io/kubernetes/pkg/scheduler/apis/config/scheme"
	extenderv1 "k8s.io/kubernetes/pkg/scheduler/apis/extender/v1"
	frameworkplugins "k8s.io/kubernetes/pkg/scheduler/framework/plugins"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/nodelabel"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/serviceaffinity"
	framework "k8s.io/kubernetes/pkg/scheduler/framework/v1alpha1"
	internalcache "k8s.io/kubernetes/pkg/scheduler/internal/cache"
	internalqueue "k8s.io/kubernetes/pkg/scheduler/internal/queue"
	schedulernodeinfo "k8s.io/kubernetes/pkg/scheduler/nodeinfo"
	nodeinfosnapshot "k8s.io/kubernetes/pkg/scheduler/nodeinfo/snapshot"
)

const (
	disablePodPreemption             = false
	bindTimeoutSeconds               = 600
	podInitialBackoffDurationSeconds = 1
	podMaxBackoffDurationSeconds     = 10
)

func TestCreate(t *testing.T) {
	client := fake.NewSimpleClientset()
	stopCh := make(chan struct{})
	defer close(stopCh)
	factory := newConfigFactory(client, v1.DefaultHardPodAffinitySymmetricWeight, stopCh)
	factory.Create()
}

// Test configures a scheduler from a policies defined in a file
// It combines some configurable predicate/priorities with some pre-defined ones
func TestCreateFromConfig(t *testing.T) {
	var configData []byte
	var policy schedulerapi.Policy

	client := fake.NewSimpleClientset()
	stopCh := make(chan struct{})
	defer close(stopCh)
	factory := newConfigFactory(client, v1.DefaultHardPodAffinitySymmetricWeight, stopCh)

	// Pre-register some predicate and priority functions
	RegisterFitPredicate("PredicateOne", PredicateFunc)
	RegisterFitPredicate("PredicateTwo", PredicateFunc)
	RegisterPriorityMapReduceFunction("PriorityOne", PriorityFunc, nil, 1)
	RegisterPriorityMapReduceFunction("PriorityTwo", PriorityFunc, nil, 1)

	configData = []byte(`{
		"kind" : "Policy",
		"apiVersion" : "v1",
		"predicates" : [
			{"name" : "TestZoneAffinity", "argument" : {"serviceAffinity" : {"labels" : ["zone"]}}},
			{"name" : "TestZoneAffinity", "argument" : {"serviceAffinity" : {"labels" : ["foo"]}}},
			{"name" : "TestRequireZone", "argument" : {"labelsPresence" : {"labels" : ["zone"], "presence" : true}}},
			{"name" : "TestNoFooLabel", "argument" : {"labelsPresence" : {"labels" : ["foo"], "presence" : false}}},
			{"name" : "PredicateOne"},
			{"name" : "PredicateTwo"}
		],
		"priorities" : [
			{"name" : "RackSpread", "weight" : 3, "argument" : {"serviceAntiAffinity" : {"label" : "rack"}}},
			{"name" : "ZoneSpread", "weight" : 3, "argument" : {"serviceAntiAffinity" : {"label" : "zone"}}},
			{"name" : "LabelPreference1", "weight" : 3, "argument" : {"labelPreference" : {"label" : "l1", "presence": true}}},
			{"name" : "LabelPreference2", "weight" : 3, "argument" : {"labelPreference" : {"label" : "l2", "presence": false}}},
			{"name" : "PriorityOne", "weight" : 2},
			{"name" : "PriorityTwo", "weight" : 1}		]
	}`)
	if err := runtime.DecodeInto(scheme.Codecs.UniversalDecoder(), configData, &policy); err != nil {
		t.Errorf("Invalid configuration: %v", err)
	}

	sched, err := factory.CreateFromConfig(policy)
	if err != nil {
		t.Fatalf("CreateFromConfig failed: %v", err)
	}
	hpa := factory.GetHardPodAffinitySymmetricWeight()
	if hpa != v1.DefaultHardPodAffinitySymmetricWeight {
		t.Errorf("Wrong hardPodAffinitySymmetricWeight, ecpected: %d, got: %d", v1.DefaultHardPodAffinitySymmetricWeight, hpa)
	}

	// Verify that node label predicate/priority are converted to framework plugins.
	wantArgs := `{"Name":"NodeLabel","Args":{"presentLabels":["zone"],"absentLabels":["foo"],"presentLabelsPreference":["l1"],"absentLabelsPreference":["l2"]}}`
	verifyPluginConvertion(t, nodelabel.Name, []string{"FilterPlugin", "ScorePlugin"}, sched, 6, wantArgs)
	// Verify that service affinity custom predicate/priority is converted to framework plugin.
	wantArgs = `{"Name":"ServiceAffinity","Args":{"labels":["zone","foo"],"antiAffinityLabelsPreference":["rack","zone"]}}`
	verifyPluginConvertion(t, serviceaffinity.Name, []string{"FilterPlugin", "ScorePlugin"}, sched, 6, wantArgs)
}

func verifyPluginConvertion(t *testing.T, name string, extentionPoints []string, sched *Scheduler, wantWeight int32, wantArgs string) {
	for _, extensionPoint := range extentionPoints {
		plugin, ok := findPlugin(name, extensionPoint, sched)
		if !ok {
			t.Fatalf("%q plugin does not exist in framework.", name)
		}
		if extensionPoint == "ScorePlugin" {
			if plugin.Weight != wantWeight {
				t.Errorf("Wrong weight. Got: %v, want: %v", plugin.Weight, wantWeight)
			}
		}
		// Verify that the policy config is converted to plugin config.
		pluginConfig := findPluginConfig(name, sched)
		encoding, err := json.Marshal(pluginConfig)
		if err != nil {
			t.Errorf("Failed to marshal %+v: %v", pluginConfig, err)
		}
		if string(encoding) != wantArgs {
			t.Errorf("Config for %v plugin mismatch. got: %v, want: %v", name, string(encoding), wantArgs)
		}
	}
}

func findPlugin(name, extensionPoint string, sched *Scheduler) (schedulerapi.Plugin, bool) {
	for _, pl := range sched.Framework.ListPlugins()[extensionPoint] {
		if pl.Name == name {
			return pl, true
		}
	}
	return schedulerapi.Plugin{}, false
}

func findPluginConfig(name string, sched *Scheduler) schedulerapi.PluginConfig {
	for _, c := range sched.PluginConfig {
		if c.Name == name {
			return c
		}
	}
	return schedulerapi.PluginConfig{}
}

func TestCreateFromConfigWithHardPodAffinitySymmetricWeight(t *testing.T) {
	var configData []byte
	var policy schedulerapi.Policy

	client := fake.NewSimpleClientset()
	stopCh := make(chan struct{})
	defer close(stopCh)
	factory := newConfigFactory(client, v1.DefaultHardPodAffinitySymmetricWeight, stopCh)

	// Pre-register some predicate and priority functions
	RegisterFitPredicate("PredicateOne", PredicateFunc)
	RegisterFitPredicate("PredicateTwo", PredicateFunc)
	RegisterPriorityMapReduceFunction("PriorityOne", PriorityFunc, nil, 1)
	RegisterPriorityMapReduceFunction("PriorityTwo", PriorityFunc, nil, 1)

	configData = []byte(`{
		"kind" : "Policy",
		"apiVersion" : "v1",
		"predicates" : [
			{"name" : "TestZoneAffinity", "argument" : {"serviceAffinity" : {"labels" : ["zone"]}}},
			{"name" : "TestRequireZone", "argument" : {"labelsPresence" : {"labels" : ["zone"], "presence" : true}}},
			{"name" : "PredicateOne"},
			{"name" : "PredicateTwo"}
		],
		"priorities" : [
			{"name" : "RackSpread", "weight" : 3, "argument" : {"serviceAntiAffinity" : {"label" : "rack"}}},
			{"name" : "PriorityOne", "weight" : 2},
			{"name" : "PriorityTwo", "weight" : 1}
		],
		"hardPodAffinitySymmetricWeight" : 10
	}`)
	if err := runtime.DecodeInto(scheme.Codecs.UniversalDecoder(), configData, &policy); err != nil {
		t.Errorf("Invalid configuration: %v", err)
	}
	factory.CreateFromConfig(policy)
	hpa := factory.GetHardPodAffinitySymmetricWeight()
	if hpa != 10 {
		t.Errorf("Wrong hardPodAffinitySymmetricWeight, ecpected: %d, got: %d", 10, hpa)
	}
}

func TestCreateFromEmptyConfig(t *testing.T) {
	var configData []byte
	var policy schedulerapi.Policy

	client := fake.NewSimpleClientset()
	stopCh := make(chan struct{})
	defer close(stopCh)
	factory := newConfigFactory(client, v1.DefaultHardPodAffinitySymmetricWeight, stopCh)

	configData = []byte(`{}`)
	if err := runtime.DecodeInto(scheme.Codecs.UniversalDecoder(), configData, &policy); err != nil {
		t.Errorf("Invalid configuration: %v", err)
	}

	factory.CreateFromConfig(policy)
}

// Test configures a scheduler from a policy that does not specify any
// predicate/priority.
// The predicate/priority from DefaultProvider will be used.
func TestCreateFromConfigWithUnspecifiedPredicatesOrPriorities(t *testing.T) {
	client := fake.NewSimpleClientset()
	stopCh := make(chan struct{})
	defer close(stopCh)
	factory := newConfigFactory(client, v1.DefaultHardPodAffinitySymmetricWeight, stopCh)

	RegisterFitPredicate("PredicateOne", PredicateFunc)
	RegisterPriorityMapReduceFunction("PriorityOne", PriorityFunc, nil, 1)

	RegisterAlgorithmProvider(DefaultProvider, sets.NewString("PredicateOne"), sets.NewString("PriorityOne"))

	configData := []byte(`{
		"kind" : "Policy",
		"apiVersion" : "v1"
	}`)
	var policy schedulerapi.Policy
	if err := runtime.DecodeInto(scheme.Codecs.UniversalDecoder(), configData, &policy); err != nil {
		t.Fatalf("Invalid configuration: %v", err)
	}

	c, err := factory.CreateFromConfig(policy)
	if err != nil {
		t.Fatalf("Failed to create scheduler from configuration: %v", err)
	}
	if _, found := c.Algorithm.Predicates()["PredicateOne"]; !found {
		t.Errorf("Expected predicate PredicateOne from %q", DefaultProvider)
	}
	if len(c.Algorithm.Prioritizers()) != 1 || c.Algorithm.Prioritizers()[0].Name != "PriorityOne" {
		t.Errorf("Expected priority PriorityOne from %q", DefaultProvider)
	}
}

// Test configures a scheduler from a policy that contains empty
// predicate/priority.
// Empty predicate/priority sets will be used.
func TestCreateFromConfigWithEmptyPredicatesOrPriorities(t *testing.T) {
	client := fake.NewSimpleClientset()
	stopCh := make(chan struct{})
	defer close(stopCh)
	factory := newConfigFactory(client, v1.DefaultHardPodAffinitySymmetricWeight, stopCh)

	RegisterFitPredicate("PredicateOne", PredicateFunc)
	RegisterPriorityMapReduceFunction("PriorityOne", PriorityFunc, nil, 1)

	RegisterAlgorithmProvider(DefaultProvider, sets.NewString("PredicateOne"), sets.NewString("PriorityOne"))

	configData := []byte(`{
		"kind" : "Policy",
		"apiVersion" : "v1",
		"predicates" : [],
		"priorities" : []
	}`)
	var policy schedulerapi.Policy
	if err := runtime.DecodeInto(scheme.Codecs.UniversalDecoder(), configData, &policy); err != nil {
		t.Fatalf("Invalid configuration: %v", err)
	}

	c, err := factory.CreateFromConfig(policy)
	if err != nil {
		t.Fatalf("Failed to create scheduler from configuration: %v", err)
	}
	if len(c.Algorithm.Predicates()) != 0 {
		t.Error("Expected empty predicate sets")
	}
	if len(c.Algorithm.Prioritizers()) != 0 {
		t.Error("Expected empty priority sets")
	}
}

func PredicateFunc(pod *v1.Pod, meta predicates.Metadata, nodeInfo *schedulernodeinfo.NodeInfo) (bool, []predicates.PredicateFailureReason, error) {
	return true, nil, nil
}

func PriorityFunc(pod *v1.Pod, meta interface{}, nodeInfo *schedulernodeinfo.NodeInfo) (framework.NodeScore, error) {
	return framework.NodeScore{}, nil
}

func TestDefaultErrorFunc(t *testing.T) {
	testPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "bar"},
		Spec:       apitesting.V1DeepEqualSafePodSpec(),
	}
	testPodInfo := &framework.PodInfo{Pod: testPod}
	client := fake.NewSimpleClientset(&v1.PodList{Items: []v1.Pod{*testPod}})
	stopCh := make(chan struct{})
	defer close(stopCh)

	timestamp := time.Now()
	queue := internalqueue.NewPriorityQueue(nil, nil, internalqueue.WithClock(clock.NewFakeClock(timestamp)))
	schedulerCache := internalcache.New(30*time.Second, stopCh)
	errFunc := MakeDefaultErrorFunc(client, queue, schedulerCache)

	// Trigger error handling again to put the pod in unschedulable queue
	errFunc(testPodInfo, nil)

	// Try up to a minute to retrieve the error pod from priority queue
	foundPodFlag := false
	maxIterations := 10 * 60
	for i := 0; i < maxIterations; i++ {
		time.Sleep(100 * time.Millisecond)
		got := getPodfromPriorityQueue(queue, testPod)
		if got == nil {
			continue
		}

		testClientGetPodRequest(client, t, testPod.Namespace, testPod.Name)

		if e, a := testPod, got; !reflect.DeepEqual(e, a) {
			t.Errorf("Expected %v, got %v", e, a)
		}

		foundPodFlag = true
		break
	}

	if !foundPodFlag {
		t.Errorf("Failed to get pod from the unschedulable queue after waiting for a minute: %v", testPod)
	}

	// Remove the pod from priority queue to test putting error
	// pod in backoff queue.
	queue.Delete(testPod)

	// Trigger a move request
	queue.MoveAllToActiveOrBackoffQueue("test")

	// Trigger error handling again to put the pod in backoff queue
	errFunc(testPodInfo, nil)

	foundPodFlag = false
	for i := 0; i < maxIterations; i++ {
		time.Sleep(100 * time.Millisecond)
		// The pod should be found from backoff queue at this time
		got := getPodfromPriorityQueue(queue, testPod)
		if got == nil {
			continue
		}

		testClientGetPodRequest(client, t, testPod.Namespace, testPod.Name)

		if e, a := testPod, got; !reflect.DeepEqual(e, a) {
			t.Errorf("Expected %v, got %v", e, a)
		}

		foundPodFlag = true
		break
	}

	if !foundPodFlag {
		t.Errorf("Failed to get pod from the backoff queue after waiting for a minute: %v", testPod)
	}
}

// getPodfromPriorityQueue is the function used in the TestDefaultErrorFunc test to get
// the specific pod from the given priority queue. It returns the found pod in the priority queue.
func getPodfromPriorityQueue(queue *internalqueue.PriorityQueue, pod *v1.Pod) *v1.Pod {
	podList := queue.PendingPods()
	if len(podList) == 0 {
		return nil
	}

	queryPodKey, err := cache.MetaNamespaceKeyFunc(pod)
	if err != nil {
		return nil
	}

	for _, foundPod := range podList {
		foundPodKey, err := cache.MetaNamespaceKeyFunc(foundPod)
		if err != nil {
			return nil
		}

		if foundPodKey == queryPodKey {
			return foundPod
		}
	}

	return nil
}

// testClientGetPodRequest function provides a routine used by TestDefaultErrorFunc test.
// It tests whether the fake client can receive request and correctly "get" the namespace
// and name of the error pod.
func testClientGetPodRequest(client *fake.Clientset, t *testing.T, podNs string, podName string) {
	requestReceived := false
	actions := client.Actions()
	for _, a := range actions {
		if a.GetVerb() == "get" {
			getAction, ok := a.(clienttesting.GetAction)
			if !ok {
				t.Errorf("Can't cast action object to GetAction interface")
				break
			}
			name := getAction.GetName()
			ns := a.GetNamespace()
			if name != podName || ns != podNs {
				t.Errorf("Expected name %s namespace %s, got %s %s",
					podName, podNs, name, ns)
			}
			requestReceived = true
		}
	}
	if !requestReceived {
		t.Errorf("Get pod request not received")
	}
}

func TestBind(t *testing.T) {
	table := []struct {
		name    string
		binding *v1.Binding
	}{
		{
			name: "binding can bind and validate request",
			binding: &v1.Binding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: metav1.NamespaceDefault,
					Name:      "foo",
				},
				Target: v1.ObjectReference{
					Name: "foohost.kubernetes.mydomain.com",
				},
			},
		},
	}

	for _, test := range table {
		t.Run(test.name, func(t *testing.T) {
			testBind(test.binding, t)
		})
	}
}

func testBind(binding *v1.Binding, t *testing.T) {
	testPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: binding.GetName(), Namespace: metav1.NamespaceDefault},
		Spec:       apitesting.V1DeepEqualSafePodSpec(),
	}
	client := fake.NewSimpleClientset(&v1.PodList{Items: []v1.Pod{*testPod}})

	b := binder{client}

	if err := b.Bind(binding); err != nil {
		t.Errorf("Unexpected error: %v", err)
		return
	}

	pod := client.CoreV1().Pods(metav1.NamespaceDefault).(*fakeV1.FakePods)

	actualBinding, err := pod.GetBinding(binding.GetName())
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
		return
	}
	if !reflect.DeepEqual(binding, actualBinding) {
		t.Errorf("Binding did not match expectation")
		t.Logf("Expected: %v", binding)
		t.Logf("Actual:   %v", actualBinding)
	}
}

func TestInvalidHardPodAffinitySymmetricWeight(t *testing.T) {
	client := fake.NewSimpleClientset()
	// factory of "default-scheduler"
	stopCh := make(chan struct{})
	factory := newConfigFactory(client, -1, stopCh)
	defer close(stopCh)
	_, err := factory.Create()
	if err == nil {
		t.Errorf("expected err: invalid hardPodAffinitySymmetricWeight, got nothing")
	}
}

func TestInvalidFactoryArgs(t *testing.T) {
	client := fake.NewSimpleClientset()

	testCases := []struct {
		name                           string
		hardPodAffinitySymmetricWeight int32
		expectErr                      string
	}{
		{
			name:                           "symmetric weight below range",
			hardPodAffinitySymmetricWeight: -1,
			expectErr:                      "invalid hardPodAffinitySymmetricWeight: -1, must be in the range [0-100]",
		},
		{
			name:                           "symmetric weight above range",
			hardPodAffinitySymmetricWeight: 101,
			expectErr:                      "invalid hardPodAffinitySymmetricWeight: 101, must be in the range [0-100]",
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			stopCh := make(chan struct{})
			factory := newConfigFactory(client, test.hardPodAffinitySymmetricWeight, stopCh)
			defer close(stopCh)
			_, err := factory.Create()
			if err == nil {
				t.Errorf("expected err: %s, got nothing", test.expectErr)
			}
		})
	}

}

func newConfigFactoryWithFrameworkRegistry(
	client clientset.Interface, hardPodAffinitySymmetricWeight int32, stopCh <-chan struct{},
	registry framework.Registry, pluginConfigProducerRegistry *frameworkplugins.ConfigProducerRegistry) *Configurator {
	informerFactory := informers.NewSharedInformerFactory(client, 0)
	snapshot := nodeinfosnapshot.NewEmptySnapshot()
	return &Configurator{
		client:                         client,
		informerFactory:                informerFactory,
		podInformer:                    informerFactory.Core().V1().Pods(),
		hardPodAffinitySymmetricWeight: hardPodAffinitySymmetricWeight,
		disablePreemption:              disablePodPreemption,
		percentageOfNodesToScore:       schedulerapi.DefaultPercentageOfNodesToScore,
		bindTimeoutSeconds:             bindTimeoutSeconds,
		podInitialBackoffSeconds:       podInitialBackoffDurationSeconds,
		podMaxBackoffSeconds:           podMaxBackoffDurationSeconds,
		StopEverything:                 stopCh,
		enableNonPreempting:            utilfeature.DefaultFeatureGate.Enabled(kubefeatures.NonPreemptingPriority),
		registry:                       registry,
		plugins:                        nil,
		pluginConfig:                   []schedulerapi.PluginConfig{},
		pluginConfigProducerRegistry:   pluginConfigProducerRegistry,
		nodeInfoSnapshot:               snapshot,
		algorithmFactoryArgs: AlgorithmFactoryArgs{
			SharedLister:                   snapshot,
			InformerFactory:                informerFactory,
			HardPodAffinitySymmetricWeight: hardPodAffinitySymmetricWeight,
		},
		configProducerArgs: &frameworkplugins.ConfigProducerArgs{},
	}
}

func newConfigFactory(
	client clientset.Interface, hardPodAffinitySymmetricWeight int32, stopCh <-chan struct{}) *Configurator {
	return newConfigFactoryWithFrameworkRegistry(client, hardPodAffinitySymmetricWeight, stopCh,
		frameworkplugins.NewDefaultRegistry(&frameworkplugins.RegistryArgs{}), frameworkplugins.NewDefaultConfigProducerRegistry())
}

type fakeExtender struct {
	isBinder          bool
	interestedPodName string
	ignorable         bool
}

func (f *fakeExtender) Name() string {
	return "fakeExtender"
}

func (f *fakeExtender) IsIgnorable() bool {
	return f.ignorable
}

func (f *fakeExtender) ProcessPreemption(
	pod *v1.Pod,
	nodeToVictims map[*v1.Node]*extenderv1.Victims,
	nodeNameToInfo map[string]*schedulernodeinfo.NodeInfo,
) (map[*v1.Node]*extenderv1.Victims, error) {
	return nil, nil
}

func (f *fakeExtender) SupportsPreemption() bool {
	return false
}

func (f *fakeExtender) Filter(
	pod *v1.Pod,
	nodes []*v1.Node,
	nodeNameToInfo map[string]*schedulernodeinfo.NodeInfo,
) (filteredNodes []*v1.Node, failedNodesMap extenderv1.FailedNodesMap, err error) {
	return nil, nil, nil
}

func (f *fakeExtender) Prioritize(
	pod *v1.Pod,
	nodes []*v1.Node,
) (hostPriorities *extenderv1.HostPriorityList, weight int64, err error) {
	return nil, 0, nil
}

func (f *fakeExtender) Bind(binding *v1.Binding) error {
	if f.isBinder {
		return nil
	}
	return errors.New("not a binder")
}

func (f *fakeExtender) IsBinder() bool {
	return f.isBinder
}

func (f *fakeExtender) IsInterested(pod *v1.Pod) bool {
	return pod != nil && pod.Name == f.interestedPodName
}

func TestGetBinderFunc(t *testing.T) {
	table := []struct {
		podName            string
		extenders          []algorithm.SchedulerExtender
		expectedBinderType string
		name               string
	}{
		{
			name:    "the extender is not a binder",
			podName: "pod0",
			extenders: []algorithm.SchedulerExtender{
				&fakeExtender{isBinder: false, interestedPodName: "pod0"},
			},
			expectedBinderType: "*scheduler.binder",
		},
		{
			name:    "one of the extenders is a binder and interested in pod",
			podName: "pod0",
			extenders: []algorithm.SchedulerExtender{
				&fakeExtender{isBinder: false, interestedPodName: "pod0"},
				&fakeExtender{isBinder: true, interestedPodName: "pod0"},
			},
			expectedBinderType: "*scheduler.fakeExtender",
		},
		{
			name:    "one of the extenders is a binder, but not interested in pod",
			podName: "pod1",
			extenders: []algorithm.SchedulerExtender{
				&fakeExtender{isBinder: false, interestedPodName: "pod1"},
				&fakeExtender{isBinder: true, interestedPodName: "pod0"},
			},
			expectedBinderType: "*scheduler.binder",
		},
	}

	for _, test := range table {
		t.Run(test.name, func(t *testing.T) {
			testGetBinderFunc(test.expectedBinderType, test.podName, test.extenders, t)
		})
	}
}

func testGetBinderFunc(expectedBinderType, podName string, extenders []algorithm.SchedulerExtender, t *testing.T) {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: podName,
		},
	}

	f := &Configurator{}
	binderFunc := getBinderFunc(f.client, extenders)
	binder := binderFunc(pod)

	binderType := fmt.Sprintf("%s", reflect.TypeOf(binder))
	if binderType != expectedBinderType {
		t.Errorf("Expected binder %q but got %q", expectedBinderType, binderType)
	}
}

type TestPlugin struct {
	name string
}

var _ framework.ScorePlugin = &TestPlugin{}
var _ framework.FilterPlugin = &TestPlugin{}

func (t *TestPlugin) Name() string {
	return t.name
}

func (t *TestPlugin) Score(ctx context.Context, state *framework.CycleState, p *v1.Pod, nodeName string) (int64, *framework.Status) {
	return 1, nil
}

func (t *TestPlugin) ScoreExtensions() framework.ScoreExtensions {
	return nil
}

func (t *TestPlugin) Filter(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeInfo *schedulernodeinfo.NodeInfo) *framework.Status {
	return nil
}

// Test configures a scheduler from a policies defined in a file
// It combines some configurable predicate/priorities with some pre-defined ones
func TestCreateWithFrameworkPlugins(t *testing.T) {
	var configData []byte
	var policy schedulerapi.Policy

	client := fake.NewSimpleClientset()
	stopCh := make(chan struct{})
	defer close(stopCh)

	predicateOneName := "PredicateOne"
	filterOneName := "FilterOne"
	predicateTwoName := "PredicateTwo"
	filterTwoName := "FilterTwo"
	predicateThreeName := "PredicateThree"
	predicateFourName := "PredicateFour"
	priorityOneName := "PriorityOne"
	scoreOneName := "ScoreOne"
	priorityTwoName := "PriorityTwo"
	scoreTwoName := "ScoreTwo"
	priorityThreeName := "PriorityThree"

	configProducerRegistry := &frameworkplugins.ConfigProducerRegistry{
		PredicateToConfigProducer: make(map[string]frameworkplugins.ConfigProducer),
		PriorityToConfigProducer:  make(map[string]frameworkplugins.ConfigProducer),
	}
	configProducerRegistry.RegisterPredicate(predicateOneName,
		func(_ frameworkplugins.ConfigProducerArgs) (schedulerapi.Plugins, []schedulerapi.PluginConfig) {
			return schedulerapi.Plugins{
				Filter: &schedulerapi.PluginSet{
					Enabled: []schedulerapi.Plugin{
						{Name: filterOneName}}}}, nil
		})

	configProducerRegistry.RegisterPredicate(predicateTwoName,
		func(_ frameworkplugins.ConfigProducerArgs) (schedulerapi.Plugins, []schedulerapi.PluginConfig) {
			return schedulerapi.Plugins{
				Filter: &schedulerapi.PluginSet{
					Enabled: []schedulerapi.Plugin{
						{Name: filterTwoName}}}}, nil
		})

	configProducerRegistry.RegisterPriority(priorityOneName,
		func(args frameworkplugins.ConfigProducerArgs) (schedulerapi.Plugins, []schedulerapi.PluginConfig) {
			return schedulerapi.Plugins{
				Score: &schedulerapi.PluginSet{
					Enabled: []schedulerapi.Plugin{
						{Name: scoreOneName, Weight: args.Weight}}}}, nil
		})

	configProducerRegistry.RegisterPriority(priorityTwoName,
		func(args frameworkplugins.ConfigProducerArgs) (schedulerapi.Plugins, []schedulerapi.PluginConfig) {
			return schedulerapi.Plugins{
				Score: &schedulerapi.PluginSet{
					Enabled: []schedulerapi.Plugin{
						{Name: scoreTwoName, Weight: args.Weight}}}}, nil
		})

	registry := framework.Registry{
		filterOneName: func(_ *runtime.Unknown, fh framework.FrameworkHandle) (framework.Plugin, error) {
			return &TestPlugin{name: filterOneName}, nil
		},
		filterTwoName: func(_ *runtime.Unknown, fh framework.FrameworkHandle) (framework.Plugin, error) {
			return &TestPlugin{name: filterTwoName}, nil
		},
		scoreOneName: func(_ *runtime.Unknown, fh framework.FrameworkHandle) (framework.Plugin, error) {
			return &TestPlugin{name: scoreOneName}, nil
		},
		scoreTwoName: func(_ *runtime.Unknown, fh framework.FrameworkHandle) (framework.Plugin, error) {
			return &TestPlugin{name: scoreTwoName}, nil
		},
	}

	factory := newConfigFactoryWithFrameworkRegistry(
		client, v1.DefaultHardPodAffinitySymmetricWeight, stopCh, registry, configProducerRegistry)

	// Pre-register some predicate and priority functions
	RegisterMandatoryFitPredicate(predicateOneName, PredicateFunc)
	RegisterFitPredicate(predicateTwoName, PredicateFunc)
	RegisterFitPredicate(predicateThreeName, PredicateFunc)
	RegisterMandatoryFitPredicate(predicateFourName, PredicateFunc)
	RegisterPriorityMapReduceFunction(priorityOneName, PriorityFunc, nil, 1)
	RegisterPriorityMapReduceFunction(priorityTwoName, PriorityFunc, nil, 1)
	RegisterPriorityMapReduceFunction(priorityThreeName, PriorityFunc, nil, 1)

	configData = []byte(`{
		"kind" : "Policy",
		"apiVersion" : "v1",
		"predicates" : [
			{"name" : "PredicateOne"},
			{"name" : "PredicateTwo"},
			{"name" : "PredicateThree"},
			{"name" : "PredicateThree"}
		],
		"priorities" : [
			{"name" : "PriorityOne", "weight" : 2},
			{"name" : "PriorityTwo", "weight" : 1},
			{"name" : "PriorityThree", "weight" : 1}		]
	}`)
	if err := runtime.DecodeInto(scheme.Codecs.UniversalDecoder(), configData, &policy); err != nil {
		t.Errorf("Invalid configuration: %v", err)
	}

	c, err := factory.CreateFromConfig(policy)
	if err != nil {
		t.Fatalf("creating config: %v", err)
	}

	gotPredicates := sets.NewString()
	for p := range c.Algorithm.Predicates() {
		gotPredicates.Insert(p)
	}
	wantPredicates := sets.NewString(
		predicateThreeName,
		predicateFourName,
	)
	if diff := cmp.Diff(wantPredicates, gotPredicates); diff != "" {
		t.Errorf("unexpected predicates diff (-want, +got): %s", diff)
	}

	gotPriorities := sets.NewString()
	for _, p := range c.Algorithm.Prioritizers() {
		gotPriorities.Insert(p.Name)
	}
	wantPriorities := sets.NewString(priorityThreeName)
	if diff := cmp.Diff(wantPriorities, gotPriorities); diff != "" {
		t.Errorf("unexpected priorities diff (-want, +got): %s", diff)
	}

	// Verify the aggregated configuration.
	wantPlugins := schedulerapi.Plugins{
		QueueSort: &schedulerapi.PluginSet{},
		PreFilter: &schedulerapi.PluginSet{},
		Filter: &schedulerapi.PluginSet{
			Enabled: []schedulerapi.Plugin{
				{Name: filterOneName},
				{Name: filterTwoName},
			},
		},
		PostFilter: &schedulerapi.PluginSet{},
		Score: &schedulerapi.PluginSet{
			Enabled: []schedulerapi.Plugin{
				{Name: scoreOneName, Weight: 2},
				{Name: scoreTwoName, Weight: 1},
			},
		},
		Reserve:   &schedulerapi.PluginSet{},
		Permit:    &schedulerapi.PluginSet{},
		PreBind:   &schedulerapi.PluginSet{},
		Bind:      &schedulerapi.PluginSet{},
		PostBind:  &schedulerapi.PluginSet{},
		Unreserve: &schedulerapi.PluginSet{},
	}

	trans := cmp.Transformer("Sort", func(in []schedulerapi.Plugin) []schedulerapi.Plugin {
		out := append([]schedulerapi.Plugin(nil), in...) // Copy input to avoid mutating it
		sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
		return out
	})

	if diff := cmp.Diff(wantPlugins, c.Plugins, trans); diff != "" {
		t.Errorf("unexpected plugin configuration (-want, +got): %s", diff)
	}
}
