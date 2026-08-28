---
name: shopware-cli-extension-store
description: Prepare and validate Shopware extensions for Store distribution. Distinguish normal extension validation, Store-compliance mode, Store listing configuration, packaging, and manual Store review. Classify every proposed change by evidence and do not invent Store requirements that current CLI behavior or authoritative Store documentation does not establish.
---

# Shopware Extension Store Distribution

Use this skill when an extension is intended for Shopware Store distribution or when investigating Store-specific validation, configuration, packaging, or submission behavior.

The key rule is to keep three different concerns separate:

1. **Normal extension validation** — checks run by `shopware-cli extension validate`.
2. **Store-compliance mode** — additional validation behavior enabled by `validation.store_compliance: true` or, while supported, `--store-compliance`.
3. **Store publication requirements** — Store listing data and manual review rules documented by Shopware. These are not automatically CLI-enforced unless current CLI behavior proves that they are.

Do not turn Store documentation, schema fields, recommendations, or remembered conventions into local validation errors unless the current CLI actually enforces them.

## Evidence classification and wording

When telling a user what must change for Store publication, classify **every finding** as exactly one of these:

- **CLI-enforced** — demonstrated by a fresh run of the intended CLI binary or directly established by the current CLI source/tests. Include the command/result or validation identifier when useful.
- **Store-doc-required** — explicitly required by current official Shopware Store documentation. Identify the authoritative documentation supporting the requirement and distinguish it from local CLI validation.
- **Schema-supported, not proven required** — accepted or described by `shopware-cli extension config-schema`, but not shown to be mandatory by CLI validation or Store documentation.
- **Recommendation / inference** — a best practice, product-quality suggestion, likely publication concern, or reasonable inference that is not established as a requirement.

Use strong requirement language only for the first two classes. Words such as **required**, **must**, **must-fix**, **blocker**, **critical**, **rejection**, and **will fail** require evidence from current CLI behavior or an explicit authoritative Store requirement.

If that evidence is unavailable, use wording such as **supported**, **recommended**, **worth reviewing**, **not established as required**, or **cannot be confirmed from the available evidence**.

A field appearing in `config-schema` proves that the configuration shape is supported. It does **not** prove that the field is required for local validation or Store acceptance.

Before calling something **Store-doc-required**, verify the exact current Store documentation. If the documentation is unavailable or does not explicitly establish the requirement, downgrade the finding to **Recommendation / inference** rather than filling the gap from memory.

When the user asks for a readiness assessment, prefer a structure such as:

| Finding | Classification | Evidence | Action |
| --- | --- | --- | --- |
| ... | CLI-enforced / Store-doc-required / Schema-supported / Recommendation | command, identifier, source, or limitation | concrete next step |

Do not create a generic "critical issues" or "required checklist" section unless every item in it has qualifying evidence.

The classification is **sticky across the entire response**. Preserve it in the finding description, issue/impact text, action column, summary, checklist, and next-step list. Do not classify an item as **Recommendation / inference** or **Schema-supported, not proven required** and then silently promote it later with mandatory language or an imperative that implies it is required.

For **Recommendation / inference** and **Schema-supported, not proven required** findings, avoid language such as **reject**, **fail**, **flag**, **block**, **needs**, **must**, **required**, or other wording that implies a verified publication gate. Phrase follow-up actions conditionally, for example: "consider replacing", "verify whether this applies", or "if required by current Store documentation, configure ...".

Before finalizing a readiness assessment, scan the whole answer for requirement language and verify that every strong claim still matches its evidence classification. This applies to prose outside tables as well as headings, summaries, and numbered next steps.

Common traps:

- Do not call `README.md`, `CHANGELOG.md`, or a physical `LICENSE` file required merely because they are common project files. Require explicit current Store documentation before classifying them as Store requirements.
- Do not mark every `store.*` field from the schema as required.
- Keep the normal plugin icon separate from `store.icon`. A Store icon schema description such as 256x256 does not make an otherwise valid `src/Resources/config/plugin.png` undersized.
- Placeholder URLs, placeholder branding, and an empty test plugin may be obvious publication-quality concerns, but do not call them blockers unless current Store documentation explicitly establishes that status.
- Do not infer manual-review rejection criteria from a broad quality guideline unless the source actually says the condition is mandatory or rejecting.

## Start with the authoritative CLI

Before diagnosing Store behavior, establish which CLI binary is authoritative:

```bash
command -v shopware-cli
shopware-cli --version
shopware-cli extension validate --help
shopware-cli extension config-schema
```

When testing unreleased changes from a `shopware/shopware-cli` checkout, prefer the binary built from that checkout over an older `shopware-cli` on `PATH`. Use the exact binary consistently for `--help`, schema inspection, validation, packaging, and account command discovery.

Do not silently mix results from different CLI versions.

## Revalidate the current working tree

Saved reports such as `validation.json`, `validation.xml`, or markdown output may describe an older state of the extension.

Before diagnosing failures:

```bash
shopware-cli extension validate . --reporter json
```

Regenerate the report from the current working tree. If a saved report conflicts with current files or a fresh validation run, treat the saved report as stale.

When the user asks for inspection only or says not to modify files, do not temporarily rewrite project files to probe behavior. Prefer `--help`, `config-schema`, source inspection, or a separate disposable fixture.

## Normal validation vs Store-compliance mode

Run normal validation first:

```bash
shopware-cli extension validate .
```

Enable Store-compliance mode persistently in `.shopware-extension.yml`:

```yaml
validation:
  store_compliance: true
```

Then run validation normally:

```bash
shopware-cli extension validate .
```

The CLI flag is currently another entry point for the same mode:

```bash
shopware-cli extension validate . --store-compliance
```

Prefer the YAML setting for Store-intended extensions because Store intent belongs in project configuration. Issue #1407 tracks deprecating the CLI flag. Do not claim the flag is already deprecated unless the current CLI emits a deprecation warning or current help says so.

### What Store-compliance mode currently adds

Verify this against the current source when version accuracy matters.

In the current implementation, `validation.store_compliance` gates `validateAssets`: Store-compliance mode checks Administration and Storefront source/build pairing, including configured extra bundles.

A concrete example:

- compiled Administration assets under `Resources/public/administration` without an Administration source entrypoint can pass normal validation;
- the same extension fails Store-compliance validation with `assets.administration.sources_missing`.

The inverse is also checked: source entrypoints without expected built assets can fail Store-compliance validation.

Do **not** describe unrelated normal validators as Store-only checks. Metadata, version constraints, license value, plugin icon, and other built-in checks may run during normal validation too.

Do **not** infer that Store-compliance mode has no checks merely because one simple fixture produces zero findings.

## Full validation is a separate dimension

`--full` controls additional validation tools such as PHPStan, ESLint, and Stylelint. It is separate from Store-compliance mode.

Examples:

```bash
# Built-in validation only
shopware-cli extension validate .

# Built-in validation with Store-compliance mode from YAML
shopware-cli extension validate .

# Add external/full validation tools
shopware-cli extension validate . --full
```

If Store compliance is not configured in YAML and the current flag still exists:

```bash
shopware-cli extension validate . --store-compliance
shopware-cli extension validate . --store-compliance --full
```

Use `--only` and `--exclude` only with tool names shown or accepted by the current CLI. Do not invent wildcard tool names.

## Store configuration schema

Use the CLI-generated schema to discover supported `.shopware-extension.yml` fields:

```bash
shopware-cli extension config-schema
```

Current Store configuration can expose fields such as:

- `store.type`
- `store.availabilities`
- `store.default_locale`
- `store.localizations`
- `store.icon`
- `store.meta_title`
- `store.meta_description`
- `store.description`
- `store.installation_manual`
- `store.tags`
- `store.videos`
- `store.highlights`
- `store.features`
- `store.faq`
- `store.images`
- `store.image_directory`
- `store.demo_shops`

The presence of a field in the schema means the CLI supports that configuration shape. It does **not** by itself prove that the field is mandatory for local validation or Store acceptance. Classify such a field as **Schema-supported, not proven required** until stronger evidence exists.

A Store-intended extension may start with:

```yaml
compatibility_date: YYYY-MM-DD

validation:
  store_compliance: true

store:
  type: extension
```

Add Store listing fields according to the extension's publication needs and current Shopware Store documentation. Validate the resulting configuration with the current CLI.

For translated Store fields, use the translation keys accepted by the current schema/config implementation rather than guessing locale-key formats.

## Plugin metadata and icon checks

Keep plugin metadata from `composer.json` distinct from Store listing metadata in `.shopware-extension.yml`.

For Shopware platform plugins, normal CLI validation currently checks plugin metadata such as translated labels/descriptions and the configured/default plugin icon. The default plugin icon path is typically:

```text
src/Resources/config/plugin.png
```

Do not replace this with a Store icon path based only on Store configuration examples.

Do not claim the normal plugin icon must be exactly 256x256. Use current validator output/source for accepted dimensions and file-size limits. If `store.icon` has a different documented dimension, treat it as a separate Store-listing asset rather than evidence that the plugin icon is invalid.

Do not claim `README.md`, `CHANGELOG.md`, or a physical `LICENSE` file is required for Store publication unless current official Store documentation explicitly says so. A declared license value being validated by the CLI is not evidence that a physical license file is required.

## New extensions: do not invent scaffolding commands

Always inspect the current command surface:

```bash
shopware-cli extension --help
```

Do not invent `shopware-cli extension create` or any other scaffolding command that is not present.

If an extension already exists and needs CLI configuration, inspect:

```bash
shopware-cli extension config --help
shopware-cli extension config init --help
```

Use `extension config init` only according to its current behavior. Preserve existing configuration and unrelated user changes.

## Store listing and account commands

Store listing management and upload are account operations, not local validation.

Discover the current command surface before suggesting commands:

```bash
shopware-cli account producer extension --help
shopware-cli account producer extension info --help
shopware-cli account producer extension upload --help
```

Current CLI versions may expose commands for pulling/pushing Store information and uploading an extension ZIP. Treat upload/push operations as remote side effects: do not execute them without explicit user intent.

For packaging, inspect:

```bash
shopware-cli extension package --help
```

Do not claim an extension is ready to upload merely because local validation passes. Local validation and Store acceptance are different gates.

## Manual Store review and official requirements

Shopware's Store quality documentation covers areas such as code quality, functionality/integration, Storefront behavior and performance, security-sensitive behavior, privacy, SEO, translations, packaging, and cleanup.

When presenting these requirements:

- classify each finding using the evidence classes above;
- label findings **Store-doc-required** only when the current official documentation explicitly requires them;
- label CLI findings **CLI-enforced** only when a fresh run or current source proves enforcement;
- distinguish recommendations from rejection criteria;
- do not turn broad quality guidance into a fabricated local checklist;
- do not use "Store review will reject/flag this" unless an authoritative source actually supports that claim;
- keep the same evidence level when restating a finding in recommendations or next steps.

If official Store documentation cannot be checked, say that clearly and keep unverified publication advice in **Recommendation / inference**.

Useful documentation:

- <https://developer.shopware.com/docs/guides/development/testing/store/quality-guidelines.html>
- <https://developer.shopware.com/docs/guides/development/testing/store/code-quality.html>
- <https://developer.shopware.com/docs/guides/development/testing/store/not-allowed-store-behaviors.html>
- <https://developer.shopware.com/docs/guides/development/testing/store/store-review-errors.html>
- <https://developer.shopware.com/docs/guides/development/testing/store/functionality-integration.html>
- <https://developer.shopware.com/docs/guides/development/testing/store/storefront-performance-and-errors.html>
- <https://developer.shopware.com/docs/guides/development/testing/store/seo-and-structured-data.html>
- <https://developer.shopware.com/docs/guides/development/testing/store/cookies-and-privacy.html>
- <https://developer.shopware.com/docs/guides/development/testing/store/content-and-translations.html>
- <https://developer.shopware.com/docs/guides/development/testing/store/installation-and-cleanup.html>

## CI and reporters

For CI, use the reporter supported by the current CLI:

```bash
shopware-cli extension validate . --full --reporter junit > validation-results.xml
```

Reporters format output; they do not automatically post comments to pull requests or merge requests.

Use the process exit code as the validation result: successful validation exits successfully; validation errors make the command fail.

Store-compliance intent should normally live in `.shopware-extension.yml` so local and CI validation use the same mode.

## Troubleshooting Store validation

When results are surprising:

1. Confirm the exact binary with `command -v` and `--version`.
2. If testing a checkout, use its built binary consistently instead of the `PATH` binary.
3. Rerun validation and ignore stale saved reports.
4. Compare normal validation with Store-compliance validation.
5. Inspect `extension validate --help` and `extension config-schema`.
6. Use `--verbose` when it provides useful diagnostics.
7. If behavior is still unclear, inspect the current `shopware/shopware-cli` source and tests.
8. Check current official Store documentation for publication claims.
9. Classify every proposed change before presenting the final assessment.
10. Check summaries and next steps for accidental promotion of recommendations into requirements.

## Information priority

When behavior is unclear, use this order:

1. The intended/current CLI binary and its fresh runtime output.
2. Current `.shopware-extension.yml` and `shopware-cli extension config-schema`.
3. Current `shopware/shopware-cli` source and tests for implementation details.
4. Official Shopware Store documentation for publication and review requirements.
5. Historical reports or older examples only as background, never as authoritative current state.

Prefer version-correct evidence over remembered behavior. If a lower-priority source conflicts with a higher-priority source, explain the conflict rather than silently merging the claims.

## See also

- `shopware-cli` skill for general CLI and validation guidance.
- Issue #1407 for the `--store-compliance` deprecation plan and current mode semantics.
- Shopware Store testing documentation: <https://developer.shopware.com/docs/guides/development/testing/store/>
