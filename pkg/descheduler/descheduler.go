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
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"
	fakemetricsclientset "k8s.io/metrics/pkg/client/clientset/versioned/fake"
	"math"
	"sigs.k8s.io/descheduler/pkg/descheduler/cache"
	"strconv"
	"strings"
	"time"

	componentbaseconfig "k8s.io/component-base/config"
	"k8s.io/klog/v2"

	v1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	listersv1 "k8s.io/client-go/listers/core/v1"
	schedulingv1 "k8s.io/client-go/listers/scheduling/v1"
	core "k8s.io/client-go/testing"

	"sigs.k8s.io/descheduler/cmd/descheduler/app/options"
	"sigs.k8s.io/descheduler/metrics"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/client"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	eutils "sigs.k8s.io/descheduler/pkg/descheduler/evictions/utils"
	nodeutil "sigs.k8s.io/descheduler/pkg/descheduler/node"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/framework"
	"sigs.k8s.io/descheduler/pkg/framework/pluginregistry"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	"sigs.k8s.io/descheduler/pkg/utils"
	"sigs.k8s.io/descheduler/pkg/version"
)

type enabledDeschedulePluginEntry struct {
	Plugin  framework.DeschedulePlugin
	Profile string
}

type enabledBalancePluginEntry struct {
	Plugin  framework.BalancePlugin
	Profile string
}

func Run(ctx context.Context, rs *options.DeschedulerServer) error {
	metrics.Register()

	clientConnection := rs.ClientConnection
	if rs.KubeconfigFile != "" && clientConnection.Kubeconfig == "" {
		clientConnection.Kubeconfig = rs.KubeconfigFile
	}
	rsclient, eventClient, err := createClients(clientConnection)
	if err != nil {
		return err
	}
	rs.MetricsClient, err = client.CreateMetricsClient(clientConnection)
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
	versionCompatibilityCheck(rs)

	evictionPolicyGroupVersion, err := eutils.SupportEviction(rs.Client)
	if err != nil || len(evictionPolicyGroupVersion) == 0 {
		return err
	}

	runFn := func() error {
		return RunDeschedulerStrategies(ctx, rs, deschedulerPolicy, evictionPolicyGroupVersion)
	}

	if rs.LeaderElection.LeaderElect && rs.DeschedulingInterval.Seconds() == 0 {
		return fmt.Errorf("leaderElection must be used with deschedulingInterval")
	}

	if rs.LeaderElection.LeaderElect && rs.DryRun {
		klog.V(1).InfoS("Warning: DryRun is set to True. You need to disable it to use Leader Election.")
	}

	if rs.LeaderElection.LeaderElect && !rs.DryRun {
		if err := NewLeaderElection(runFn, rsclient, &rs.LeaderElection, ctx); err != nil {
			return fmt.Errorf("leaderElection: %w", err)
		}
		return nil
	}

	return runFn()
}

func versionCompatibilityCheck(rs *options.DeschedulerServer) {
	serverVersion, serverErr := rs.Client.Discovery().ServerVersion()
	if serverErr != nil {
		klog.V(1).InfoS("Warning: Get Kubernetes server version fail")
		return
	}

	deschedulerMinorVersion := strings.Split(version.Get().Minor, ".")[0]
	deschedulerMinorVersionFloat, err := strconv.ParseFloat(deschedulerMinorVersion, 64)
	if err != nil {
		klog.Warning("Warning: Convert Descheduler minor version to float fail")
	}

	kubernetesMinorVersionFloat, err := strconv.ParseFloat(serverVersion.Minor, 64)
	if err != nil {
		klog.Warning("Warning: Convert Kubernetes server minor version to float fail")
	}

	if math.Abs(deschedulerMinorVersionFloat-kubernetesMinorVersionFloat) > 3 {
		klog.Warningf("Warning: Descheduler minor version %v is not supported on your version of Kubernetes %v.%v. See compatibility docs for more info: https://github.com/kubernetes-sigs/descheduler#compatibility-matrix", deschedulerMinorVersion, serverVersion.Major, serverVersion.Minor)
	}
}

func cachedMetricsClient() (metricsclientset.Interface, error) {
	fakeMetricsClient := fakemetricsclientset.NewSimpleClientset()
	fakeMetricsClient.PrependReactor("list", "*", func(action core.Action) (handled bool, ret runtime.Object, err error) {
		switch action := action.(type) {
		case core.ListActionImpl:
			gvr := action.GetResource()
			if gvr.Resource == "nodes" {
				gvr.Resource = "nodemetricses"
			}
			if gvr.Resource == "pods" {
				gvr.Resource = "podmetricses"
			}
			ns := action.GetNamespace()
			obj, err := fakeMetricsClient.Tracker().List(gvr, action.GetKind(), ns)
			return true, obj, err
		}
		return false, nil, err
	})
	_, err := fakeMetricsClient.MetricsV1beta1().NodeMetricses().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("unable to list node metrics")
	}
	_, err = fakeMetricsClient.MetricsV1beta1().PodMetricses("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("unable to list pod metrics")
	}
	return fakeMetricsClient, nil
}
func cachedClient(
	realClient clientset.Interface,
	podLister listersv1.PodLister,
	nodeLister listersv1.NodeLister,
	namespaceLister listersv1.NamespaceLister,
	priorityClassLister schedulingv1.PriorityClassLister,
) (clientset.Interface, error) {
	fakeClient := fakeclientset.NewSimpleClientset()
	// simulate a pod eviction by deleting a pod
	fakeClient.PrependReactor("create", "pods", func(action core.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() == "eviction" {
			createAct, matched := action.(core.CreateActionImpl)
			if !matched {
				return false, nil, fmt.Errorf("unable to convert action to core.CreateActionImpl")
			}
			eviction, matched := createAct.Object.(*policy.Eviction)
			if !matched {
				return false, nil, fmt.Errorf("unable to convert action object into *policy.Eviction")
			}
			if err := fakeClient.Tracker().Delete(action.GetResource(), eviction.GetNamespace(), eviction.GetName()); err != nil {
				return false, nil, fmt.Errorf("unable to delete pod %v/%v: %v", eviction.GetNamespace(), eviction.GetName(), err)
			}
			return true, nil, nil
		}
		// fallback to the default reactor
		return false, nil, nil
	})

	klog.V(3).Infof("Pulling resources for the cached client from the cluster")
	pods, err := podLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("unable to list pods: %v", err)
	}

	for _, item := range pods {
		if _, err := fakeClient.CoreV1().Pods(item.Namespace).Create(context.TODO(), item, metav1.CreateOptions{}); err != nil {
			return nil, fmt.Errorf("unable to copy pod: %v", err)
		}
	}

	nodes, err := nodeLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("unable to list nodes: %v", err)
	}

	for _, item := range nodes {
		if _, err := fakeClient.CoreV1().Nodes().Create(context.TODO(), item, metav1.CreateOptions{}); err != nil {
			return nil, fmt.Errorf("unable to copy node: %v", err)
		}
	}

	namespaces, err := namespaceLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("unable to list namespaces: %v", err)
	}

	for _, item := range namespaces {
		if _, err := fakeClient.CoreV1().Namespaces().Create(context.TODO(), item, metav1.CreateOptions{}); err != nil {
			return nil, fmt.Errorf("unable to copy node: %v", err)
		}
	}

	priorityClasses, err := priorityClassLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("unable to list priorityclasses: %v", err)
	}

	for _, item := range priorityClasses {
		if _, err := fakeClient.SchedulingV1().PriorityClasses().Create(context.TODO(), item, metav1.CreateOptions{}); err != nil {
			return nil, fmt.Errorf("unable to copy priorityclass: %v", err)
		}
	}

	return fakeClient, nil
}

// evictorImpl implements the Evictor interface so plugins
// can evict a pod without importing a specific pod evictor
type evictorImpl struct {
	podEvictor    *evictions.PodEvictor
	evictorFilter framework.EvictorPlugin
}

var _ framework.Evictor = &evictorImpl{}

// Filter checks if a pod can be evicted
func (ei *evictorImpl) Filter(pod *v1.Pod) bool {
	return ei.evictorFilter.Filter(pod)
}

// PreEvictionFilter checks if pod can be evicted right before eviction
func (ei *evictorImpl) PreEvictionFilter(pod *v1.Pod) bool {
	return ei.evictorFilter.PreEvictionFilter(pod)
}

// Evict evicts a pod (no pre-check performed)
func (ei *evictorImpl) Evict(ctx context.Context, pod *v1.Pod, opts evictions.EvictOptions) bool {
	return ei.podEvictor.EvictPod(ctx, pod, opts)
}

func (ei *evictorImpl) NodeLimitExceeded(node *v1.Node) bool {
	return ei.podEvictor.NodeLimitExceeded(node)
}

// handleImpl implements the framework handle which gets passed to plugins
type handleImpl struct {
	clientSet                 clientset.Interface
	getPodsAssignedToNodeFunc podutil.GetPodsAssignedToNodeFunc
	sharedInformerFactory     informers.SharedInformerFactory
	evictor                   *evictorImpl
}

var _ framework.Handle = &handleImpl{}

// ClientSet retrieves kube client set
func (hi *handleImpl) ClientSet() clientset.Interface {
	return hi.clientSet
}

// GetPodsAssignedToNodeFunc retrieves GetPodsAssignedToNodeFunc implementation
func (hi *handleImpl) GetPodsAssignedToNodeFunc() podutil.GetPodsAssignedToNodeFunc {
	return hi.getPodsAssignedToNodeFunc
}

// SharedInformerFactory retrieves shared informer factory
func (hi *handleImpl) SharedInformerFactory() informers.SharedInformerFactory {
	return hi.sharedInformerFactory
}

// Evictor retrieves evictor so plugins can filter and evict pods
func (hi *handleImpl) Evictor() framework.Evictor {
	return hi.evictor
}

func RunDeschedulerStrategies(ctx context.Context, rs *options.DeschedulerServer, deschedulerPolicy *api.DeschedulerPolicy, evictionPolicyGroupVersion string) error {
	sharedInformerFactory := informers.NewSharedInformerFactory(rs.Client, 0)
	podInformer := sharedInformerFactory.Core().V1().Pods().Informer()
	podLister := sharedInformerFactory.Core().V1().Pods().Lister()
	nodeLister := sharedInformerFactory.Core().V1().Nodes().Lister()
	nodeInformer := sharedInformerFactory.Core().V1().Nodes().Informer()
	namespaceLister := sharedInformerFactory.Core().V1().Namespaces().Lister()
	priorityClassLister := sharedInformerFactory.Scheduling().V1().PriorityClasses().Lister()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	getPodsAssignedToNode, err := podutil.BuildGetPodsAssignedToNodeFunc(podInformer)
	if err != nil {
		return fmt.Errorf("build get pods assigned to node function error: %v", err)
	}

	var icache cache.BasicCache
	var metricsClient metricsclientset.Interface
	if rs.DryRun {
		metricsClient = fakemetricsclientset.NewSimpleClientset()
	} else {
		metricsClient = rs.MetricsClient
	}

	// init real metrics cache before start informerFactory
	icache, err = cache.InitCache(rs.Client, nodeInformer, nodeLister, podInformer, metricsClient, ctx.Done())
	if err != nil {
		cancel()
		klog.V(1).Infof("init metrics cache failed")
	}
	sharedInformerFactory.Start(ctx.Done())
	sharedInformerFactory.WaitForCacheSync(ctx.Done())

	var nodeSelector string
	if deschedulerPolicy.NodeSelector != nil {
		nodeSelector = *deschedulerPolicy.NodeSelector
	}

	var eventClient clientset.Interface
	if rs.DryRun {
		eventClient = fakeclientset.NewSimpleClientset()
	} else {
		eventClient = rs.Client
	}

	eventBroadcaster, eventRecorder := utils.GetRecorderAndBroadcaster(ctx, eventClient)
	defer eventBroadcaster.Shutdown()

	wait.NonSlidingUntil(func() {
		loopStartDuration := time.Now()
		defer metrics.DeschedulerLoopDuration.With(map[string]string{}).Observe(time.Since(loopStartDuration).Seconds())
		nodes, err := nodeutil.ReadyNodes(ctx, rs.Client, nodeLister, nodeSelector)
		if err != nil {
			klog.V(1).InfoS("Unable to get ready nodes", "err", err)
			cancel()
			return
		}

		if len(nodes) <= 1 {
			klog.V(1).InfoS("The cluster size is 0 or 1 meaning eviction causes service disruption or degradation. So aborting..")
			cancel()
			return
		}

		var podEvictorClient clientset.Interface
		// When the dry mode is enable, collect all the relevant objects (mostly pods) under a fake client.
		// So when evicting pods while running multiple strategies in a row have the cummulative effect
		// as is when evicting pods for real.
		if rs.DryRun {
			klog.V(3).Infof("Building a cached client from the cluster for the dry run")
			// Create a new cache so we start from scratch without any leftovers
			fakeClient, err := cachedClient(rs.Client, podLister, nodeLister, namespaceLister, priorityClassLister)
			if err != nil {
				klog.Error(err)
				return
			}

			fakeSharedInformerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
			getPodsAssignedToNode, err = podutil.BuildGetPodsAssignedToNodeFunc(fakeSharedInformerFactory.Core().V1().Pods().Informer())
			if err != nil {
				klog.Errorf("build get pods assigned to node function error: %v", err)
				return
			}

			fakeCtx, cncl := context.WithCancel(context.TODO())
			defer cncl()
			fakeSharedInformerFactory.Start(fakeCtx.Done())
			fakeSharedInformerFactory.WaitForCacheSync(fakeCtx.Done())

			podEvictorClient = fakeClient
		} else {
			podEvictorClient = rs.Client
		}
		//run cache
		icache.Run(rs.MetricsCacheSyncInterval)

		klog.V(3).Infof("Building a pod evictor")
		podEvictor := evictions.NewPodEvictor(
			podEvictorClient,
			evictionPolicyGroupVersion,
			rs.DryRun,
			deschedulerPolicy.MaxNoOfPodsToEvictPerNode,
			deschedulerPolicy.MaxNoOfPodsToEvictPerNamespace,
			nodes,
			!rs.DisableMetrics,
			eventRecorder,
		)

		var enabledDeschedulePlugins []enabledDeschedulePluginEntry
		var enabledBalancePlugins []enabledBalancePluginEntry

		// Build plugins
		for _, profile := range deschedulerPolicy.Profiles {
			pc, _ := GetPluginConfig(defaultevictor.PluginName, profile.PluginConfigs)
			if pc == nil {
				klog.ErrorS(fmt.Errorf("unable to get plugin config"), "skipping plugin", "plugin", defaultevictor.PluginName, "profile", profile.Name)
				continue
			}
			evictorFilter, err := defaultevictor.New(
				pc.Args,
				&handleImpl{
					clientSet:                 rs.Client,
					getPodsAssignedToNodeFunc: getPodsAssignedToNode,
					sharedInformerFactory:     sharedInformerFactory,
				},
			)
			if err != nil {
				klog.ErrorS(fmt.Errorf("unable to construct a plugin"), "skipping plugin", "plugin", defaultevictor.PluginName)
				continue
			}
			handle := &handleImpl{
				clientSet:                 rs.Client,
				getPodsAssignedToNodeFunc: getPodsAssignedToNode,
				sharedInformerFactory:     sharedInformerFactory,
				evictor: &evictorImpl{
					podEvictor:    podEvictor,
					evictorFilter: evictorFilter.(framework.EvictorPlugin),
				},
			}
			// Assuming only a list of enabled extension points.
			// Later, when a default list of plugins and their extension points is established,
			// compute the list of enabled extension points as (DefaultEnabled + Enabled - Disabled)
			for _, plugin := range append(profile.Plugins.Deschedule.Enabled, profile.Plugins.Balance.Enabled...) {
				pc, _ := GetPluginConfig(plugin, profile.PluginConfigs)
				if pc == nil {
					klog.ErrorS(fmt.Errorf("unable to get plugin config"), "skipping plugin", "plugin", plugin)
					continue
				}
				registryPlugin, ok := pluginregistry.PluginRegistry[plugin]
				pgFnc := registryPlugin.PluginBuilder
				if !ok {
					klog.ErrorS(fmt.Errorf("unable to find plugin in the pluginsMap"), "skipping plugin", "plugin", plugin)
				}
				pg, err := pgFnc(pc.Args, handle)
				if err != nil {
					klog.ErrorS(err, "unable to initialize a plugin", "pluginName", plugin)
				}
				if pg != nil {
					// pg can be of any of each type, or both
					enabledDeschedulePlugins, enabledBalancePlugins = includeProfilePluginsByType(enabledDeschedulePlugins, enabledBalancePlugins, pg, profile.Name)
				}
			}
		}

		// Execute extension points
		for _, pg := range enabledDeschedulePlugins {
			// TODO: strategyName should be accessible from within the strategy using a framework
			// handle or function which the Evictor has access to. For migration/in-progress framework
			// work, we are currently passing this via context. To be removed
			// (See discussion thread https://github.com/kubernetes-sigs/descheduler/pull/885#discussion_r919962292)
			strategyStart := time.Now()
			childCtx := context.WithValue(ctx, "strategyName", pg.Plugin.Name())
			status := pg.Plugin.Deschedule(childCtx, nodes)
			metrics.DeschedulerStrategyDuration.With(map[string]string{"strategy": pg.Plugin.Name(), "profile": pg.Profile}).Observe(time.Since(strategyStart).Seconds())
			if status != nil && status.Err != nil {
				klog.ErrorS(status.Err, "plugin finished with error", "pluginName", pg.Plugin.Name())
			}
		}

		for _, pg := range enabledBalancePlugins {
			// TODO: strategyName should be accessible from within the strategy using a framework
			// handle or function which the Evictor has access to. For migration/in-progress framework
			// work, we are currently passing this via context. To be removed
			// (See discussion thread https://github.com/kubernetes-sigs/descheduler/pull/885#discussion_r919962292)
			strategyStart := time.Now()
			childCtx := context.WithValue(ctx, "strategyName", pg.Plugin.Name())
			status := pg.Plugin.Balance(childCtx, nodes)
			metrics.DeschedulerStrategyDuration.With(map[string]string{"strategy": pg.Plugin.Name(), "profile": pg.Profile}).Observe(time.Since(strategyStart).Seconds())
			if status != nil && status.Err != nil {
				klog.ErrorS(status.Err, "plugin finished with error", "pluginName", pg.Plugin.Name())
			}
		}

		klog.V(1).InfoS("Number of evicted pods", "totalEvicted", podEvictor.TotalEvicted())

		// If there was no interval specified, send a signal to the stopChannel to end the wait.Until loop after 1 iteration
		if rs.DeschedulingInterval.Seconds() == 0 {
			cancel()
		}
	}, rs.DeschedulingInterval, ctx.Done())

	return nil
}

func includeProfilePluginsByType(enabledDeschedulePlugins []enabledDeschedulePluginEntry, enabledBalancePlugins []enabledBalancePluginEntry, pg framework.Plugin, profile string) ([]enabledDeschedulePluginEntry, []enabledBalancePluginEntry) {
	enabledDeschedulePlugins = includeDeschedule(enabledDeschedulePlugins, pg, profile)
	enabledBalancePlugins = includeBalance(enabledBalancePlugins, pg, profile)
	return enabledDeschedulePlugins, enabledBalancePlugins
}

func includeDeschedule(enabledDeschedulePlugins []enabledDeschedulePluginEntry, pg framework.Plugin, profile string) []enabledDeschedulePluginEntry {
	_, ok := pg.(framework.DeschedulePlugin)
	if ok {
		enabledDeschedulePlugins = append(enabledDeschedulePlugins, enabledDeschedulePluginEntry{Plugin: pg.(framework.DeschedulePlugin), Profile: profile})
	}
	return enabledDeschedulePlugins
}

func includeBalance(enabledBalancePlugins []enabledBalancePluginEntry, pg framework.Plugin, profile string) []enabledBalancePluginEntry {
	_, ok := pg.(framework.BalancePlugin)
	if ok {
		enabledBalancePlugins = append(enabledBalancePlugins, enabledBalancePluginEntry{Plugin: pg.(framework.BalancePlugin), Profile: profile})
	}
	return enabledBalancePlugins
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
