#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

source "$(dirname "${BASH_SOURCE}")/lib/init.sh"
DESCHEDULER_ROOT=$(dirname "${BASH_SOURCE}")/..
_tmpdir="$(mktemp -d "${DESCHEDULER_ROOT}/_tmp/kube-verify.XXXXXX")"

_deschedulertmp="${_tmpdir}"
mkdir -p "${_deschedulertmp}"

git archive --format=tar --prefix=descheduler/ "$(git write-tree)" | (cd "${_deschedulertmp}" && tar xf -)
_deschedulertmp="${_deschedulertmp}/descheduler"

pushd "${_deschedulertmp}" > /dev/null 2>&1
./hack/update-generated-defaulters.sh
popd > /dev/null 2>&1

pushd "${DESCHEDULER_ROOT}" > /dev/null 2>&1
if ! _out="$(diff -Naupr pkg/ "${_deschedulertmp}/pkg/")"; then
    echo "Generated defaulters output differs:" >&2
    echo "${_out}" >&2
    echo "Generated defaulters verify failed. Please run ./hack/update-generated-defaulters.sh"
fi
popd > /dev/null 2>&1

echo "Generated Defaulters verified."
