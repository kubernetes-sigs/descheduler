/*
Copyright 2021 The Kubernetes Authors.

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
package strategies

import "sync"

// StrategyOption represents an optional paramater to a strategy evaluation
// function. The structure of the stratey option  is loosely based on
// (blatently plagerized from) the GRPC client option code.
type StrategyOption interface {
	apply(*strategyOption)
}

// WithWaitGroup provides a strategy with a wait group reference that
// can be utilized by the strategy to informm the calling loop when
// the strategy is run to completion. This an be usedful if the
// strategy needs to implement a retry loop for updating k8s resources.
func WithWaitGroup(wg *sync.WaitGroup) StrategyOption {
	return newFuncStrategyOption(func(o *strategyOption) {
		o.wg = wg
	})
}

// buildStrategyOptions combines the given options into a single
// option structure
func buildStrategyOptions(sopts []StrategyOption) *strategyOption {
	opts := strategyOption{}
	for _, sopt := range sopts {
		sopt.apply(&opts)
	}
	return &opts
}

// strategyOption optional parameters to strategy evaluators. Each
// strategy implementation can choose to leverage or ignore these
// values and thus this can be used to add additional parameters
// to strategies without having to update each evaluator signature.
type strategyOption struct {
	wg *sync.WaitGroup
}

// funcStrategyOption wraps a function that modifies the
// strategyOptions into an implementation of a StrategyOption
// interface
type funcStrategyOption struct {
	f func(*strategyOption)
}

func (fso *funcStrategyOption) apply(o *strategyOption) {
	fso.f(o)
}

func newFuncStrategyOption(f func(o *strategyOption)) *funcStrategyOption {
	return &funcStrategyOption{f: f}
}
