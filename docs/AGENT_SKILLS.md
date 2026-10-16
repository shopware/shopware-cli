# Agent Skills

Shopware CLI publishes Agent Skills for AI coding tools.

The canonical skills live in this repository so that changes to Shopware CLI and
changes to its agent guidance can be reviewed and released together.

## Repository structure

```text
shopware-cli/
├── AGENTS.md
├── docs/
│   └── AGENT_SKILLS.md
└── skills/
    ├── shopware-cli/
    │   └── SKILL.md
    ├── shopware-cli-docker/
    │   └── SKILL.md
    └── shopware-cli-extension-store/
        └── SKILL.md
```

Each skill is a single `SKILL.md`, optionally with a `scripts/` directory for
helper scripts the agent executes (the Agent Skills layout). Keep reference
material inside `SKILL.md` itself.

Do not maintain separate Claude, Cursor, Codex, Copilot, or other
client-specific copies in this repository.

## Available skills

### `shopware-cli`

General Shopware CLI guidance.

It teaches agents how to:

- discover the current CLI command surface;
- prefer Shopware CLI abstractions over lower-level tooling;
- distinguish project, extension, and account workflows;
- reason about command safety and side effects;
- work non-interactively;
- troubleshoot CLI failures.

### `shopware-cli-docker`

Guidance for Shopware CLI projects using Docker.

It teaches agents to let Shopware CLI resolve the project environment instead
of defaulting to raw Docker commands.

In particular, Symfony Console commands should normally go through:

```bash
shopware-cli project console <command>
```

and development environment lifecycle through:

```bash
shopware-cli project dev
shopware-cli project dev start
shopware-cli project dev status
shopware-cli project dev stop
```

### `shopware-cli-extension-store`

Read-only Shopware Store readiness assessment for an extension.

It teaches agents to:

- collect evidence by calling the CLI directly — two `extension validate` runs
  (normal and `--store-compliance`) plus reading the extension's metadata and icon;
- classify every finding against a fixed table with a re-checkable source;
- keep local file state separate from the remote Store listing, which it never
  inspects;
- never modify files.

## Source of truth

The files under `skills/` are the canonical source.

Do not duplicate their content in generated client-specific files.

The skills should contain durable workflow knowledge rather than an exhaustive
copy of the command reference. Agents are instructed to inspect the current
CLI with `--help`, which reduces the amount of guidance that needs updating
when commands or flags are added.

## Pull request checklist

For user-facing Shopware CLI changes:

1. Implement the CLI change.
2. Check whether `skills/shopware-cli/SKILL.md` is affected.
3. Check whether `skills/shopware-cli-docker/SKILL.md` or `skills/shopware-cli-extension-store/SKILL.md` is affected.
4. Update the skill in the same PR when required.
5. Validate the skills.
6. Verify that the skills can still be discovered by the skills CLI.

Consider adding the following item to the repository PR template:

```text
- [ ] I reviewed `skills/` for user-facing CLI changes.
```

## Validation

Validate each skill against the Agent Skills format:

```bash
skills-ref validate ./skills/shopware-cli
skills-ref validate ./skills/shopware-cli-docker
skills-ref validate ./skills/shopware-cli-extension-store
```

Verify repository discovery:

```bash
npx skills add . --list
```

CI should run these checks so an invalid skill cannot be merged.

Where practical, CI should also test commands explicitly referenced by the
skills, such as:

```text
project console
project dev
project dev start
project dev status
project dev stop
project dump
```

This catches obvious drift when commands are renamed or removed.

Semantic changes still require human review.

## Distribution

The canonical Agent Skills are maintained under `skills/` in this repository.

They are distributed through the Agent Skills ecosystem directly from the
`shopware/shopware-cli` GitHub repository. Do not maintain client-specific
copies of the skills.

After merging changes to the default branch, verify public discovery:

```bash
npx skills add shopware/shopware-cli --list
```

The `skills` CLI is responsible for installing the canonical skills for
supported AI clients. Shopware CLI does not maintain client-specific copies.

## Installing with Shopware CLI

Shopware CLI exposes an `ai` command group as a thin front door for installing
integrations: it runs skills.sh for skills and writes the agent's MCP config
itself for MCP servers. Installing a skill requires Node.js/npx on PATH
(git-delivered integrations also need git and network access).

```bash
# discover integrations
shopware-cli ai list
shopware-cli ai info deployment-helper

# install into an agent for the current Shopware project (pin with @<tag>)
shopware-cli ai add shopware-cli --agent claude-code
shopware-cli ai add deployment-helper@0.1.7 --agent claude-code

# install at the user level, from anywhere
shopware-cli ai add shopware-cli --agent claude-code --global

# preview, list what the CLI installed, and remove
shopware-cli ai add shopware-cli --agent claude-code --dry-run
shopware-cli ai list --installed
shopware-cli ai remove shopware-cli --agent claude-code
```

A project install resolves the Shopware project root (so the skill is written
where agents read it). "Project" here means a Shopware shop (a `bin/console` plus
a `composer.json` requiring `shopware/core`); a project install refuses to run
anywhere else. An extension (plugin or app) repository is not a shop, so install
the general `shopware-cli` skill with `--global` there — which is the natural scope
for it anyway, making it available in every repository. The agent name is whatever
skills.sh supports (e.g. `claude-code`, `codex`); the CLI keeps no record of its
own — skills.sh is the source of truth for what is installed. Commit
`.claude/skills/` (or the agent's directory) and `skills-lock.json` to share an
integration with your team, or gitignore them to keep it local.

### MCP integrations

An integration of type `mcp` (for example `meteor`, the Meteor MCP server) is a
remote server rather than a skill. The CLI configures it by editing the agent's
own MCP config file directly — no external tooling — so `add` and `remove` target
exactly the named server. A remote server is not tied to a shop, so an MCP install
is always at user level (global) and takes no `@<tag>`.

```bash
shopware-cli ai info meteor
shopware-cli ai add meteor --agent claude-code          # always global
shopware-cli ai add meteor --agent claude-code --dry-run
shopware-cli ai remove meteor --agent claude-code
```

Supported agents are the ones Meteor documents, each written to its own config
file:

| Agent            | Config file                                            |
| ---------------- | ------------------------------------------------------ |
| `claude-code`    | `~/.claude.json`                                       |
| `cursor`         | `~/.cursor/mcp.json`                                    |
| `vscode`         | `<user config dir>/Code/User/mcp.json`                 |
| `claude-desktop` | `<user config dir>/Claude/claude_desktop_config.json`  |

`add` writes a remote `{"type":"http","url":…}` entry (VS Code uses the `servers`
key; the others use `mcpServers`). Claude Desktop has no native remote transport,
so the CLI writes an `npx mcp-remote <url>` bridge for it. Other keys in the file
are preserved, and `remove` deletes only the named server. `ai list --installed`
reads these files back and reports configured MCP servers alongside skills, with a
`Type` column.

### Known limitations

- `ai remove` asks skills.sh whether the skill is installed before deleting it, so
  it reports `Removed` only when there was something to remove. The Codex agent can
  still leave files behind on a project-scope `ai remove` (a skills.sh defect); the
  CLI reports the removal that skills.sh carried out.

## Updating installed skills

The canonical source changes whenever `skills/` changes on the default branch.

Users can check installed skills for available updates with:

```bash
npx skills check
```

Update the Shopware skills with:

```bash
npx skills update shopware-cli shopware-cli-docker shopware-cli-extension-store
```

or update all installed project skills with:

```bash
npx skills update -y
```

Updating the Shopware CLI binary does not automatically modify skills installed
by external Agent Skills tooling.

## Keeping the skills current

Agent Skills are part of the user-facing Shopware CLI contract.

Every pull request that changes user-facing CLI behavior must review `skills/`
for impact.

Review the skills when changing:

- command names or hierarchy;
- important execution modes;
- environment or Docker behavior;
- recommended workflows;
- destructive or state-changing behavior;
- commands explicitly referenced in a skill.

Do not turn the skills into a duplicate command reference. The running CLI and
its `--help` output remain authoritative for exact commands, flags, and
version-specific behavior.

Where possible, CI should verify that commands explicitly referenced by the
skills continue to exist.

## Versioning

Skills do not have an independent Shopware version.

Because their canonical source lives in the Shopware CLI repository, every Git
tag and Shopware CLI release records the exact skill source that existed for
that release.

The default branch contains the latest maintained guidance.

## Future signed distribution

If Shopware later requires cryptographically verified, pinned, or offline skill
distribution, the canonical `skills/` directory may additionally be published
as a signed OCI artifact.

This must remain a second distribution of the same canonical source, not a
separate copy of the skills.

Until such a requirement exists, the `skills` CLI and skills.sh ecosystem are the preferred cross-client distribution and update mechanism.
