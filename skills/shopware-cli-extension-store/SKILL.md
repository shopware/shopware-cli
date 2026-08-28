---
name: shopware-cli-extension-store
description: MUST use for Shopware Store publication/readiness/submission/compliance questions, including "prepare this plugin/extension for the Shopware Store", "what needs to change before Store publication", Store listing metadata, Store-compliance validation, packaging, or upload readiness. Verify the live workspace, current CLI behavior, and current official Store docs before declaring requirements.
---

# Shopware Store Readiness

Use this skill whenever the user asks whether a Shopware extension/plugin is ready for the Shopware Store or what must change before publication.

## Core rule

Never generate a generic Store checklist from memory.

For every readiness assessment, establish facts in this order:

1. **Live workspace** — what files/config actually exist now.
2. **Fresh CLI behavior** — normal and Store-compliance validation using one authoritative binary.
3. **Current CLI schema/source** — what configuration is supported and what the CLI really enforces.
4. **Current official Shopware Store docs** — what publication/manual-review requirements are explicitly documented.
5. **Recommendations** — useful advice that is not proven mandatory.

If evidence is missing, say so. Do not upgrade an assumption into a requirement.

## Mandatory read-only assessment workflow

When the user says to inspect only / not modify files, do not edit the extension, even temporarily.

### 1. Inspect the current extension first

Before saying that a file is missing, present, invalid, or has a particular size/dimension, verify it from the live tree.

Typical checks:

```bash
pwd
find . -maxdepth 4 -type f \
  -not -path './vendor/*' \
  -not -path './node_modules/*' \
  -not -path './.git/*' | sort
cat composer.json
[ -f .shopware-extension.yml ] && cat .shopware-extension.yml
[ -e src/Resources/config/plugin.png ] && file src/Resources/config/plugin.png
```

For specific claims, use explicit existence checks. Never infer "missing" from a checklist.

### 2. Resolve one authoritative CLI binary

```bash
command -v shopware-cli
shopware-cli --version
```

If this project is testing unreleased behavior from a sibling `shopware/shopware-cli` checkout and `../shopware-cli/bin/shopware-cli` exists, prefer that checkout-built binary and use it consistently.

Do not mix an older PATH/Homebrew CLI with a checkout-built CLI in one assessment.

### 3. Run fresh normal validation

```bash
<cli> extension validate . --reporter markdown
```

Fresh output outranks saved `validation.json`, XML, markdown, screenshots, or earlier agent responses.

### 4. Run Store-compliance validation read-only

Inspect help first:

```bash
<cli> extension validate --help
```

If `--store-compliance` is supported and Store compliance is not already enabled in YAML, use the flag for this read-only assessment:

```bash
<cli> extension validate . --store-compliance --reporter markdown
```

Do not edit `.shopware-extension.yml` just to test the mode when the user said not to modify files.

`validation.store_compliance: true` is the persistent project configuration for Store intent. Issue #1407 tracks deprecating the CLI flag; do not claim the flag is already deprecated unless current help/runtime says so.

### 5. Inspect schema only to learn supported configuration

```bash
<cli> extension config-schema
```

A field appearing in the schema means **supported**, not automatically **required**.

Never invent Store fields such as `category`, `keywords`, or `changelog` unless the current schema or current official docs actually show them.

### 6. Verify Store publication claims against current official docs

Before using words such as `required`, `must`, `blocker`, `critical`, `reject`, or `will fail`, verify the claim against either:

- fresh/current CLI enforcement, or
- current official Shopware Store documentation.

Useful official starting points:

- <https://developer.shopware.com/docs/guides/development/testing/store/quality-guidelines.html>
- <https://developer.shopware.com/docs/guides/development/testing/store/content-and-translations.html>
- <https://developer.shopware.com/docs/guides/development/testing/store/code-quality.html>
- <https://developer.shopware.com/docs/guides/development/testing/store/functionality-integration.html>
- <https://developer.shopware.com/docs/guides/development/testing/store/store-review-errors.html>
- <https://developer.shopware.com/docs/guides/development/testing/store/cookies-and-privacy.html>
- <https://developer.shopware.com/docs/guides/development/testing/store/installation-and-cleanup.html>

If current official docs cannot be checked or do not explicitly support a claim, classify it as a recommendation or unconfirmed item.

## Evidence classes

Every proposed change must be exactly one of:

- **CLI-enforced** — fresh validation fails because of it, or current source/tests directly prove enforcement.
- **Store-doc-required** — current official Shopware Store docs explicitly require it.
- **Schema-supported, not proven required** — current schema supports it, but it is not proven mandatory.
- **Recommendation / inference** — useful product-quality advice without proof that it is mandatory.

These classifications are sticky. A recommendation must not later become an unconditional `Add`, `Fix`, `Replace`, `Must`, or `Critical` action.

For schema-supported/recommendation items, use conditional wording such as `consider`, `verify whether`, or `if required for your submission path`.

## Current CLI behavior to keep straight

For the current implementation associated with this skill:

- normal extension validation includes normal metadata checks and plugin icon validation;
- a missing normal plugin icon can produce `metadata.icon`;
- normal plugin icon dimensions are accepted from 112x112 through 256x256, with a maximum file size of 30 KB in the current validator;
- Store-compliance mode gates additional Administration/Storefront source/build pairing checks (`validateAssets`), including cases such as `assets.administration.sources_missing`;
- `--full` is separate from Store-compliance mode and controls additional tools such as PHPStan/ESLint/Stylelint.

Do not infer that the CLI "does not enforce" something merely because the current fixture contains that thing and passes. A passing fixture is not a negative test.

## Known traps — do not repeat these mistakes

- Do **not** claim `README.md`, `CHANGELOG.md`, or a physical `LICENSE` file is Store-required unless current official Store docs explicitly establish it.
- Do **not** claim a normal plugin icon is missing without checking `src/Resources/config/plugin.png`.
- Do **not** turn `store.icon` dimensions into normal-plugin-icon requirements.
- Do **not** recommend arbitrary icon sizes such as 500x500 from memory.
- Do **not** invent `category`, `keywords`, `changelog`, or other Store config fields from memory.
- Do **not** require translation files merely because `composer.json` contains translated labels/descriptions. Report what the CLI/docs actually require.
- Do **not** require `services.xml`/`services.yaml` for an extension that does not define services. An empty plugin class does not imply a missing services configuration.
- Do **not** say placeholder URLs or test branding are formal blockers unless current Store docs explicitly establish that. They may still be sensible recommendations for a real product.
- Do **not** present packaging/upload as the next step in an inspection-only readiness assessment unless readiness has actually been established and the user asked for submission guidance.
- Do **not** invent `shopware-cli extension create` or any command not shown by current `--help`.

## Plugin icon vs Store listing assets

Keep these distinct:

1. `src/Resources/config/plugin.png` — normal plugin icon validated by the CLI.
2. `.shopware-extension.yml` `store.icon` — Store listing configuration when supported by current schema.
3. Store documentation requirements — may specify publication assets independently of CLI validation.

If these sources specify different dimensions, report the difference rather than merging them into one rule.

## Store configuration

`extension config-schema` describes what `.shopware-extension.yml` can contain. It is not itself a Store submission checklist.

If there is no `store:` block, report that as a workspace fact. Do not say the extension "needs" one unless current CLI semantics or Store docs establish that for the relevant submission workflow.

Persisting:

```yaml
validation:
  store_compliance: true
```

is useful for Store-intended extensions so normal local/CI validation consistently includes Store-compliance checks. Treat persistence as configuration guidance unless Store docs explicitly make it a submission requirement.

## Required response format for readiness assessments

Start with a concise **Validation status** that states:

- exact CLI binary/version used;
- normal validation result;
- Store-compliance validation result;
- whether any files were modified.

Then use:

| Finding | Current evidence | Classification | Action |
| --- | --- | --- | --- |

Then include exactly these sections when applicable:

### Required changes

Only include **CLI-enforced** and **Store-doc-required** items.

If none are proven, say:

> No required changes were established by the current CLI checks or the Store requirements verified in this assessment.

### Recommendations / items to verify

Put **Schema-supported, not proven required** and **Recommendation / inference** items here with conditional wording.

Do not produce a generic `Critical / Major / Minor` checklist unless each severity is supported by evidence.

## Final self-check

Before answering, verify:

1. Every `missing` claim came from a live file check.
2. Every dimension/size claim came from current file inspection or authoritative source.
3. One CLI binary was used consistently.
4. Validation output is fresh.
5. No passing fixture was used to prove non-enforcement.
6. Every Store `required` claim has current official documentation or current CLI enforcement.
7. No schema-supported field was promoted to mandatory without evidence.
8. Normal plugin icon and `store.icon` were kept separate.
9. README/CHANGELOG/LICENSE were not invented as requirements.
10. No category/keywords/changelog/services/translation requirement was invented.
11. Recommendations stayed recommendations in the final action/summary.
12. No files were modified if the user requested inspection only.

If any check fails, correct the answer before sending it.
