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
    ├── shopware-cli-extension-store/
    │   └── SKILL.md
    ├── shopware-cli-extension-anonymize/
    │   └── SKILL.md
    └── shopware-contribute-start/      (also -submit, -verify)
        ├── SKILL.md
        └── evals/
            ├── scenarios.json
            └── reports/0.0.1.md
```

Each skill is a single `SKILL.md`, optionally with a `scripts/` directory for
helper scripts the agent executes (the Agent Skills layout). Keep reference
material inside `SKILL.md` itself. Contributor skills add an evaluation suite
and a report per version (see [Contributor skills](#contributor-skills)).

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
  (normal and with `validation.store_compliance: true` in the extension
  configuration) plus reading the extension's metadata and icon;
- classify every finding against a fixed table with a re-checkable source;
- keep local file state separate from the remote Store listing, which it never
  inspects;
- never modify files.

### `shopware-cli-extension-anonymize`

Bootstrap the `anonymize` section of an extension's
`.config/shopware-extension.yml`.

It teaches agents to:

- edit only the config file the CLI actually loads;
- take `system_config` keys from `config.xml` and SystemConfig calls;
- take table and column names from entity storage names and migrations;
- choose faker expressions for personal data and `''` or `NULL` for secrets;
- omit a `system_config` row, or replace it with a custom value or a faker
  value;
- leave Shopware core anonymization and unrelated extension validation alone.

### Contributor skills

Three skills help people contribute to Shopware repositories
(`shopware/shopware`, `shopware/shopware-cli`, `shopware/deployment-helper`,
and others that follow the same pattern):

- `shopware-contribute-start`: where to discuss an idea, whether a change
  needs an issue first, setup, fork and branch;
- `shopware-contribute-submit`: actionable bug reports and feature requests,
  pull request descriptions that say which checks ran, review feedback;
- `shopware-contribute-verify`: which local checks apply, and whether a failing
  CI check is a likely regression, a known failure, an infrastructure failure or
  unknown.

They read each repository's own `AGENTS.md`, `CONTRIBUTING.md` and templates
and defer to them. In `shopware/shopware` they defer to its own skills under
`.agents/skills/`. They are a front door, not a second copy of the
contribution rules.

Install them with `--global`. A project install needs a Shopware shop, and
a clone of this repository or of Deployment Helper isn't one:

```bash
shopware-cli ai add shopware-contribute-start --agent claude-code --global
```

They are shipped as products, following
[Google's model for Agent Skills](https://cloud.google.com/blog/topics/developers-practitioners/behind-the-scenes-two-months-later-how-we-ship-google-agent-skills-like-products).
The rules apply to every skill with `metadata.version`:

- **Versioning:** `metadata.version` is a quoted SemVer string. A new skill
  starts at `0.0.1`. Any change to `SKILL.md`, `references/`, `scripts/` or
  `assets/` needs the next major, minor or patch version (lower parts reset to
  zero), or the pull request is blocked. A renamed skill counts as new and
  restarts at `0.0.1`. The version's evaluation report says what changed in its
  "Changes" section; there is no separate changelog.
- **Evaluation report:** every version has `evals/reports/<version>.md`. It
  compares the skill with the previous version, or a new skill with an agent
  without the skill, on every scenario in `evals/scenarios.json`, scored per
  criterion by a separate judge. The report records `skill_sha256`, the hash
  of the instructions it ran against; if the skill changes afterwards, the
  test fails until the evaluation is re-run. Link the report in the pull
  request description, or the pull request is blocked. A planned or unrun
  evaluation is not a pass.
- **Scenarios:** each criterion checks one thing; split compound ones. No
  duplicate prompts or criteria. Include prompts that must not trigger the
  skill.
- **Structure:** `SKILL.md` stays under 500 lines; only `SKILL.md`, `evals/`,
  `references/`, `scripts/` and `assets/` are allowed; the skill is listed in
  the AI directory and in this document. `skills-ref validate` already checks
  the name, folder and description length for every skill.
- **Links:** the Markdown lint already checks relative links on every pull
  request (MD057). External links are checked every Monday (the `links` job in
  `agent-skills.yml`); nothing else in this repository checks them.
- **Gates:** `TestVersionedSkillsFollowTheProductContract` runs in the test
  suite on every pull request; the `versions` job in `agent-skills.yml` runs
  `TestChangedSkillsBumpTheirVersion` against the pull request's base and
  description.
- **Review:** a maintainer (`CODEOWNERS`) approves every change, and a
  maintainer of each covered repository reviews content about it.
- **Freshness:** when a covered repository changes its `CONTRIBUTING.md`,
  `AGENTS.md`, templates or default branch, review the skills. The skills
  read those files at run time, so most changes need no edit.
- **Retirement:** retire a skill when the repositories' own `AGENTS.md` and
  `CONTRIBUTING.md` cover it, or usage shows nobody needs it. Mark the
  directory entry `deprecated` first, then remove it in a later release.

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
3. Changing `CONTRIBUTING.md`, `AGENTS.md` or the issue and PR templates?
   Check the `skills/shopware-contribute-*` skills.
4. Check whether `skills/shopware-cli-docker/SKILL.md`,
   `skills/shopware-cli-extension-store/SKILL.md`, or
   `skills/shopware-cli-extension-anonymize/SKILL.md` is affected.
5. Update the skill in the same PR when required.
6. Validate the skills.
7. Verify that the skills can still be discovered by the skills CLI.

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
skills-ref validate ./skills/shopware-cli-extension-anonymize
skills-ref validate ./skills/shopware-contribute-start
skills-ref validate ./skills/shopware-contribute-submit
skills-ref validate ./skills/shopware-contribute-verify
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
integrations: it runs skills.sh for skills and writes the project's `.mcp.json`
for MCP servers. Installing a skill requires Node.js/npx on PATH (git-delivered
integrations also need git and network access).

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
where agents read it). "Project" here means a Shopware shop (a `bin/console`
plus a `composer.json` requiring `shopware/core`); a project install refuses to
run anywhere else. An extension (plugin or app) repository is not a shop, so
install the general `shopware-cli` skill with `--global` there — which is the
natural scope for it anyway, making it available in every repository. The agent
name is whatever skills.sh supports (e.g. `claude-code`, `codex`); the CLI keeps
no record of its own — skills.sh is the source of truth for what is installed.
Commit `.claude/skills/` (or the agent's directory) and `skills-lock.json` to
share an integration with your team, or gitignore them to keep it local.

### MCP integrations

An integration of type `mcp` (for example `meteor`, the Meteor MCP server) is a
remote server rather than a skill. The CLI configures it by writing the
project's `.mcp.json` — the standard `{"mcpServers": {…}}` file MCP clients read
— instead of any agent's own config. Because the CLI owns that one file, `add`
and `remove` target exactly the named server, and no per-agent config paths need
to be known.

An MCP install is project-scoped: `.mcp.json` lives in the Shopware project
root, and committing it shares the server with your team. It takes no `@<tag>`
and no `--agent` (the file is not agent-specific), and `--global` is not
supported.

```bash
shopware-cli ai info meteor
shopware-cli ai add meteor               # writes <project>/.mcp.json
shopware-cli ai add meteor --dry-run
shopware-cli ai remove meteor
```

`add` writes a remote `{"type":"http","url":…}` entry under `mcpServers`,
preserving other servers and unrelated keys; `remove` deletes only the named
server. `ai list --installed` reads the project's `.mcp.json` back and reports
configured MCP servers alongside skills, with a `Type` column.

Claude Code reads `.mcp.json` from the project root natively. Clients that use a
different project path (for example Cursor's `.cursor/mcp.json` or VS Code's
`.vscode/mcp.json`) can import it; if broader coverage is needed later, the
server can be distributed with a dedicated MCP installer instead.

### Recommendation

When a supported AI client runs the CLI (detected from the `AI_AGENT`
environment variable), and the `shopware-cli` skill is not yet installed for it,
the CLI prints a short, non-blocking recommendation to stderr pointing at
`shopware-cli ai add shopware-cli --agent <client> --global`. It never installs
anything and asks the agent to confirm with the user first. The detection itself
is the trigger, so it is shown in non-interactive runs too (that is how agents
invoke the CLI). It is not shown again for the same client within 24 hours, and
stays silent in CI runs, during shell completion, while using `ai` commands, and
when `--no-ai-hint` is passed.

### Known limitations

- `ai remove` asks skills.sh whether the skill is installed before deleting it,
  so it reports `Removed` only when there was something to remove. The Codex
  agent can still leave files behind on a project-scope `ai remove` (a skills.sh
  defect); the CLI reports the removal that skills.sh carried out.

## Updating installed skills

The canonical source changes whenever `skills/` changes on the default branch.

Users can check installed skills for available updates with:

```bash
npx skills check
```

Update the Shopware skills with:

```bash
npx skills update shopware-cli shopware-cli-docker shopware-cli-extension-store shopware-cli-extension-anonymize
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

Skills do not have an independent Shopware version. Contributor skills also
carry their own `metadata.version` and a report per version, so a contributor
can tell which behaviour they installed (see
[Contributor skills](#contributor-skills)).

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

Until such a requirement exists, the `skills` CLI and skills.sh ecosystem are
the preferred cross-client distribution and update mechanism.
