package descheduler

import (
	"context"
	"fmt"
	v1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1"
	//policy "k8s.io/api/policy/v1beta1"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/descheduler/cmd/descheduler/app/options"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/test"
)

func TestTaintsUpdated(t *testing.T) {
	ctx := context.Background()
	n1 := test.BuildTestNode("n1", 2000, 3000, 10, nil)
	n2 := test.BuildTestNode("n2", 2000, 3000, 10, nil)

	p1 := test.BuildTestPod(fmt.Sprintf("pod_1_%s", n1.Name), 200, 0, n1.Name, nil)
	p1.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
		{},
	}

	client := fakeclientset.NewSimpleClientset(n1, n2, p1)
	eventClient := fakeclientset.NewSimpleClientset(n1, n2, p1)
	dp := &api.DeschedulerPolicy{
		Strategies: api.StrategyList{
			"RemovePodsViolatingNodeTaints": api.DeschedulerStrategy{
				Enabled: true,
			},
		},
	}

	rs, err := options.NewDeschedulerServer()
	if err != nil {
		t.Fatalf("Unable to initialize server: %v", err)
	}
	rs.Client = client
	rs.EventClient = eventClient
	rs.DeschedulingInterval = 100 * time.Millisecond
	errChan := make(chan error, 1)
	defer close(errChan)
	go func() {
		err := RunDeschedulerStrategies(ctx, rs, dp, "v1")
		errChan <- err
	}()
	select {
	case err := <-errChan:
		if err != nil {
			t.Fatalf("Unable to run descheduler strategies: %v", err)
		}
	case <-time.After(1 * time.Second):
		// Wait for few cycles and then verify the only pod still exists
	}

	pods, err := client.CoreV1().Pods(p1.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Errorf("Unable to list pods: %v", err)
	}
	if len(pods.Items) < 1 {
		t.Errorf("The pod was evicted before a node was tained")
	}

	n1WithTaint := n1.DeepCopy()
	n1WithTaint.Spec.Taints = []v1.Taint{
		{
			Key:    "key",
			Value:  "value",
			Effect: v1.TaintEffectNoSchedule,
		},
	}

	if _, err := client.CoreV1().Nodes().Update(ctx, n1WithTaint, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("Unable to update node: %v\n", err)
	}

	if err := wait.PollImmediate(100*time.Millisecond, time.Second, func() (bool, error) {
		// Get over evicted pod result in panic
		//pods, err := client.CoreV1().Pods(p1.Namespace).Get(p1.Name, metav1.GetOptions{})
		// List is better, it does not panic.
		// Though once the pod is evicted, List starts to error with "can't assign or convert v1beta1.Eviction into v1.Pod"
		pods, err := client.CoreV1().Pods(p1.Namespace).List(ctx, metav1.ListOptions{})
		if err == nil {
			if len(pods.Items) > 0 {
				return false, nil
			}
			return true, nil
		}
		if strings.Contains(err.Error(), "can't assign or convert v1beta1.Eviction into v1.Pod") {
			return true, nil
		}

		return false, nil
	}); err != nil {
		t.Fatalf("Unable to evict pod, node taint did not get propagated to descheduler strategies %v\n", err)
	}
}

func TestDuplicate(t *testing.T) {
	ctx := context.Background()
	node1 := test.BuildTestNode("n1", 2000, 3000, 10, nil)
	node2 := test.BuildTestNode("n2", 2000, 3000, 10, nil)

	p1 := test.BuildTestPod("p1", 100, 0, node1.Name, nil)
	p1.Namespace = "dev"
	p2 := test.BuildTestPod("p2", 100, 0, node1.Name, nil)
	p2.Namespace = "dev"
	p3 := test.BuildTestPod("p3", 100, 0, node1.Name, nil)
	p3.Namespace = "dev"

	ownerRef1 := test.GetReplicaSetOwnerRefList()
	p1.ObjectMeta.OwnerReferences = ownerRef1
	p2.ObjectMeta.OwnerReferences = ownerRef1
	p3.ObjectMeta.OwnerReferences = ownerRef1

	client := fakeclientset.NewSimpleClientset(node1, node2, p1, p2, p3)
	eventClient := fakeclientset.NewSimpleClientset(node1, node2, p1, p2, p3)
	dp := &api.DeschedulerPolicy{
		Strategies: api.StrategyList{
			"RemoveDuplicates": api.DeschedulerStrategy{
				Enabled: true,
			},
		},
	}

	rs, err := options.NewDeschedulerServer()
	if err != nil {
		t.Fatalf("Unable to initialize server: %v", err)
	}
	rs.Client = client
	rs.EventClient = eventClient
	rs.DeschedulingInterval = 100 * time.Millisecond
	errChan := make(chan error, 1)
	defer close(errChan)
	go func() {
		err := RunDeschedulerStrategies(ctx, rs, dp, "v1")
		errChan <- err
	}()
	select {
	case err := <-errChan:
		if err != nil {
			t.Fatalf("Unable to run descheduler strategies: %v", err)
		}
	case <-time.After(1 * time.Second):
		// Wait for few cycles and then verify the only pod still exists
	}

	pods, err := client.CoreV1().Pods(p1.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Errorf("Unable to list pods: %v", err)
	}

	if len(pods.Items) != 3 {
		t.Errorf("Pods number should be 3 before evict")
	}

	if err := wait.PollImmediate(100*time.Millisecond, 5*time.Second, func() (bool, error) {
		// Get over evicted pod result in panic
		//pods, err := client.CoreV1().Pods(p1.Namespace).Get(p1.Name, metav1.GetOptions{})
		// List is better, it does not panic.
		// Though once the pod is evicted, List starts to error with "can't assign or convert v1beta1.Eviction into v1.Pod"
		pods, err := client.CoreV1().Pods(p1.Namespace).List(ctx, metav1.ListOptions{})
		if err == nil {
			if len(pods.Items) > 2 {
				return false, nil
			}
			return true, nil
		}
		if strings.Contains(err.Error(), "can't assign or convert v1beta1.Eviction into v1.Pod") {
			return true, nil
		}

		return false, nil
	}); err != nil {
		t.Fatalf("Unable to evict pod, node taint did not get propagated to descheduler strategies %v\n", err)
	}
}

func TestRootCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	n1 := test.BuildTestNode("n1", 2000, 3000, 10, nil)
	n2 := test.BuildTestNode("n2", 2000, 3000, 10, nil)
	client := fakeclientset.NewSimpleClientset(n1, n2)
	eventClient := fakeclientset.NewSimpleClientset(n1, n2)
	dp := &api.DeschedulerPolicy{
		Strategies: api.StrategyList{}, // no strategies needed for this test
	}

	rs, err := options.NewDeschedulerServer()
	if err != nil {
		t.Fatalf("Unable to initialize server: %v", err)
	}
	rs.Client = client
	rs.EventClient = eventClient
	rs.DeschedulingInterval = 100 * time.Millisecond
	errChan := make(chan error, 1)
	defer close(errChan)

	go func() {
		err := RunDeschedulerStrategies(ctx, rs, dp, "v1beta1")
		errChan <- err
	}()
	cancel()
	select {
	case err := <-errChan:
		if err != nil {
			t.Fatalf("Unable to run descheduler strategies: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Root ctx should have canceled immediately")
	}
}

func TestRootCancelWithNoInterval(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	n1 := test.BuildTestNode("n1", 2000, 3000, 10, nil)
	n2 := test.BuildTestNode("n2", 2000, 3000, 10, nil)
	client := fakeclientset.NewSimpleClientset(n1, n2)
	eventClient := fakeclientset.NewSimpleClientset(n1, n2)
	dp := &api.DeschedulerPolicy{
		Strategies: api.StrategyList{}, // no strategies needed for this test
	}

	rs, err := options.NewDeschedulerServer()
	if err != nil {
		t.Fatalf("Unable to initialize server: %v", err)
	}
	rs.Client = client
	rs.EventClient = eventClient
	rs.DeschedulingInterval = 0
	errChan := make(chan error, 1)
	defer close(errChan)

	go func() {
		err := RunDeschedulerStrategies(ctx, rs, dp, "v1beta1")
		errChan <- err
	}()
	cancel()
	select {
	case err := <-errChan:
		if err != nil {
			t.Fatalf("Unable to run descheduler strategies: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Root ctx should have canceled immediately")
	}
}

func TestNewSimpleClientset(t *testing.T) {
	client := fakeclientset.NewSimpleClientset()
	client.CoreV1().Pods("default").Create(context.Background(), &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-1",
			Namespace: "default",
		},
	}, metav1.CreateOptions{})
	client.CoreV1().Pods("default").Create(context.Background(), &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-2",
			Namespace: "default",
		},
	}, metav1.CreateOptions{})
	err := client.CoreV1().Pods("default").EvictV1(context.Background(), &policy.Eviction{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod-2",
		},
	})

	if err != nil {
		t.Errorf("TestNewSimpleClientset() res = %v", err.Error())
	}

	pods, err := client.CoreV1().Pods("default").List(context.Background(), metav1.ListOptions{})
	// err: item[0]: can't assign or convert v1beta1.Eviction into v1.Pod
	if err != nil {
		t.Errorf("TestNewSimpleClientset() res = %v", err.Error())
	} else {
		t.Log(len(pods.Items))
		t.Logf("TestNewSimpleClientset() res = %v", pods)
	}

	err = client.PolicyV1().Evictions("default").Evict(context.Background(), &policy.Eviction{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod-1",
		},
	})
	if err != nil {
		t.Fatalf(err.Error())
	}
	pods, err = client.CoreV1().Pods("default").List(context.Background(), metav1.ListOptions{})
	// err: item[0]: can't assign or convert v1beta1.Eviction into v1.Pod
	if err != nil {
		t.Errorf("TestNewSimpleClientset() res = %v", err.Error())
	} else {
		t.Log(len(pods.Items))
		t.Logf("TestNewSimpleClientset() res = %v", pods)
	}
}
