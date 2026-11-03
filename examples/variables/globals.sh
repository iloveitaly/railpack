#!/bin/sh
set -eu

test "$MODE" = global
test "${EMPTY+x}" = x
test -z "$EMPTY"
test "$GLOBAL_VALUE" = from-global
printf 'variables-global-succeeded:%s\n' "$MODE"
