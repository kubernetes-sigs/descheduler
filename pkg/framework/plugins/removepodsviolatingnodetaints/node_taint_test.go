package removepodsviolatingnodetaints

import (
	"context"
	"fmt"
	"testing"

	v1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/framework"
	"sigs.k8s.io/descheduler/pkg/utils"
	"sigs.k8s.io/descheduler/test"
)

type frameworkHandle struct {
	clientset                 clientset.Interface
	podEvictor                *evictions.PodEvictor
	getPodsAssignedToNodeFunc podutil.GetPodsAssignedToNodeFunc
}

func (f frameworkHandle) ClientSet() clientset.Interface {
	return f.clientset
}
func (f frameworkHandle) PodEvictor() *evictions.PodEvictor {
	return f.podEvictor
}
func (f frameworkHandle) GetPodsAssignedToNodeFunc() podutil.GetPodsAssignedToNodeFunc {
	return f.getPodsAssignedToNodeFunc
}

func createNoScheduleTaint(key, value string, index int) v1.Taint {
	return v1.Taint{
		Key:    "testTaint" + fmt.Sprintf("%v", index),
		Value:  "test" + fmt.Sprintf("%v", index),
		Effect: v1.TaintEffectNoSchedule,
	}
}

func createPreferNoScheduleTaint(key, value string, index int) v1.Taint {
	return v1.Taint{
		Key:    "testTaint" + fmt.Sprintf("%v", index),
		Value:  "test" + fmt.Sprintf("%v", index),
		Effect: v1.TaintEffectPreferNoSchedule,
	}
}

func addTaintsToNode(node *v1.Node, key, value string, indices []int) *v1.Node {
	taints := []v1.Taint{}
	for _, index := range indices {
		taints = append(taints, createNoScheduleTaint(key, value, index))
	}
	node.Spec.Taints = taints
	return node
}

func addTolerationToPod(pod *v1.Pod, key, value string, index int, effect v1.TaintEffect) *v1.Pod {
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}

	pod.Spec.Tolerations = []v1.Toleration{{Key: key + fmt.Sprintf("%v", index), Value: value + fmt.Sprintf("%v", index), Effect: effect}}

	return pod
}

func TestDeletePodsViolatingNodeTaints(t *testing.T) {
	node1 := test.BuildTestNode("n1", 2000, 3000, 10, nil)
	node1 = addTaintsToNode(node1, "testTaint", "test", []int{1})
	node2 := test.BuildTestNode("n2", 2000, 3000, 10, nil)
	node2 = addTaintsToNode(node2, "testingTaint", "testing", []int{1})

	node3 := test.BuildTestNode("n3", 2000, 3000, 10, func(node *v1.Node) {
		node.ObjectMeta.Labels = map[string]string{
			"datacenter": "east",
		}
	})
	node4 := test.BuildTestNode("n4", 2000, 3000, 10, func(node *v1.Node) {
		node.Spec = v1.NodeSpec{
			Unschedulable: true,
		}
	})

	node5 := test.BuildTestNode("n5", 2000, 3000, 10, nil)
	node5.Spec.Taints = []v1.Taint{
		createPreferNoScheduleTaint("testTaint", "test", 1),
	}

	p1 := test.BuildTestPod("p1", 100, 0, node1.Name, nil)
	p2 := test.BuildTestPod("p2", 100, 0, node1.Name, nil)
	p3 := test.BuildTestPod("p3", 100, 0, node1.Name, nil)
	p4 := test.BuildTestPod("p4", 100, 0, node1.Name, nil)
	p5 := test.BuildTestPod("p5", 100, 0, node1.Name, nil)
	p6 := test.BuildTestPod("p6", 100, 0, node1.Name, nil)

	p1.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
	p2.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
	p3.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
	p4.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
	p5.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
	p6.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
	p7 := test.BuildTestPod("p7", 100, 0, node2.Name, nil)
	p8 := test.BuildTestPod("p8", 100, 0, node2.Name, nil)
	p9 := test.BuildTestPod("p9", 100, 0, node2.Name, nil)
	p10 := test.BuildTestPod("p10", 100, 0, node2.Name, nil)
	p11 := test.BuildTestPod("p11", 100, 0, node2.Name, nil)
	p11.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
	p12 := test.BuildTestPod("p11", 100, 0, node2.Name, nil)
	p12.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()

	// The following 4 pods won't get evicted.
	// A Critical Pod.
	p7.Namespace = "kube-system"
	priority := utils.SystemCriticalPriority
	p7.Spec.Priority = &priority
	p7.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()

	// A daemonset.
	p8.ObjectMeta.OwnerReferences = test.GetDaemonSetOwnerRefList()
	// A pod with local storage.
	p9.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
	p9.Spec.Volumes = []v1.Volume{
		{
			Name: "sample",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{Path: "somePath"},
				EmptyDir: &v1.EmptyDirVolumeSource{
					SizeLimit: resource.NewQuantity(int64(10), resource.BinarySI)},
			},
		},
	}
	// A Mirror Pod.
	p10.Annotations = test.GetMirrorPodAnnotation()

	p1 = addTolerationToPod(p1, "testTaint", "test", 1, v1.TaintEffectNoSchedule)
	p3 = addTolerationToPod(p3, "testTaint", "test", 1, v1.TaintEffectNoSchedule)
	p4 = addTolerationToPod(p4, "testTaintX", "testX", 1, v1.TaintEffectNoSchedule)

	p12.Spec.NodeSelector = map[string]string{
		"datacenter": "west",
	}

	p13 := test.BuildTestPod("p13", 100, 0, node5.Name, nil)
	p13.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
	// node5 has PreferNoSchedule:testTaint1=test1, so the p13 has to have
	// PreferNoSchedule:testTaint0=test0 so the pod is not tolarated
	p13 = addTolerationToPod(p13, "testTaint", "test", 0, v1.TaintEffectPreferNoSchedule)

	var uint1 uint = 1

	tests := []struct {
		description                    string
		nodes                          []*v1.Node
		pods                           []*v1.Pod
		evictLocalStoragePods          bool
		evictSystemCriticalPods        bool
		maxPodsToEvictPerNode          *uint
		maxNoOfPodsToEvictPerNamespace *uint
		expectedEvictedPodCount        uint
		nodeFit                        bool
		includePreferNoSchedule        bool
		excludedTaints                 []string
	}{

		{
			description:             "Pods not tolerating node taint should be evicted",
			pods:                    []*v1.Pod{p1, p2, p3},
			nodes:                   []*v1.Node{node1},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			expectedEvictedPodCount: 1, //p2 gets evicted
		},
		{
			description:             "Pods with tolerations but not tolerating node taint should be evicted",
			pods:                    []*v1.Pod{p1, p3, p4},
			nodes:                   []*v1.Node{node1},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			expectedEvictedPodCount: 1, //p4 gets evicted
		},
		{
			description:             "Only <maxPodsToEvictPerNode> number of Pods not tolerating node taint should be evicted",
			pods:                    []*v1.Pod{p1, p5, p6},
			nodes:                   []*v1.Node{node1},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			maxPodsToEvictPerNode:   &uint1,
			expectedEvictedPodCount: 1, //p5 or p6 gets evicted
		},
		{
			description:                    "Only <maxNoOfPodsToEvictPerNamespace> number of Pods not tolerating node taint should be evicted",
			pods:                           []*v1.Pod{p1, p5, p6},
			nodes:                          []*v1.Node{node1},
			evictLocalStoragePods:          false,
			evictSystemCriticalPods:        false,
			maxNoOfPodsToEvictPerNamespace: &uint1,
			expectedEvictedPodCount:        1, //p5 or p6 gets evicted
		},
		{
			description:             "Critical pods not tolerating node taint should not be evicted",
			pods:                    []*v1.Pod{p7, p8, p9, p10},
			nodes:                   []*v1.Node{node2},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			expectedEvictedPodCount: 0, //nothing is evicted
		},
		{
			description:             "Critical pods except storage pods not tolerating node taint should not be evicted",
			pods:                    []*v1.Pod{p7, p8, p9, p10},
			nodes:                   []*v1.Node{node2},
			evictLocalStoragePods:   true,
			evictSystemCriticalPods: false,
			expectedEvictedPodCount: 1, //p9 gets evicted
		},
		{
			description:             "Critical and non critical pods, only non critical pods not tolerating node taint should be evicted",
			pods:                    []*v1.Pod{p7, p8, p10, p11},
			nodes:                   []*v1.Node{node2},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			expectedEvictedPodCount: 1, //p11 gets evicted
		},
		{
			description:             "Critical and non critical pods, pods not tolerating node taint should be evicted even if they are critical",
			pods:                    []*v1.Pod{p2, p7, p9, p10},
			nodes:                   []*v1.Node{node1, node2},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: true,
			expectedEvictedPodCount: 2, //p2 and p7 are evicted
		},
		{
			description:             "Pod p2 doesn't tolerate taint on it's node, but also doesn't tolerate taints on other nodes",
			pods:                    []*v1.Pod{p1, p2, p3},
			nodes:                   []*v1.Node{node1, node2},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			expectedEvictedPodCount: 0, //p2 gets evicted
			nodeFit:                 true,
		},
		{
			description:             "Pod p12 doesn't tolerate taint on it's node, but other nodes don't match it's selector",
			pods:                    []*v1.Pod{p1, p3, p12},
			nodes:                   []*v1.Node{node1, node3},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			expectedEvictedPodCount: 0, //p2 gets evicted
			nodeFit:                 true,
		},
		{
			description:             "Pod p2 doesn't tolerate taint on it's node, but other nodes are unschedulable",
			pods:                    []*v1.Pod{p1, p2, p3},
			nodes:                   []*v1.Node{node1, node4},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			expectedEvictedPodCount: 0, //p2 gets evicted
			nodeFit:                 true,
		},
		{
			description:             "Pods not tolerating PreferNoSchedule node taint should not be evicted when not enabled",
			pods:                    []*v1.Pod{p13},
			nodes:                   []*v1.Node{node5},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			expectedEvictedPodCount: 0,
		},
		{
			description:             "Pods not tolerating PreferNoSchedule node taint should be evicted when enabled",
			pods:                    []*v1.Pod{p13},
			nodes:                   []*v1.Node{node5},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			includePreferNoSchedule: true,
			expectedEvictedPodCount: 1, // p13 gets evicted
		},
		{
			description:             "Pods not tolerating excluded node taints (by key) should not be evicted",
			pods:                    []*v1.Pod{p2},
			nodes:                   []*v1.Node{node1},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			excludedTaints:          []string{"excludedTaint1", "testTaint1"},
			expectedEvictedPodCount: 0, // nothing gets evicted, as one of the specified excludedTaints matches the key of node1's taint
		},
		{
			description:             "Pods not tolerating excluded node taints (by key and value) should not be evicted",
			pods:                    []*v1.Pod{p2},
			nodes:                   []*v1.Node{node1},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			excludedTaints:          []string{"testTaint1=test1"},
			expectedEvictedPodCount: 0, // nothing gets evicted, as both the key and value of the excluded taint match node1's taint
		},
		{
			description:             "The excluded taint matches the key of node1's taint, but does not match the value",
			pods:                    []*v1.Pod{p2},
			nodes:                   []*v1.Node{node1},
			evictLocalStoragePods:   false,
			evictSystemCriticalPods: false,
			excludedTaints:          []string{"testTaint1=test2"},
			expectedEvictedPodCount: 1, // pod gets evicted, as excluded taint value does not match node1's taint value
		},
	}

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			var objs []runtime.Object
			for _, node := range tc.nodes {
				objs = append(objs, node)
			}
			for _, pod := range tc.pods {
				objs = append(objs, pod)
			}
			fakeClient := fake.NewSimpleClientset(objs...)

			sharedInformerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
			podInformer := sharedInformerFactory.Core().V1().Pods()

			getPodsAssignedToNode, err := podutil.BuildGetPodsAssignedToNodeFunc(podInformer)
			if err != nil {
				t.Errorf("Build get pods assigned to node function error: %v", err)
			}

			sharedInformerFactory.Start(ctx.Done())
			sharedInformerFactory.WaitForCacheSync(ctx.Done())

			podEvictor := evictions.NewPodEvictor(
				fakeClient,
				policyv1.SchemeGroupVersion.String(),
				false,
				tc.maxPodsToEvictPerNode,
				tc.maxNoOfPodsToEvictPerNamespace,
				tc.nodes,
				tc.evictLocalStoragePods,
				tc.evictSystemCriticalPods,
				false,
				false,
				false,
			)

			plugin, err := New(&framework.RemovePodsViolatingNodeTaintsArgs{
				CommonArgs: framework.CommonArgs{
					NodeFit: tc.nodeFit,
				},
				IncludePreferNoSchedule: tc.includePreferNoSchedule,
				ExcludedTaints:          tc.excludedTaints,
			},
				frameworkHandle{
					clientset:                 fakeClient,
					podEvictor:                podEvictor,
					getPodsAssignedToNodeFunc: getPodsAssignedToNode,
				},
			)
			if err != nil {
				t.Fatalf("Unable to initialize the plugin: %v", err)
			}

			plugin.(interface{}).(framework.DeschedulePlugin).Deschedule(ctx, tc.nodes)
			actualEvictedPodCount := podEvictor.TotalEvicted()
			if actualEvictedPodCount != tc.expectedEvictedPodCount {
				t.Errorf("Test %#v failed, Unexpected no of pods evicted: pods evicted: %d, expected: %d", tc.description, actualEvictedPodCount, tc.expectedEvictedPodCount)
			}
		})
	}
}

func TestToleratesTaint(t *testing.T) {

	testCases := []struct {
		description     string
		toleration      v1.Toleration
		taint           v1.Taint
		expectTolerated bool
	}{
		{
			description: "toleration and taint have the same key and effect, and operator is Exists, and taint has no value, expect tolerated",
			toleration: v1.Toleration{
				Key:      "foo",
				Operator: v1.TolerationOpExists,
				Effect:   v1.TaintEffectNoSchedule,
			},
			taint: v1.Taint{
				Key:    "foo",
				Effect: v1.TaintEffectNoSchedule,
			},
			expectTolerated: true,
		},
		{
			description: "toleration and taint have the same key and effect, and operator is Exists, and taint has some value, expect tolerated",
			toleration: v1.Toleration{
				Key:      "foo",
				Operator: v1.TolerationOpExists,
				Effect:   v1.TaintEffectNoSchedule,
			},
			taint: v1.Taint{
				Key:    "foo",
				Value:  "bar",
				Effect: v1.TaintEffectNoSchedule,
			},
			expectTolerated: true,
		},
		{
			description: "toleration and taint have the same effect, toleration has empty key and operator is Exists, means match all taints, expect tolerated",
			toleration: v1.Toleration{
				Key:      "",
				Operator: v1.TolerationOpExists,
				Effect:   v1.TaintEffectNoSchedule,
			},
			taint: v1.Taint{
				Key:    "foo",
				Value:  "bar",
				Effect: v1.TaintEffectNoSchedule,
			},
			expectTolerated: true,
		},
		{
			description: "toleration and taint have the same key, effect and value, and operator is Equal, expect tolerated",
			toleration: v1.Toleration{
				Key:      "foo",
				Operator: v1.TolerationOpEqual,
				Value:    "bar",
				Effect:   v1.TaintEffectNoSchedule,
			},
			taint: v1.Taint{
				Key:    "foo",
				Value:  "bar",
				Effect: v1.TaintEffectNoSchedule,
			},
			expectTolerated: true,
		},
		{
			description: "toleration and taint have the same key and effect, but different values, and operator is Equal, expect not tolerated",
			toleration: v1.Toleration{
				Key:      "foo",
				Operator: v1.TolerationOpEqual,
				Value:    "value1",
				Effect:   v1.TaintEffectNoSchedule,
			},
			taint: v1.Taint{
				Key:    "foo",
				Value:  "value2",
				Effect: v1.TaintEffectNoSchedule,
			},
			expectTolerated: false,
		},
		{
			description: "toleration and taint have the same key and value, but different effects, and operator is Equal, expect not tolerated",
			toleration: v1.Toleration{
				Key:      "foo",
				Operator: v1.TolerationOpEqual,
				Value:    "bar",
				Effect:   v1.TaintEffectNoSchedule,
			},
			taint: v1.Taint{
				Key:    "foo",
				Value:  "bar",
				Effect: v1.TaintEffectNoExecute,
			},
			expectTolerated: false,
		},
	}
	for _, tc := range testCases {
		if tolerated := tc.toleration.ToleratesTaint(&tc.taint); tc.expectTolerated != tolerated {
			t.Errorf("[%s] expect %v, got %v: toleration %+v, taint %s", tc.description, tc.expectTolerated, tolerated, tc.toleration, tc.taint.ToString())
		}
	}
}
