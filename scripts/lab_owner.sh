#!/bin/sh
set -eu
umask 077
if test -f /state/lab/owner.initialized; then
    test -f /state/v1/owner.db
    exit 0
fi
epochalypse owner init --state-dir /state --password-file /state/lab/password
touch /state/lab/owner.initialized
