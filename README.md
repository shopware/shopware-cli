# Shopware CLI

[![Hosted By: Cloudsmith](https://img.shields.io/badge/OSS%20hosting%20by-cloudsmith-blue?logo=cloudsmith&style=flat-square)](https://cloudsmith.com)

Shopware CLI is a command line companion for common Shopware account, project, and extension workflows.

## Table of Contents

- [What it helps with](#what-it-helps-with)
- [Highlights](#highlights)
- [Install](#install)
- [Usage](#usage)
- [Repository Layout](#repository-layout)
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

## Repository Layout

- `cmd/`: Cobra command groups for account, extension, and project workflows
- `internal/`: implementation packages for APIs, build steps, validation, TUI, and utilities
- `.github/`: automation and workflow definitions
- `scripts/`: repository helper scripts
- `env-bridge/`: environment bridge helper entrypoint

## Documentation

- Official docs: <https://developer.shopware.com/docs/products/cli/>

## Contributing

Contributions are welcome. If you want to improve commands, docs, or developer workflows, open an issue or send a pull request.

## License

See [LICENSE](LICENSE).
