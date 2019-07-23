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
	"github.com/kubernetes-incubator/descheduler/cmd/descheduler/app/options"
	deschedulerapi "github.com/kubernetes-incubator/descheduler/pkg/api"
	"github.com/kubernetes-incubator/descheduler/pkg/descheduler/client"
	eutils "github.com/kubernetes-incubator/descheduler/pkg/descheduler/evictions/utils"
	nodeutil "github.com/kubernetes-incubator/descheduler/pkg/descheduler/node"
	podutil "github.com/kubernetes-incubator/descheduler/pkg/descheduler/pod"
	"github.com/kubernetes-incubator/descheduler/pkg/descheduler/strategies"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/pkg/api/testapi"
	"k8s.io/kubernetes/pkg/util/labels"
	"math"
	"testing"
	"time"
)

func makeAffinity() *v1.Affinity {
	return &v1.Affinity{
		PodAntiAffinity: &v1.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
				{
					LabelSelector: &metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "foo",
								Operator: metav1.LabelSelectorOpIn,
								Values:   []string{"bar"},
							},
						},
					},
					TopologyKey: "kubernetes.io/hostname",
				},
			},
		},
		NodeAffinity: &v1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
				NodeSelectorTerms: []v1.NodeSelectorTerm{
					{
						MatchExpressions: []v1.NodeSelectorRequirement{
							{
								Key:      "kubernetes.io/node-type",
								Operator: v1.NodeSelectorOpIn,
								Values:   []string{"local"},
							},
						},
					},
				},
			},
		},
	}
}

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
		Affinity: makeAffinity(),
	}
}

// RcByNameContainer returns a ReplicationControoler with specified name and container
func RcByNameContainer(name string, replicas int32, labels map[string]string, gracePeriod *int64) *v1.ReplicationController {

	zeroGracePeriod := int64(0)

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
			Selector: labels,
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
func startEndToEndForLowNodeUtilization(clientSet clientset.Interface) {
	var thresholds = make(deschedulerapi.ResourceThresholds)
	var targetThresholds = make(deschedulerapi.ResourceThresholds)
	thresholds[v1.ResourceMemory] = 20
	thresholds[v1.ResourcePods] = 20
	thresholds[v1.ResourceCPU] = 85
	targetThresholds[v1.ResourceMemory] = 20
	targetThresholds[v1.ResourcePods] = 20
	targetThresholds[v1.ResourceCPU] = 90
	// Run descheduler.
	nodeUtilizationThresholds := deschedulerapi.NodeResourceUtilizationThresholds{Thresholds: thresholds, TargetThresholds: targetThresholds}
	nodeUtilizationStrategyParams := deschedulerapi.StrategyParameters{NodeResourceUtilizationThresholds: nodeUtilizationThresholds}
	runStrategy(clientSet, strategies.LowNodeUtilization, nodeUtilizationStrategyParams)
}

func E2eTestForRemovePodsViolatingInterPodAntiAffinity(clientSet clientset.Interface) {
	runStrategy(clientSet, strategies.RemovePodsViolatingInterPodAntiAffinity, deschedulerapi.StrategyParameters{})
}

// Run selected strategy base on the func and param
func runStrategy(clientSet clientset.Interface, strategyFunc func(ds *options.DeschedulerServer, strategy deschedulerapi.DeschedulerStrategy, policyGroupVersion string, nodes []*v1.Node, nodePodCount strategies.NodePodEvictedCount), strategyParam deschedulerapi.StrategyParameters) strategies.NodePodEvictedCount {
	evictionPolicyGroupVersion, err := eutils.SupportEviction(clientSet)
	if err != nil || len(evictionPolicyGroupVersion) == 0 {
		glog.Fatalf("%v", err)
	}
	stopChannel := make(chan struct{})
	nodes, err := nodeutil.ReadyNodes(clientSet, "", stopChannel)
	if err != nil {
		glog.Fatalf("%v", err)
	}
	ds := &options.DeschedulerServer{Client: clientSet}
	strategy := deschedulerapi.DeschedulerStrategy{Enabled: true, Params: strategyParam}
	nodePodCount := strategies.InitializeNodePodCount(nodes)
	strategyFunc(ds, strategy, evictionPolicyGroupVersion, nodes, nodePodCount)
	time.Sleep(15 * time.Second)
	return nodePodCount
}

func E2eForViolatingNodeAffinity(clientSet clientset.Interface) {
	nodeAffinityStrategyParams := deschedulerapi.StrategyParameters{NodeAffinityType: []string{"requiredDuringSchedulingIgnoredDuringExecution"}}
	runStrategy(clientSet, strategies.RemovePodsViolatingNodeAffinity, nodeAffinityStrategyParams)
}

func getLeastUtilizedNode(clientSet clientset.Interface) (*v1.Node, int) {
	var leastLoadedNode v1.Node
	nodeList, err := clientSet.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		glog.Errorf("Error listing node with %v", err)
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
			glog.Errorf("Error listing pods on a node %v", err)
		}
		// Update leastLoadedNode if necessary
		if tmpLoads := len(podsOnANode); tmpLoads != 0 && tmpLoads < podsBefore {
			leastLoadedNode = nodeList.Items[i]
			podsBefore = tmpLoads
		}
	}
	return &leastLoadedNode, podsBefore
}

func TestE2E(t *testing.T) {
	// If we have reached here, it means cluster would have been already setup and the kubeconfig file should
	// be in /tmp directory as admin.conf.
	clientSet, err := client.CreateClient("/tmp/admin.conf")
	if err != nil {
		t.Errorf("Error during client creation with %v", err)
	}
	nodeList, err := clientSet.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		t.Errorf("Error listing node with %v", err)
	}
	// Label nodes for node affinity test
	for _, node := range nodeList.Items {
		node.Labels["kubernetes.io/node-type"] = "local"
		if _, err = clientSet.CoreV1().Nodes().Update(&node); err != nil {
			t.Errorf("Error assiging labels to node for node affinity test %v", err)
		}
	}
	// Assumption: We would have 3 node cluster by now. Kubeadm brings all the master components onto master node.
	// So, the last node would have least utilization.
	ns, rcName := "default", "test-rc"
	rc := RcByNameContainer(rcName, int32(15), map[string]string{"test": "app"}, nil)
	_, err = clientSet.CoreV1().ReplicationControllers("default").Create(rc)
	if err != nil {
		t.Errorf("Error creating deployment %v", err)
	} else {
		time.Sleep(15 * time.Second)
	}
	leastLoadedNode, podsBefore := getLeastUtilizedNode(clientSet)
	t.Log("Evicting pods for low node utilization test")
	startEndToEndForLowNodeUtilization(clientSet)
	podsOnleastUtilizedNode, err := podutil.ListEvictablePodsOnNode(clientSet, leastLoadedNode, true)
	if err != nil {
		t.Errorf("Error listing pods on a node %v", err)
	}
	podsAfter := len(podsOnleastUtilizedNode)
	if podsBefore > podsAfter {
		t.Errorf("Failed: E2e test for low node utilization. We should have see more or equal pods on %v node %v, %v", leastLoadedNode.Name, podsBefore, podsAfter)
	} else {
		t.Logf("The lease utilized node had %v pods before descheduling and %v pods after", podsBefore, podsAfter)
	}
	// Test for evicting pods based on Inter-Pod Anti-Affinity violations
	podsBefore = podsAfter
	podsOnleastUtilizedNode, err = podutil.ListEvictablePodsOnNode(clientSet, leastLoadedNode, true)
	if err != nil {
		t.Errorf("Error listing pods on a node %v", err)
	}
	podsLabeled := 1
	// change pod labels so that it matched pod anti-affinity
	for i, pod := range podsOnleastUtilizedNode {
		if i < podsLabeled {
			labels.AddLabel(pod.Labels, "foo", "bar")
			if _, err = clientSet.CoreV1().Pods(ns).Update(pod); err != nil {
				t.Errorf("Error updating pods: %v", err)
			}
		}
	}
	t.Log("Evicting pods for Inter-Pod Anti-Affinity violation test")
	E2eTestForRemovePodsViolatingInterPodAntiAffinity(clientSet)
	podsOnleastUtilizedNode, err = podutil.ListEvictablePodsOnNode(clientSet, leastLoadedNode, true)
	if err != nil {
		t.Errorf("Error listing pods on a node %v", err)
	}
	if podsAfter = len(podsOnleastUtilizedNode); podsAfter != podsLabeled {
		t.Errorf("Failed: E2e test for Inter-Pod Anti-Affinity. We expect to eviect: %v pods, instead we evicted: %v pods.", podsBefore-podsLabeled, podsBefore-podsAfter)
	} else {
		t.Logf("Inter-pod affinity evicted all but %v pods.", podsAfter)
	}
	t.Log("Evicting pods for node affinity violation test.")
	leastLoadedNode, _ = getLeastUtilizedNode(clientSet)
	// Change one node label and unmatch node affinity
	leastLoadedNode.Labels["kubernetes.io/node-type"] = "remote"
	if _, err = clientSet.CoreV1().Nodes().Update(leastLoadedNode); err != nil {
		t.Errorf("Error reassigning labels to node during node affinity test %v", err)
	}
	E2eForViolatingNodeAffinity(clientSet)
	podsOnleastUtilizedNode, err = podutil.ListEvictablePodsOnNode(clientSet, leastLoadedNode, true)
	if err != nil {
		t.Errorf("Error listing pods on a node %v", err)
	}
	if len(podsOnleastUtilizedNode) != int(0) {
		t.Errorf("Failed: E2e test for Node affinity. Pods are not evicted or statyed away from this node that is violating node affinity.")
	} else {
		t.Logf("Node affinity violating pods are all evicted from %v node.", leastLoadedNode.Name)
	}
	// Delete test replication controller and all dependent pods after e2e tests
	policy := metav1.DeletePropagationBackground
	if err = clientSet.CoreV1().ReplicationControllers(ns).Delete(rcName, &metav1.DeleteOptions{PropagationPolicy: &policy}); err != nil {
		t.Errorf("Error shutting down test Replication Controller %v", err)
	}
}
