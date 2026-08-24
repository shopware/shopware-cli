---
name: shopware-cli-docker
description: Use when working on Docker-backed Shopware projects and needing to run Shopware or Symfony CLI commands, interact with project services or databases, start or stop the development environment, troubleshoot Docker execution, or determine whether a command should run on the host or inside the project container.
---

# Shopware CLI with Docker

In a Shopware CLI-managed Docker project, prefer asking Shopware CLI to perform the operation instead of manually deciding how to enter containers.

Shopware CLI knows the project configuration and selected execution environment and can route supported operations through the correct executor.

## Core rule

Do not default to:

```bash
docker compose exec ...
```

when Shopware CLI already exposes the operation.

Prefer:

```bash
shopware-cli project ...
```

The CLI should decide when Docker is required.

## Symfony and Shopware Console commands

When a task requires `bin/console`, use:

```bash
shopware-cli project console <command> [arguments]
```

Instead of manually doing something like:

```bash
docker compose exec web php bin/console <command>
```

prefer:

```bash
shopware-cli project console <command>
```

`project console` resolves the current Shopware project and its configured executor. In a Docker-backed environment, the CLI routes the Symfony command or Composer script through the project container.

This applies to Shopware and Symfony Console workflows such as:

- cache commands;
- plugin and app lifecycle;
- indexing;
- migrations;
- scheduled tasks;
- system configuration;
- other commands exposed by `bin/console`;
- custom Composer scripts from the project `composer.json`.

Before executing an unfamiliar or state-changing command:

```bash
shopware-cli project console <command> --help
```

Do not assume a Symfony command is safe merely because it exists.

## Development environment

Use Shopware CLI to manage the development environment:

```bash
shopware-cli project dev
shopware-cli project dev start
shopware-cli project dev status
shopware-cli project dev stop
```

Do not replace these with `docker compose up` or `docker compose down` unless there is a specific reason to bypass the CLI.

The CLI knows the configured environment and can prepare and manage the Docker setup required by the project.

## Other project operations

Prefer the corresponding Shopware CLI command for supported tasks such as builds, watchers, logs, validation, upgrades, project maintenance, and extension management.

Do not manually wrap a Shopware CLI command in `docker compose exec`.

If Shopware CLI already exposes the operation, let the CLI handle Docker.

## Database work

First determine what kind of database operation the user actually needs.

For database exports, inspect:

```bash
shopware-cli project dump --help
```

For database-related operations exposed through Symfony or Shopware Console, prefer:

```bash
shopware-cli project console <command>
```

For arbitrary SQL, first inspect the available Shopware CLI and Symfony commands. Do not invent a CLI command.

If no suitable Shopware CLI abstraction exists, direct database access may be necessary. Before doing that:

1. identify the configured database service and target environment;
2. prefer discovered configuration over hard-coded container names or credentials;
3. use read-only queries where possible;
4. treat writes, deletes, schema changes, imports, and bulk updates as destructive operations.

Never execute database-changing commands against an ambiguous environment.

## Composer, PHP, and npm

Prefer a higher-level Shopware CLI operation when one exists.

Do not immediately run:

```bash
docker compose exec web composer ...
docker compose exec web php ...
docker compose exec web npm ...
```

First check whether Shopware CLI already exposes the intended workflow.

If no CLI abstraction exists:

1. confirm this using the relevant command group's `--help`;
2. determine the configured Docker service and working directory;
3. run the low-level tool in the correct project environment;
4. avoid hard-coding paths and service names when they can be discovered.

Direct Docker execution is the fallback, not the default.

## Environment selection

Do not assume that an environment named `local`, `dev`, `staging`, or similar is Docker-backed or safe to modify.

Inspect `.shopware-project.yml` and use the CLI's environment selection mechanisms. Empty `-e` / `--env` targets `environments.local`. Store shop URL and Admin API credentials under `environments`, not at the top level.

Before destructive commands, establish exactly which environment will be affected.

## Starting from a user request

When a user asks for something like:

- "clear the Shopware cache";
- "run this Symfony command";
- "update the plugin";
- "run migrations";
- "check the database";
- "start the shop";
- "show me the logs";

do not translate the request directly into raw Docker commands.

First ask: **Does Shopware CLI already know how to do this?**

For Symfony commands, the answer should usually begin with:

```bash
shopware-cli project console ...
```

For development environment lifecycle, begin with:

```bash
shopware-cli project dev ...
```

For other workflows, inspect:

```bash
shopware-cli project --help
```

## Troubleshooting

When Docker-backed execution fails:

1. Run `shopware-cli project dev status`.
2. Inspect the relevant Shopware CLI command's `--help`.
3. For Shopware CLI debug output, place `--verbose` before the command group, for example `shopware-cli --verbose project console <command>`.
4. Verify the selected environment in project configuration.
5. Inspect Docker directly only after establishing how Shopware CLI attempted to execute the operation.

Do not move a command from the container to the host just to make it work unless the project is explicitly configured for local execution.

## Safety

Docker does not make a command safe.

Commands executed through `project console`, Composer, the database, or project-management commands can still modify files, dependencies, application state, or data.

Before a destructive action:

- identify the environment;
- inspect the command;
- understand the expected changes;
- prefer previews or read-only checks when available;
- preserve unrelated user work.
