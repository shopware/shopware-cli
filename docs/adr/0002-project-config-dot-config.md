# 2. Shopware CLI configuration under `.config/`

- Status: Proposed
- Date: 2026-08-18
- Related: [Issue #1387](https://github.com/shopware/shopware-cli/issues/1387)

## Context

Shopware CLI stores two kinds of checkout-level YAML configuration as root-level dotfiles:

| Kind | Current files | Written by | Missing file |
| --- | --- | --- | --- |
| Project (CLI + Deployment Helper) | `.shopware-project.yml`, `.shopware-project.yaml` | `project config init`, `project create`, TUI | some commands fall back; others fail |
| Extension | `.shopware-extension.yml`, `.shopware-extension.yaml` | `extension config init` | silently falls back to an empty config |

Project config also supports a sibling local override (`.shopware-project.local.yml` / `.yaml`) that is deep-merged at read time. `--project-config` selects an explicit project-config path. Extension config has no local-override file and no equivalent flag; it is always discovered relative to the extension directory.

This works, but it adds more root-level dotfiles. [The `.config` convention](https://dot-config.github.io/) lets tools move those files under `.config/` without claiming the project root.

Issue #1387 originally sketched a vendor directory (`.config/shopware/project.yaml` and `lsp.json`). This ADR instead keeps the existing filenames and moves them one level down: `.config/shopware-project.yml` and `.config/shopware-extension.yml`. That is a smaller rename, matches the files people already know, and still clears the repository root.

Extension config belongs in the same move. It is the other first-party CLI YAML file, it has the same discovery and init pattern, and leaving it at the root would keep the clutter the issue is trying to remove.

LSP / agent configuration remains a separate file chosen by shopware-lsp. The CLI does not read it.

This ADR applies to Shopware CLI and the project-config contract it shares with Deployment Helper. It does not define a company-wide location for every Shopware tool.

## Decision

Adopt `.config/shopware-project.yml` and `.config/shopware-extension.yml` as the preferred checkout configuration paths. Keep the current YAML schemas and merge rules. Treat the root-level `.shopware-project.*` and `.shopware-extension.*` files as legacy locations during a deprecation period.

### Preferred layout

```text
.config/shopware-project.yml
.config/shopware-extension.yml
```

In a Shopware project that also contains extensions, the files live next to the checkout they describe:

```text
.config/shopware-project.yml
custom/plugins/MyPlugin/.config/shopware-extension.yml
```

A standalone extension repository uses the same relative path at its own root: `.config/shopware-extension.yml`.

| Role | Preferred path | Legacy paths still read |
| --- | --- | --- |
| Project config | `.config/shopware-project.yml` | `.shopware-project.yml`, `.shopware-project.yaml` |
| Project local override (derived from the resolved base file) | `.config/shopware-project.local.yml` | `.shopware-project.local.yml`, `.shopware-project.local.yaml` |
| Extension config | `.config/shopware-extension.yml` | `.shopware-extension.yml`, `.shopware-extension.yaml` |
| LSP / agent config | chosen by shopware-lsp (for example `.config/shopware-lsp.json`) | out of scope for the CLI |

New locations use the `.yml` suffix only. We will not also invent `.config/shopware-project.yaml` or `.config/shopware-extension.yaml`.

### Shared discovery rules

For each config kind, discovery runs against the relevant root (process working directory for project config when `--project-config` is unset; the extension directory for extension config).

Precedence for project config:

1. `.config/shopware-project.yml` if the file exists
2. `.shopware-project.yaml` if the file exists
3. `.shopware-project.yml` if the file exists

Precedence for extension config:

1. `.config/shopware-extension.yml` if the file exists
2. `.shopware-extension.yml` if the file exists
3. `.shopware-extension.yaml` if the file exists

Legacy order stays as it is today for each family (project prefers `.yaml` then `.yml`; extension prefers `.yml` then `.yaml`).

If both the preferred file and a legacy file of the same kind exist, the preferred file wins and the CLI emits a warning that names both paths and tells the user the legacy file is ignored for this run.

If none of the files exist, the CLI behaves as it does today: project commands that allow a missing config continue with an empty fallback; project commands that require a config fail and point the user at `shopware-cli project config init`; extension commands keep the current empty-config fallback.

`--project-config <path>` always wins over project-config discovery, including when a preferred file and a legacy file both exist. Environment variables that already override *values* inside the project file (`SHOPWARE_CLI_API_*` and similar) are unchanged. This ADR does not add a new environment variable for either file path, and it does not add an `--extension-config` flag.

`--project-config` must not bake a discovered filename in as the Cobra flag default at process start. Today `DefaultConfigFileName()` is evaluated when flags are registered and only distinguishes the two legacy project names. After this change, an unset flag means "discover"; only an explicit flag value is treated as a fixed path.

### Writing configuration

New files are written to the preferred path for that kind:

- `shopware-cli project config init`
- `shopware-cli project create`
- first write of a newly created project from the TUI
- `shopware-cli extension config init` when no extension config exists yet

The CLI creates `.config/` as needed.

Updates and forced overwrites write back to the file that was actually resolved. The CLI does not silently copy or move a legacy file to the preferred path on save. Doing so would leave two files on disk and, on the next run, ignore the file the user just edited.

`extension config init --force` therefore overwrites the existing resolved file. It does not create `.config/shopware-extension.yml` beside a leftover `.shopware-extension.yml`.

Project local override writes follow the same rule: the override path is derived from the resolved base file by inserting `.local` before the extension (the existing `localConfigFileName` behavior). A preferred base file therefore uses `.config/shopware-project.local.yml`. A legacy base file keeps `.shopware-project.local.yml` / `.yaml`. The CLI does not merge a preferred base file with a legacy local file, or the reverse. Extension config has no local-override file today and does not gain one here.

### Deployment Helper

Deployment Helper consumes the same `deployment:` block from the *project* config and must use the same project-config discovery and precedence rules, including the both-files warning. The lookup contract belongs with this ADR so the two tools do not diverge. The implementation in Deployment Helper is a follow-up in that repository. Deployment Helper does not read extension config.

### Migration

There is no breaking cut-over and no removal date in this ADR.

The documented migration for an existing checkout is:

1. Create `.config/` if needed.
2. Move `.shopware-project.yml` or `.shopware-project.yaml` to `.config/shopware-project.yml`.
3. If a sibling `.shopware-project.local.yml` / `.yaml` exists, move it to `.config/shopware-project.local.yml`.
4. Move `.shopware-extension.yml` or `.shopware-extension.yaml` to `.config/shopware-extension.yml`.
5. Commit the preferred files (not the local override).
6. Delete leftover root-level files so the both-files warning goes away.

Do this in the project root for project config, and in each extension root for extension config.

`project config init` and `extension config init` are not migrators. They only create a new preferred file when none of the discovered locations for that kind exist.

## Consequences

### Compatibility

Existing projects and extensions keep working without changes. Users who already pass `--project-config` keep that path. CI jobs and scripts that assume the default write location will see `.config/shopware-project.yml` or `.config/shopware-extension.yml` for newly created files.

### Documentation and messaging

Help text, doctor output, error messages, skills, and architecture notes should name the preferred paths and mention that the legacy root-level files still work. The JSON schemas themselves do not change; only the published description of where the files live does.

### Implementation surface in this repository

Project-config changes are concentrated in `internal/shop` (`DefaultConfigFileName` / discovery, `WriteConfig`, `WriteLocalConfig`) and callers that hard-code `.shopware-project.yml` (`cmd/project`, TUI, verifier project loading).

Extension-config changes are concentrated in `internal/extension` (`ConfigPath`, `InitConfig`, `readExtensionConfig`) and help text in `cmd/extension`.

Tests should cover, for each kind:

- preferred file is discovered
- each legacy file is still discovered
- preferred file wins when both exist, with a warning
- init writes the preferred path when no file exists
- a missing config still falls back or errors as today

Additionally for project config:

- `--project-config` wins over both
- local override is derived from the resolved base file

Additionally for extension config:

- `--force` overwrites the resolved existing file and does not create a second location

### Out of scope

- global CLI config (`.shopware-cli.yaml`)
- reading or writing shopware-lsp configuration
- a local-override file for extension config
- automatic rewriting of existing repositories
- a hard deadline for removing legacy paths
- a company-wide mandate that every Shopware tool adopt these filenames
