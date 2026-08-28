---
name: shopware-cli-extension-store
description: MUST use for Shopware Store publication/readiness/submission/compliance questions, including "prepare this plugin/extension for the Shopware Store", "what needs to change before Store publication", Store listing metadata, Store-compliance validation, packaging, or upload readiness. Base Store claims on current sources and expose those sources to the user.
---

# Shopware Store Readiness

Use this skill whenever the user asks whether a Shopware extension/plugin is ready for the Shopware Store or what must change before publication.

The assessment must be **source-first and traceable**. Do not generate a generic Store checklist from memory.

## Core principle: no source, no Store requirement

Every Store-policy claim must be tied to a current authoritative source and that source must be visible in the final answer.

For each claim, identify the source type:

- **Workspace** — current local file/config inspection.
- **CLI runtime** — fresh output from the selected Shopware CLI binary.
- **CLI source/test** — current `shopware/shopware-cli` implementation or tests when enforcement semantics matter.
- **CLI schema** — current `extension config-schema`; proves support/shape, not publication necessity.
- **Official Store docs** — current Shopware developer documentation; establishes Store requirements/guidance.
- **Recommendation / inference** — explicitly not an authoritative requirement source.

For every **Store-doc-required** or **Store-doc-guidance** finding, include a direct official Shopware documentation link plus the relevant page/section name when possible.

If no authoritative Store source can be named and linked, the claim cannot be classified as Store-doc-required or Store-doc-guidance. Downgrade it to **Recommendation / inference** or say it could not be verified.

Do not cite a broad documentation page for a narrower claim that the page does not actually establish.

## Four different truths

Never collapse these into one:

1. **Workspace truth** — what exists in the current local extension.
2. **CLI truth** — what the selected CLI validates/enforces.
3. **Store-policy truth** — what current official Store docs require or recommend.
4. **Remote-listing truth** — what is configured in the Shopware Account / Store listing.

A clean CLI run proves only CLI truth. It does not prove Store publication readiness.

A local metadata value that fits a Store constraint is only a candidate/source for remote Store data until the remote listing is inspected.

## Mandatory read-only workflow

When asked to inspect only, do not modify files.

### 1. Inspect the live extension

Verify current state before claiming anything is missing/present or has a certain size:

```bash
pwd
find . -maxdepth 5 -type f \
  -not -path './vendor/*' \
  -not -path './node_modules/*' \
  -not -path './.git/*' | sort
cat composer.json
[ -f .shopware-extension.yml ] && cat .shopware-extension.yml

if [ -e src/Resources/config/plugin.png ]; then
  file src/Resources/config/plugin.png
else
  echo 'plugin icon absent'
fi
```

Do not use shell chains where failure of an optional command can falsely print that an existing file is absent.

### 2. Resolve one authoritative CLI deterministically

```bash
command -v shopware-cli
shopware-cli --version

if [ -x ../shopware-cli/bin/shopware-cli ]; then
  echo 'checkout CLI present'
  ../shopware-cli/bin/shopware-cli --version
else
  echo 'checkout CLI absent'
fi
```

If literal `../shopware-cli/bin/shopware-cli` exists, it is authoritative unless the user explicitly asks to test the installed release.

After selecting it, every CLI command in this assessment must use that binary. Do not reconstruct or guess another absolute sibling path. If later commands accidentally use plain `shopware-cli`, rerun them before answering.

### 3. Run fresh validation

```bash
<cli> extension validate . --reporter markdown
<cli> extension validate --help
<cli> extension validate . --store-compliance --reporter markdown
```

Run Store compliance only when supported by current help. Fresh output outranks saved validation reports.

### 4. Inspect current schema

```bash
<cli> extension config-schema
```

Schema presence proves a field is supported, not that it is a Store requirement.

A `store:` block is a supported local synchronization mechanism. Absence of local `store.*` values does not prove the remote Store listing lacks those values.

### 5. Read current official Store documentation

Before declaring a Store requirement or Store guidance item, inspect the relevant current official Shopware source.

Primary sources include:

- [Content and translations](https://developer.shopware.com/docs/guides/development/testing/store/content-and-translations.html)
- [Quality guidelines](https://developer.shopware.com/docs/guides/development/testing/store/quality-guidelines.html)
- [Code quality](https://developer.shopware.com/docs/guides/development/testing/store/code-quality.html)
- [Functionality and integration](https://developer.shopware.com/docs/guides/development/testing/store/functionality-integration.html)
- [Store review errors](https://developer.shopware.com/docs/guides/development/testing/store/store-review-errors.html)
- [Cookies and privacy](https://developer.shopware.com/docs/guides/development/testing/store/cookies-and-privacy.html)
- [Installation and cleanup](https://developer.shopware.com/docs/guides/development/testing/store/installation-and-cleanup.html)

Preserve the wording strength of the source:

- `must`, `required`, or equivalent mandatory wording → may support **Store-doc-required**.
- `should`, `preferred`, `recommended`, or equivalent → **Store-doc-guidance**.

If the docs cannot be accessed or the relevant statement cannot be found, do not substitute memory.

## Evidence classifications

Each actionable finding must be exactly one of:

- **CLI-enforced** — fresh CLI output fails because of it, or current CLI source/tests directly establish enforcement.
- **Store-doc-required** — current official Store docs explicitly establish the requirement.
- **Store-doc-guidance** — current official Store docs explicitly provide non-mandatory guidance.
- **Schema-supported, not proven required** — current schema supports it, but no stronger evidence proves it mandatory.
- **Recommendation / inference** — useful advice without authoritative requirement evidence.

Classifications are sticky across the whole response.

### Mandatory source format by classification

- **CLI-enforced:** Source must be the fresh command/result and, when available, the validation identifier; alternatively cite the exact current CLI source/test proving the rule.
- **Store-doc-required:** Source must be a direct official Shopware docs link plus page/section label.
- **Store-doc-guidance:** Source must be a direct official Shopware docs link plus page/section label.
- **Schema-supported:** Source must identify `extension config-schema` and the exact field/path.
- **Recommendation / inference:** Source must say `Recommendation / inference` rather than implying authority.

## Current distinctions to preserve

When current docs/source still establish these facts:

- International Store publication is required; German Store publication is optional.
- German/English content parity applies when publishing in the German Store.
- Store short description is 150–185 characters.
- Store long description is at least 200 characters.
- Store listing text must accurately describe the extension and include clear setup/configuration instructions.
- Shopware Account license must match `composer.json`.
- Official Store docs specify `src/Resources/config/plugin.png` at 112x112 px.
- Current CLI accepts the normal plugin icon from 112x112 through 256x256 and max 30 KB; therefore CLI pass and Store-doc compliance can differ.
- Store screenshots/images are guidance when the docs use `should`; do not promote them to blockers.
- Store-compliance validation currently covers Store-specific asset source/build pairing such as Administration/Storefront built assets without rebuildable source.

Always cite the current source for these claims in the user-facing answer instead of relying on this summary alone.

## Remote Store requirements

Store listing requirements apply to the Shopware Account/listing unless the docs explicitly require a local artifact.

If remote state was not inspected, mark it **remote unverified**. Never say missing, satisfied, pass, compliant, ready for submission, or nothing must change.

A local value can be described as:

> local candidate/source looks compatible, remote unverified

For example, a 165-character `composer.json` description may be a suitable candidate for the Store short description, but does not prove the remote Store short description is compliant.

When remote state is unavailable, explicitly consider applicable required conditions including:

- International Store availability;
- German/English parity when relevant;
- short description 150–185 characters;
- long description at least 200 characters;
- meaningful/accurate listing text;
- setup/configuration instructions;
- English fallback for applicable user-facing settings/error messages;
- Shopware Account license matching `composer.json`;
- local `plugin.png` Store favicon requirement.

Do not omit a remote requirement merely because a local candidate value looks compatible.

## Known traps

Do not:

- call README.md, CHANGELOG.md, or a physical LICENSE file Store-required without an explicit current source;
- treat changelog-generation schema as evidence that CHANGELOG.md is required;
- turn `store.icon`'s 256x256 schema description into the normal `plugin.png` requirement;
- infer Store requirements from schema fields such as `store.type`, `store.default_locale`, `store.availabilities`, or `store.localizations`;
- invent category, keywords, translation-file, services-config, arbitrary icon-size, or test-coverage requirements;
- label screenshots CLI-enforced unless the CLI actually reports them;
- say `Descriptions pass` based only on local composer metadata;
- say `ready for Store submission` because CLI validation passes;
- imply Store metadata must be in `.shopware-extension.yml`; it can be a local synchronization source, while the requirement applies to the resulting Store listing;
- cite a docs home page when the specific claim is not supported by the linked page/section.

## Required response format

Start with **Validation status**:

- PATH CLI binary/version;
- authoritative CLI binary/version;
- normal validation result;
- Store-compliance validation result;
- files modified: yes/no.

Then provide:

| Finding | Current evidence | Classification | Source | Applies to | Action |
| --- | --- | --- | --- | --- | --- |

The **Source** column is mandatory.

Examples:

- `Store-doc-required | [Content and translations — description](https://developer.shopware.com/docs/guides/development/testing/store/content-and-translations.html)`
- `CLI-enforced | fresh: <cli> extension validate . --store-compliance; assets.administration.sources_missing`
- `Schema-supported | <cli> extension config-schema → store.icon`
- `Recommendation / inference | no authoritative requirement source`

Then use:

### Required local changes

Only demonstrated CLI-enforced or Store-doc-required deficiencies whose target is local.

### Required Store conditions to verify

Every applicable Store-doc-required remote condition not actually inspected. If a local candidate exists, show it but keep remote state unverified.

State explicitly when applicable:

> Store publication readiness cannot be confirmed from local validation alone because the required remote Store listing state was not inspected.

### Guidance / recommendations

Store-doc-guidance, schema-supported items, and recommendations only.

### Sources checked

End with a compact list of the authoritative sources actually used in the assessment. Include direct clickable links for every official Store documentation page used and identify relevant CLI evidence, for example:

- [Content and translations](https://developer.shopware.com/docs/guides/development/testing/store/content-and-translations.html) — descriptions, translations, favicon, screenshots.
- `<cli> extension validate . --reporter markdown` — current normal validation.
- `<cli> extension validate . --store-compliance --reporter markdown` — current Store-compliance validation.
- `<cli> extension config-schema` — supported local Store synchronization fields.

Do not list sources that were not actually consulted.

## Final source audit

Before sending the answer, verify:

1. Every Store-doc-required/guidance claim has a direct official source link.
2. The cited page/section really supports the exact claim and wording strength.
3. Every CLI-enforced claim identifies fresh CLI output/identifier or specific current source/test.
4. Every schema claim names the exact schema field/path.
5. Recommendations are clearly labeled as non-authoritative.
6. Local candidate values are not presented as proof of remote listing compliance.
7. Remote Store requirements remain unverified unless the remote listing was inspected.
8. The user can follow the provided links to independently verify each Store-policy claim.
9. No requirement appears in the summary or action column without the same evidence/source level it had in the findings table.
10. No files were modified during inspection-only work.

If any check fails, correct the answer before sending it.
