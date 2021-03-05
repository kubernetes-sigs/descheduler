#!/bin/bash
source "$(dirname "${BASH_SOURCE}")/lib/init.sh"

# forked from github.com/ekalinin/github-markdown-toc/gh-md-toc
wget -O ${OS_OUTPUT_BINPATH}/gh-md-toc https://raw.githubusercontent.com/damemi/github-markdown-toc/hide-footer/gh-md-toc
chmod a+x ${OS_OUTPUT_BINPATH}/gh-md-toc

${OS_OUTPUT_BINPATH}/gh-md-toc --insert --no-backup --hide-footer README.md
