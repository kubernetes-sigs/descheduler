package runtime

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/pkg/api/v1alpha2"
	"sigs.k8s.io/descheduler/pkg/framework"
	"sigs.k8s.io/descheduler/pkg/framework/profile"
	"sigs.k8s.io/descheduler/pkg/framework/registry"
)

// Option for the handleImpl.
type Option func(*frameworkImplOpts)

type frameworkImplOpts struct {
	clientSet             clientset.Interface
	sharedInformerFactory informers.SharedInformerFactory
	podEvictor            framework.Evictable
	registry              registry.Registry
}

// WithClientSet sets clientSet for the scheduling Framework.
func WithClientSet(clientSet clientset.Interface) Option {
	return func(o *frameworkImplOpts) {
		o.clientSet = clientSet
	}
}

func WithSharedInformerFactory(sharedInformerFactory informers.SharedInformerFactory) Option {
	return func(o *frameworkImplOpts) {
		o.sharedInformerFactory = sharedInformerFactory
	}
}

func WithPodEvictor(podEvictor framework.Evictable) Option {
	return func(o *frameworkImplOpts) {
		o.podEvictor = podEvictor
	}
}

func WithRegistry(registry registry.Registry) Option {
	return func(o *frameworkImplOpts) {
		o.registry = registry
	}
}

type Framework struct {
	profiles []*profile.Profile

	podEvictor framework.Evictable
	evicted    uint
}

func (f *Framework) Evict(ctx context.Context, pod *v1.Pod) bool {
	return f.podEvictor.Evict(ctx, pod)
}

func NewFramework(config v1alpha2.DeschedulerConfiguration, opts ...Option) (*Framework, error) {
	fOpts := &frameworkImplOpts{}
	for _, optFnc := range opts {
		optFnc(fOpts)
	}

	frmwrk := &Framework{
		podEvictor: fOpts.podEvictor,
	}

	for _, profileCfg := range config.Profiles {
		profImpl, err := profile.NewProfile(
			profileCfg,
			fOpts.registry,
			profile.WithClientSet(fOpts.clientSet),
			profile.WithPodEvictor(frmwrk),
			profile.WithSharedInformerFactory(fOpts.sharedInformerFactory),
		)
		if err != nil {
			return nil, fmt.Errorf("unable to create profile for %v: %v", profileCfg.Name, err)
		}
		frmwrk.profiles = append(frmwrk.profiles, profImpl)
	}

	return frmwrk, nil
}

func (f *Framework) RunDeschedulePlugins(ctx context.Context, nodes []*v1.Node) *framework.Status {
	errs := []error{}
	f.evicted = 0
	for _, profile := range f.profiles {
		status := profile.RunDeschedulePlugins(ctx, nodes)
		if status != nil && status.Err != nil {
			errs = append(errs, fmt.Errorf("profile=%v, %v", profile, status.Err))
		}
	}
	klog.V(1).InfoS("Total number of pods evicted by Deschedule extension point", "evictedPods", f.evicted)
	aggrErr := errors.NewAggregate(errs)
	if aggrErr == nil {
		return &framework.Status{}
	}

	return &framework.Status{
		Err: fmt.Errorf("%v", aggrErr.Error()),
	}
}

func (f *Framework) RunBalancePlugins(ctx context.Context, nodes []*v1.Node) *framework.Status {
	errs := []error{}
	f.evicted = 0
	for _, profile := range f.profiles {
		status := profile.RunBalancePlugins(ctx, nodes)
		if status != nil && status.Err != nil {
			errs = append(errs, fmt.Errorf("profile=%v, %v", profile, status.Err))
		}
	}
	klog.V(1).InfoS("Total number of pods evicted by Balance extension point", "evictedPods", f.evicted)
	aggrErr := errors.NewAggregate(errs)
	if aggrErr == nil {
		return &framework.Status{}
	}

	return &framework.Status{
		Err: fmt.Errorf("%v", aggrErr.Error()),
	}
}
