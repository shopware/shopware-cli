# AI Directory — Contract v1

Frozen contract for `shopware-cli ai list` / `ai info`. Changing any field
name or enum value here is a breaking change to a public interface.

## Scope of v1
- Only `type: skill` entries are listed. `mcp` (shopware-core) is reserved for a later increment.
- No network access, no project detection in this version.
- The directory has no remote source: entries are hardwired in Go
  (`integrations.go`).

## Enums (values may be ADDED later, never renamed/removed)

| Field           | v1 values                          | reserved for later |
|-----------------|------------------------------------|--------------------|
| type            | skill                              | mcp                |
| delivery.kind   | bundled, git                       | project            |
| status          | active, deprecated                 | —                  |

## Entry fields (defined in Go)
Required:  name, displayName, type, provider, description, status,
           delivery (+ delivery.kind), documentation
Conditional: delivery.repository — required when delivery.kind == git
Optional:  compatibility (compatibility.source: owner)

`name` is lowercase, hyphenated, and unique across the directory.

## JSON output (camelCase) — public contract

Selected with `--format json` on both commands (default `--format table`), per
the CLI-wide output-flag convention.

`ai list` — array of:
{ name, displayName, type, provider, description, status }

`ai info <name>` — object (superset of list):
{ name, displayName, type, provider, description, status,
  documentation,
  delivery: { kind, repository? },
  compatibility?: { source } }

## --installed state (written by `ai add` / `ai remove`)
Install-state file records, per installed entry:
{ name, agent, scope: project|global, requestedTag, resolvedRevision }
`ai list --installed` returns only recorded entries.

## Guarantees
- All human + JSON output → stdout; logs/errors → stderr.
- Unknown name → clear stderr message, exit code 1.
- No network calls in the ai list / ai info code path.
