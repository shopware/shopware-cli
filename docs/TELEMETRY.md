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
- Is the CLI mostly run interactively by developers, or non-interactively in
  CI pipelines?
- For key flows (project creation, upgrade checks), which configuration options
  are popular (e.g. which Shopware version, which CI provider, Docker yes/no)?
- For the interactive development TUI (`project dev`): do the setup wizards
  succeed or where do users abandon them, do Docker environments start
  reliably, and which dashboard features (watchers, builds, tabs) are used?

## How we identify users

We never identify a *person*. We identify an **install / environment** with a
pseudonymous `user_id`. How that ID is derived depends on where the CLI runs.

### 1. CI environments (deterministic, hashed)

If the CLI detects it is running in a CI system, it derives a **stable, hashed**
ID from a repository identifier. This means the same repository always maps to
the same ID, without us knowing which repository it is.

The following environment variables are checked, in order:

| Provider  | Environment variable        | Example value                              |
|-----------|-----------------------------|--------------------------------------------|
| GitHub    | `GITHUB_REPOSITORY`         | `shopware/shopware`                         |
| GitLab    | `CI_PROJECT_URL`            | `https://gitlab.example.com/group/repo`    |
| Bitbucket | `BITBUCKET_REPO_FULL_NAME`  | `workspace/repo`                           |

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

| Field       | Description                                                        |
|-------------|--------------------------------------------------------------------|
| `event`     | Event name, always prefixed with `shopware_cli.`                   |
| `user_id`   | The pseudonymous ID described above                                |
| `timestamp` | RFC 3339 timestamp of when the event was sent                      |
| `tags`      | Event-specific key/value pairs (all string values), described below|

There are three command-level events, plus a set of events sent by the
interactive development TUI (`shopware-cli project dev`).

### `shopware_cli.command` — every command run

Sent after (almost) any sub-command finishes. This is our broadest signal of
overall CLI usage.

| Tag            | Meaning                                                       | Example                |
|----------------|---------------------------------------------------------------|------------------------|
| `command_name` | The command path, dot-separated (dashes become underscores)   | `extension.build`      |
| `result`       | Outcome of the command                                        | `success` / `failure` / `cancelled` |
| `duration_ms`  | How long the command took, in milliseconds                    | `1423`                 |
| `cli_version`  | The version of `shopware-cli` in use                          | `0.4.123`              |
| `os`           | Operating system the CLI runs on (Go `runtime.GOOS`)          | `linux` / `darwin` / `windows` |
| `is_tui`       | Whether the run was interactive (a terminal / TUI session)    | `true` / `false`       |

Notes:
- The command *name* is captured (e.g. `project.create`), but **not** its
  arguments, flags, paths, or any free-text input.
- The root command alone (running `shopware-cli` with no sub-command) is not
  tracked.

### `shopware_cli.project.create` — project scaffolding

Sent when a user creates a new Shopware project. Helps us understand which
starting configurations are popular.

| Tag                  | Meaning                                  | Example         |
|----------------------|------------------------------------------|-----------------|
| `version`            | Selected Shopware version                | `6.6`           |
| `deployment`         | Selected deployment target               | `shopware-paas` |
| `ci`                 | Selected CI provider                     | `github`        |
| `docker`             | Whether Docker setup was chosen          | `true`          |
| `with_elasticsearch` | Whether Elasticsearch was enabled        | `false`         |
| `with_amqp`          | Whether AMQP was enabled                 | `false`         |
| `interactive`        | Whether the wizard ran interactively     | `true`          |

### `shopware_cli.project.upgrade_check` — upgrade compatibility check

Sent when a user runs an upgrade check. Helps us understand upgrade paths and
how often blockers are encountered.

| Tag              | Meaning                                          | Example  |
|------------------|--------------------------------------------------|----------|
| `from_version`   | The current Shopware version                     | `6.5.8`  |
| `target_version` | The version the user wants to upgrade to         | `6.6.0`  |
| `has_blockers`   | Whether any blocking incompatibilities were found| `true`   |

### `shopware_cli.project.upgrade` — interactive upgrade wizard run

Sent when the upgrade wizard (`shopware-cli project upgrade`) finishes
executing an upgrade, successfully or not. Helps us understand which upgrade
paths succeed and where guided upgrades fail.

| Tag              | Meaning                                   | Example    |
|------------------|-------------------------------------------|------------|
| `from_version`   | The Shopware version before the upgrade   | `6.6.10.3` |
| `target_version` | The version the wizard upgraded to        | `6.7.11.0` |
| `result`         | Outcome of the upgrade run                | `success` / `failure` |

## Development TUI events (`shopware-cli project dev`)

The interactive development dashboard sends the events below so we can see
where the setup flows lose users, whether Docker environments start reliably,
and which dashboard features are actually used. As everywhere else: no URLs,
no sales-channel or theme names, no credentials, and no free-text input are
ever transmitted — only enumerated choices, outcomes, and durations.

### `shopware_cli.project.dev.install` — first-run Shopware installation

Sent when the "Shopware is not initialized yet" wizard reaches a terminal
state. Choice tags are only present once the user made that choice (an event
for a run cancelled on the language step carries no `language` tag).

| Tag                  | Meaning                                                       | Example          |
|----------------------|---------------------------------------------------------------|------------------|
| `result`             | Outcome of the wizard                                         | `success` / `failure` / `cancelled` / `skipped` |
| `abandoned_at`       | Step shown when the user quit (only for `cancelled`)          | `ask` / `language` / `currency` / `credentials` / `installing` |
| `failed_step`        | Last install step that had started (only for `failure`)       | `system:install` |
| `duration_ms`        | Install runtime, once the install actually started            | `84213`          |
| `language`           | Selected default language                                     | `de-DE`          |
| `currency`           | Selected default currency                                     | `EUR`            |
| `custom_credentials` | Whether the default admin username/password were changed. The credentials themselves are **never** sent. | `false` |

### `shopware_cli.project.dev.migration_wizard` — dev-environment setup wizard

Sent when the wizard that migrates an existing project to the Docker dev
environment reaches a terminal state. This replaces the earlier
`shopware_cli.migration_wizard_completed` event, which only reported
successful runs.

| Tag                       | Meaning                                              | Example      |
|---------------------------|------------------------------------------------------|--------------|
| `result`                  | Outcome of the wizard                                | `completed` / `cancelled` / `failed` |
| `abandoned_at`            | Step shown when the user quit (only for `cancelled`) | `welcome` / `admin_user` / `docker_php` / `review` |
| `duration_ms`             | Time since the welcome screen was confirmed          | `45120`      |
| `php_version`             | Selected PHP version (`completed` / `failed` only)   | `8.3`        |
| `deployment_helper_added` | Whether composer.json was missing `shopware/deployment-helper` and it was added (`completed` only) | `true` |

### `shopware_cli.project.dev.docker_start` — container startup

Sent when a `docker compose up -d` run by the TUI finishes.

| Tag           | Meaning                                                      | Example         |
|---------------|--------------------------------------------------------------|-----------------|
| `trigger`     | Why the containers were (re)started                          | `initial` / `config_change` |
| `result`      | Outcome (`cancelled` when the user quit while waiting)       | `success` / `failure` / `cancelled` |
| `duration_ms` | How long the startup took                                    | `12894`         |

### `shopware_cli.project.dev.action` — dashboard actions

Sent when a command-palette action runs. Instant actions (opening the shop or
admin in the browser) carry only the action name; task actions also report
their outcome and runtime.

| Tag           | Meaning                                    | Example        |
|---------------|--------------------------------------------|----------------|
| `action`      | The invoked action                         | `open-shop` / `open-admin` / `cache-clear` / `admin-build` / `sf-build` |
| `result`      | Task outcome (task actions only)           | `success` / `failure` / `cancelled` |
| `duration_ms` | Task runtime (task actions only)           | `31022`        |

### `shopware_cli.project.dev.watcher` — admin/storefront watchers

Sent once per watcher run, when the watcher ends.

| Tag           | Meaning                                                     | Example    |
|---------------|-------------------------------------------------------------|------------|
| `watcher`     | Which watcher ran                                           | `admin` / `storefront` |
| `result`      | How the run ended: preparation failed, the dev-server process exited on its own, the user stopped it, or the TUI session ended | `prep_failed` / `crashed` / `user_stopped` / `session_end` |
| `duration_ms` | How long the watcher ran                                    | `1830211`  |

### `shopware_cli.project.dev.health` — setup health snapshot

Sent once per session when the Overview tab's setup-health report first
loads: one event per check. This tells us how common misconfigurations are in
real projects — and which checks are worth turning into automatic fixes. Only
the check's level is sent, never the underlying values.

| Tag      | Meaning                                       | Example |
|----------|-----------------------------------------------|---------|
| `check`  | Which check ran (`php_version`, `memory_limit`, `admin_worker`, `flow_builder_log_level`) | `admin_worker` |
| `result` | The check's level                             | `ok` / `warn` / `critical` |

### `shopware_cli.project.dev.session` — dashboard session shape

Sent when the user leaves the dashboard (the overall command duration is
already covered by `shopware_cli.command`; this event adds what happened
inside the TUI).

| Tag             | Meaning                                            | Example                     |
|-----------------|----------------------------------------------------|-----------------------------|
| `executor`      | Environment the project runs in                    | `docker` / `local`          |
| `duration_ms`   | TUI session length                                 | `1912734`                   |
| `tabs_visited`  | Which tabs were opened (sorted, comma-separated)   | `config,instance,overview`  |
| `actions`       | Number of palette actions invoked                  | `3`                         |
| `watchers_used` | Which watchers were started (omitted if none)      | `admin,storefront`          |
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
