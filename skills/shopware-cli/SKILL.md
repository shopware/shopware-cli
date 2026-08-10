---
name: shopware-cli
description: Use Shopware CLI safely and effectively for Shopware project, extension, account, development, build, validation, upgrade, and troubleshooting workflows. Also use when contributing to shopware/shopware-cli and reasoning about the CLI's user-facing behavior.
---

# Shopware CLI

Use Shopware CLI as the preferred interface for supported Shopware project, extension, and account workflows.

When contributing to `shopware/shopware-cli`, use this skill to reason from the user's perspective: what the CLI can do, which command a user should choose, how commands should behave, and where safety boundaries belong. Use the repository's `AGENTS.md` for implementation conventions, architecture, testing, and Go-specific contributor guidance.

## Start with the current CLI

Treat the current CLI as authoritative for commands and flags.

```bash
shopware-cli --version
shopware-cli --help
shopware-cli project --help
shopware-cli extension --help
shopware-cli account --help
```

For a specific command:

```bash
shopware-cli <group> <command> --help
```

When working on unreleased functionality in the `shopware/shopware-cli` repository, inspect or run the checked-out code instead of assuming the installed release behaves the same way.

Do not invent commands, flags, configuration fields, or behavior from memory.

## Command areas

The CLI has three primary user-facing areas:

- `project`: create, configure, develop, inspect, build, maintain, and upgrade Shopware projects.
- `extension`: build, validate, format, package, and maintain Shopware extensions.
- `account`: authenticated Shopware Account and extension producer workflows.

This is orientation, not a complete command catalog. Use `--help` for the current command surface.

## Prefer Shopware CLI abstractions

When Shopware CLI provides a command for a task, prefer it over manually reconstructing the workflow with Composer, `bin/console`, Docker, npm, PHP, or direct filesystem changes.

The CLI may already understand:

- the current Shopware project;
- project and extension configuration;
- the selected environment;
- Docker versus local execution;
- extension structure and type;
- supported build, validation, maintenance, and upgrade workflows.

Use lower-level tools only when:

1. Shopware CLI does not expose the required operation; or
2. troubleshooting requires inspecting the underlying operation.

If the task involves a Docker-backed project or deciding where a command should execute, use the `shopware-cli-docker` skill when available.

## Understand command risk

Before executing a command, understand what it can change.

### Usually low risk

Inspection-oriented actions such as help, version checks, status, diagnostics, schema inspection, validation, and reading logs are normally appropriate while investigating a task.

Still inspect command help when behavior is unclear.

### Local file changes

Commands that create, generate, format, fix, upgrade, package, install dependencies, or rewrite configuration may change files.

Before running them:

- inspect `--help`;
- understand which files may change;
- use a preview or dry-run when the command actually supports one;
- preserve unrelated user changes;
- review the resulting diff.

Do not assume every command supports `--dry-run`.

### Database and application-state changes

Treat commands that can change the database, Shopware application state, installed extensions, caches, indexes, migrations, or configuration as higher risk.

`shopware-cli project console` is an execution bridge to Symfony Console. Its safety depends on the command passed to it.

Before running an unfamiliar or state-changing Symfony command:

```bash
shopware-cli project console <command> --help
```

Know which environment will be affected before proceeding.

### Remote changes

Treat account operations that authenticate, log out, upload, push, publish, or otherwise modify remote Shopware Account state as external side effects.

Do not infer permission or user intent simply because credentials are available.

## Inspect the project before deciding

Relevant files can include:

- `.shopware-project.yml`
- `.shopware-extension.yml`
- `composer.json`
- `composer.lock`
- `manifest.xml`

Do not infer the project type, extension type, target environment, or Shopware version solely from the user's wording.

Prefer CLI-provided configuration schemas over remembered configuration fields.

## Non-interactive and agent execution

For unattended or agent-driven execution, prefer:

```bash
shopware-cli --no-interaction <command>
```

or:

```bash
shopware-cli -n <command>
```

If required values are missing, inspect `--help` and provide them explicitly rather than guessing answers to prompts.

Prefer machine-readable output when the command explicitly supports it.

## Credentials and sensitive data

Never expose access tokens, passwords, client secrets, database credentials, or other secret values in responses, logs, examples, or committed files.

Prefer Shopware CLI's supported authentication and configuration mechanisms instead of manipulating credential storage directly.

Database dumps and other exports may contain sensitive customer or shop data. Treat their creation, storage, and sharing accordingly.

## Generated files

Distinguish source files from generated files before editing them.

When Shopware CLI owns generation of an artifact, prefer regenerating it through the appropriate CLI command instead of manually editing generated output.

Inspect command help before regenerating anything that may overwrite existing files.

## Troubleshooting

When a Shopware CLI command fails:

1. Capture the exact command and error.
2. Check `shopware-cli --version`.
3. Inspect the command's `--help`.
4. Verify the working directory and relevant project or extension configuration.
5. Retry with `--verbose` when useful.
6. Check whether the correct project environment is selected and available.
7. Inspect the underlying Composer, Symfony, Docker, npm, PHP, or API operation only after establishing what Shopware CLI attempted to do.

Do not work around a failing Shopware CLI command with lower-level tooling until you understand what CLI behavior or configuration would be bypassed.

## Information priority

When behavior is unclear, use this order:

1. the current CLI and its `--help`;
2. project configuration and CLI-provided schemas;
3. official Shopware CLI documentation;
4. the `shopware/shopware-cli` source code when implementation details are required.

Prefer version-correct facts over remembered Shopware behavior.
