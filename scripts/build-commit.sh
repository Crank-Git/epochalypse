#!/bin/sh
set -eu
commit=$(git rev-parse HEAD)
if [ -n "$(git status --porcelain)" ]; then
    commit="$commit-dirty"
fi
printf '%s\n' "$commit"
