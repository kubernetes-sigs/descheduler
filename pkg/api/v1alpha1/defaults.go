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

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
)

var (
	nodeAffinityDefaults = DeschedulerStrategy{
		Params: &StrategyParameters{
			NodeFit: pointer.Bool(true),
		},
	}
	genericStrategyDefaults = DeschedulerStrategy{
		Params: &StrategyParameters{
			NodeFit: pointer.Bool(false),
		},
	}

	strategyDefaults = StrategyList{
		StrategyName("RemoveDuplicates"):                            genericStrategyDefaults,
		StrategyName("LowNodeUtilization"):                          genericStrategyDefaults,
		StrategyName("HighNodeUtilization"):                         genericStrategyDefaults,
		StrategyName("RemovePodsViolatingInterPodAntiAffinity"):     genericStrategyDefaults,
		StrategyName("RemovePodsViolatingNodeAffinity"):             nodeAffinityDefaults,
		StrategyName("RemovePodsViolatingNodeTaints"):               genericStrategyDefaults,
		StrategyName("RemovePodsHavingTooManyRestarts"):             genericStrategyDefaults,
		StrategyName("PodLifeTime"):                                 genericStrategyDefaults,
		StrategyName("RemovePodsViolatingTopologySpreadConstraint"): genericStrategyDefaults,
		StrategyName("RemoveFailedPods"):                            genericStrategyDefaults,
	}
)

func SetDefaults_DeschedulerPolicy(obj *DeschedulerPolicy) {
	for strategyName, strategy := range obj.Strategies {
		if defaults, ok := strategyDefaults[strategyName]; ok {
			if strategy.Params == nil {
				strategy.Params = defaults.Params
			} else {
				if strategy.Params.NodeFit == nil {
					strategy.Params.NodeFit = defaults.Params.NodeFit
				}
				// put other parameter default copying here after adding new ones to strategyDefaults
			}
		}
	}
}

func addDefaultingFuncs(scheme *runtime.Scheme) error {
	return RegisterDefaults(scheme)
}
