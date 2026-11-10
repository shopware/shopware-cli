#!/usr/bin/env bash
set -euo pipefail

# Snapshot tests do not account for expandEnd when producing fixed output.
# Exercise the actual fixer instead, including both rules in the same pass.
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fixture_dir="$repo_root/ast-grep/tests/fixes"
test_dir="$(mktemp -d)"
trap 'rm -rf "$test_dir"' EXIT

cp "$fixture_dir/input.go.txt" "$test_dir/commands.go"
ast-grep scan --config "$repo_root/sgconfig.yml" "$test_dir" --update-all
# Field removal can leave indentation on an otherwise empty line.
diff -ub "$fixture_dir/expected.go.txt" "$test_dir/commands.go"

# A fixed file must lint cleanly and remain identical after another fix pass.
ast-grep scan --config "$repo_root/sgconfig.yml" "$test_dir"
cp "$test_dir/commands.go" "$test_dir/first-pass.go.txt"
ast-grep scan --config "$repo_root/sgconfig.yml" "$test_dir" --update-all
cmp "$test_dir/first-pass.go.txt" "$test_dir/commands.go"
