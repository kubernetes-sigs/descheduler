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

# Set to empty if unbound/empty
SKIP_INSTALL=${SKIP_INSTALL:-}
KIND_E2E=${KIND_E2E:-}
CONTAINER_ENGINE=${CONTAINER_ENGINE:-docker}
KIND_SUDO=${KIND_SUDO:-}
SKIP_KUBECTL_INSTALL=${SKIP_KUBECTL_INSTALL:-}
SKIP_KIND_INSTALL=${SKIP_KIND_INSTALL:-}
SKIP_KUBEVIRT_INSTALL=${SKIP_KUBEVIRT_INSTALL:-}
KUBEVIRT_VERSION=${KUBEVIRT_VERSION:-v1.3.0-rc.1}

# Build a descheduler image
IMAGE_TAG=v$(date +%Y%m%d)-$(git describe --tags)
BASEDIR=$(dirname "$0")
VERSION="${IMAGE_TAG}" make -C ${BASEDIR}/.. image

export DESCHEDULER_IMAGE="docker.io/library/descheduler:${IMAGE_TAG}"
echo "DESCHEDULER_IMAGE: ${DESCHEDULER_IMAGE}"

# This just runs e2e tests.
if [ -n "$KIND_E2E" ]; then
    K8S_VERSION=${KUBERNETES_VERSION:-v1.34.0}
    if [ -z "${SKIP_KUBECTL_INSTALL}" ]; then
        curl -Lo kubectl https://dl.k8s.io/release/${K8S_VERSION}/bin/linux/amd64/kubectl && chmod +x kubectl && mv kubectl /usr/local/bin/
    fi
    if [ -z "${SKIP_KIND_INSTALL}" ]; then
        wget https://github.com/kubernetes-sigs/kind/releases/download/v0.30.0/kind-linux-amd64
        chmod +x kind-linux-amd64
        mv kind-linux-amd64 kind
        export PATH=$PATH:$PWD
    fi

    # If we did not set SKIP_INSTALL
    if [ -z "$SKIP_INSTALL" ]; then
        ${KIND_SUDO} kind create cluster --image kindest/node:${K8S_VERSION} --config=./hack/kind_config.yaml
    fi
    ${CONTAINER_ENGINE} pull registry.k8s.io/pause
    if [ "${CONTAINER_ENGINE}" == "podman" ]; then
      podman save registry.k8s.io/pause -o /tmp/pause.tar
      ${KIND_SUDO} kind load image-archive /tmp/pause.tar
      rm /tmp/pause.tar
      podman save ${DESCHEDULER_IMAGE} -o /tmp/descheduler.tar
      ${KIND_SUDO} kind load image-archive /tmp/descheduler.tar
      rm /tmp/descheduler.tar
    else
      ${KIND_SUDO} kind load docker-image registry.k8s.io/pause
      ${KIND_SUDO} kind load docker-image ${DESCHEDULER_IMAGE}
    fi
    ${KIND_SUDO} kind get kubeconfig > /tmp/admin.conf

    export KUBECONFIG="/tmp/admin.conf"
    mkdir -p ~/gopath/src/sigs.k8s.io/
fi

# Deploy rbac, sa and binding for a descheduler running through a deployment
kubectl apply -f kubernetes/base/rbac.yaml

collect_logs() {
  echo "Collecting pods and logs"
  kubectl get pods -n default
  kubectl get pods -n kubevirt

  for pod in $(kubectl get pods -n default -o name); do
    echo "Logs for ${pod}"
    kubectl logs -n default ${pod}
  done

  for pod in $(kubectl get pods -n kubevirt -o name); do
    echo "Logs for ${pod}"
    kubectl logs -n kubevirt ${pod}
  done
}

trap "collect_logs" ERR

if [ -z "${SKIP_KUBEVIRT_INSTALL}" ]; then
  kubectl create -f https://github.com/kubevirt/kubevirt/releases/download/${KUBEVIRT_VERSION}/kubevirt-operator.yaml
  kubectl create -f https://github.com/kubevirt/kubevirt/releases/download/${KUBEVIRT_VERSION}/kubevirt-cr.yaml
  kubectl wait --timeout=180s --for=condition=Available -n kubevirt kv/kubevirt
  kubectl -n kubevirt patch kubevirt kubevirt --type=merge --patch '{"spec":{"configuration":{"developerConfiguration":{"useEmulation":true}}}}'
fi

METRICS_SERVER_VERSION="v0.5.0"
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/download/${METRICS_SERVER_VERSION}/components.yaml
kubectl patch -n kube-system deployment metrics-server --type=json \
  -p '[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--kubelet-insecure-tls"}]'

PRJ_PREFIX="sigs.k8s.io/descheduler"
go test ${PRJ_PREFIX}/test/e2e/ -v -timeout 0 --args --descheduler-image ${DESCHEDULER_IMAGE} --pod-run-as-user-id 1000 --pod-run-as-group-id 1000
