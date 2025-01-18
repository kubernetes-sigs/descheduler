package testing

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/events"
	"k8s.io/component-base/featuregate"

	clientset "k8s.io/client-go/kubernetes"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/features"
	frameworkfake "sigs.k8s.io/descheduler/pkg/framework/fake"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
)

func InitFrameworkHandle(
	ctx context.Context,
	client clientset.Interface,
	evictionOptions *evictions.Options,
	defaultEvictorArgs defaultevictor.DefaultEvictorArgs,
	getPodsAssignedToNodeSorter func([]*v1.Pod),
) (*frameworkfake.HandleImpl, *evictions.PodEvictor, error) {
	sharedInformerFactory := informers.NewSharedInformerFactory(client, 0)
	podInformer := sharedInformerFactory.Core().V1().Pods().Informer()
	podsAssignedToNode, err := podutil.BuildGetPodsAssignedToNodeFunc(podInformer)
	if err != nil {
		return nil, nil, fmt.Errorf("Build get pods assigned to node function error: %v", err)
	}

	var getPodsAssignedToNode func(ctx context.Context, s string, filterFunc podutil.FilterFunc) ([]*v1.Pod, error)
	if getPodsAssignedToNodeSorter != nil {
		getPodsAssignedToNode = func(ctx context.Context, s string, filterFunc podutil.FilterFunc) ([]*v1.Pod, error) {
			pods, err := podsAssignedToNode(ctx, s, filterFunc)
			getPodsAssignedToNodeSorter(pods)
			return pods, err
		}
	} else {
		getPodsAssignedToNode = podsAssignedToNode
	}

	sharedInformerFactory.Start(ctx.Done())
	sharedInformerFactory.WaitForCacheSync(ctx.Done())
	eventRecorder := &events.FakeRecorder{}
	featureGates := featuregate.NewFeatureGate()
	featureGates.Add(map[featuregate.Feature]featuregate.FeatureSpec{
		features.EvictionsInBackground: {Default: false, PreRelease: featuregate.Alpha},
	})
	podEvictor, err := evictions.NewPodEvictor(ctx, client, eventRecorder, podInformer, featureGates, evictionOptions)
	if err != nil {
		return nil, nil, fmt.Errorf("Unable to initialize pod evictor: %v", err)
	}
	evictorFilter, err := defaultevictor.New(
		ctx,
		&defaultEvictorArgs,
		&frameworkfake.HandleImpl{
			ClientsetImpl:                 client,
			GetPodsAssignedToNodeFuncImpl: getPodsAssignedToNode,
			SharedInformerFactoryImpl:     sharedInformerFactory,
		},
	)
	if err != nil {
		return nil, nil, fmt.Errorf("Unable to initialize the plugin: %v", err)
	}
	return &frameworkfake.HandleImpl{
		ClientsetImpl:                 client,
		GetPodsAssignedToNodeFuncImpl: getPodsAssignedToNode,
		PodEvictorImpl:                podEvictor,
		EvictorFilterImpl:             evictorFilter.(frameworktypes.EvictorPlugin),
		SharedInformerFactoryImpl:     sharedInformerFactory,
	}, podEvictor, nil
}
