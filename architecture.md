---
title: Evolving Shopware CLI from a command set into a CLI-as-Platform
date: 2026-06-17
area: architecture
tags: [boundaries, commands, patterns]
---

## 1. Summary

`shopware-cli` is a Go/Cobra CLI with four command groups, registering from
[cmd/root.go](cmd/root.go): `account`, `ai`, `extension`, and `project`.
Entry is [main.go](main.go) → `cmd.Execute`.

This summarizes current architecture, then sets a strategic direction across
three main bets:

- Enabling an AI/agent to be a first-class consumer
- Providing a richer dev-inner-loop (build/watch/preview)
- Providing a guided-but-delegating deploy experience

The CLI keeps deployment configuration declarative and uses the verifier
`Tool` registry as an extension model. The strategic focus is the "run,
watch, and build" stage of the user flow.

## 2. Extensibility surfaces: where new features plug in

Some of these easily support expansion and new development; others present some
limitations or friction.

### 2.1 Commands: adding new commands is low-friction

The root command registers command groups explicitly in `cmd/root.go`.
Subcommands are generally defined in `cmd/<group>/<group>_<subcommand>.go`
and added to their parent from an `init` function. The Go filename does not
register a Cobra command by itself; the registration call is required.

### 2.2 Verifier tools: provides reproducible pattern

Each verifier tool registers a name and implements only the capabilities it
supports: checking, fixing, formatting, or a combination. Commands select tools
by capability, then apply `--only` and (where available) `--exclude`. An
unsupported `--only` name is an error that lists the tools available to that
command.

Registration is `func init() { AddTool(PhpStan{}) }` into a global
`availableTools`. Consumers call `GetToolsOf[CheckTool]()` (or `FixTool` /
`FormatTool`) to get a typed `ToolList[T]`; its `Only` and `Exclude` methods
preserve that capability type.

Current checkers are `builtin`, `phpstan`, `eslint`, `stylelint`, and
`storefront-twig`. Fixers are `rector`, `eslint`, `stylelint`, and
`symfony-xml`; formatters are `php-cs-fixer`, and `prettier`. `builtin` enforces
built-in extension rules; the legacy name `sw-cli` is still accepted as an input
alias. It does not validate per-extension metadata in a project context. There
is no separate Composer verifier tool.

The shared runner collects findings and execution errors through `ToolExecution`
and produces a result for each tool. Reports distinguish completed, failed and
skipped tools; a selected tool can also skip itself when it does not apply.
A completed tool may still report source findings. See
[the tool-results ADR](docs/adr/2026-10-22-unified-tool-results.md).

### 2.3 Extension types: simple interface

Two primary types, app and plugin (themes and bundles are kinds of plugins). The
CLI detects three packaging signatures behind one shared `Extension` interface
([internal/extension/root.go:103](https://github.com/shopware/shopware-cli/blob/6ebf84df16147190a258d2aeadf91a4a15c707d7/internal/extension/root.go#L103)),
and the CLI tells them apart by a file it finds: `manifest.xml` → App;
`composer.json` with `shopware-platform-plugin` → plugin; shopware-bundle →
bundle.

The `Extension` interface abstracts extension type behind
`GetName/GetType/GetSourceDirs/Validate/GetExtensionConfig/…`.

### 2.4 Config schema: one field to add, embedded, self-validating

In the CLI, the config YAML files (`.config/shopware-extension.yml` /
`.config/shopware-project.yml`) are validated against a built-in schema, and
that schema is published via `config-schema` so editors and agents can read the
rules and introspect the contract. Adding a config option means adding a field
(+ schema where needed) in `internal/extension/config.go` /
`internal/shop/config.go`.

Deployment Helper re-parses this same `deployment:` block by hand rather than
from the schema.

### 2.5 Dependency injection: inconsistent

In the CLI, only the `account` command group builds its dependencies (clients,
config) through a real service container (`cmd/account/account.go:20`):

- `Register(rootCmd, onInit func(cmd string) (ServiceContainer, error))` wires
  deps in `PersistentPreRunE`
  ([cmd/account/account.go#L20](https://github.com/shopware/shopware-cli/blob/6ebf84df16147190a258d2aeadf91a4a15c707d7/cmd/account/account.go#L20)).

**Scalability blocker:** `extension` and `project` use ad-hoc context lookups +
direct instantiation. As the surface grows, it makes commands harder to test or
mock and forces expensive clients to be built even when they aren't used.

- We could generalize the dependency injection pattern `account` uses to
  `project` and `extension`, but with lazy getters (build the shop client only
  when a command asks for it).

The Deployment Helper approach to dependency injection—a single Symfony
container wired and compiled once—is generally stronger.

### 2.6 CLI-level hooks and middleware

The root command has a `PersistentPreRunE` hook and enables Cobra's traversal of
run hooks. `Execute` also sets up context, logging, interaction and update
checks, and records command execution after Cobra returns. Individual groups
and commands can define their own hooks.
([cmd/root.go#L36](https://github.com/shopware/shopware-cli/blob/6ebf84df16147190a258d2aeadf91a4a15c707d7/cmd/root.go#L36))

- Cleanup activity:
  _instead of each being baked into `Execute()` or duplicated per group_, cobra
  has pre and post functions that we could use to offload some of that activity.
  We use these for `account` already; will extend into the rest of CLI.

For Deployment Helper extension points, see section 4.3.

### 2.7 Project and extension validation

Both `extension validate` and `project validate` select registered checkers
and pass their configuration to the shared verifier runner. An extension is
discovered from the supplied path; project validation discovers project
extensions and source directories, excluding vendor extensions and configured
ignored extensions. Project validation also offers `--local-only`.

Both commands accept `--format` to select a report. Supported report formats
include summary, JSON, GitHub, GitLab, JUnit and Markdown.

The runner normalizes finding paths relative to the analysis root and filters
findings according to configured ignores. A tool can report execution errors
alongside findings; an execution error or remaining error-level finding makes
the report fail.

## 3. Scalability & performance

### 3.1 Where the CLI already does/runs things concurrently, for speed

- **Validate / format / fix:** runs the selected capability-specific tools
  concurrently via the shared runner and waits for them; an execution error
  makes the command fail.
- **npm installs:** installs dependencies for multiple extensions in parallel,
  with as many workers as you have CPU cores (`runtime.NumCPU()`,
  internal/extension/npm.go:70).
- **Asset file hashing:** hashes files using a pool of eight workers at once
  (asset_config.go:231).
- **DB dump:** dumps multiple database tables at once (`--parallel`), with a cap
  so it doesn't overload (internal/mysqldump/mysql.go).
- **Worker command:** runs several job consumers at once, rate-limited so they
  don't flood the system (cmd/project/project_worker.go:86).

### 3.2 Two ways to build admin/storefront assets

- **Fast path (esbuild):** the CLI builds assets itself, in-process, by calling
  esbuild directly (`api.Build(...)`, internal/esbuild/esbuild.go:130). No
  separate Node process to spin up.
- **Slow path (webpack fallback):** when esbuild can't be used, the CLI shells
  out to Node to run webpack, and builds the admin assets first, then the
  storefront one after the other, per extension
  (internal/extension/asset_platform.go).

### 3.3 Caching

The asset cache key combines the Shopware version with hashes of source
files and configuration (`internal/extension/asset_cache.go`).

### 3.4 Performance issues/weaknesses

None are broken, but some are slower.

- **Medium:** The file-hashing pool uses eight workers instead of scaling to
  CPU. `internal/extension/asset_config.go:231`
- **Medium:** Code checkers run in parallel with each other, but each one still
  processes its files one at a time. `internal/verifier/`
- **Medium:** cache is local/CI-only. No shared backend a whole CI fleet can
  reuse. `internal/system/cache_*`
  - **TODO:** For GH, whole fleet can use it. for gitlab you kinda have to share
    the directory, so we need to document what to do in GitLab.
- **Low:** untuned HTTP client. Uses Go's defaults: no connection-reuse tuning,
  no retries. `internal/shop/client.go:37`
  - **TODO:** remove and replace with a general http client for Shopware in Go,
    wired into <https://github.com/shopware/app-sdk-go>

### 3.5 Observability and what we can measure

Since the CLI primarily orchestrates external tools (PHP, Node.js, Docker, etc.)
rather than performing significant computation itself, profiling the Go process
would provide limited value. The most useful metrics are command duration, step
duration, time spent in external processes, and success/failure rates.

The root command records command execution through the tracking package after
Cobra returns. The verifier runner also records subprocess telemetry. Tracking
is disabled when the `DO_NOT_TRACK` environment variable is present
(`internal/tracking/tracking.go`).

We can build on this signal.

## 4. Deployment Helper: boundary must stay clean

The CLI builds, validates and packages projects locally and in CI. It defines
and validates the `deployment:` configuration; Deployment Helper executes
deployment workflows. The CLI can also invoke the Helper for workflows such as
headless local installation.

For extension management, the CLI changes the local project, while Deployment
Helper reconciles the deployed shop with that project state. Proposed deploy
commands would prepare and trigger deployments through the Helper.

### 4.1 Deployment orchestration

Deployment Helper's `RunCommand` delegates to installation or upgrade
managers, which run sequences of commands through `ProcessHelper`. Their
stages are implicit in control flow, making dry runs, structured reports and
resumability difficult. Named stages and structured step results could make
the workflow easier to extend.

### 4.2 Configuration inputs

Deployment Helper uses both YAML configuration and environment variables. The
environment variables are primarily intended as short-lived inputs for
installation and deployment workflows, making a reasonable split between
file-based configuration and env-based inputs.

### 4.3 Extension seams

`PostDeploy` subscribers (Fastly, Platform.sh, staging, consent, cache) keep
provider/env behavior out of the managers. Hooks (pre/post/preInstall/…) run
config strings via the shell-out layer. This maintains flexibility but allows
behavior to move into shell commands that the PHP code cannot model.

One-time tasks are the strongest primitive: run-once scripts keyed by id,
tracked in the DB, marked only on success.

### 4.4 Version-upgrade workflow

`project upgrade` provides an interactive local upgrade wizard. In
non-interactive mode it requires `--target`; `--dry-run` runs the
read-only preflight, and `--no-audit` controls whether the workflow continues
when dependencies are blocked by security advisories. The command resolves a
project executor and delegates the headless workflow to the project upgrader in
`internal/shop/upgrade`.

## 5. Supporting AI / agents as first-class consumers

Machine-readable output becomes a design rule, not an afterthought:

- Per-plugin Markdown scan output an agent (or human) can work through (for
  `plugin validate`)
- Explicit opt-in project skills that produce agent-ready plans, making
  AI-assisted workflows a deliberate user choice while remaining independent of
  any specific provider or agent implementation. (See
  <https://github.com/shadcn/improve>)
- a natural-language env command that drives local actions (start/stop env,
  watchers, cache clear, logs) via skills.

### 5.1 Output & interaction need to become consumable and consistent

Two related gaps, both blocking the AI/agent direction and the machine-readable
promise:

- **Machine-readable output is ad-hoc, not a contract.** `--format` selects a
  command's output representation; supported values vary by command. Structured
  output should follow this convention wherever it makes sense.
- **Output isn't centralized.** Commands mix direct `fmt.Print*` calls with
  `logging.FromContext` calls. Logging is structured (`zap`) and goes to
  stderr; the `fmt` calls go to stdout with no level, no format control, and no
  way to silence or serialize them. The situation is further complicated by
  native CI/CD output that is heavily used by commands such as `project ci`.
  That fragmentation is why a clean JSON mode is hard today. There's no single
  output path to switch. Routing result output through one channel (and
  reserving stderr for logs) is the prerequisite.

## 6. Logging

The logger is human-oriented (colored, dev-style encoder); `zap` already
supports JSON via `NewProductionConfig`, so a `--log-format=json` is a small
lever once output is centralized.

## 7. Parity between interactive/non-interactive

Interactive behavior varies by command. The root context disables interaction
when `--no-interaction` is supplied or stdin is not a terminal; commands that
consult that context can use flags and defaults or fail when input is required.
Some commands check the terminal directly, so the root flag alone does not
establish parity across every command. Many use `system.IsInteractionEnabled`
from the root context.
