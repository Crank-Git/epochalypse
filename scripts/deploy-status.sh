#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
export GOCACHE="${GOCACHE:-$PWD/var/cache/go-build}"
export GOMODCACHE="${GOMODCACHE:-$PWD/var/cache/go-mod}"
export GOTMPDIR="${GOTMPDIR:-$PWD/var/tmp}"
export GOPATH="${GOPATH:-$PWD/var/gopath}"
export XDG_CACHE_HOME="${XDG_CACHE_HOME:-$PWD/var/cache}"
export BUILDX_CONFIG="${BUILDX_CONFIG:-$PWD/var/cache/buildx}"
mkdir -p "$GOCACHE" "$GOMODCACHE" "$GOTMPDIR" "$GOPATH" "$XDG_CACHE_HOME" "$BUILDX_CONFIG"
exec go run ./cmd/epochalypse status "$@"
