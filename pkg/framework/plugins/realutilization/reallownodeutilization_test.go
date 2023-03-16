package realutilization

import (
	"context"
	"fmt"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/events"
	"k8s.io/metrics/pkg/apis/metrics/v1beta1"
	fakemetricsclientset "k8s.io/metrics/pkg/client/clientset/versioned/fake"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/cache"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/framework"
	frameworkfake "sigs.k8s.io/descheduler/pkg/framework/fake"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	"sigs.k8s.io/descheduler/test"
)

func TestRealLowNodeUtilization(t *testing.T) {
	type testCase struct {
		name                         string
		useDeviationThresholds       bool
		thresholds, targetThresholds api.ResourceThresholds
		nodes                        []*v1.Node
		pods                         []*v1.Pod
		nodeMetrics                  []*v1beta1.NodeMetrics
		podMetrics                   []*v1beta1.PodMetrics
		expectedPodsEvicted          uint
		evictedPods                  []string
		evictableNamespaces          *api.Namespaces
	}
	target1Node := "target1"
	low1Node := "low1"
	testCases := []testCase{
		{
			name:                   "test1",
			useDeviationThresholds: true,
			thresholds: api.ResourceThresholds{
				v1.ResourceCPU:    30,
				v1.ResourceMemory: 30,
			},
			targetThresholds: api.ResourceThresholds{
				v1.ResourceCPU:    70,
				v1.ResourceMemory: 70,
			},
			nodes: []*v1.Node{
				test.BuildTestNode(target1Node, 10000, 10000, 0, nil),
				test.BuildTestNode(low1Node, 10000, 10000, 0, nil),
			},
			pods: []*v1.Pod{
				test.BuildTestPod("p1", 2000, 2000, target1Node, test.SetRSOwnerRef),
				test.BuildTestPod("p2", 2000, 2000, target1Node, test.SetRSOwnerRef),
				test.BuildTestPod("p3", 2000, 2000, target1Node, test.SetRSOwnerRef),
				test.BuildTestPod("p4", 2000, 2000, target1Node, test.SetRSOwnerRef),
				test.BuildTestPod("p5", 2000, 2000, low1Node, test.SetRSOwnerRef),
			},
			nodeMetrics: []*v1beta1.NodeMetrics{
				test.BuildTestNodeMetrics(target1Node, 8000, 8000, nil),
				test.BuildTestNodeMetrics(low1Node, 2000, 2000, nil),
			},
			podMetrics: []*v1beta1.PodMetrics{
				test.BuildTestPodMetrics("p1", 2000, 2000, nil),
				test.BuildTestPodMetrics("p2", 2000, 2000, nil),
				test.BuildTestPodMetrics("p3", 2000, 2000, nil),
				test.BuildTestPodMetrics("p4", 2000, 2000, nil),
				test.BuildTestPodMetrics("p5", 2000, 2000, nil),
			},
			expectedPodsEvicted: 4,
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			var objs []runtime.Object
			for _, node := range test.nodes {
				objs = append(objs, node)
			}
			for _, pod := range test.pods {
				objs = append(objs, pod)
			}
			fakeClient := fake.NewSimpleClientset(objs...)

			sharedInformerFactory := informers.NewSharedInformerFactory(fakeClient, time.Second*5)
			podInformer := sharedInformerFactory.Core().V1().Pods().Informer()

			var metricsObjs []runtime.Object
			for _, metrics := range test.nodeMetrics {
				metricsObjs = append(metricsObjs, metrics)
			}
			for _, metrics := range test.podMetrics {
				metricsObjs = append(metricsObjs, metrics)
			}
			fakeMetricsClient := fakemetricsclientset.NewSimpleClientset(metricsObjs...)
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

			iCache, err := cache.InitCache(fakeClient, sharedInformerFactory.Core().V1().Nodes().Informer(),
				sharedInformerFactory.Core().V1().Nodes().Lister(), sharedInformerFactory.Core().V1().Pods().Informer(), fakeMetricsClient, ctx.Done())

			iCache.Run(time.Second * 5)

			getPodsAssignedToNode, err := podutil.BuildGetPodsAssignedToNodeFunc(podInformer)
			if err != nil {
				t.Errorf("Build get pods assigned to node function error: %v", err)
			}

			podsForEviction := make(map[string]struct{})
			for _, pod := range test.evictedPods {
				podsForEviction[pod] = struct{}{}
			}

			evictionFailed := false
			if len(test.evictedPods) > 0 {
				fakeClient.Fake.AddReactor("create", "pods", func(action core.Action) (bool, runtime.Object, error) {
					getAction := action.(core.CreateAction)
					obj := getAction.GetObject()
					if eviction, ok := obj.(*policy.Eviction); ok {
						if _, exists := podsForEviction[eviction.Name]; exists {
							return true, obj, nil
						}
						evictionFailed = true
						return true, nil, fmt.Errorf("pod %q was unexpectedly evicted", eviction.Name)
					}
					return true, obj, nil
				})
			}

			sharedInformerFactory.Start(ctx.Done())
			sharedInformerFactory.WaitForCacheSync(ctx.Done())
			cacheReady := false
			for i := 0; i < 100 && !cacheReady; i++ {
				time.Sleep(time.Second * 6)
				nodeUsage := iCache.GetReadyNodeUsage(&cache.QueryCacheOption{})
				for _, u := range nodeUsage {
					if len(u.UsageList) > cache.LATEST_REAL_METRICS_STORE && len(u.AllPods) > 0 && len(u.AllPods[0].UsageList) > 0 {
						cacheReady = true
					}
				}
			}

			t.Logf("cache is ready")

			eventRecorder := &events.FakeRecorder{}

			podEvictor := evictions.NewPodEvictor(
				fakeClient,
				policy.SchemeGroupVersion.String(),
				false,
				nil,
				nil,
				test.nodes,
				false,
				eventRecorder,
			)

			defaultEvictorFilterArgs := &defaultevictor.DefaultEvictorArgs{
				EvictLocalStoragePods:   false,
				EvictSystemCriticalPods: false,
				IgnorePvcPods:           false,
				EvictFailedBarePods:     false,
				NodeFit:                 true,
			}

			evictorFilter, err := defaultevictor.New(
				defaultEvictorFilterArgs,
				&frameworkfake.HandleImpl{
					ClientsetImpl:                 fakeClient,
					GetPodsAssignedToNodeFuncImpl: getPodsAssignedToNode,
					SharedInformerFactoryImpl:     sharedInformerFactory,
				},
			)
			if err != nil {
				t.Fatalf("Unable to initialize the plugin: %v", err)
			}

			handle := &frameworkfake.HandleImpl{
				ClientsetImpl:                 fakeClient,
				GetPodsAssignedToNodeFuncImpl: getPodsAssignedToNode,
				PodEvictorImpl:                podEvictor,
				EvictorFilterImpl:             evictorFilter.(framework.EvictorPlugin),
				SharedInformerFactoryImpl:     sharedInformerFactory,
			}

			plugin, err := NewRealLowNodeUtilization(&LowNodeRealUtilizationArgs{
				Thresholds:          test.thresholds,
				TargetThresholds:    test.targetThresholds,
				EvictableNamespaces: test.evictableNamespaces,
			}, handle)
			if err != nil {
				t.Fatalf("Unable to initialize the plugin: %v", err)
			}
			plugin.(framework.BalancePlugin).Balance(ctx, test.nodes)

			podsEvicted := podEvictor.TotalEvicted()
			if test.expectedPodsEvicted != podsEvicted {
				t.Errorf("Expected %v pods to be evicted but %v got evicted", test.expectedPodsEvicted, podsEvicted)
			}
			if evictionFailed {
				t.Errorf("Pod evictions failed unexpectedly")
			}
		})
	}
}
