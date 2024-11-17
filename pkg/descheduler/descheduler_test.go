package descheduler

import (
	"context"
	"fmt"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apiversion "k8s.io/apimachinery/pkg/version"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/informers"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/klog/v2"
	utilptr "k8s.io/utils/ptr"
	"sigs.k8s.io/descheduler/cmd/descheduler/app/options"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/framework/pluginregistry"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removeduplicates"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodsviolatingnodetaints"
	"sigs.k8s.io/descheduler/pkg/utils"
	deschedulerversion "sigs.k8s.io/descheduler/pkg/version"
	"sigs.k8s.io/descheduler/test"
)

func initPluginRegistry() {
	pluginregistry.PluginRegistry = pluginregistry.NewRegistry()
	pluginregistry.Register(removeduplicates.PluginName, removeduplicates.New, &removeduplicates.RemoveDuplicates{}, &removeduplicates.RemoveDuplicatesArgs{}, removeduplicates.ValidateRemoveDuplicatesArgs, removeduplicates.SetDefaults_RemoveDuplicatesArgs, pluginregistry.PluginRegistry)
	pluginregistry.Register(defaultevictor.PluginName, defaultevictor.New, &defaultevictor.DefaultEvictor{}, &defaultevictor.DefaultEvictorArgs{}, defaultevictor.ValidateDefaultEvictorArgs, defaultevictor.SetDefaults_DefaultEvictorArgs, pluginregistry.PluginRegistry)
	pluginregistry.Register(removepodsviolatingnodetaints.PluginName, removepodsviolatingnodetaints.New, &removepodsviolatingnodetaints.RemovePodsViolatingNodeTaints{}, &removepodsviolatingnodetaints.RemovePodsViolatingNodeTaintsArgs{}, removepodsviolatingnodetaints.ValidateRemovePodsViolatingNodeTaintsArgs, removepodsviolatingnodetaints.SetDefaults_RemovePodsViolatingNodeTaintsArgs, pluginregistry.PluginRegistry)
}

func removePodsViolatingNodeTaintsPolicy() *api.DeschedulerPolicy {
	return &api.DeschedulerPolicy{
		Profiles: []api.DeschedulerProfile{
			{
				Name: "Profile",
				PluginConfigs: []api.PluginConfig{
					{
						Name: "RemovePodsViolatingNodeTaints",
						Args: &removepodsviolatingnodetaints.RemovePodsViolatingNodeTaintsArgs{},
					},
					{
						Name: "DefaultEvictor",
						Args: &defaultevictor.DefaultEvictorArgs{},
					},
				},
				Plugins: api.Plugins{
					Filter: api.PluginSet{
						Enabled: []string{
							"DefaultEvictor",
						},
					},
					Deschedule: api.PluginSet{
						Enabled: []string{
							"RemovePodsViolatingNodeTaints",
						},
					},
				},
			},
		},
	}
}

func removeDuplicatesPolicy() *api.DeschedulerPolicy {
	return &api.DeschedulerPolicy{
		Profiles: []api.DeschedulerProfile{
			{
				Name: "Profile",
				PluginConfigs: []api.PluginConfig{
					{
						Name: "RemoveDuplicates",
						Args: &removeduplicates.RemoveDuplicatesArgs{},
					},
					{
						Name: "DefaultEvictor",
						Args: &defaultevictor.DefaultEvictorArgs{},
					},
				},
				Plugins: api.Plugins{
					Filter: api.PluginSet{
						Enabled: []string{
							"DefaultEvictor",
						},
					},
					Balance: api.PluginSet{
						Enabled: []string{
							"RemoveDuplicates",
						},
					},
				},
			},
		},
	}
}

func initDescheduler(t *testing.T, ctx context.Context, internalDeschedulerPolicy *api.DeschedulerPolicy, objects ...runtime.Object) (*options.DeschedulerServer, *descheduler, *fakeclientset.Clientset) {
	client := fakeclientset.NewSimpleClientset(objects...)
	eventClient := fakeclientset.NewSimpleClientset(objects...)

	rs, err := options.NewDeschedulerServer()
	if err != nil {
		t.Fatalf("Unable to initialize server: %v", err)
	}
	rs.Client = client
	rs.EventClient = eventClient

	sharedInformerFactory := informers.NewSharedInformerFactoryWithOptions(rs.Client, 0, informers.WithTransform(trimManagedFields))
	eventBroadcaster, eventRecorder := utils.GetRecorderAndBroadcaster(ctx, client)

	descheduler, err := newDescheduler(rs, internalDeschedulerPolicy, "v1", eventRecorder, sharedInformerFactory)
	if err != nil {
		eventBroadcaster.Shutdown()
		t.Fatalf("Unable to create a descheduler instance: %v", err)
	}

	sharedInformerFactory.Start(ctx.Done())
	sharedInformerFactory.WaitForCacheSync(ctx.Done())

	return rs, descheduler, client
}

func TestTaintsUpdated(t *testing.T) {
	initPluginRegistry()

	ctx := context.Background()
	n1 := test.BuildTestNode("n1", 2000, 3000, 10, nil)
	n2 := test.BuildTestNode("n2", 2000, 3000, 10, nil)

	p1 := test.BuildTestPod(fmt.Sprintf("pod_1_%s", n1.Name), 200, 0, n1.Name, nil)
	p1.ObjectMeta.OwnerReferences = test.GetReplicaSetOwnerRefList()

	client := fakeclientset.NewSimpleClientset(n1, n2, p1)
	eventClient := fakeclientset.NewSimpleClientset(n1, n2, p1)

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
	client.PrependReactor("create", "pods", podEvictionReactionTestingFnc(&evictedPods))

	if err := RunDeschedulerStrategies(ctx, rs, removePodsViolatingNodeTaintsPolicy(), "v1"); err != nil {
		t.Fatalf("Unable to run descheduler strategies: %v", err)
	}

	if len(evictedPods) != 1 {
		t.Fatalf("Unable to evict pod, node taint did not get propagated to descheduler strategies %v\n", err)
	}
}

func TestDuplicate(t *testing.T) {
	initPluginRegistry()

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
	client.PrependReactor("create", "pods", podEvictionReactionTestingFnc(&evictedPods))

	if err := RunDeschedulerStrategies(ctx, rs, removeDuplicatesPolicy(), "v1"); err != nil {
		t.Fatalf("Unable to run descheduler strategies: %v", err)
	}

	if len(evictedPods) == 0 {
		t.Fatalf("Unable to evict pods\n")
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
		name               string
		deschedulerVersion deschedulerversion.Info
		serverVersion      string
		expectError        bool
	}
	testCases := []testCase{
		{
			name:               "no error when descheduler minor equals to server minor",
			deschedulerVersion: deschedulerversion.Info{Major: "0", Minor: "26"},
			serverVersion:      "v1.26.1",
			expectError:        false,
		},
		{
			name:               "no error when descheduler minor is 3 behind server minor",
			deschedulerVersion: deschedulerversion.Info{Major: "0", Minor: "23"},
			serverVersion:      "v1.26.1",
			expectError:        false,
		},
		{
			name:               "no error when descheduler minor is 3 ahead of server minor",
			deschedulerVersion: deschedulerversion.Info{Major: "0", Minor: "26"},
			serverVersion:      "v1.26.1",
			expectError:        false,
		},
		{
			name:               "error when descheduler minor is 4 behind server minor",
			deschedulerVersion: deschedulerversion.Info{Major: "0", Minor: "22"},
			serverVersion:      "v1.26.1",
			expectError:        true,
		},
		{
			name:               "error when descheduler minor is 4 ahead of server minor",
			deschedulerVersion: deschedulerversion.Info{Major: "0", Minor: "27"},
			serverVersion:      "v1.23.1",
			expectError:        true,
		},
		{
			name:               "no error when using managed provider version",
			deschedulerVersion: deschedulerversion.Info{Major: "0", Minor: "25"},
			serverVersion:      "v1.25.12-eks-2d98532",
			expectError:        false,
		},
	}
	client := fakeclientset.NewSimpleClientset()
	fakeDiscovery, _ := client.Discovery().(*fakediscovery.FakeDiscovery)
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeDiscovery.FakedServerVersion = &apiversion.Info{GitVersion: tc.serverVersion}
			err := validateVersionCompatibility(fakeDiscovery, tc.deschedulerVersion)

			hasError := err != nil
			if tc.expectError != hasError {
				t.Error("unexpected version compatibility behavior")
			}
		})
	}
}

func podEvictionReactionTestingFnc(evictedPods *[]string) func(action core.Action) (bool, runtime.Object, error) {
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

func taintNodeNoSchedule(node *v1.Node) {
	node.Spec.Taints = []v1.Taint{
		{
			Key:    "key",
			Value:  "value",
			Effect: v1.TaintEffectNoSchedule,
		},
	}
}

func TestPodEvictorReset(t *testing.T) {
	initPluginRegistry()

	ctx := context.Background()
	node1 := test.BuildTestNode("n1", 2000, 3000, 10, taintNodeNoSchedule)
	node2 := test.BuildTestNode("n2", 2000, 3000, 10, nil)
	nodes := []*v1.Node{node1, node2}

	ownerRef1 := test.GetReplicaSetOwnerRefList()
	updatePod := func(pod *v1.Pod) {
		pod.Namespace = "dev"
		pod.ObjectMeta.OwnerReferences = ownerRef1
	}

	p1 := test.BuildTestPod("p1", 100, 0, node1.Name, updatePod)
	p2 := test.BuildTestPod("p2", 100, 0, node1.Name, updatePod)

	internalDeschedulerPolicy := removePodsViolatingNodeTaintsPolicy()
	ctxCancel, cancel := context.WithCancel(ctx)
	rs, descheduler, client := initDescheduler(t, ctxCancel, internalDeschedulerPolicy, node1, node2, p1, p2)
	defer cancel()

	var evictedPods []string
	client.PrependReactor("create", "pods", podEvictionReactionTestingFnc(&evictedPods))

	var fakeEvictedPods []string
	descheduler.podEvictionReactionFnc = func(*fakeclientset.Clientset) func(action core.Action) (bool, runtime.Object, error) {
		return podEvictionReactionTestingFnc(&fakeEvictedPods)
	}

	// a single pod eviction expected
	klog.Infof("2 pod eviction expected per a descheduling cycle, 2 real evictions in total")
	if err := descheduler.runDeschedulerLoop(ctx, nodes); err != nil {
		t.Fatalf("Unable to run a descheduling loop: %v", err)
	}
	if descheduler.podEvictor.TotalEvicted() != 2 || len(evictedPods) != 2 || len(fakeEvictedPods) != 0 {
		t.Fatalf("Expected (2,2,0) pods evicted, got (%v, %v, %v) instead", descheduler.podEvictor.TotalEvicted(), len(evictedPods), len(fakeEvictedPods))
	}

	// a single pod eviction expected
	klog.Infof("2 pod eviction expected per a descheduling cycle, 4 real evictions in total")
	if err := descheduler.runDeschedulerLoop(ctx, nodes); err != nil {
		t.Fatalf("Unable to run a descheduling loop: %v", err)
	}
	if descheduler.podEvictor.TotalEvicted() != 2 || len(evictedPods) != 4 || len(fakeEvictedPods) != 0 {
		t.Fatalf("Expected (2,4,0) pods evicted, got (%v, %v, %v) instead", descheduler.podEvictor.TotalEvicted(), len(evictedPods), len(fakeEvictedPods))
	}

	// check the fake client syncing and the right pods evicted
	klog.Infof("Enabling the dry run mode")
	rs.DryRun = true
	evictedPods = []string{}

	klog.Infof("2 pod eviction expected per a descheduling cycle, 2 fake evictions in total")
	if err := descheduler.runDeschedulerLoop(ctx, nodes); err != nil {
		t.Fatalf("Unable to run a descheduling loop: %v", err)
	}
	if descheduler.podEvictor.TotalEvicted() != 2 || len(evictedPods) != 0 || len(fakeEvictedPods) != 2 {
		t.Fatalf("Expected (2,0,2) pods evicted, got (%v, %v, %v) instead", descheduler.podEvictor.TotalEvicted(), len(evictedPods), len(fakeEvictedPods))
	}

	klog.Infof("2 pod eviction expected per a descheduling cycle, 4 fake evictions in total")
	if err := descheduler.runDeschedulerLoop(ctx, nodes); err != nil {
		t.Fatalf("Unable to run a descheduling loop: %v", err)
	}
	if descheduler.podEvictor.TotalEvicted() != 2 || len(evictedPods) != 0 || len(fakeEvictedPods) != 4 {
		t.Fatalf("Expected (2,0,4) pods evicted, got (%v, %v, %v) instead", descheduler.podEvictor.TotalEvicted(), len(evictedPods), len(fakeEvictedPods))
	}
}

func TestDeschedulingLimits(t *testing.T) {
	initPluginRegistry()

	tests := []struct {
		description string
		policy      *api.DeschedulerPolicy
		limit       uint
	}{
		{
			description: "limits per node",
			policy: func() *api.DeschedulerPolicy {
				policy := removePodsViolatingNodeTaintsPolicy()
				policy.MaxNoOfPodsToEvictPerNode = utilptr.To[uint](4)
				return policy
			}(),
			limit: uint(4),
		},
		{
			description: "limits per namespace",
			policy: func() *api.DeschedulerPolicy {
				policy := removePodsViolatingNodeTaintsPolicy()
				policy.MaxNoOfPodsToEvictPerNamespace = utilptr.To[uint](4)
				return policy
			}(),
			limit: uint(4),
		},
		{
			description: "limits per cycle",
			policy: func() *api.DeschedulerPolicy {
				policy := removePodsViolatingNodeTaintsPolicy()
				policy.MaxNoOfPodsToEvictTotal = utilptr.To[uint](4)
				return policy
			}(),
			limit: uint(4),
		},
	}

	ownerRef1 := test.GetReplicaSetOwnerRefList()
	updatePod := func(pod *v1.Pod) {
		pod.Namespace = "dev"
		pod.ObjectMeta.OwnerReferences = ownerRef1
	}

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			ctx := context.Background()
			node1 := test.BuildTestNode("n1", 2000, 3000, 10, taintNodeNoSchedule)
			node2 := test.BuildTestNode("n2", 2000, 3000, 10, nil)
			nodes := []*v1.Node{node1, node2}
			ctxCancel, cancel := context.WithCancel(ctx)
			_, descheduler, client := initDescheduler(t, ctxCancel, tc.policy, node1, node2)
			defer cancel()

			pods := []*v1.Pod{
				test.BuildTestPod("p1", 100, 0, node1.Name, updatePod),
				test.BuildTestPod("p2", 100, 0, node1.Name, updatePod),
				test.BuildTestPod("p3", 100, 0, node1.Name, updatePod),
				test.BuildTestPod("p4", 100, 0, node1.Name, updatePod),
				test.BuildTestPod("p5", 100, 0, node1.Name, updatePod),
			}

			for j := 0; j < 5; j++ {
				idx := j
				if _, err := client.CoreV1().Pods(pods[idx].Namespace).Create(context.TODO(), pods[idx], metav1.CreateOptions{}); err != nil {
					t.Fatalf("unable to create a pod: %v", err)
				}
				defer func() {
					if err := client.CoreV1().Pods(pods[idx].Namespace).Delete(context.TODO(), pods[idx].Name, metav1.DeleteOptions{}); err != nil {
						t.Fatalf("unable to delete a pod: %v", err)
					}
				}()
			}
			time.Sleep(100 * time.Millisecond)

			err := descheduler.runDeschedulerLoop(ctx, nodes)
			if err != nil {
				t.Fatalf("Unable to run a descheduling loop: %v", err)
			}
			totalEs := descheduler.podEvictor.TotalEvicted()
			if totalEs > tc.limit {
				t.Fatalf("Expected %v evictions in total, got %v instead", tc.limit, totalEs)
			}
			t.Logf("Total evictions: %v", totalEs)
		})
	}
}
