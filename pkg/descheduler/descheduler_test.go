package descheduler

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/descheduler/cmd/descheduler/app/options"
	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/test"
)

func TestTaintsUpdated(t *testing.T) {
	n1 := test.BuildTestNode("n1", 2000, 3000, 10)
	n2 := test.BuildTestNode("n2", 2000, 3000, 10)

	p1 := test.BuildTestPod(fmt.Sprintf("pod_1_%s", n1.Name), 200, 0, n1.Name)
	p1.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
		{},
	}

	client := fakeclientset.NewSimpleClientset(n1, n2, p1)
	dp := &api.DeschedulerPolicy{
		Strategies: api.StrategyList{
			"RemovePodsViolatingNodeTaints": api.DeschedulerStrategy{
				Enabled: true,
			},
		},
	}

	stopChannel := make(chan struct{})
	defer close(stopChannel)

	rs := options.NewDeschedulerServer()
	rs.Client = client
	rs.DeschedulingInterval = 100 * time.Millisecond
	go func() {
		err := RunDeschedulerStrategies(rs, dp, "v1beta1", stopChannel)
		if err != nil {
			t.Fatalf("Unable to run descheduler strategies: %v", err)
		}
	}()

	// Wait for few cycles and then verify the only pod still exists
	time.Sleep(300 * time.Millisecond)
	pods, err := client.CoreV1().Pods(p1.Namespace).List(metav1.ListOptions{})
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

	if _, err := client.CoreV1().Nodes().Update(n1WithTaint); err != nil {
		t.Fatalf("Unable to update node: %v\n", err)
	}

	if err := wait.PollImmediate(100*time.Millisecond, time.Second, func() (bool, error) {
		// Get over evicted pod result in panic
		//pods, err := client.CoreV1().Pods(p1.Namespace).Get(p1.Name, metav1.GetOptions{})
		// List is better, it does not panic.
		// Though once the pod is evicted, List starts to error with "can't assign or convert v1beta1.Eviction into v1.Pod"
		pods, err := client.CoreV1().Pods(p1.Namespace).List(metav1.ListOptions{})
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
		t.Fatalf("Unable to evict pod, node taint did not get propagated to descheduler strategies")
	}
}
