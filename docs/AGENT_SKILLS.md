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

Shopware CLI exposes an `ai` command group as a thin front door over skills.sh —
it decides what to install and records it, while skills.sh writes the agent
configuration. It requires Node.js/npx on PATH (git-delivered integrations also
need git and network access).

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
where agents read it) and records the install under `.shopware-cli/ai/`. "Project"
here means a Shopware shop (a `bin/console` plus a `composer.json` requiring
`shopware/core`); a project install refuses to run anywhere else. An extension
(plugin or app) repository is not a shop, so install the general `shopware-cli`
skill with `--global` there — which is the natural scope for it anyway, making it
available in every repository. The agent name is whatever skills.sh supports (e.g.
`claude-code`, `codex`); the CLI keeps no list of its own. Commit `.claude/skills/`
(or the agent's directory), `skills-lock.json`, and
`.shopware-cli/ai/installed.json` to share an integration with your team, or
gitignore them to keep it local.

### Known limitations

- The result line describes the CLI's own record, not what skills.sh did on disk:
  a re-install of a manually deleted skill still reads `Already installed`, and
  `ai remove` reports `Removed` for a recorded install even if skills.sh found
  nothing to delete.
- The Codex agent can leave files behind on a project-scope `ai remove` (a
  skills.sh defect); the CLI still reports the removal.

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
