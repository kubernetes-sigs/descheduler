#!/bin/bash

# Copyright 2024 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# go::verify_version verifies the go version is supported by the project.
# descheduler actively supports 3 versions, therefore 3 go versions are supported.
go::verify_version() {
  GO_VERSION=($(go version))

  if [[ -z $(echo "${GO_VERSION[2]}" | grep -E 'go1.21|go1.22|go1.23') ]]; then
    echo "Unknown go version '${GO_VERSION[2]}', skipping gofmt."
    exit 1
  fi
}
