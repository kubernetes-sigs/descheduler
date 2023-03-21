package profile

import (
	"context"
	"fmt"
	"testing"

	v1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/framework"
	fakeplugin "sigs.k8s.io/descheduler/pkg/framework/fake/plugin"
	"sigs.k8s.io/descheduler/pkg/framework/pluginregistry"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	"sigs.k8s.io/descheduler/pkg/utils"
	testutils "sigs.k8s.io/descheduler/test"
)

func TestProfileTopExtensionPoints(t *testing.T) {
	tests := []struct {
		name             string
		config           api.Profile
		extensionPoint   framework.ExtensionPoint
		expectedEviction bool
	}{
		{
			name: "profile with deschedule extension point enabled single eviction",
			config: api.Profile{
				Name: "strategy-test-profile-with-deschedule",
				PluginConfigs: []api.PluginConfig{
					{
						Name: defaultevictor.PluginName,
						Args: &defaultevictor.DefaultEvictorArgs{
							PriorityThreshold: &api.PriorityThreshold{
								Value: nil,
							},
						},
					},
					{
						Name: "FakePlugin",
						Args: &fakeplugin.FakePluginArgs{},
					},
				},
				Plugins: api.Plugins{
					Deschedule: api.PluginSet{
						Enabled: []string{"FakePlugin"},
					},
					Evict: api.PluginSet{
						Enabled: []string{defaultevictor.PluginName},
					},
				},
			},
			extensionPoint:   framework.DescheduleExtensionPoint,
			expectedEviction: true,
		},
		{
			name: "profile with balance extension point enabled single eviction",
			config: api.Profile{
				Name: "strategy-test-profile-with-balance",
				PluginConfigs: []api.PluginConfig{
					{
						Name: defaultevictor.PluginName,
						Args: &defaultevictor.DefaultEvictorArgs{
							PriorityThreshold: &api.PriorityThreshold{
								Value: nil,
							},
						},
					},
					{
						Name: "FakePlugin",
						Args: &fakeplugin.FakePluginArgs{},
					},
				},
				Plugins: api.Plugins{
					Balance: api.PluginSet{
						Enabled: []string{"FakePlugin"},
					},
					Evict: api.PluginSet{
						Enabled: []string{defaultevictor.PluginName},
					},
				},
			},
			extensionPoint:   framework.BalanceExtensionPoint,
			expectedEviction: true,
		},
		{
			name: "profile with deschedule extension point balance enabled no eviction",
			config: api.Profile{
				Name: "strategy-test-profile-with-deschedule",
				PluginConfigs: []api.PluginConfig{
					{
						Name: defaultevictor.PluginName,
						Args: &defaultevictor.DefaultEvictorArgs{
							PriorityThreshold: &api.PriorityThreshold{
								Value: nil,
							},
						},
					},
					{
						Name: "FakePlugin",
						Args: &fakeplugin.FakePluginArgs{},
					},
				},
				Plugins: api.Plugins{
					Balance: api.PluginSet{
						Enabled: []string{"FakePlugin"},
					},
					Evict: api.PluginSet{
						Enabled: []string{defaultevictor.PluginName},
					},
				},
			},
			extensionPoint:   framework.DescheduleExtensionPoint,
			expectedEviction: false,
		},
		{
			name: "profile with balance extension point deschedule enabled no eviction",
			config: api.Profile{
				Name: "strategy-test-profile-with-deschedule",
				PluginConfigs: []api.PluginConfig{
					{
						Name: defaultevictor.PluginName,
						Args: &defaultevictor.DefaultEvictorArgs{
							PriorityThreshold: &api.PriorityThreshold{
								Value: nil,
							},
						},
					},
					{
						Name: "FakePlugin",
						Args: &fakeplugin.FakePluginArgs{},
					},
				},
				Plugins: api.Plugins{
					Deschedule: api.PluginSet{
						Enabled: []string{"FakePlugin"},
					},
					Evict: api.PluginSet{
						Enabled: []string{defaultevictor.PluginName},
					},
				},
			},
			extensionPoint:   framework.BalanceExtensionPoint,
			expectedEviction: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()

			n1 := testutils.BuildTestNode("n1", 2000, 3000, 10, nil)
			n2 := testutils.BuildTestNode("n2", 2000, 3000, 10, nil)
			nodes := []*v1.Node{n1, n2}

			p1 := testutils.BuildTestPod(fmt.Sprintf("pod_1_%s", n1.Name), 200, 0, n1.Name, nil)
			p1.ObjectMeta.OwnerReferences = []metav1.OwnerReference{{}}

			fakePlugin := fakeplugin.FakePlugin{}
			if test.extensionPoint == framework.DescheduleExtensionPoint {
				fakePlugin.AddReactor(string(framework.DescheduleExtensionPoint), func(action fakeplugin.Action) (handled bool, err error) {
					if dAction, ok := action.(fakeplugin.DescheduleAction); ok {
						if dAction.Handle().Evictor().Evict(ctx, p1, evictions.EvictOptions{}) {
							return true, nil
						}
						return true, fmt.Errorf("pod not evicted")
					}
					return false, nil
				})
			}
			if test.extensionPoint == framework.BalanceExtensionPoint {
				fakePlugin.AddReactor(string(framework.BalanceExtensionPoint), func(action fakeplugin.Action) (handled bool, err error) {
					if dAction, ok := action.(fakeplugin.BalanceAction); ok {
						if dAction.Handle().Evictor().Evict(ctx, p1, evictions.EvictOptions{}) {
							return true, nil
						}
						return true, fmt.Errorf("pod not evicted")
					}
					return false, nil
				})
			}

			pluginregistry.PluginRegistry = pluginregistry.NewRegistry()
			pluginregistry.Register(
				"FakePlugin",
				fakeplugin.NewPluginFncFromFake(&fakePlugin),
				&fakeplugin.FakePluginArgs{},
				fakeplugin.ValidateFakePluginArgs,
				fakeplugin.SetDefaults_FakePluginArgs,
				pluginregistry.PluginRegistry,
			)

			pluginregistry.Register(
				defaultevictor.PluginName,
				defaultevictor.New,
				&defaultevictor.DefaultEvictorArgs{},
				defaultevictor.ValidateDefaultEvictorArgs,
				defaultevictor.SetDefaults_DefaultEvictorArgs,
				pluginregistry.PluginRegistry,
			)

			client := fakeclientset.NewSimpleClientset(n1, n2, p1)
			var evictedPods []string
			client.PrependReactor("create", "pods", podEvictionReactionFuc(&evictedPods))

			sharedInformerFactory := informers.NewSharedInformerFactory(client, 0)
			podInformer := sharedInformerFactory.Core().V1().Pods().Informer()
			getPodsAssignedToNode, err := podutil.BuildGetPodsAssignedToNodeFunc(podInformer)
			if err != nil {
				t.Fatalf("build get pods assigned to node function error: %v", err)
			}

			sharedInformerFactory.Start(ctx.Done())
			sharedInformerFactory.WaitForCacheSync(ctx.Done())

			eventClient := fakeclientset.NewSimpleClientset(n1, n2, p1)
			eventBroadcaster, eventRecorder := utils.GetRecorderAndBroadcaster(ctx, eventClient)
			defer eventBroadcaster.Shutdown()

			podEvictor := evictions.NewPodEvictor(client, "policy/v1", false, nil, nil, nodes, true, eventRecorder)

			prfl, err := NewProfile(
				test.config,
				pluginregistry.PluginRegistry,
				WithClientSet(client),
				WithSharedInformerFactory(sharedInformerFactory),
				WithPodEvictor(podEvictor),
				WithGetPodsAssignedToNodeFnc(getPodsAssignedToNode),
			)
			if err != nil {
				t.Fatalf("unable to create %q profile: %v", test.config.Name, err)
			}

			var status *framework.Status
			switch test.extensionPoint {
			case framework.DescheduleExtensionPoint:
				status = prfl.RunDeschedulePlugins(ctx, nodes)
			case framework.BalanceExtensionPoint:
				status = prfl.RunBalancePlugins(ctx, nodes)
			default:
				t.Fatalf("unknown %q extension point", test.extensionPoint)
			}

			if status == nil {
				t.Fatalf("Unexpected nil status")
			}
			if status.Err != nil {
				t.Fatalf("Expected nil error in status, got %q instead", status.Err)
			}

			if test.expectedEviction && len(evictedPods) < 1 {
				t.Fatalf("Expected eviction, got none")
			}
			if !test.expectedEviction && len(evictedPods) > 0 {
				t.Fatalf("Unexpected eviction, expected none")
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
