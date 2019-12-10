package strategies

import (
	"fmt"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"sigs.k8s.io/descheduler/test"
)

func createNoScheduleTaint(key, value string, index int) v1.Taint {
	return v1.Taint{
		Key:    "testTaint" + fmt.Sprintf("%v", index),
		Value:  "test" + fmt.Sprintf("%v", index),
		Effect: v1.TaintEffectNoSchedule,
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

func addTolerationToPod(pod *v1.Pod, key, value string, index int) *v1.Pod {
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}

	pod.Spec.Tolerations = []v1.Toleration{{Key: key + fmt.Sprintf("%v", index), Value: value + fmt.Sprintf("%v", index), Effect: v1.TaintEffectNoSchedule}}

	return pod
}

func TestDeletePodsViolatingNodeTaints(t *testing.T) {

	node1 := test.BuildTestNode("n1", 2000, 3000, 10)
	node1 = addTaintsToNode(node1, "testTaint", "test", []int{1})
	node2 := test.BuildTestNode("n2", 2000, 3000, 10)
	node1 = addTaintsToNode(node2, "testingTaint", "testing", []int{1})

	p1 := test.BuildTestPod("p1", 100, 0, node1.Name)
	p2 := test.BuildTestPod("p2", 100, 0, node1.Name)
	p3 := test.BuildTestPod("p3", 100, 0, node1.Name)
	p4 := test.BuildTestPod("p4", 100, 0, node1.Name)
	p5 := test.BuildTestPod("p5", 100, 0, node1.Name)
	p6 := test.BuildTestPod("p6", 100, 0, node1.Name)

	p1.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
	p2.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
	p3.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
	p4.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
	p5.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
	p6.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
	p7 := test.BuildTestPod("p7", 100, 0, node2.Name)
	p8 := test.BuildTestPod("p8", 100, 0, node2.Name)
	p9 := test.BuildTestPod("p9", 100, 0, node2.Name)
	p10 := test.BuildTestPod("p10", 100, 0, node2.Name)
	p11 := test.BuildTestPod("p11", 100, 0, node2.Name)
	p11.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()

	// The following 4 pods won't get evicted.
	// A Critical Pod.
	p7.Namespace = "kube-system"
	p7.Annotations = test.GetCriticalPodAnnotation()

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

	p1 = addTolerationToPod(p1, "testTaint", "test", 1)
	p3 = addTolerationToPod(p3, "testTaint", "test", 1)
	p4 = addTolerationToPod(p4, "testTaintX", "testX", 1)

	tests := []struct {
		description             string
		nodes                   []*v1.Node
		pods                    []v1.Pod
		evictLocalStoragePods   bool
		npe                     nodePodEvictedCount
		maxPodsToEvict          int
		expectedEvictedPodCount int
	}{

		{
			description:             "Pods not tolerating node taint should be evicted",
			pods:                    []v1.Pod{*p1, *p2, *p3},
			nodes:                   []*v1.Node{node1},
			evictLocalStoragePods:   false,
			npe:                     nodePodEvictedCount{node1: 0},
			maxPodsToEvict:          0,
			expectedEvictedPodCount: 1, //p2 gets evicted
		},
		{
			description:             "Pods with tolerations but not tolerating node taint should be evicted",
			pods:                    []v1.Pod{*p1, *p3, *p4},
			nodes:                   []*v1.Node{node1},
			evictLocalStoragePods:   false,
			npe:                     nodePodEvictedCount{node1: 0},
			maxPodsToEvict:          0,
			expectedEvictedPodCount: 1, //p4 gets evicted
		},
		{
			description:             "Only <maxPodsToEvict> number of Pods not tolerating node taint should be evicted",
			pods:                    []v1.Pod{*p1, *p5, *p6},
			nodes:                   []*v1.Node{node1},
			evictLocalStoragePods:   false,
			npe:                     nodePodEvictedCount{node1: 0},
			maxPodsToEvict:          1,
			expectedEvictedPodCount: 1, //p5 or p6 gets evicted
		},
		{
			description:             "Critical pods not tolerating node taint should not be evicted",
			pods:                    []v1.Pod{*p7, *p8, *p9, *p10},
			nodes:                   []*v1.Node{node2},
			evictLocalStoragePods:   false,
			npe:                     nodePodEvictedCount{node2: 0},
			maxPodsToEvict:          0,
			expectedEvictedPodCount: 0,
		},
		{
			description:             "Critical pods except storage pods not tolerating node taint should not be evicted",
			pods:                    []v1.Pod{*p7, *p8, *p9, *p10},
			nodes:                   []*v1.Node{node2},
			evictLocalStoragePods:   true,
			npe:                     nodePodEvictedCount{node2: 0},
			maxPodsToEvict:          0,
			expectedEvictedPodCount: 1,
		},
		{
			description:             "Critical and non critical pods, only non critical pods not tolerating node taint should be evicted",
			pods:                    []v1.Pod{*p7, *p8, *p10, *p11},
			nodes:                   []*v1.Node{node2},
			evictLocalStoragePods:   false,
			npe:                     nodePodEvictedCount{node2: 0},
			maxPodsToEvict:          0,
			expectedEvictedPodCount: 1,
		},
	}

	for _, tc := range tests {
		labelSelector := ""
		// create fake client
		fakeClient := &fake.Clientset{}
		fakeClient.Fake.AddReactor("list", "pods", func(action core.Action) (bool, runtime.Object, error) {
			return true, &v1.PodList{Items: tc.pods}, nil
		})

		actualEvictedPodCount := deletePodsViolatingNodeTaints(fakeClient, "v1", tc.nodes, false, tc.npe, labelSelector, tc.maxPodsToEvict, tc.evictLocalStoragePods)
		if actualEvictedPodCount != tc.expectedEvictedPodCount {
			t.Errorf("Test %#v failed, Unexpected no of pods evicted: pods evicted: %d, expected: %d", tc.description, actualEvictedPodCount, tc.expectedEvictedPodCount)
		}
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
				Operator: TolerationOpExists,
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
				Operator: TolerationOpExists,
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
				Operator: TolerationOpExists,
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
				Operator: TolerationOpEqual,
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
				Operator: TolerationOpEqual,
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
				Operator: TolerationOpEqual,
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
		if tolerated := toleratesTaint(&tc.toleration, &tc.taint); tc.expectTolerated != tolerated {
			t.Errorf("[%s] expect %v, got %v: toleration %+v, taint %s", tc.description, tc.expectTolerated, tolerated, tc.toleration, tc.taint.ToString())
		}
	}
}

func TestFilterNoExecuteTaints(t *testing.T) {
	taints := []v1.Taint{
		{
			Key:    "one",
			Value:  "one",
			Effect: v1.TaintEffectNoExecute,
		},
		{
			Key:    "two",
			Value:  "two",
			Effect: v1.TaintEffectNoSchedule,
		},
	}
	taints = getNoScheduleTaints(taints)
	if len(taints) != 1 || taints[0].Key != "two" {
		t.Errorf("Filtering doesn't work. Got %v", taints)
	}
}
