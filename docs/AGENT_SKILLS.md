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
    └── shopware-cli-docker/
        └── SKILL.md
```

Each skill is intentionally self-contained in a single `SKILL.md`.

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
3. Check whether `skills/shopware-cli-docker/SKILL.md` is affected.
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

## Updating installed skills

The canonical source changes whenever `skills/` changes on the default branch.

Users can check installed skills for available updates with:

```bash
npx skills check
```

Update the Shopware skills with:

```bash
npx skills update shopware-cli shopware-cli-docker
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
