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

load("@bazel_tools//tools/build_defs/pkg:rpm.bzl", "pkg_rpm")

GOARCH_TO_RPMARCH = {
    "386": "i386",
    "amd64": "x86_64",
    "arm": "armhfp",
    "arm64": "aarch64",
    "ppc64le": "ppc64le",
    "s390x": "s390x",
}

def pkg_rpm_for_goarch(name, data, goarch, tags = None, **kwargs):
    rpmarch = GOARCH_TO_RPMARCH[goarch]
    pkg_rpm(
        name = name + "-" + goarch,
        architecture = rpmarch,
        data = select(
            {
                "@io_bazel_rules_go//go/platform:" + goarch: [
                    d.format(GOARCH = goarch, RPMARCH = rpmarch)
                    for d in data
                ],
            },
        ),
        tags = tags,
        **kwargs
    )
