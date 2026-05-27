#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$REPO_DIR"

COVERPROFILE="${COVERAGE_FILE:-}"

if [ $# -eq 0 ]; then
    set -- -v ./...
fi

# Warm the build cache while we still have network so the sandboxed run
# does not need to fetch or compile anything from the module proxy.
go test -run='^$' ./...

# If a coverage file is requested, install gocover-cobertura while we have network.
if [ -n "$COVERPROFILE" ]; then
    go install github.com/boumenot/gocover-cobertura@v1.5.0
fi

# Inside the sandbox, force the toolchain to fail fast on any cache miss
# instead of attempting (and hanging on) a network fetch.
export GOFLAGS="${GOFLAGS:-} -mod=readonly"
export GOPROXY=off

# Tells tests that depend on real external downloads (e.g. fetching PHP wasm
# binaries or dart-sass) to skip themselves instead of failing on DNS.
export SHOPWARE_CLI_NO_NETWORK=1

COVER_FLAGS=()
if [ -n "$COVERPROFILE" ]; then
    COVER_FLAGS=("-coverprofile=$COVERPROFILE")
fi

case "$(uname -s)" in
    Darwin)
        if ! command -v sandbox-exec >/dev/null 2>&1; then
            echo "error: sandbox-exec not found" >&2
            exit 1
        fi
        sandbox-exec -f "$REPO_DIR/sandbox-no-network.sb" go test "${COVER_FLAGS[@]}" "$@"
        ;;
    Linux)
        if ! command -v unshare >/dev/null 2>&1; then
            echo "error: unshare not found (expected from util-linux)" >&2
            exit 1
        fi
        # Bring loopback up inside the new netns so tests that use
        # httptest.NewServer (127.0.0.1) keep working; only external
        # network is blocked, matching the nix-build sandbox.
        unshare --user --map-root-user --net -- bash -c '
            if command -v ip >/dev/null 2>&1; then
                ip link set dev lo up
            elif command -v ifconfig >/dev/null 2>&1; then
                ifconfig lo up
            else
                echo "error: need either ip (iproute2) or ifconfig to bring up loopback" >&2
                exit 1
            fi
            exec go test "${COVER_FLAGS[@]}" "$@"
        ' bash "$@"
        ;;
    *)
        echo "error: unsupported OS: $(uname -s)" >&2
        exit 1
        ;;
esac

# Convert Go coverage profile to Cobertura XML if requested.
if [ -n "$COVERPROFILE" ]; then
    echo "Converting coverage to Cobertura XML..."
    gocover-cobertura < "$COVERPROFILE" > "${COVERPROFILE%.out}.xml"
    echo "Coverage report: ${COVERPROFILE%.out}.xml"
fi
