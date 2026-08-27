# AI Directory — Manifest Contract v1

Frozen contract for `shopware-cli ai list` / `ai info`. Changing any field
name or enum value here is a breaking change to a public interface.

## Scope of v1
- Only `type: skill` entries are listed. `mcp` (shopware-core) is deferred to #1336.
- No network access, no project detection in this version.

## Enums (all validated; values may be ADDED later, never renamed/removed)
| Field           | v1 values                          | reserved for later |
|-----------------|------------------------------------|--------------------|
| type            | skill                              | mcp                |
| delivery.kind   | bundled, git                       | project            |
| status          | active, coming-soon, deprecated    | —                  |

## Manifest entry (YAML, snake_case)
Required:  name, display_name, type, provider, description, status,
           delivery (+ delivery.kind), documentation
Conditional: delivery.repository — required when delivery.kind == git
Optional:  compatibility (compatibility.source: owner),
           internal (internal.maintainer)  ← NEVER emitted in any output

name: must match ^[a-z][a-z0-9-]*$ and be unique across the manifest.

## JSON output (camelCase) — public contract

Selected with the boolean `--json` flag on both commands (repo convention, e.g.
`project extension list`). Not `--output=json`: `--output`/`-o` already means an
output file path elsewhere in the CLI (e.g. `project sbom`).

`ai list` — array of:
{ name, displayName, type, provider, description, status, available }

`ai info <name>` — object (superset of list):
{ name, displayName, type, provider, description, status,
  available, availabilityReason?,        // ? = omitted when empty
  documentation,
  delivery: { kind, repository? },
  compatibility?: { source } }

Never present in output: internal, internal.maintainer.

## availability (computed, not stored)
- available (bool): can the entry be used in the current context.
- v1 rules: status == coming-soon → available:false (reason set);
  otherwise available:true. bundled always true; git true (install
  resolution deferred to #1337, no network here).

## --installed state (shape defined now, WRITTEN by #1337)
Install-state file records, per installed entry:
{ name, client, scope: project|global, requestedTag, resolvedRevision }
`ai list --installed` returns only recorded entries; empty until #1337.

## Guarantees
- All human + JSON output → stdout; logs/errors → stderr.
- Unknown name → clear stderr message, exit code 1.
- No network calls in the ai list / ai info code path.
