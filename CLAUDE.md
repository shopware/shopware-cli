# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Development Commands

### Building and Running
- **Build the CLI**: `go build -o shopware-cli .`
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
2. **`extension/`** - Extension development tools (build, validate, AI assistance)
3. **`project/`** - Shopware project management (creation, configuration, deployment)

### Key Internal Packages
- **`internal/verifier/`** - Code quality tools (PHPStan, ESLint, Twig linting)
- **`internal/account-api/`** - Shopware Account API integration
- **`internal/system/`** - System utilities (PHP/Node detection, filesystem)
- **`internal/packagist/`** - Composer/Packagist integration
- **`internal/llm/`** - AI integration (OpenAI, Gemini, OpenRouter)
- **`internal/twigparser/`** - Custom Twig parsing for validation
- **`internal/html/`** - HTML parsing and formatting
- **`internal/git/`** - Git operations
- **`internal/ci/`** - CI/CD configuration generation

### Extension System
The CLI supports three extension types:
- **Platform Plugins**: Standard Shopware 6 plugins with `composer.json`
- **Shopware Apps**: App system extensions with `manifest.xml`
- **Shopware Bundles**: Custom bundle implementations

Extension detection is automatic based on file presence. Each type has specific build, validation, and packaging rules.

### Configuration Files
- **`.shopware-cli.yaml`** - Global CLI configuration
- **`.shopware-extension.yml`** - Extension-specific settings (schema: `extension/shopware-extension-schema.json`)
- **`.shopware-project.yml`** - Project-specific settings (schema: `shop/shopware-project-schema.json`)

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

### AI Integration
The CLI includes AI-powered features for:
- Twig template upgrades (`extension ai twig-upgrade`)
- Code quality suggestions
- Automated fixes for common issues

LLM providers are configurable (OpenAI, Gemini, OpenRouter) with API key management.

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
shopware-cli extension zip
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

The verifier system provides comprehensive code quality checks:
- **PHP**: PHPStan, PHP-CS-Fixer, Rector
- **JavaScript**: ESLint, Prettier, Stylelint  
- **Twig**: Custom admin Twig linter with auto-fix capabilities
- **Composer**: Dependency validation

Tools are configurable via JSON schemas and run automatically during builds.