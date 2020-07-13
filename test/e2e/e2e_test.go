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

package e2e

import (
	"context"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/cmd/descheduler/app/options"
	"sigs.k8s.io/descheduler/pkg/api"
	deschedulerapi "sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler"
	"sigs.k8s.io/descheduler/pkg/descheduler/client"
	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
	eutils "sigs.k8s.io/descheduler/pkg/descheduler/evictions/utils"
	nodeutil "sigs.k8s.io/descheduler/pkg/descheduler/node"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/descheduler/strategies"
)

func MakePodSpec() v1.PodSpec {
	return v1.PodSpec{
		Containers: []v1.Container{{
			Name:            "pause",
			ImagePullPolicy: "Never",
			Image:           "kubernetes/pause",
			Ports:           []v1.ContainerPort{{ContainerPort: 80}},
			Resources: v1.ResourceRequirements{
				Limits: v1.ResourceList{
					v1.ResourceCPU:    resource.MustParse("100m"),
					v1.ResourceMemory: resource.MustParse("1000Mi"),
				},
				Requests: v1.ResourceList{
					v1.ResourceCPU:    resource.MustParse("100m"),
					v1.ResourceMemory: resource.MustParse("800Mi"),
				},
			},
		}},
	}
}

// RcByNameContainer returns a ReplicationControoler with specified name and container
func RcByNameContainer(name string, replicas int32, labels map[string]string, gracePeriod *int64) *v1.ReplicationController {
	zeroGracePeriod := int64(0)

	// Add "name": name to the labels, overwriting if it exists.
	labels["name"] = name
	if gracePeriod == nil {
		gracePeriod = &zeroGracePeriod
	}
	return &v1.ReplicationController{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ReplicationController",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1.ReplicationControllerSpec{
			Replicas: func(i int32) *int32 { return &i }(replicas),
			Selector: map[string]string{
				"name": name,
			},
			Template: &v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: MakePodSpec(),
			},
		},
	}
}

// startEndToEndForLowNodeUtilization tests the lownode utilization strategy.
func startEndToEndForLowNodeUtilization(ctx context.Context, clientset clientset.Interface, nodeInformer coreinformers.NodeInformer, podEvictor *evictions.PodEvictor) {
	// Run descheduler.
	nodes, err := nodeutil.ReadyNodes(ctx, clientset, nodeInformer, "", nil)
	if err != nil {
		klog.Fatalf("%v", err)
	}

	strategies.LowNodeUtilization(
		ctx,
		clientset,
		deschedulerapi.DeschedulerStrategy{
			Enabled: true,
			Params: &deschedulerapi.StrategyParameters{
				NodeResourceUtilizationThresholds: &deschedulerapi.NodeResourceUtilizationThresholds{
					Thresholds: deschedulerapi.ResourceThresholds{
						v1.ResourceMemory: 20,
						v1.ResourcePods:   20,
						v1.ResourceCPU:    85,
					},
					TargetThresholds: deschedulerapi.ResourceThresholds{
						v1.ResourceMemory: 20,
						v1.ResourcePods:   20,
						v1.ResourceCPU:    90,
					},
				},
			},
		},
		nodes,
		podEvictor,
	)

	time.Sleep(10 * time.Second)
}

func TestLowNodeUtilization(t *testing.T) {
	ctx := context.Background()

	clientSet, err := client.CreateClient(os.Getenv("KUBECONFIG"))
	if err != nil {
		t.Errorf("Error during client creation with %v", err)
	}

	stopChannel := make(chan struct{}, 0)

	sharedInformerFactory := informers.NewSharedInformerFactory(clientSet, 0)
	sharedInformerFactory.Start(stopChannel)
	sharedInformerFactory.WaitForCacheSync(stopChannel)
	defer close(stopChannel)

	nodeInformer := sharedInformerFactory.Core().V1().Nodes()

	nodeList, err := clientSet.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Errorf("Error listing node with %v", err)
	}

	var nodes []*v1.Node
	for i := range nodeList.Items {
		node := nodeList.Items[i]
		nodes = append(nodes, &node)
	}

	rc := RcByNameContainer("test-rc-node-utilization", int32(15), map[string]string{"test": "node-utilization"}, nil)
	if _, err := clientSet.CoreV1().ReplicationControllers(rc.Namespace).Create(ctx, rc, metav1.CreateOptions{}); err != nil {
		t.Errorf("Error creating deployment %v", err)
	}

	evictPods(ctx, t, clientSet, nodeInformer, nodes, rc)
	deleteRC(ctx, t, clientSet, rc)
}

func TestEvictAnnotation(t *testing.T) {
	ctx := context.Background()

	clientSet, err := client.CreateClient(os.Getenv("KUBECONFIG"))
	if err != nil {
		t.Errorf("Error during client creation with %v", err)
	}

	stopChannel := make(chan struct{}, 0)

	sharedInformerFactory := informers.NewSharedInformerFactory(clientSet, 0)
	sharedInformerFactory.Start(stopChannel)
	sharedInformerFactory.WaitForCacheSync(stopChannel)
	defer close(stopChannel)

	nodeInformer := sharedInformerFactory.Core().V1().Nodes()

	nodeList, err := clientSet.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Errorf("Error listing node with %v", err)
	}

	var nodes []*v1.Node
	for i := range nodeList.Items {
		node := nodeList.Items[i]
		nodes = append(nodes, &node)
	}

	rc := RcByNameContainer("test-rc-evict-annotation", int32(15), map[string]string{"test": "annotation"}, nil)
	rc.Spec.Template.Annotations = map[string]string{"descheduler.alpha.kubernetes.io/evict": "true"}
	rc.Spec.Template.Spec.Volumes = []v1.Volume{
		{
			Name: "sample",
			VolumeSource: v1.VolumeSource{
				EmptyDir: &v1.EmptyDirVolumeSource{
					SizeLimit: resource.NewQuantity(int64(10), resource.BinarySI)},
			},
		},
	}

	if _, err := clientSet.CoreV1().ReplicationControllers(rc.Namespace).Create(ctx, rc, metav1.CreateOptions{}); err != nil {
		t.Errorf("Error creating deployment %v", err)
	}

	evictPods(ctx, t, clientSet, nodeInformer, nodes, rc)
	deleteRC(ctx, t, clientSet, rc)
}

func TestDeschedulingInterval(t *testing.T) {
	ctx := context.Background()
	clientSet, err := client.CreateClient(os.Getenv("KUBECONFIG"))
	if err != nil {
		t.Errorf("Error during client creation with %v", err)
	}

	// By default, the DeschedulingInterval param should be set to 0, meaning Descheduler only runs once then exits
	s := options.NewDeschedulerServer()
	s.Client = clientSet

	deschedulerPolicy := &api.DeschedulerPolicy{}

	c := make(chan bool)
	go func() {
		evictionPolicyGroupVersion, err := eutils.SupportEviction(s.Client)
		if err != nil || len(evictionPolicyGroupVersion) == 0 {
			t.Errorf("Error when checking support for eviction: %v", err)
		}

		stopChannel := make(chan struct{})
		if err := descheduler.RunDeschedulerStrategies(ctx, s, deschedulerPolicy, evictionPolicyGroupVersion, stopChannel); err != nil {
			t.Errorf("Error running descheduler strategies: %+v", err)
		}
		c <- true
	}()

	select {
	case <-c:
		// successfully returned
	case <-time.After(3 * time.Minute):
		t.Errorf("descheduler.Run timed out even without descheduling-interval set")
	}
}

func deleteRC(ctx context.Context, t *testing.T, clientSet clientset.Interface, rc *v1.ReplicationController) {
	//set number of replicas to 0
	rcdeepcopy := rc.DeepCopy()
	rcdeepcopy.Spec.Replicas = func(i int32) *int32 { return &i }(0)
	if _, err := clientSet.CoreV1().ReplicationControllers(rcdeepcopy.Namespace).Update(ctx, rcdeepcopy, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("Error updating replica controller %v", err)
	}
	allPodsDeleted := false
	//wait 30 seconds until all pods are deleted
	for i := 0; i < 6; i++ {
		scale, _ := clientSet.CoreV1().ReplicationControllers(rc.Namespace).GetScale(ctx, rc.Name, metav1.GetOptions{})
		if scale.Spec.Replicas == 0 {
			allPodsDeleted = true
			break
		}
		time.Sleep(5 * time.Second)
	}

	if !allPodsDeleted {
		t.Errorf("Deleting of rc pods took too long")
	}

	if err := clientSet.CoreV1().ReplicationControllers(rc.Namespace).Delete(ctx, rc.Name, metav1.DeleteOptions{}); err != nil {
		t.Fatalf("Error deleting rc %v", err)
	}

	if err := wait.PollImmediate(time.Second, 10*time.Second, func() (bool, error) {
		_, err := clientSet.CoreV1().ReplicationControllers(rc.Namespace).Get(ctx, rc.Name, metav1.GetOptions{})
		if err != nil && strings.Contains(err.Error(), "not found") {
			return true, nil
		}
		return false, nil
	}); err != nil {
		t.Fatalf("Error deleting rc %v", err)
	}
}

func evictPods(ctx context.Context, t *testing.T, clientSet clientset.Interface, nodeInformer coreinformers.NodeInformer, nodeList []*v1.Node, rc *v1.ReplicationController) {
	var leastLoadedNode *v1.Node
	podsBefore := math.MaxInt16
	evictionPolicyGroupVersion, err := eutils.SupportEviction(clientSet)
	if err != nil || len(evictionPolicyGroupVersion) == 0 {
		klog.Fatalf("%v", err)
	}
	podEvictor := evictions.NewPodEvictor(
		clientSet,
		evictionPolicyGroupVersion,
		false,
		0,
		nodeList,
		true,
	)
	for _, node := range nodeList {
		// Skip the Master Node
		if _, exist := node.Labels["node-role.kubernetes.io/master"]; exist {
			continue
		}
		// List all the pods on the current Node
		podsOnANode, err := podutil.ListPodsOnANode(ctx, clientSet, node, podEvictor.IsEvictable)
		if err != nil {
			t.Errorf("Error listing pods on a node %v", err)
		}
		// Update leastLoadedNode if necessary
		if tmpLoads := len(podsOnANode); tmpLoads < podsBefore {
			leastLoadedNode = node
			podsBefore = tmpLoads
		}
	}
	t.Log("Eviction of pods starting")
	startEndToEndForLowNodeUtilization(ctx, clientSet, nodeInformer, podEvictor)
	podsOnleastUtilizedNode, err := podutil.ListPodsOnANode(ctx, clientSet, leastLoadedNode, podEvictor.IsEvictable)
	if err != nil {
		t.Errorf("Error listing pods on a node %v", err)
	}
	podsAfter := len(podsOnleastUtilizedNode)
	if podsBefore > podsAfter {
		t.Fatalf("We should have see more pods on this node as per kubeadm's way of installing %v, %v", podsBefore, podsAfter)
	}
}
