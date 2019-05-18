# Copyright 2017 The Kubernetes Authors.
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

load(
    "@bazel_tools//tools/build_defs/pkg:pkg.bzl",
    _real_pkg_tar = "pkg_tar",
)

# pkg_tar wraps the official pkg_tar rule with our faster
# Go-based build_tar binary.
# Additionally, the upstream pkg_tar rule defaults mode to "0555",
# which prevents build_tar from automatically choosing an
# appropriate mode, so we instead default it to "".
def pkg_tar(
        build_tar = "@io_k8s_repo_infra//tools/build_tar",
        mode = "",
        **kwargs):
    _real_pkg_tar(build_tar = build_tar, mode = mode, **kwargs)
