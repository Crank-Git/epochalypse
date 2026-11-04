#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
export GOCACHE="${GOCACHE:-$PWD/var/cache/go-build}"
export GOMODCACHE="${GOMODCACHE:-$PWD/var/cache/go-mod}"
export GOTMPDIR="${GOTMPDIR:-$PWD/var/tmp}"
mkdir -p "$GOCACHE" "$GOMODCACHE" "$GOTMPDIR"
exec go run ./cmd/epochalypse status "$@"
