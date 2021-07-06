#!/bin/bash

# Copyright 2014 The Kubernetes Authors.
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

set -o errexit
set -o pipefail


# os::util::absolute_path returns the absolute path to the directory provided
function os::util::absolute_path() {
        local relative_path="$1"
        local absolute_path

        pushd "${relative_path}" >/dev/null
        relative_path="$( pwd )"
        if [[ -h "${relative_path}" ]]; then
                absolute_path="$( readlink "${relative_path}" )"
        else
                absolute_path="${relative_path}"
        fi
        popd >/dev/null

        echo "${absolute_path}"
}
readonly -f os::util::absolute_path

# os::util::project_within_gopath checks if the OS_ROOT exists within a GOPATH
function os::util::project_within_gopath() {
	local within_gopath=false

	if [[ ! -z "$GOPATH" ]]; then
		for p in $GOPATH; do
			# As OS_ROOT is an absolute path, we can simply check
			# to see if it begins with any of the GOPATH paths.
			if [[ $1 == $p* ]]; then
				within_gopath=true
				break
			fi
		done
	fi

	echo $within_gopath
}
readonly -f os::util::project_within_gopath

# find the absolute path to the root of the Origin source tree
init_source="$( dirname "${BASH_SOURCE}" )/../.."
OS_ROOT="$( os::util::absolute_path "${init_source}" )"
export OS_ROOT
cd "${OS_ROOT}"

WITHIN_GOPATH="$( os::util::project_within_gopath "${OS_ROOT}" )"

if [ "$WITHIN_GOPATH" = true ]; then
	PRJ_PREFIX="sigs.k8s.io/descheduler"
else
	PRJ_PREFIX="."
fi

OS_OUTPUT_BINPATH="${OS_ROOT}/_output/bin"
