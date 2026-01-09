/*
Copyright 2023 The Kubernetes Authors.
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

package plugin

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/descheduler/pkg/framework/pluginregistry"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
)

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// FakePluginArgs holds arguments used to configure FakePlugin plugin.
type FakePluginArgs struct {
	metav1.TypeMeta `json:",inline"`
}

func ValidateFakePluginArgs(obj runtime.Object) error {
	return nil
}

func SetDefaults_FakePluginArgs(obj runtime.Object) {}

var (
	_ frameworktypes.EvictorPlugin    = &FakePlugin{}
	_ frameworktypes.DeschedulePlugin = &FakePlugin{}
	_ frameworktypes.BalancePlugin    = &FakePlugin{}
	_ frameworktypes.EvictorPlugin    = &FakeFilterPlugin{}
	_ frameworktypes.DeschedulePlugin = &FakeDeschedulePlugin{}
	_ frameworktypes.BalancePlugin    = &FakeBalancePlugin{}
)

// FakePlugin is a configurable plugin used for testing
type FakePlugin struct {
	PluginName string

	// ReactionChain is the list of reactors that will be attempted for every
	// request in the order they are tried.
	ReactionChain []Reactor

	args   runtime.Object
	handle frameworktypes.Handle
}

func NewPluginFncFromFake(fp *FakePlugin) pluginregistry.PluginBuilder {
	return func(ctx context.Context, args runtime.Object, handle frameworktypes.Handle) (frameworktypes.Plugin, error) {
		fakePluginArgs, ok := args.(*FakePluginArgs)
		if !ok {
			return nil, fmt.Errorf("want args to be of type FakePluginArgs, got %T", args)
		}

		fp.handle = handle
		fp.args = fakePluginArgs

		return fp, nil
	}
}

func NewPluginFncFromFakeWithReactor(fp *FakePlugin, callback func(ActionImpl)) pluginregistry.PluginBuilder {
	return func(ctx context.Context, args runtime.Object, handle frameworktypes.Handle) (frameworktypes.Plugin, error) {
		fakePluginArgs, ok := args.(*FakePluginArgs)
		if !ok {
			return nil, fmt.Errorf("want args to be of type FakePluginArgs, got %T", args)
		}

		fp.handle = handle
		fp.args = fakePluginArgs

		callback(ActionImpl{handle: fp.handle})

		return fp, nil
	}
}

// New builds plugin from its arguments while passing a handle
func New(ctx context.Context, args runtime.Object, handle frameworktypes.Handle) (frameworktypes.Plugin, error) {
	fakePluginArgs, ok := args.(*FakePluginArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type FakePluginArgs, got %T", args)
	}

	ev := &FakePlugin{}
	ev.handle = handle
	ev.args = fakePluginArgs

	return ev, nil
}

func (c *FakePlugin) AddReactor(extensionPoint string, reaction ReactionFunc) {
	c.ReactionChain = append(c.ReactionChain, &SimpleReactor{ExtensionPoint: extensionPoint, Reaction: reaction})
}

// Name retrieves the plugin name
func (d *FakePlugin) Name() string {
	return d.PluginName
}

func (d *FakePlugin) PreEvictionFilter(pod *v1.Pod) bool {
	return true
}

func (d *FakePlugin) Filter(pod *v1.Pod) bool {
	return true
}

func (d *FakePlugin) handleAction(action Action) *frameworktypes.Status {
	actionCopy := action.DeepCopy()
	for _, reactor := range d.ReactionChain {
		if !reactor.Handles(actionCopy) {
			continue
		}
		handled, _, err := reactor.React(actionCopy)
		if !handled {
			continue
		}

		return &frameworktypes.Status{
			Err: err,
		}
	}
	return &frameworktypes.Status{
		Err: fmt.Errorf("unhandled %q action", action.GetExtensionPoint()),
	}
}

func (d *FakePlugin) Deschedule(ctx context.Context, nodes []*v1.Node) *frameworktypes.Status {
	return d.handleAction(&DescheduleActionImpl{
		ActionImpl: ActionImpl{
			handle:         d.handle,
			extensionPoint: string(frameworktypes.DescheduleExtensionPoint),
		},
		nodes: nodes,
	})
}

func (d *FakePlugin) Balance(ctx context.Context, nodes []*v1.Node) *frameworktypes.Status {
	return d.handleAction(&BalanceActionImpl{
		ActionImpl: ActionImpl{
			handle:         d.handle,
			extensionPoint: string(frameworktypes.BalanceExtensionPoint),
		},
		nodes: nodes,
	})
}

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// FakeDeschedulePluginArgs holds arguments used to configure FakeDeschedulePlugin plugin.
type FakeDeschedulePluginArgs struct {
	metav1.TypeMeta `json:",inline"`
}

// FakeDeschedulePlugin is a configurable plugin used for testing
type FakeDeschedulePlugin struct {
	PluginName string

	// ReactionChain is the list of reactors that will be attempted for every
	// request in the order they are tried.
	ReactionChain []Reactor

	args   runtime.Object
	handle frameworktypes.Handle
}

func NewFakeDeschedulePluginFncFromFake(fp *FakeDeschedulePlugin) pluginregistry.PluginBuilder {
	return func(ctx context.Context, args runtime.Object, handle frameworktypes.Handle) (frameworktypes.Plugin, error) {
		fakePluginArgs, ok := args.(*FakeDeschedulePluginArgs)
		if !ok {
			return nil, fmt.Errorf("want args to be of type FakeDeschedulePluginArgs, got %T", args)
		}

		fp.handle = handle
		fp.args = fakePluginArgs

		return fp, nil
	}
}

// New builds plugin from its arguments while passing a handle
func NewFakeDeschedule(args runtime.Object, handle frameworktypes.Handle) (frameworktypes.Plugin, error) {
	fakePluginArgs, ok := args.(*FakeDeschedulePluginArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type FakePluginArgs, got %T", args)
	}

	ev := &FakeDeschedulePlugin{}
	ev.handle = handle
	ev.args = fakePluginArgs

	return ev, nil
}

func (c *FakeDeschedulePlugin) AddReactor(extensionPoint string, reaction ReactionFunc) {
	c.ReactionChain = append(c.ReactionChain, &SimpleReactor{ExtensionPoint: extensionPoint, Reaction: reaction})
}

// Name retrieves the plugin name
func (d *FakeDeschedulePlugin) Name() string {
	return d.PluginName
}

func (d *FakeDeschedulePlugin) Deschedule(ctx context.Context, nodes []*v1.Node) *frameworktypes.Status {
	return d.handleAction(&DescheduleActionImpl{
		ActionImpl: ActionImpl{
			handle:         d.handle,
			extensionPoint: string(frameworktypes.DescheduleExtensionPoint),
		},
		nodes: nodes,
	})
}

func (d *FakeDeschedulePlugin) handleAction(action Action) *frameworktypes.Status {
	actionCopy := action.DeepCopy()
	for _, reactor := range d.ReactionChain {
		if !reactor.Handles(actionCopy) {
			continue
		}
		handled, _, err := reactor.React(actionCopy)
		if !handled {
			continue
		}

		return &frameworktypes.Status{
			Err: err,
		}
	}
	return &frameworktypes.Status{
		Err: fmt.Errorf("unhandled %q action", action.GetExtensionPoint()),
	}
}

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// FakeBalancePluginArgs holds arguments used to configure FakeBalancePlugin plugin.
type FakeBalancePluginArgs struct {
	metav1.TypeMeta `json:",inline"`
}

// FakeBalancePlugin is a configurable plugin used for testing
type FakeBalancePlugin struct {
	PluginName string

	// ReactionChain is the list of reactors that will be attempted for every
	// request in the order they are tried.
	ReactionChain []Reactor

	args   runtime.Object
	handle frameworktypes.Handle
}

func NewFakeBalancePluginFncFromFake(fp *FakeBalancePlugin) pluginregistry.PluginBuilder {
	return func(ctx context.Context, args runtime.Object, handle frameworktypes.Handle) (frameworktypes.Plugin, error) {
		fakePluginArgs, ok := args.(*FakeBalancePluginArgs)
		if !ok {
			return nil, fmt.Errorf("want args to be of type FakeBalancePluginArgs, got %T", args)
		}

		fp.handle = handle
		fp.args = fakePluginArgs

		return fp, nil
	}
}

// New builds plugin from its arguments while passing a handle
func NewFakeBalance(args runtime.Object, handle frameworktypes.Handle) (frameworktypes.Plugin, error) {
	fakePluginArgs, ok := args.(*FakeBalancePluginArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type FakePluginArgs, got %T", args)
	}

	ev := &FakeBalancePlugin{}
	ev.handle = handle
	ev.args = fakePluginArgs

	return ev, nil
}

func (c *FakeBalancePlugin) AddReactor(extensionPoint string, reaction ReactionFunc) {
	c.ReactionChain = append(c.ReactionChain, &SimpleReactor{ExtensionPoint: extensionPoint, Reaction: reaction})
}

// Name retrieves the plugin name
func (d *FakeBalancePlugin) Name() string {
	return d.PluginName
}

func (d *FakeBalancePlugin) Balance(ctx context.Context, nodes []*v1.Node) *frameworktypes.Status {
	return d.handleAction(&BalanceActionImpl{
		ActionImpl: ActionImpl{
			handle:         d.handle,
			extensionPoint: string(frameworktypes.BalanceExtensionPoint),
		},
		nodes: nodes,
	})
}

func (d *FakeBalancePlugin) handleAction(action Action) *frameworktypes.Status {
	actionCopy := action.DeepCopy()
	for _, reactor := range d.ReactionChain {
		if !reactor.Handles(actionCopy) {
			continue
		}
		handled, _, err := reactor.React(actionCopy)
		if !handled {
			continue
		}

		return &frameworktypes.Status{
			Err: err,
		}
	}
	return &frameworktypes.Status{
		Err: fmt.Errorf("unhandled %q action", action.GetExtensionPoint()),
	}
}

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// FakeFilterPluginArgs holds arguments used to configure FakeFilterPlugin plugin.
type FakeFilterPluginArgs struct {
	metav1.TypeMeta `json:",inline"`
}

// FakeFilterPlugin is a configurable plugin used for testing
type FakeFilterPlugin struct {
	PluginName string

	// ReactionChain is the list of reactors that will be attempted for every
	// request in the order they are tried.
	ReactionChain []Reactor

	args   runtime.Object
	handle frameworktypes.Handle
}

func NewFakeFilterPluginFncFromFake(fp *FakeFilterPlugin) pluginregistry.PluginBuilder {
	return func(ctx context.Context, args runtime.Object, handle frameworktypes.Handle) (frameworktypes.Plugin, error) {
		fakePluginArgs, ok := args.(*FakeFilterPluginArgs)
		if !ok {
			return nil, fmt.Errorf("want args to be of type FakeFilterPluginArgs, got %T", args)
		}

		fp.handle = handle
		fp.args = fakePluginArgs

		return fp, nil
	}
}

// New builds plugin from its arguments while passing a handle
func NewFakeFilter(args runtime.Object, handle frameworktypes.Handle) (frameworktypes.Plugin, error) {
	fakePluginArgs, ok := args.(*FakeFilterPluginArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type FakePluginArgs, got %T", args)
	}

	ev := &FakeFilterPlugin{}
	ev.handle = handle
	ev.args = fakePluginArgs

	return ev, nil
}

func (c *FakeFilterPlugin) AddReactor(extensionPoint string, reaction ReactionFunc) {
	c.ReactionChain = append(c.ReactionChain, &SimpleReactor{ExtensionPoint: extensionPoint, Reaction: reaction})
}

// Name retrieves the plugin name
func (d *FakeFilterPlugin) Name() string {
	return d.PluginName
}

func (d *FakeFilterPlugin) Filter(pod *v1.Pod) bool {
	return d.handleBoolAction(&FilterActionImpl{
		ActionImpl: ActionImpl{
			handle:         d.handle,
			extensionPoint: string(frameworktypes.FilterExtensionPoint),
		},
	})
}

func (d *FakeFilterPlugin) PreEvictionFilter(pod *v1.Pod) bool {
	return d.handleBoolAction(&PreEvictionFilterActionImpl{
		ActionImpl: ActionImpl{
			handle:         d.handle,
			extensionPoint: string(frameworktypes.PreEvictionFilterExtensionPoint),
		},
	})
}

func (d *FakeFilterPlugin) handleBoolAction(action Action) bool {
	actionCopy := action.DeepCopy()
	for _, reactor := range d.ReactionChain {
		if !reactor.Handles(actionCopy) {
			continue
		}
		handled, filter, _ := reactor.React(actionCopy)
		if !handled {
			continue
		}

		return filter
	}
	panic(fmt.Errorf("unhandled %q action", action.GetExtensionPoint()))
}

// RegisterFakePlugin registers a FakePlugin with the given registry
func RegisterFakePlugin(name string, plugin *FakePlugin, registry pluginregistry.Registry) {
	pluginregistry.Register(
		name,
		NewPluginFncFromFake(plugin),
		&FakePlugin{},
		&FakePluginArgs{},
		ValidateFakePluginArgs,
		SetDefaults_FakePluginArgs,
		registry,
	)
}

// RegisterFakeDeschedulePlugin registers a FakeDeschedulePlugin with the given registry
func RegisterFakeDeschedulePlugin(name string, plugin *FakeDeschedulePlugin, registry pluginregistry.Registry) {
	pluginregistry.Register(
		name,
		NewFakeDeschedulePluginFncFromFake(plugin),
		&FakeDeschedulePlugin{},
		&FakeDeschedulePluginArgs{},
		ValidateFakePluginArgs,
		SetDefaults_FakePluginArgs,
		registry,
	)
}

// RegisterFakeBalancePlugin registers a FakeBalancePlugin with the given registry
func RegisterFakeBalancePlugin(name string, plugin *FakeBalancePlugin, registry pluginregistry.Registry) {
	pluginregistry.Register(
		name,
		NewFakeBalancePluginFncFromFake(plugin),
		&FakeBalancePlugin{},
		&FakeBalancePluginArgs{},
		ValidateFakePluginArgs,
		SetDefaults_FakePluginArgs,
		registry,
	)
}

// RegisterFakeFilterPlugin registers a FakeFilterPlugin with the given registry
func RegisterFakeFilterPlugin(name string, plugin *FakeFilterPlugin, registry pluginregistry.Registry) {
	pluginregistry.Register(
		name,
		NewFakeFilterPluginFncFromFake(plugin),
		&FakeFilterPlugin{},
		&FakeFilterPluginArgs{},
		ValidateFakePluginArgs,
		SetDefaults_FakePluginArgs,
		registry,
	)
}
