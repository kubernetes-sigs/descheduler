/*
Copyright 2025 The Kubernetes Authors.

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

package example

import (
	"fmt"
	"regexp"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
)

// ValidateExampleArgs validates if the plugin arguments are correct (we have
// everything we need). On this case we only validate if we have a valid
// regular expression and maximum age.
func ValidateExampleArgs(obj runtime.Object) error {
	args := obj.(*ExampleArgs)
	if args.Regex == "" {
		return fmt.Errorf("regex argument must be set")
	}

	if _, err := regexp.Compile(args.Regex); err != nil {
		return fmt.Errorf("invalid regex: %v", err)
	}

	if _, err := time.ParseDuration(args.MaxAge); err != nil {
		return fmt.Errorf("invalid max age: %v", err)
	}

	return nil
}
