---
title: Evolving shopware-cli from a command set into a CLI-as-Platform
date: 2026-17-06
area: architecture
tags: [boundaries, commands, patterns]
---

# Evolving Shopware CLI from a Command Set Into a CLI-as-Platform

## 1. Summary

`shopware-cli` is a Go/Cobra CLI with three command groups, registering from [cmd/root.go](../cmd/root.go): `account`, `extension`, and `project`. Entry is [main.go](../main.go) → `cmd.Execute`.

The CLI already runs every command in two modes from one code path: TUI-guided/interactive and headless (flags / CI / AI agent). That duality is a global invariant, not a per-command feature, and it is the contract that supports "rarely leave the IDE/TUI."

This summarizes current architecture, then sets a strategic direction across four main bets:

- Providing a much larger diagnostics (`doctor`) surface
- Enabling an AI/agent to be a first-class consumer
- Providing a richer dev-inner-loop (build/watch/preview)
- Providing a guided-but-delegating deploy experience

The Deployment Helper boundary remains declarative, reuses the verifier `Tool` registry as the canonical extension model, and prioritizes the "run, watch, and build" stage of our user flow, where users will spend substantial time.

## 2. Extensibility surfaces: where new features plug in

Some of these easily support expansion and new development; others present some limitations or friction.

### 2.1 Commands: adding new commands is low-friction

A new command is just a new file: drop `cmd/<group>/<group><sub>.go` (`cmd/root.go`) and it wires itself in on startup. There's no centralized list to edit. The filename pattern/naming convention is the only rule/contract: `cmd/<group>/<group><subcommand>.go`.

### 2.2 Verifier tools: provides reproducible pattern for implementing other capabilities

Each code-quality tool implements one small interface (name, check, fix, format) and adds itself to a shared list. Callers can then run them all, or filter to just some, in parallel. Currently these are code quality checkers: phpstan, eslint, stylelint, prettier, php-cs-fixer, rector, composer, admin-twig, storefront-twig, sw-cli.

**Decision**: will drop `dry run` and use Git. Why: Underlying tools do not support it. Under the hood, it uses eslint for js, rector for PHP.

```go
// internal/verifier/tool.go:50
type Tool interface {
    Name() string
    Check(ctx context.Context, check *Check, config ToolConfig) error
    Fix(ctx context.Context, config ToolConfig) error
    Format(ctx context.Context, config ToolConfig, dryRun bool) error
}
```

Registration is `func init() { AddTool(PhpStan{}) }` into a global `availableTools`; consumers call `verifier.GetTools().Only(...)` / `.Exclude(...)`.

Currently registered: phpstan, eslint, stylelint, prettier, php-cs-fixer, rector, composer, admin-twig, storefront-twig, sw-cli. The last one is a tool that enforces Shopware-specific validation rules the CLI implements itself; it runs through the same machinery as the external tools.

### 2.3 Extension types: simple interface

Two primary types, app and plugin (themes and bundles are kinds of plugins). The CLI detects three packaging signatures behind one shared `Extension` interface ([internal/extension/root.go:103](https://github.com/shopware/shopware-cli/blob/6ebf84df16147190a258d2aeadf91a4a15c707d7/internal/extension/root.go#L103)), and the CLI tells them apart by a file it finds: `manifest.xml` → App; `composer.json` with `shopware-platform-plugin` → plugin; shopware-bundle → bundle.

The `Extension` interface abstracts extension type behind `GetName/GetType/GetSourceDirs/Validate/GetExtensionConfig/…`. Detection is by file signature ([../internal/extension/root.go#L28](https://github.com/shopware/shopware-cli/blob/6ebf84df16147190a258d2aeadf91a4a15c707d7/internal/extension/root.go#L28)).

### 2.4 Config schema: one field to add, embedded, self-validating

In the CLI, the config YAML files (`.shopware-extension.yml` / `.shopware-project.yml`) are validated against a built-in schema, and that schema is published via `config-schema` so editors and agents can read the rules and introspect the contract. Adding a config option means adding a field (+ schema where needed) in `internal/extension/config.go` / `internal/shop/config.go`.

Deployment Helper re-parses this same `deployment:` block by hand rather than from the schema.

### 2.5 Dependency injection: inconsistent

In the CLI, only the `account` command group builds its dependencies (clients, config) through a real service container (`cmd/account/account.go:20`):

- `Register(rootCmd, onInit func(cmd string) (ServiceContainer, error))` wires deps in `PersistentPreRunE` ([../cmd/account/account.go#L20](https://github.com/shopware/shopware-cli/blob/6ebf84df16147190a258d2aeadf91a4a15c707d7/cmd/account/account.go#L20)).

**Scalability blocker:** `extension` and `project` use ad-hoc context lookups + direct instantiation. As the surface grows, it makes commands harder to test or mock and forces expensive clients to be built even when they aren't used.

- We could generalize the dependency injection pattern `account` uses to `project` and `extension`, but with lazy getters (build the shop client only when a command asks for it).

The Deployment Helper approach to dependency injection—a single Symfony container wired and compiled once—is generally stronger.

### 2.6 CLI-level hooks and middleware: main missing primitive for "platform" behavior

There's no shared place in the CLI to run logic around every command (auth refresh, policy checks, telemetry, third-party commands). Telemetry is middleware-shaped; `Execute()` and other setup is repeated per group. ([cmd/root.go#L36](https://github.com/shopware/shopware-cli/blob/6ebf84df16147190a258d2aeadf91a4a15c707d7/cmd/root.go#L36))

- Cleanup activity: *instead of each being baked into `Execute()` or duplicated per group*, cobra has pre and post functions that we could use to offload some of thatactivity. We use these for `account` already; will extend into the rest of CLI.

Deployment Helper offers a registry-style seam via `PostDeploy` subscribers (`RunCommand.php:80`). Fastly, Platform.sh, staging, usage-consent, always-clear-cache. The strong seam: keeps provider/env behavior out of the managers. `FastlyServiceUpdater` is opt-in via `config/fastly`, env creds, diff-before-write, activate only on change.

Deployment Helper also offers hooks: `HookExecutor`: pre/post/preInstall/postInstall/preUpdate/postUpdate), each a config string run via `ProcessHelper::runAndTail`. Flexible, but the escape hatch: behavior can move into shell the PHP can't model.

### 2.7 Diagnostics `doctor` vs. validation

`project doctor` is in place but is a fixed, project-scoped list of checks printed to the terminal (no severity, no `--fix`, no machine-readable output). Our workflow surfaces `doctor` in multiple areas: install/verify, dev loop, deploy preflight, operate. We need an implementation strategy for applying diagnostics across the journey as is possible.

In discussions we identified confusion opportunities around `doctor` vs. `validate`. Clarification:

- `doctor`: "Is the world around my project/extension healthy?" Covers environment, install, ports, deps, CLI version, account link. Things the user didn't author.
- `validate`: "Is the thing I authored correct?" Covers project/config/extension checked against schema. Pass/fail orientation. Is a sub-command (like `project code format`, `project code validate`) to make it more explicit what is being validated.

**To resolve**:

- what "healthy" means at each key step (prerequisites at install, env/connectivity in the dev loop, go-live checks before deploy, drift/compat in maintain) so diagnostics are one consistent capability, not ad-hoc per command.
- if and how to apply checks at specific workflow points, gated by what's actually knowable there: e.g. local-only checks vs ones needing a shop connection, or checks that depend on env availability. Each point declares which checks it can run and why others are skipped.
- if and how to make checks pluggable by reusing the reproducible pattern identified above, so the same checks can be invoked from different journey steps rather than re-implemented every time

## 3. Scalability & performance

### 3.1 Where the CLI already does/runs things concurrently, for speed

- **Validate / format / fix:** runs all the code checkers at the same time via `errgroup.Group` (cmd/extension/extension_validate.go:112 and the project equivalents); if one fails, it stops the rest.
- **npm installs:** installs dependencies for multiple extensions in parallel, with as many workers as you have CPU cores (`runtime.NumCPU()`, internal/extension/npm.go:70).
- **Asset file hashing:** hashes files using a pool of eight workers at once (asset_config.go:231).
- **DB dump:** dumps multiple database tables at once (`--parallel`), with a cap so it doesn't overload (internal/mysqldump/mysql.go).
- **Worker command:** runs several job consumers at once, rate-limited so they don't flood the system (cmd/project/project_worker.go:86).

### 3.2 Two ways to build admin/storefront assets: a fast way and a slow fallback

- **Fast path (esbuild):** the CLI builds assets itself, in-process, by calling esbuild directly (`api.Build(...)`, internal/esbuild/esbuild.go:130). No separate Node process to spin up.
- **Slow path (webpack fallback):** when esbuild can't be used, the CLI shells out to Node to run webpack, and builds the admin assets first, then the storefront one after the other, per extension (internal/extension/asset_platform.go).

### 3.3 Caching

The CLI remembers past asset builds so it doesn't redo work that hasn't changed.

- The cache key is a fingerprint of the inputs: the Shopware version plus a hash of the source files and config (xxhash, internal/extension/asset_cache.go). If nothing changed, you get a cache hit; if anything changes, the fingerprint changes and it rebuilds. So it can't go stale.
- It's stored on local disk, or in the GitHub Actions cache for CI. Both sit behind one shared `Cache` interface (internal/system/cache_interface.go, `cache_disk.go`, `cache_github_actions.go`). Because it's one interface, adding a new backend (e.g. a shared S3 cache for a CI fleet) is a clean drop-in.

### 3.4 Performance issues/weaknesses

None are broken, but some are slower.

- **Critical:** PHP lint capped to two threads. A hardcoded `runtime.GOMAXPROCS(2)` limits PHP linting to 2 CPU threads no matter how many cores you have. `internal/phplint/lint.go:62` The wasm engine crashed at some point. so that why it's limited.
  - **TODO:** Before taking action, find out whether this is a problem for users.
  - **TODO:** Move https://github.com/shopwareArchive/php-cli-wasm-binaries/ out of archived, as we're actively using it.
- **Medium:** The file-hashing pool uses eight workers instead of scaling to CPU. `internal/extension/asset_config.go:231`
- **Medium:** Code checkers run in parallel with each other, but each one still processes its files one at a time. `internal/verifier/`
- **Medium:** cache is local/CI-only. No shared backend a whole CI fleet can reuse. `internal/system/cache_*`
  - **TODO:** For GH, whole fleet can use it. for gitlab you kinda have to share the directory, so we need to document what to do in GitLab.
- **Low:** untuned HTTP client. Uses Go's defaults: no connection-reuse tuning, no retries. `internal/shop/client.go:37`
  - **TODO:** remove and replace with a general http client for Shopware in Go, wired into https://github.com/shopware/app-sdk-go

A fresh php/composer/node process is started for every call, but it's not possible to reuse them so this isn't an issue. `internal/phpexec`, `internal/npm`

### 3.5 Observability and what we can measure

Since the CLI primarily orchestrates external tools (PHP, Node.js, Docker, etc.) rather than performing significant computation itself, profiling the Go process would provide limited value. The most useful metrics are command duration, step duration, time spent in external processes, and success/failure rates.

The CLI does phone home a little after every command: `tracking.Track` records the command `name`, whether it succeeded, how long it took, the OS, and whether it ran in the TUI (cmd/root.go).

We can build on this signal.

## 4. Deployment Helper: boundary must stay clean

shopware-cli is the tool you run locally and in CI to build, develop, and package. It writes the deploy config and pokes live shops over the API. The Deployment Helper is what runs on the server to actually carry out a deploy. They talk to each other only through config files the CLI writes, never by calling into each other.

In code, this boundary is maintained like so:

- The project config (`.shopware-project.yml`) has a `deployment:` section `ConfigDeployment` covering hooks, extension management, one-time tasks, and staging (internal/shop/config.go:329). The CLI defines and validates that section, but nothing in the CLI ever runs it: only the struct and its schema touch it. The CLI writes the instructions, and Deployment Helper carries them out. Keep it declarative-only on the CLI side.
- Every project the CLI scaffolds automatically pulls in Deployment Helper as a dependency (`require shopware/deployment-helper`, internal/packagist/project_composer_json.go:64).
- The CI files that the CLI generates (`internal/ci`, covering both GitHub Actions and GitLab) hand the actual deploy to Deployment Helper: The GitHub Actions deploy job `github-deploy.yml` calls `shopware/github-actions/project-deployer`; the `deploy.php` recipe calls `vendor/bin/shopware-deployment-helper run` (Deployer task).

### 4.1 Who owns what

The split isn't arbitrary, but follows the question: **where does the work run?** If it happens on your machine or in CI, it's the CLI's. If it happens on the server as part of going live, it's the Helper's. Almost every job lands cleanly on one side.

**Runs locally / in CI → shopware-cli**

- Scaffold a project / `composer.json`
- Build theme/admin assets
- Validate / lint / format
- Package an extension zip / changelog
- Dump the database
- Upload an extension to the Account/shop
- Write + validate the `deployment:` config — but only writes it; the Helper is what runs it

**Runs on the server at deploy time → Deployment Helper**

- Run migrations / `system:install`
- Compile the theme on the server
- Run the deploy lifecycle hooks

### 4.2 The fuzzy edge we need to watch

Live-shop extension management exists in both tools, but at different layers:

- **CLI:** modifies the project locally. For example, adding a plugin updates the root `composer.json` so the dependency becomes part of the codebase.
- **Deployment Helper:** reconciles the deployed system with the codebase. If a plugin is present but not yet installed, it installs and activates it as part of the deployment process.

In practice, the CLI makes plugins available in the project, while the Deployment Helper ensures the deployed Shopware instance reflects that state. Preserving this distinction keeps project changes and deployment execution clearly separated.

### 4.3 Responsibility of the proposed `deploy` commands

The proposed `project deploy` and `project deploy plan` would act as the bridge between local project state and deployment execution. The CLI prepares and triggers the deployment, while the Deployment Helper remains responsible for executing the deployment workflow.

### 4.4 Inside Deployment Helper

A Symfony Console app that installs or updates a shop on the server, orchestrating a fixed sequence of console commands through one shell-out layer (`ProcessHelper`, imported by 11 files, the most-depended-on class), driven by the `deployment:` config. `RunCommand` is the entrypoint: it checks `isInstalled()` and hands off to `InstallationManager` or `UpgradeManager`. DI is a single Symfony container compiled once (the unified wiring 2.5 wants the CLI to have).

### 4.5 Install / update flows

Install stands a shop up from nothing (`system:install`, admin user, transports, storefront + theme, plugin then app lifecycle, record version); update maintains one (one-time tasks, maintenance mode, `system:update:finish` only if the version changed, plugin/theme refresh, lifecycles, `theme:compile`). Both are one long imperative method — a sequence of shell-outs. Readable now, but the stage list lives in control flow, not data, so it's hard to extend with dry-run, structured reporting, or resumability.

### 4.6 Config, defined twice

Deployment Helper uses both YAML configuration and environment variables. The environment variables are primarily intended as short-lived inputs for installation and deployment workflows, making a reasonable split between file-based configuration and env-based inputs.

### 4.7 Extension seams

`PostDeploy` subscribers (Fastly, Platform.sh, staging, consent, cache) keep provider/env behavior out of the managers. Hooks (pre/post/preInstall/…) run config strings via the shell-out layer. This maintains flexibility, but the escape hatch where behavior moves into shell the PHP can't model it.

One-time tasks are the strongest primitive: run-once scripts keyed by id, tracked in the DB, marked only on success.

### 4.8 Orchestration is where complexity concentrates

The two managers own ordering, branching, command assembly, side effects, and telemetry. Lifecycle ordering is duplicated across install/update (for a reason: plugins carry asset handling, apps don't). Config grows fastest. Failure is implicit: no skipped/retryable/partial model.

The levers: make the implicit pipeline explicit (named stages), a shared lifecycle runner, and a structured step-result, the last also being the prerequisite for the machine-readable output below.

### 4.9 Version-upgrade workflow

In the "deploy/release" stage of our user journey we propose a guided `project upgrade plan " run " status` for Shopware version upgrades, with the run/report executed via the Helper. Today only `upgrade-check` exists. The "upgrade report: upgraded/skipped/failed" it calls for is the structured step-result that the previous section notes is missing.

## 5. Supporting AI / agents as first-class consumers

Machine-readable output becomes a design rule, not an afterthought:

- Per-plugin Markdown scan output an agent (or human) can work through (for `plugin validate`)
- Explicit opt-in project skills that produce agent-ready plans, making AI-assisted workflows a deliberate user choice while remaining independent of any specific provider or agent implementation. (See https://github.com/shadcn/improve)
- a natural-language env command that drives local actions (start/stop env, watchers, cache clear, logs) via skills.

### 5.1 Output & interaction need to become consumable and consistent

Two related gaps, both blocking the AI/agent direction and the machine-readable promise:

- **Machine-readable output is ad-hoc, not a contract.** A `--json` flag exists on two commands (`project extension list` / `outdated`), and the validation reporter supports json/github/gitlab/junit/markdown (internal/validation/reporter.go). But there's no consistent output-format convention across the CLI. Most commands print human text only.
- For agents and CI to consume any command, structured output needs to be a standard (a shared `--output=json` honored everywhere it makes sense), not something bolted onto individual commands. This is the same need behind the `doctor` command.
- **Output isn't centralized.** Commands mix ~95 direct `fmt.Print*` calls with ~296 `logging.FromContext` calls. Logging is structured (`zap`) and goes to stderr; the `fmt` calls go to stdout with no level, no format control, and no way to silence or serialize them. The situation is further complicated by native CI/CD output that is heavily used by commands such as `project ci`. That fragmentation is why a clean JSON mode is hard today. There's no single output path to switch. Routing result output through one channel (and reserving stderr for logs) is the prerequisite.
- The interactive/non-interactive split is enforced per command, at the point of prompting (`system.IsInteractionEnabled`), and only seven non-test files check it. So headless mode holds by convention, not structurally. A new command can forget the check and silently break it.

## 6. Logging

The logger is intentionally optimized for human consumption. The logger is human-oriented (colored, dev-style encoder); `zap` already supports JSON via `NewProductionConfig`, so a `--log-format=json` is a small lever once output is centralized.

## 7. Parity between interactive/non-interactive

The design depends on every command working in both contexts: interactive (the TUI prompts for input) and non-interactive (everything passed as flags, no prompts, the mode CI and agents use). Protect that with a CI gate: run every command with `--no-interaction` in tests, so a command can't quietly lose its non-interactive mode without a test catching it.
