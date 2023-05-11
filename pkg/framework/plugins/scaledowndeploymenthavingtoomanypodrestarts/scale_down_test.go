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

package scaledowndeploymenthavingtoomanypodrestarts

import (
	"context"
	"fmt"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/events"
	"k8s.io/utils/pointer"

	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	frameworkfake "sigs.k8s.io/descheduler/pkg/framework/fake"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
	"sigs.k8s.io/descheduler/test"
)

const namespace = "test"

func testDeployment(name string, replicas int32) (deploy *appsv1.Deployment, rs *appsv1.ReplicaSet) {
	deploy = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
		},
	}

	rs = &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "Deployment", APIVersion: "apps/v1", Name: name},
			},
		},
		Spec: appsv1.ReplicaSetSpec{
			Replicas: &replicas,
		},
	}

	return deploy, rs
}

func testPod(podName, nodeName string, containersRestarts, initContainersRestarts int32, deployName, dsName *string) *v1.Pod {
	pod := test.BuildTestPod(podName, 100, 0, nodeName, nil)
	pod.Namespace = namespace
	pod.Status = v1.PodStatus{
		InitContainerStatuses: []v1.ContainerStatus{
			{
				RestartCount: initContainersRestarts,
			},
		},
		ContainerStatuses: []v1.ContainerStatus{
			{
				RestartCount: containersRestarts,
			},
		},
	}

	if deployName != nil {
		pod.SetOwnerReferences([]metav1.OwnerReference{{Kind: "ReplicaSet", APIVersion: "apps/v1", Name: *deployName}})
	} else if dsName != nil {
		pod.SetOwnerReferences([]metav1.OwnerReference{{Kind: "DaemonSet", APIVersion: "apps/v1", Name: *dsName}})
	}

	return pod
}

func getDeploymentsDeepCopy(deployments ...*appsv1.Deployment) []*appsv1.Deployment {
	deploymentCopy := make([]*appsv1.Deployment, 0, len(deployments))
	for _, deploy := range deployments {
		deploymentCopy = append(deploymentCopy, deploy.DeepCopy())
	}
	return deploymentCopy
}

func TestScaleDownDeploymentHavingTooManyPodRestarts(t *testing.T) {
	node1 := test.BuildTestNode("node-1", 2000, 3000, 10, nil)
	node2 := test.BuildTestNode("node-2", 200, 3000, 10, nil)
	node3 := test.BuildTestNode("node-3", 2000, 3000, 10, nil)

	deployments := make([]*appsv1.Deployment, 4)
	replicasets := make([]*appsv1.ReplicaSet, 4)
	for i := 1; i <= 3; i++ {
		// deploy-i will have i replicas.
		deployments[i], replicasets[i] = testDeployment(fmt.Sprintf("deploy-%d", i), int32(i))
	}

	createScaleDownDeploymentHavingTooManyPodRestartsArgs := func(
		replicasThreshold int32,
		podRestartThresholds int32,
		includingInitContainers bool,
	) ScaleDownDeploymentHavingTooManyPodRestartsArgs {
		return ScaleDownDeploymentHavingTooManyPodRestartsArgs{
			ReplicasThreshold:       pointer.Int32(replicasThreshold),
			PodRestartThreshold:     podRestartThresholds,
			IncludingInitContainers: includingInitContainers,
		}
	}

	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ds-test",
			Namespace: namespace,
		},
	}

	tests := []struct {
		description                     string
		nodes                           []*v1.Node
		deploys                         []*appsv1.Deployment
		rss                             []*appsv1.ReplicaSet
		pods                            []*v1.Pod
		args                            ScaleDownDeploymentHavingTooManyPodRestartsArgs
		expectToBeScaledDownDeployNames map[string]struct{}
	}{
		{
			description: "All pods have total restarts under threshold, no deployments will be scaled down",
			args:        createScaleDownDeploymentHavingTooManyPodRestartsArgs(1, 1, true),
			nodes:       []*v1.Node{node1, node2, node3},
			deploys:     getDeploymentsDeepCopy(deployments[1], deployments[2], deployments[3]),
			rss:         []*appsv1.ReplicaSet{replicasets[1], replicasets[2], replicasets[3]},
			pods: []*v1.Pod{
				// node 1
				testPod("orphan-pod-1", node1.Name, 0, 0, nil, nil),
				testPod("deploy-1-pod-1", node1.Name, 0, 0, &deployments[1].Name, nil),
				testPod("ds-pod-1", node1.Name, 0, 0, nil, &ds.Name),
				// node 2
				testPod("orphan-pod-2", node2.Name, 0, 0, nil, nil),
				testPod("deploy-2-pod-1", node2.Name, 0, 0, &deployments[2].Name, nil),
				testPod("deploy-3-pod-1", node2.Name, 0, 0, &deployments[3].Name, nil),
				testPod("ds-pod-2", node2.Name, 0, 0, nil, &ds.Name),
				// node 3
				testPod("deploy-2-pod-2", node3.Name, 0, 0, &deployments[2].Name, nil),
				testPod("deploy-3-pod-2", node3.Name, 0, 0, &deployments[3].Name, nil),
				testPod("deploy-3-pod-3", node3.Name, 0, 0, &deployments[3].Name, nil),
			},
			expectToBeScaledDownDeployNames: map[string]struct{}{},
		},
		{
			description: "Some pods have total restarts bigger than threshold, and exceed the replicas threshold",
			args:        createScaleDownDeploymentHavingTooManyPodRestartsArgs(1, 1, true),
			nodes:       []*v1.Node{node1, node2, node3},
			deploys:     getDeploymentsDeepCopy(deployments[1], deployments[2], deployments[3]),
			rss:         []*appsv1.ReplicaSet{replicasets[1], replicasets[2], replicasets[3]},
			pods: []*v1.Pod{
				// node 1
				testPod("orphan-pod-1", node1.Name, 0, 0, nil, nil),
				testPod("deploy-1-pod-1", node1.Name, 2, 0, &deployments[1].Name, nil),
				testPod("ds-pod-1", node1.Name, 0, 0, nil, &ds.Name),
				// node 2
				testPod("orphan-pod-2", node2.Name, 0, 0, nil, nil),
				testPod("deploy-2-pod-1", node2.Name, 0, 0, &deployments[2].Name, nil),
				testPod("deploy-3-pod-1", node2.Name, 0, 0, &deployments[3].Name, nil),
				testPod("ds-pod-2", node2.Name, 0, 0, nil, &ds.Name),
				// node 3
				testPod("deploy-2-pod-2", node3.Name, 0, 0, &deployments[2].Name, nil),
				testPod("deploy-3-pod-2", node3.Name, 0, 0, &deployments[3].Name, nil),
				testPod("deploy-3-pod-3", node3.Name, 0, 2, &deployments[3].Name, nil),
			},
			expectToBeScaledDownDeployNames: map[string]struct{}{
				deployments[1].Name: {},
				deployments[3].Name: {},
			},
		},
		{
			description: "Some pods have total restarts bigger than threshold, but do not exceed the replicas threshold",
			args:        createScaleDownDeploymentHavingTooManyPodRestartsArgs(2, 1, true),
			nodes:       []*v1.Node{node1, node2, node3},
			deploys:     getDeploymentsDeepCopy(deployments[1], deployments[2], deployments[3]),
			rss:         []*appsv1.ReplicaSet{replicasets[1], replicasets[2], replicasets[3]},
			pods: []*v1.Pod{
				// node 1
				testPod("orphan-pod-1", node1.Name, 1, 0, nil, nil),
				testPod("deploy-1-pod-1", node1.Name, 2, 0, &deployments[1].Name, nil),
				testPod("ds-pod-1", node1.Name, 3, 0, nil, &ds.Name),
				// node 2
				testPod("orphan-pod-2", node2.Name, 0, 4, nil, nil),
				testPod("deploy-2-pod-1", node2.Name, 5, 0, &deployments[2].Name, nil),
				testPod("deploy-3-pod-1", node2.Name, 0, 6, &deployments[3].Name, nil),
				testPod("ds-pod-2", node2.Name, 7, 0, nil, &ds.Name),
				// node 3
				testPod("deploy-2-pod-2", node3.Name, 0, 8, &deployments[2].Name, nil),
				testPod("deploy-3-pod-2", node3.Name, 0, 9, &deployments[3].Name, nil),
				testPod("deploy-3-pod-3", node3.Name, 10, 2, &deployments[3].Name, nil),
			},
			expectToBeScaledDownDeployNames: map[string]struct{}{
				deployments[2].Name: {},
				deployments[3].Name: {},
			},
		},
		{
			description: "deployments where all pods' restarts exceed the threshold will be scaled down",
			args:        createScaleDownDeploymentHavingTooManyPodRestartsArgs(0, 10, true),
			nodes:       []*v1.Node{node1, node2, node3},
			deploys:     getDeploymentsDeepCopy(deployments[1], deployments[2], deployments[3]),
			rss:         []*appsv1.ReplicaSet{replicasets[1], replicasets[2], replicasets[3]},
			pods: []*v1.Pod{
				// node 1
				testPod("orphan-pod-1", node1.Name, 1, 0, nil, nil),
				testPod("deploy-1-pod-1", node1.Name, 0, 0, &deployments[1].Name, nil),
				testPod("ds-pod-1", node1.Name, 3, 0, nil, &ds.Name),
				// node 2
				testPod("orphan-pod-2", node2.Name, 0, 4, nil, nil),
				testPod("deploy-2-pod-1", node2.Name, 5, 0, &deployments[2].Name, nil),
				testPod("deploy-3-pod-1", node2.Name, 4, 6, &deployments[3].Name, nil),
				testPod("ds-pod-2", node2.Name, 7, 0, nil, &ds.Name),
				// node 3
				testPod("deploy-2-pod-2", node3.Name, 1, 2, &deployments[2].Name, nil),
				testPod("deploy-3-pod-2", node3.Name, 1, 9, &deployments[3].Name, nil),
				testPod("deploy-3-pod-3", node3.Name, 10, 2, &deployments[3].Name, nil),
			},
			expectToBeScaledDownDeployNames: map[string]struct{}{
				deployments[3].Name: {},
			},
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
			for _, deploy := range tc.deploys {
				objs = append(objs, deploy)
			}
			for _, rs := range tc.rss {
				objs = append(objs, rs)
			}
			for _, pod := range tc.pods {
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
				tc.nodes,
				false,
				eventRecorder,
			)

			defaultevictorArgs := &defaultevictor.DefaultEvictorArgs{
				EvictLocalStoragePods:   false,
				EvictSystemCriticalPods: false,
				IgnorePvcPods:           false,
				EvictFailedBarePods:     false,
			}

			evictorFilter, err := defaultevictor.New(
				defaultevictorArgs,
				&frameworkfake.HandleImpl{
					ClientsetImpl:                 fakeClient,
					GetPodsAssignedToNodeFuncImpl: getPodsAssignedToNode,
					SharedInformerFactoryImpl:     sharedInformerFactory,
				},
			)
			if err != nil {
				t.Fatalf("Unable to initialize the plugin: %v", err)
			}

			plugin, err := New(
				&tc.args,
				&frameworkfake.HandleImpl{
					ClientsetImpl:                 fakeClient,
					PodEvictorImpl:                podEvictor,
					EvictorFilterImpl:             evictorFilter.(frameworktypes.EvictorPlugin),
					SharedInformerFactoryImpl:     sharedInformerFactory,
					GetPodsAssignedToNodeFuncImpl: getPodsAssignedToNode,
				})
			if err != nil {
				t.Fatalf("Unable to initialize the plugin: %v", err)
			}

			plugin.(frameworktypes.DeschedulePlugin).Deschedule(ctx, tc.nodes)

			for _, d := range tc.deploys {
				deploy, err := fakeClient.AppsV1().Deployments(namespace).Get(ctx, d.Name, metav1.GetOptions{})
				if err != nil {
					t.Fatalf("Unable to get deployment %s: %v", d.Name, err)
				}
				if _, ok := tc.expectToBeScaledDownDeployNames[deploy.Name]; ok && deploy.Spec.Replicas != nil && *deploy.Spec.Replicas != 0 {
					t.Fatalf("deployment %s should be scaled down to 0 replicas", deploy.Name)
				} else if !ok && deploy.Spec.Replicas != nil && *deploy.Spec.Replicas == 0 {
					t.Fatalf("deployment %s should not be scaled down to 0 replicas", deploy.Name)
				}
			}

			for _, r := range tc.rss {
				rs, err := fakeClient.AppsV1().ReplicaSets(namespace).Get(ctx, r.Name, metav1.GetOptions{})
				if err != nil {
					t.Fatalf("Unable to get replicaset %s: %v", r.Name, err)
				}
				if rs.Spec.Replicas != nil && *rs.Spec.Replicas == 0 {
					t.Fatalf("replicaset %s should not be scaled down to 0 replicas", rs.Name)
				}
			}
		})
	}
}
