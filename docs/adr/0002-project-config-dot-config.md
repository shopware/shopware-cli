# 2. Project configuration under `.config/shopware/`

- Status: Proposed
- Date: 2026-08-18
- Related: [Issue #1387](https://github.com/shopware/shopware-cli/issues/1387)

## Context

Shopware CLI stores project-level settings in a YAML file at the repository root. Today that file is discovered as:

- `.shopware-project.yml` (default when creating or when no `.yaml` file is present)
- `.shopware-project.yaml` (accepted when it already exists)

`shopware-cli project config init` and `shopware-cli project create` always write `.shopware-project.yml`. A sibling override file (`.shopware-project.local.yml` / `.shopware-project.local.yaml`) is deep-merged at read time for credentials and other values that should not be committed. `--project-config` selects an explicit path and bypasses discovery.

This works, but it adds another root-level dotfile and spreads Shopware-specific configuration across ad-hoc names as more developer tools appear. [The `.config` convention](https://dot-config.github.io/) gives tools a shared, predictable namespace under `.config/<vendor>/` without claiming the entire project root.

Issue #1387 asks the CLI (and, later, Deployment Helper) to prefer:

```text
.config/shopware/
├── project.yaml
└── lsp.json
```

`project.yaml` is the CLI and Deployment Helper configuration. `lsp.json` is reserved for shopware-lsp / agent tooling and is not read by the CLI.

This ADR applies to Shopware CLI and the config contract it shares with Deployment Helper. It does not define a company-wide location for every Shopware tool. Extension configuration (`.shopware-extension.yml`) is unchanged.

## Decision

Adopt `.config/shopware/project.yaml` as the preferred project configuration path. Keep the current YAML schema, merge rules, and `--project-config` override. Treat the root-level `.shopware-project.yml` / `.shopware-project.yaml` files as legacy locations during a deprecation period.

### Preferred layout

| Role | Preferred path | Legacy paths still read |
| --- | --- | --- |
| Project config | `.config/shopware/project.yaml` | `.shopware-project.yml`, `.shopware-project.yaml` |
| Local override (derived from the resolved base file) | `.config/shopware/project.local.yaml` | `.shopware-project.local.yml`, `.shopware-project.local.yaml` |
| LSP / agent config | `.config/shopware/lsp.json` | out of scope for the CLI |

The new location uses the `.yaml` suffix only. We will not also invent `.config/shopware/project.yml`.

### Automatic discovery

Discovery runs against the current working directory when `--project-config` is not set. Precedence:

1. `.config/shopware/project.yaml` if the file exists
2. `.shopware-project.yaml` if the file exists
3. `.shopware-project.yml` if the file exists

If both the preferred file and a legacy file exist, the preferred file wins and the CLI emits a warning that names both paths and tells the user the legacy file is ignored for this run.

If none of the files exist, the CLI behaves as it does today: commands that allow a missing config continue with an empty fallback; commands that require a config fail and point the user at `shopware-cli project config init`.

`--project-config <path>` always wins over discovery, including when a preferred file and a legacy file both exist. Environment variables that already override *values* inside the file (`SHOPWARE_CLI_API_*` and similar) are unchanged. This ADR does not add a new environment variable for the file path.

`--project-config` must not bake a discovered filename in as the Cobra flag default at process start. Today `DefaultConfigFileName()` is evaluated when flags are registered and only distinguishes the two legacy names. After this change, an unset flag means "discover"; only an explicit flag value is treated as a fixed path.

### Writing configuration

New files are written to the preferred path:

- `shopware-cli project config init`
- `shopware-cli project create`
- first write of a newly created project from the TUI

The CLI creates `.config/shopware/` as needed.

Updates write back to the file that was actually resolved for that project. The CLI does not silently copy or move a legacy file to the preferred path on save. Doing so would leave two files on disk and, on the next run, ignore the file the user just edited.

Local override writes follow the same rule: the override path is derived from the resolved base file by inserting `.local` before the extension (the existing `localConfigFileName` behavior). A preferred base file therefore uses `.config/shopware/project.local.yaml`. A legacy base file keeps `.shopware-project.local.yml` / `.yaml`. The CLI does not merge a preferred base file with a legacy local file, or the reverse.

### Deployment Helper

Deployment Helper consumes the same `deployment:` block and must use the same discovery and precedence rules, including the both-files warning. The lookup contract belongs with this ADR so the two tools do not diverge. The implementation in Deployment Helper is a follow-up in that repository.

### Migration

There is no breaking cut-over and no removal date in this ADR.

The documented migration for an existing project is:

1. Create `.config/shopware/` if needed.
2. Move `.shopware-project.yml` or `.shopware-project.yaml` to `.config/shopware/project.yaml`.
3. If a sibling `.shopware-project.local.yml` / `.yaml` exists, move it to `.config/shopware/project.local.yaml`.
4. Commit the preferred file (not the local override).
5. Delete the leftover root-level file so the both-files warning goes away.

`shopware-cli project config init` is not a migrator. It only creates a new preferred file when none of the discovered locations exist, or when the user explicitly asks it to write.

## Consequences

### Compatibility

Existing projects keep working without changes. Users who already pass `--project-config` keep that path. CI jobs and scripts that assume the default write location will see `.config/shopware/project.yaml` for newly created projects.

### Documentation and messaging

Help text, doctor output, error messages, skills, and architecture notes should name the preferred path and mention that the legacy root-level files still work. The JSON schema itself does not change; only the published description of where the file lives does.

### Implementation surface in this repository

The behavior change is concentrated in `internal/shop` (`DefaultConfigFileName` / discovery, `WriteConfig`, `WriteLocalConfig`) and the callers that hard-code `.shopware-project.yml` when they should use discovery or the preferred write path (`cmd/project`, TUI, verifier project loading). Tests should cover:

- preferred file is discovered
- each legacy file is still discovered
- preferred file wins when both exist, with a warning
- `--project-config` wins over both
- init and create write the preferred path
- local override is derived from the resolved base file
- a missing config still falls back or errors as today

### Out of scope

- `.shopware-extension.yml` / `.shopware-extension.yaml`
- global CLI config (`.shopware-cli.yaml`)
- reading or writing `.config/shopware/lsp.json`
- automatic rewriting of existing repositories
- a hard deadline for removing legacy paths
- a company-wide mandate that every Shopware tool adopt `.config/shopware/`
