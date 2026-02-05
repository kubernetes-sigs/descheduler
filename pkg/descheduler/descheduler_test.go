package descheduler

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	promapi "github.com/prometheus/client_golang/api"
	v1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	apiversion "k8s.io/apimachinery/pkg/version"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/informers"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/component-base/featuregate"
	"k8s.io/klog/v2"
	"k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
	fakemetricsclient "k8s.io/metrics/pkg/client/clientset/versioned/fake"
	utilptr "k8s.io/utils/ptr"

	"sigs.k8s.io/descheduler/cmd/descheduler/app/options"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	"sigs.k8s.io/descheduler/pkg/features"
	fakeplugin "sigs.k8s.io/descheduler/pkg/framework/fake/plugin"
	"sigs.k8s.io/descheduler/pkg/framework/pluginregistry"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/nodeutilization"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removeduplicates"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/removepodsviolatingnodetaints"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
	"sigs.k8s.io/descheduler/pkg/utils"
	deschedulerversion "sigs.k8s.io/descheduler/pkg/version"
	"sigs.k8s.io/descheduler/test"
)

type mockPrometheusClient struct {
	name string
}

func (m *mockPrometheusClient) URL(ep string, args map[string]string) *url.URL {
	return nil
}

func (m *mockPrometheusClient) Do(ctx context.Context, req *http.Request) (*http.Response, []byte, error) {
	return nil, nil, nil
}

var _ promapi.Client = &mockPrometheusClient{}

var (
	podEvictionError     = errors.New("PodEvictionError")
	tooManyRequestsError = &apierrors.StatusError{
		ErrStatus: metav1.Status{
			Status:  metav1.StatusFailure,
			Code:    http.StatusTooManyRequests,
			Reason:  metav1.StatusReasonTooManyRequests,
			Message: "admission webhook \"virt-launcher-eviction-interceptor.kubevirt.io\" denied the request: Eviction triggered evacuation of VMI",
		},
	}
	nodesgvr = schema.GroupVersionResource{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "nodes"}
	podsgvr  = schema.GroupVersionResource{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "pods"}
)

func initFeatureGates() featuregate.FeatureGate {
	featureGates := featuregate.NewFeatureGate()
	featureGates.Add(map[featuregate.Feature]featuregate.FeatureSpec{
		features.EvictionsInBackground: {Default: false, PreRelease: featuregate.Alpha},
	})
	return featureGates
}

func initPluginRegistry() {
	pluginregistry.PluginRegistry = pluginregistry.NewRegistry()
	pluginregistry.Register(removeduplicates.PluginName, removeduplicates.New, &removeduplicates.RemoveDuplicates{}, &removeduplicates.RemoveDuplicatesArgs{}, removeduplicates.ValidateRemoveDuplicatesArgs, removeduplicates.SetDefaults_RemoveDuplicatesArgs, pluginregistry.PluginRegistry)
	pluginregistry.Register(defaultevictor.PluginName, defaultevictor.New, &defaultevictor.DefaultEvictor{}, &defaultevictor.DefaultEvictorArgs{}, defaultevictor.ValidateDefaultEvictorArgs, defaultevictor.SetDefaults_DefaultEvictorArgs, pluginregistry.PluginRegistry)
	pluginregistry.Register(removepodsviolatingnodetaints.PluginName, removepodsviolatingnodetaints.New, &removepodsviolatingnodetaints.RemovePodsViolatingNodeTaints{}, &removepodsviolatingnodetaints.RemovePodsViolatingNodeTaintsArgs{}, removepodsviolatingnodetaints.ValidateRemovePodsViolatingNodeTaintsArgs, removepodsviolatingnodetaints.SetDefaults_RemovePodsViolatingNodeTaintsArgs, pluginregistry.PluginRegistry)
	pluginregistry.Register(nodeutilization.LowNodeUtilizationPluginName, nodeutilization.NewLowNodeUtilization, &nodeutilization.LowNodeUtilization{}, &nodeutilization.LowNodeUtilizationArgs{}, nodeutilization.ValidateLowNodeUtilizationArgs, nodeutilization.SetDefaults_LowNodeUtilizationArgs, pluginregistry.PluginRegistry)
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

func lowNodeUtilizationPolicy(thresholds, targetThresholds api.ResourceThresholds, metricsEnabled bool) *api.DeschedulerPolicy {
	var metricsSource api.MetricsSource = ""
	if metricsEnabled {
		metricsSource = api.KubernetesMetrics
	}
	return &api.DeschedulerPolicy{
		Profiles: []api.DeschedulerProfile{
			{
				Name: "Profile",
				PluginConfigs: []api.PluginConfig{
					{
						Name: nodeutilization.LowNodeUtilizationPluginName,
						Args: &nodeutilization.LowNodeUtilizationArgs{
							Thresholds:       thresholds,
							TargetThresholds: targetThresholds,
							MetricsUtilization: &nodeutilization.MetricsUtilization{
								Source: metricsSource,
							},
						},
					},
					{
						Name: defaultevictor.PluginName,
						Args: &defaultevictor.DefaultEvictorArgs{},
					},
				},
				Plugins: api.Plugins{
					Filter: api.PluginSet{
						Enabled: []string{
							defaultevictor.PluginName,
						},
					},
					Balance: api.PluginSet{
						Enabled: []string{
							nodeutilization.LowNodeUtilizationPluginName,
						},
					},
				},
			},
		},
	}
}

func initDescheduler(t *testing.T, ctx context.Context, featureGates featuregate.FeatureGate, internalDeschedulerPolicy *api.DeschedulerPolicy, metricsClient metricsclient.Interface, dryRun bool, objects ...runtime.Object) (*options.DeschedulerServer, *descheduler, runFncType, *fakeclientset.Clientset) {
	client := fakeclientset.NewSimpleClientset(objects...)
	eventClient := fakeclientset.NewSimpleClientset(objects...)

	rs, err := options.NewDeschedulerServer()
	if err != nil {
		t.Fatalf("Unable to initialize server: %v", err)
	}
	rs.Client = client
	rs.EventClient = eventClient
	rs.DefaultFeatureGates = featureGates
	rs.MetricsClient = metricsClient
	rs.DryRun = dryRun

	sharedInformerFactory := informers.NewSharedInformerFactoryWithOptions(rs.Client, 0, informers.WithTransform(trimManagedFields))
	eventBroadcaster, eventRecorder := utils.GetRecorderAndBroadcaster(ctx, client)

	var namespacedSharedInformerFactory informers.SharedInformerFactory
	prometheusConfig := getPrometheusConfig(internalDeschedulerPolicy.MetricsProviders)
	if prometheusConfig != nil && configureSecretPromClientReconciler(prometheusConfig) {
		namespacedSharedInformerFactory = informers.NewSharedInformerFactoryWithOptions(rs.Client, 0, informers.WithTransform(trimManagedFields), informers.WithNamespace(prometheusConfig.AuthToken.SecretReference.Namespace))
	}

	// Always create descheduler with real client/factory first to register all informers
	descheduler, runFnc, err := bootstrapDescheduler(ctx, rs, internalDeschedulerPolicy, "v1", sharedInformerFactory, namespacedSharedInformerFactory, eventRecorder)
	if err != nil {
		eventBroadcaster.Shutdown()
		t.Fatalf("Failed to bootstrap a descheduler: %v", err)
	}

	if dryRun {
		if err := wait.PollUntilContextTimeout(ctx, 100*time.Millisecond, 5*time.Second, true, func(ctx context.Context) (bool, error) {
			for _, obj := range objects {
				// Only check for nodes - secrets are handled by namespacedSharedInformerFactory
				if _, ok := obj.(*v1.Node); !ok {
					continue
				}
				exists, err := descheduler.kubeClientSandbox.hasRuntimeObjectInIndexer(obj)
				if err != nil {
					return false, err
				}
				metaObj, err := meta.Accessor(obj)
				if err != nil {
					return false, fmt.Errorf("failed to get object metadata: %w", err)
				}
				key := cache.MetaObjectToName(metaObj).String()
				if !exists {
					klog.Infof("Object %q has not propagated to the indexer", key)
					return false, nil
				}
				klog.Infof("Object %q has propagated to the indexer", key)
			}
			return true, nil
		}); err != nil {
			t.Fatalf("nodes did not propagate to the indexer: %v", err)
		}
	}

	return rs, descheduler, runFnc, client
}

func waitForSecretsPropagation(ctx context.Context, secretsLister corev1listers.SecretNamespaceLister, objects []runtime.Object) error {
	return wait.PollUntilContextTimeout(ctx, 100*time.Millisecond, 5*time.Second, true, func(ctx context.Context) (bool, error) {
		for _, obj := range objects {
			secret, ok := obj.(*v1.Secret)
			if !ok {
				continue
			}
			_, err := secretsLister.Get(secret.Name)
			if err != nil {
				if apierrors.IsNotFound(err) {
					klog.Infof("Secret %s/%s has not propagated to the indexer", secret.Namespace, secret.Name)
					return false, nil
				}
				return false, err
			}
			klog.Infof("Secret %s/%s has propagated to the indexer", secret.Namespace, secret.Name)
		}
		return true, nil
	})
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
	rs.DefaultFeatureGates = initFeatureGates()

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
	client.PrependReactor("create", "pods", podEvictionReactionTestingFnc(&evictedPods, nil, nil))

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
	rs.DefaultFeatureGates = initFeatureGates()

	pods, err := client.CoreV1().Pods(p1.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Errorf("Unable to list pods: %v", err)
	}

	if len(pods.Items) != 3 {
		t.Errorf("Pods number should be 3 before evict")
	}

	var evictedPods []string
	client.PrependReactor("create", "pods", podEvictionReactionTestingFnc(&evictedPods, nil, nil))

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
	rs.DefaultFeatureGates = initFeatureGates()
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
	rs.DefaultFeatureGates = initFeatureGates()
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

func podEvictionReactionTestingFnc(evictedPods *[]string, isEvictionsInBackground func(podName string) bool, evictionErr error) func(action core.Action) (bool, runtime.Object, error) {
	return func(action core.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() == "eviction" {
			createAct, matched := action.(core.CreateActionImpl)
			if !matched {
				return false, nil, fmt.Errorf("unable to convert action to core.CreateActionImpl")
			}
			if eviction, matched := createAct.Object.(*policy.Eviction); matched {
				if isEvictionsInBackground != nil && isEvictionsInBackground(eviction.GetName()) {
					return true, nil, tooManyRequestsError
				}
				if evictionErr != nil {
					return true, nil, evictionErr
				}
				*evictedPods = append(*evictedPods, eviction.GetName())
				return true, nil, nil
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

	tests := []struct {
		name   string
		dryRun bool
		cycles []struct {
			expectedTotalEvicted  uint
			expectedRealEvictions int
			expectedFakeEvictions int
		}
	}{
		{
			name:   "real mode",
			dryRun: false,
			cycles: []struct {
				expectedTotalEvicted  uint
				expectedRealEvictions int
				expectedFakeEvictions int
			}{
				{expectedTotalEvicted: 2, expectedRealEvictions: 2, expectedFakeEvictions: 0},
				{expectedTotalEvicted: 2, expectedRealEvictions: 4, expectedFakeEvictions: 0},
			},
		},
		{
			name:   "dry mode",
			dryRun: true,
			cycles: []struct {
				expectedTotalEvicted  uint
				expectedRealEvictions int
				expectedFakeEvictions int
			}{
				{expectedTotalEvicted: 2, expectedRealEvictions: 0, expectedFakeEvictions: 2},
				{expectedTotalEvicted: 2, expectedRealEvictions: 0, expectedFakeEvictions: 4},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			node1 := test.BuildTestNode("n1", 2000, 3000, 10, taintNodeNoSchedule)
			node2 := test.BuildTestNode("n2", 2000, 3000, 10, nil)

			p1 := test.BuildTestPod("p1", 100, 0, node1.Name, test.SetRSOwnerRef)
			p2 := test.BuildTestPod("p2", 100, 0, node1.Name, test.SetRSOwnerRef)

			internalDeschedulerPolicy := removePodsViolatingNodeTaintsPolicy()
			ctxCancel, cancel := context.WithCancel(ctx)
			_, descheduler, _, client := initDescheduler(t, ctxCancel, initFeatureGates(), internalDeschedulerPolicy, nil, tc.dryRun, node1, node2, p1, p2)
			defer cancel()

			var evictedPods []string
			client.PrependReactor("create", "pods", podEvictionReactionTestingFnc(&evictedPods, nil, nil))

			var fakeEvictedPods []string
			for i, cycle := range tc.cycles {
				evictedPodNames := runDeschedulerLoopAndGetEvictedPods(ctx, t, descheduler, tc.dryRun)
				fakeEvictedPods = append(fakeEvictedPods, evictedPodNames...)

				if descheduler.podEvictor.TotalEvicted() != cycle.expectedTotalEvicted || len(evictedPods) != cycle.expectedRealEvictions || len(fakeEvictedPods) != cycle.expectedFakeEvictions {
					t.Fatalf("Cycle %d: Expected (%v,%v,%v) pods evicted, got (%v,%v,%v) instead", i+1, cycle.expectedTotalEvicted, cycle.expectedRealEvictions, cycle.expectedFakeEvictions, descheduler.podEvictor.TotalEvicted(), len(evictedPods), len(fakeEvictedPods))
				}
			}
		})
	}
}

// runDeschedulerLoopAndGetEvictedPods runs a descheduling cycle and returns the names of evicted pods.
// This is similar to runDeschedulerLoop but captures evicted pod names before the cache is reset.
func runDeschedulerLoopAndGetEvictedPods(ctx context.Context, t *testing.T, d *descheduler, dryRun bool) []string {
	d.podEvictor.ResetCounters()

	d.runProfiles(ctx)

	var evictedPodNames []string
	if dryRun {
		evictedPodsFromCache := d.kubeClientSandbox.evictedPodsCache.list()
		for _, pod := range evictedPodsFromCache {
			evictedPodNames = append(evictedPodNames, pod.Name)
		}

		if err := d.kubeClientSandbox.restoreEvictedPods(ctx); err != nil {
			t.Fatalf("Failed to restore evicted pods: %v", err)
		}
		d.kubeClientSandbox.reset()
	}

	return evictedPodNames
}

func checkTotals(t *testing.T, ctx context.Context, descheduler *descheduler, totalEvictionRequests, totalEvicted uint) {
	if total := descheduler.podEvictor.TotalEvictionRequests(); total != totalEvictionRequests {
		t.Fatalf("Expected %v total eviction requests, got %v instead", totalEvictionRequests, total)
	}
	if total := descheduler.podEvictor.TotalEvicted(); total != totalEvicted {
		t.Fatalf("Expected %v total evictions, got %v instead", totalEvicted, total)
	}
	t.Logf("Total evictions: %v, total eviction requests: %v, total evictions and eviction requests: %v", totalEvicted, totalEvictionRequests, totalEvicted+totalEvictionRequests)
}

func runDeschedulingCycleAndCheckTotals(t *testing.T, ctx context.Context, nodes []*v1.Node, descheduler *descheduler, runFnc runFncType, totalEvictionRequests, totalEvicted uint) {
	err := runFnc(ctx)
	if err != nil {
		t.Fatalf("Unable to run a descheduling loop: %v", err)
	}
	checkTotals(t, ctx, descheduler, totalEvictionRequests, totalEvicted)
}

func TestEvictionRequestsCache(t *testing.T) {
	initPluginRegistry()

	ctx := context.Background()
	node1 := test.BuildTestNode("n1", 2000, 3000, 10, taintNodeNoSchedule)
	node2 := test.BuildTestNode("n2", 2000, 3000, 10, nil)
	nodes := []*v1.Node{node1, node2}

	ownerRef1 := test.GetReplicaSetOwnerRefList()
	updatePod := func(pod *v1.Pod) {
		pod.Namespace = "dev"
		pod.ObjectMeta.OwnerReferences = ownerRef1
		pod.Status.Phase = v1.PodRunning
	}
	updatePodWithEvictionInBackground := func(pod *v1.Pod) {
		updatePod(pod)
		pod.Annotations = map[string]string{
			evictions.EvictionRequestAnnotationKey: "",
		}
	}

	p1 := test.BuildTestPod("p1", 100, 0, node1.Name, updatePodWithEvictionInBackground)
	p2 := test.BuildTestPod("p2", 100, 0, node1.Name, updatePodWithEvictionInBackground)
	p3 := test.BuildTestPod("p3", 100, 0, node1.Name, updatePod)
	p4 := test.BuildTestPod("p4", 100, 0, node1.Name, updatePod)
	p5 := test.BuildTestPod("p5", 100, 0, node1.Name, updatePod)

	internalDeschedulerPolicy := removePodsViolatingNodeTaintsPolicy()
	ctxCancel, cancel := context.WithCancel(ctx)
	featureGates := featuregate.NewFeatureGate()
	featureGates.Add(map[featuregate.Feature]featuregate.FeatureSpec{
		features.EvictionsInBackground: {Default: true, PreRelease: featuregate.Alpha},
	})
	_, descheduler, runFnc, client := initDescheduler(t, ctxCancel, featureGates, internalDeschedulerPolicy, nil, false, node1, node2, p1, p2, p3, p4)
	defer cancel()

	var evictedPods []string
	client.PrependReactor("create", "pods", podEvictionReactionTestingFnc(&evictedPods, func(name string) bool { return name == "p1" || name == "p2" }, nil))

	klog.Infof("2 evictions in background expected, 2 normal evictions")
	runDeschedulingCycleAndCheckTotals(t, ctx, nodes, descheduler, runFnc, 2, 2)

	klog.Infof("Repeat the same as previously to confirm no more evictions in background are requested")
	// No evicted pod is actually deleted on purpose so the test can run the descheduling cycle repeatedly
	// without recreating the pods.
	runDeschedulingCycleAndCheckTotals(t, ctx, nodes, descheduler, runFnc, 2, 2)

	klog.Infof("Scenario: Eviction in background got initiated")
	p2.Annotations[evictions.EvictionInProgressAnnotationKey] = ""
	if _, err := client.CoreV1().Pods(p2.Namespace).Update(context.TODO(), p2, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("unable to update a pod: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	klog.Infof("Repeat the same as previously to confirm no more evictions in background are requested")
	runDeschedulingCycleAndCheckTotals(t, ctx, nodes, descheduler, runFnc, 2, 2)

	klog.Infof("Scenario: Another eviction in background got initiated")
	p1.Annotations[evictions.EvictionInProgressAnnotationKey] = ""
	if _, err := client.CoreV1().Pods(p1.Namespace).Update(context.TODO(), p1, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("unable to update a pod: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	klog.Infof("Repeat the same as previously to confirm no more evictions in background are requested")
	runDeschedulingCycleAndCheckTotals(t, ctx, nodes, descheduler, runFnc, 2, 2)

	klog.Infof("Scenario: Eviction in background completed")
	if err := client.CoreV1().Pods(p1.Namespace).Delete(context.TODO(), p1.Name, metav1.DeleteOptions{}); err != nil {
		t.Fatalf("unable to delete a pod: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	klog.Infof("Check the number of evictions in background decreased")
	runDeschedulingCycleAndCheckTotals(t, ctx, nodes, descheduler, runFnc, 1, 2)

	klog.Infof("Scenario: A new pod without eviction in background added")
	if _, err := client.CoreV1().Pods(p5.Namespace).Create(context.TODO(), p5, metav1.CreateOptions{}); err != nil {
		t.Fatalf("unable to create a pod: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	klog.Infof("Check the number of evictions increased after running a descheduling cycle")
	runDeschedulingCycleAndCheckTotals(t, ctx, nodes, descheduler, runFnc, 1, 3)

	klog.Infof("Scenario: Eviction in background canceled => eviction in progress annotation removed")
	delete(p2.Annotations, evictions.EvictionInProgressAnnotationKey)
	if _, err := client.CoreV1().Pods(p2.Namespace).Update(context.TODO(), p2, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("unable to update a pod: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	klog.Infof("Check the number of evictions in background decreased")
	checkTotals(t, ctx, descheduler, 0, 3)

	klog.Infof("Scenario: Re-run the descheduling cycle to re-request eviction in background")
	runDeschedulingCycleAndCheckTotals(t, ctx, nodes, descheduler, runFnc, 1, 3)

	klog.Infof("Scenario: Eviction in background completed with a pod in completed state")
	p2.Status.Phase = v1.PodSucceeded
	if _, err := client.CoreV1().Pods(p2.Namespace).Update(context.TODO(), p2, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("unable to delete a pod: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	klog.Infof("Check the number of evictions in background decreased")
	runDeschedulingCycleAndCheckTotals(t, ctx, nodes, descheduler, runFnc, 0, 3)
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

	updatePodWithEvictionInBackground := func(pod *v1.Pod) {
		updatePod(pod)
		pod.Annotations = map[string]string{
			evictions.EvictionRequestAnnotationKey: "",
		}
	}

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			ctx := context.Background()
			node1 := test.BuildTestNode("n1", 2000, 3000, 10, taintNodeNoSchedule)
			node2 := test.BuildTestNode("n2", 2000, 3000, 10, nil)
			ctxCancel, cancel := context.WithCancel(ctx)
			featureGates := featuregate.NewFeatureGate()
			featureGates.Add(map[featuregate.Feature]featuregate.FeatureSpec{
				features.EvictionsInBackground: {Default: true, PreRelease: featuregate.Alpha},
			})
			_, descheduler, runFnc, client := initDescheduler(t, ctxCancel, featureGates, tc.policy, nil, false, node1, node2)
			defer cancel()

			var evictedPods []string
			client.PrependReactor("create", "pods", podEvictionReactionTestingFnc(&evictedPods, func(name string) bool { return name == "p1" || name == "p2" }, nil))

			rand.Seed(time.Now().UnixNano())
			pods := []*v1.Pod{
				test.BuildTestPod("p1", 100, 0, node1.Name, updatePodWithEvictionInBackground),
				test.BuildTestPod("p2", 100, 0, node1.Name, updatePodWithEvictionInBackground),
				test.BuildTestPod("p3", 100, 0, node1.Name, updatePod),
				test.BuildTestPod("p4", 100, 0, node1.Name, updatePod),
				test.BuildTestPod("p5", 100, 0, node1.Name, updatePod),
			}

			for i := 0; i < 10; i++ {
				rand.Shuffle(len(pods), func(i, j int) { pods[i], pods[j] = pods[j], pods[i] })
				func() {
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

					klog.Infof("2 evictions in background expected, 2 normal evictions")
					err := runFnc(ctx)
					if err != nil {
						t.Fatalf("Unable to run a descheduling loop: %v", err)
					}
					totalERs := descheduler.podEvictor.TotalEvictionRequests()
					totalEs := descheduler.podEvictor.TotalEvicted()
					if totalERs+totalEs > tc.limit {
						t.Fatalf("Expected %v evictions and eviction requests in total, got %v instead", tc.limit, totalERs+totalEs)
					}
					t.Logf("Total evictions and eviction requests: %v (er=%v, e=%v)", totalERs+totalEs, totalERs, totalEs)
				}()
			}
		})
	}
}

func TestNodeLabelSelectorBasedEviction(t *testing.T) {
	initPluginRegistry()

	// createNodes creates 4 nodes with different labels and applies a taint to all of them
	createNodes := func() (*v1.Node, *v1.Node, *v1.Node, *v1.Node) {
		taint := []v1.Taint{
			{
				Key:    "test-taint",
				Value:  "test-value",
				Effect: v1.TaintEffectNoSchedule,
			},
		}
		node1 := test.BuildTestNode("n1", 2000, 3000, 10, func(node *v1.Node) {
			node.Labels = map[string]string{
				"zone":        "us-east-1a",
				"node-type":   "compute",
				"environment": "production",
			}
			node.Spec.Taints = taint
		})
		node2 := test.BuildTestNode("n2", 2000, 3000, 10, func(node *v1.Node) {
			node.Labels = map[string]string{
				"zone":        "us-east-1b",
				"node-type":   "compute",
				"environment": "production",
			}
			node.Spec.Taints = taint
		})
		node3 := test.BuildTestNode("n3", 2000, 3000, 10, func(node *v1.Node) {
			node.Labels = map[string]string{
				"zone":        "us-west-1a",
				"node-type":   "storage",
				"environment": "staging",
			}
			node.Spec.Taints = taint
		})
		node4 := test.BuildTestNode("n4", 2000, 3000, 10, func(node *v1.Node) {
			node.Labels = map[string]string{
				"zone":        "us-west-1b",
				"node-type":   "storage",
				"environment": "staging",
			}
			node.Spec.Taints = taint
		})
		return node1, node2, node3, node4
	}

	tests := []struct {
		description              string
		nodeSelector             string
		dryRun                   bool
		expectedEvictedFromNodes []string
	}{
		{
			description:              "Evict from n1, n2",
			nodeSelector:             "environment=production",
			dryRun:                   false,
			expectedEvictedFromNodes: []string{"n1", "n2"},
		},
		{
			description:              "Evict from n1, n2 in dry run mode",
			nodeSelector:             "environment=production",
			dryRun:                   true,
			expectedEvictedFromNodes: []string{"n1", "n2"},
		},
		{
			description:              "Evict from n3, n4",
			nodeSelector:             "environment=staging",
			dryRun:                   false,
			expectedEvictedFromNodes: []string{"n3", "n4"},
		},
		{
			description:              "Evict from n3, n4 in dry run mode",
			nodeSelector:             "environment=staging",
			dryRun:                   true,
			expectedEvictedFromNodes: []string{"n3", "n4"},
		},
		{
			description:              "Evict from n1, n4",
			nodeSelector:             "zone in (us-east-1a, us-west-1b)",
			dryRun:                   false,
			expectedEvictedFromNodes: []string{"n1", "n4"},
		},
		{
			description:              "Evict from n1, n4 in dry run mode",
			nodeSelector:             "zone in (us-east-1a, us-west-1b)",
			dryRun:                   true,
			expectedEvictedFromNodes: []string{"n1", "n4"},
		},
		{
			description:              "Evict from n2, n3",
			nodeSelector:             "zone in (us-east-1b, us-west-1a)",
			dryRun:                   false,
			expectedEvictedFromNodes: []string{"n2", "n3"},
		},
		{
			description:              "Evict from n2, n3 in dry run mode",
			nodeSelector:             "zone in (us-east-1b, us-west-1a)",
			dryRun:                   true,
			expectedEvictedFromNodes: []string{"n2", "n3"},
		},
		{
			description:              "Evict from all nodes",
			nodeSelector:             "",
			dryRun:                   false,
			expectedEvictedFromNodes: []string{"n1", "n2", "n3", "n4"},
		},
		{
			description:              "Evict from all nodes in dry run mode",
			nodeSelector:             "",
			dryRun:                   true,
			expectedEvictedFromNodes: []string{"n1", "n2", "n3", "n4"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			ctx := context.Background()

			// Create nodes with different labels and taints
			node1, node2, node3, node4 := createNodes()

			ownerRef := test.GetReplicaSetOwnerRefList()
			updatePod := func(pod *v1.Pod) {
				pod.ObjectMeta.OwnerReferences = ownerRef
				pod.Status.Phase = v1.PodRunning
			}

			// Create one pod per node
			p1 := test.BuildTestPod("p1", 200, 0, node1.Name, updatePod)
			p2 := test.BuildTestPod("p2", 200, 0, node2.Name, updatePod)
			p3 := test.BuildTestPod("p3", 200, 0, node3.Name, updatePod)
			p4 := test.BuildTestPod("p4", 200, 0, node4.Name, updatePod)

			objects := []runtime.Object{node1, node2, node3, node4, p1, p2, p3, p4}

			// Map pod names to their node names for validation
			podToNode := map[string]string{
				"p1": "n1",
				"p2": "n2",
				"p3": "n3",
				"p4": "n4",
			}

			policy := removePodsViolatingNodeTaintsPolicy()
			if tc.nodeSelector != "" {
				policy.NodeSelector = &tc.nodeSelector
			}

			ctxCancel, cancel := context.WithCancel(ctx)
			_, deschedulerInstance, _, client := initDescheduler(t, ctxCancel, initFeatureGates(), policy, nil, tc.dryRun, objects...)
			defer cancel()

			// Verify all pods are created initially
			pods, err := client.CoreV1().Pods(p1.Namespace).List(ctx, metav1.ListOptions{})
			if err != nil {
				t.Fatalf("Unable to list pods: %v", err)
			}
			if len(pods.Items) != 4 {
				t.Errorf("Expected 4 pods initially, got %d", len(pods.Items))
			}

			var evictedPods []string
			if !tc.dryRun {
				client.PrependReactor("create", "pods", podEvictionReactionTestingFnc(&evictedPods, nil, nil))
			}

			evictedPodNames := runDeschedulerLoopAndGetEvictedPods(ctx, t, deschedulerInstance, tc.dryRun)
			if tc.dryRun {
				evictedPods = evictedPodNames
			}

			// Collect which nodes had pods evicted from them
			nodesWithEvictedPods := make(map[string]bool)
			for _, podName := range evictedPods {
				if nodeName, ok := podToNode[podName]; ok {
					nodesWithEvictedPods[nodeName] = true
				}
			}

			// Verify the correct number of nodes had pods evicted
			if len(nodesWithEvictedPods) != len(tc.expectedEvictedFromNodes) {
				t.Fatalf("Expected pods to be evicted from %d nodes, got %d nodes: %v", len(tc.expectedEvictedFromNodes), len(nodesWithEvictedPods), nodesWithEvictedPods)
			}

			// Verify pods were evicted from the correct nodes
			for _, nodeName := range tc.expectedEvictedFromNodes {
				if !nodesWithEvictedPods[nodeName] {
					t.Fatalf("Expected pod to be evicted from node %s, but it was not", nodeName)
				}
			}

			// Verify no unexpected nodes had pods evicted
			for nodeName := range nodesWithEvictedPods {
				found := false
				for _, expectedNode := range tc.expectedEvictedFromNodes {
					if nodeName == expectedNode {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("Unexpected eviction from node %s", nodeName)
				}
			}

			t.Logf("Successfully evicted pods from nodes: %v", tc.expectedEvictedFromNodes)
		})
	}
}

func TestLoadAwareDescheduling(t *testing.T) {
	initPluginRegistry()

	ownerRef1 := test.GetReplicaSetOwnerRefList()
	updatePod := func(pod *v1.Pod) {
		pod.ObjectMeta.OwnerReferences = ownerRef1
	}

	ctx := context.Background()
	node1 := test.BuildTestNode("n1", 2000, 3000, 10, taintNodeNoSchedule)
	node2 := test.BuildTestNode("n2", 2000, 3000, 10, nil)

	p1 := test.BuildTestPod("p1", 300, 0, node1.Name, updatePod)
	p2 := test.BuildTestPod("p2", 300, 0, node1.Name, updatePod)
	p3 := test.BuildTestPod("p3", 300, 0, node1.Name, updatePod)
	p4 := test.BuildTestPod("p4", 300, 0, node1.Name, updatePod)
	p5 := test.BuildTestPod("p5", 300, 0, node1.Name, updatePod)

	nodemetricses := []*v1beta1.NodeMetrics{
		test.BuildNodeMetrics("n1", 2400, 3000),
		test.BuildNodeMetrics("n2", 400, 0),
	}

	podmetricses := []*v1beta1.PodMetrics{
		test.BuildPodMetrics("p1", 400, 0),
		test.BuildPodMetrics("p2", 400, 0),
		test.BuildPodMetrics("p3", 400, 0),
		test.BuildPodMetrics("p4", 400, 0),
		test.BuildPodMetrics("p5", 400, 0),
	}

	metricsClientset := fakemetricsclient.NewSimpleClientset()
	for _, nodemetrics := range nodemetricses {
		metricsClientset.Tracker().Create(nodesgvr, nodemetrics, "")
	}
	for _, podmetrics := range podmetricses {
		metricsClientset.Tracker().Create(podsgvr, podmetrics, podmetrics.Namespace)
	}

	policy := lowNodeUtilizationPolicy(
		api.ResourceThresholds{
			v1.ResourceCPU:  30,
			v1.ResourcePods: 30,
		},
		api.ResourceThresholds{
			v1.ResourceCPU:  50,
			v1.ResourcePods: 50,
		},
		true, // enabled metrics utilization
	)
	policy.MetricsProviders = []api.MetricsProvider{{Source: api.KubernetesMetrics}}

	ctxCancel, cancel := context.WithCancel(ctx)
	_, descheduler, runFnc, _ := initDescheduler(
		t,
		ctxCancel,
		initFeatureGates(),
		policy,
		metricsClientset,
		false,
		node1, node2, p1, p2, p3, p4, p5)
	defer cancel()

	// This needs to be run since the metrics collector is started
	// after newDescheduler in RunDeschedulerStrategies.
	descheduler.metricsCollector.Collect(ctx)

	err := runFnc(ctx)
	if err != nil {
		t.Fatalf("Unable to run a descheduling loop: %v", err)
	}
	totalEs := descheduler.podEvictor.TotalEvicted()
	if totalEs != 2 {
		t.Fatalf("Expected %v evictions in total, got %v instead", 2, totalEs)
	}
	t.Logf("Total evictions: %v", totalEs)
}

func TestPodEvictionReactionFncErrorHandling(t *testing.T) {
	podsGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}

	testCases := []struct {
		name             string
		setupFnc         func(*fakeclientset.Clientset) (name, namespace string)
		expectHandled    bool
		expectError      bool
		errorContains    string
		expectedCacheLen int
	}{
		{
			name: "handles pod eviction successfully and adds to cache",
			setupFnc: func(fakeClient *fakeclientset.Clientset) (string, string) {
				pod := test.BuildTestPod("pod1", 100, 0, "node1", test.SetRSOwnerRef)
				err := fakeClient.Tracker().Add(pod)
				if err != nil {
					t.Fatalf("Failed to add pod: %v", err)
				}
				return pod.Name, pod.Namespace
			},
			expectHandled:    true,
			expectError:      false,
			expectedCacheLen: 1,
		},
		{
			name: "returns false and error when delete fails allowing other reactors to handle",
			setupFnc: func(fakeClient *fakeclientset.Clientset) (string, string) {
				pod := test.BuildTestPod("pod1", 100, 0, "node1", test.SetRSOwnerRef)
				if err := fakeClient.Tracker().Add(pod); err != nil {
					t.Fatalf("Failed to add pod: %v", err)
				}
				if err := fakeClient.Tracker().Delete(podsGVR, pod.Namespace, pod.Name); err != nil {
					t.Fatalf("Failed to pre-delete pod: %v", err)
				}
				return pod.Name, pod.Namespace
			},
			expectHandled:    false,
			expectError:      true,
			errorContains:    "unable to delete pod",
			expectedCacheLen: 0,
		},
		{
			name: "returns error when pod doesn't exist in tracker from the start",
			setupFnc: func(fakeClient *fakeclientset.Clientset) (string, string) {
				// Don't add the pod to the tracker at all
				return "nonexistent-pod", "default"
			},
			expectHandled:    false,
			expectError:      true,
			errorContains:    "unable to delete pod",
			expectedCacheLen: 0,
		},
		{
			name: "returns error when object is not a pod",
			setupFnc: func(fakeClient *fakeclientset.Clientset) (string, string) {
				configMap := &v1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-config",
						Namespace: "default",
					},
				}
				if err := fakeClient.Tracker().Create(podsGVR, configMap, "default"); err != nil {
					t.Fatalf("Failed to add ConfigMap to pods resource: %v", err)
				}
				return configMap.Name, configMap.Namespace
			},
			expectHandled:    false,
			expectError:      true,
			errorContains:    "unable to convert object to *v1.Pod",
			expectedCacheLen: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeClient := fakeclientset.NewSimpleClientset()
			cache := newEvictedPodsCache()

			name, namespace := tc.setupFnc(fakeClient)

			reactionFnc := podEvictionReactionFnc(fakeClient, cache)

			handled, _, err := reactionFnc(core.NewCreateSubresourceAction(
				podsGVR,
				name,
				"eviction",
				namespace,
				&policy.Eviction{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
					},
				},
			))

			if handled != tc.expectHandled {
				t.Errorf("Expected handled=%v, got %v", tc.expectHandled, handled)
			}

			if tc.expectError {
				if err == nil {
					t.Fatal("Expected error, got nil")
				}
				if !strings.Contains(err.Error(), tc.errorContains) {
					t.Errorf("Expected error message to contain '%s', got: %v", tc.errorContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
			}

			if len(cache.list()) != tc.expectedCacheLen {
				t.Errorf("Expected %d pods in cache, got %d", tc.expectedCacheLen, len(cache.list()))
			}
		})
	}
}

// verifyPodIdentityFields checks if name, namespace, and UID match expected values
func verifyPodIdentityFields(t *testing.T, name, namespace, uid, expectedName, expectedNamespace, expectedUID, context string) {
	t.Helper()
	if name != expectedName {
		t.Fatalf("Expected pod name %s%s, got %s", expectedName, context, name)
	}
	if namespace != expectedNamespace {
		t.Fatalf("Expected pod namespace %s%s, got %s", expectedNamespace, context, namespace)
	}
	if uid != expectedUID {
		t.Fatalf("Expected pod UID %s%s, got %s", expectedUID, context, uid)
	}
}

// verifyPodIdentity checks if a pod has the expected name, namespace, and UID
func verifyPodIdentity(t *testing.T, pod *v1.Pod, expectedName, expectedNamespace string, expectedUID types.UID) {
	t.Helper()
	verifyPodIdentityFields(t, pod.Name, pod.Namespace, string(pod.UID), expectedName, expectedNamespace, string(expectedUID), "")
}

func TestEvictedPodRestorationInDryRun(t *testing.T) {
	// Initialize klog flags
	// klog.InitFlags(nil)

	// Set verbosity level (higher number = more verbose)
	// 0 = errors only, 1-4 = info, 5-9 = debug, 10+ = trace
	// flag.Set("v", "4")

	initPluginRegistry()

	ctx := context.Background()
	node1 := test.BuildTestNode("n1", 2000, 3000, 10, taintNodeNoSchedule)
	node2 := test.BuildTestNode("n2", 2000, 3000, 10, nil)

	p1 := test.BuildTestPod("p1", 100, 0, node1.Name, test.SetRSOwnerRef)

	internalDeschedulerPolicy := removePodsViolatingNodeTaintsPolicy()
	ctxCancel, cancel := context.WithCancel(ctx)
	defer cancel()

	// Create descheduler with DryRun mode
	client := fakeclientset.NewSimpleClientset(node1, node2, p1)
	eventClient := fakeclientset.NewSimpleClientset(node1, node2, p1)

	rs, err := options.NewDeschedulerServer()
	if err != nil {
		t.Fatalf("Unable to initialize server: %v", err)
	}
	rs.Client = client
	rs.EventClient = eventClient
	rs.DefaultFeatureGates = initFeatureGates()
	rs.DryRun = true // Set DryRun before creating descheduler

	sharedInformerFactory := informers.NewSharedInformerFactoryWithOptions(rs.Client, 0, informers.WithTransform(trimManagedFields))
	eventBroadcaster, eventRecorder := utils.GetRecorderAndBroadcaster(ctxCancel, client)
	defer eventBroadcaster.Shutdown()

	// Always create descheduler with real client/factory first to register all informers
	descheduler, err := newDescheduler(ctxCancel, rs, internalDeschedulerPolicy, "v1", eventRecorder, rs.Client, sharedInformerFactory, nil, nil)
	if err != nil {
		t.Fatalf("Unable to create descheduler instance: %v", err)
	}

	sharedInformerFactory.Start(ctxCancel.Done())
	sharedInformerFactory.WaitForCacheSync(ctxCancel.Done())

	// Create sandbox with resources to mirror from real client
	kubeClientSandbox, err := newDefaultKubeClientSandbox(rs.Client, sharedInformerFactory)
	if err != nil {
		t.Fatalf("Failed to create kube client sandbox: %v", err)
	}

	// Replace descheduler with one using fake client/factory
	descheduler, err = newDescheduler(ctxCancel, rs, internalDeschedulerPolicy, "v1", eventRecorder, kubeClientSandbox.fakeClient(), kubeClientSandbox.fakeSharedInformerFactory(), nil, kubeClientSandbox)
	if err != nil {
		t.Fatalf("Unable to create dry run descheduler instance: %v", err)
	}

	// Start and sync the fake factory after creating the descheduler
	kubeClientSandbox.fakeSharedInformerFactory().Start(ctxCancel.Done())
	kubeClientSandbox.fakeSharedInformerFactory().WaitForCacheSync(ctxCancel.Done())

	// Verify the pod exists in the fake client after initialization
	pod, err := kubeClientSandbox.fakeClient().CoreV1().Pods(p1.Namespace).Get(ctx, p1.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Expected pod %s to exist in fake client after initialization, but got error: %v", p1.Name, err)
	}
	verifyPodIdentity(t, pod, p1.Name, p1.Namespace, p1.UID)
	klog.Infof("Pod %s exists in fake client after initialization", p1.Name)

	// Run two descheduling cycles to verify pod eviction and restoration works repeatedly
	for i := 1; i <= 2; i++ {
		// Run descheduling cycle
		klog.Infof("Running descheduling cycle %d", i)
		descheduler.podEvictor.ResetCounters()
		descheduler.runProfiles(ctx)

		// Verify the pod was evicted (should not exist in fake client anymore)
		_, err = kubeClientSandbox.fakeClient().CoreV1().Pods(p1.Namespace).Get(ctx, p1.Name, metav1.GetOptions{})
		if err == nil {
			t.Fatalf("Expected pod %s to be evicted from fake client in cycle %d, but it still exists", p1.Name, i)
		}
		if !apierrors.IsNotFound(err) {
			t.Fatalf("Expected NotFound error for pod %s in cycle %d, got: %v", p1.Name, i, err)
		}
		klog.Infof("Pod %s was successfully evicted from fake client in cycle %d", p1.Name, i)

		// Verify the pod was added to the evicted pods cache
		evictedPods := descheduler.kubeClientSandbox.evictedPodsCache.list()
		if len(evictedPods) != 1 {
			t.Fatalf("Expected 1 pod in evicted cache in cycle %d, got %d", i, len(evictedPods))
		}
		verifyPodIdentityFields(t, evictedPods[0].Name, evictedPods[0].Namespace, evictedPods[0].UID, p1.Name, p1.Namespace, string(p1.UID), fmt.Sprintf(" in cycle %d", i))
		klog.Infof("Pod %s was successfully added to evicted pods cache in cycle %d (UID: %s)", p1.Name, i, p1.UID)

		// Restore evicted pods
		klog.Infof("Restoring evicted pods from cache in cycle %d", i)
		if err := descheduler.kubeClientSandbox.restoreEvictedPods(ctx); err != nil {
			t.Fatalf("Failed to restore evicted pods in cycle %d: %v", i, err)
		}
		descheduler.kubeClientSandbox.evictedPodsCache.clear()

		// Verify the pod was restored back to the fake client
		pod, err = kubeClientSandbox.fakeClient().CoreV1().Pods(p1.Namespace).Get(ctx, p1.Name, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("Expected pod %s to be restored to fake client in cycle %d, but got error: %v", p1.Name, i, err)
		}
		verifyPodIdentity(t, pod, p1.Name, p1.Namespace, p1.UID)
		klog.Infof("Pod %s was successfully restored to fake client in cycle %d (UID: %s)", p1.Name, i, pod.UID)

		// Verify cache was cleared after restoration
		evictedPods = descheduler.kubeClientSandbox.evictedPodsCache.list()
		if len(evictedPods) != 0 {
			t.Fatalf("Expected evicted cache to be empty after restoration in cycle %d, got %d pods", i, len(evictedPods))
		}
		klog.Infof("Evicted pods cache was cleared after restoration in cycle %d", i)
	}
}

// verifyAllPrometheusClientsEqual checks that all Prometheus client variables are equal to the expected value
func verifyAllPrometheusClientsEqual(t *testing.T, expected, fromReactor, fromDescheduler promapi.Client) {
	t.Helper()
	if fromReactor != expected {
		t.Fatalf("Prometheus client from reactor: expected %v, got %v", expected, fromReactor)
	}
	if fromDescheduler != expected {
		t.Fatalf("Prometheus client from descheduler: expected %v, got %v", expected, fromDescheduler)
	}
	t.Logf("All Prometheus clients variables correctly set to: %v", expected)
}

// TestPluginPrometheusClientAccess tests that the Prometheus client is accessible through the plugin handle
func TestPluginPrometheusClientAccess_Secret(t *testing.T) {
	// klog.InitFlags(nil)
	// flag.Set("v", "4")
	testCases := []struct {
		name   string
		dryRun bool
	}{
		{
			name:   "dry run disabled",
			dryRun: false,
		},
		{
			name:   "dry run enabled",
			dryRun: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			initPluginRegistry()

			newInvoked := false
			reactorInvoked := false
			var prometheusClientFromPluginNewHandle promapi.Client
			var prometheusClientFromReactor promapi.Client

			fakePlugin := &fakeplugin.FakePlugin{
				PluginName: "TestPluginWithPrometheusClient",
			}

			fakePlugin.AddReactor(string(frameworktypes.DescheduleExtensionPoint), func(action fakeplugin.Action) (handled, filter bool, err error) {
				if dAction, ok := action.(fakeplugin.DescheduleAction); ok {
					reactorInvoked = true
					prometheusClientFromReactor = dAction.Handle().PrometheusClient()
					return true, false, nil
				}
				return false, false, nil
			})

			pluginregistry.Register(
				fakePlugin.PluginName,
				fakeplugin.NewPluginFncFromFakeWithReactor(fakePlugin, func(action fakeplugin.ActionImpl) {
					newInvoked = true
					prometheusClientFromPluginNewHandle = action.Handle().PrometheusClient()
				}),
				&fakeplugin.FakePlugin{},
				&fakeplugin.FakePluginArgs{},
				fakeplugin.ValidateFakePluginArgs,
				fakeplugin.SetDefaults_FakePluginArgs,
				pluginregistry.PluginRegistry,
			)

			deschedulerPolicy := &api.DeschedulerPolicy{
				MetricsProviders: []api.MetricsProvider{
					{
						Source:     api.PrometheusMetrics,
						Prometheus: newPrometheusConfig(),
					},
				},
				Profiles: []api.DeschedulerProfile{
					{
						Name: "test-profile",
						PluginConfigs: []api.PluginConfig{
							{
								Name: fakePlugin.PluginName,
								Args: &fakeplugin.FakePluginArgs{},
							},
						},
						Plugins: api.Plugins{
							Deschedule: api.PluginSet{
								Enabled: []string{fakePlugin.PluginName},
							},
						},
					},
				},
			}

			node1 := test.BuildTestNode("node1", 1000, 2000, 9, nil)
			node2 := test.BuildTestNode("node2", 1000, 2000, 9, nil)

			_, descheduler, runFnc, fakeClient := initDescheduler(t, ctx, initFeatureGates(), deschedulerPolicy, nil, tc.dryRun, node1, node2)

			if !newInvoked {
				t.Fatalf("Expected plugin New to be invoked")
			}
			newInvoked = false
			if prometheusClientFromPluginNewHandle != nil {
				t.Fatalf("Expected nil prometheus client from plugin New's handle, got %v", prometheusClientFromPluginNewHandle)
			}

			// Test cycles with different Prometheus client values
			cycles := []struct {
				name        string
				operation   func() error
				skipWaiting bool
				client      promapi.Client
				token       string
			}{
				{
					name:        "no secret initially",
					operation:   func() error { return nil },
					skipWaiting: true,
					client:      nil,
					token:       "",
				},
				{
					name: "add secret",
					operation: func() error {
						secret := newPrometheusAuthSecret(withToken("token-1"))
						_, err := fakeClient.CoreV1().Secrets(secret.Namespace).Create(ctx, secret, metav1.CreateOptions{})
						return err
					},
					client: &mockPrometheusClient{name: "new-init-client"},
					token:  "token-1",
				},
				{
					name: "update secret",
					operation: func() error {
						secret := newPrometheusAuthSecret(withToken("token-2"))
						_, err := fakeClient.CoreV1().Secrets(secret.Namespace).Update(ctx, secret, metav1.UpdateOptions{})
						return err
					},
					client: &mockPrometheusClient{name: "new-client"},
					token:  "token-2",
				},
				{
					name: "delete secret",
					operation: func() error {
						secret := newPrometheusAuthSecret(withToken("token-3"))
						return fakeClient.CoreV1().Secrets(secret.Namespace).Delete(ctx, secret.Name, metav1.DeleteOptions{})
					},
					client: nil,
					token:  "",
				},
			}

			for i, cycle := range cycles {
				t.Logf("Cycle %d: %s", i+1, cycle.name)

				// Set the descheduler's Prometheus client
				descheduler.secretBasedPromClientCtrl.mu.Lock()
				t.Logf("Setting descheduler.secretBasedPromClientCtrl.promClient from %v to %v", descheduler.secretBasedPromClientCtrl.promClient, cycle.client)
				descheduler.secretBasedPromClientCtrl.createPrometheusClient = func(url, token string) (promapi.Client, *http.Transport, error) {
					if token != cycle.token {
						t.Fatalf("Expected token to be %q, got %q", cycle.token, token)
					}
					if url != prometheusURL {
						t.Fatalf("Expected url to be %q, got %q", prometheusURL, url)
					}
					return cycle.client, &http.Transport{}, nil
				}
				descheduler.secretBasedPromClientCtrl.mu.Unlock()

				if err := cycle.operation(); err != nil {
					t.Fatalf("operation failed: %v", err)
				}

				if !cycle.skipWaiting {
					err := wait.PollUntilContextTimeout(ctx, 50*time.Millisecond, 200*time.Millisecond, true, func(ctx context.Context) (bool, error) {
						currentPromClient := descheduler.secretBasedPromClientCtrl.prometheusClient()
						if currentPromClient != cycle.client {
							t.Logf("Waiting for prometheus client to be set to %v, got %v instead, waiting", cycle.client, currentPromClient)
							return false, nil
						}
						return true, nil
					})
					if err != nil {
						t.Fatalf("Timed out waiting for expected conditions: %v", err)
					}
				}

				reactorInvoked = false
				prometheusClientFromReactor = nil

				if err := runFnc(ctx); err != nil {
					t.Fatalf("Unexpected error during running a descheduling cycle: %v", err)
				}

				t.Logf("After cycle %d: prometheusClientFromReactor=%v, descheduler.secretBasedPromClientCtrl.promClient=%v", i+1, prometheusClientFromReactor, descheduler.secretBasedPromClientCtrl.prometheusClient())

				if newInvoked {
					t.Fatalf("Expected plugin New not to be invoked during cycle %d", i+1)
				}

				if !reactorInvoked {
					t.Fatalf("Expected deschedule reactor to be invoked during cycle %d", i+1)
				}

				verifyAllPrometheusClientsEqual(t, cycle.client, prometheusClientFromReactor, descheduler.secretBasedPromClientCtrl.prometheusClient())
			}
		})
	}
}

func TestPluginPrometheusClientAccess_InCluster(t *testing.T) {
	testCases := []struct {
		name   string
		dryRun bool
	}{
		{
			name:   "dry run disabled",
			dryRun: false,
		},
		{
			name:   "dry run enabled",
			dryRun: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			initPluginRegistry()

			newInvoked := false
			reactorInvoked := false
			var prometheusClientFromPluginNewHandle promapi.Client
			var prometheusClientFromReactor promapi.Client

			fakePlugin := &fakeplugin.FakePlugin{
				PluginName: "TestPluginWithPrometheusClient",
			}

			fakePlugin.AddReactor(string(frameworktypes.DescheduleExtensionPoint), func(action fakeplugin.Action) (handled, filter bool, err error) {
				if dAction, ok := action.(fakeplugin.DescheduleAction); ok {
					reactorInvoked = true
					prometheusClientFromReactor = dAction.Handle().PrometheusClient()
					return true, false, nil
				}
				return false, false, nil
			})

			pluginregistry.Register(
				fakePlugin.PluginName,
				fakeplugin.NewPluginFncFromFakeWithReactor(fakePlugin, func(action fakeplugin.ActionImpl) {
					newInvoked = true
					prometheusClientFromPluginNewHandle = action.Handle().PrometheusClient()
				}),
				&fakeplugin.FakePlugin{},
				&fakeplugin.FakePluginArgs{},
				fakeplugin.ValidateFakePluginArgs,
				fakeplugin.SetDefaults_FakePluginArgs,
				pluginregistry.PluginRegistry,
			)

			deschedulerPolicy := &api.DeschedulerPolicy{
				MetricsProviders: []api.MetricsProvider{
					{
						Source: api.PrometheusMetrics,
						Prometheus: &api.Prometheus{
							URL: prometheusURL,
						},
					},
				},
				Profiles: []api.DeschedulerProfile{
					{
						Name: "test-profile",
						PluginConfigs: []api.PluginConfig{
							{
								Name: fakePlugin.PluginName,
								Args: &fakeplugin.FakePluginArgs{},
							},
						},
						Plugins: api.Plugins{
							Deschedule: api.PluginSet{
								Enabled: []string{fakePlugin.PluginName},
							},
						},
					},
				},
			}

			node1 := test.BuildTestNode("node1", 1000, 2000, 9, nil)
			node2 := test.BuildTestNode("node2", 1000, 2000, 9, nil)

			_, descheduler, runFnc, _ := initDescheduler(t, ctx, initFeatureGates(), deschedulerPolicy, nil, tc.dryRun, node1, node2)

			if !newInvoked {
				t.Fatalf("Expected plugin New to be invoked")
			}
			newInvoked = false
			if prometheusClientFromPluginNewHandle != nil {
				t.Fatalf("Expected nil prometheus client from plugin New's handle, got %v", prometheusClientFromPluginNewHandle)
			}

			// Test cycles with different Prometheus client values
			cycles := []struct {
				name   string
				client promapi.Client
				token  string
			}{
				{
					name:   "initial client",
					client: &mockPrometheusClient{name: "new-init-client"},
					token:  "init-token",
				},
				{
					name:   "nil client",
					client: nil,
					token:  "",
				},
				{
					name:   "new client",
					client: &mockPrometheusClient{name: "new-client"},
					token:  "new-token",
				},
				{
					name:   "another client",
					client: &mockPrometheusClient{name: "another-client"},
					token:  "another-token",
				},
			}

			for i, cycle := range cycles {
				t.Logf("Cycle %d: %s", i+1, cycle.name)

				// Set the descheduler's Prometheus client
				descheduler.inClusterPromClientCtrl.mu.Lock()
				t.Logf("Setting descheduler.inClusterPromClientCtrl.promClient from %v to %v", descheduler.inClusterPromClientCtrl.promClient, cycle.client)
				descheduler.inClusterPromClientCtrl.inClusterConfig = func() (*rest.Config, error) {
					return &rest.Config{BearerToken: cycle.token}, nil
				}
				descheduler.inClusterPromClientCtrl.createPrometheusClient = func(url, token string) (promapi.Client, *http.Transport, error) {
					if token != cycle.token {
						t.Errorf("Expected token to be %q, got %q", cycle.token, token)
					}
					if url != prometheusURL {
						t.Errorf("Expected url to be %q, got %q", prometheusURL, url)
					}
					return cycle.client, &http.Transport{}, nil
				}
				descheduler.inClusterPromClientCtrl.mu.Unlock()

				reactorInvoked = false
				prometheusClientFromReactor = nil

				if err := runFnc(ctx); err != nil {
					t.Fatalf("Unexpected error during running a descheduling cycle: %v", err)
				}

				t.Logf("After cycle %d: prometheusClientFromReactor=%v, descheduler.inClusterPromClientCtrl.promClient=%v", i+1, prometheusClientFromReactor, descheduler.inClusterPromClientCtrl.prometheusClient())

				if newInvoked {
					t.Fatalf("Expected plugin New not to be invoked during cycle %d", i+1)
				}

				if !reactorInvoked {
					t.Fatalf("Expected deschedule reactor to be invoked during cycle %d", i+1)
				}

				verifyAllPrometheusClientsEqual(t, cycle.client, prometheusClientFromReactor, descheduler.inClusterPromClientCtrl.prometheusClient())
			}
		})
	}
}

func withToken(token string) func(*v1.Secret) {
	return func(s *v1.Secret) {
		s.Data[prometheusAuthTokenSecretKey] = []byte(token)
	}
}

func newPrometheusAuthSecret(apply func(*v1.Secret)) *v1.Secret {
	secret := &v1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "prom-token",
			Namespace: "kube-system",
		},
		Data: map[string][]byte{},
	}
	if apply != nil {
		apply(secret)
	}
	return secret
}

const prometheusURL = "http://prometheus:9090"

type promClientControllerTestSetup struct {
	fakeClient                *fakeclientset.Clientset
	namespacedInformerFactory informers.SharedInformerFactory
	metricsProviders          map[api.MetricsSource]*api.MetricsProvider
	ctrl                      *secretBasedPromClientController
	namespace                 string
}

func setupPromClientControllerTest(ctx context.Context, t *testing.T, objects []runtime.Object, prometheusConfig *api.Prometheus, setNamespacedSharedInformerFactory bool) (*promClientControllerTestSetup, error) {
	fakeClient := fakeclientset.NewSimpleClientset(objects...)

	namespace := "default"
	if prometheusConfig != nil && prometheusConfig.AuthToken != nil && prometheusConfig.AuthToken.SecretReference != nil {
		namespace = prometheusConfig.AuthToken.SecretReference.Namespace
	}

	namespacedInformerFactory := informers.NewSharedInformerFactoryWithOptions(fakeClient, 0, informers.WithNamespace(namespace))
	_ = namespacedInformerFactory.Core().V1().Secrets().Informer()

	metricsProviders := metricsProviderListToMap([]api.MetricsProvider{{
		Source:     api.PrometheusMetrics,
		Prometheus: prometheusConfig,
	}})

	ctrl, err := newSecretBasedPromClientController(nil, prometheusConfig, namespacedInformerFactory)
	if err != nil {
		return nil, err
	}

	namespacedInformerFactory.Start(ctx.Done())
	namespacedInformerFactory.WaitForCacheSync(ctx.Done())

	// Wait for secrets to propagate to the informer
	if len(objects) > 0 && setNamespacedSharedInformerFactory {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		secretsLister := namespacedInformerFactory.Core().V1().Secrets().Lister().Secrets(namespace)
		if err := waitForSecretsPropagation(ctx, secretsLister, objects); err != nil {
			t.Fatalf("secrets did not propagate to the indexer: %v", err)
		}
	}

	return &promClientControllerTestSetup{
		fakeClient:                fakeClient,
		namespacedInformerFactory: namespacedInformerFactory,
		metricsProviders:          metricsProviders,
		ctrl:                      ctrl,
		namespace:                 namespace,
	}, nil
}

func newPrometheusConfig() *api.Prometheus {
	return &api.Prometheus{
		URL: prometheusURL,
		AuthToken: &api.AuthToken{
			SecretReference: &api.SecretReference{
				Namespace: "kube-system",
				Name:      "prom-token",
			},
		},
	}
}


// TestPluginInformerRegistration tests that plugin-specific informers are registered during newDescheduler
func TestPluginInformerRegistration(t *testing.T) {
	testCases := []struct {
		name   string
		dryRun bool
	}{
		{
			name:   "dry run disabled",
			dryRun: false,
		},
		{
			name:   "dry run enabled",
			dryRun: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			initPluginRegistry()

			// Define the custom informers that should be registered by the plugin
			customInformers := []schema.GroupVersionResource{
				{Group: "apps", Version: "v1", Resource: "daemonsets"},
				{Group: "apps", Version: "v1", Resource: "replicasets"},
				{Group: "apps", Version: "v1", Resource: "statefulsets"},
			}

			callbackInvoked := false
			fakePlugin := &fakeplugin.FakePlugin{
				PluginName: "TestPluginWithInformers",
			}

			reactorInvoked := false
			fakePlugin.AddReactor(string(frameworktypes.DescheduleExtensionPoint), func(action fakeplugin.Action) (handled, filter bool, err error) {
				if _, ok := action.(fakeplugin.DescheduleAction); ok {
					reactorInvoked = true
					return true, false, nil
				}
				return false, false, nil
			})

			// Register our mock plugin using NewPluginFncFromFakeWithReactor
			pluginregistry.Register(
				fakePlugin.PluginName,
				fakeplugin.NewPluginFncFromFakeWithReactor(fakePlugin, func(action fakeplugin.ActionImpl) {
					callbackInvoked = true
					for _, gvr := range customInformers {
						_, err := action.Handle().SharedInformerFactory().ForResource(gvr)
						if err != nil {
							t.Fatalf("Failed to register informer for %s: %v", gvr.Resource, err)
						}
						t.Logf("Informer for %v registered inside of plugin's New", gvr)
					}
				}),
				&fakeplugin.FakePlugin{},
				&fakeplugin.FakePluginArgs{},
				fakeplugin.ValidateFakePluginArgs,
				fakeplugin.SetDefaults_FakePluginArgs,
				pluginregistry.PluginRegistry,
			)

			deschedulerPolicy := &api.DeschedulerPolicy{
				Profiles: []api.DeschedulerProfile{
					{
						Name: "test-profile",
						PluginConfigs: []api.PluginConfig{
							{
								Name: fakePlugin.PluginName,
								Args: &fakeplugin.FakePluginArgs{},
							},
						},
						Plugins: api.Plugins{
							Deschedule: api.PluginSet{
								Enabled: []string{fakePlugin.PluginName},
							},
						},
					},
				},
			}

			node1 := test.BuildTestNode("node1", 1000, 2000, 9, nil)
			node2 := test.BuildTestNode("node2", 1000, 2000, 9, nil)

			_, descheduler, runFnc, _ := initDescheduler(t, ctx, initFeatureGates(), deschedulerPolicy, nil, tc.dryRun, node1, node2)

			if !callbackInvoked {
				t.Fatal("Expected plugin initialization callback to be invoked")
			}

			// Verify that custom informers were registered in the SharedInformerFactory
			for _, gvr := range customInformers {
				informer, err := descheduler.sharedInformerFactory.ForResource(gvr)
				if err != nil {
					t.Errorf("Expected %s informer to be registered in SharedInformerFactory, got error: %v", gvr.Resource, err)
					continue
				}

				if informer.Informer() == nil {
					t.Errorf("Expected %s informer to be registered in SharedInformerFactory", gvr.Resource)
					continue
				}

				var informer2 informers.GenericInformer
				informer2, err = descheduler.sharedInformerFactory.ForResource(gvr)
				if err != nil {
					t.Errorf("Expected %s informer to be cached in factory, got error: %v", gvr.Resource, err)
					continue
				}

				if informer.Informer() != informer2.Informer() {
					t.Errorf("Expected %s informer to be cached in factory", gvr.Resource)
				}
				t.Logf("Found %v informer after initializing the descheduler", gvr)
			}

			// Verify profileRunners were created
			if len(descheduler.profileRunners) == 0 {
				t.Fatal("Expected profileRunners to be created, got empty slice")
			}

			if len(descheduler.profileRunners) != 1 {
				t.Fatalf("Expected 1 profileRunner, got %d", len(descheduler.profileRunners))
			}

			// Verify profile name
			if descheduler.profileRunners[0].name != "test-profile" {
				t.Errorf("Expected profile name to be 'test-profile', got '%s'", descheduler.profileRunners[0].name)
			}

			callbackInvoked = false
			t.Logf("Running a descheduling cycle, no informer registration is expected")
			if err := runFnc(ctx); err != nil {
				t.Fatalf("Unable to run a descheduling loop: %v", err)
			}

			if !reactorInvoked {
				t.Fatalf("Expected reactorInvoked to be set")
			}
			t.Logf("Deschedule reactor invoked")

			if callbackInvoked {
				t.Fatal("Unexpected plugin initialization callback")
			}
			t.Logf("Plugin initialization callback not invoked as expected")
		})
	}
}
