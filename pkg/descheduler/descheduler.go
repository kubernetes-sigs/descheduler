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

package descheduler

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	promapi "github.com/prometheus/client_golang/api"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/labels"
	utilversion "k8s.io/apimachinery/pkg/util/version"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/events"
	componentbaseconfig "k8s.io/component-base/config"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/cmd/descheduler/app/options"
	"sigs.k8s.io/descheduler/metrics"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/client"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	eutils "sigs.k8s.io/descheduler/pkg/descheduler/evictions/utils"
	"sigs.k8s.io/descheduler/pkg/descheduler/metricscollector"
	nodeutil "sigs.k8s.io/descheduler/pkg/descheduler/node"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/framework/pluginregistry"
	frameworkprofile "sigs.k8s.io/descheduler/pkg/framework/profile"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
	"sigs.k8s.io/descheduler/pkg/tracing"
	"sigs.k8s.io/descheduler/pkg/utils"
	"sigs.k8s.io/descheduler/pkg/version"
)

const (
	indexerNodeSelectorGlobal = "indexer_node_selector_global"
)

type eprunner func(ctx context.Context, nodes []*v1.Node) *frameworktypes.Status

type profileRunner struct {
	name                      string
	descheduleEPs, balanceEPs eprunner
}

type descheduler struct {
	rs                        *options.DeschedulerServer
	client                    clientset.Interface
	kubeClientSandbox         *kubeClientSandbox
	getPodsAssignedToNode     podutil.GetPodsAssignedToNodeFunc
	sharedInformerFactory     informers.SharedInformerFactory
	deschedulerPolicy         *api.DeschedulerPolicy
	eventRecorder             events.EventRecorder
	podEvictor                *evictions.PodEvictor
	metricsCollector          *metricscollector.MetricsCollector
	inClusterPromClientCtrl   *inClusterPromClientController
	secretBasedPromClientCtrl *secretBasedPromClientController
	profileRunners            []profileRunner
}

func nodeSelectorFromPolicy(deschedulerPolicy *api.DeschedulerPolicy) (labels.Selector, error) {
	nodeSelector := labels.Everything()
	if deschedulerPolicy.NodeSelector != nil {
		sel, err := labels.Parse(*deschedulerPolicy.NodeSelector)
		if err != nil {
			return nil, err
		}
		nodeSelector = sel
	}
	return nodeSelector, nil
}

func addNodeSelectorIndexer(sharedInformerFactory informers.SharedInformerFactory, nodeSelector labels.Selector) error {
	return nodeutil.AddNodeSelectorIndexer(sharedInformerFactory.Core().V1().Nodes().Informer(), indexerNodeSelectorGlobal, nodeSelector)
}

func metricsProviderListToMap(providersList []api.MetricsProvider) map[api.MetricsSource]*api.MetricsProvider {
	providersMap := make(map[api.MetricsSource]*api.MetricsProvider)
	for _, provider := range providersList {
		provider := provider
		providersMap[provider.Source] = &provider
	}
	return providersMap
}

func getPrometheusConfig(providersList []api.MetricsProvider) *api.Prometheus {
	prometheusProvider := metricsProviderListToMap(providersList)[api.PrometheusMetrics]
	if prometheusProvider == nil {
		return nil
	}
	return prometheusProvider.Prometheus
}

func configureSecretPromClientReconciler(prometheusConfig *api.Prometheus) bool {
	return prometheusConfig.URL != "" && prometheusConfig.AuthToken != nil
}

func newDescheduler(ctx context.Context, rs *options.DeschedulerServer, deschedulerPolicy *api.DeschedulerPolicy, evictionPolicyGroupVersion string, eventRecorder events.EventRecorder, client clientset.Interface, sharedInformerFactory, namespacedSharedInformerFactory informers.SharedInformerFactory, kubeClientSandbox *kubeClientSandbox) (*descheduler, error) {
	podInformer := sharedInformerFactory.Core().V1().Pods().Informer()
	// Temporarily register the PVC because it is used by the DefaultEvictor plugin during
	// the descheduling cycle, where informer registration is ignored.
	_ = sharedInformerFactory.Core().V1().PersistentVolumeClaims().Informer()

	getPodsAssignedToNode, err := podutil.BuildGetPodsAssignedToNodeFunc(podInformer)
	if err != nil {
		return nil, fmt.Errorf("build get pods assigned to node function error: %v", err)
	}

	podEvictor, err := evictions.NewPodEvictor(
		ctx,
		client,
		eventRecorder,
		podInformer,
		rs.DefaultFeatureGates,
		evictions.NewOptions().
			WithPolicyGroupVersion(evictionPolicyGroupVersion).
			WithMaxPodsToEvictPerNode(deschedulerPolicy.MaxNoOfPodsToEvictPerNode).
			WithMaxPodsToEvictPerNamespace(deschedulerPolicy.MaxNoOfPodsToEvictPerNamespace).
			WithMaxPodsToEvictTotal(deschedulerPolicy.MaxNoOfPodsToEvictTotal).
			WithEvictionFailureEventNotification(deschedulerPolicy.EvictionFailureEventNotification).
			WithGracePeriodSeconds(deschedulerPolicy.GracePeriodSeconds).
			WithDryRun(rs.DryRun).
			WithMetricsEnabled(!rs.DisableMetrics),
	)
	if err != nil {
		return nil, err
	}

	desch := &descheduler{
		rs:                    rs,
		client:                client,
		kubeClientSandbox:     kubeClientSandbox,
		getPodsAssignedToNode: getPodsAssignedToNode,
		sharedInformerFactory: sharedInformerFactory,
		deschedulerPolicy:     deschedulerPolicy,
		eventRecorder:         eventRecorder,
		podEvictor:            podEvictor,
	}

	prometheusConfig := getPrometheusConfig(deschedulerPolicy.MetricsProviders)
	if prometheusConfig != nil && prometheusConfig.URL != "" {
		if configureSecretPromClientReconciler(prometheusConfig) {
			// Secret-based mode
			ctrl, err := newSecretBasedPromClientController(rs.PrometheusClient, prometheusConfig, namespacedSharedInformerFactory)
			if err != nil {
				return nil, err
			}
			desch.secretBasedPromClientCtrl = ctrl
		} else {
			// In-cluster mode
			desch.inClusterPromClientCtrl = newInClusterPromClientController(rs.PrometheusClient, prometheusConfig)
		}
	}

	nodeSelector, err := nodeSelectorFromPolicy(deschedulerPolicy)
	if err != nil {
		return nil, err
	}

	if err := addNodeSelectorIndexer(sharedInformerFactory, nodeSelector); err != nil {
		return nil, err
	}

	if rs.MetricsClient != nil {
		desch.metricsCollector = metricscollector.NewMetricsCollector(sharedInformerFactory.Core().V1().Nodes().Lister(), rs.MetricsClient, nodeSelector)
	}

	var profileRunners []profileRunner
	for idx, profile := range deschedulerPolicy.Profiles {
		var promClientGetter func() promapi.Client
		if desch.inClusterPromClientCtrl != nil {
			promClientGetter = desch.inClusterPromClientCtrl.prometheusClient
		} else if desch.secretBasedPromClientCtrl != nil {
			promClientGetter = desch.secretBasedPromClientCtrl.prometheusClient
		}
		currProfile, err := frameworkprofile.NewProfile(
			ctx,
			profile,
			pluginregistry.PluginRegistry,
			frameworkprofile.WithClientSet(desch.client),
			frameworkprofile.WithSharedInformerFactory(desch.sharedInformerFactory),
			frameworkprofile.WithPodEvictor(desch.podEvictor),
			frameworkprofile.WithGetPodsAssignedToNodeFnc(desch.getPodsAssignedToNode),
			frameworkprofile.WithMetricsCollector(desch.metricsCollector),
			frameworkprofile.WithPrometheusClient(promClientGetter),
			// Generate a unique instance ID using just the index to avoid long IDs
			// when profile names are very long
			frameworkprofile.WithProfileInstanceID(fmt.Sprintf("%d", idx)),
		)
		if err != nil {
			return nil, fmt.Errorf("unable to create %q profile: %v", profile.Name, err)
		}
		profileRunners = append(profileRunners, profileRunner{profile.Name, currProfile.RunDeschedulePlugins, currProfile.RunBalancePlugins})
	}

	desch.profileRunners = profileRunners
	return desch, nil
}

func (d *descheduler) runDeschedulerLoop(ctx context.Context) error {
	var span trace.Span
	ctx, span = tracing.Tracer().Start(ctx, "runDeschedulerLoop")
	defer span.End()
	defer func(loopStartDuration time.Time) {
		metrics.DeschedulerLoopDuration.With(map[string]string{}).Observe(time.Since(loopStartDuration).Seconds())
		metrics.LoopDuration.With(map[string]string{}).Observe(time.Since(loopStartDuration).Seconds())
	}(time.Now())

	klog.V(3).Infof("Resetting pod evictor counters")
	d.podEvictor.ResetCounters()

	d.runProfiles(ctx)

	if d.rs.DryRun {
		if d.kubeClientSandbox == nil {
			return fmt.Errorf("kubeClientSandbox is nil in DryRun mode")
		}
		klog.V(3).Infof("Restoring evicted pods from cache")
		if err := d.kubeClientSandbox.restoreEvictedPods(ctx); err != nil {
			klog.ErrorS(err, "Failed to restore evicted pods")
			return fmt.Errorf("failed to restore evicted pods: %w", err)
		}
		d.kubeClientSandbox.reset()
	}

	klog.V(1).InfoS("Number of evictions/requests", "totalEvicted", d.podEvictor.TotalEvicted(), "evictionRequests", d.podEvictor.TotalEvictionRequests())

	return nil
}

// runProfiles runs all the deschedule plugins of all profiles and
// later runs through all balance plugins of all profiles. (All Balance plugins should come after all Deschedule plugins)
// see https://github.com/kubernetes-sigs/descheduler/issues/979
func (d *descheduler) runProfiles(ctx context.Context) {
	var span trace.Span
	ctx, span = tracing.Tracer().Start(ctx, "runProfiles")
	defer span.End()

	nodesAsInterface, err := d.sharedInformerFactory.Core().V1().Nodes().Informer().GetIndexer().ByIndex(indexerNodeSelectorGlobal, indexerNodeSelectorGlobal)
	if err != nil {
		span.AddEvent("Failed to list nodes with global node selector", trace.WithAttributes(attribute.String("err", err.Error())))
		klog.Error(err)
		return
	}

	nodes, err := nodeutil.ReadyNodesFromInterfaces(nodesAsInterface)
	if err != nil {
		span.AddEvent("Failed to convert node as interfaces into ready nodes", trace.WithAttributes(attribute.String("err", err.Error())))
		klog.Error(err)
		return
	}

	// if len is still <= 1 error out
	if len(nodes) <= 1 {
		klog.InfoS("Skipping descheduling cycle: requires >=2 nodes", "found", len(nodes))
		return // gracefully skip this cycle instead of aborting
	}

	for _, profileR := range d.profileRunners {
		// First deschedule
		status := profileR.descheduleEPs(ctx, nodes)
		if status != nil && status.Err != nil {
			span.AddEvent("failed to perform deschedule operations", trace.WithAttributes(attribute.String("err", status.Err.Error()), attribute.String("profile", profileR.name), attribute.String("operation", tracing.DescheduleOperation)))
			klog.ErrorS(status.Err, "running deschedule extension point failed with error", "profile", profileR.name)
			continue
		}
	}

	for _, profileR := range d.profileRunners {
		// Balance Later
		status := profileR.balanceEPs(ctx, nodes)
		if status != nil && status.Err != nil {
			span.AddEvent("failed to perform balance operations", trace.WithAttributes(attribute.String("err", status.Err.Error()), attribute.String("profile", profileR.name), attribute.String("operation", tracing.BalanceOperation)))
			klog.ErrorS(status.Err, "running balance extension point failed with error", "profile", profileR.name)
			continue
		}
	}
}

func Run(ctx context.Context, rs *options.DeschedulerServer) error {
	var span trace.Span
	ctx, span = tracing.Tracer().Start(ctx, "Run")
	defer span.End()
	metrics.Register()

	clientConnection := rs.ClientConnection
	if rs.KubeconfigFile != "" && clientConnection.Kubeconfig == "" {
		clientConnection.Kubeconfig = rs.KubeconfigFile
	}
	rsclient, eventClient, err := createClients(clientConnection)
	if err != nil {
		return err
	}
	rs.Client = rsclient
	rs.EventClient = eventClient

	deschedulerPolicy, err := LoadPolicyConfig(rs.PolicyConfigFile, rs.Client, pluginregistry.PluginRegistry)
	if err != nil {
		return err
	}
	if deschedulerPolicy == nil {
		return fmt.Errorf("deschedulerPolicy is nil")
	}

	// Add k8s compatibility warnings to logs
	if err := validateVersionCompatibility(rs.Client.Discovery(), version.Get()); err != nil {
		klog.Warning(err.Error())
	}

	evictionPolicyGroupVersion, err := eutils.SupportEviction(rs.Client)
	if err != nil || len(evictionPolicyGroupVersion) == 0 {
		return err
	}

	if (deschedulerPolicy.MetricsCollector != nil && deschedulerPolicy.MetricsCollector.Enabled) || metricsProviderListToMap(deschedulerPolicy.MetricsProviders)[api.KubernetesMetrics] != nil {
		metricsClient, err := client.CreateMetricsClient(clientConnection, "descheduler")
		if err != nil {
			return err
		}
		rs.MetricsClient = metricsClient
	}

	runFn := func() error {
		return RunDeschedulerStrategies(ctx, rs, deschedulerPolicy, evictionPolicyGroupVersion)
	}

	if rs.LeaderElection.LeaderElect && rs.DeschedulingInterval.Seconds() == 0 {
		span.AddEvent("Validation Failure", trace.WithAttributes(attribute.String("err", "leaderElection must be used with deschedulingInterval")))
		return fmt.Errorf("leaderElection must be used with deschedulingInterval")
	}

	if rs.LeaderElection.LeaderElect && rs.DryRun {
		klog.V(1).Info("Warning: DryRun is set to True. You need to disable it to use Leader Election.")
	}

	if rs.LeaderElection.LeaderElect && !rs.DryRun {
		if err := NewLeaderElection(runFn, rsclient, &rs.LeaderElection, ctx); err != nil {
			span.AddEvent("Leader Election Failure", trace.WithAttributes(attribute.String("err", err.Error())))
			return fmt.Errorf("leaderElection: %w", err)
		}
		return nil
	}

	return runFn()
}

func validateVersionCompatibility(discovery discovery.DiscoveryInterface, deschedulerVersionInfo version.Info) error {
	kubeServerVersionInfo, err := discovery.ServerVersion()
	if err != nil {
		return fmt.Errorf("failed to discover Kubernetes server version: %v", err)
	}

	kubeServerVersion, err := utilversion.ParseSemantic(kubeServerVersionInfo.String())
	if err != nil {
		return fmt.Errorf("failed to parse Kubernetes server version '%s': %v", kubeServerVersionInfo.String(), err)
	}

	deschedulerMinor, err := strconv.ParseFloat(deschedulerVersionInfo.Minor, 64)
	if err != nil {
		return fmt.Errorf("failed to convert Descheduler minor version '%s' to float: %v", deschedulerVersionInfo.Minor, err)
	}

	kubeServerMinor := float64(kubeServerVersion.Minor())
	if math.Abs(deschedulerMinor-kubeServerMinor) > 3 {
		return fmt.Errorf(
			"descheduler version %s.%s may not be supported on your version of Kubernetes %v."+
				"See compatibility docs for more info: https://github.com/kubernetes-sigs/descheduler#compatibility-matrix",
			deschedulerVersionInfo.Major,
			deschedulerVersionInfo.Minor,
			kubeServerVersionInfo.String(),
		)
	}

	return nil
}

type runFncType func(context.Context) error

func bootstrapDescheduler(
	ctx context.Context,
	rs *options.DeschedulerServer,
	deschedulerPolicy *api.DeschedulerPolicy,
	evictionPolicyGroupVersion string,
	sharedInformerFactory, namespacedSharedInformerFactory informers.SharedInformerFactory,
	eventRecorder events.EventRecorder,
) (*descheduler, runFncType, error) {
	// Always create descheduler with real client/factory first to register all informers
	descheduler, err := newDescheduler(ctx, rs, deschedulerPolicy, evictionPolicyGroupVersion, eventRecorder, rs.Client, sharedInformerFactory, namespacedSharedInformerFactory, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create new descheduler: %v", err)
	}

	// If in dry run mode, replace the descheduler with one using fake client/factory
	if rs.DryRun {
		// Create sandbox with resources to mirror from real client
		kubeClientSandbox, err := newDefaultKubeClientSandbox(rs.Client, sharedInformerFactory)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create kube client sandbox: %v", err)
		}

		klog.V(3).Infof("Building a cached client from the cluster for the dry run")

		// TODO(ingvagabund): drop the previous queue
		// TODO(ingvagabund): stop the previous pod evictor
		// Replace descheduler with one using fake client/factory
		descheduler, err = newDescheduler(ctx, rs, deschedulerPolicy, evictionPolicyGroupVersion, eventRecorder, kubeClientSandbox.fakeClient(), kubeClientSandbox.fakeSharedInformerFactory(), namespacedSharedInformerFactory, kubeClientSandbox)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create dry run descheduler: %v", err)
		}
	}

	// init is responsible for starting all informer factories, metrics providers
	// and other parts that require to start before a first descheduling cycle is run
	deschedulerInitFnc := func(ctx context.Context) error {
		// In dry run mode, start and sync the fake shared informer factory so it can mirror
		// events from the real factory. Reliable propagation depends on both factories being
		// fully synced (see WaitForCacheSync calls below), not solely on startup order.
		if rs.DryRun {
			descheduler.kubeClientSandbox.fakeSharedInformerFactory().Start(ctx.Done())
			descheduler.kubeClientSandbox.fakeSharedInformerFactory().WaitForCacheSync(ctx.Done())
		}
		sharedInformerFactory.Start(ctx.Done())
		if descheduler.secretBasedPromClientCtrl != nil {
			namespacedSharedInformerFactory.Start(ctx.Done())
		}

		sharedInformerFactory.WaitForCacheSync(ctx.Done())
		if descheduler.secretBasedPromClientCtrl != nil {
			namespacedSharedInformerFactory.WaitForCacheSync(ctx.Done())
		}

		descheduler.podEvictor.WaitForEventHandlersSync(ctx)

		if descheduler.metricsCollector != nil {
			go func() {
				klog.V(2).Infof("Starting metrics collector")
				descheduler.metricsCollector.Run(ctx)
				klog.V(2).Infof("Stopped metrics collector")
			}()
			klog.V(2).Infof("Waiting for metrics collector to sync")
			if err := wait.PollWithContext(ctx, time.Second, time.Minute, func(context.Context) (done bool, err error) {
				return descheduler.metricsCollector.HasSynced(), nil
			}); err != nil {
				return fmt.Errorf("unable to wait for metrics collector to sync: %v", err)
			}
		}

		if descheduler.secretBasedPromClientCtrl != nil {
			go descheduler.secretBasedPromClientCtrl.runAuthenticationSecretReconciler(ctx)
		}

		return nil
	}

	if err := deschedulerInitFnc(ctx); err != nil {
		return nil, nil, err
	}

	runFnc := func(ctx context.Context) error {
		if descheduler.inClusterPromClientCtrl != nil {
			// Read the sa token and assume it has the sufficient permissions to authenticate
			if err := descheduler.inClusterPromClientCtrl.reconcileInClusterSAToken(); err != nil {
				return fmt.Errorf("unable to reconcile an in cluster SA token: %v", err)
			}
		}

		err = descheduler.runDeschedulerLoop(ctx)
		if err != nil {
			return fmt.Errorf("failed to run descheduler loop: %v", err)
		}

		return nil
	}

	return descheduler, runFnc, nil
}

func RunDeschedulerStrategies(ctx context.Context, rs *options.DeschedulerServer, deschedulerPolicy *api.DeschedulerPolicy, evictionPolicyGroupVersion string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var span trace.Span
	ctx, span = tracing.Tracer().Start(ctx, "RunDeschedulerStrategies")
	defer span.End()

	sharedInformerFactory := informers.NewSharedInformerFactoryWithOptions(rs.Client, 0, informers.WithTransform(trimManagedFields))

	var eventClient clientset.Interface
	if rs.DryRun {
		eventClient = fakeclientset.NewSimpleClientset()
	} else {
		eventClient = rs.Client
	}
	eventBroadcaster, eventRecorder := utils.GetRecorderAndBroadcaster(ctx, eventClient)
	defer eventBroadcaster.Shutdown()

	var namespacedSharedInformerFactory informers.SharedInformerFactory
	prometheusConfig := getPrometheusConfig(deschedulerPolicy.MetricsProviders)
	if prometheusConfig != nil && configureSecretPromClientReconciler(prometheusConfig) {
		namespacedSharedInformerFactory = informers.NewSharedInformerFactoryWithOptions(rs.Client, 0, informers.WithTransform(trimManagedFields), informers.WithNamespace(prometheusConfig.AuthToken.SecretReference.Namespace))
	}

	_, runLoop, err := bootstrapDescheduler(ctx, rs, deschedulerPolicy, evictionPolicyGroupVersion, sharedInformerFactory, namespacedSharedInformerFactory, eventRecorder)
	if err != nil {
		span.AddEvent("Failed to bootstrap a descheduler", trace.WithAttributes(attribute.String("err", err.Error())))
		return err
	}

	wait.NonSlidingUntil(func() {
		// A next context is created here intentionally to avoid nesting the spans via context.
		sCtx, sSpan := tracing.Tracer().Start(ctx, "NonSlidingUntil")
		defer sSpan.End()

		if err := runLoop(sCtx); err != nil {
			sSpan.AddEvent("Descheduling loop failed", trace.WithAttributes(attribute.String("err", err.Error())))
			klog.Error(err)
			return
		}
		// If there was no interval specified, send a signal to the stopChannel to end the wait.Until loop after 1 iteration
		if rs.DeschedulingInterval.Seconds() == 0 {
			cancel()
		}
	}, rs.DeschedulingInterval, ctx.Done())

	return nil
}

func GetPluginConfig(pluginName string, pluginConfigs []api.PluginConfig) (*api.PluginConfig, int) {
	for idx, pluginConfig := range pluginConfigs {
		if pluginConfig.Name == pluginName {
			return &pluginConfig, idx
		}
	}
	return nil, 0
}

func createClients(clientConnection componentbaseconfig.ClientConnectionConfiguration) (clientset.Interface, clientset.Interface, error) {
	kClient, err := client.CreateClient(clientConnection, "descheduler")
	if err != nil {
		return nil, nil, err
	}

	eventClient, err := client.CreateClient(clientConnection, "")
	if err != nil {
		return nil, nil, err
	}

	return kClient, eventClient, nil
}

func trimManagedFields(obj interface{}) (interface{}, error) {
	if accessor, err := meta.Accessor(obj); err == nil {
		accessor.SetManagedFields(nil)
	}
	return obj, nil
}
