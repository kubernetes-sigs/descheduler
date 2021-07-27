#!/usr/bin/env bash

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

set -x
set -o errexit
set -o nounset

# This just runs e2e tests.
if [ -n "$KIND_E2E" ]; then
    K8S_VERSION=${KUBERNETES_VERSION:-v1.21.1}
    curl -Lo kubectl https://storage.googleapis.com/kubernetes-release/release/${K8S_VERSION}/bin/linux/amd64/kubectl && chmod +x kubectl && mv kubectl /usr/local/bin/
    wget https://github.com/kubernetes-sigs/kind/releases/download/v0.11.0/kind-linux-amd64
    chmod +x kind-linux-amd64
    mv kind-linux-amd64 kind
    export PATH=$PATH:$PWD
    kind create cluster --image kindest/node:${K8S_VERSION} --config=./hack/kind_config.yaml
    docker pull kubernetes/pause
    kind load docker-image kubernetes/pause
    kind get kubeconfig > /tmp/admin.conf
    export KUBECONFIG="/tmp/admin.conf"
    mkdir -p ~/gopath/src/sigs.k8s.io/
fi

PRJ_PREFIX="sigs.k8s.io/descheduler"
go test ${PRJ_PREFIX}/test/e2e/ -v
