#!/usr/bin/env bash

# Copyright 2021 The Kubernetes Authors.
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

K8S_VERSION=${KUBERNETES_VERSION:-v1.21.1}
IMAGE_REPO=${HELM_IMAGE_REPO:-descheduler}
IMAGE_TAG=${HELM_IMAGE_TAG:-helm-test}
CHART_LOCATION=${HELM_CHART_LOCATION:-./charts/descheduler}
VERSION=helm-test make image
wget https://github.com/kubernetes-sigs/kind/releases/download/v0.11.0/kind-linux-amd64
chmod +x kind-linux-amd64
mv kind-linux-amd64 kind
export PATH=$PATH:$PWD
kind create cluster --image kindest/node:"${K8S_VERSION}" --config=./hack/kind_config.yaml
kind load docker-image descheduler:helm-test
helm install descheduler-ci --set image.repository="${IMAGE_REPO}",image.tag="${IMAGE_TAG}",schedule="* * * * *" --namespace kube-system "${CHART_LOCATION}"
sleep 61 # sleep until Job is triggered
helm test descheduler-ci --namespace kube-system

# Delete kind cluster once test is finished
kind delete cluster
