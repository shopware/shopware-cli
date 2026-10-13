# AI Directory — Contract v1

Frozen contract for `shopware-cli ai list` / `ai info`. Changing any field
name or enum value here is a breaking change to a public interface.

## Scope of v1
- Only `type: skill` entries are listed. `mcp` (shopware-core) is reserved for a later increment.
- The directory has no remote source: entries are hardwired in Go
  (`integrations.go`). `ai list` / `ai info` make no network calls.
- `ai list --installed` is not part of this catalog contract: it reports what
  skills.sh has installed (via `skills list --json`), not the directory.

## Enums (values may be ADDED later, never renamed/removed)

| Field           | v1 values                          | reserved for later |
|-----------------|------------------------------------|--------------------|
| type            | skill                              | mcp                |
| delivery.kind   | git                                | project            |
| status          | active, deprecated                 | —                  |

## Entry fields (defined in Go)
Required:  name, displayName, type, provider, description, status,
           delivery (+ delivery.kind), documentation
Conditional: delivery.repository ("owner/repo") — required when delivery.kind == git

`name` is lowercase, hyphenated, and unique across the directory.

## JSON output (camelCase) — public contract

Selected with `--format json` on `ai list` and `ai info` (default `--format
table`), per the CLI-wide output-flag convention. `ai add` / `ai remove` print a
human line only.

`ai list` — array of:
{ name, displayName, type, provider, description, status }

`ai info <name>` — object (superset of list):
{ name, displayName, type, provider, description, status,
  documentation,
  delivery: { kind, repository? } }

`ai list --installed` is reported straight from skills.sh (`skills list --json`),
filtered to the directory's skills, and is not frozen by this contract:
{ name, scope, agents: [ ... ], source }

## Guarantees
- All human + JSON output → stdout; logs/errors → stderr.
- Unknown name → clear stderr message, exit code 1.
- No network calls in the ai list / ai info code path (`--installed` excepted).
