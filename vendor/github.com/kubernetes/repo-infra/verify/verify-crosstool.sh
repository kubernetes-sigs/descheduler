#!/usr/bin/env bash
# Copyright 2019 The Kubernetes Authors.
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

checked_in_crosstool=$1
generated_crosstool=$2

diff=$(diff -u "${checked_in_crosstool}" "${generated_crosstool}" || true)

if [[ -n "${diff}" ]]; then
  echo "Checked-in CROSSTOOL does not match generated CROSSTOOL"
  echo "${diff}"
  echo
  echo
  echo "To fix: "
  echo "  cp $(realpath ${generated_crosstool}) $(realpath ${checked_in_crosstool})"
  exit 1
fi
