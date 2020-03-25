#!/bin/bash
source "$(dirname "${BASH_SOURCE}")/lib/init.sh"

go build -o "${OS_OUTPUT_BINPATH}/defaulter-gen" "k8s.io/code-generator/cmd/defaulter-gen"

${OS_OUTPUT_BINPATH}/defaulter-gen \
                --go-header-file "hack/boilerplate/boilerplate.go.txt" \
                --input-dirs "${PRJ_PREFIX}/pkg/apis/componentconfig/v1alpha1,${PRJ_PREFIX}/pkg/api/v1alpha1" \
		--extra-peer-dirs "${PRJ_PREFIX}/pkg/apis/componentconfig/v1alpha1,${PRJ_PREFIX}/pkg/api/v1alpha1" \
                --output-file-base zz_generated.defaults
