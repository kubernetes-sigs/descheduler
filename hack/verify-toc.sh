#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

source "$(dirname "${BASH_SOURCE}")/lib/init.sh"

DESCHEDULER_ROOT=$(dirname "${BASH_SOURCE}")/..

mkdir -p "${DESCHEDULER_ROOT}/_tmp"
_tmpdir="$(mktemp -d "${DESCHEDULER_ROOT}/_tmp/kube-vendor.XXXXXX")"

if [[ -z ${KEEP_TMP:-} ]]; then
    KEEP_TMP=false
fi

function cleanup {
  # make go module dirs writeable
  chmod -R +w "${_tmpdir}"
  if [ "${KEEP_TMP}" == "true" ]; then
    echo "Leaving ${_tmpdir} for you to examine or copy. Please delete it manually when finished. (rm -rf ${_tmpdir})"
  else
    echo "Removing ${_tmpdir}"
    rm -rf "${_tmpdir}"
  fi
}
trap "cleanup" EXIT

_deschedulertmp="${_tmpdir}"
mkdir -p "${_deschedulertmp}"

git archive --format=tar --prefix=descheduler/ "$(git write-tree)" | (cd "${_deschedulertmp}" && tar xf -)
_deschedulertmp="${_deschedulertmp}/descheduler"

cp -r _output "${_deschedulertmp}"

pushd "${_deschedulertmp}" > /dev/null 2>&1
hack/update-toc.sh
popd > /dev/null 2>&1

ret=0

pushd "${DESCHEDULER_ROOT}" > /dev/null 2>&1
if ! _out="$(diff README.md "${_deschedulertmp}/README.md")"; then
    echo "Generated table of contents differs:" >&2
    echo "${_out}" >&2
    echo "TOC verify failed." >&2
    echo "${_out}" > vendordiff.patch
    echo "If you're seeing this locally, run the below command to fix your table of contents:" >&2
    echo "hack/update-toc.sh" >&2
fi
popd > /dev/null 2>&1

if [[ ${ret} -gt 0 ]]; then
  exit ${ret}
fi

echo "Table of contents verified."
