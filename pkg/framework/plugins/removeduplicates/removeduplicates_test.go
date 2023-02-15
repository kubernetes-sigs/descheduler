/*
Copyright 2022 The Kubernetes Authors.

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

package removeduplicates

import (
	"context"
	"testing"

	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/descheduler/pkg/framework"
	frameworkfake "sigs.k8s.io/descheduler/pkg/framework/fake"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"

	v1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/utils"
	"sigs.k8s.io/descheduler/test"
)

func buildTestPodWithImage(podName, node, image string) *v1.Pod {
	pod := test.BuildTestPod(podName, 100, 0, node, test.SetRSOwnerRef)
	pod.Spec.Containers = append(pod.Spec.Containers, v1.Container{
		Name:  image,
		Image: image,
	})
	return pod
}

func TestFindDuplicatePods(t *testing.T) {
	// first setup pods
	node1 := test.BuildTestNode("n1", 2000, 3000, 10, nil)
	node2 := test.BuildTestNode("n2", 2000, 3000, 10, nil)
	node3 := test.BuildTestNode("n3", 2000, 3000, 10, func(node *v1.Node) {
		node.Spec.Taints = []v1.Taint{
			{
				Key:    "hardware",
				Value:  "gpu",
				Effect: v1.TaintEffectNoSchedule,
			},
		}
	})
	node4 := test.BuildTestNode("n4", 2000, 3000, 10, func(node *v1.Node) {
		node.ObjectMeta.Labels = map[string]string{
			"datacenter": "east",
		}
	})
	node5 := test.BuildTestNode("n5", 2000, 3000, 10, func(node *v1.Node) {
		node.Spec = v1.NodeSpec{
			Unschedulable: true,
		}
	})
	node6 := test.BuildTestNode("n6", 200, 200, 10, nil)

	p1 := test.BuildTestPod("p1", 100, 0, node1.Name, nil)
	p1.Namespace = "dev"
	p2 := test.BuildTestPod("p2", 100, 0, node1.Name, nil)
	p2.Namespace = "dev"
	p3 := test.BuildTestPod("p3", 100, 0, node1.Name, nil)
	p3.Namespace = "dev"
	p4 := test.BuildTestPod("p4", 100, 0, node1.Name, nil)
	p5 := test.BuildTestPod("p5", 100, 0, node1.Name, nil)
	p6 := test.BuildTestPod("p6", 100, 0, node1.Name, nil)
	p7 := test.BuildTestPod("p7", 100, 0, node1.Name, nil)
	p7.Namespace = "kube-system"
	p8 := test.BuildTestPod("p8", 100, 0, node1.Name, nil)
	p8.Namespace = "test"
	p9 := test.BuildTestPod("p9", 100, 0, node1.Name, nil)
	p9.Namespace = "test"
	p10 := test.BuildTestPod("p10", 100, 0, node1.Name, nil)
	p10.Namespace = "test"
	p11 := test.BuildTestPod("p11", 100, 0, node1.Name, nil)
	p11.Namespace = "different-images"
	p12 := test.BuildTestPod("p12", 100, 0, node1.Name, nil)
	p12.Namespace = "different-images"
	p13 := test.BuildTestPod("p13", 100, 0, node1.Name, nil)
	p13.Namespace = "different-images"
	p14 := test.BuildTestPod("p14", 100, 0, node1.Name, nil)
	p14.Namespace = "different-images"
	p15 := test.BuildTestPod("p15", 100, 0, node1.Name, nil)
	p15.Namespace = "node-fit"
	p16 := test.BuildTestPod("NOT1", 100, 0, node1.Name, nil)
	p16.Namespace = "node-fit"
	p17 := test.BuildTestPod("NOT2", 100, 0, node1.Name, nil)
	p17.Namespace = "node-fit"
	p18 := test.BuildTestPod("TARGET", 100, 0, node1.Name, nil)
	p18.Namespace = "node-fit"

	// This pod sits on node6 and is used to take up CPU requests on the node
	p19 := test.BuildTestPod("CPU-eater", 150, 150, node6.Name, nil)
	p19.Namespace = "test"

	// Dummy pod for node6 used to do the opposite of p19
	p20 := test.BuildTestPod("CPU-saver", 100, 150, node6.Name, nil)
	p20.Namespace = "test"

	// ### Evictable Pods ###

	// Three Pods in the "default" Namespace, bound to same ReplicaSet. 2 should be evicted.
	ownerRef1 := test.GetReplicaSetOwnerRefList()
	p1.ObjectMeta.OwnerReferences = ownerRef1
	p2.ObjectMeta.OwnerReferences = ownerRef1
	p3.ObjectMeta.OwnerReferences = ownerRef1

	// Three Pods in the "test" Namespace, bound to same ReplicaSet. 2 should be evicted.
	ownerRef2 := test.GetReplicaSetOwnerRefList()
	p8.ObjectMeta.OwnerReferences = ownerRef2
	p9.ObjectMeta.OwnerReferences = ownerRef2
	p10.ObjectMeta.OwnerReferences = ownerRef2

	// ### Non-evictable Pods ###

	// A DaemonSet.
	p4.ObjectMeta.OwnerReferences = test.GetDaemonSetOwnerRefList()

	// A Pod with local storage.
	p5.ObjectMeta.OwnerReferences = test.GetNormalPodOwnerRefList()
	p5.Spec.Volumes = []v1.Volume{
		{
			Name: "sample",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{Path: "somePath"},
				EmptyDir: &v1.EmptyDirVolumeSource{
					SizeLimit: resource.NewQuantity(int64(10), resource.BinarySI),
				},
			},
		},
	}

	// A Mirror Pod.
	p6.Annotations = test.GetMirrorPodAnnotation()

	// A Critical Pod.
	priority := utils.SystemCriticalPriority
	p7.Spec.Priority = &priority

	// Same owners, but different images
	p11.Spec.Containers[0].Image = "foo"
	p11.ObjectMeta.OwnerReferences = ownerRef1
	p12.Spec.Containers[0].Image = "bar"
	p12.ObjectMeta.OwnerReferences = ownerRef1

	// Multiple containers
	p13.ObjectMeta.OwnerReferences = ownerRef1
	p13.Spec.Containers = append(p13.Spec.Containers, v1.Container{
		Name:  "foo",
		Image: "foo",
	})

	// ### Pods Evictable Based On Node Fit ###

	ownerRef3 := test.GetReplicaSetOwnerRefList()
	p15.ObjectMeta.OwnerReferences = ownerRef3
	p16.ObjectMeta.OwnerReferences = ownerRef3
	p17.ObjectMeta.OwnerReferences = ownerRef3
	p18.ObjectMeta.OwnerReferences = ownerRef3

	p15.Spec.NodeSelector = map[string]string{
		"datacenter": "west",
	}
	p16.Spec.NodeSelector = map[string]string{
		"datacenter": "west",
	}
	p17.Spec.NodeSelector = map[string]string{
		"datacenter": "west",
	}

	testCases := []struct {
		description             string
		pods                    []*v1.Pod
		nodes                   []*v1.Node
		expectedEvictedPodCount uint
		excludeOwnerKinds       []string
		nodefit                 bool
	}{
		{
			description:             "Three pods in the `dev` Namespace, bound to same ReplicaSet. 1 should be evicted.",
			pods:                    []*v1.Pod{p1, p2, p3},
			nodes:                   []*v1.Node{node1, node2},
			expectedEvictedPodCount: 1,
		},
		{
			description:             "Three pods in the `dev` Namespace, bound to same ReplicaSet, but ReplicaSet kind is excluded. 0 should be evicted.",
			pods:                    []*v1.Pod{p1, p2, p3},
			nodes:                   []*v1.Node{node1, node2},
			expectedEvictedPodCount: 0,
			excludeOwnerKinds:       []string{"ReplicaSet"},
		},
		{
			description:             "Three Pods in the `test` Namespace, bound to same ReplicaSet. 1 should be evicted.",
			pods:                    []*v1.Pod{p8, p9, p10},
			nodes:                   []*v1.Node{node1, node2},
			expectedEvictedPodCount: 1,
		},
		{
			description:             "Three Pods in the `dev` Namespace, three Pods in the `test` Namespace. Bound to ReplicaSet with same name. 4 should be evicted.",
			pods:                    []*v1.Pod{p1, p2, p3, p8, p9, p10},
			nodes:                   []*v1.Node{node1, node2},
			expectedEvictedPodCount: 2,
		},
		{
			description:             "Pods are: part of DaemonSet, with local storage, mirror pod annotation, critical pod annotation - none should be evicted.",
			pods:                    []*v1.Pod{p4, p5, p6, p7},
			nodes:                   []*v1.Node{node1, node2},
			expectedEvictedPodCount: 0,
		},
		{
			description:             "Test all Pods: 4 should be evicted.",
			pods:                    []*v1.Pod{p1, p2, p3, p4, p5, p6, p7, p8, p9, p10},
			nodes:                   []*v1.Node{node1, node2},
			expectedEvictedPodCount: 2,
		},
		{
			description:             "Pods with the same owner but different images should not be evicted",
			pods:                    []*v1.Pod{p11, p12},
			nodes:                   []*v1.Node{node1, node2},
			expectedEvictedPodCount: 0,
		},
		{
			description:             "Pods with multiple containers should not match themselves",
			pods:                    []*v1.Pod{p13},
			nodes:                   []*v1.Node{node1, node2},
			expectedEvictedPodCount: 0,
		},
		{
			description:             "Pods with matching ownerrefs and at not all matching image should not trigger an eviction",
			pods:                    []*v1.Pod{p11, p13},
			nodes:                   []*v1.Node{node1, node2},
			expectedEvictedPodCount: 0,
		},
		{
			description:             "Three pods in the `dev` Namespace, bound to same ReplicaSet. Only node available has a taint, and nodeFit set to true. 0 should be evicted.",
			pods:                    []*v1.Pod{p1, p2, p3},
			nodes:                   []*v1.Node{node1, node3},
			expectedEvictedPodCount: 0,
			nodefit:                 true,
		},
		{
			description:             "Three pods in the `node-fit` Namespace, bound to same ReplicaSet, all with a nodeSelector. Only node available has an incorrect node label, and nodeFit set to true. 0 should be evicted.",
			pods:                    []*v1.Pod{p15, p16, p17},
			nodes:                   []*v1.Node{node1, node4},
			expectedEvictedPodCount: 0,
			nodefit:                 true,
		},
		{
			description:             "Three pods in the `node-fit` Namespace, bound to same ReplicaSet. Only node available is not schedulable, and nodeFit set to true. 0 should be evicted.",
			pods:                    []*v1.Pod{p1, p2, p3},
			nodes:                   []*v1.Node{node1, node5},
			expectedEvictedPodCount: 0,
			nodefit:                 true,
		},
		{
			description:             "Three pods in the `node-fit` Namespace, bound to same ReplicaSet. Only node available does not have enough CPU, and nodeFit set to true. 0 should be evicted.",
			pods:                    []*v1.Pod{p1, p2, p3, p19},
			nodes:                   []*v1.Node{node1, node6},
			expectedEvictedPodCount: 0,
			nodefit:                 true,
		},
		{
			description:             "Three pods in the `node-fit` Namespace, bound to same ReplicaSet. Only node available has enough CPU, and nodeFit set to true. 1 should be evicted.",
			pods:                    []*v1.Pod{p1, p2, p3, p20},
			nodes:                   []*v1.Node{node1, node6},
			expectedEvictedPodCount: 1,
			nodefit:                 true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.description, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			var objs []runtime.Object
			for _, node := range testCase.nodes {
				objs = append(objs, node)
			}
			for _, pod := range testCase.pods {
				objs = append(objs, pod)
			}
			fakeClient := fake.NewSimpleClientset(objs...)

			sharedInformerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
			podInformer := sharedInformerFactory.Core().V1().Pods().Informer()

			getPodsAssignedToNode, err := podutil.BuildGetPodsAssignedToNodeFunc(podInformer)
			if err != nil {
				t.Errorf("Build get pods assigned to node function error: %v", err)
			}

			sharedInformerFactory.Start(ctx.Done())
			sharedInformerFactory.WaitForCacheSync(ctx.Done())

			eventRecorder := &events.FakeRecorder{}

			podEvictor := evictions.NewPodEvictor(
				fakeClient,
				"v1",
				false,
				nil,
				nil,
				nil,
				testCase.nodes,
				false,
				eventRecorder,
			)

			nodeFit := testCase.nodefit

			defaultEvictorFilterArgs := &defaultevictor.DefaultEvictorArgs{
				EvictLocalStoragePods:   false,
				EvictSystemCriticalPods: false,
				IgnorePvcPods:           false,
				EvictFailedBarePods:     false,
				NodeFit:                 nodeFit,
			}

			evictorFilter, _ := defaultevictor.New(
				defaultEvictorFilterArgs,
				&frameworkfake.HandleImpl{
					ClientsetImpl:                 fakeClient,
					GetPodsAssignedToNodeFuncImpl: getPodsAssignedToNode,
					SharedInformerFactoryImpl:     sharedInformerFactory,
				},
			)

			handle := &frameworkfake.HandleImpl{
				ClientsetImpl:                 fakeClient,
				GetPodsAssignedToNodeFuncImpl: getPodsAssignedToNode,
				PodEvictorImpl:                podEvictor,
				EvictorFilterImpl:             evictorFilter.(framework.EvictorPlugin),
				SharedInformerFactoryImpl:     sharedInformerFactory,
			}

			plugin, err := New(&RemoveDuplicatesArgs{
				ExcludeOwnerKinds: testCase.excludeOwnerKinds,
			},
				handle,
			)
			if err != nil {
				t.Fatalf("Unable to initialize the plugin: %v", err)
			}

			plugin.(framework.BalancePlugin).Balance(ctx, testCase.nodes)
			actualEvictedPodCount := podEvictor.TotalEvicted()
			if actualEvictedPodCount != testCase.expectedEvictedPodCount {
				t.Errorf("Test %#v failed, Unexpected no of pods evicted: pods evicted: %d, expected: %d", testCase.description, actualEvictedPodCount, testCase.expectedEvictedPodCount)
			}
		})
	}
}

func TestRemoveDuplicatesUniformly(t *testing.T) {
	setRSOwnerRef2 := func(pod *v1.Pod) {
		pod.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
			{Kind: "ReplicaSet", APIVersion: "v1", Name: "replicaset-2"},
		}
	}
	setTwoRSOwnerRef := func(pod *v1.Pod) {
		pod.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
			{Kind: "ReplicaSet", APIVersion: "v1", Name: "replicaset-1"},
			{Kind: "ReplicaSet", APIVersion: "v1", Name: "replicaset-2"},
		}
	}

	setTolerationsK1 := func(pod *v1.Pod) {
		test.SetRSOwnerRef(pod)
		pod.Spec.Tolerations = []v1.Toleration{
			{
				Key:      "k1",
				Value:    "v1",
				Operator: v1.TolerationOpEqual,
				Effect:   v1.TaintEffectNoSchedule,
			},
		}
	}
	setTolerationsK2 := func(pod *v1.Pod) {
		test.SetRSOwnerRef(pod)
		pod.Spec.Tolerations = []v1.Toleration{
			{
				Key:      "k2",
				Value:    "v2",
				Operator: v1.TolerationOpEqual,
				Effect:   v1.TaintEffectNoSchedule,
			},
		}
	}

	setMasterNoScheduleTaint := func(node *v1.Node) {
		node.Spec.Taints = []v1.Taint{
			{
				Effect: v1.TaintEffectNoSchedule,
				Key:    "node-role.kubernetes.io/control-plane",
			},
		}
	}

	setMasterNoScheduleLabel := func(node *v1.Node) {
		if node.ObjectMeta.Labels == nil {
			node.ObjectMeta.Labels = map[string]string{}
		}
		node.ObjectMeta.Labels["node-role.kubernetes.io/control-plane"] = ""
	}

	setWorkerLabel := func(node *v1.Node) {
		if node.ObjectMeta.Labels == nil {
			node.ObjectMeta.Labels = map[string]string{}
		}
		node.ObjectMeta.Labels["node-role.kubernetes.io/worker"] = "k1"
		node.ObjectMeta.Labels["node-role.kubernetes.io/worker"] = "k2"
	}

	setNotMasterNodeSelectorK1 := func(pod *v1.Pod) {
		test.SetRSOwnerRef(pod)
		pod.Spec.Affinity = &v1.Affinity{
			NodeAffinity: &v1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
					NodeSelectorTerms: []v1.NodeSelectorTerm{
						{
							MatchExpressions: []v1.NodeSelectorRequirement{
								{
									Key:      "node-role.kubernetes.io/control-plane",
									Operator: v1.NodeSelectorOpDoesNotExist,
								},
								{
									Key:      "k1",
									Operator: v1.NodeSelectorOpDoesNotExist,
								},
							},
						},
					},
				},
			},
		}
	}

	setNotMasterNodeSelectorK2 := func(pod *v1.Pod) {
		test.SetRSOwnerRef(pod)
		pod.Spec.Affinity = &v1.Affinity{
			NodeAffinity: &v1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
					NodeSelectorTerms: []v1.NodeSelectorTerm{
						{
							MatchExpressions: []v1.NodeSelectorRequirement{
								{
									Key:      "node-role.kubernetes.io/control-plane",
									Operator: v1.NodeSelectorOpDoesNotExist,
								},
								{
									Key:      "k2",
									Operator: v1.NodeSelectorOpDoesNotExist,
								},
							},
						},
					},
				},
			},
		}
	}

	setWorkerLabelSelectorK1 := func(pod *v1.Pod) {
		test.SetRSOwnerRef(pod)
		if pod.Spec.NodeSelector == nil {
			pod.Spec.NodeSelector = map[string]string{}
		}
		pod.Spec.NodeSelector["node-role.kubernetes.io/worker"] = "k1"
	}

	setWorkerLabelSelectorK2 := func(pod *v1.Pod) {
		test.SetRSOwnerRef(pod)
		if pod.Spec.NodeSelector == nil {
			pod.Spec.NodeSelector = map[string]string{}
		}
		pod.Spec.NodeSelector["node-role.kubernetes.io/worker"] = "k2"
	}

	testCases := []struct {
		description             string
		pods                    []*v1.Pod
		nodes                   []*v1.Node
		expectedEvictedPodCount uint
	}{
		{
			description: "Evict pods uniformly",
			pods: []*v1.Pod{
				// (5,3,1) -> (3,3,3) -> 2 evictions
				test.BuildTestPod("p1", 100, 0, "n1", test.SetRSOwnerRef),
				test.BuildTestPod("p2", 100, 0, "n1", test.SetRSOwnerRef),
				test.BuildTestPod("p3", 100, 0, "n1", test.SetRSOwnerRef),
				test.BuildTestPod("p4", 100, 0, "n1", test.SetRSOwnerRef),
				test.BuildTestPod("p5", 100, 0, "n1", test.SetRSOwnerRef),
				test.BuildTestPod("p6", 100, 0, "n2", test.SetRSOwnerRef),
				test.BuildTestPod("p7", 100, 0, "n2", test.SetRSOwnerRef),
				test.BuildTestPod("p8", 100, 0, "n2", test.SetRSOwnerRef),
				test.BuildTestPod("p9", 100, 0, "n3", test.SetRSOwnerRef),
			},
			expectedEvictedPodCount: 2,
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 2000, 3000, 10, nil),
				test.BuildTestNode("n2", 2000, 3000, 10, nil),
				test.BuildTestNode("n3", 2000, 3000, 10, nil),
			},
		},
		{
			description: "Evict pods uniformly with one node left out",
			pods: []*v1.Pod{
				// (5,3,1) -> (4,4,1) -> 1 eviction
				test.BuildTestPod("p1", 100, 0, "n1", test.SetRSOwnerRef),
				test.BuildTestPod("p2", 100, 0, "n1", test.SetRSOwnerRef),
				test.BuildTestPod("p3", 100, 0, "n1", test.SetRSOwnerRef),
				test.BuildTestPod("p4", 100, 0, "n1", test.SetRSOwnerRef),
				test.BuildTestPod("p5", 100, 0, "n1", test.SetRSOwnerRef),
				test.BuildTestPod("p6", 100, 0, "n2", test.SetRSOwnerRef),
				test.BuildTestPod("p7", 100, 0, "n2", test.SetRSOwnerRef),
				test.BuildTestPod("p8", 100, 0, "n2", test.SetRSOwnerRef),
				test.BuildTestPod("p9", 100, 0, "n3", test.SetRSOwnerRef),
			},
			expectedEvictedPodCount: 1,
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 2000, 3000, 10, nil),
				test.BuildTestNode("n2", 2000, 3000, 10, nil),
			},
		},
		{
			description: "Evict pods uniformly with two replica sets",
			pods: []*v1.Pod{
				// (5,3,1) -> (3,3,3) -> 2 evictions
				test.BuildTestPod("p11", 100, 0, "n1", setTwoRSOwnerRef),
				test.BuildTestPod("p12", 100, 0, "n1", setTwoRSOwnerRef),
				test.BuildTestPod("p13", 100, 0, "n1", setTwoRSOwnerRef),
				test.BuildTestPod("p14", 100, 0, "n1", setTwoRSOwnerRef),
				test.BuildTestPod("p15", 100, 0, "n1", setTwoRSOwnerRef),
				test.BuildTestPod("p16", 100, 0, "n2", setTwoRSOwnerRef),
				test.BuildTestPod("p17", 100, 0, "n2", setTwoRSOwnerRef),
				test.BuildTestPod("p18", 100, 0, "n2", setTwoRSOwnerRef),
				test.BuildTestPod("p19", 100, 0, "n3", setTwoRSOwnerRef),
			},
			expectedEvictedPodCount: 4,
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 2000, 3000, 10, nil),
				test.BuildTestNode("n2", 2000, 3000, 10, nil),
				test.BuildTestNode("n3", 2000, 3000, 10, nil),
			},
		},
		{
			description: "Evict pods uniformly with two owner references",
			pods: []*v1.Pod{
				// (5,3,1) -> (3,3,3) -> 2 evictions
				test.BuildTestPod("p11", 100, 0, "n1", test.SetRSOwnerRef),
				test.BuildTestPod("p12", 100, 0, "n1", test.SetRSOwnerRef),
				test.BuildTestPod("p13", 100, 0, "n1", test.SetRSOwnerRef),
				test.BuildTestPod("p14", 100, 0, "n1", test.SetRSOwnerRef),
				test.BuildTestPod("p15", 100, 0, "n1", test.SetRSOwnerRef),
				test.BuildTestPod("p16", 100, 0, "n2", test.SetRSOwnerRef),
				test.BuildTestPod("p17", 100, 0, "n2", test.SetRSOwnerRef),
				test.BuildTestPod("p18", 100, 0, "n2", test.SetRSOwnerRef),
				test.BuildTestPod("p19", 100, 0, "n3", test.SetRSOwnerRef),
				// (1,3,5) -> (3,3,3) -> 2 evictions
				test.BuildTestPod("p21", 100, 0, "n1", setRSOwnerRef2),
				test.BuildTestPod("p22", 100, 0, "n2", setRSOwnerRef2),
				test.BuildTestPod("p23", 100, 0, "n2", setRSOwnerRef2),
				test.BuildTestPod("p24", 100, 0, "n2", setRSOwnerRef2),
				test.BuildTestPod("p25", 100, 0, "n3", setRSOwnerRef2),
				test.BuildTestPod("p26", 100, 0, "n3", setRSOwnerRef2),
				test.BuildTestPod("p27", 100, 0, "n3", setRSOwnerRef2),
				test.BuildTestPod("p28", 100, 0, "n3", setRSOwnerRef2),
				test.BuildTestPod("p29", 100, 0, "n3", setRSOwnerRef2),
			},
			expectedEvictedPodCount: 4,
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 2000, 3000, 10, nil),
				test.BuildTestNode("n2", 2000, 3000, 10, nil),
				test.BuildTestNode("n3", 2000, 3000, 10, nil),
			},
		},
		{
			description: "Evict pods with number of pods less than nodes",
			pods: []*v1.Pod{
				// (2,0,0) -> (1,1,0) -> 1 eviction
				test.BuildTestPod("p1", 100, 0, "n1", test.SetRSOwnerRef),
				test.BuildTestPod("p2", 100, 0, "n1", test.SetRSOwnerRef),
			},
			expectedEvictedPodCount: 1,
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 2000, 3000, 10, nil),
				test.BuildTestNode("n2", 2000, 3000, 10, nil),
				test.BuildTestNode("n3", 2000, 3000, 10, nil),
			},
		},
		{
			description: "Evict pods with number of pods less than nodes, but ignore different pods with the same ownerref",
			pods: []*v1.Pod{
				// (1, 0, 0) for "bar","baz" images -> no eviction, even with a matching ownerKey
				// (2, 0, 0) for "foo" image -> (1,1,0) - 1 eviction
				// In this case the only "real" duplicates are p1 and p4, so one of those should be evicted
				buildTestPodWithImage("p1", "n1", "foo"),
				buildTestPodWithImage("p2", "n1", "bar"),
				buildTestPodWithImage("p3", "n1", "baz"),
				buildTestPodWithImage("p4", "n1", "foo"),
			},
			expectedEvictedPodCount: 1,
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 2000, 3000, 10, nil),
				test.BuildTestNode("n2", 2000, 3000, 10, nil),
				test.BuildTestNode("n3", 2000, 3000, 10, nil),
			},
		},
		{
			description: "Evict pods with a single pod with three nodes",
			pods: []*v1.Pod{
				// (2,0,0) -> (1,1,0) -> 1 eviction
				test.BuildTestPod("p1", 100, 0, "n1", test.SetRSOwnerRef),
			},
			expectedEvictedPodCount: 0,
			nodes: []*v1.Node{
				test.BuildTestNode("n1", 2000, 3000, 10, nil),
				test.BuildTestNode("n2", 2000, 3000, 10, nil),
				test.BuildTestNode("n3", 2000, 3000, 10, nil),
			},
		},
		{
			description: "Evict pods uniformly respecting taints",
			pods: []*v1.Pod{
				// (5,3,1,0,0,0) -> (3,3,3,0,0,0) -> 2 evictions
				test.BuildTestPod("p1", 100, 0, "worker1", setTolerationsK1),
				test.BuildTestPod("p2", 100, 0, "worker1", setTolerationsK2),
				test.BuildTestPod("p3", 100, 0, "worker1", setTolerationsK1),
				test.BuildTestPod("p4", 100, 0, "worker1", setTolerationsK2),
				test.BuildTestPod("p5", 100, 0, "worker1", setTolerationsK1),
				test.BuildTestPod("p6", 100, 0, "worker2", setTolerationsK2),
				test.BuildTestPod("p7", 100, 0, "worker2", setTolerationsK1),
				test.BuildTestPod("p8", 100, 0, "worker2", setTolerationsK2),
				test.BuildTestPod("p9", 100, 0, "worker3", setTolerationsK1),
			},
			expectedEvictedPodCount: 2,
			nodes: []*v1.Node{
				test.BuildTestNode("worker1", 2000, 3000, 10, nil),
				test.BuildTestNode("worker2", 2000, 3000, 10, nil),
				test.BuildTestNode("worker3", 2000, 3000, 10, nil),
				test.BuildTestNode("master1", 2000, 3000, 10, setMasterNoScheduleTaint),
				test.BuildTestNode("master2", 2000, 3000, 10, setMasterNoScheduleTaint),
				test.BuildTestNode("master3", 2000, 3000, 10, setMasterNoScheduleTaint),
			},
		},
		{
			description: "Evict pods uniformly respecting RequiredDuringSchedulingIgnoredDuringExecution node affinity",
			pods: []*v1.Pod{
				// (5,3,1,0,0,0) -> (3,3,3,0,0,0) -> 2 evictions
				test.BuildTestPod("p1", 100, 0, "worker1", setNotMasterNodeSelectorK1),
				test.BuildTestPod("p2", 100, 0, "worker1", setNotMasterNodeSelectorK2),
				test.BuildTestPod("p3", 100, 0, "worker1", setNotMasterNodeSelectorK1),
				test.BuildTestPod("p4", 100, 0, "worker1", setNotMasterNodeSelectorK2),
				test.BuildTestPod("p5", 100, 0, "worker1", setNotMasterNodeSelectorK1),
				test.BuildTestPod("p6", 100, 0, "worker2", setNotMasterNodeSelectorK2),
				test.BuildTestPod("p7", 100, 0, "worker2", setNotMasterNodeSelectorK1),
				test.BuildTestPod("p8", 100, 0, "worker2", setNotMasterNodeSelectorK2),
				test.BuildTestPod("p9", 100, 0, "worker3", setNotMasterNodeSelectorK1),
			},
			expectedEvictedPodCount: 2,
			nodes: []*v1.Node{
				test.BuildTestNode("worker1", 2000, 3000, 10, nil),
				test.BuildTestNode("worker2", 2000, 3000, 10, nil),
				test.BuildTestNode("worker3", 2000, 3000, 10, nil),
				test.BuildTestNode("master1", 2000, 3000, 10, setMasterNoScheduleLabel),
				test.BuildTestNode("master2", 2000, 3000, 10, setMasterNoScheduleLabel),
				test.BuildTestNode("master3", 2000, 3000, 10, setMasterNoScheduleLabel),
			},
		},
		{
			description: "Evict pods uniformly respecting node selector",
			pods: []*v1.Pod{
				// (5,3,1,0,0,0) -> (3,3,3,0,0,0) -> 2 evictions
				test.BuildTestPod("p1", 100, 0, "worker1", setWorkerLabelSelectorK1),
				test.BuildTestPod("p2", 100, 0, "worker1", setWorkerLabelSelectorK2),
				test.BuildTestPod("p3", 100, 0, "worker1", setWorkerLabelSelectorK1),
				test.BuildTestPod("p4", 100, 0, "worker1", setWorkerLabelSelectorK2),
				test.BuildTestPod("p5", 100, 0, "worker1", setWorkerLabelSelectorK1),
				test.BuildTestPod("p6", 100, 0, "worker2", setWorkerLabelSelectorK2),
				test.BuildTestPod("p7", 100, 0, "worker2", setWorkerLabelSelectorK1),
				test.BuildTestPod("p8", 100, 0, "worker2", setWorkerLabelSelectorK2),
				test.BuildTestPod("p9", 100, 0, "worker3", setWorkerLabelSelectorK1),
			},
			expectedEvictedPodCount: 2,
			nodes: []*v1.Node{
				test.BuildTestNode("worker1", 2000, 3000, 10, setWorkerLabel),
				test.BuildTestNode("worker2", 2000, 3000, 10, setWorkerLabel),
				test.BuildTestNode("worker3", 2000, 3000, 10, setWorkerLabel),
				test.BuildTestNode("master1", 2000, 3000, 10, nil),
				test.BuildTestNode("master2", 2000, 3000, 10, nil),
				test.BuildTestNode("master3", 2000, 3000, 10, nil),
			},
		},
		{
			description: "Evict pods uniformly respecting node selector with zero target nodes",
			pods: []*v1.Pod{
				// (5,3,1,0,0,0) -> (3,3,3,0,0,0) -> 2 evictions
				test.BuildTestPod("p1", 100, 0, "worker1", setWorkerLabelSelectorK1),
				test.BuildTestPod("p2", 100, 0, "worker1", setWorkerLabelSelectorK2),
				test.BuildTestPod("p3", 100, 0, "worker1", setWorkerLabelSelectorK1),
				test.BuildTestPod("p4", 100, 0, "worker1", setWorkerLabelSelectorK2),
				test.BuildTestPod("p5", 100, 0, "worker1", setWorkerLabelSelectorK1),
				test.BuildTestPod("p6", 100, 0, "worker2", setWorkerLabelSelectorK2),
				test.BuildTestPod("p7", 100, 0, "worker2", setWorkerLabelSelectorK1),
				test.BuildTestPod("p8", 100, 0, "worker2", setWorkerLabelSelectorK2),
				test.BuildTestPod("p9", 100, 0, "worker3", setWorkerLabelSelectorK1),
			},
			expectedEvictedPodCount: 0,
			nodes: []*v1.Node{
				test.BuildTestNode("worker1", 2000, 3000, 10, nil),
				test.BuildTestNode("worker2", 2000, 3000, 10, nil),
				test.BuildTestNode("worker3", 2000, 3000, 10, nil),
				test.BuildTestNode("master1", 2000, 3000, 10, nil),
				test.BuildTestNode("master2", 2000, 3000, 10, nil),
				test.BuildTestNode("master3", 2000, 3000, 10, nil),
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.description, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			var objs []runtime.Object
			for _, node := range testCase.nodes {
				objs = append(objs, node)
			}
			for _, pod := range testCase.pods {
				objs = append(objs, pod)
			}
			fakeClient := fake.NewSimpleClientset(objs...)

			sharedInformerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
			podInformer := sharedInformerFactory.Core().V1().Pods().Informer()

			getPodsAssignedToNode, err := podutil.BuildGetPodsAssignedToNodeFunc(podInformer)
			if err != nil {
				t.Errorf("Build get pods assigned to node function error: %v", err)
			}

			sharedInformerFactory.Start(ctx.Done())
			sharedInformerFactory.WaitForCacheSync(ctx.Done())

			eventRecorder := &events.FakeRecorder{}

			podEvictor := evictions.NewPodEvictor(
				fakeClient,
				policyv1.SchemeGroupVersion.String(),
				false,
				nil,
				nil,
				nil,
				testCase.nodes,
				false,
				eventRecorder,
			)

			defaultEvictorFilterArgs := &defaultevictor.DefaultEvictorArgs{
				EvictLocalStoragePods:   false,
				EvictSystemCriticalPods: false,
				IgnorePvcPods:           false,
				EvictFailedBarePods:     false,
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

			plugin, err := New(&RemoveDuplicatesArgs{},
				handle,
			)
			if err != nil {
				t.Fatalf("Unable to initialize the plugin: %v", err)
			}

			plugin.(framework.BalancePlugin).Balance(ctx, testCase.nodes)
			actualEvictedPodCount := podEvictor.TotalEvicted()
			if actualEvictedPodCount != testCase.expectedEvictedPodCount {
				t.Errorf("Test %#v failed, Unexpected no of pods evicted: pods evicted: %d, expected: %d", testCase.description, actualEvictedPodCount, testCase.expectedEvictedPodCount)
			}
		})
	}
}
