package framework

import (
	"context"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/descheduler/pkg/api/v1alpha2"
	"sigs.k8s.io/descheduler/pkg/framework"
	"sigs.k8s.io/descheduler/pkg/framework/registry"
	"sigs.k8s.io/descheduler/test"
)

func TestNewProfile(t *testing.T) {
	var seconds uint = 10
	cfg := v1alpha2.Profile{
		Name: "test-profile",
		PluginConfig: []v1alpha2.PluginConfig{
			v1alpha2.PluginConfig{
				Name: "PodLifeTime",
				Args: &framework.PodLifeTimeArgs{
					MaxPodLifeTimeSeconds: &seconds,
				},
			},
			v1alpha2.PluginConfig{
				Name: "RemoveFailedPods",
				Args: &framework.RemoveFailedPodsArgs{},
			},
			v1alpha2.PluginConfig{
				Name: "DefaultEvictor",
				Args: &framework.DefaultEvictorArgs{},
			},
		},
		Plugins: v1alpha2.Plugins{
			Deschedule: v1alpha2.Plugin{
				Enabled: []string{
					"PodLifeTime",
					// "RemoveFailedPods",
				},
			},
			Sort: v1alpha2.Plugin{
				Enabled: []string{
					"DefaultEvictor",
				},
			},
			Evict: v1alpha2.Plugin{
				Enabled: []string{
					"DefaultEvictor",
				},
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// TODO(jchaloup): implement plugin args defaulting
	reg := registry.NewRegistry()
	node1 := test.BuildTestNode("n1", 2000, 3000, 10, nil)
	olderPodCreationTime := metav1.NewTime(time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC))
	p1 := test.BuildTestPod("p1", 100, 0, node1.Name, nil)
	p1.ObjectMeta.CreationTimestamp = olderPodCreationTime
	fakeClient := fake.NewSimpleClientset(node1, p1)

	sharedInformerFactory := informers.NewSharedInformerFactory(fakeClient, 0)

	podEvictor := framework.NewPodEvictor(
		fakeClient,
		policyv1.SchemeGroupVersion.String(),
		false,
		nil,
		nil,
		false,
	)

	h, err := newProfile(cfg, reg, WithClientSet(fakeClient), WithSharedInformerFactory(sharedInformerFactory), WithPodEvictor(podEvictor))
	if err != nil {
		t.Fatal(err)
	}

	sharedInformerFactory.Start(ctx.Done())
	sharedInformerFactory.WaitForCacheSync(ctx.Done())

	h.runDeschedulePlugins(ctx, []*v1.Node{node1})
	h.runBalancePlugins(ctx, []*v1.Node{node1})
}
