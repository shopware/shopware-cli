#!/usr/bin/env bash
#
# Automated runner for docs/manual-testing/ai-add-remove.md.
#
# It builds shopware-cli, runs each step from the manual plan, and asserts the
# deterministic parts (exit codes, error text, installed.json contents). Steps
# that a human eyeballs in the doc (streamed skills.sh output) still run and
# their output is shown; only the machine-checkable parts are turned into
# PASS/FAIL.
#
# Usage:
#   docs/manual-testing/run-ai-tests.sh            # full run (needs npx + network)
#   docs/manual-testing/run-ai-tests.sh --smoke    # offline subset (A, B, C1, D2)
#   docs/manual-testing/run-ai-tests.sh --keep     # do not delete artifacts at the end
#
# Env overrides:
#   REPO=/path/to/shopware-cli   # repo root (default: two levels up from this script)
#   SWCLI=/path/to/shopware-cli  # use an existing binary instead of building one

set -uo pipefail

# --- config -----------------------------------------------------------------

REPO="${REPO:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}"

# All work happens under a private mktemp directory this script owns, so cleanup
# never touches caller data. A caller-provided $SWCLI is used as-is and never
# deleted; otherwise the binary is built into the work dir.
WORKDIR="$(mktemp -d "${TMPDIR:-/tmp}/ai-e2e.XXXXXX")"
PROJ="$WORKDIR/proj"
SHOP="$WORKDIR/shopware"
HOMEDIR="$WORKDIR/home"

PROVIDED_SWCLI="${SWCLI:-}"
SWCLI="${SWCLI:-$WORKDIR/swcli}"

SMOKE=0
KEEP=0
for arg in "$@"; do
	case "$arg" in
	--smoke) SMOKE=1 ;;
	--keep) KEEP=1 ;;
	*)
		echo "unknown option: $arg" >&2
		exit 2
		;;
	esac
done

pass=0
fail=0

# --- helpers ----------------------------------------------------------------

step() { printf '\n== %s ==\n' "$1"; }
ok() {
	printf '  PASS: %s\n' "$1"
	pass=$((pass + 1))
}
bad() {
	printf '  FAIL: %s\n' "$1"
	fail=$((fail + 1))
}

expect_exit() { # desc want_code got_code
	if [ "$3" -eq "$2" ]; then ok "$1 (exit $3)"; else bad "$1 (exit $3, want $2)"; fi
}
expect_contains() { # desc needle haystack
	case "$3" in *"$2"*) ok "$1" ;; *) bad "$1 (missing: $2)" ;; esac
}
expect_not_contains() { # desc needle haystack
	case "$3" in *"$2"*) bad "$1 (unexpected: $2)" ;; *) ok "$1" ;; esac
}
file_contains() { # desc needle file
	if [ -f "$3" ] && grep -q "$2" "$3"; then ok "$1"; else bad "$1 (in $3)"; fi
}
file_missing() { # desc needle file  (passes when file has no such line)
	if [ ! -f "$3" ] || ! grep -q "$2" "$3"; then ok "$1"; else bad "$1 (unexpected in $3)"; fi
}

proj_state="$PROJ/.shopware-cli/ai/installed.json"
shop_state="$SHOP/.shopware-cli/ai/installed.json"
global_state() { find "$HOMEDIR" -path '*shopware-cli/ai/installed.json' 2>/dev/null | head -1; }

cleanup() {
	if [ "$KEEP" -eq 1 ]; then
		printf '\n(--keep) artifacts left in %s\n' "$WORKDIR"
		return
	fi
	rm -rf "$WORKDIR"
}
trap cleanup EXIT

# --- prerequisites ----------------------------------------------------------

step "Prerequisites"
if [ -n "$PROVIDED_SWCLI" ]; then
	[ -x "$SWCLI" ] || {
		echo "provided SWCLI is not executable: $SWCLI" >&2
		exit 1
	}
	echo "Using provided binary $SWCLI"
	ok "binary"
else
	command -v go >/dev/null || {
		echo "go not found on PATH" >&2
		exit 1
	}
	echo "Building $SWCLI from $REPO ..."
	(cd "$REPO" && go build -o "$SWCLI" .) || {
		echo "build failed" >&2
		exit 1
	}
	ok "build"
fi

if [ "$SMOKE" -eq 0 ] && ! command -v npx >/dev/null; then
	echo "WARNING: npx not found — real install/remove steps (C2+, D3, D4, E, F2, F5) will fail." >&2
fi

mkdir -p "$PROJ" "$SHOP" "$HOMEDIR"
printf '{"require":{"shopware/core":"^6.6"}}\n' >"$SHOP/composer.json"

# --- A. Directory (read-only) ----------------------------------------------

step "A1 — list (table)"
out=$("$SWCLI" ai list 2>&1)
code=$?
expect_exit "list exits 0" 0 $code
expect_contains "table lists deployment-helper" "deployment-helper" "$out"
expect_contains "table shows active status" "active" "$out"

step "A2 — list (JSON contract)"
out=$("$SWCLI" ai list --format json 2>&1)
code=$?
expect_exit "list json exits 0" 0 $code
expect_contains "json has displayName" '"displayName"' "$out"

step "A3 — type filter reserved but empty"
out=$("$SWCLI" ai list --type mcp --format json 2>&1)
code=$?
expect_exit "mcp filter exits 0" 0 $code
expect_contains "mcp filter yields empty array" "[]" "$out"

step "A4 — invalid type rejected"
out=$("$SWCLI" ai list --type bogus 2>&1)
code=$?
expect_exit "bogus type exits 1" 1 $code

step "A5 — info superset"
out=$("$SWCLI" ai info deployment-helper --format json 2>&1)
code=$?
expect_exit "info exits 0" 0 $code
expect_contains "info has delivery.repository" "deployment-helper" "$out"
expect_contains "info has compatibility.source owner" '"owner"' "$out"

# --- B. add guards (no install) --------------------------------------------

step "B1 — unknown integration"
out=$( (cd "$PROJ" && "$SWCLI" ai add does-not-exist --agent claude-code) 2>&1)
code=$?
expect_exit "unknown exits 1" 1 $code
expect_contains "reports unknown integration" "unknown integration" "$out"

step "B2 — missing --agent"
out=$( (cd "$PROJ" && "$SWCLI" ai add shopware-cli) 2>&1)
code=$?
expect_exit "missing agent exits 1" 1 $code
expect_contains "wording says agent, not client" "specify the target agent" "$out"

step "B3 — git skill + --global not supported"
out=$( (cd "$PROJ" && "$SWCLI" ai add deployment-helper --agent claude-code --global) 2>&1)
code=$?
expect_exit "git+global exits 1" 1 $code
expect_contains "message speaks of a project" "must be installed into a project" "$out"

# --- C1 / D2: dry-run (offline, no side effects) ---------------------------

step "C1 — bundled dry-run touches nothing"
out=$( (cd "$PROJ" && "$SWCLI" ai add shopware-cli --agent claude-code --dry-run) 2>&1)
code=$?
expect_exit "dry-run exits 0" 0 $code
expect_contains "prints dry-run line" "[dry-run] would install shopware-cli" "$out"
file_missing "no state file written by dry-run" '"name"' "$proj_state"

step "D2 — git dry-run with explicit tag (no network)"
out=$( (cd "$SHOP" && "$SWCLI" ai add deployment-helper@0.1.7 --agent claude-code --dry-run) 2>&1)
code=$?
expect_exit "git dry-run exits 0" 0 $code
expect_contains "pins the requested tag" "deployment-helper@0.1.7" "$out"

if [ "$SMOKE" -eq 1 ]; then
	step "SMOKE mode — skipping real install/remove (C2+, D3, D4, E, F)"
	printf '\n== Summary ==\n  PASS: %d  FAIL: %d\n' "$pass" "$fail"
	[ "$fail" -eq 0 ]
	exit $?
fi

# --- C. bundled skill, real install (project) ------------------------------

step "C2 — real install (project)"
out=$( (cd "$PROJ" && "$SWCLI" ai add shopware-cli --agent claude-code) 2>&1)
code=$?
echo "$out"
expect_exit "install exits 0" 0 $code
file_contains "state records agent (not client)" '"agent": "claude-code"' "$proj_state"
file_contains "state records project scope" '"scope": "project"' "$proj_state"

step "C3 — idempotency (repeat = no-op)"
before=$(shasum "$proj_state" 2>/dev/null | awk '{print $1}')
out=$( (cd "$PROJ" && "$SWCLI" ai add shopware-cli --agent claude-code) 2>&1)
code=$?
after=$(shasum "$proj_state" 2>/dev/null | awk '{print $1}')
count=$(grep -c '"name"' "$proj_state" 2>/dev/null)
expect_exit "repeat add exits 0" 0 $code
[ "$before" = "$after" ] && ok "state file unchanged" || bad "state file changed on repeat add"
[ "$count" -eq 1 ] && ok "still a single entry" || bad "entry count = $count, want 1"

step "C4 — installed filter"
out=$( (cd "$PROJ" && "$SWCLI" ai list --installed 2>&1))
code=$?
expect_exit "list --installed exits 0" 0 $code
expect_contains "shows the recorded skill" "shopware-cli" "$out"
expect_not_contains "hides non-installed deployment-helper" "deployment-helper" "$out"

# --- D. git skill + compat-check gate (needs network) ----------------------

step "D3 — compat-check BLOCKS install on a non-Shopware project"
out=$( (cd "$PROJ" && "$SWCLI" ai add deployment-helper --agent claude-code) 2>&1)
code=$?
echo "$out"
expect_exit "incompatible install exits 1" 1 $code
expect_contains "reports incompatibility" "not compatible" "$out"
file_missing "no partial state for deployment-helper" "deployment-helper" "$proj_state"

step "D4 — compat-check PASSES on a Shopware project → installs"
out=$( (cd "$SHOP" && "$SWCLI" ai add deployment-helper --agent claude-code) 2>&1)
code=$?
echo "$out"
expect_exit "compatible install exits 0" 0 $code
file_contains "state records deployment-helper" "deployment-helper" "$shop_state"
file_contains "state records a resolved revision" '"resolvedRevision"' "$shop_state"

# --- E. global scope (sandboxed HOME) --------------------------------------

step "E1 — global install"
out=$(env HOME="$HOMEDIR" XDG_CONFIG_HOME="$HOMEDIR/.config" "$SWCLI" ai add shopware-cli --agent claude-code --global 2>&1)
code=$?
echo "$out"
expect_exit "global install exits 0" 0 $code
gstate=$(global_state)
[ -n "$gstate" ] && ok "global state file created ($gstate)" || bad "no global state file under $HOMEDIR"
file_contains "global entry has global scope" '"scope": "global"' "${gstate:-/nonexistent}"

# --- F. remove -------------------------------------------------------------

step "F1 — remove dry-run (nothing changes)"
out=$( (cd "$PROJ" && "$SWCLI" ai remove shopware-cli --agent claude-code --dry-run) 2>&1)
code=$?
expect_exit "remove dry-run exits 0" 0 $code
expect_contains "prints dry-run remove line" "[dry-run] would remove" "$out"
file_contains "state still present after dry-run" "shopware-cli" "$proj_state"

step "F2 — real remove (project)"
out=$( (cd "$PROJ" && "$SWCLI" ai remove shopware-cli --agent claude-code) 2>&1)
code=$?
echo "$out"
expect_exit "remove exits 0" 0 $code
expect_contains "prints Removed line" "Removed shopware-cli" "$out"
file_contains "state entry gone (installed empty)" '"installed": \[\]' "$proj_state"

step "F3 — remove not-recorded = safe no-op"
out=$( (cd "$PROJ" && "$SWCLI" ai remove shopware-cli --agent claude-code) 2>&1)
code=$?
expect_exit "no-op remove exits 0" 0 $code
expect_contains "reports nothing to remove" "nothing to remove" "$out"

step "F4 — remove guards"
out=$( (cd "$PROJ" && "$SWCLI" ai remove does-not-exist --agent claude-code) 2>&1)
code=$?
expect_exit "unknown remove exits 1" 1 $code
expect_contains "reports unknown integration" "unknown integration" "$out"
out=$( (cd "$PROJ" && "$SWCLI" ai remove shopware-cli) 2>&1)
code=$?
expect_exit "missing agent exits 1" 1 $code
expect_contains "wording says agent" "specify the target agent" "$out"

step "F5 — global remove"
out=$(env HOME="$HOMEDIR" XDG_CONFIG_HOME="$HOMEDIR/.config" "$SWCLI" ai remove shopware-cli --agent claude-code --global 2>&1)
code=$?
echo "$out"
expect_exit "global remove exits 0" 0 $code
gstate=$(global_state)
file_contains "global state entry gone" '"installed": \[\]' "${gstate:-/nonexistent}"

# --- summary ----------------------------------------------------------------

printf '\n== Summary ==\n  PASS: %d  FAIL: %d\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
