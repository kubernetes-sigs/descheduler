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

set -o errexit
set -o nounset
set -o pipefail

TASK=$1
IMAGE=$2

KUBE_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
source "${KUBE_ROOT}/hack/lib/util.sh"

# Mapping of go ARCH to actual architectures shipped part of multiarch/qemu-user-static project
declare -A QEMUARCHS=( ["amd64"]="x86_64" ["arm"]="arm" ["arm64"]="aarch64" ["ppc64le"]="ppc64le" ["s390x"]="s390x" )

# Returns list of all supported architectures from BASEIMAGE file
listArchs() {
  cut -d "=" -f 1 "${IMAGE}"/BASEIMAGE
}

# Returns baseimage need to used in Dockerfile for any given architecture
getBaseImage() {
  arch=$1
  grep "${arch}=" BASEIMAGE | cut -d= -f2
}

# This function will build test image for all the architectures
# mentioned in BASEIMAGE file. In the absence of BASEIMAGE file,
# it will build for all the supported arch list - amd64, arm,
# arm64, ppc64le, s390x
build() {
  if [[ -f ${IMAGE}/BASEIMAGE ]]; then
    archs=$(listArchs)
  else
    archs=${!QEMUARCHS[*]}
  fi

  kube::util::ensure-gnu-sed

  for arch in ${archs}; do
    echo "Building image for ${IMAGE} ARCH: ${arch}..."

    # Create a temporary directory for every architecture and copy the image content
    # and build the image from temporary directory
    mkdir -p "${KUBE_ROOT}"/_tmp
    temp_dir=$(mktemp -d "${KUBE_ROOT}"/_tmp/test-images-build.XXXXXX)
    kube::util::trap_add "rm -rf ${temp_dir}" EXIT

    cp -r "${IMAGE}"/* "${temp_dir}"
    if [[ -f ${IMAGE}/Makefile ]]; then
      # make bin will take care of all the prerequisites needed
      # for building the docker image
      make -C "${IMAGE}" bin ARCH="${arch}" TARGET="${temp_dir}"
    fi
    pushd "${temp_dir}"
    # image tag
    TAG=$(<VERSION)

    if [[ -f BASEIMAGE ]]; then
      BASEIMAGE=$(getBaseImage "${arch}")
      ${SED} -i "s|BASEIMAGE|${BASEIMAGE}|g" Dockerfile
      ${SED} -i "s|BASEARCH|${arch}|g" Dockerfile
    fi

    # copy the qemu-*-static binary to docker image to build the multi architecture image on x86 platform
    if grep -q "CROSS_BUILD_" Dockerfile; then
      if [[ "${arch}" == "amd64" ]]; then
        ${SED} -i "/CROSS_BUILD_/d" Dockerfile
      else
        ${SED} -i "s|QEMUARCH|${QEMUARCHS[$arch]}|g" Dockerfile
        # Register qemu-*-static for all supported processors except the current one
        echo "Registering qemu-*-static binaries in the kernel"
        local sudo=""
        if [[ $(id -u) != 0 ]]; then
          sudo=sudo
        fi
        ${sudo} "${KUBE_ROOT}/third_party/multiarch/qemu-user-static/register/register.sh" --reset
        curl -sSL https://github.com/multiarch/qemu-user-static/releases/download/"${QEMUVERSION}"/x86_64_qemu-"${QEMUARCHS[$arch]}"-static.tar.gz | tar -xz -C "${temp_dir}"
        # Ensure we don't get surprised by umask settings
        chmod 0755 "${temp_dir}/qemu-${QEMUARCHS[$arch]}-static"
        ${SED} -i "s/CROSS_BUILD_//g" Dockerfile
      fi
    fi

    docker build --pull -t "${REGISTRY}/${IMAGE}-${arch}:${TAG}" .

    popd
  done
}

docker_version_check() {
  # The reason for this version check is even though "docker manifest" command is available in 18.03, it does
  # not work properly in that version. So we insist on 18.06.0 or higher.
  docker_version=$(docker version --format '{{.Client.Version}}' | cut -d"-" -f1)
  if [[ ${docker_version} != 18.06.0 && ${docker_version} < 18.06.0 ]]; then
    echo "Minimum docker version 18.06.0 is required for creating and pushing manifest images[found: ${docker_version}]"
    exit 1
  fi
}

# This function will push the docker images
push() {
  docker_version_check
  TAG=$(<"${IMAGE}"/VERSION)
  if [[ -f ${IMAGE}/BASEIMAGE ]]; then
    archs=$(listArchs)
  else
    archs=${!QEMUARCHS[*]}
  fi
  for arch in ${archs}; do
    docker push "${REGISTRY}/${IMAGE}-${arch}:${TAG}"
  done

  kube::util::ensure-gnu-sed

  # The manifest command is still experimental as of Docker 18.09.2
  export DOCKER_CLI_EXPERIMENTAL="enabled"
  # Make archs list into image manifest. Eg: 'amd64 ppc64le' to '${REGISTRY}/${IMAGE}-amd64:${TAG} ${REGISTRY}/${IMAGE}-ppc64le:${TAG}'
  while IFS='' read -r line; do manifest+=("$line"); done < <(echo "$archs" | ${SED} -e "s~[^ ]*~$REGISTRY\/$IMAGE\-&:$TAG~g")
  docker manifest create --amend "${REGISTRY}/${IMAGE}:${TAG}" "${manifest[@]}"
  for arch in ${archs}; do
    docker manifest annotate --arch "${arch}" "${REGISTRY}/${IMAGE}:${TAG}" "${REGISTRY}/${IMAGE}-${arch}:${TAG}"
  done
  docker manifest push --purge "${REGISTRY}/${IMAGE}:${TAG}"
}

# This function is for building the go code
bin() {
  local arch_prefix=""
  if [[ "${ARCH:-}" == "arm" ]]; then
    arch_prefix="GOARM=${GOARM:-7}"
  fi
  for SRC in "$@";
  do
  docker run --rm -it -v "${TARGET}:${TARGET}:Z" -v "${KUBE_ROOT}":/go/src/k8s.io/kubernetes:Z \
        golang:"${GOLANG_VERSION}" \
        /bin/bash -c "\
                cd /go/src/k8s.io/kubernetes/test/images/${SRC_DIR} && \
                CGO_ENABLED=0 ${arch_prefix} GOARCH=${ARCH} go build -a -installsuffix cgo --ldflags '-w' -o ${TARGET}/${SRC} ./$(dirname "${SRC}")"
  done
}

shift

eval "${TASK}" "$@"
