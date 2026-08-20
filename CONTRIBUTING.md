# Contributing to the Shopware CLI

Thanks for your interest in contributing.

## Before opening a pull request

Small fixes can go straight to a PR. Examples:

- typo fixes
- broken links
- small documentation improvements
- obvious bug fixes with a clear test or reproduction

For anything larger, please open an issue first and describe what you want to change before starting implementation. This includes:

- new features, commands, or subcommands
- changes to existing command behavior
- changes to flags, arguments, defaults, or output format
- larger refactors

This helps us confirm the direction, avoid duplicate work, and keep the project maintainable.

A draft PR is welcome if it helps explain the idea, but feature PRs should generally be discussed before they are reviewed or merged.

## Pull requests

When opening a PR, please include:

- what changed
- why it changed
- how it was tested
- any related issue or discussion

Please keep PRs focused. Smaller PRs are easier to review and merge.

## Development

Branch from `main` unless you work on a large feature or epic that should not
block bug-fix releases. In that case, use a dedicated feature branch (for
example, `next`) as the pull request target branch.

Before submitting, run the relevant checks locally:

```sh
go test ./...
golangci-lint run ./...
```

Add or update tests for bug fixes and new behavior.

### Using mise

This repository includes a `mise.toml` file for managing the recommended Go
and golangci-lint versions and for providing convenient development tasks.

After installing mise, set up the project tools with:

```sh
mise install
```

You can then run the complete local check suite with:

```sh
mise run check
```

Individual tasks are also available:

```sh
mise run format       # Format Go source files
mise run format-check # Check Go formatting (gofmt and gci) without changing files
mise run build        # Build the shopware-cli binary
mise run test         # Run the network-isolated test suite (Linux/macOS only)
mise run test-unit    # Run Go tests without the sandbox wrapper (works everywhere)
mise run vet          # Run go vet
mise run lint         # Run golangci-lint
```

For machine-specific settings, create `mise.local.toml`. This file is ignored
by Git and should not contain secrets that belong in a dedicated secret
manager.

## Reviews

Maintainers may ask for changes, suggest a different direction, or decline a PR if the approach was not discussed beforehand. That is not personal; it is how we keep the project consistent and sustainable.

## Releases

In general releases happen from the default `main` branch with everything included there at the time of release.
If a feature branch (e.g. `next`) is ready for release it needs to be merged back into `main` before the release to be included.
The specific release process is explained in [RELEASING.md](RELEASING.md) and done on demand by the maintainers.
