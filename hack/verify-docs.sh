#!/bin/bash

# Copyright 2023 The Kubernetes Authors.
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
set -o nounset
set -o pipefail

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

temp_dir=$(mktemp -d)

go run -ldflags "-X main.docGenPath=${temp_dir}" ${SCRIPT_DIR}/doc-gen

if ! _out="$(diff -Naupr ${SCRIPT_DIR}/../docs/cli "${temp_dir}")"; then
    echo "Generated output differs:" >&2
    echo "${_out}" >&2
    echo "Generated conversions verify failed. Please run ./hack/update-docs.sh"
    rm -rf ${temp_dir}
    exit 1
fi

rm -rf ${temp_dir}
