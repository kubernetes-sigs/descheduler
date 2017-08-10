#!/bin/bash
source "$(dirname "${BASH_SOURCE}")/lib/init.sh"

go build -o "${OS_OUTPUT_BINPATH}/defaulter-gen" "${PRJ_PREFIX}/vendor/k8s.io/kubernetes/cmd/libs/go2idl/defaulter-gen"

${OS_OUTPUT_BINPATH}/defaulter-gen \
                --go-header-file "hack/boilerplate/boilerplate.go.txt" \
                --input-dirs "${PRJ_PREFIX}/pkg/apis/componentconfig,${PRJ_PREFIX}/pkg/apis/componentconfig/v1alpha1,${PRJ_PREFIX}/pkg/api/v1alpha1" \
		--extra-peer-dirs "${PRJ_PREFIX}/pkg/apis/componentconfig,${PRJ_PREFIX}/pkg/apis/componentconfig/v1alpha1,${PRJ_PREFIX}/pkg/api/v1alpha1" \
                --output-file-base zz_generated.defaults
