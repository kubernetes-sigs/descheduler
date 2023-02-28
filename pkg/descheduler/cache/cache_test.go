package cache

import (
	"context"
	"fmt"
	v1 "k8s.io/api/core/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/metrics/pkg/apis/metrics/v1beta1"
	fakemetricsclientset "k8s.io/metrics/pkg/client/clientset/versioned/fake"
	"sigs.k8s.io/descheduler/test"
	"testing"
	"time"
)

// test query real usage from cache
func TestCache(t *testing.T) {
	type testCase struct {
		nodes       []*v1.Node
		pods        []*v1.Pod
		nodeMetrics []*v1beta1.NodeMetrics
		podMetrics  []*v1beta1.PodMetrics
	}
	var t1 testCase
	for i := 0; i < 10; i++ {
		// 40000 cpu allocatable
		node := test.BuildTestNode(fmt.Sprintf("fakeNode-%d", i), 40000, 40000, 10, nil)
		t1.nodes = append(t1.nodes, node)
		// 10000 real used
		nodeMetrics := test.BuildTestNodeMetrics(node.Name, 10000, 10000, nil)
		t1.nodeMetrics = append(t1.nodeMetrics, nodeMetrics)
		for j := 0; j < 10; j++ {
			// pod request 4000
			pod := test.BuildTestPod(fmt.Sprintf("%s-%d", node.Name, j), 4000, 4000, node.Name, nil)
			t1.pods = append(t1.pods, pod)
			// pod real used 4000
			podMetrics := test.BuildTestPodMetrics(pod.Name, 1000, 1000, nil)
			t1.podMetrics = append(t1.podMetrics, podMetrics)
		}
	}
	var objs []runtime.Object
	for _, node := range t1.nodes {
		objs = append(objs, node)
	}
	for _, pod := range t1.pods {
		objs = append(objs, pod)
	}
	fakeClient := fake.NewSimpleClientset(objs...)
	var metricsObjs []runtime.Object
	for _, metrics := range t1.nodeMetrics {
		metricsObjs = append(metricsObjs, metrics)
	}
	for _, metrics := range t1.podMetrics {
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
	nodes, err := fakeClient.CoreV1().Nodes().List(context.Background(), v12.ListOptions{})
	if err != nil {
		t.Fatalf("get node error with %+v", err)
	}
	t.Logf("get nodes %+v", len(nodes.Items))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nm, err := fakeMetricsClient.MetricsV1beta1().NodeMetricses().List(context.Background(), v12.ListOptions{})
	if err != nil {
		t.Fatalf("list nodeMetricses with error %+v", err)
	}
	t.Logf("node metrics %d", len(nm.Items))
	pm, err := fakeMetricsClient.MetricsV1beta1().PodMetricses("").List(context.Background(), v12.ListOptions{})
	if err != nil {
		t.Fatalf("list nodeMetricses with error %+v", err)
	}
	t.Logf("pod metrics %d", len(pm.Items))
	sharedInformerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
	if err != nil {
		t.Fatalf("init cache with error %+v", err)
	}

	iCache, err := InitCache(fakeClient, sharedInformerFactory.Core().V1().Nodes().Informer(),
		sharedInformerFactory.Core().V1().Nodes().Lister(), sharedInformerFactory.Core().V1().Pods().Informer(), fakeMetricsClient, ctx.Done())

	sharedInformerFactory.Start(ctx.Done())
	sharedInformerFactory.WaitForCacheSync(ctx.Done())
	iCache.Run(time.Second * 5)
	time.Sleep(time.Second * 10)
	t.Logf("init cache success")
	usage := iCache.GetReadyNodeUsage(&QueryCacheOption{})
	if len(usage) != 10 {
		t.Fatalf("usage length is not 10")
	}
	for node, u := range usage {
		if len(u.UsageList) == 0 {
			t.Fatalf("node %v usage is empty", node)
		}
		if len(u.AllPods) != 10 {
			t.Fatalf("usage length is not 10")
		}
		for _, pu := range u.AllPods {
			if pu.Pod == nil || len(pu.UsageList) == 0 {
				t.Fatalf("pod %v usage is empty", pu.Pod.Name)
			}
		}
	}
}
