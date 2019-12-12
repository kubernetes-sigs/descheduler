/*
Copyright 2019 The Kubernetes Authors.

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

package v1alpha1

import (
	"context"
	"fmt"
	"reflect"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/scheduler/apis/config"
	schedulerlisters "k8s.io/kubernetes/pkg/scheduler/listers"
	"k8s.io/kubernetes/pkg/scheduler/metrics"
	schedulernodeinfo "k8s.io/kubernetes/pkg/scheduler/nodeinfo"
	schedutil "k8s.io/kubernetes/pkg/scheduler/util"
)

const (
	// Specifies the maximum timeout a permit plugin can return.
	maxTimeout                  time.Duration = 15 * time.Minute
	preFilter                                 = "PreFilter"
	preFilterExtensionAddPod                  = "PreFilterExtensionAddPod"
	preFilterExtensionRemovePod               = "PreFilterExtensionRemovePod"
	filter                                    = "Filter"
	postFilter                                = "PostFilter"
	score                                     = "Score"
	scoreExtensionNormalize                   = "ScoreExtensionNormalize"
	preBind                                   = "PreBind"
	bind                                      = "Bind"
	postBind                                  = "PostBind"
	reserve                                   = "Reserve"
	unreserve                                 = "Unreserve"
	permit                                    = "Permit"
)

// framework is the component responsible for initializing and running scheduler
// plugins.
type framework struct {
	registry              Registry
	snapshotSharedLister  schedulerlisters.SharedLister
	waitingPods           *waitingPodsMap
	pluginNameToWeightMap map[string]int
	queueSortPlugins      []QueueSortPlugin
	preFilterPlugins      []PreFilterPlugin
	filterPlugins         []FilterPlugin
	postFilterPlugins     []PostFilterPlugin
	scorePlugins          []ScorePlugin
	reservePlugins        []ReservePlugin
	preBindPlugins        []PreBindPlugin
	bindPlugins           []BindPlugin
	postBindPlugins       []PostBindPlugin
	unreservePlugins      []UnreservePlugin
	permitPlugins         []PermitPlugin

	clientSet       clientset.Interface
	informerFactory informers.SharedInformerFactory

	metricsRecorder *metricsRecorder
}

// extensionPoint encapsulates desired and applied set of plugins at a specific extension
// point. This is used to simplify iterating over all extension points supported by the
// framework.
type extensionPoint struct {
	// the set of plugins to be configured at this extension point.
	plugins *config.PluginSet
	// a pointer to the slice storing plugins implementations that will run at this
	// extenstion point.
	slicePtr interface{}
}

func (f *framework) getExtensionPoints(plugins *config.Plugins) []extensionPoint {
	return []extensionPoint{
		{plugins.PreFilter, &f.preFilterPlugins},
		{plugins.Filter, &f.filterPlugins},
		{plugins.Reserve, &f.reservePlugins},
		{plugins.PostFilter, &f.postFilterPlugins},
		{plugins.Score, &f.scorePlugins},
		{plugins.PreBind, &f.preBindPlugins},
		{plugins.Bind, &f.bindPlugins},
		{plugins.PostBind, &f.postBindPlugins},
		{plugins.Unreserve, &f.unreservePlugins},
		{plugins.Permit, &f.permitPlugins},
		{plugins.QueueSort, &f.queueSortPlugins},
	}
}

type frameworkOptions struct {
	clientSet            clientset.Interface
	informerFactory      informers.SharedInformerFactory
	snapshotSharedLister schedulerlisters.SharedLister
	metricsRecorder      *metricsRecorder
}

// Option for the framework.
type Option func(*frameworkOptions)

// WithClientSet sets clientSet for the scheduling framework.
func WithClientSet(clientSet clientset.Interface) Option {
	return func(o *frameworkOptions) {
		o.clientSet = clientSet
	}
}

// WithInformerFactory sets informer factory for the scheduling framework.
func WithInformerFactory(informerFactory informers.SharedInformerFactory) Option {
	return func(o *frameworkOptions) {
		o.informerFactory = informerFactory
	}
}

// WithSnapshotSharedLister sets the SharedLister of the snapshot.
func WithSnapshotSharedLister(snapshotSharedLister schedulerlisters.SharedLister) Option {
	return func(o *frameworkOptions) {
		o.snapshotSharedLister = snapshotSharedLister
	}
}

// withMetricsRecorder is only used in tests.
func withMetricsRecorder(recorder *metricsRecorder) Option {
	return func(o *frameworkOptions) {
		o.metricsRecorder = recorder
	}
}

var defaultFrameworkOptions = frameworkOptions{
	metricsRecorder: newMetricsRecorder(1000, time.Second),
}

var _ Framework = &framework{}

// NewFramework initializes plugins given the configuration and the registry.
func NewFramework(r Registry, plugins *config.Plugins, args []config.PluginConfig, opts ...Option) (Framework, error) {
	options := defaultFrameworkOptions
	for _, opt := range opts {
		opt(&options)
	}

	f := &framework{
		registry:              r,
		snapshotSharedLister:  options.snapshotSharedLister,
		pluginNameToWeightMap: make(map[string]int),
		waitingPods:           newWaitingPodsMap(),
		clientSet:             options.clientSet,
		informerFactory:       options.informerFactory,
		metricsRecorder:       options.metricsRecorder,
	}
	if plugins == nil {
		return f, nil
	}

	// get needed plugins from config
	pg := f.pluginsNeeded(plugins)
	if len(pg) == 0 {
		return f, nil
	}

	pluginConfig := make(map[string]*runtime.Unknown, 0)
	for i := range args {
		pluginConfig[args[i].Name] = &args[i].Args
	}

	pluginsMap := make(map[string]Plugin)
	for name, factory := range r {
		// initialize only needed plugins.
		if _, ok := pg[name]; !ok {
			continue
		}

		p, err := factory(pluginConfig[name], f)
		if err != nil {
			return nil, fmt.Errorf("error initializing plugin %q: %v", name, err)
		}
		pluginsMap[name] = p

		// a weight of zero is not permitted, plugins can be disabled explicitly
		// when configured.
		f.pluginNameToWeightMap[name] = int(pg[name].Weight)
		if f.pluginNameToWeightMap[name] == 0 {
			f.pluginNameToWeightMap[name] = 1
		}
	}

	for _, e := range f.getExtensionPoints(plugins) {
		if err := updatePluginList(e.slicePtr, e.plugins, pluginsMap); err != nil {
			return nil, err
		}
	}

	// Verifying the score weights again since Plugin.Name() could return a different
	// value from the one used in the configuration.
	for _, scorePlugin := range f.scorePlugins {
		if f.pluginNameToWeightMap[scorePlugin.Name()] == 0 {
			return nil, fmt.Errorf("score plugin %q is not configured with weight", scorePlugin.Name())
		}
	}

	if len(f.queueSortPlugins) > 1 {
		return nil, fmt.Errorf("only one queue sort plugin can be enabled")
	}

	return f, nil
}

func updatePluginList(pluginList interface{}, pluginSet *config.PluginSet, pluginsMap map[string]Plugin) error {
	if pluginSet == nil {
		return nil
	}

	plugins := reflect.ValueOf(pluginList).Elem()
	pluginType := plugins.Type().Elem()
	set := sets.NewString()
	for _, ep := range pluginSet.Enabled {
		pg, ok := pluginsMap[ep.Name]
		if !ok {
			return fmt.Errorf("%s %q does not exist", pluginType.Name(), ep.Name)
		}

		if !reflect.TypeOf(pg).Implements(pluginType) {
			return fmt.Errorf("plugin %q does not extend %s plugin", ep.Name, pluginType.Name())
		}

		if set.Has(ep.Name) {
			return fmt.Errorf("plugin %q already registered as %q", ep.Name, pluginType.Name())
		}

		set.Insert(ep.Name)

		newPlugins := reflect.Append(plugins, reflect.ValueOf(pg))
		plugins.Set(newPlugins)
	}
	return nil
}

// QueueSortFunc returns the function to sort pods in scheduling queue
func (f *framework) QueueSortFunc() LessFunc {
	if len(f.queueSortPlugins) == 0 {
		return nil
	}

	// Only one QueueSort plugin can be enabled.
	return f.queueSortPlugins[0].Less
}

// RunPreFilterPlugins runs the set of configured PreFilter plugins. It returns
// *Status and its code is set to non-success if any of the plugins returns
// anything but Success. If a non-success status is returned, then the scheduling
// cycle is aborted.
func (f *framework) RunPreFilterPlugins(ctx context.Context, state *CycleState, pod *v1.Pod) (status *Status) {
	if state.ShouldRecordFrameworkMetrics() {
		startTime := time.Now()
		defer func() {
			f.metricsRecorder.observeExtensionPointDurationAsync(preFilter, status, metrics.SinceInSeconds(startTime))
		}()
	}
	for _, pl := range f.preFilterPlugins {
		status = f.runPreFilterPlugin(ctx, pl, state, pod)
		if !status.IsSuccess() {
			if status.IsUnschedulable() {
				msg := fmt.Sprintf("rejected by %q at prefilter: %v", pl.Name(), status.Message())
				klog.V(4).Infof(msg)
				return NewStatus(status.Code(), msg)
			}
			msg := fmt.Sprintf("error while running %q prefilter plugin for pod %q: %v", pl.Name(), pod.Name, status.Message())
			klog.Error(msg)
			return NewStatus(Error, msg)
		}
	}

	return nil
}

func (f *framework) runPreFilterPlugin(ctx context.Context, pl PreFilterPlugin, state *CycleState, pod *v1.Pod) *Status {
	if !state.ShouldRecordFrameworkMetrics() {
		return pl.PreFilter(ctx, state, pod)
	}
	startTime := time.Now()
	status := pl.PreFilter(ctx, state, pod)
	f.metricsRecorder.observePluginDurationAsync(preFilter, pl.Name(), status, metrics.SinceInSeconds(startTime))
	return status
}

// RunPreFilterExtensionAddPod calls the AddPod interface for the set of configured
// PreFilter plugins. It returns directly if any of the plugins return any
// status other than Success.
func (f *framework) RunPreFilterExtensionAddPod(
	ctx context.Context,
	state *CycleState,
	podToSchedule *v1.Pod,
	podToAdd *v1.Pod,
	nodeInfo *schedulernodeinfo.NodeInfo,
) (status *Status) {
	if state.ShouldRecordFrameworkMetrics() {
		startTime := time.Now()
		defer func() {
			f.metricsRecorder.observeExtensionPointDurationAsync(preFilterExtensionAddPod, status, metrics.SinceInSeconds(startTime))
		}()
	}
	for _, pl := range f.preFilterPlugins {
		if pl.PreFilterExtensions() == nil {
			continue
		}
		status = f.runPreFilterExtensionAddPod(ctx, pl, state, podToSchedule, podToAdd, nodeInfo)
		if !status.IsSuccess() {
			msg := fmt.Sprintf("error while running AddPod for plugin %q while scheduling pod %q: %v",
				pl.Name(), podToSchedule.Name, status.Message())
			klog.Error(msg)
			return NewStatus(Error, msg)
		}
	}

	return nil
}

func (f *framework) runPreFilterExtensionAddPod(ctx context.Context, pl PreFilterPlugin, state *CycleState, podToSchedule *v1.Pod, podToAdd *v1.Pod, nodeInfo *schedulernodeinfo.NodeInfo) *Status {
	if !state.ShouldRecordFrameworkMetrics() {
		return pl.PreFilterExtensions().AddPod(ctx, state, podToSchedule, podToAdd, nodeInfo)
	}
	startTime := time.Now()
	status := pl.PreFilterExtensions().AddPod(ctx, state, podToSchedule, podToAdd, nodeInfo)
	f.metricsRecorder.observePluginDurationAsync(preFilterExtensionAddPod, pl.Name(), status, metrics.SinceInSeconds(startTime))
	return status
}

// RunPreFilterExtensionRemovePod calls the RemovePod interface for the set of configured
// PreFilter plugins. It returns directly if any of the plugins return any
// status other than Success.
func (f *framework) RunPreFilterExtensionRemovePod(
	ctx context.Context,
	state *CycleState,
	podToSchedule *v1.Pod,
	podToRemove *v1.Pod,
	nodeInfo *schedulernodeinfo.NodeInfo,
) (status *Status) {
	if state.ShouldRecordFrameworkMetrics() {
		startTime := time.Now()
		defer func() {
			f.metricsRecorder.observeExtensionPointDurationAsync(preFilterExtensionRemovePod, status, metrics.SinceInSeconds(startTime))
		}()
	}
	for _, pl := range f.preFilterPlugins {
		if pl.PreFilterExtensions() == nil {
			continue
		}
		status = f.runPreFilterExtensionRemovePod(ctx, pl, state, podToSchedule, podToRemove, nodeInfo)
		if !status.IsSuccess() {
			msg := fmt.Sprintf("error while running RemovePod for plugin %q while scheduling pod %q: %v",
				pl.Name(), podToSchedule.Name, status.Message())
			klog.Error(msg)
			return NewStatus(Error, msg)
		}
	}

	return nil
}

func (f *framework) runPreFilterExtensionRemovePod(ctx context.Context, pl PreFilterPlugin, state *CycleState, podToSchedule *v1.Pod, podToAdd *v1.Pod, nodeInfo *schedulernodeinfo.NodeInfo) *Status {
	if !state.ShouldRecordFrameworkMetrics() {
		return pl.PreFilterExtensions().RemovePod(ctx, state, podToSchedule, podToAdd, nodeInfo)
	}
	startTime := time.Now()
	status := pl.PreFilterExtensions().RemovePod(ctx, state, podToSchedule, podToAdd, nodeInfo)
	f.metricsRecorder.observePluginDurationAsync(preFilterExtensionRemovePod, pl.Name(), status, metrics.SinceInSeconds(startTime))
	return status
}

// RunFilterPlugins runs the set of configured Filter plugins for pod on
// the given node. If any of these plugins doesn't return "Success", the
// given node is not suitable for running pod.
// Meanwhile, the failure message and status are set for the given node.
func (f *framework) RunFilterPlugins(
	ctx context.Context,
	state *CycleState,
	pod *v1.Pod,
	nodeInfo *schedulernodeinfo.NodeInfo,
) (status *Status) {
	if state.ShouldRecordFrameworkMetrics() {
		startTime := time.Now()
		defer func() {
			f.metricsRecorder.observeExtensionPointDurationAsync(filter, status, metrics.SinceInSeconds(startTime))
		}()
	}
	for _, pl := range f.filterPlugins {
		status = f.runFilterPlugin(ctx, pl, state, pod, nodeInfo)
		if !status.IsSuccess() {
			if !status.IsUnschedulable() {
				errMsg := fmt.Sprintf("error while running %q filter plugin for pod %q: %v",
					pl.Name(), pod.Name, status.Message())
				klog.Error(errMsg)
				return NewStatus(Error, errMsg)
			}
			return status
		}
	}

	return nil
}

func (f *framework) runFilterPlugin(ctx context.Context, pl FilterPlugin, state *CycleState, pod *v1.Pod, nodeInfo *schedulernodeinfo.NodeInfo) *Status {
	if !state.ShouldRecordFrameworkMetrics() {
		return pl.Filter(ctx, state, pod, nodeInfo)
	}
	startTime := time.Now()
	status := pl.Filter(ctx, state, pod, nodeInfo)
	f.metricsRecorder.observePluginDurationAsync(filter, pl.Name(), status, metrics.SinceInSeconds(startTime))
	return status
}

// RunPostFilterPlugins runs the set of configured post-filter plugins. If any
// of these plugins returns any status other than "Success", the given node is
// rejected. The filteredNodeStatuses is the set of filtered nodes and their statuses.
func (f *framework) RunPostFilterPlugins(
	ctx context.Context,
	state *CycleState,
	pod *v1.Pod,
	nodes []*v1.Node,
	filteredNodesStatuses NodeToStatusMap,
) (status *Status) {
	if state.ShouldRecordFrameworkMetrics() {
		startTime := time.Now()
		defer func() {
			f.metricsRecorder.observeExtensionPointDurationAsync(postFilter, status, metrics.SinceInSeconds(startTime))
		}()
	}
	for _, pl := range f.postFilterPlugins {
		status = f.runPostFilterPlugin(ctx, pl, state, pod, nodes, filteredNodesStatuses)
		if !status.IsSuccess() {
			msg := fmt.Sprintf("error while running %q postfilter plugin for pod %q: %v", pl.Name(), pod.Name, status.Message())
			klog.Error(msg)
			return NewStatus(Error, msg)
		}
	}

	return nil
}

func (f *framework) runPostFilterPlugin(ctx context.Context, pl PostFilterPlugin, state *CycleState, pod *v1.Pod, nodes []*v1.Node, filteredNodesStatuses NodeToStatusMap) *Status {
	if !state.ShouldRecordFrameworkMetrics() {
		return pl.PostFilter(ctx, state, pod, nodes, filteredNodesStatuses)
	}
	startTime := time.Now()
	status := pl.PostFilter(ctx, state, pod, nodes, filteredNodesStatuses)
	f.metricsRecorder.observePluginDurationAsync(postFilter, pl.Name(), status, metrics.SinceInSeconds(startTime))
	return status
}

// RunScorePlugins runs the set of configured scoring plugins. It returns a list that
// stores for each scoring plugin name the corresponding NodeScoreList(s).
// It also returns *Status, which is set to non-success if any of the plugins returns
// a non-success status.
func (f *framework) RunScorePlugins(ctx context.Context, state *CycleState, pod *v1.Pod, nodes []*v1.Node) (ps PluginToNodeScores, status *Status) {
	if state.ShouldRecordFrameworkMetrics() {
		startTime := time.Now()
		defer func() {
			f.metricsRecorder.observeExtensionPointDurationAsync(score, status, metrics.SinceInSeconds(startTime))
		}()
	}
	pluginToNodeScores := make(PluginToNodeScores, len(f.scorePlugins))
	for _, pl := range f.scorePlugins {
		pluginToNodeScores[pl.Name()] = make(NodeScoreList, len(nodes))
	}
	ctx, cancel := context.WithCancel(ctx)
	errCh := schedutil.NewErrorChannel()

	// Run Score method for each node in parallel.
	workqueue.ParallelizeUntil(ctx, 16, len(nodes), func(index int) {
		for _, pl := range f.scorePlugins {
			nodeName := nodes[index].Name
			s, status := f.runScorePlugin(ctx, pl, state, pod, nodeName)
			if !status.IsSuccess() {
				errCh.SendErrorWithCancel(fmt.Errorf(status.Message()), cancel)
				return
			}
			pluginToNodeScores[pl.Name()][index] = NodeScore{
				Name:  nodeName,
				Score: int64(s),
			}
		}
	})
	if err := errCh.ReceiveError(); err != nil {
		msg := fmt.Sprintf("error while running score plugin for pod %q: %v", pod.Name, err)
		klog.Error(msg)
		return nil, NewStatus(Error, msg)
	}

	// Run NormalizeScore method for each ScorePlugin in parallel.
	workqueue.ParallelizeUntil(ctx, 16, len(f.scorePlugins), func(index int) {
		pl := f.scorePlugins[index]
		nodeScoreList := pluginToNodeScores[pl.Name()]
		if pl.ScoreExtensions() == nil {
			return
		}
		status := f.runScoreExtension(ctx, pl, state, pod, nodeScoreList)
		if !status.IsSuccess() {
			err := fmt.Errorf("normalize score plugin %q failed with error %v", pl.Name(), status.Message())
			errCh.SendErrorWithCancel(err, cancel)
			return
		}
	})
	if err := errCh.ReceiveError(); err != nil {
		msg := fmt.Sprintf("error while running normalize score plugin for pod %q: %v", pod.Name, err)
		klog.Error(msg)
		return nil, NewStatus(Error, msg)
	}

	// Apply score defaultWeights for each ScorePlugin in parallel.
	workqueue.ParallelizeUntil(ctx, 16, len(f.scorePlugins), func(index int) {
		pl := f.scorePlugins[index]
		// Score plugins' weight has been checked when they are initialized.
		weight := f.pluginNameToWeightMap[pl.Name()]
		nodeScoreList := pluginToNodeScores[pl.Name()]

		for i, nodeScore := range nodeScoreList {
			// return error if score plugin returns invalid score.
			if nodeScore.Score > int64(MaxNodeScore) || nodeScore.Score < int64(MinNodeScore) {
				err := fmt.Errorf("score plugin %q returns an invalid score %v, it should in the range of [%v, %v] after normalizing", pl.Name(), nodeScore.Score, MinNodeScore, MaxNodeScore)
				errCh.SendErrorWithCancel(err, cancel)
				return
			}
			nodeScoreList[i].Score = nodeScore.Score * int64(weight)
		}
	})
	if err := errCh.ReceiveError(); err != nil {
		msg := fmt.Sprintf("error while applying score defaultWeights for pod %q: %v", pod.Name, err)
		klog.Error(msg)
		return nil, NewStatus(Error, msg)
	}

	return pluginToNodeScores, nil
}

func (f *framework) runScorePlugin(ctx context.Context, pl ScorePlugin, state *CycleState, pod *v1.Pod, nodeName string) (int64, *Status) {
	if !state.ShouldRecordFrameworkMetrics() {
		return pl.Score(ctx, state, pod, nodeName)
	}
	startTime := time.Now()
	s, status := pl.Score(ctx, state, pod, nodeName)
	f.metricsRecorder.observePluginDurationAsync(score, pl.Name(), status, metrics.SinceInSeconds(startTime))
	return s, status
}

func (f *framework) runScoreExtension(ctx context.Context, pl ScorePlugin, state *CycleState, pod *v1.Pod, nodeScoreList NodeScoreList) *Status {
	if !state.ShouldRecordFrameworkMetrics() {
		return pl.ScoreExtensions().NormalizeScore(ctx, state, pod, nodeScoreList)
	}
	startTime := time.Now()
	status := pl.ScoreExtensions().NormalizeScore(ctx, state, pod, nodeScoreList)
	f.metricsRecorder.observePluginDurationAsync(scoreExtensionNormalize, pl.Name(), status, metrics.SinceInSeconds(startTime))
	return status
}

// RunPreBindPlugins runs the set of configured prebind plugins. It returns a
// failure (bool) if any of the plugins returns an error. It also returns an
// error containing the rejection message or the error occurred in the plugin.
func (f *framework) RunPreBindPlugins(ctx context.Context, state *CycleState, pod *v1.Pod, nodeName string) (status *Status) {
	if state.ShouldRecordFrameworkMetrics() {
		startTime := time.Now()
		defer func() {
			f.metricsRecorder.observeExtensionPointDurationAsync(preBind, status, metrics.SinceInSeconds(startTime))
		}()
	}
	for _, pl := range f.preBindPlugins {
		status = f.runPreBindPlugin(ctx, pl, state, pod, nodeName)
		if !status.IsSuccess() {
			msg := fmt.Sprintf("error while running %q prebind plugin for pod %q: %v", pl.Name(), pod.Name, status.Message())
			klog.Error(msg)
			return NewStatus(Error, msg)
		}
	}
	return nil
}

func (f *framework) runPreBindPlugin(ctx context.Context, pl PreBindPlugin, state *CycleState, pod *v1.Pod, nodeName string) *Status {
	if !state.ShouldRecordFrameworkMetrics() {
		return pl.PreBind(ctx, state, pod, nodeName)
	}
	startTime := time.Now()
	status := pl.PreBind(ctx, state, pod, nodeName)
	f.metricsRecorder.observePluginDurationAsync(preBind, pl.Name(), status, metrics.SinceInSeconds(startTime))
	return status
}

// RunBindPlugins runs the set of configured bind plugins until one returns a non `Skip` status.
func (f *framework) RunBindPlugins(ctx context.Context, state *CycleState, pod *v1.Pod, nodeName string) (status *Status) {
	if state.ShouldRecordFrameworkMetrics() {
		startTime := time.Now()
		defer func() {
			f.metricsRecorder.observeExtensionPointDurationAsync(bind, status, metrics.SinceInSeconds(startTime))
		}()
	}
	if len(f.bindPlugins) == 0 {
		return NewStatus(Skip, "")
	}
	for _, bp := range f.bindPlugins {
		status = f.runBindPlugin(ctx, bp, state, pod, nodeName)
		if status != nil && status.Code() == Skip {
			continue
		}
		if !status.IsSuccess() {
			msg := fmt.Sprintf("bind plugin %q failed to bind pod \"%v/%v\": %v", bp.Name(), pod.Namespace, pod.Name, status.Message())
			klog.Error(msg)
			return NewStatus(Error, msg)
		}
		return status
	}
	return status
}

func (f *framework) runBindPlugin(ctx context.Context, bp BindPlugin, state *CycleState, pod *v1.Pod, nodeName string) *Status {
	if !state.ShouldRecordFrameworkMetrics() {
		return bp.Bind(ctx, state, pod, nodeName)
	}
	startTime := time.Now()
	status := bp.Bind(ctx, state, pod, nodeName)
	f.metricsRecorder.observePluginDurationAsync(bind, bp.Name(), status, metrics.SinceInSeconds(startTime))
	return status
}

// RunPostBindPlugins runs the set of configured postbind plugins.
func (f *framework) RunPostBindPlugins(ctx context.Context, state *CycleState, pod *v1.Pod, nodeName string) {
	if state.ShouldRecordFrameworkMetrics() {
		startTime := time.Now()
		defer func() {
			f.metricsRecorder.observeExtensionPointDurationAsync(postBind, nil, metrics.SinceInSeconds(startTime))
		}()
	}
	for _, pl := range f.postBindPlugins {
		f.runPostBindPlugin(ctx, pl, state, pod, nodeName)
	}
}

func (f *framework) runPostBindPlugin(ctx context.Context, pl PostBindPlugin, state *CycleState, pod *v1.Pod, nodeName string) {
	if !state.ShouldRecordFrameworkMetrics() {
		pl.PostBind(ctx, state, pod, nodeName)
		return
	}
	startTime := time.Now()
	pl.PostBind(ctx, state, pod, nodeName)
	f.metricsRecorder.observePluginDurationAsync(postBind, pl.Name(), nil, metrics.SinceInSeconds(startTime))
}

// RunReservePlugins runs the set of configured reserve plugins. If any of these
// plugins returns an error, it does not continue running the remaining ones and
// returns the error. In such case, pod will not be scheduled.
func (f *framework) RunReservePlugins(ctx context.Context, state *CycleState, pod *v1.Pod, nodeName string) (status *Status) {
	if state.ShouldRecordFrameworkMetrics() {
		startTime := time.Now()
		defer func() {
			f.metricsRecorder.observeExtensionPointDurationAsync(reserve, status, metrics.SinceInSeconds(startTime))
		}()
	}
	for _, pl := range f.reservePlugins {
		status = f.runReservePlugin(ctx, pl, state, pod, nodeName)
		if !status.IsSuccess() {
			msg := fmt.Sprintf("error while running %q reserve plugin for pod %q: %v", pl.Name(), pod.Name, status.Message())
			klog.Error(msg)
			return NewStatus(Error, msg)
		}
	}
	return nil
}

func (f *framework) runReservePlugin(ctx context.Context, pl ReservePlugin, state *CycleState, pod *v1.Pod, nodeName string) *Status {
	if !state.ShouldRecordFrameworkMetrics() {
		return pl.Reserve(ctx, state, pod, nodeName)
	}
	startTime := time.Now()
	status := pl.Reserve(ctx, state, pod, nodeName)
	f.metricsRecorder.observePluginDurationAsync(reserve, pl.Name(), status, metrics.SinceInSeconds(startTime))
	return status
}

// RunUnreservePlugins runs the set of configured unreserve plugins.
func (f *framework) RunUnreservePlugins(ctx context.Context, state *CycleState, pod *v1.Pod, nodeName string) {
	if state.ShouldRecordFrameworkMetrics() {
		startTime := time.Now()
		defer func() {
			f.metricsRecorder.observeExtensionPointDurationAsync(unreserve, nil, metrics.SinceInSeconds(startTime))
		}()
	}
	for _, pl := range f.unreservePlugins {
		f.runUnreservePlugin(ctx, pl, state, pod, nodeName)
	}
}

func (f *framework) runUnreservePlugin(ctx context.Context, pl UnreservePlugin, state *CycleState, pod *v1.Pod, nodeName string) {
	if !state.ShouldRecordFrameworkMetrics() {
		pl.Unreserve(ctx, state, pod, nodeName)
		return
	}
	startTime := time.Now()
	pl.Unreserve(ctx, state, pod, nodeName)
	f.metricsRecorder.observePluginDurationAsync(unreserve, pl.Name(), nil, metrics.SinceInSeconds(startTime))
}

// RunPermitPlugins runs the set of configured permit plugins. If any of these
// plugins returns a status other than "Success" or "Wait", it does not continue
// running the remaining plugins and returns an error. Otherwise, if any of the
// plugins returns "Wait", then this function will block for the timeout period
// returned by the plugin, if the time expires, then it will return an error.
// Note that if multiple plugins asked to wait, then we wait for the minimum
// timeout duration.
func (f *framework) RunPermitPlugins(ctx context.Context, state *CycleState, pod *v1.Pod, nodeName string) (status *Status) {
	if state.ShouldRecordFrameworkMetrics() {
		startTime := time.Now()
		defer func() {
			f.metricsRecorder.observeExtensionPointDurationAsync(permit, status, metrics.SinceInSeconds(startTime))
		}()
	}
	pluginsWaitTime := make(map[string]time.Duration)
	statusCode := Success
	for _, pl := range f.permitPlugins {
		status, timeout := f.runPermitPlugin(ctx, pl, state, pod, nodeName)
		if !status.IsSuccess() {
			if status.IsUnschedulable() {
				msg := fmt.Sprintf("rejected by %q at permit: %v", pl.Name(), status.Message())
				klog.V(4).Infof(msg)
				return NewStatus(status.Code(), msg)
			}
			if status.Code() == Wait {
				// Not allowed to be greater than maxTimeout.
				if timeout > maxTimeout {
					timeout = maxTimeout
				}
				pluginsWaitTime[pl.Name()] = timeout
				statusCode = Wait
			} else {
				msg := fmt.Sprintf("error while running %q permit plugin for pod %q: %v", pl.Name(), pod.Name, status.Message())
				klog.Error(msg)
				return NewStatus(Error, msg)
			}
		}
	}

	// We now wait for the minimum duration if at least one plugin asked to
	// wait (and no plugin rejected the pod)
	if statusCode == Wait {
		startTime := time.Now()
		w := newWaitingPod(pod, pluginsWaitTime)
		f.waitingPods.add(w)
		defer f.waitingPods.remove(pod.UID)
		klog.V(4).Infof("waiting for pod %q at permit", pod.Name)
		s := <-w.s
		metrics.PermitWaitDuration.WithLabelValues(s.Code().String()).Observe(metrics.SinceInSeconds(startTime))
		if !s.IsSuccess() {
			if s.IsUnschedulable() {
				msg := fmt.Sprintf("pod %q rejected while waiting at permit: %v", pod.Name, s.Message())
				klog.V(4).Infof(msg)
				return NewStatus(s.Code(), msg)
			}
			msg := fmt.Sprintf("error received while waiting at permit for pod %q: %v", pod.Name, s.Message())
			klog.Error(msg)
			return NewStatus(Error, msg)
		}
	}

	return nil
}

func (f *framework) runPermitPlugin(ctx context.Context, pl PermitPlugin, state *CycleState, pod *v1.Pod, nodeName string) (*Status, time.Duration) {
	if !state.ShouldRecordFrameworkMetrics() {
		return pl.Permit(ctx, state, pod, nodeName)
	}
	startTime := time.Now()
	status, timeout := pl.Permit(ctx, state, pod, nodeName)
	f.metricsRecorder.observePluginDurationAsync(permit, pl.Name(), status, metrics.SinceInSeconds(startTime))
	return status, timeout
}

// SnapshotSharedLister returns the scheduler's SharedLister of the latest NodeInfo
// snapshot. The snapshot is taken at the beginning of a scheduling cycle and remains
// unchanged until a pod finishes "Reserve". There is no guarantee that the information
// remains unchanged after "Reserve".
func (f *framework) SnapshotSharedLister() schedulerlisters.SharedLister {
	return f.snapshotSharedLister
}

// IterateOverWaitingPods acquires a read lock and iterates over the WaitingPods map.
func (f *framework) IterateOverWaitingPods(callback func(WaitingPod)) {
	f.waitingPods.iterate(callback)
}

// GetWaitingPod returns a reference to a WaitingPod given its UID.
func (f *framework) GetWaitingPod(uid types.UID) WaitingPod {
	return f.waitingPods.get(uid)
}

// RejectWaitingPod rejects a WaitingPod given its UID.
func (f *framework) RejectWaitingPod(uid types.UID) {
	waitingPod := f.waitingPods.get(uid)
	if waitingPod != nil {
		waitingPod.Reject("removed")
	}
}

// HasFilterPlugins returns true if at least one filter plugin is defined.
func (f *framework) HasFilterPlugins() bool {
	return len(f.filterPlugins) > 0
}

// HasScorePlugins returns true if at least one score plugin is defined.
func (f *framework) HasScorePlugins() bool {
	return len(f.scorePlugins) > 0
}

// ListPlugins returns a map of extension point name to plugin names configured at each extension
// point. Returns nil if no plugins where configred.
func (f *framework) ListPlugins() map[string][]config.Plugin {
	m := make(map[string][]config.Plugin)

	for _, e := range f.getExtensionPoints(&config.Plugins{}) {
		plugins := reflect.ValueOf(e.slicePtr).Elem()
		extName := plugins.Type().Elem().Name()
		var cfgs []config.Plugin
		for i := 0; i < plugins.Len(); i++ {
			name := plugins.Index(i).Interface().(Plugin).Name()
			p := config.Plugin{Name: name}
			if extName == "ScorePlugin" {
				// Weights apply only to score plugins.
				p.Weight = int32(f.pluginNameToWeightMap[name])
			}
			cfgs = append(cfgs, p)
		}
		if len(cfgs) > 0 {
			m[extName] = cfgs
		}
	}
	if len(m) > 0 {
		return m
	}
	return nil
}

// ClientSet returns a kubernetes clientset.
func (f *framework) ClientSet() clientset.Interface {
	return f.clientSet
}

// SharedInformerFactory returns a shared informer factory.
func (f *framework) SharedInformerFactory() informers.SharedInformerFactory {
	return f.informerFactory
}

func (f *framework) pluginsNeeded(plugins *config.Plugins) map[string]config.Plugin {
	pgMap := make(map[string]config.Plugin)

	if plugins == nil {
		return pgMap
	}

	find := func(pgs *config.PluginSet) {
		if pgs == nil {
			return
		}
		for _, pg := range pgs.Enabled {
			pgMap[pg.Name] = pg
		}
	}
	for _, e := range f.getExtensionPoints(plugins) {
		find(e.plugins)
	}
	return pgMap
}
