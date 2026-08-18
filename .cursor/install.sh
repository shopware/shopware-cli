#!/usr/bin/env bash
# Idempotent Cloud Agent bootstrap for the shopware-cli Go project.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_DIR"

# Pinned to match CONTRIBUTING.md / mise.toml. Keep the Go version aligned with go.mod.
GO_VERSION="go1.26.4"
GOLANGCI_LINT_VERSION="v2.12.0"

# Warm the module cache and download the Go toolchain declared in go.mod
# (the base image ships an older Go, so GOTOOLCHAIN=auto fetches ${GO_VERSION}).
go mod download

# Build the CLI to verify the toolchain works and to warm the build cache.
go build -o shopware-cli .

# Install golangci-lint into GOPATH/bin (already on PATH). It must be built with
# the same Go toolchain the project targets, otherwise it refuses to lint code
# that declares a newer language version than the one it was compiled with.
GOBIN_DIR="$(go env GOPATH)/bin"
if ! "${GOBIN_DIR}/golangci-lint" --version 2>/dev/null \
    | grep -q "has version ${GOLANGCI_LINT_VERSION#v} built with ${GO_VERSION}"; then
    GOTOOLCHAIN="${GO_VERSION}" go install \
        "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${GOLANGCI_LINT_VERSION}"
fi

echo "shopware-cli environment ready:"
go version
"${GOBIN_DIR}/golangci-lint" --version
