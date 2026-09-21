/*
Copyright 2026 The Kubernetes Authors.

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

package deploymentevictionlimit

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
)

// ValidateDeploymentEvictionLimitArgs validates the plugin arguments.
func ValidateDeploymentEvictionLimitArgs(obj runtime.Object) error {
	args := obj.(*DeploymentEvictionLimitArgs)
	if len(args.Deployments) == 0 {
		return fmt.Errorf("deployments must list at least one deployment")
	}

	var errs []error
	seen := map[string]struct{}{}
	for i, d := range args.Deployments {
		if d.Namespace == "" {
			errs = append(errs, fmt.Errorf("deployments[%d]: namespace is required", i))
		}
		if d.Name == "" {
			errs = append(errs, fmt.Errorf("deployments[%d]: name is required", i))
		}
		key := d.Namespace + "/" + d.Name
		if _, dup := seen[key]; dup {
			errs = append(errs, fmt.Errorf("deployments[%d]: %q is listed more than once", i, key))
		}
		seen[key] = struct{}{}
	}
	return utilerrors.NewAggregate(errs)
}
