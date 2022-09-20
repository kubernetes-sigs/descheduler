#!/usr/bin/env bash

# Utility command based on 'find' command. The pipeline is as following:
#   1. find all the go files; (exclude specific path: vendor etc)
#   2. find all the files containing specific tags in contents;
#   3. extract related dirs;
#   4. remove duplicated paths;
#   5. replace . with ${PRJ_PREFIX}
#   6. merge all dirs in array with delimiter ,;
#
# Example:
#   find_dirs_containing_comment_tags("+k8s:")
# Return:
#   sigs.k8s.io/descheduler/a,sigs.k8s.io/descheduler/b,sigs.k8s.io/descheduler/c
function find_dirs_containing_comment_tags() {
   array=()
   while IFS='' read -r line; do array+=("$line"); done < <( \
     find . -type f -name \*.go -not -path "./vendor/*" -print0  \
     | xargs -0 grep --color=never -l "$@" \
     | xargs -n1 dirname \
     | LC_ALL=C sort -u \
     | sed "s|.|${PRJ_PREFIX}|" \
     )

   IFS=",";
   printf '%s' "${array[*]}";
}