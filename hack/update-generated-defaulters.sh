#!/bin/bash
source "$(dirname "${BASH_SOURCE}")/lib/init.sh"
source "$(dirname "${BASH_SOURCE}")/lib/generator-help.sh"

go build -o "${OS_OUTPUT_BINPATH}/defaulter-gen" "k8s.io/code-generator/cmd/defaulter-gen"

${OS_OUTPUT_BINPATH}/defaulter-gen \
                --go-header-file "hack/boilerplate/boilerplate.go.txt" \
                --input-dirs "$(find_dirs_containing_comment_tags "+k8s:defaulter-gen=")" \
		--extra-peer-dirs "${PRJ_PREFIX}/pkg/apis/componentconfig/v1alpha1,${PRJ_PREFIX}/pkg/api/v1alpha1" \
                --output-file-base zz_generated.defaults
