# Telemetry

This document describes the anonymous usage telemetry built into `shopware-cli`:
how users are identified, what data we collect, where it goes, and how users can
opt out. It is accurate to the current implementation
(`internal/tracking/tracking.go`).

## TL;DR

- We send **anonymous usage events** so we can understand which commands are
  used, on which platforms, and how often they succeed or fail.
- We do **not** collect names, emails, IP-based identity, source code, project
  contents, or any account information.
- Each install is identified by a **pseudonymous ID** (a random or hashed
  value), not by a person.
- Telemetry can be **fully disabled** by setting the `DO_NOT_TRACK` environment
  variable.
- Events are sent over **UDP** (fire-and-forget). If the network is unavailable
  the CLI continues normally — telemetry never blocks or slows down a command in
  a way the user notices.

## What we want to learn

The goal of telemetry is to answer questions like:

- Which commands are actually used in the wild, and which are not worth
  maintaining?
- What operating systems do our users run (Linux / macOS / Windows)?
- How often do commands fail vs. succeed, and how long do they take?
- Which CI systems run the CLI, and does WSL behave differently from native
  Linux?
- Is the CLI mostly run interactively by developers, or non-interactively in
  CI pipelines?
- For key flows (project creation, upgrade checks), which configuration options
  are popular (e.g. which Shopware version, which CI provider, Docker yes/no)?
- For the interactive development TUI (`project dev`): do the setup wizards
  succeed or where do users abandon them, do Docker environments start
  reliably, and which dashboard features (watchers, builds, tabs) are used?
- For the AI integrations (`shopware-cli ai`): which skills and MCP servers are
  actually installed, into which agents, at project vs. user scope, how often a
  specific release is pinned, and how often installs succeed vs. fail?

## How we identify users

We never identify a _person_. We identify an **install / environment** with a
pseudonymous `user_id`. How that ID is derived depends on where the CLI runs.

### 1. CI environments (deterministic, hashed)

If the CLI detects it is running in a CI system, it derives a **stable, hashed**
ID from a repository identifier. This means the same repository always maps to
the same ID, without us knowing which repository it is.

The following environment variables are checked, in order:

| Provider  | Environment variable       | Example value                           |
| --------- | -------------------------- | --------------------------------------- |
| GitHub    | `GITHUB_REPOSITORY`        | `shopware/shopware`                     |
| GitLab    | `CI_PROJECT_URL`           | `https://gitlab.example.com/group/repo` |
| Bitbucket | `BITBUCKET_REPO_FULL_NAME` | `workspace/repo`                        |

The raw value is run through **SHA-256** and we keep the first 16 bytes (32 hex
characters). The original repository name is **never transmitted** — only its
hash. The hash is one-way, so we cannot recover the repository name from it.

### 2. Local machines (random, persisted)

On a developer's local machine, the CLI generates a **random** 16-byte ID
(32 hex characters) the first time it runs and stores it in the user's config
directory as `.shopware-cli-id` (e.g. `~/.config/.shopware-cli-id` on Linux).
Subsequent runs reuse the same ID, so a single machine appears as one consistent
install over time.

This ID is random — it is **not** derived from anything about the user, the
machine, or its network.

### 3. Fallback (ephemeral)

If neither a CI identifier nor a writable config directory is available, the CLI
uses a one-off random ID for that invocation. These runs cannot be correlated
over time.

> **Priority:** CI identification always wins over the locally persisted ID, so
> CLI runs inside CI are grouped by repository rather than by the runner's home
> directory.

## What we collect (tracking points)

All events share a common envelope:

| Field       | Description                                                         |
| ----------- | ------------------------------------------------------------------- |
| `event`     | Event name, always prefixed with `shopware_cli.`                    |
| `user_id`   | The pseudonymous ID described above                                 |
| `timestamp` | RFC 3339 timestamp of when the event was sent                       |
| `tags`      | Event-specific key/value pairs (all string values), described below |

Every event's `tags` also carry `cli_version`, the version of `shopware-cli`
that sent it, so dev builds can be excluded for every event.

There are the command-level events below, plus a set of events sent by the
interactive development TUI (`shopware-cli project dev`).

### `shopware_cli.tool.subprocess` — subprocess duration and output sizes

Sent after each checker, formatter, or fixer subprocess attempt, including failed
starts and preparation commands such as Composer installation. This respects `DO_NOT_TRACK`.

| Tag            | Meaning                                                                                                    |
| -------------- | ---------------------------------------------------------------------------------------------------------- |
| `tool`         | Tool name, such as `phpstan` or `eslint`                                                                   |
| `operation`    | Fixed operation label identifying the subprocess purpose; see below                                        |
| `stdout_bytes` | Number of bytes captured from stdout                                                                       |
| `stderr_bytes` | Number of bytes captured from stderr                                                                       |
| `duration_ms`  | Elapsed wall-clock milliseconds around command execution, including startup; may be zero for fast attempts |
| `started`      | `"true"` if the process started, `"false"` if it failed to start                                           |
| `exit_code`    | Exit code when available; omitted for failed starts and signal termination                                 |

Operation labels are defined in the CLI code and never derived from command
arguments, paths, or package names. They are included for failed starts too.

| Tool                          | Operation                             |
| ----------------------------- | ------------------------------------- |
| PHPStan or Rector preparation | `composer_require`, `composer_update` |
| PHPStan                       | `analyse`                             |
| Rector                        | `fix`                                 |
| ESLint, Stylelint             | `check`, `fix`                        |
| Prettier, PHP-CS-Fixer        | `check` (dry run), `format`           |

Repeated invocations across source directories share the same operation label;
directory details remain in local verbose logs.

Duration excludes output parsing and telemetry delivery. Processes terminated by
a signal still report their duration and captured output sizes. A nonzero exit code can indicate
findings rather than an execution failure, depending on the tool.
Output contents, command arguments, working directories, and error messages are not included.
These measurements help compare subprocess runtime, output sizes, and exit codes,
identify startup failures, and decide which tools need streaming output instead of
buffering it in memory.

### `shopware_cli.command` — every command run

Sent after (almost) any sub-command finishes. This is our broadest signal of
overall CLI usage.

| Tag            | Meaning                                                                                                                                                                                                                                                                                  | Example                                     |
| -------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------- |
| `command_name` | The command path, dot-separated (dashes become underscores)                                                                                                                                                                                                                              | `extension.build`                           |
| `result`       | Outcome of the command. A run that ends with an error after Ctrl+C or SIGTERM, or because the user aborted a prompt, is `cancelled`, even when a child process reported the error. Long-running commands that shut down cleanly after a signal, such as `project worker`, stay `success` | `success` / `failure` / `cancelled`         |
| `duration_ms`  | How long the command took, in milliseconds                                                                                                                                                                                                                                               | `1423`                                      |
| `cli_version`  | The version of `shopware-cli` in use                                                                                                                                                                                                                                                     | `0.4.123`                                   |
| `os`           | Operating system the CLI runs on (Go `runtime.GOOS`)                                                                                                                                                                                                                                     | `linux` / `darwin` / `windows`              |
| `is_tui`       | Whether the run was interactive (a terminal / TUI session)                                                                                                                                                                                                                               | `true` / `false`                            |
| `is_wsl`       | Whether a Linux run happens inside the Windows Subsystem for Linux                                                                                                                                                                                                                       | `true` / `false`                            |
| `ci_provider`  | The CI system a run executes on; only present on CI runs. `other` means CI was detected without a known provider                                                                                                                                                                         | `github` / `gitlab` / `bitbucket` / `other` |

Notes:

- The command _name_ is captured (e.g. `project.create`), but **not** its
  arguments, flags, paths, or any free-text input.
- The root command alone (running `shopware-cli` with no sub-command) is not
  tracked.
- Plugin generators (`shopware-cli extension add <generator>`) are measured
  through this event. Hyphens become underscores, so Grafana can filter
  `command_name` values that start with `extension.add.` — for example
  `extension.add.admin_module` and `extension.add.store_api_route`. Entity
  names and other generator arguments are never sent.

### `shopware_cli.project.create` — project scaffolding

Sent when a user creates a new Shopware project. Helps us understand which
starting configurations are popular.

| Tag                  | Meaning                              | Example         |
| -------------------- | ------------------------------------ | --------------- |
| `version`            | Selected Shopware version            | `6.6`           |
| `deployment`         | Selected deployment target           | `shopware-paas` |
| `ci`                 | Selected CI provider                 | `github`        |
| `docker`             | Whether Docker setup was chosen      | `true`          |
| `with_elasticsearch` | Whether Elasticsearch was enabled    | `false`         |
| `with_amqp`          | Whether AMQP was enabled             | `false`         |
| `interactive`        | Whether the wizard ran interactively | `true`          |

### `shopware_cli.project.upgrade` — interactive upgrade wizard run

Sent when the upgrade wizard (`shopware-cli project upgrade`) finishes
executing an upgrade, successfully or not. Helps us understand which upgrade
paths succeed and where guided upgrades fail.

| Tag              | Meaning                                 | Example               |
| ---------------- | --------------------------------------- | --------------------- |
| `from_version`   | The Shopware version before the upgrade | `6.6.10.3`            |
| `target_version` | The version the wizard upgraded to      | `6.7.11.0`            |
| `result`         | Outcome of the upgrade run              | `success` / `failure` |

## AI integration events (`shopware-cli ai`)

The `ai` command group sends the events below after an integration has been
**resolved against the built-in directory**, so the `integration` and `agent`
values are always enumerated catalog/client identifiers — never free-text, a
file path, or an unrecognized name a user typed. An unknown name, a missing
`--agent`, and other input errors are covered by `shopware_cli.command`
(`result=failure`) and are not reported here. No URLs and no project paths are
ever sent.

### `shopware_cli.ai.add` — AI integration installed

Sent when `shopware-cli ai add` installs (or previews, with `--dry-run`) a known
integration, whether the attempt succeeds or fails.

| Tag           | Meaning                                                                                                       | Example                                         |
| ------------- | ------------------------------------------------------------------------------------------------------------- | ----------------------------------------------- |
| `integration` | The integration from the directory                                                                            | `shopware-cli` / `deployment-helper` / `meteor` |
| `type`        | Kind of integration                                                                                           | `skill` / `mcp`                                 |
| `agent`       | Target AI agent (skills only; omitted for MCP)                                                                | `claude-code` / `codex`                         |
| `scope`       | Where it was installed                                                                                        | `project` / `global`                            |
| `pinned`      | Whether a specific release was requested with `@<tag>` (skills only). The tag value itself is **never** sent. | `false`                                         |
| `dry_run`     | Whether this was a preview that changed nothing on disk                                                       | `false`                                         |
| `result`      | Outcome of the install                                                                                        | `success` / `failure`                           |

MCP installs are always `scope=project` and carry no `agent` (the project's
`.mcp.json` is not agent-specific). The command's duration is already captured by
`shopware_cli.command`, so it is not repeated here.

### `shopware_cli.ai.remove` — AI integration removed

Sent when `shopware-cli ai remove` removes (or previews, with `--dry-run`) a known
integration.

| Tag           | Meaning                                                                              | Example               |
| ------------- | ------------------------------------------------------------------------------------ | --------------------- |
| `integration` | The integration from the directory                                                   | `meteor`              |
| `type`        | Kind of integration                                                                  | `skill` / `mcp`       |
| `agent`       | Target AI agent (skills only; omitted for MCP)                                       | `claude-code`         |
| `scope`       | Which install was targeted                                                           | `project` / `global`  |
| `dry_run`     | Whether this was a preview that changed nothing                                      | `false`               |
| `removed`     | Whether the integration was actually present (`false` = there was nothing to remove) | `true`                |
| `result`      | Outcome of the removal                                                               | `success` / `failure` |

## Development TUI events (`shopware-cli project dev`)

The interactive development dashboard sends the events below so we can see
where the setup flows lose users, whether Docker environments start reliably,
and which dashboard features are actually used. As everywhere else: no URLs,
no sales-channel or theme names, no credentials, and no free-text input are
ever transmitted — only enumerated choices, outcomes, and durations.

### `shopware_cli.project.dev.install` — first-run Shopware installation

Sent when the "Shopware is not initialized yet" wizard reaches a terminal
state, or when the non-interactive `project dev install` command finishes an
installation attempt (failures to start the development environment before
the installation begins are not reported under this event). Choice tags are
only present once the user made that choice (an event for a run cancelled on
the language step carries no `language` tag).

| Tag                  | Meaning                                                                                                                                                                                                                                           | Example                                                                                                                                                                                      |
| -------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `result`             | Outcome of the wizard                                                                                                                                                                                                                             | `success` / `failure` / `cancelled` / `skipped`                                                                                                                                              |
| `abandoned_at`       | Step shown when the user quit (only for `cancelled`)                                                                                                                                                                                              | `ask` / `language` / `currency` / `credentials` / `installing`                                                                                                                               |
| `failed_step`        | Step that was running when the install failed (only for `failure`). `install_start` means the helper failed before any step began; `save_credentials` means the install succeeded but writing the admin credentials to the project config failed. | `system:install` / `install_start` / `save_credentials`                                                                                                                                      |
| `failure_category`   | Classified reason the install failed (only for `failure`). Closed enum — the raw error text is **never** sent.                                                                                                                                    | `db_connection` / `db_version` / `env_config` / `migration` / `permission` / `php` / `invalid_input` / `already_exists` / `missing_prerequisite` / `theme_compile` / `transport` / `unknown` |
| `duration_ms`        | Install runtime, once the install actually started                                                                                                                                                                                                | `84213`                                                                                                                                                                                      |
| `language`           | Selected default language                                                                                                                                                                                                                         | `de-DE`                                                                                                                                                                                      |
| `currency`           | Selected default currency                                                                                                                                                                                                                         | `EUR`                                                                                                                                                                                        |
| `custom_credentials` | Whether the default admin username/password were changed. The credentials themselves are **never** sent.                                                                                                                                          | `false`                                                                                                                                                                                      |
| `interactive`        | `true` for the TUI wizard, `false` for `project dev install`                                                                                                                                                                                      | `true`                                                                                                                                                                                       |

### `shopware_cli.project.dev.migration_wizard` — dev-environment setup wizard

Sent when the wizard that migrates an existing project to the Docker dev
environment reaches a terminal state. This replaces the earlier
`shopware_cli.migration_wizard_completed` event, which only reported
successful runs.

| Tag                       | Meaning                                                                                            | Example                                            |
| ------------------------- | -------------------------------------------------------------------------------------------------- | -------------------------------------------------- |
| `result`                  | Outcome of the wizard                                                                              | `completed` / `cancelled` / `failed`               |
| `abandoned_at`            | Step shown when the user quit (only for `cancelled`)                                               | `welcome` / `admin_user` / `docker_php` / `review` |
| `duration_ms`             | Time since the welcome screen was confirmed                                                        | `45120`                                            |
| `php_version`             | Selected PHP version (`completed` / `failed` only)                                                 | `8.3`                                              |
| `deployment_helper_added` | Whether composer.json was missing `shopware/deployment-helper` and it was added (`completed` only) | `true`                                             |

### `shopware_cli.project.dev.docker_start` — container startup

Sent when a `docker compose up -d` run by the TUI finishes.

| Tag           | Meaning                                                | Example                             |
| ------------- | ------------------------------------------------------ | ----------------------------------- |
| `trigger`     | Why the containers were (re)started                    | `initial` / `config_change`         |
| `result`      | Outcome (`cancelled` when the user quit while waiting) | `success` / `failure` / `cancelled` |
| `duration_ms` | How long the startup took                              | `12894`                             |

### `shopware_cli.project.dev.action` — dashboard actions

Sent when a command-palette action runs. Instant actions (opening the shop or
admin in the browser) carry only the action name; task actions also report
their outcome and runtime.

| Tag           | Meaning                          | Example                                                                 |
| ------------- | -------------------------------- | ----------------------------------------------------------------------- |
| `action`      | The invoked action               | `open-shop` / `open-admin` / `cache-clear` / `admin-build` / `sf-build` |
| `result`      | Task outcome (task actions only) | `success` / `failure` / `cancelled`                                     |
| `duration_ms` | Task runtime (task actions only) | `31022`                                                                 |

### `shopware_cli.project.dev.watcher` — admin/storefront watchers

Sent once per watcher run, when the watcher ends. A run that ends while it is
still preparing or starting carries that phase, so runs that never got going
are visible.

| Tag           | Meaning                                                                                                                                                                                   | Example                                                                                                                           |
| ------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------- |
| `watcher`     | Which watcher ran                                                                                                                                                                         | `admin` / `storefront`                                                                                                            |
| `result`      | How the run ended: preparation failed, the dev-server process exited on its own, the user stopped it, or the TUI session ended                                                            | `prep_failed` / `crashed` / `user_stopped` / `session_end`                                                                        |
| `phase`       | How far the run got when it ended: the preparation steps, the dev server started but not answering yet, or answering                                                                      | `preparing` / `starting` / `running`                                                                                              |
| `failed_step` | The preparation step that failed (only for `prep_failed`)                                                                                                                                 | `plugins_json` / `feature_dump` / `npm_install` / `entity_schema` / `theme_select` / `theme_compile` / `theme_dump` / `proxy_env` |
| `proxied`     | Whether the watcher is configured to run through the shared proxy; when the proxy could not start and the dashboard fell back to local ports, the storefront watcher still reports `true` | `true` / `false`                                                                                                                  |
| `duration_ms` | How long the watcher ran                                                                                                                                                                  | `1830211`                                                                                                                         |

### `shopware_cli.project.dev.health` — setup health snapshot

Sent once per session when the Overview tab's setup-health report first
loads: one event per check. This tells us how common misconfigurations are in
real projects — and which checks are worth turning into automatic fixes. Only
the check's level is sent, never the underlying values.

| Tag      | Meaning                                                                                   | Example                    |
| -------- | ----------------------------------------------------------------------------------------- | -------------------------- |
| `check`  | Which check ran (`php_version`, `memory_limit`, `admin_worker`, `flow_builder_log_level`) | `admin_worker`             |
| `result` | The check's level                                                                         | `ok` / `warn` / `critical` |

### `shopware_cli.project.dev.session` — dashboard session shape

Sent when the user leaves the dashboard (the overall command duration is
already covered by `shopware_cli.command`; this event adds what happened
inside the TUI).

| Tag             | Meaning                                                                  | Example                                     |
| --------------- | ------------------------------------------------------------------------ | ------------------------------------------- |
| `executor`      | Environment the project runs in                                          | `docker` / `local`                          |
| `duration_ms`   | TUI session length                                                       | `1912734`                                   |
| `tabs_visited`  | Which tabs were opened (sorted, comma-separated)                         | `config,instance,overview`                  |
| `actions`       | Number of palette actions invoked                                        | `3`                                         |
| `watchers_used` | Which watchers were started (omitted if none)                            | `admin,storefront`                          |
| `result`        | How the session ended: via the "stop containers?" dialog or a plain quit | `stop_containers` / `keep_running` / `quit` |

## What we explicitly do **not** collect

- No names, emails, usernames, or account / license identifiers.
- No source code, file contents, file names, or directory paths.
- No command arguments, flags, or free-text input.
- No repository names in clear text (CI repos are one-way hashed).
- No IP addresses are stored as identity (UDP packets carry a source IP at the
  network layer, but it is not used as the `user_id`).

## How data is transmitted

- Events are serialized as JSON and sent via **UDP** to
  `udp.usage.shopware.io:9000`.
- The destination domain can be overridden with the `SHOPWARE_TRACKING_DOMAIN`
  environment variable (primarily for testing / self-hosting).
- Sending is **fire-and-forget** with a short timeout (~300 ms) and runs in a
  way that does not block the command result. If the endpoint is unreachable,
  the error is silently logged at debug level and the CLI proceeds normally.

## Opting out

Telemetry honors the cross-tool [Console Do Not Track](https://consoledonottrack.com/)
standard. Setting the `DO_NOT_TRACK` environment variable to any value disables
all telemetry:

```bash
export DO_NOT_TRACK=1
```

When set, the `Track` function returns immediately and **no event is ever sent**.
