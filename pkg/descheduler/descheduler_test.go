package descheduler

import (
	"context"
	"fmt"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/runtime"
	apiversion "k8s.io/apimachinery/pkg/version"
	fakediscovery "k8s.io/client-go/discovery/fake"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"sigs.k8s.io/descheduler/cmd/descheduler/app/options"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/api/v1alpha1"
	"sigs.k8s.io/descheduler/pkg/framework/pluginregistry"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removeduplicates"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodsviolatingnodetaints"
	deschedulerversion "sigs.k8s.io/descheduler/pkg/version"
	"sigs.k8s.io/descheduler/test"
)

// scope contains information about an ongoing conversion.
type scope struct {
	converter *conversion.Converter
	meta      *conversion.Meta
}

// Convert continues a conversion.
func (s scope) Convert(src, dest interface{}) error {
	return s.converter.Convert(src, dest, s.meta)
}

// Meta returns the meta object that was originally passed to Convert.
func (s scope) Meta() *conversion.Meta {
	return s.meta
}

func TestTaintsUpdated(t *testing.T) {
	pluginregistry.PluginRegistry = pluginregistry.NewRegistry()
	pluginregistry.Register(removepodsviolatingnodetaints.PluginName, removepodsviolatingnodetaints.New, &removepodsviolatingnodetaints.RemovePodsViolatingNodeTaints{}, &removepodsviolatingnodetaints.RemovePodsViolatingNodeTaintsArgs{}, removepodsviolatingnodetaints.ValidateRemovePodsViolatingNodeTaintsArgs, removepodsviolatingnodetaints.SetDefaults_RemovePodsViolatingNodeTaintsArgs, pluginregistry.PluginRegistry)
	pluginregistry.Register(defaultevictor.PluginName, defaultevictor.New, &defaultevictor.DefaultEvictor{}, &defaultevictor.DefaultEvictorArgs{}, defaultevictor.ValidateDefaultEvictorArgs, defaultevictor.SetDefaults_DefaultEvictorArgs, pluginregistry.PluginRegistry)

	ctx := context.Background()
	n1 := test.BuildTestNode("n1", 2000, 3000, 10, nil)
	n2 := test.BuildTestNode("n2", 2000, 3000, 10, nil)

	p1 := test.BuildTestPod(fmt.Sprintf("pod_1_%s", n1.Name), 200, 0, n1.Name, nil)
	p1.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
		{},
	}

	client := fakeclientset.NewSimpleClientset(n1, n2, p1)
	eventClient := fakeclientset.NewSimpleClientset(n1, n2, p1)
	dp := &v1alpha1.DeschedulerPolicy{
		Strategies: v1alpha1.StrategyList{
			"RemovePodsViolatingNodeTaints": v1alpha1.DeschedulerStrategy{
				Enabled: true,
			},
		},
	}

	rs, err := options.NewDeschedulerServer()
	if err != nil {
		t.Fatalf("Unable to initialize server: %v", err)
	}
	rs.Client = client
	rs.EventClient = eventClient

	pods, err := client.CoreV1().Pods(p1.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Errorf("Unable to list pods: %v", err)
	}
	if len(pods.Items) < 1 {
		t.Errorf("The pod was evicted before a node was tained")
	}

	n1WithTaint := n1.DeepCopy()
	n1WithTaint.Spec.Taints = []v1.Taint{
		{
			Key:    "key",
			Value:  "value",
			Effect: v1.TaintEffectNoSchedule,
		},
	}

	if _, err := client.CoreV1().Nodes().Update(ctx, n1WithTaint, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("Unable to update node: %v\n", err)
	}

	var evictedPods []string
	client.PrependReactor("create", "pods", podEvictionReactionFuc(&evictedPods))

	internalDeschedulerPolicy := &api.DeschedulerPolicy{}
	scope := scope{}
	err = v1alpha1.V1alpha1ToInternal(dp, pluginregistry.PluginRegistry, internalDeschedulerPolicy, scope)
	if err != nil {
		t.Fatalf("Unable to convert v1alpha1 to v1alpha2: %v", err)
	}

	if err := RunDeschedulerStrategies(ctx, rs, internalDeschedulerPolicy, "v1"); err != nil {
		t.Fatalf("Unable to run descheduler strategies: %v", err)
	}

	if len(evictedPods) != 1 {
		t.Fatalf("Unable to evict pod, node taint did not get propagated to descheduler strategies %v\n", err)
	}
}

func TestDuplicate(t *testing.T) {
	pluginregistry.PluginRegistry = pluginregistry.NewRegistry()
	pluginregistry.Register(removeduplicates.PluginName, removeduplicates.New, &removeduplicates.RemoveDuplicates{}, &removeduplicates.RemoveDuplicatesArgs{}, removeduplicates.ValidateRemoveDuplicatesArgs, removeduplicates.SetDefaults_RemoveDuplicatesArgs, pluginregistry.PluginRegistry)
	pluginregistry.Register(defaultevictor.PluginName, defaultevictor.New, &defaultevictor.DefaultEvictor{}, &defaultevictor.DefaultEvictorArgs{}, defaultevictor.ValidateDefaultEvictorArgs, defaultevictor.SetDefaults_DefaultEvictorArgs, pluginregistry.PluginRegistry)

	ctx := context.Background()
	node1 := test.BuildTestNode("n1", 2000, 3000, 10, nil)
	node2 := test.BuildTestNode("n2", 2000, 3000, 10, nil)

	p1 := test.BuildTestPod("p1", 100, 0, node1.Name, nil)
	p1.Namespace = "dev"
	p2 := test.BuildTestPod("p2", 100, 0, node1.Name, nil)
	p2.Namespace = "dev"
	p3 := test.BuildTestPod("p3", 100, 0, node1.Name, nil)
	p3.Namespace = "dev"

	ownerRef1 := test.GetReplicaSetOwnerRefList()
	p1.ObjectMeta.OwnerReferences = ownerRef1
	p2.ObjectMeta.OwnerReferences = ownerRef1
	p3.ObjectMeta.OwnerReferences = ownerRef1

	client := fakeclientset.NewSimpleClientset(node1, node2, p1, p2, p3)
	eventClient := fakeclientset.NewSimpleClientset(node1, node2, p1, p2, p3)
	dp := &v1alpha1.DeschedulerPolicy{
		Strategies: v1alpha1.StrategyList{
			"RemoveDuplicates": v1alpha1.DeschedulerStrategy{
				Enabled: true,
			},
		},
	}

	rs, err := options.NewDeschedulerServer()
	if err != nil {
		t.Fatalf("Unable to initialize server: %v", err)
	}
	rs.Client = client
	rs.EventClient = eventClient

	pods, err := client.CoreV1().Pods(p1.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Errorf("Unable to list pods: %v", err)
	}

	if len(pods.Items) != 3 {
		t.Errorf("Pods number should be 3 before evict")
	}

	var evictedPods []string
	client.PrependReactor("create", "pods", podEvictionReactionFuc(&evictedPods))

	internalDeschedulerPolicy := &api.DeschedulerPolicy{}
	scope := scope{}
	err = v1alpha1.V1alpha1ToInternal(dp, pluginregistry.PluginRegistry, internalDeschedulerPolicy, scope)
	if err != nil {
		t.Fatalf("Unable to convert v1alpha1 to v1alpha2: %v", err)
	}
	if err := RunDeschedulerStrategies(ctx, rs, internalDeschedulerPolicy, "v1"); err != nil {
		t.Fatalf("Unable to run descheduler strategies: %v", err)
	}

	if len(evictedPods) == 0 {
		t.Fatalf("Unable to evict pod, node taint did not get propagated to descheduler strategies %v\n", err)
	}
}

func TestRootCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	n1 := test.BuildTestNode("n1", 2000, 3000, 10, nil)
	n2 := test.BuildTestNode("n2", 2000, 3000, 10, nil)
	client := fakeclientset.NewSimpleClientset(n1, n2)
	eventClient := fakeclientset.NewSimpleClientset(n1, n2)
	dp := &api.DeschedulerPolicy{
		Profiles: []api.DeschedulerProfile{}, // no strategies needed for this test
	}

	rs, err := options.NewDeschedulerServer()
	if err != nil {
		t.Fatalf("Unable to initialize server: %v", err)
	}
	rs.Client = client
	rs.EventClient = eventClient
	rs.DeschedulingInterval = 100 * time.Millisecond
	errChan := make(chan error, 1)
	defer close(errChan)

	go func() {
		err := RunDeschedulerStrategies(ctx, rs, dp, "v1")
		errChan <- err
	}()
	cancel()
	select {
	case err := <-errChan:
		if err != nil {
			t.Fatalf("Unable to run descheduler strategies: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Root ctx should have canceled immediately")
	}
}

func TestRootCancelWithNoInterval(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	n1 := test.BuildTestNode("n1", 2000, 3000, 10, nil)
	n2 := test.BuildTestNode("n2", 2000, 3000, 10, nil)
	client := fakeclientset.NewSimpleClientset(n1, n2)
	eventClient := fakeclientset.NewSimpleClientset(n1, n2)
	dp := &api.DeschedulerPolicy{
		Profiles: []api.DeschedulerProfile{}, // no strategies needed for this test
	}

	rs, err := options.NewDeschedulerServer()
	if err != nil {
		t.Fatalf("Unable to initialize server: %v", err)
	}
	rs.Client = client
	rs.EventClient = eventClient
	rs.DeschedulingInterval = 0
	errChan := make(chan error, 1)
	defer close(errChan)

	go func() {
		err := RunDeschedulerStrategies(ctx, rs, dp, "v1")
		errChan <- err
	}()
	cancel()
	select {
	case err := <-errChan:
		if err != nil {
			t.Fatalf("Unable to run descheduler strategies: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Root ctx should have canceled immediately")
	}
}

func TestValidateVersionCompatibility(t *testing.T) {
	type testCase struct {
		name             string
		deschedulerMinor string
		serverMinor      string
		expectError      bool
	}
	testCases := []testCase{
		{
			name:             "no error when descheduler minor equals to server minor",
			deschedulerMinor: "26.0",
			serverMinor:      "26",
			expectError:      false,
		},
		{
			name:             "no error when descheduler minor is 3 behind server minor",
			deschedulerMinor: "23.0",
			serverMinor:      "26",
			expectError:      false,
		},
		{
			name:             "no error when descheduler minor is 3 ahead of server minor",
			deschedulerMinor: "26.0",
			serverMinor:      "23",
			expectError:      false,
		},
		{
			name:             "error when descheduler minor is 4 behind server minor",
			deschedulerMinor: "22.0",
			serverMinor:      "26",
			expectError:      true,
		},
		{
			name:             "error when descheduler minor is 4 ahead of server minor",
			deschedulerMinor: "27.0",
			serverMinor:      "23",
			expectError:      true,
		},
	}
	client := fakeclientset.NewSimpleClientset()
	fakeDiscovery, _ := client.Discovery().(*fakediscovery.FakeDiscovery)
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeDiscovery.FakedServerVersion = &apiversion.Info{Major: "1", Minor: tc.serverMinor}
			deschedulerVersion := deschedulerversion.Info{Major: "0", Minor: tc.deschedulerMinor}
			err := validateVersionCompatibility(fakeDiscovery, deschedulerVersion)

			hasError := err != nil
			if tc.expectError != hasError {
				t.Error("unexpected version compatibility behavior")
			}
		})
	}
}

func podEvictionReactionFuc(evictedPods *[]string) func(action core.Action) (bool, runtime.Object, error) {
	return func(action core.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() == "eviction" {
			createAct, matched := action.(core.CreateActionImpl)
			if !matched {
				return false, nil, fmt.Errorf("unable to convert action to core.CreateActionImpl")
			}
			if eviction, matched := createAct.Object.(*policy.Eviction); matched {
				*evictedPods = append(*evictedPods, eviction.GetName())
			}
		}
		return false, nil, nil // fallback to the default reactor
	}
}
