# Shopware CLI

[![Hosted By: Cloudsmith](https://img.shields.io/badge/OSS%20hosting%20by-cloudsmith-blue?logo=cloudsmith&style=flat-square)](https://cloudsmith.com)
[![skills.sh](https://skills.sh/b/shopware/shopware-cli)](https://skills.sh/shopware/shopware-cli)

Shopware CLI is a command line companion for common Shopware account, project, and extension workflows.

## Table of Contents

- [What it helps with](#what-it-helps-with)
- [Highlights](#highlights)
- [Install](#install)
- [Usage](#usage)
- [Repository Layout](#repository-layout)
- [Official Agent Skills](#official-agent-skills)
- [Documentation](#documentation)
- [Contributing](#contributing)
- [License](#license)

## What it helps with

Use this CLI to create and set up Shopware projects, manage Shopware accounts, build and validate extensions, automate project maintenance, or run everyday developer tasks without leaving the terminal.

## Highlights

- Installation, initialization, and migration wizards for creating new Shopware projects or moving existing projects into the local development setup
- Integrated Docker-based local development environment with an interactive terminal user interface (TUI)
- Daily development loop support from one place: start environments, run watchers, check logs, access database and mail tools
- Project automation commands for config, cache, admin, storefront, and CI workflows
- Account-related commands under `shopware-cli account`
- Extension build, validation, formatting, changelog, and packaging helpers
- Non-interactive mode for scripts and CI

## Install

### From source with Go

```bash
go install github.com/shopware/shopware-cli@latest
```

### Build locally from this repository

```bash
git clone https://github.com/shopware/shopware-cli.git
cd shopware-cli
go build -o bin/shopware-cli .
```

## Usage

Show the available commands:

```bash
shopware-cli --help
```

Common command groups:

```bash
shopware-cli account --help
shopware-cli extension --help
shopware-cli project --help
```

Generate a CycloneDX SBOM for a project without running the full CI build:

```bash
shopware-cli project sbom
shopware-cli project sbom ./my-shop --format cyclonedx-json --output sbom.json
```

If you need CI-friendly behavior, disable prompts:

```bash
shopware-cli --no-interaction <command>
```

Silence update notifications for a single command:

```bash
shopware-cli --no-update-hint
```

## Configuration

Shop URL and Admin API credentials belong under named `environments` in `.config/shopware-project.yml`. Project-level keys such as `build`, `dump`, `docker`, and `php_version` stay at the top level.

```yaml
environments:
  local:
    type: local
    url: http://127.0.0.1:8000
    admin_api:
      username: admin
      password: shopware
  staging:
    url: https://staging.example.com
    admin_api:
      client_id: ...
      client_secret: ...
```

Omit `-e` / `--env` to target `environments.local`. Use `-e staging` (or another name) to target a different environment.

### Anonymized dumps and extensions

`shopware-cli project dump --anonymize` rewrites personal data in Shopware core tables. An extension can add its own tables and system config secrets in `.config/shopware-extension.yml`. A project dump with `--anonymize` merges those rules from every installed extension. A column already set under `dump.rewrite` in the project config is left unchanged.

```yaml
anonymize:
  tables:
    swag_example_token:
      access_token: "''"
      email: faker.Internet.Email()
  system_config:
    - SwagExample.config.clientSecret
    - key: SwagExample.config.merchantEmail
      value: faker.Internet.Email()
    - key: SwagExample.config.environment
      value: sandbox
```

`tables` uses the same column expressions as the project `dump.rewrite` map. Write `"''"` to store an empty string and `"NULL"` to store NULL. `system_config` matches `system_config.configuration_key`. A plain key omits that row from the dump. `value` replaces `configuration_value` with `{"_value": value}`. A value that starts with `faker.` is generated per row.

`shopware-cli project create` and `shopware-cli project config init` write `environments.local`. Top-level `url` and `admin_api` are deprecated: they are used only when `environments.local` is absent. When both are present, `environments.local` wins.

### Disable update notifications

To disable update notifications for the current shell session, set:

```bash
export SHOPWARE_CLI_NO_UPDATE_NOTIFICATION=true
```

To make the setting **persistent**, add the export command to your shell profile, such as `~/.bashrc` or `~/.zshrc`.

## Repository Layout

- `cmd/`: Cobra command groups for account, extension, and project workflows
- `internal/`: implementation packages for APIs, build steps, validation, TUI, and utilities
- `.github/`: automation and workflow definitions
- `scripts/`: repository helper scripts
- `env-bridge/`: environment bridge helper entrypoint

## Official Agent Skills

Shopware CLI maintainers provide official Agent Skills in this repository's `skills/` folder. These skills teach compatible AI coding agents how to use Shopware CLI workflows safely and correctly. 

Install them with:

```bash
npx skills add shopware/shopware-cli
```

Available skills:

- **`shopware-cli`** — Use Shopware CLI for project, extension, and account workflows. Validate, build, format, and troubleshoot extensions. Reason about command safety and side effects.
- **`shopware-cli-docker`** — For Docker-backed projects. Run commands in the correct container, manage development environment lifecycle, and troubleshoot Docker execution.
- **`shopware-cli-extension-store`** — Assess an extension's readiness for Shopware Store distribution. Read-only: classifies metadata, localization, and asset findings against Store requirements and cites a re-checkable source for each, without modifying files.
- **`shopware-cli-extension-anonymize`** — Fill an extension's `anonymize` section in `.config/shopware-extension.yml` from its tables and system config secrets so `project dump --anonymize` includes them.

## Documentation

- Official docs: <https://developer.shopware.com/docs/products/cli/>

## Contributing

Contributions are welcome. If you want to improve commands, docs, or developer workflows, open an issue or send a pull request.
See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

See [LICENSE](LICENSE).
