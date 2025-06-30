package nodeutilization

import (
	"context"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/framework/plugins/defaultevictor"
	frameworktesting "sigs.k8s.io/descheduler/pkg/framework/testing"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
	"sigs.k8s.io/descheduler/test"
)

// TestHighNodeUtilizationPodNotEvictedIfNoFit validates the regression fix that avoids
// evicting pods when they cannot be scheduled onto any of the potential destination
// nodes. Without the PodFitsAnyNode check, the pod below would have been evicted
// even though it cannot run on the only under-utilised node.
func TestHighNodeUtilizationPodNotEvictedIfNoFit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const nodeSelectorKey = "datacenter"

	// Source node (UNDER-utilised) – labelled "east" so the pod fits only here.
	// We keep capacity high but give the pod only a tiny CPU request so that
	// utilisation stays below the threshold and the node is considered for
	// eviction.
	n1 := test.BuildTestNode("n1", 4000, 3000, 10, func(node *v1.Node) {
		node.Labels[nodeSelectorKey] = "east"
	})
	// Potential destination node – labelled "west". We later place a heavy pod on
	// it so that it is *not* classified as under-utilised, making it eligible to
	// receive evicted pods.
	n2 := test.BuildTestNode("n2", 4000, 3000, 10, func(node *v1.Node) {
		node.Labels[nodeSelectorKey] = "west"
	})

	// Extra under-utilised node that is still schedulable. This helps exercise the
	// loop over multiple source nodes.
	n3 := test.BuildTestNode("n3", 4000, 3000, 10, func(node *v1.Node) {
		node.Labels[nodeSelectorKey] = "east"
	})

	// Single pod with a SMALL CPU request to keep node utilisation low (< threshold).
	// The pod is unschedulable on n2 due to the node selector, triggering the
	// PodFitsAnyNode guard.
	p1 := test.BuildTestPod("p1", 100, 0, n1.Name, func(pod *v1.Pod) {
		test.SetRSOwnerRef(pod)
		pod.Spec.NodeSelector = map[string]string{nodeSelectorKey: "east"}
	})

	// Add a workload pod to n2 so that it is *not* considered underutilised.
	// This ensures n2 becomes a potential destination node.
	loadPod := test.BuildTestPod("load", 3000, 0, n2.Name, test.SetRSOwnerRef)

	objs := []runtime.Object{n1, n2, n3, p1, loadPod}
	fakeClient := fake.NewSimpleClientset(objs...)

	handle, podEvictor, err := frameworktesting.InitFrameworkHandle(
		ctx, fakeClient, nil, defaultevictor.DefaultEvictorArgs{NodeFit: true}, nil,
	)
	if err != nil {
		t.Fatalf("Unable to initialise framework handle: %v", err)
	}

	plugin, err := NewHighNodeUtilization(ctx, &HighNodeUtilizationArgs{
		Thresholds: api.ResourceThresholds{v1.ResourceCPU: 50}, // 50% CPU threshold – n1 will be under it
	}, handle)
	if err != nil {
		t.Fatalf("Unable to initialise HighNodeUtilization plugin: %v", err)
	}

	plugin.(frameworktypes.BalancePlugin).Balance(ctx, []*v1.Node{n1, n2, n3})

	if evicted := podEvictor.TotalEvicted(); evicted != 0 {
		t.Errorf("expected no pods to be evicted, but got %d", evicted)
	}
}
