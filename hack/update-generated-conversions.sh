#!/bin/bash
source "$(dirname "${BASH_SOURCE}")/lib/init.sh"
source "$(dirname "${BASH_SOURCE}")/lib/generator-help.sh"

go build -o "${OS_OUTPUT_BINPATH}/conversion-gen" "k8s.io/code-generator/cmd/conversion-gen"

${OS_OUTPUT_BINPATH}/conversion-gen \
		--go-header-file "hack/boilerplate/boilerplate.go.txt" \
		--input-dirs "$(find_dirs_containing_comment_tags "+k8s:conversion-gen=")" \
		--output-file-base zz_generated.conversion
