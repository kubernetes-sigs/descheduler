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
	v1 "k8s.io/api/core/v1"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
)

type Action interface {
	Handle() frameworktypes.Handle
	GetExtensionPoint() string
	DeepCopy() Action
}

type ReactionFunc func(action Action) (handled, filter bool, err error)

// Reactor is an interface to allow the composition of reaction functions.
type Reactor interface {
	// Handles indicates whether or not this Reactor deals with a given
	// action.
	Handles(action Action) bool
	// React handles the action.  It may choose to
	// delegate by indicated handled=false.
	// filter is used to store results of filter based actions
	React(action Action) (handled, filter bool, err error)
}

// SimpleReactor is a Reactor. Each reaction function is attached to a given extensionPoint. "*" in either field matches everything for that value.
type SimpleReactor struct {
	ExtensionPoint string
	Reaction       ReactionFunc
}

func (r *SimpleReactor) Handles(action Action) bool {
	return r.ExtensionPoint == "*" || r.ExtensionPoint == action.GetExtensionPoint()
}

func (r *SimpleReactor) React(action Action) (bool, bool, error) {
	return r.Reaction(action)
}

type DescheduleAction interface {
	Action
	CanDeschedule() bool
	Nodes() []*v1.Node
}

type BalanceAction interface {
	Action
	CanBalance() bool
	Nodes() []*v1.Node
}

type FilterAction interface {
	Action
	CanFilter() bool
}

type PreEvictionFilterAction interface {
	Action
	CanPreEvictionFilter() bool
}

type ActionImpl struct {
	handle         frameworktypes.Handle
	extensionPoint string
}

func (a ActionImpl) Handle() frameworktypes.Handle {
	return a.handle
}

func (a ActionImpl) GetExtensionPoint() string {
	return a.extensionPoint
}

func (a ActionImpl) DeepCopy() Action {
	// The handle is expected to be accessed only throuh interface methods
	// Thus, no deep copy needed.
	ret := a
	return ret
}

type DescheduleActionImpl struct {
	ActionImpl
	nodes []*v1.Node
}

func (d DescheduleActionImpl) CanDeschedule() bool {
	return true
}

func (d DescheduleActionImpl) Nodes() []*v1.Node {
	return d.nodes
}

func (d DescheduleActionImpl) DeepCopy() Action {
	nodesCopy := []*v1.Node{}
	for _, node := range d.nodes {
		nodesCopy = append(nodesCopy, node.DeepCopy())
	}
	return DescheduleActionImpl{
		ActionImpl: d.ActionImpl.DeepCopy().(ActionImpl),
		nodes:      nodesCopy,
	}
}

type BalanceActionImpl struct {
	ActionImpl
	nodes []*v1.Node
}

func (d BalanceActionImpl) CanBalance() bool {
	return true
}

func (d BalanceActionImpl) Nodes() []*v1.Node {
	return d.nodes
}

func (d BalanceActionImpl) DeepCopy() Action {
	nodesCopy := []*v1.Node{}
	for _, node := range d.nodes {
		nodesCopy = append(nodesCopy, node.DeepCopy())
	}
	return BalanceActionImpl{
		ActionImpl: d.ActionImpl.DeepCopy().(ActionImpl),
		nodes:      nodesCopy,
	}
}

type FilterActionImpl struct {
	ActionImpl
}

func (d FilterActionImpl) CanFilter() bool {
	return true
}

func (d FilterActionImpl) DeepCopy() Action {
	return FilterActionImpl{
		ActionImpl: d.ActionImpl.DeepCopy().(ActionImpl),
	}
}

type PreEvictionFilterActionImpl struct {
	ActionImpl
}

func (d PreEvictionFilterActionImpl) CanPreEvictionFilter() bool {
	return true
}

func (d PreEvictionFilterActionImpl) DeepCopy() Action {
	return PreEvictionFilterActionImpl{
		ActionImpl: d.ActionImpl.DeepCopy().(ActionImpl),
	}
}
