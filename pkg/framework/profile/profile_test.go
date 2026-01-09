package profile

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	v1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	fakeplugin "sigs.k8s.io/descheduler/pkg/framework/fake/plugin"
	"sigs.k8s.io/descheduler/pkg/framework/pluginregistry"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	frameworktesting "sigs.k8s.io/descheduler/pkg/framework/testing"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
	testutils "sigs.k8s.io/descheduler/test"
)

// registerDefaultEvictor registers the DefaultEvictor plugin with the given registry
func registerDefaultEvictor(registry pluginregistry.Registry) {
	pluginregistry.Register(
		defaultevictor.PluginName,
		defaultevictor.New,
		&defaultevictor.DefaultEvictor{},
		&defaultevictor.DefaultEvictorArgs{},
		defaultevictor.ValidateDefaultEvictorArgs,
		defaultevictor.SetDefaults_DefaultEvictorArgs,
		registry,
	)
}

func TestProfileDescheduleBalanceExtensionPointsEviction(t *testing.T) {
	// Helper to build profile config with default Filter and PreEvictionFilter
	buildProfileConfig := func(name string, descheduleEnabled, balanceEnabled bool) api.DeschedulerProfile {
		config := api.DeschedulerProfile{
			Name: name,
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
				Filter: api.PluginSet{
					Enabled: []string{defaultevictor.PluginName},
				},
				PreEvictionFilter: api.PluginSet{
					Enabled: []string{defaultevictor.PluginName},
				},
			},
		}
		if descheduleEnabled {
			config.Plugins.Deschedule = api.PluginSet{
				Enabled: []string{"FakePlugin"},
			}
		}
		if balanceEnabled {
			config.Plugins.Balance = api.PluginSet{
				Enabled: []string{"FakePlugin"},
			}
		}
		return config
	}

	tests := []struct {
		name             string
		config           api.DeschedulerProfile
		extensionPoint   frameworktypes.ExtensionPoint
		expectedEviction bool
	}{
		{
			name:             "profile with deschedule extension point enabled single eviction",
			config:           buildProfileConfig("strategy-test-profile-with-deschedule", true, false),
			extensionPoint:   frameworktypes.DescheduleExtensionPoint,
			expectedEviction: true,
		},
		{
			name:             "profile with balance extension point enabled single eviction",
			config:           buildProfileConfig("strategy-test-profile-with-balance", false, true),
			extensionPoint:   frameworktypes.BalanceExtensionPoint,
			expectedEviction: true,
		},
		{
			name:             "profile with deschedule extension point balance enabled no eviction",
			config:           buildProfileConfig("strategy-test-profile-with-balance", false, true),
			extensionPoint:   frameworktypes.DescheduleExtensionPoint,
			expectedEviction: false,
		},
		{
			name:             "profile with balance extension point deschedule enabled no eviction",
			config:           buildProfileConfig("strategy-test-profile-with-deschedule", true, false),
			extensionPoint:   frameworktypes.BalanceExtensionPoint,
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
			if test.extensionPoint == frameworktypes.DescheduleExtensionPoint {
				fakePlugin.AddReactor(string(frameworktypes.DescheduleExtensionPoint), func(action fakeplugin.Action) (handled, filter bool, err error) {
					if dAction, ok := action.(fakeplugin.DescheduleAction); ok {
						err := dAction.Handle().Evictor().Evict(ctx, p1, evictions.EvictOptions{StrategyName: fakePlugin.PluginName})
						if err == nil {
							return true, false, nil
						}
						return true, false, fmt.Errorf("pod not evicted: %v", err)
					}
					return false, false, nil
				})
			}
			if test.extensionPoint == frameworktypes.BalanceExtensionPoint {
				fakePlugin.AddReactor(string(frameworktypes.BalanceExtensionPoint), func(action fakeplugin.Action) (handled, filter bool, err error) {
					if dAction, ok := action.(fakeplugin.BalanceAction); ok {
						err := dAction.Handle().Evictor().Evict(ctx, p1, evictions.EvictOptions{StrategyName: fakePlugin.PluginName})
						if err == nil {
							return true, false, nil
						}
						return true, false, fmt.Errorf("pod not evicted: %v", err)
					}
					return false, false, nil
				})
			}

			pluginregistry.PluginRegistry = pluginregistry.NewRegistry()
			fakeplugin.RegisterFakePlugin("FakePlugin", &fakePlugin, pluginregistry.PluginRegistry)

			registerDefaultEvictor(pluginregistry.PluginRegistry)

			client := fakeclientset.NewSimpleClientset(n1, n2, p1)
			var evictedPods []string
			client.PrependReactor("create", "pods", podEvictionReactionFuc(&evictedPods))

			handle, podEvictor, err := frameworktesting.InitFrameworkHandle(
				ctx,
				client,
				nil,
				defaultevictor.DefaultEvictorArgs{},
				nil,
			)
			if err != nil {
				t.Fatalf("Unable to initialize a framework handle: %v", err)
			}

			prfl, err := NewProfile(
				ctx,
				test.config,
				pluginregistry.PluginRegistry,
				WithClientSet(client),
				WithSharedInformerFactory(handle.SharedInformerFactoryImpl),
				WithPodEvictor(podEvictor),
				WithGetPodsAssignedToNodeFnc(handle.GetPodsAssignedToNodeFuncImpl),
			)
			if err != nil {
				t.Fatalf("unable to create %q profile: %v", test.config.Name, err)
			}

			var status *frameworktypes.Status
			switch test.extensionPoint {
			case frameworktypes.DescheduleExtensionPoint:
				status = prfl.RunDeschedulePlugins(ctx, nodes)
			case frameworktypes.BalanceExtensionPoint:
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

func TestProfileExtensionPoints(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	n1 := testutils.BuildTestNode("n1", 2000, 3000, 10, nil)
	n2 := testutils.BuildTestNode("n2", 2000, 3000, 10, nil)

	p1 := testutils.BuildTestPod(fmt.Sprintf("pod_1_%s", n1.Name), 200, 0, n1.Name, nil)
	p1.ObjectMeta.OwnerReferences = []metav1.OwnerReference{{}}

	pluginregistry.PluginRegistry = pluginregistry.NewRegistry()

	for i := 0; i < 3; i++ {
		fakePluginName := fmt.Sprintf("FakePlugin_%v", i)
		deschedulePluginName := fmt.Sprintf("DeschedulePlugin_%v", i)
		balancePluginName := fmt.Sprintf("BalancePlugin_%v", i)
		filterPluginName := fmt.Sprintf("FilterPlugin_%v", i)

		fakePlugin := &fakeplugin.FakePlugin{PluginName: fakePluginName}
		fakeDeschedulePlugin := &fakeplugin.FakeDeschedulePlugin{PluginName: deschedulePluginName}
		fakeBalancePlugin := &fakeplugin.FakeBalancePlugin{PluginName: balancePluginName}
		fakeFilterPlugin := &fakeplugin.FakeFilterPlugin{PluginName: filterPluginName}

		fakeplugin.RegisterFakePlugin(fakePluginName, fakePlugin, pluginregistry.PluginRegistry)
		fakeplugin.RegisterFakeDeschedulePlugin(deschedulePluginName, fakeDeschedulePlugin, pluginregistry.PluginRegistry)
		fakeplugin.RegisterFakeBalancePlugin(balancePluginName, fakeBalancePlugin, pluginregistry.PluginRegistry)
		fakeplugin.RegisterFakeFilterPlugin(filterPluginName, fakeFilterPlugin, pluginregistry.PluginRegistry)
	}

	registerDefaultEvictor(pluginregistry.PluginRegistry)

	client := fakeclientset.NewSimpleClientset(n1, n2, p1)
	var evictedPods []string
	client.PrependReactor("create", "pods", podEvictionReactionFuc(&evictedPods))

	handle, podEvictor, err := frameworktesting.InitFrameworkHandle(
		ctx,
		client,
		nil,
		defaultevictor.DefaultEvictorArgs{},
		nil,
	)
	if err != nil {
		t.Fatalf("Unable to initialize a framework handle: %v", err)
	}

	prfl, err := NewProfile(
		ctx,
		api.DeschedulerProfile{
			Name: "strategy-test-profile",
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
					Name: "FakePlugin_0",
					Args: &fakeplugin.FakePluginArgs{},
				},
				{
					Name: "FilterPlugin_0",
					Args: &fakeplugin.FakeFilterPluginArgs{},
				},
				{
					Name: "FilterPlugin_1",
					Args: &fakeplugin.FakeFilterPluginArgs{},
				},
			},
			Plugins: api.Plugins{
				Deschedule: api.PluginSet{
					Enabled: []string{"FakePlugin_0"},
				},
				Filter: api.PluginSet{
					Enabled: []string{defaultevictor.PluginName, "FilterPlugin_1", "FilterPlugin_0"},
				},
				PreEvictionFilter: api.PluginSet{
					Enabled: []string{defaultevictor.PluginName},
				},
			},
		},
		pluginregistry.PluginRegistry,
		WithClientSet(client),
		WithSharedInformerFactory(handle.SharedInformerFactoryImpl),
		WithPodEvictor(podEvictor),
		WithGetPodsAssignedToNodeFnc(handle.GetPodsAssignedToNodeFuncImpl),
	)
	if err != nil {
		t.Fatalf("unable to create profile: %v", err)
	}

	// Validate the extension points of all registered plugins are properly detected

	diff := cmp.Diff(sets.New("DeschedulePlugin_0", "DeschedulePlugin_1", "DeschedulePlugin_2", "FakePlugin_0", "FakePlugin_1", "FakePlugin_2"), prfl.deschedule)
	if diff != "" {
		t.Errorf("check for deschedule failed. Results are not deep equal. mismatch (-want +got):\n%s", diff)
	}
	diff = cmp.Diff(sets.New("BalancePlugin_0", "BalancePlugin_1", "BalancePlugin_2", "FakePlugin_0", "FakePlugin_1", "FakePlugin_2"), prfl.balance)
	if diff != "" {
		t.Errorf("check for balance failed. Results are not deep equal. mismatch (-want +got):\n%s", diff)
	}
	diff = cmp.Diff(sets.New("DefaultEvictor", "FakePlugin_0", "FakePlugin_1", "FakePlugin_2", "FilterPlugin_0", "FilterPlugin_1", "FilterPlugin_2"), prfl.filter)
	if diff != "" {
		t.Errorf("check for filter failed. Results are not deep equal. mismatch (-want +got):\n%s", diff)
	}
	diff = cmp.Diff(sets.New("DefaultEvictor", "FakePlugin_0", "FakePlugin_1", "FakePlugin_2", "FilterPlugin_0", "FilterPlugin_1", "FilterPlugin_2"), prfl.preEvictionFilter)
	if diff != "" {
		t.Errorf("check for preEvictionFilter failed. Results are not deep equal. mismatch (-want +got):\n%s", diff)
	}

	// One deschedule ep enabled
	names := []string{}
	for _, pl := range prfl.deschedulePlugins {
		names = append(names, pl.Name())
	}
	sort.Strings(names)
	diff = cmp.Diff(sets.New("FakePlugin_0"), sets.New(names...))
	if diff != "" {
		t.Errorf("check for deschedule failed. Results are not deep equal. mismatch (-want +got):\n%s", diff)
	}

	// No balance ep enabled
	names = []string{}
	for _, pl := range prfl.balancePlugins {
		names = append(names, pl.Name())
	}
	sort.Strings(names)
	diff = cmp.Diff(sets.New[string](), sets.New(names...))
	if diff != "" {
		t.Errorf("check for balance failed. Results are not deep equal. mismatch (-want +got):\n%s", diff)
	}

	// Two filter eps enabled
	names = []string{}
	for _, pl := range prfl.filterPlugins {
		names = append(names, pl.Name())
	}
	sort.Strings(names)
	diff = cmp.Diff(sets.New("DefaultEvictor", "FilterPlugin_0", "FilterPlugin_1"), sets.New(names...))
	if diff != "" {
		t.Errorf("check for filter failed. Results are not deep equal. mismatch (-want +got):\n%s", diff)
	}
}

func TestProfileExtensionPointOrdering(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	n1 := testutils.BuildTestNode("n1", 2000, 3000, 10, nil)
	n2 := testutils.BuildTestNode("n2", 2000, 3000, 10, nil)
	nodes := []*v1.Node{n1, n2}

	p1 := testutils.BuildTestPod(fmt.Sprintf("pod_1_%s", n1.Name), 200, 0, n1.Name, nil)
	p1.ObjectMeta.OwnerReferences = []metav1.OwnerReference{{}}

	pluginregistry.PluginRegistry = pluginregistry.NewRegistry()

	filterInvocationOrder := []string{}
	preEvictionFilterInvocationOrder := []string{}
	descheduleInvocationOrder := []string{}
	balanceInvocationOrder := []string{}

	for i := 0; i < 3; i++ {
		pluginName := fmt.Sprintf("Filter_%v", i)
		fakeFilterPlugin := &fakeplugin.FakeFilterPlugin{PluginName: pluginName}
		fakeFilterPlugin.AddReactor(string(frameworktypes.FilterExtensionPoint), func(action fakeplugin.Action) (handled, filter bool, err error) {
			if _, ok := action.(fakeplugin.FilterAction); ok {
				filterInvocationOrder = append(filterInvocationOrder, pluginName+"_filter")
				return true, true, nil
			}
			return false, false, nil
		})

		fakeFilterPlugin.AddReactor(string(frameworktypes.PreEvictionFilterExtensionPoint), func(action fakeplugin.Action) (handled, filter bool, err error) {
			if _, ok := action.(fakeplugin.PreEvictionFilterAction); ok {
				preEvictionFilterInvocationOrder = append(preEvictionFilterInvocationOrder, pluginName+"_preEvictionFilter")
				return true, true, nil
			}
			return false, false, nil
		})

		// plugin implementing Filter extension point
		fakeplugin.RegisterFakeFilterPlugin(pluginName, fakeFilterPlugin, pluginregistry.PluginRegistry)

		fakePluginName := fmt.Sprintf("FakePlugin_%v", i)
		fakePlugin := fakeplugin.FakePlugin{}
		idx := i
		fakePlugin.AddReactor(string(frameworktypes.DescheduleExtensionPoint), func(action fakeplugin.Action) (handled, filter bool, err error) {
			descheduleInvocationOrder = append(descheduleInvocationOrder, fakePluginName)
			if idx == 0 {
				if dAction, ok := action.(fakeplugin.DescheduleAction); ok {
					// Invoke filters
					dAction.Handle().Evictor().Filter(p1)
					// Invoke pre-eviction filters
					dAction.Handle().Evictor().PreEvictionFilter(p1)
					return true, true, nil
				}
				return false, false, nil
			}
			return true, false, nil
		})

		fakePlugin.AddReactor(string(frameworktypes.BalanceExtensionPoint), func(action fakeplugin.Action) (handled, filter bool, err error) {
			balanceInvocationOrder = append(balanceInvocationOrder, fakePluginName)
			return true, false, nil
		})

		fakeplugin.RegisterFakePlugin(fakePluginName, &fakePlugin, pluginregistry.PluginRegistry)
	}

	registerDefaultEvictor(pluginregistry.PluginRegistry)

	client := fakeclientset.NewSimpleClientset(n1, n2, p1)
	var evictedPods []string
	client.PrependReactor("create", "pods", podEvictionReactionFuc(&evictedPods))

	handle, podEvictor, err := frameworktesting.InitFrameworkHandle(
		ctx,
		client,
		nil,
		defaultevictor.DefaultEvictorArgs{},
		nil,
	)
	if err != nil {
		t.Fatalf("Unable to initialize a framework handle: %v", err)
	}

	prfl, err := NewProfile(
		ctx,
		api.DeschedulerProfile{
			Name: "strategy-test-profile",
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
					Name: "FakePlugin_0",
					Args: &fakeplugin.FakePluginArgs{},
				},
				{
					Name: "FakePlugin_1",
					Args: &fakeplugin.FakePluginArgs{},
				},
				{
					Name: "FakePlugin_2",
					Args: &fakeplugin.FakePluginArgs{},
				},
				{
					Name: "Filter_0",
					Args: &fakeplugin.FakeFilterPluginArgs{},
				},
				{
					Name: "Filter_1",
					Args: &fakeplugin.FakeFilterPluginArgs{},
				},
				{
					Name: "Filter_2",
					Args: &fakeplugin.FakeFilterPluginArgs{},
				},
			},
			Plugins: api.Plugins{
				Deschedule: api.PluginSet{
					Enabled: []string{"FakePlugin_2", "FakePlugin_0", "FakePlugin_1"},
				},
				Balance: api.PluginSet{
					Enabled: []string{"FakePlugin_1", "FakePlugin_0", "FakePlugin_2"},
				},
				Filter: api.PluginSet{
					Enabled: []string{defaultevictor.PluginName, "Filter_2", "Filter_1", "Filter_0"},
				},
				PreEvictionFilter: api.PluginSet{
					Enabled: []string{defaultevictor.PluginName, "Filter_2", "Filter_1", "Filter_0"},
				},
			},
		},
		pluginregistry.PluginRegistry,
		WithClientSet(client),
		WithSharedInformerFactory(handle.SharedInformerFactoryImpl),
		WithPodEvictor(podEvictor),
		WithGetPodsAssignedToNodeFnc(handle.GetPodsAssignedToNodeFuncImpl),
	)
	if err != nil {
		t.Fatalf("unable to create profile: %v", err)
	}

	prfl.RunDeschedulePlugins(ctx, nodes)

	diff := cmp.Diff([]string{"Filter_2_filter", "Filter_1_filter", "Filter_0_filter"}, filterInvocationOrder)
	if diff != "" {
		t.Errorf("check for filter invocation order failed. Results are not deep equal. mismatch (-want +got):\n%s", diff)
	}

	diff = cmp.Diff([]string{"Filter_2_preEvictionFilter", "Filter_1_preEvictionFilter", "Filter_0_preEvictionFilter"}, preEvictionFilterInvocationOrder)
	if diff != "" {
		t.Errorf("check for filter invocation order failed. Results are not deep equal. mismatch (-want +got):\n%s", diff)
	}

	diff = cmp.Diff([]string{"FakePlugin_2", "FakePlugin_0", "FakePlugin_1"}, descheduleInvocationOrder)
	if diff != "" {
		t.Errorf("check for deschedule invocation order failed. Results are not deep equal. mismatch (-want +got):\n%s", diff)
	}

	prfl.RunBalancePlugins(ctx, nodes)

	diff = cmp.Diff([]string{"FakePlugin_1", "FakePlugin_0", "FakePlugin_2"}, balanceInvocationOrder)
	if diff != "" {
		t.Errorf("check for balance invocation order failed. Results are not deep equal. mismatch (-want +got):\n%s", diff)
	}
}

// verifyInstanceIDsMatch verifies that instance IDs captured at creation, deschedule, and balance match
func verifyInstanceIDsMatch(t *testing.T, profileInstanceID string, pluginNames []string, creationIDs, descheduleIDs, balanceIDs map[string]string) {
	for _, pluginName := range pluginNames {
		creationID := creationIDs[pluginName]
		descheduleID := descheduleIDs[pluginName]
		balanceID := balanceIDs[pluginName]

		if creationID == "" {
			t.Errorf("Profile %s, plugin %s: plugin creation did not capture instance ID", profileInstanceID, pluginName)
		}
		if descheduleID == "" {
			t.Errorf("Profile %s, plugin %s: deschedule extension point did not capture instance ID", profileInstanceID, pluginName)
		}
		if balanceID == "" {
			t.Errorf("Profile %s, plugin %s: balance extension point did not capture instance ID", profileInstanceID, pluginName)
		}

		// Verify all IDs match
		if creationID != descheduleID {
			t.Errorf("Profile %s, plugin %s: instance ID mismatch - creation: %s, deschedule: %s", profileInstanceID, pluginName, creationID, descheduleID)
		}
		if creationID != balanceID {
			t.Errorf("Profile %s, plugin %s: instance ID mismatch - creation: %s, balance: %s", profileInstanceID, pluginName, creationID, balanceID)
		}
		if descheduleID != balanceID {
			t.Errorf("Profile %s, plugin %s: instance ID mismatch - deschedule: %s, balance: %s", profileInstanceID, pluginName, descheduleID, balanceID)
		}
	}
}

// verifyInstanceIDFormat verifies that instance IDs have correct format and sequential indices
func verifyInstanceIDFormat(t *testing.T, profileInstanceID string, pluginNames []string, pluginIDs map[string]string) sets.Set[string] {
	if len(pluginIDs) != len(pluginNames) {
		t.Errorf("Profile %s: expected %d plugins to be invoked, got %d", profileInstanceID, len(pluginNames), len(pluginIDs))
	}

	// Collect all instance IDs for this profile
	profileInstanceIDs := sets.New[string]()
	for pluginName, instanceID := range pluginIDs {
		if instanceID == "" {
			t.Errorf("Profile %s, plugin %s: expected instance ID to be set, got empty string", profileInstanceID, pluginName)
		}
		profileInstanceIDs.Insert(instanceID)
	}

	// Verify all IDs within this profile are unique
	if profileInstanceIDs.Len() != len(pluginIDs) {
		t.Errorf("Profile %s: duplicate instance IDs found", profileInstanceID)
	}

	// Verify all IDs match the expected format: "{profileInstanceID}-{index}"
	// and contain sequential indices from 0 to n-1
	expectedIndices := sets.New[int]()
	for i := 0; i < len(pluginNames); i++ {
		expectedIndices.Insert(i)
	}
	actualIndices := sets.New[int]()
	for pluginName, instanceID := range pluginIDs {
		var idx int
		expectedPrefix := profileInstanceID + "-"
		if !strings.HasPrefix(instanceID, expectedPrefix) {
			t.Errorf("Profile %s, plugin %s: instance ID %s does not start with %s", profileInstanceID, pluginName, instanceID, expectedPrefix)
			continue
		}
		_, err := fmt.Sscanf(instanceID, profileInstanceID+"-%d", &idx)
		if err != nil {
			t.Errorf("Profile %s, plugin %s: instance ID %s does not match expected format", profileInstanceID, pluginName, instanceID)
			continue
		}
		actualIndices.Insert(idx)
	}
	// Verify we have indices 0 through n-1
	diff := cmp.Diff(expectedIndices, actualIndices)
	if diff != "" {
		t.Errorf("Profile %s: instance ID indices mismatch (-want +got):\n%s", profileInstanceID, diff)
	}

	return profileInstanceIDs
}

func TestPluginInstanceIDs(t *testing.T) {
	tests := []struct {
		name     string
		profiles []struct {
			profileInstanceID string
			pluginNames       []string
		}
	}{
		{
			name: "single plugin gets instance ID",
			profiles: []struct {
				profileInstanceID string
				pluginNames       []string
			}{
				{
					profileInstanceID: "0",
					pluginNames:       []string{"TestPlugin"},
				},
			},
		},
		{
			name: "two plugins get different instance IDs",
			profiles: []struct {
				profileInstanceID string
				pluginNames       []string
			}{
				{
					profileInstanceID: "0",
					pluginNames:       []string{"Plugin_0", "Plugin_1"},
				},
			},
		},
		{
			name: "three profiles with two plugins each get unique instance IDs",
			profiles: []struct {
				profileInstanceID string
				pluginNames       []string
			}{
				{
					profileInstanceID: "0",
					pluginNames:       []string{"Plugin_A", "Plugin_B"},
				},
				{
					profileInstanceID: "1",
					pluginNames:       []string{"Plugin_C", "Plugin_D"},
				},
				{
					profileInstanceID: "2",
					pluginNames:       []string{"Plugin_E", "Plugin_F"},
				},
			},
		},
		{
			name: "three profiles with same plugin names get different instance IDs per profile",
			profiles: []struct {
				profileInstanceID string
				pluginNames       []string
			}{
				{
					profileInstanceID: "0",
					pluginNames:       []string{"CommonPlugin_X", "CommonPlugin_Y"},
				},
				{
					profileInstanceID: "1",
					pluginNames:       []string{"CommonPlugin_X", "CommonPlugin_Y"},
				},
				{
					profileInstanceID: "2",
					pluginNames:       []string{"CommonPlugin_X", "CommonPlugin_Y"},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()

			n1 := testutils.BuildTestNode("n1", 2000, 3000, 10, nil)
			n2 := testutils.BuildTestNode("n2", 2000, 3000, 10, nil)
			nodes := []*v1.Node{n1, n2}

			// Track instance IDs by profile from different stages
			profileDescheduleIDs := make(map[string]map[string]string) // profileInstanceID -> pluginName -> instanceID (from Deschedule execution)
			profileBalanceIDs := make(map[string]map[string]string)    // profileInstanceID -> pluginName -> instanceID (from Balance execution)
			profileCreationIDs := make(map[string]map[string]string)   // profileInstanceID -> pluginName -> instanceID (from plugin creation)
			registry := pluginregistry.NewRegistry()

			// Collect all distinct plugin names across all profiles
			allPluginNames := sets.New[string]()
			for _, profileCfg := range test.profiles {
				allPluginNames.Insert(profileCfg.pluginNames...)
			}

			// Helper function to validate and store instance ID
			captureInstanceID := func(instanceID, pluginName string, targetMap map[string]map[string]string) {
				parts := strings.Split(instanceID, "-")
				if len(parts) < 2 {
					t.Fatalf("Plugin %s: instance ID %s does not have expected format 'profileID-index'", pluginName, instanceID)
				}
				profileID := parts[0]
				if targetMap[profileID] == nil {
					targetMap[profileID] = make(map[string]string)
				}
				targetMap[profileID][pluginName] = instanceID
			}

			// Register all plugins before creating profiles
			for _, pluginName := range allPluginNames.UnsortedList() {
				// Capture plugin name for closure
				name := pluginName

				pluginregistry.Register(
					pluginName,
					func(ctx context.Context, args runtime.Object, handle frameworktypes.Handle) (frameworktypes.Plugin, error) {
						fakePlugin := &fakeplugin.FakePlugin{PluginName: name}

						fakePlugin.AddReactor(string(frameworktypes.DescheduleExtensionPoint), func(action fakeplugin.Action) (handled, filter bool, err error) {
							if dAction, ok := action.(fakeplugin.DescheduleAction); ok {
								captureInstanceID(dAction.Handle().PluginInstanceID(), name, profileDescheduleIDs)
								return true, false, nil
							}
							return false, false, nil
						})

						fakePlugin.AddReactor(string(frameworktypes.BalanceExtensionPoint), func(action fakeplugin.Action) (handled, filter bool, err error) {
							if bAction, ok := action.(fakeplugin.BalanceAction); ok {
								captureInstanceID(bAction.Handle().PluginInstanceID(), name, profileBalanceIDs)
								return true, false, nil
							}
							return false, false, nil
						})

						// Use NewPluginFncFromFakeWithReactor to wrap and capture instance ID at creation
						builder := fakeplugin.NewPluginFncFromFakeWithReactor(fakePlugin, func(action fakeplugin.ActionImpl) {
							captureInstanceID(action.Handle().PluginInstanceID(), name, profileCreationIDs)
						})

						return builder(ctx, args, handle)
					},
					&fakeplugin.FakePlugin{},
					&fakeplugin.FakePluginArgs{},
					fakeplugin.ValidateFakePluginArgs,
					fakeplugin.SetDefaults_FakePluginArgs,
					registry,
				)
			}

			client := fakeclientset.NewSimpleClientset(n1, n2)
			handle, podEvictor, err := frameworktesting.InitFrameworkHandle(
				ctx,
				client,
				nil,
				defaultevictor.DefaultEvictorArgs{},
				nil,
			)
			if err != nil {
				t.Fatalf("Unable to initialize a framework handle: %v", err)
			}

			// Create all profiles
			var profiles []*profileImpl
			for _, profileCfg := range test.profiles {

				var pluginConfigs []api.PluginConfig
				for _, pluginName := range profileCfg.pluginNames {
					pluginConfigs = append(pluginConfigs, api.PluginConfig{
						Name: pluginName,
						Args: &fakeplugin.FakePluginArgs{},
					})
				}

				prfl, err := NewProfile(
					ctx,
					api.DeschedulerProfile{
						Name:          "test-profile",
						PluginConfigs: pluginConfigs,
						Plugins: api.Plugins{
							Deschedule: api.PluginSet{
								Enabled: profileCfg.pluginNames,
							},
							Balance: api.PluginSet{
								Enabled: profileCfg.pluginNames,
							},
						},
					},
					registry,
					WithClientSet(client),
					WithSharedInformerFactory(handle.SharedInformerFactoryImpl),
					WithPodEvictor(podEvictor),
					WithGetPodsAssignedToNodeFnc(handle.GetPodsAssignedToNodeFuncImpl),
					WithProfileInstanceID(profileCfg.profileInstanceID),
				)
				if err != nil {
					t.Fatalf("unable to create profile: %v", err)
				}
				profiles = append(profiles, prfl)
			}

			// Run deschedule and balance plugins for all profiles
			for _, prfl := range profiles {
				prfl.RunDeschedulePlugins(ctx, nodes)
				prfl.RunBalancePlugins(ctx, nodes)
			}

			// Verify creation, deschedule, and balance IDs all match
			for _, profileCfg := range test.profiles {
				verifyInstanceIDsMatch(
					t,
					profileCfg.profileInstanceID,
					profileCfg.pluginNames,
					profileCreationIDs[profileCfg.profileInstanceID],
					profileDescheduleIDs[profileCfg.profileInstanceID],
					profileBalanceIDs[profileCfg.profileInstanceID],
				)
			}

			// Verify all plugins were invoked and have correct instance IDs
			allInstanceIDs := sets.New[string]()
			for _, profileCfg := range test.profiles {
				profileInstanceIDs := verifyInstanceIDFormat(
					t,
					profileCfg.profileInstanceID,
					profileCfg.pluginNames,
					profileDescheduleIDs[profileCfg.profileInstanceID],
				)
				allInstanceIDs = allInstanceIDs.Union(profileInstanceIDs)
			}

			// Verify all instance IDs are unique across all profiles
			totalExpectedPlugins := 0
			for _, profileCfg := range test.profiles {
				totalExpectedPlugins += len(profileCfg.pluginNames)
			}
			if allInstanceIDs.Len() != totalExpectedPlugins {
				t.Errorf("Expected %d unique instance IDs across all profiles, got %d", totalExpectedPlugins, allInstanceIDs.Len())
			}
		})
	}
}
