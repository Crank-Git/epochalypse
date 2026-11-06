#!/bin/sh
set -eu

usage() {
    printf '%s\n' 'Usage: scripts/recovery-drill.sh [--help]' \
        'Run the isolated Linux recovery drill with synthetic content only.' \
        'Requires Docker and Go 1.27.3. Accepts no remote targets.'
}

if [ "$#" -ne 0 ]; then
    if [ "$#" -eq 1 ] && [ "$1" = '--help' ]; then
        usage
        exit 0
    fi
    usage >&2
    exit 2
fi

cd -- "$(dirname -- "$0")/.."
root=$(pwd -P)
export GOCACHE="$root/var/cache/go-build"
export GOMODCACHE="$root/var/cache/go-mod"
export GOPATH="$root/var/gopath"
export GOTMPDIR="$root/var/tmp"
export XDG_CACHE_HOME="$root/var/cache"
export BUILDX_CONFIG="$root/var/cache/buildx"
mkdir -p "$GOCACHE" "$GOMODCACHE" "$GOPATH" "$GOTMPDIR" "$XDG_CACHE_HOME" "$BUILDX_CONFIG"
exec python3 scripts/test_switch_arm.py
