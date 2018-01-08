#!/bin/bash
source "$(dirname "${BASH_SOURCE}")/lib/init.sh"

go build -o "${OS_OUTPUT_BINPATH}/conversion-gen" "${PRJ_PREFIX}/vendor/k8s.io/code-generator/cmd/conversion-gen"

${OS_OUTPUT_BINPATH}/conversion-gen \
		--go-header-file "hack/boilerplate/boilerplate.go.txt" \
		--input-dirs "${PRJ_PREFIX}/pkg/apis/componentconfig/v1alpha1,${PRJ_PREFIX}/pkg/api/v1alpha1" \
		--output-file-base zz_generated.conversion
