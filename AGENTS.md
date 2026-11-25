# AGENTS.md

This file provides guidance to AI coding agents when working with code in this
repository.

## Development Commands

### Building and Running

- Prefer using `mise run [task]` if installed, otherwise:
  - **Build the CLI**: `go build -o ./bin/shopware-cli .`
  - **Run directly**: `go run main.go [command]`
  - **Run tests**: `go test ./...`
  - **Run specific test**: `go test ./[package_path]`
  - **Verbose test output**: `go test -v ./...`

### Code Quality

- **Prefer Go 1.24 packages** like slices package
- **Check Go modules**: `go mod tidy`
- **Format code**: `go fmt ./...`
- **Run static analysis**: `go vet ./...`

## Architecture Overview

### Core Command Groups

1. **`account/`** - Shopware Account management (login, companies, extensions)
2. **`extension/`** - Extension development tools (build, validate, AI
   assistance)
3. **`project/`** - Shopware project management (creation, configuration,
   deployment)

### Key Internal Packages

- **`internal/verifier/`** - Code quality tools (PHPStan, ESLint, Twig linting)
- **`internal/account-api/`** - Shopware Account API integration
- **`internal/system/`** - System utilities (PHP/Node detection, filesystem)
- **`internal/packagist/`** - Composer/Packagist integration
- **`internal/llm/`** - AI integration (OpenAI, Gemini, OpenRouter)
- **`internal/html/`** - Twig + HTML parsing and formatting
- **`internal/git/`** - Git operations
- **`internal/ci/`** - CI/CD configuration generation

### Extension System

The CLI supports three extension types:

- **Platform Plugins**: Standard Shopware 6 plugins with `composer.json`
- **Shopware Apps**: App system extensions with `manifest.xml`
- **Shopware Bundles**: Custom bundle implementations

Extension detection is automatic based on file presence. Each type has specific
build, validation, and packaging rules.

### Configuration Files

- **`<user config dir>/shopware-cli/config.yaml`** - Optional user preferences
  (`telemetry`, `no_update_hint`), loaded by `internal/userconfig`; path can be
  overridden with `SHOPWARE_CLI_CONFIG`. Never written by the CLI
- **`.config/shopware-extension.yml` / `.shopware-extension.yml` /
  `.shopware-extension.yaml`** - Extension-specific settings (schema:
  `internal/extension/config_schema.json`, embedded and exposed via
  `shopware-cli extension config-schema`). Only one file is loaded and the first
  is preferred / used for new projects, the others are legacy and still
  supported
- **`.config/shopware-project.yml` / `.shopware-project.yaml` /
  `.shopware-project.yml`** - Project-specific settings (schema:
  `internal/shop/config_schema.json`, embedded and exposed via
  `shopware-cli project config-schema`). Shop URL and Admin API credentials
  belong under `environments`; empty `-e` defaults to `environments.local`.
  Top-level `url` / `admin_api` are deprecated. An environment's `type` is
  `local`, `docker`, or `ssh`; `ssh` targets point at a remote host via
  `ssh.host`/`ssh.user`/`ssh.directory` and run commands through a multiplexed
  SSH connection (database connections are tunneled automatically). Only one
  file is loaded and the first is preferred / used for new projects, the others
  are legacy and still supported

### Project export / import

- `project export` writes a portable archive (`metadata.json`, `database.sql`,
  `public.tar.gz`, `private.tar.gz`) and `project import` restores it; the
  layout is defined in `docs/adr/2026-08-11-project-import-export.md` and
  implemented in `internal/projectarchive`. The database part reuses
  `shop.DumpDatabase`; files are transferred with `tar` through
  `Executor.Command`, so local, Docker, and SSH environments share one path.
  Only the `local` filesystem adapter is supported.

### Deployment

- The `deployment` package defines backend capabilities and initialization
  inputs. `internal/deployment/backend` selects an implementation from project
  configuration. `internal/deployment/ssh` owns SSH transport, release layout,
  and multi-host coordination. The experimental `project deploy` commands
  require the `deployment` build tag.
- Keep remote entrypoints under `internal/deployment/ssh/php/commands`. Put
  shared functions in `php/lib` and load them through `php/bootstrap.php` with
  relative `require_once` paths. Go packages the PHP files with `internal/phar`.
  Reuse `runDeploymentCommand` for one-shot commands and
  `openDeploymentSession` for interactive rollouts.
- Send the PHAR first on stdin. The loader reads exactly the archive length,
  leaving stdin available for initialization JSON or the rollout's artifact
  bytes and protocol messages. Pass only non-sensitive JSON in remote command
  arguments. Keep diagnostics on stderr and command output on stdout. The
  remote PHP needs the `phar` extension and a writable temporary directory.
- Validate `project deploy init --init-config` before remote inspection or
  writes. Keep initialization idempotent and preserve an existing `APP_SECRET`
  unless an explicit replacement is supplied before initialization completes.
  For named-host groups, write on the migration host and verify that every
  host sees the same shared runtime configuration. Use `--init-config -` to
  supply unattended initialization JSON on stdin.
- Keep PHP available locally to run the payload tests, which cover
  initialization, rollout, rollback, and prune. Run deployment checks with
  `mise run test -- -tags deployment ./cmd/project ./internal/deployment/... ./internal/phar`.

## Development Patterns

### Command Structure

Commands follow Cobra CLI patterns with:

- Main command in `cmd/[group]/[group].go`
- Subcommands in `cmd/[group]/[group]_[subcommand].go`
- Service containers for dependency injection

### Testing Strategy

- Unit tests alongside source files (`*_test.go`)
- Use testify assert for test assertions (`github.com/stretchr/testify/assert`)
- Test data in `testdata/` directories
- Integration tests use real extension samples in `testdata/`
- Prefer assert.ElementsMatch on lists to ignore ordering issues
- Use t.Setenv for environment variables
- Use t.Context() for Context creation in tests

### Error Handling

- Use structured logging via `go.uber.org/zap`
- Context-based logging: `logging.FromContext(ctx)`
- Graceful error reporting to users

### Command Conventions

Follow `docs/COMMAND_CONVENTIONS.md` for every command, flag and message. The
short version:

- Short texts are imperative, capitalised, no period; a parent command describes
  the group
- `Use` shows required arguments bare and optional ones in brackets, no angle
  brackets; every leaf command declares `Args`
- Errors are lowercase `cannot <verb> ...: %w` without Go identifiers, hints in
  quotes; log lines are capitalised sentences
- Any failure exits non-zero in every output format; never log success after a
  failed step
- Test the invoked `internal/` function, never the cobra layer, and test
  behaviour, not message wording

### AI Integration

The CLI includes AI-powered features for:

- Twig template upgrades (`extension ai twig-upgrade`)
- Code quality suggestions
- Automated fixes for common issues

LLM providers are configurable (OpenAI, Gemini, OpenRouter) with API key
management.

## Extension Development Workflow

### Building Extensions

```bash
# Build extension (auto-detects type)
shopware-cli extension build

# Watch mode for development
shopware-cli extension admin-watch

# Validate extension
shopware-cli extension validate

# Create distribution package
shopware-cli extension package
```

### Project Management

```bash
# Create new project
shopware-cli project create

# Build assets
shopware-cli project admin-build
shopware-cli project storefront-build

# Development servers
shopware-cli project admin-watch
shopware-cli project storefront-watch
```

## Code Quality Integration

The verifier registers tools through the name-only `Tool` interface.
`CheckTool`, `FixTool`, and `FormatTool` add capabilities; commands select the
relevant capability before applying `--only` or `--exclude`. An unsupported tool
name is an error, and `ToolList[T]` preserves the capability type through
filtering.

- **Checkers**: `builtin` (legacy alias: `sw-cli`), PHPStan, ESLint, Stylelint,
  Storefront Twig, PHP-CS-Fixer, Prettier
- **Fixers**: Rector, ESLint, Stylelint, Symfony XML conversion
- **Formatters**: PHP-CS-Fixer, Prettier

`extension validate` and `project validate` run all checkers by default,
including formatting dry runs. These findings can fail previously passing CI
jobs; use `--exclude-formatters` to skip formatting checks and
`format` to apply formatting. Rector remains a fixer and is not run by
validation. The deprecated `--full` flag remains accepted but has no effect;
use `--only` or
`--exclude` to select checkers. Tools report findings and failures through a
`ToolExecution` helper and return a `ToolResult` with an optional note or skip
reason; the shared runner (`RunTools`) filters and collects findings, and
reports show completed, failed, or skipped outcomes (see
`docs/adr/2026-10-22-unified-tool-results.md`). Structured report schemas stay
compatible; JSON only adds fields. A completed tool can still report source
errors; invocation does not guarantee that files were analyzed or changed.
