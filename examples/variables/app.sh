#!/bin/sh
set -eu

test "$MODE" = deploy
test "$EMPTY" = deploy
test "$GLOBAL_VALUE" = from-global
test "${SCOPED_EMPTY+x}" = x
test -z "$SCOPED_EMPTY"
test "$DEPLOY_ONLY" = runtime
printf 'variables-build-succeeded:%s\n' "$MODE"
