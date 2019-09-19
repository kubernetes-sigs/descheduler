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
	"github.com/golang/glog"
	"math"
	"testing"
	"time"

	"sigs.k8s.io/descheduler/cmd/descheduler/app/options"
	deschedulerapi "sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/client"
	eutils "sigs.k8s.io/descheduler/pkg/descheduler/evictions/utils"
	nodeutil "sigs.k8s.io/descheduler/pkg/descheduler/node"
	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	"sigs.k8s.io/descheduler/pkg/descheduler/strategies"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/pkg/api/testapi"
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
			APIVersion: testapi.Groups[v1.GroupName].GroupVersion().String(),
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
func startEndToEndForLowNodeUtilization(clientset clientset.Interface) {
	var thresholds = make(deschedulerapi.ResourceThresholds)
	var targetThresholds = make(deschedulerapi.ResourceThresholds)
	thresholds[v1.ResourceMemory] = 20
	thresholds[v1.ResourcePods] = 20
	thresholds[v1.ResourceCPU] = 85
	targetThresholds[v1.ResourceMemory] = 20
	targetThresholds[v1.ResourcePods] = 20
	targetThresholds[v1.ResourceCPU] = 90
	// Run descheduler.
	evictionPolicyGroupVersion, err := eutils.SupportEviction(clientset)
	if err != nil || len(evictionPolicyGroupVersion) == 0 {
		glog.Fatalf("%v", err)
	}
	stopChannel := make(chan struct{})
	nodes, err := nodeutil.ReadyNodes(clientset, "", stopChannel)
	if err != nil {
		glog.Fatalf("%v", err)
	}
	nodeUtilizationThresholds := deschedulerapi.NodeResourceUtilizationThresholds{Thresholds: thresholds, TargetThresholds: targetThresholds}
	nodeUtilizationStrategyParams := deschedulerapi.StrategyParameters{NodeResourceUtilizationThresholds: nodeUtilizationThresholds}
	lowNodeUtilizationStrategy := deschedulerapi.DeschedulerStrategy{Enabled: true, Params: nodeUtilizationStrategyParams}
	ds := &options.DeschedulerServer{Client: clientset}
	nodePodCount := strategies.InitializeNodePodCount(nodes)
	strategies.LowNodeUtilization(ds, lowNodeUtilizationStrategy, evictionPolicyGroupVersion, nodes, nodePodCount)
	time.Sleep(10 * time.Second)
}

func TestE2E(t *testing.T) {
	// If we have reached here, it means cluster would have been already setup and the kubeconfig file should
	// be in /tmp directory as admin.conf.
	var leastLoadedNode v1.Node
	clientSet, err := client.CreateClient("/tmp/admin.conf")
	if err != nil {
		t.Errorf("Error during client creation with %v", err)
	}
	nodeList, err := clientSet.Core().Nodes().List(metav1.ListOptions{})
	if err != nil {
		t.Errorf("Error listing node with %v", err)
	}
	// Assumption: We would have 3 node cluster by now. Kubeadm brings all the master components onto master node.
	// So, the last node would have least utilization.
	rc := RcByNameContainer("test-rc", int32(15), map[string]string{"test": "app"}, nil)
	_, err = clientSet.CoreV1().ReplicationControllers("default").Create(rc)
	if err != nil {
		t.Errorf("Error creating deployment %v", err)
	}
	podsBefore := math.MaxInt16
	for i := range nodeList.Items {
		// Skip the Master Node
		if _, exist := nodeList.Items[i].Labels["node-role.kubernetes.io/master"]; exist {
			continue
		}
		// List all the pods on the current Node
		podsOnANode, err := podutil.ListEvictablePodsOnNode(clientSet, &nodeList.Items[i], true)
		if err != nil {
			t.Errorf("Error listing pods on a node %v", err)
		}
		// Update leastLoadedNode if necessary
		if tmpLoads := len(podsOnANode); tmpLoads < podsBefore {
			leastLoadedNode = nodeList.Items[i]
			podsBefore = tmpLoads
		}
	}
	t.Log("Eviction of pods starting")
	startEndToEndForLowNodeUtilization(clientSet)
	podsOnleastUtilizedNode, err := podutil.ListEvictablePodsOnNode(clientSet, &leastLoadedNode, true)
	if err != nil {
		t.Errorf("Error listing pods on a node %v", err)
	}
	podsAfter := len(podsOnleastUtilizedNode)
	if podsBefore > podsAfter {
		t.Fatalf("We should have see more pods on this node as per kubeadm's way of installing %v, %v", podsBefore, podsAfter)
	}
}
