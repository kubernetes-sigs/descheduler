/*
Copyright 2022 The Kubernetes Authors.
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

package defaultevictor

import (
	"fmt"

	"k8s.io/klog/v2"

	"k8s.io/apimachinery/pkg/runtime"
)

func ValidateDefaultEvictorArgs(logger klog.Logger, obj runtime.Object) error {
	args := obj.(*DefaultEvictorArgs)

	if args.PriorityThreshold != nil && args.PriorityThreshold.Value != nil && len(args.PriorityThreshold.Name) > 0 {
		return fmt.Errorf("priority threshold misconfigured, only one of priorityThreshold fields can be set, got %v", args)
	}

	if args.MinReplicas == 1 {
		logger.V(4).Info("DefaultEvictor minReplicas must be greater than 1 to check for min pods during eviction. This check will be ignored during eviction.")
	}

	return nil
}
