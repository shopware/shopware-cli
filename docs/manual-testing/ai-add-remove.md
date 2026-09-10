# Manual test plan — `ai add` / `ai remove`

Manual end-to-end checklist for the `shopware-cli ai` install/remove commands.
Global-scope tests run with an overridden `HOME` so the sandbox never touches
your real `~/.claude` or user config.

Each step lists: **what** it verifies, the **command**, the **expected**
result, and where noted a **verify** command.

## Prerequisites

Build once. `npx`/Node.js must be on PATH (skills.sh runs through npx).

```bash
cd /your/work/directory/shopware-cli && go build -o /tmp/swcli .
```

```bash
rm -rf /tmp/ai-proj /tmp/ai-shopware /tmp/ai-home && mkdir -p /tmp/ai-proj /tmp/ai-shopware /tmp/ai-home
```

Make `/tmp/ai-shopware` look like a Shopware project so the deployment-helper
compatibility check passes:

```bash
printf '{"require":{"shopware/core":"^6.6"}}\n' > /tmp/ai-shopware/composer.json
```

> The `ai` group is visible in `shopware-cli --help`.

---

## A. Directory (read-only, no side effects)

### A1 — list (table)
```bash
/tmp/swcli ai list
```
Table with columns `Name | Type | Provider | Status | Description`; rows
`shopware-cli`, `shopware-cli-docker`, `deployment-helper`; all `Type=skill`,
`Status=active`. Output on stdout, exit 0.

### A2 — list (JSON contract)
```bash
/tmp/swcli ai list --format json
```
JSON array; each item exactly `{name, displayName, type, provider, description,
status}` (camelCase, no extra fields).

### A3 — type filter reserved but empty
```bash
/tmp/swcli ai list --type mcp
```
Empty result (empty table / `[]`), exit 0 — reserved type, not an error.

### A4 — invalid type rejected
```bash
/tmp/swcli ai list --type bogus; echo "exit=$?"
```
Error on stderr, `exit=1`.

### A5 — info superset
```bash
/tmp/swcli ai info deployment-helper --format json
```
Object superset of the list shape, incl. `documentation`,
`delivery:{kind:"git", repository:"…deployment-helper"}`,
`compatibility:{source:"owner"}`.

---

## B. `ai add` — input guards (no install)

### B1 — unknown integration
```bash
cd /tmp/ai-proj && /tmp/swcli ai add does-not-exist --agent claude-code; echo "exit=$?"
```
`unknown integration "does-not-exist" (see 'shopware-cli ai list')` on stderr,
`exit=1`.

### B2 — missing `--agent`
```bash
cd /tmp/ai-proj && /tmp/swcli ai add shopware-cli; echo "exit=$?"
```
`specify the target agent with --agent (e.g. --agent claude-code)`, `exit=1`.
(Confirms the wording says **agent**, not client.)

### B3 — git skill + `--global` not supported
```bash
cd /tmp/ai-proj && /tmp/swcli ai add deployment-helper --agent claude-code --global; echo "exit=$?"
```
`"deployment-helper" must be installed into a project, not globally: it checks
compatibility against that project (omit --global)`, `exit=1`.

---

## C. `ai add` — bundled skill, project scope

### C1 — dry-run shows exact argv, touches nothing
```bash
cd /tmp/ai-proj && /tmp/swcli ai add shopware-cli --agent claude-code --dry-run
```
`[dry-run] would install shopware-cli for claude-code (project):` followed by
`npx --yes skills@1.5.18 add shopware/shopware-cli… --skill shopware-cli --agent
claude-code -y`. No `.claude/` and no `.shopware-cli/` created.

Verify nothing was written:
```bash
ls -la /tmp/ai-proj
```

### C2 — real install (project)
```bash
cd /tmp/ai-proj && /tmp/swcli ai add shopware-cli --agent claude-code
```
skills.sh output streamed to stderr; final line `Installed shopware-cli for
claude-code (project)…` on stdout. Skill lands in
`/tmp/ai-proj/.claude/skills/…`; state written.

Verify state (JSON uses `agent`, not `client`):
```bash
cat /tmp/ai-proj/.shopware-cli/ai/installed.json
```
Expected entry: `{"name":"shopware-cli","agent":"claude-code","scope":"project",…}`.

### C3 — idempotency (repeat = no-op)
```bash
cd /tmp/ai-proj && /tmp/swcli ai add shopware-cli --agent claude-code
```
Prints the same `Installed …` line, but skills.sh is NOT re-run (no
npx/download output this time). State file unchanged (single entry).

### C4 — installed filter (merges scopes)
```bash
cd /tmp/ai-proj && /tmp/swcli ai list --installed
```
Only `shopware-cli` listed (the one recorded), not the full directory.

---

## D. `ai add` — git skill + owner compat-check gate

### D2 — offline dry-run with explicit tag (no network for tag resolution)
```bash
cd /tmp/ai-shopware && /tmp/swcli ai add deployment-helper@0.1.7 --agent claude-code --dry-run
```
`[dry-run] would install deployment-helper …` with source
`shopware/deployment-helper@0.1.7`. No tag lookup, no compat-check, nothing
written.

### D3 — compat-check BLOCKS install on a non-Shopware project
```bash
cd /tmp/ai-proj && /tmp/swcli ai add deployment-helper --agent claude-code; echo "exit=$?"
```
Resolves latest tag, fetches + runs the owner compat-check, which fails (no
`shopware/core`); report on stderr; `… is not compatible with this project …`;
no install, no state written, `exit=1`.

Verify no partial install:
```bash
cat /tmp/ai-proj/.shopware-cli/ai/installed.json
```
Still only the `shopware-cli` entry (deployment-helper absent).

### D4 — compat-check PASSES on a Shopware project → installs
```bash
cd /tmp/ai-shopware && /tmp/swcli ai add deployment-helper --agent claude-code
```
Compat-check passes, skills.sh installs, `Installed deployment-helper for
claude-code (project) @<tag>`. State records the resolved revision.

Verify resolved revision recorded:
```bash
cat /tmp/ai-shopware/.shopware-cli/ai/installed.json
```

---

## E. `ai add --global` (sandboxed HOME)

> All global commands prefix `HOME=/tmp/ai-home XDG_CONFIG_HOME=/tmp/ai-home/.config`
> so both the install state and the skills.sh global config (`$HOME/.claude`)
> stay in the sandbox. The state file lives at `shopware-cli/ai/installed.json`
> inside the OS config dir (macOS: `Library/Application Support`; Linux:
> `$XDG_CONFIG_HOME`), so locate it with `find` rather than a fixed path.

### E1 — global install
```bash
HOME=/tmp/ai-home XDG_CONFIG_HOME=/tmp/ai-home/.config /tmp/swcli ai add shopware-cli --agent claude-code --global
```
`Installed shopware-cli for claude-code (global)…`, with `"scope":"global"`
recorded.

Verify (platform-neutral lookup):
```bash
find /tmp/ai-home -path '*shopware-cli/ai/installed.json' -exec cat {} +
```

---

## F. `ai remove`

### F1 — dry-run (nothing changes)
```bash
cd /tmp/ai-proj && /tmp/swcli ai remove shopware-cli --agent claude-code --dry-run
```
`[dry-run] would remove shopware-cli for claude-code (project): npx --yes
skills@1.5.18 remove shopware-cli --agent claude-code -y`. State + skill still
present.

### F2 — real remove (project)
```bash
cd /tmp/ai-proj && /tmp/swcli ai remove shopware-cli --agent claude-code
```
Runs `skills … remove …`; skills.sh reports success on stderr; `Removed
shopware-cli for claude-code (project):` on stdout. Skill gone from
`.claude/skills`; state entry removed.

Verify state empty:
```bash
cat /tmp/ai-proj/.shopware-cli/ai/installed.json
```
Expected: `"installed": []`.

### F3 — remove not-recorded = safe no-op
```bash
cd /tmp/ai-proj && /tmp/swcli ai remove shopware-cli --agent claude-code; echo "exit=$?"
```
`shopware-cli is not recorded for claude-code (project) by shopware-cli; nothing
to remove`, `exit=0`, skills.sh not invoked (we don't touch hand-written
config).

### F4 — remove guards
```bash
cd /tmp/ai-proj && /tmp/swcli ai remove does-not-exist --agent claude-code; /tmp/swcli ai remove shopware-cli
```
First → `unknown integration`; second → `specify the target agent with
--agent`. Both exit 1.

### F5 — global remove
```bash
HOME=/tmp/ai-home XDG_CONFIG_HOME=/tmp/ai-home/.config /tmp/swcli ai remove shopware-cli --agent claude-code --global
```
`Removed … (global)`; global state entry gone.

---

## Cleanup

Removes all test artifacts. Your real `~/.claude` is untouched because global
tests used `HOME=/tmp/ai-home`.

```bash
rm -rf /tmp/ai-proj /tmp/ai-shopware /tmp/ai-home /tmp/swcli
```

---

## Notes

- **D1** is intentionally skipped in the numbering — the latest-tag resolution
  (`git ls-remote`) is covered in practice by **D4** (add without `@tag`), so
  there is no separate step just for the lookup.
- A bundled skill built via `go build` without ldflags has an empty `Version`,
  so the dry-run shows `shopware/shopware-cli` with no `@ref` — that is correct.
  To test version pinning for a bundled skill, use `shopware-cli@<tag>`.
