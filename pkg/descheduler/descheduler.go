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
	"net/http"
	"strconv"
	"time"

	promapi "github.com/prometheus/client_golang/api"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	utilversion "k8s.io/apimachinery/pkg/util/version"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/events"
	"k8s.io/client-go/util/workqueue"
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
	prometheusAuthTokenSecretKey = "prometheusAuthToken"
	workQueueKey                 = "key"
	indexerNodeSelectorGlobal    = "indexer_node_selector_global"
)

type eprunner func(ctx context.Context, nodes []*v1.Node) *frameworktypes.Status

type profileRunner struct {
	name                      string
	descheduleEPs, balanceEPs eprunner
}

type descheduler struct {
	rs                    *options.DeschedulerServer
	client                clientset.Interface
	kubeClientSandbox     *kubeClientSandbox
	getPodsAssignedToNode podutil.GetPodsAssignedToNodeFunc
	sharedInformerFactory informers.SharedInformerFactory
	deschedulerPolicy     *api.DeschedulerPolicy
	eventRecorder         events.EventRecorder
	podEvictor            *evictions.PodEvictor
	metricsCollector      *metricscollector.MetricsCollector
	promClientCtrl        *promClientController
}

type promClientController struct {
	promClient                        promapi.Client
	previousPrometheusClientTransport *http.Transport
	queue                             workqueue.RateLimitingInterface
	currentPrometheusAuthToken        string
	namespacedSecretsLister           corev1listers.SecretNamespaceLister
	metricsProviders                  map[api.MetricsSource]*api.MetricsProvider
}

func newPromClientController(prometheusClient promapi.Client, metricsProviders map[api.MetricsSource]*api.MetricsProvider) *promClientController {
	return &promClientController{
		promClient:       prometheusClient,
		queue:            workqueue.NewRateLimitingQueueWithConfig(workqueue.DefaultControllerRateLimiter(), workqueue.RateLimitingQueueConfig{Name: "descheduler"}),
		metricsProviders: metricsProviders,
	}
}

func (d *promClientController) prometheusClient() promapi.Client {
	return d.promClient
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
		providersMap[provider.Source] = &provider
	}
	return providersMap
}

// setupPrometheusProvider sets up the prometheus provider on the descheduler if configured
func setupPrometheusProvider(d *descheduler, namespacedSharedInformerFactory informers.SharedInformerFactory) error {
	if d.promClientCtrl == nil {
		return nil
	}
	prometheusProvider := d.promClientCtrl.metricsProviders[api.PrometheusMetrics]
	if prometheusProvider != nil && prometheusProvider.Prometheus != nil && prometheusProvider.Prometheus.AuthToken != nil {
		authTokenSecret := prometheusProvider.Prometheus.AuthToken.SecretReference
		if authTokenSecret == nil || authTokenSecret.Namespace == "" {
			return fmt.Errorf("prometheus metrics source configuration is missing authentication token secret")
		}
		if namespacedSharedInformerFactory == nil {
			return fmt.Errorf("namespacedSharedInformerFactory not configured")
		}
		namespacedSharedInformerFactory.Core().V1().Secrets().Informer().AddEventHandler(d.promClientCtrl.eventHandler())
		d.promClientCtrl.namespacedSecretsLister = namespacedSharedInformerFactory.Core().V1().Secrets().Lister().Secrets(authTokenSecret.Namespace)
	}
	return nil
}

func newDescheduler(ctx context.Context, rs *options.DeschedulerServer, deschedulerPolicy *api.DeschedulerPolicy, evictionPolicyGroupVersion string, eventRecorder events.EventRecorder, client clientset.Interface, sharedInformerFactory informers.SharedInformerFactory, kubeClientSandbox *kubeClientSandbox) (*descheduler, error) {
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
		promClientCtrl:        newPromClientController(rs.PrometheusClient, metricsProviderListToMap(deschedulerPolicy.MetricsProviders)),
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

	return desch, nil
}

func (d *promClientController) reconcileInClusterSAToken() error {
	// Read the sa token and assume it has the sufficient permissions to authenticate
	cfg, err := rest.InClusterConfig()
	if err == nil {
		if d.currentPrometheusAuthToken != cfg.BearerToken {
			klog.V(2).Infof("Creating Prometheus client (with SA token)")
			prometheusClient, transport, err := client.CreatePrometheusClient(d.metricsProviders[api.PrometheusMetrics].Prometheus.URL, cfg.BearerToken)
			if err != nil {
				return fmt.Errorf("unable to create a prometheus client: %v", err)
			}
			d.promClient = prometheusClient
			if d.previousPrometheusClientTransport != nil {
				d.previousPrometheusClientTransport.CloseIdleConnections()
			}
			d.previousPrometheusClientTransport = transport
			d.currentPrometheusAuthToken = cfg.BearerToken
		}
		return nil
	}
	if err == rest.ErrNotInCluster {
		return nil
	}
	return fmt.Errorf("unexpected error when reading in cluster config: %v", err)
}

func (d *promClientController) runAuthenticationSecretReconciler(ctx context.Context) {
	defer utilruntime.HandleCrash()
	defer d.queue.ShutDown()

	klog.Infof("Starting authentication secret reconciler")
	defer klog.Infof("Shutting down authentication secret reconciler")

	go wait.UntilWithContext(ctx, d.runAuthenticationSecretReconcilerWorker, time.Second)

	<-ctx.Done()
}

func (d *promClientController) runAuthenticationSecretReconcilerWorker(ctx context.Context) {
	for d.processNextWorkItem(ctx) {
	}
}

func (d *promClientController) processNextWorkItem(ctx context.Context) bool {
	dsKey, quit := d.queue.Get()
	if quit {
		return false
	}
	defer d.queue.Done(dsKey)

	err := d.sync()
	if err == nil {
		d.queue.Forget(dsKey)
		return true
	}

	utilruntime.HandleError(fmt.Errorf("%v failed with : %v", dsKey, err))
	d.queue.AddRateLimited(dsKey)

	return true
}

func (d *promClientController) sync() error {
	prometheusConfig := d.metricsProviders[api.PrometheusMetrics].Prometheus
	if prometheusConfig == nil || prometheusConfig.AuthToken == nil || prometheusConfig.AuthToken.SecretReference == nil {
		return fmt.Errorf("prometheus metrics source configuration is missing authentication token secret")
	}
	ns := prometheusConfig.AuthToken.SecretReference.Namespace
	name := prometheusConfig.AuthToken.SecretReference.Name
	secretObj, err := d.namespacedSecretsLister.Get(name)
	if err != nil {
		// clear the token if the secret is not found
		if apierrors.IsNotFound(err) {
			d.currentPrometheusAuthToken = ""
			if d.previousPrometheusClientTransport != nil {
				d.previousPrometheusClientTransport.CloseIdleConnections()
			}
			d.previousPrometheusClientTransport = nil
			d.promClient = nil
		}
		return fmt.Errorf("unable to get %v/%v secret", ns, name)
	}
	authToken := string(secretObj.Data[prometheusAuthTokenSecretKey])
	if authToken == "" {
		return fmt.Errorf("prometheus authentication token secret missing %q data or empty", prometheusAuthTokenSecretKey)
	}
	if d.currentPrometheusAuthToken == authToken {
		return nil
	}

	klog.V(2).Infof("authentication secret token updated, recreating prometheus client")
	prometheusClient, transport, err := client.CreatePrometheusClient(prometheusConfig.URL, authToken)
	if err != nil {
		return fmt.Errorf("unable to create a prometheus client: %v", err)
	}
	d.promClient = prometheusClient
	if d.previousPrometheusClientTransport != nil {
		d.previousPrometheusClientTransport.CloseIdleConnections()
	}
	d.previousPrometheusClientTransport = transport
	d.currentPrometheusAuthToken = authToken
	return nil
}

func (d *promClientController) eventHandler() cache.ResourceEventHandler {
	return cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { d.queue.Add(workQueueKey) },
		UpdateFunc: func(old, new interface{}) { d.queue.Add(workQueueKey) },
		DeleteFunc: func(obj interface{}) { d.queue.Add(workQueueKey) },
	}
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

	var profileRunners []profileRunner
	for idx, profile := range d.deschedulerPolicy.Profiles {
		var promClient promapi.Client
		if d.promClientCtrl != nil {
			promClient = d.promClientCtrl.prometheusClient()
		}
		currProfile, err := frameworkprofile.NewProfile(
			ctx,
			profile,
			pluginregistry.PluginRegistry,
			frameworkprofile.WithClientSet(d.client),
			frameworkprofile.WithSharedInformerFactory(d.sharedInformerFactory),
			frameworkprofile.WithPodEvictor(d.podEvictor),
			frameworkprofile.WithGetPodsAssignedToNodeFnc(d.getPodsAssignedToNode),
			frameworkprofile.WithMetricsCollector(d.metricsCollector),
			frameworkprofile.WithPrometheusClient(promClient),
			// Generate a unique instance ID using just the index to avoid long IDs
			// when profile names are very long
			frameworkprofile.WithProfileInstanceID(fmt.Sprintf("%d", idx)),
		)
		if err != nil {
			klog.ErrorS(err, "unable to create a profile", "profile", profile.Name)
			continue
		}
		profileRunners = append(profileRunners, profileRunner{profile.Name, currProfile.RunDeschedulePlugins, currProfile.RunBalancePlugins})
	}

	for _, profileR := range profileRunners {
		// First deschedule
		status := profileR.descheduleEPs(ctx, nodes)
		if status != nil && status.Err != nil {
			span.AddEvent("failed to perform deschedule operations", trace.WithAttributes(attribute.String("err", status.Err.Error()), attribute.String("profile", profileR.name), attribute.String("operation", tracing.DescheduleOperation)))
			klog.ErrorS(status.Err, "running deschedule extension point failed with error", "profile", profileR.name)
			continue
		}
	}

	for _, profileR := range profileRunners {
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

type tokenReconciliation int

const (
	noReconciliation tokenReconciliation = iota
	inClusterReconciliation
	secretReconciliation
)

func RunDeschedulerStrategies(ctx context.Context, rs *options.DeschedulerServer, deschedulerPolicy *api.DeschedulerPolicy, evictionPolicyGroupVersion string) error {
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
	metricProviderTokenReconciliation := noReconciliation

	prometheusProvider := metricsProviderListToMap(deschedulerPolicy.MetricsProviders)[api.PrometheusMetrics]
	if prometheusProvider != nil && prometheusProvider.Prometheus != nil && prometheusProvider.Prometheus.URL != "" {
		if prometheusProvider.Prometheus.AuthToken != nil {
			// Will get reconciled
			namespacedSharedInformerFactory = informers.NewSharedInformerFactoryWithOptions(rs.Client, 0, informers.WithTransform(trimManagedFields), informers.WithNamespace(prometheusProvider.Prometheus.AuthToken.SecretReference.Namespace))
			metricProviderTokenReconciliation = secretReconciliation
		} else {
			// Use the sa token and assume it has the sufficient permissions to authenticate
			metricProviderTokenReconciliation = inClusterReconciliation
		}
	}

	// Always create descheduler with real client/factory first to register all informers
	descheduler, err := newDescheduler(ctx, rs, deschedulerPolicy, evictionPolicyGroupVersion, eventRecorder, rs.Client, sharedInformerFactory, nil)
	if err != nil {
		span.AddEvent("Failed to create new descheduler", trace.WithAttributes(attribute.String("err", err.Error())))
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Setup Prometheus provider (only for real client case, not for dry run)
	if err := setupPrometheusProvider(descheduler, namespacedSharedInformerFactory); err != nil {
		span.AddEvent("Failed to setup Prometheus provider", trace.WithAttributes(attribute.String("err", err.Error())))
		return err
	}

	// If in dry run mode, replace the descheduler with one using fake client/factory
	if rs.DryRun {
		// Create sandbox with resources to mirror from real client
		kubeClientSandbox, err := newDefaultKubeClientSandbox(rs.Client, sharedInformerFactory)
		if err != nil {
			span.AddEvent("Failed to create kube client sandbox", trace.WithAttributes(attribute.String("err", err.Error())))
			return fmt.Errorf("failed to create kube client sandbox: %v", err)
		}

		klog.V(3).Infof("Building a cached client from the cluster for the dry run")

		// TODO(ingvagabund): drop the previous queue
		// TODO(ingvagabund): stop the previous pod evictor
		// Replace descheduler with one using fake client/factory
		descheduler, err = newDescheduler(ctx, rs, deschedulerPolicy, evictionPolicyGroupVersion, eventRecorder, kubeClientSandbox.fakeClient(), kubeClientSandbox.fakeSharedInformerFactory(), kubeClientSandbox)
		if err != nil {
			span.AddEvent("Failed to create dry run descheduler", trace.WithAttributes(attribute.String("err", err.Error())))
			return err
		}
	}

	// In dry run mode, start and sync the fake shared informer factory so it can mirror
	// events from the real factory. Reliable propagation depends on both factories being
	// fully synced (see WaitForCacheSync calls below), not solely on startup order.
	if rs.DryRun {
		descheduler.kubeClientSandbox.fakeSharedInformerFactory().Start(ctx.Done())
		descheduler.kubeClientSandbox.fakeSharedInformerFactory().WaitForCacheSync(ctx.Done())
	}
	sharedInformerFactory.Start(ctx.Done())
	if metricProviderTokenReconciliation == secretReconciliation {
		namespacedSharedInformerFactory.Start(ctx.Done())
	}

	sharedInformerFactory.WaitForCacheSync(ctx.Done())
	if metricProviderTokenReconciliation == secretReconciliation {
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

	if metricProviderTokenReconciliation == secretReconciliation {
		go descheduler.promClientCtrl.runAuthenticationSecretReconciler(ctx)
	}

	wait.NonSlidingUntil(func() {
		if metricProviderTokenReconciliation == inClusterReconciliation {
			// Read the sa token and assume it has the sufficient permissions to authenticate
			if err := descheduler.promClientCtrl.reconcileInClusterSAToken(); err != nil {
				klog.ErrorS(err, "unable to reconcile an in cluster SA token")
				return
			}
		}

		// A next context is created here intentionally to avoid nesting the spans via context.
		sCtx, sSpan := tracing.Tracer().Start(ctx, "NonSlidingUntil")
		defer sSpan.End()

		err = descheduler.runDeschedulerLoop(sCtx)
		if err != nil {
			sSpan.AddEvent("Failed to run descheduler loop", trace.WithAttributes(attribute.String("err", err.Error())))
			klog.Error(err)
			cancel()
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
