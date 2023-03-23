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
	return func(args runtime.Object, handle frameworktypes.Handle) (frameworktypes.Plugin, error) {
		fakePluginArgs, ok := args.(*FakePluginArgs)
		if !ok {
			return nil, fmt.Errorf("want args to be of type FakePluginArgs, got %T", args)
		}

		fp.handle = handle
		fp.args = fakePluginArgs

		return fp, nil
	}
}

// New builds plugin from its arguments while passing a handle
func New(args runtime.Object, handle frameworktypes.Handle) (frameworktypes.Plugin, error) {
	fakePluginArgs, ok := args.(*FakePluginArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type FakePluginArgs, got %T", args)
	}

	ev := &FakePlugin{}
	ev.handle = handle
	ev.args = fakePluginArgs

	return ev, nil
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
		handled, err := reactor.React(actionCopy)
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
