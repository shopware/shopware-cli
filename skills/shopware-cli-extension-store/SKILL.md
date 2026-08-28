---
name: shopware-cli-extension-store
description: MUST use for Shopware Store publication/readiness/submission/compliance questions, including "prepare this plugin/extension for the Shopware Store", "what needs to change before Store publication", Store listing metadata, Store-compliance validation, packaging, or upload readiness. Verify the live workspace, the checkout-built CLI when present, current CLI behavior, and current official Store docs before declaring requirements.
---

# Shopware Store Readiness

Use this skill whenever the user asks whether a Shopware extension/plugin is ready for the Shopware Store or what must change before publication.

The goal is an evidence-backed readiness assessment, not a generic Store checklist.

## Non-negotiable rules

1. Inspect the **live working tree** before describing files, metadata, or image dimensions.
2. If an executable sibling checkout binary exists at `../shopware-cli/bin/shopware-cli`, use it as the authoritative CLI for the assessment unless the user explicitly asks to test the installed release. This Store-readiness rule overrides a generic preference for the PATH binary.
3. Use the same authoritative binary for version, help, schema, normal validation, Store-compliance validation, and packaging/account command discovery.
4. Fresh runtime output outranks saved validation reports and earlier agent output.
5. Do not modify project files during an inspection-only request.
6. A schema field being supported does **not** make it required.
7. A Store listing requirement does **not** prove that the value must live in `.shopware-extension.yml`.
8. Preserve the strength of official wording: `must`/`required` may establish a requirement; `should`/`prefer`/`recommended` is guidance, not a blocker.
9. Never infer non-enforcement from a passing fixture when the relevant item is present.
10. Strong words such as `required`, `must`, `blocker`, `critical`, `reject`, `will fail`, or `non-negotiable` require current CLI enforcement or explicit current official Store documentation.

## Mandatory read-only assessment workflow

Follow this sequence before answering a Store-readiness request.

### 1. Inspect the current extension

Verify live state before making claims:

```bash
pwd
find . -maxdepth 5 -type f \
  -not -path './vendor/*' \
  -not -path './node_modules/*' \
  -not -path './.git/*' | sort
cat composer.json
[ -f .shopware-extension.yml ] && cat .shopware-extension.yml
[ -e src/Resources/config/plugin.png ] && file src/Resources/config/plugin.png
```

Use explicit checks for any path you plan to call missing:

```bash
test -f README.md && echo 'README present' || echo 'README absent'
test -f CHANGELOG.md && echo 'CHANGELOG present' || echo 'CHANGELOG absent'
test -f LICENSE && echo 'LICENSE present' || echo 'LICENSE absent'
test -f src/Resources/config/plugin.png && echo 'plugin icon present' || echo 'plugin icon absent'
```

Do not infer missing screenshots merely because no `store.images` field exists; inspect the current tree. Conversely, finding no local Store screenshots does not by itself prove the remote Store listing has none.

### 2. Resolve the authoritative CLI

Always inspect both the PATH CLI and the sibling checkout binary when present:

```bash
command -v shopware-cli
shopware-cli --version

if [ -x ../shopware-cli/bin/shopware-cli ]; then
  ../shopware-cli/bin/shopware-cli --version
fi
```

If `../shopware-cli/bin/shopware-cli` exists and is executable, use it for the rest of this Store-readiness assessment unless the user explicitly asked to assess the installed release.

Report both the PATH binary/version and which binary is authoritative when they differ.

Do not silently use Homebrew/PATH `shopware-cli` when the sibling checkout binary exists.

### 3. Run fresh validation

Using the authoritative binary:

```bash
<cli> extension validate . --reporter markdown
<cli> extension validate --help
```

If `--store-compliance` is supported and Store compliance is not already enabled in YAML, run it read-only:

```bash
<cli> extension validate . --store-compliance --reporter markdown
```

Do not edit `.shopware-extension.yml` merely to enable the mode during an inspection-only request.

`validation.store_compliance: true` is useful persistent project configuration for Store-intended extensions. Treat persistence as configuration guidance unless current Store docs explicitly make it a submission requirement.

Issue #1407 tracks deprecating the flag. Do not call the flag already deprecated unless current help/runtime says so.

### 4. Inspect the current schema

```bash
<cli> extension config-schema
```

Only claim a field exists if the current schema shows it. Do not invent fields from memory.

Important distinction:

- `store.*` fields are a **local CLI-supported representation** that can be used to synchronize Store information.
- Their presence in the schema does not prove that each field is mandatory.
- Absence of a local `store:` block does not prove the Shopware Account / remote Store listing lacks those values.

In the current CLI implementation, `account producer extension info push` maps local metadata and optional `store.*` values to the Shopware Account when those values are present. Therefore `.shopware-extension.yml` is a synchronization mechanism, not evidence that Store listing requirements can only be satisfied there.

If remote Account state has not been read, describe Store listing completeness as **not verified**, rather than `missing`.

### 5. Verify current official Store documentation

Before classifying a publication requirement, read the relevant current official Shopware documentation.

Useful starting points:

- <https://developer.shopware.com/docs/guides/development/testing/store/quality-guidelines.html>
- <https://developer.shopware.com/docs/guides/development/testing/store/content-and-translations.html>
- <https://developer.shopware.com/docs/guides/development/testing/store/code-quality.html>
- <https://developer.shopware.com/docs/guides/development/testing/store/functionality-integration.html>
- <https://developer.shopware.com/docs/guides/development/testing/store/store-review-errors.html>
- <https://developer.shopware.com/docs/guides/development/testing/store/cookies-and-privacy.html>
- <https://developer.shopware.com/docs/guides/development/testing/store/installation-and-cleanup.html>

Do not fill documentation gaps from memory.

## Evidence classifications

Classify every actionable finding as exactly one of:

- **CLI-enforced** — a fresh validation run fails because of it, or current source/tests directly establish enforcement.
- **Store-doc-required** — current official Store docs explicitly use mandatory wording or otherwise clearly establish the requirement.
- **Store-doc-guidance** — current official Store docs say `should`, `prefer`, `recommended`, or otherwise present non-mandatory guidance.
- **Schema-supported, not proven required** — current schema supports the field/shape, but CLI enforcement and Store docs do not establish it as mandatory.
- **Recommendation / inference** — sensible product-quality or workflow advice without authoritative requirement evidence.

Preserve the classification everywhere in the response. Do not classify something as guidance/recommendation and later turn it into an unconditional `Add`, `Fix`, or `Replace` step.

## Requirement location: do not confuse content with YAML

For every **Store-doc-required** item, state where the requirement applies:

- **extension artifact** — e.g. a file/path that Store docs explicitly require in the extension package;
- **Store listing / Shopware Account** — listing content that must exist for publication but may be managed remotely;
- **CLI configuration** — only when current CLI semantics or docs explicitly require local configuration.

A documentation requirement for Store listing text does not automatically mean `.shopware-extension.yml` must contain `store.description`, `store.default_locale`, `store.availabilities`, etc.

If the Store listing itself was not queried, do not report those remote values as missing. You may say the local CLI sync configuration does not currently provide them.

## Current Store documentation facts to preserve precisely

For the current official `Content and translations` guidance:

- All extensions must be published in the **International Store**; German Store publication is optional.
- If also published in the German Store, English/German content must have 1:1 parity.
- Store listing short description is **150–185 characters**.
- Store listing long description is **at least 200 characters**.
- Listing text must meaningfully and accurately describe the extension/use cases and include clear setup/configuration instructions.
- For images, the docs say at least one Storefront and one Administration screenshot **should** show the main features. Preserve `should`: classify this as **Store-doc-guidance**, not a hard blocker unless another authoritative source makes it mandatory.
- The docs specify a valid favicon named `plugin.png` at `src/Resources/config/plugin.png` with **112x112px**.

Do not transform these Store listing/content requirements into claims that particular `.shopware-extension.yml` fields are mandatory unless a source separately establishes that.

## Current CLI behavior to preserve precisely

For the current implementation associated with this skill:

- normal extension validation includes normal metadata and plugin-icon validation;
- a missing normal plugin icon can produce `metadata.icon`;
- the current normal icon validator accepts dimensions from **112x112 through 256x256**, with maximum file size **30 KB**;
- therefore a 128x128 `plugin.png` can pass current CLI validation while still differing from current Store documentation that specifies 112x112;
- Store-compliance mode gates Administration/Storefront source/build pairing checks such as `assets.administration.sources_missing`;
- `--full` is separate from Store-compliance mode and adds external/full validation tools when supported.

When CLI and Store docs differ, report both. Do not overwrite one with the other.

## Known traps

Never repeat these mistakes:

- Do not label screenshots **CLI-enforced** merely because Store docs discuss screenshots. If Store-compliance validation passes without them, that is direct evidence they are not a current CLI finding on that tree.
- Do not call `README.md`, `CHANGELOG.md`, or a physical `LICENSE` file Store-required unless current official Store docs explicitly establish that.
- Do not call `store.type`, `store.default_locale`, `store.availabilities`, `store.localizations`, or Store metadata fields **Store-doc-required local YAML** solely because the schema supports them.
- Do not infer remote Store listing data is missing from absence of the local `store:` block.
- Do not turn `store.icon`'s schema description (256x256) into the normal `plugin.png` requirement.
- Do not say a 128x128 normal plugin icon is fully Store-compliant merely because the CLI accepts it; current Store docs specify 112x112.
- Do not invent arbitrary icon sizes such as 500x500.
- Do not invent Store config fields such as `category` or `keywords` unless current schema/docs show them.
- Do not treat top-level `changelog` build/generation configuration as proof a Store listing changelog is mandatory.
- Do not require translation files merely because `composer.json` contains translated metadata.
- Do not require `services.xml`/`services.yaml` unless the extension actually defines services that require configuration.
- Do not call placeholder URLs/branding formal blockers without an authoritative source. They may still be recommendations.
- Do not pad the assessment with speculative `Compliant` rows such as prohibited-code scans unless they are directly relevant to the user's question or current validation findings.
- Do not present packaging/upload as the immediate next step until readiness is established and the user asks for submission actions.
- Do not invent CLI commands; inspect current `--help` first.

## Required response format

Start with **Validation status**:

- PATH CLI binary/version;
- authoritative CLI binary/version;
- normal validation result;
- Store-compliance validation result;
- files modified: yes/no.

Then provide only evidence-backed actionable findings:

| Finding | Current evidence | Classification | Applies to | Action |
| --- | --- | --- | --- | --- |

Then exactly these sections when applicable:

### Required changes

Only **CLI-enforced** and **Store-doc-required** items go here.

For Store listing/Account requirements, distinguish `requirement exists` from `current remote state not verified`.

If no required local changes are demonstrated, say so explicitly. Do not manufacture a local YAML change just to populate this section.

### Guidance / recommendations / items to verify

Put **Store-doc-guidance**, **Schema-supported, not proven required**, and **Recommendation / inference** items here. Keep wording conditional.

Do not end with an unconditional generic numbered `Next steps` list that re-promotes recommendations into requirements.

## Final self-audit

Before answering, verify all of these:

1. Every missing/present claim was checked in the live tree.
2. Every dimension/size claim came from current file inspection or an authoritative source.
3. If `../shopware-cli/bin/shopware-cli` existed, it was used as the authoritative binary unless the user explicitly requested the installed release.
4. One authoritative CLI binary was used consistently after selection.
5. Validation output is fresh.
6. No passing fixture was used as proof of non-enforcement.
7. Every Store requirement claim reflects the exact strength of current official wording (`must` vs `should`).
8. No remote Store listing value was declared missing merely because local YAML lacks a field.
9. No schema-supported field was promoted to mandatory without separate evidence.
10. Normal `plugin.png` and `store.icon` stayed separate.
11. The 112x112 Store-doc favicon rule and the CLI's 112–256 accepted range were reported distinctly when relevant.
12. README/CHANGELOG/LICENSE, category/keywords, translation files, or services config were not invented as requirements.
13. Recommendations stayed recommendations in actions and summaries.
14. No files were modified during inspection-only work.

If any check fails, correct the answer before sending it.
