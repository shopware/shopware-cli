#!/usr/bin/env sh
set -euo pipefail

if [ "${PHP_SESSION_SAVE_PATH:-}" = "" ]; then
    unset PHP_SESSION_SAVE_PATH
fi

exec "$@"
