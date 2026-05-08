#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$REPO_DIR"

if [ $# -eq 0 ]; then
    set -- -v ./...
fi

# Warm the build cache while we still have network so the sandboxed run
# does not need to fetch or compile anything from the module proxy.
go test -run='^$' ./...

# Inside the sandbox, force the toolchain to fail fast on any cache miss
# instead of attempting (and hanging on) a network fetch.
export GOFLAGS="${GOFLAGS:-} -mod=readonly"
export GOPROXY=off

case "$(uname -s)" in
    Darwin)
        if ! command -v sandbox-exec >/dev/null 2>&1; then
            echo "error: sandbox-exec not found" >&2
            exit 1
        fi
        exec sandbox-exec -f "$REPO_DIR/sandbox-no-network.sb" go test "$@"
        ;;
    Linux)
        if ! command -v bwrap >/dev/null 2>&1; then
            echo "error: bwrap (bubblewrap) not found; install it (e.g. apt-get install bubblewrap)" >&2
            exit 1
        fi
        exec bwrap \
            --dev-bind / / \
            --tmpfs /tmp \
            --unshare-net \
            --die-with-parent \
            --chdir "$REPO_DIR" \
            go test "$@"
        ;;
    *)
        echo "error: unsupported OS: $(uname -s)" >&2
        exit 1
        ;;
esac
