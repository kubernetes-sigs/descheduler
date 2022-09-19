#!/bin/bash
source "$(dirname "${BASH_SOURCE}")/lib/init.sh"

go build -o "${OS_OUTPUT_BINPATH}/conversion-gen" "k8s.io/code-generator/cmd/conversion-gen"

${OS_OUTPUT_BINPATH}/conversion-gen \
		--go-header-file "hack/boilerplate/boilerplate.go.txt" \
		--input-dirs "${PRJ_PREFIX}/pkg/apis/componentconfig/v1alpha1,${PRJ_PREFIX}/pkg/api/v1alpha1,${PRJ_PREFIX}/pkg/framework/plugins/removefailedpods,${PRJ_PREFIX}/pkg/framework/plugins/nodeutilization,${PRJ_PREFIX}/pkg/framework/plugins/podlifetime,${PRJ_PREFIX}/pkg/framework/plugins/removeduplicates,${PRJ_PREFIX}/pkg/framework/plugins/removepodshavingtoomanyrestarts" \
		--output-file-base zz_generated.conversion
