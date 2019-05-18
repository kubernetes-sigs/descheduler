#!/bin/bash
source "$(dirname "${BASH_SOURCE}")/lib/init.sh"

go build -o "${OS_OUTPUT_BINPATH}/openapi-gen" "${PRJ_PREFIX}/vendor/k8s.io/kube-openapi/cmd/openapi-gen" 

${OS_OUTPUT_BINPATH}/openapi-gen --go-header-file "hack/boilerplate/boilerplate.go.txt" --output-package "${PRJ_PREFIX}/vendor/k8s.io/kubernetes/pkg/generated/openapi/" --input-dirs "${PRJ_PREFIX}/vendor/k8s.io/api/admission/v1beta1,${PRJ_PREFIX}/vendor/k8s.io/api/apps/v1,${PRJ_PREFIX}/vendor/k8s.io/api/apps/v1beta1,${PRJ_PREFIX}/vendor/k8s.io/api/core/v1,${PRJ_PREFIX}/vendor/k8s.io/api/admissionregistration/v1beta1,${PRJ_PREFIX}/vendor/k8s.io/client-go/pkg/version"  --output-file-base zz_generated.openapi.go

