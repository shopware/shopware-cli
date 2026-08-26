---
name: shopware-cli-extension-store
description: Create Shopware extensions with Store compliance intent. Distinguish private/customer extensions from Shopware Store distribution. Configure validation, metadata, localization, and packaging upfront. Move Store requirements discovery to scaffolding phase, reducing rework and improving path to publication.
---

# Shopware Extension Store Compliance

Build extensions for Shopware Store publication with confidence. Scaffold with Store intent upfront, validate automatically, and resolve compliance requirements early.

## Extension intent: private vs Store

When creating or configuring an extension, clarify its distribution target early:

**Private extensions** (projects, customers, internal use):
- Minimal metadata requirements
- No localization structure required
- Basic validation sufficient
- Internal-focused configuration

**Store extensions** (Shopware Store publication):
- Comprehensive metadata required
- Multi-language support (German, English at minimum)
- Full Store compliance validation
- Professional packaging and presentation

Mixing targets in scaffolding costs rework. Decide at creation time.

## Extension creation with Store intent

### Create a private extension

```bash
shopware-cli extension create my-plugin
```

This generates minimal boilerplate. Add Store metadata later if needs change.

### Configure extension for Store distribution

When creating or updating an extension for Store submission:

1. **Set Store compliance in configuration:**

```yaml
# .shopware-extension.yml
compatibility_date: 2025-08-26

validation:
  store_compliance: true    # Enable Store compliance checks

store:
  type: extension           # or 'theme'
  availabilities:
    - German
    - International
  default_locale: de_DE
  localizations:
    - de_DE
    - en_GB
  # Metadata (required for Store)
  meta_title:
    de: "Plugin Name"
    en: "Plugin Name"
  meta_description:
    de: "Short description in German (max 185 chars)"
    en: "Short description in English (max 185 chars)"
  description:
    de: "Detailed description in German"
    en: "Detailed description in English"
  features:
    de:
      - "Feature 1"
      - "Feature 2"
    en:
      - "Feature 1"
      - "Feature 2"
  icon: Resources/icon.png  # 256x256 px

build:
  zip:
    composer:
      enabled: true
    assets:
      enabled: true
```

2. **Prepare required assets:**

- Icon: 256×256 pixels, placed at `Resources/icon.png`
- Screenshots/gallery images (optional but recommended)
- Translation files for supported localizations

3. **Validate early and often:**

```bash
# Full validation including Store compliance
shopware-cli extension validate . --full
```

## Store compliance validation

Enable Store compliance checking via YAML configuration:

```yaml
validation:
  store_compliance: true
```

This runs automated checks for:

- **Metadata completeness** — title, description, icon presence
- **Localization structure** — translations present for declared localizations
- **Icon validity** — file exists, correct dimensions (256×256 px)
- **Version compatibility** — proper semantic versioning and Shopware compatibility
- **License presence** — LICENSE file required
- **README standards** — documentation for users

### Understanding Store validation categories

**Mechanically verifiable** (automated, non-blocking):
- File structure and naming
- Image dimensions and formats
- Metadata field presence and length
- Localization file syntax

**Manual review required** (not automated):
- Content quality and clarity
- Legal compliance (trademark, copyright)
- Subjective quality decisions
- Functionality review by Shopware team

Automated validation finds structural issues. Manual review evaluates content quality and legal compliance.

### Using `--only` and `--exclude` during development

Run specific validation tools without full checks:

```bash
# Check only Shopware CLI built-in validators
shopware-cli extension validate . --only sw-cli

# Exclude specific tools (e.g., PHPStan if not yet ready)
shopware-cli extension validate . --full --exclude phpstan

# Run only Store compliance checks
shopware-cli extension validate . --full --only sw-cli
```

See `shopware-cli extension validate --help` for available tools.

## Configuration vs command-line flags

**Prefer YAML configuration** for Store compliance settings:

```bash
# ✓ Use YAML config (recommended)
validation:
  store_compliance: true
```

The `--store-compliance` command-line flag is deprecated in favor of configuration. When the flag is removed, YAML config will be required:

```bash
# ✓ Current: use YAML
shopware-cli extension validate .

# ⚠ Deprecated: flag will be removed
shopware-cli extension validate . --store-compliance
```

Store compliance choice is architectural, not transient. Configure it in `.shopware-extension.yml` so it persists across CI runs and team collaboration.

## Store checklist before submission

Ensure your extension meets automatic validation before manual review:

1. **Extension configuration** — `.shopware-extension.yml` is valid
2. **Metadata** — title, description, icon all present and localized
3. **Localization** — translation files exist for declared languages
4. **Icon** — 256×256 px, placed at `Resources/icon.png`
5. **License** — LICENSE file in extension root
6. **Version** — semantic versioning (e.g., 1.0.0)
7. **Shopware compatibility** — `composer.json` has valid `shopware/core` constraint
8. **README** — user-facing documentation present
9. **Code quality** — pass full validation: `shopware-cli extension validate . --full`
10. **Store compliance** — pass Store validation: `shopware-cli extension validate . --full` (with `validation.store_compliance: true`)

## Workflows

### New Store extension from scratch

```bash
# 1. Create extension scaffolding
shopware-cli extension create my-plugin

cd my-plugin

# 2. Set Store intent in config
# Edit .shopware-extension.yml: set validation.store_compliance: true

# 3. Add Store metadata
# Edit .shopware-extension.yml: add store block with metadata

# 4. Prepare assets
# Create Resources/icon.png (256×256 px)

# 5. Validate early
shopware-cli extension validate . --full

# 6. Iterate until validation passes
# Fix metadata, translations, code quality issues

# 7. Submit to Shopware Store when ready
```

### Convert existing extension to Store-ready

```bash
# 1. Initialize or update configuration
shopware-cli extension config init --force

# 2. Enable Store compliance
# Edit .shopware-extension.yml: set validation.store_compliance: true

# 3. Add Store metadata (copy structure from checklist)
# Edit .shopware-extension.yml: add/complete store block

# 4. Run validation to identify gaps
shopware-cli extension validate . --full

# 5. Add missing translations, assets, documentation

# 6. Repeat validation until compliant
```

### CI/CD with Store compliance

For automated validation in CI pipelines:

```bash
# Use YAML config (not flags) for Store compliance
shopware-cli extension validate . --full --reporter junit > validation-results.xml
```

The `--reporter` flag allows machine-readable output for CI systems:
- `json` — parsed programmatically
- `junit` — standard test reporting format
- `github` — GitHub PR inline annotations
- `gitlab` — GitLab MR inline annotations
- `markdown` — markdown report

## Store quality requirements

Before submission, extensions must satisfy comprehensive quality standards across code, functionality, security, and SEO. Official quality guidelines cover:

**Code quality** — PHPStan analysis, forbidden patterns (die, exit, var_dump), logging compliance, JavaScript source readability

**Functional standards** — error handling, performance optimization, clean uninstallation, API integration, cookie consent, privacy compliance

**Storefront compliance** — WCAG accessibility, Lighthouse performance benchmarks, no console errors, CSS/JavaScript best practices

**Security** — no cross-domain messaging without explicit domain targets, proper dependency declaration (no wildcards), no bundled test/version-control files

**SEO & structured data** — proper sitemaps, canonical URLs, schema.org validation, robots.txt compliance

**Packaging** — correct ZIP structure, no development artifacts (test configs, `.git`, `.github`, build files), no `composer.lock` in archive

**Admin integration** — main menu guidelines, validation message standards, media folder organization

**Content & translations** — accurate descriptions and metadata, complete translations for declared localizations

Refer to the official Store quality guidelines for the complete ruleset:

- <https://developer.shopware.com/docs/guides/development/testing/store/quality-guidelines.html> — comprehensive standards overview
- <https://developer.shopware.com/docs/guides/development/testing/store/code-quality.html> — code analysis and prohibited patterns
- <https://developer.shopware.com/docs/guides/development/testing/store/not-allowed-store-behaviors.html> — forbidden behaviors and rejection criteria
- <https://developer.shopware.com/docs/guides/development/testing/store/store-review-errors.html> — common submission failures and fixes

Automated validation (`shopware-cli extension validate . --full`) catches many structural issues. Manual review evaluates content quality, legal compliance, and subjective decisions.

## Troubleshooting Store validation

### Validation fails, unclear why

1. **Enable verbose output:**
   ```bash
   shopware-cli extension validate . --full --verbose
   ```

2. **Check which tools ran:**
   Verbose output shows which validation tools executed, which were skipped, and why.

3. **Inspect configuration:**
   ```bash
   shopware-cli extension config-schema  # View all available fields
   ```

4. **Validate config syntax:**
   ```bash
   shopware-cli extension config init --force  # Regenerate if corrupted
   ```

### "Metadata missing" errors

Ensure `.shopware-extension.yml` has complete `store` block:

```yaml
store:
  type: extension
  default_locale: de_DE
  availabilities:
    - German
    - International
  localizations:
    - de_DE
    - en_GB
  meta_title:
    de: "Your Plugin Name"
    en: "Your Plugin Name"
  meta_description:
    de: "German description"
    en: "English description"
  description:
    de: "Longer German text"
    en: "Longer English text"
  features:
    de: ["Feature 1", "Feature 2"]
    en: ["Feature 1", "Feature 2"]
```

All localizations declared must have corresponding translation fields (de, en, etc.).

### Icon validation fails

Check:
- File exists at `Resources/icon.png` (relative to extension root)
- Dimensions exactly 256×256 pixels
- Format: PNG
- File size reasonable (< 1 MB)

### Localization mismatch errors

Declared localizations must have translation keys present:

```yaml
store:
  localizations:
    - de_DE    # Must have `de` translations
    - en_GB    # Must have `en` translations
  
  # These must match:
  meta_title:
    de: "..."
    en: "..."
```

## Extension vs app workflows

This skill covers **extensions** (plugins and themes).

For **Shopware Apps**, use platform-specific documentation and the `shopware-cli app` commands. Apps have separate Store submission requirements and configuration.

## Information priority

When behavior is unclear:

1. Current `.shopware-extension.yml` and `shopware-cli extension config-schema`
2. `shopware-cli extension validate --help`
3. `shopware-cli extension config --help`
4. Official Shopware extension developer documentation
5. `shopware/shopware-cli` source code for implementation details

Do not infer Store requirements from older documentation or outdated extensions. Always verify against the current schema.

## See also

- `shopware-cli` skill for general CLI guidance
- Shopware developer documentation for extension architecture
- **Store submission & quality** — <https://developer.shopware.com/docs/guides/development/testing/store/>
  - Quality Guidelines: <https://developer.shopware.com/docs/guides/development/testing/store/quality-guidelines.html>
  - Code Quality: <https://developer.shopware.com/docs/guides/development/testing/store/code-quality.html>
  - Review Errors: <https://developer.shopware.com/docs/guides/development/testing/store/store-review-errors.html>
  - Not-Allowed Behaviors: <https://developer.shopware.com/docs/guides/development/testing/store/not-allowed-store-behaviors.html>
  - Functionality & Integration: <https://developer.shopware.com/docs/guides/development/testing/store/functionality-integration.html>
  - Performance & Errors: <https://developer.shopware.com/docs/guides/development/testing/store/storefront-performance-and-errors.html>
  - SEO & Structured Data: <https://developer.shopware.com/docs/guides/development/testing/store/seo-and-structured-data.html>
  - Cookies & Privacy: <https://developer.shopware.com/docs/guides/development/testing/store/cookies-and-privacy.html>
  - Content & Translations: <https://developer.shopware.com/docs/guides/development/testing/store/content-and-translations.html>
  - Installation & Cleanup: <https://developer.shopware.com/docs/guides/development/testing/store/installation-and-cleanup.html>
  - FAQ: <https://developer.shopware.com/docs/guides/development/testing/store/faq.html>
- **CLI Store commands** — <https://developer.shopware.com/docs/products/tools/cli/command-types.html#store-commands>
- Shopware Store for published extension examples
