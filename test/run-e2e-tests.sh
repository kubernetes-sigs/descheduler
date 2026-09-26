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

BASEDIR=$(dirname "$0")
# shellcheck source=test/lib/e2e-common.sh
source "${BASEDIR}/lib/e2e-common.sh"
# shellcheck source=test/lib/e2e-versions.env
source "${BASEDIR}/lib/e2e-versions.env"
REPO_ROOT="$(e2e_repo_root "${BASEDIR}")"

# Set to empty if unbound/empty
SKIP_INSTALL=${SKIP_INSTALL:-}
KIND_E2E=${KIND_E2E:-}
CONTAINER_ENGINE=${CONTAINER_ENGINE:-docker}
KIND_SUDO=${KIND_SUDO:-}
SKIP_KUBECTL_INSTALL=${SKIP_KUBECTL_INSTALL:-}
SKIP_KIND_INSTALL=${SKIP_KIND_INSTALL:-}
SKIP_KUBEVIRT_INSTALL=${SKIP_KUBEVIRT_INSTALL:-}
SKIP_METRICS_SERVER_INSTALL=${SKIP_METRICS_SERVER_INSTALL:-}
KUBEVIRT_VERSION=${KUBEVIRT_VERSION:-$(grep 'kubevirt.io/api ' "${REPO_ROOT}/go.mod" | awk '{print $2}')}

# Build a descheduler image
IMAGE_TAG=v$(date +%Y%m%d)-$(git describe --tags)
VERSION="${IMAGE_TAG}" make -C "${REPO_ROOT}" image

export DESCHEDULER_IMAGE="docker.io/library/descheduler:${IMAGE_TAG}"
echo "DESCHEDULER_IMAGE: ${DESCHEDULER_IMAGE}"

if [ -n "${KIND_E2E}" ]; then
  # shellcheck source=test/lib/setup-kind.sh
  source "${BASEDIR}/lib/setup-kind.sh"
fi

# Deploy rbac, sa and binding for a descheduler running through a deployment
kubectl apply -f "${REPO_ROOT}/kubernetes/base/rbac.yaml"

trap 'collect_logs default kubevirt' ERR

if [ -z "${SKIP_KUBEVIRT_INSTALL}" ]; then
  # shellcheck source=test/lib/install-kubevirt.sh
  source "${BASEDIR}/lib/install-kubevirt.sh"
fi

if [ -z "${SKIP_METRICS_SERVER_INSTALL}" ]; then
  # shellcheck source=test/lib/install-metrics-server.sh
  source "${BASEDIR}/lib/install-metrics-server.sh"
fi

PRJ_PREFIX="sigs.k8s.io/descheduler"
GOEXPERIMENT="${GOEXPERIMENT:-none}" go test ${PRJ_PREFIX}/test/e2e/ -v -timeout 0 --args --descheduler-image "${DESCHEDULER_IMAGE}" --kubevirt-version-tag "${KUBEVIRT_VERSION}" --pod-run-as-user-id 1000 --pod-run-as-group-id 1000
