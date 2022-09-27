#!/bin/bash
source "$(dirname "${BASH_SOURCE}")/lib/init.sh"
source "$(dirname "${BASH_SOURCE}")/lib/generator-help.sh"

go build -o "${OS_OUTPUT_BINPATH}/deepcopy-gen" "k8s.io/code-generator/cmd/deepcopy-gen"

${OS_OUTPUT_BINPATH}/deepcopy-gen \
                --go-header-file "hack/boilerplate/boilerplate.go.txt" \
                --input-dirs "$(find_dirs_containing_comment_tags "+k8s:deepcopy-gen=")" \
                --output-file-base zz_generated.deepcopy

