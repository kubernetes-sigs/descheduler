# Copyright 2016 The Kubernetes Authors.
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

load("//defs:pkg.bzl", "pkg_tar")
load("@bazel_tools//tools/build_defs/pkg:pkg.bzl", "pkg_deb")

KUBERNETES_AUTHORS = "Kubernetes Authors <kubernetes-dev+release@googlegroups.com>"

KUBERNETES_HOMEPAGE = "http://kubernetes.io"

GOARCH_TO_DEBARCH = {
    "386": "i386",
    "amd64": "amd64",
    "arm": "armhf",
    "arm64": "arm64",
    "ppc64le": "ppc64el",
    "s390x": "s390x",
}

def k8s_deb(name, goarch = "amd64", tags = None, **kwargs):
    debarch = GOARCH_TO_DEBARCH[goarch]
    pkg_deb(
        name = name + "-" + goarch,
        architecture = debarch,
        data = select({"@io_bazel_rules_go//go/platform:" + goarch: name + "-data-" + goarch}),
        homepage = KUBERNETES_HOMEPAGE,
        maintainer = KUBERNETES_AUTHORS,
        package = name,
        tags = tags,
        **kwargs
    )

def deb_data(name, goarch = "amd64", data = [], tags = None, visibility = None):
    deps = []
    for i, info in enumerate(data):
        dname = "%s-deb-data-%s-%s" % (name, goarch, i)
        deps += [dname]
        pkg_tar(
            name = dname,
            srcs = select({"@io_bazel_rules_go//go/platform:" + goarch: info["files"]}),
            mode = info["mode"],
            package_dir = info["dir"],
            tags = tags,
            visibility = visibility,
        )
    pkg_tar(
        name = name + "-data-" + goarch,
        tags = tags,
        visibility = visibility,
        deps = select({"@io_bazel_rules_go//go/platform:" + goarch: deps}),
    )
