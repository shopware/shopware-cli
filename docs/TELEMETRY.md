# Telemetry

This document describes the anonymous usage telemetry built into `shopware-cli`:
how users are identified, what data we collect, where it goes, and how users can
opt out. It is written for a product/PM audience but is accurate to the current
implementation (`internal/tracking/tracking.go`).

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

## What we want to learn (product perspective)

The goal of telemetry is to answer questions like:

- Which commands are actually used in the wild, and which are not worth
  maintaining?
- What operating systems do our users run (Linux / macOS / Windows)?
- How often do commands fail vs. succeed, and how long do they take?
- Is the CLI mostly run interactively by developers, or non-interactively in
  CI pipelines?
- For key flows (project creation, upgrade checks), which configuration options
  are popular (e.g. which Shopware version, which CI provider, Docker yes/no)?

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

There are currently **three** tracked events.

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

## Where this lives in the code

- Core implementation: `internal/tracking/tracking.go`
- Tests (including ID derivation behavior): `internal/tracking/tracking_test.go`
- Tracking call sites:
  - `cmd/root.go` — `shopware_cli.command`
  - `cmd/project/project_create.go` — `shopware_cli.project.create`
  - `cmd/project/project_upgrade_check.go` — `shopware_cli.project.upgrade_check`
