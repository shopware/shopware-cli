---
name: shopware-cli-extension-store
description: MUST use for Shopware Store publication/readiness/submission/compliance questions, including "prepare this plugin/extension for the Shopware Store", "what needs to change before Store publication", Store listing metadata, Store-compliance validation, packaging, or upload readiness. Start immediately with safe read-only checks, resolve the CLI from the workspace, and base every Store claim on current sources exposed to the user.
---

# Shopware Store Readiness

Use this skill whenever the user asks whether a Shopware extension/plugin is ready for the Shopware Store or what must change before publication.

The assessment must be **source-first, traceable, and execution-first**. Do not generate a generic Store checklist from memory and do not stop for preflight questions that the workspace can answer.

## Default execution: proceed, do not ask preflight questions

A Store-readiness request already provides enough intent to begin a **read-only local assessment**.

Do not ask the user to choose:

- local readiness vs. remote listing inspection;
- PATH CLI vs. a nearby checkout CLI;
- whether to start the assessment;
- whether to inspect ordinary local files;
- whether to run read-only validation commands.

Resolve these automatically:

1. Inspect the local extension immediately.
2. Detect the CLI using the deterministic rule below.
3. Run fresh normal and Store-compliance validation read-only.
4. Inspect the current schema and current official Store documentation.
5. If remote Shopware Account/listing state is not already available through an authenticated read-only path, continue without it and mark those conditions **remote unverified**.

Do not ask for credentials or account access merely to complete the local assessment. If the user explicitly asks for remote listing inspection and authenticated read access is unavailable, explain that remote state could not be inspected and continue with all local/source-backed findings.

Only ask a clarifying question when a genuinely unresolved choice would materially change the requested work and cannot be determined from the workspace or the user's prompt.

## Core principle: no source, no Store requirement

Every Store-policy claim must be tied to a current authoritative source, and that source must be visible in the final answer.

Source types:

- **Workspace** — current local file/config inspection.
- **CLI runtime** — fresh output from the selected Shopware CLI binary.
- **CLI source/test** — current `shopware/shopware-cli` implementation or tests when enforcement semantics matter.
- **CLI schema** — current `extension config-schema`; proves support/shape, not publication necessity.
- **Official Store docs** — current Shopware developer documentation; establishes Store requirements/guidance.
- **Recommendation / inference** — explicitly not an authoritative requirement source.

For every **Store-doc-required** or **Store-doc-guidance** finding, include a direct official Shopware documentation link plus the relevant page/section name when possible.

If no authoritative Store source can be named and linked, the claim cannot be classified as Store-doc-required or Store-doc-guidance. Downgrade it to **Recommendation / inference** or state that it could not be verified.

Do not cite a broad documentation page for a narrower claim that the page does not establish.

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

Run this without asking the user which binary to use:

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

Selection rule:

- If literal `../shopware-cli/bin/shopware-cli` exists, it is authoritative unless the user explicitly asks to assess the installed release.
- After selecting it, every CLI command in this assessment must use that binary.
- Do not reconstruct or guess another absolute sibling path.
- If later commands accidentally use plain `shopware-cli`, rerun them with the checkout binary before answering.
- Fall back to PATH only when the literal sibling check fails.

Report both PATH and authoritative versions when they differ.

### 3. Run fresh validation

Using the authoritative binary:

```bash
<cli> extension validate . --reporter markdown
<cli> extension validate --help
```

If current help supports `--store-compliance`, run:

```bash
<cli> extension validate . --store-compliance --reporter markdown
```

Fresh output outranks saved validation reports and earlier agent output.

Do not edit `.shopware-extension.yml` just to activate Store compliance during an inspection-only request.

### 4. Inspect current schema

```bash
<cli> extension config-schema
```

Schema presence proves a field is supported, not that it is a Store requirement.

A `store:` block is a supported local synchronization mechanism. Absence of local `store.*` values does not prove the remote Store listing lacks those values.

### 5. Read current official Store documentation

Before declaring a Store requirement or guidance item, inspect the relevant current official Shopware source.

Primary sources include:

- [Content and translations](https://developer.shopware.com/docs/guides/development/testing/store/content-and-translations.html)
- [Quality guidelines](https://developer.shopware.com/docs/guides/development/testing/store/quality-guidelines.html)
- [Code quality](https://developer.shopware.com/docs/guides/development/testing/store/code-quality.html)
- [Functionality and integration](https://developer.shopware.com/docs/guides/development/testing/store/functionality-integration.html)
- [Store review errors](https://developer.shopware.com/docs/guides/development/testing/store/store-review-errors.html)
- [Cookies and privacy](https://developer.shopware.com/docs/guides/development/testing/store/cookies-and-privacy.html)
- [Installation and cleanup](https://developer.shopware.com/docs/guides/development/testing/store/installation-and-cleanup.html)

Preserve source wording strength:

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

- **CLI-enforced:** fresh command/result and, when available, the validation identifier; or exact current CLI source/test.
- **Store-doc-required:** direct official Shopware docs link plus page/section label.
- **Store-doc-guidance:** direct official Shopware docs link plus page/section label.
- **Schema-supported:** `extension config-schema` plus exact field/path.
- **Recommendation / inference:** explicitly say there is no authoritative requirement source.

## Current distinctions to preserve

Always verify these against the current source before reporting them:

- International Store publication is required; German Store publication is optional.
- German/English content parity applies when publishing in the German Store.
- Store short description is 150–185 characters.
- Store long description is at least 200 characters.
- Store listing text must accurately describe the extension and include clear setup/configuration instructions.
- Shopware Account license must match `composer.json`.
- Official Store docs specify `src/Resources/config/plugin.png` at 112x112 px.
- Current CLI accepts the normal plugin icon from 112x112 through 256x256 and max 30 KB; CLI pass and Store-doc compliance can therefore differ.
- Store screenshots/images are guidance when the docs use `should`; do not promote them to blockers.
- Store-compliance validation covers Store-specific checks such as Administration/Storefront asset source/build pairing when current CLI source/runtime establishes it.

Expose the actual source in the final answer instead of relying on this summary alone.

## Remote Store requirements

Store listing requirements apply to the Shopware Account/listing unless docs explicitly require a local artifact.

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

- ask “local or remote?” before starting; default to the complete local assessment and mark remote state unverified;
- ask which CLI to use; resolve it with the sibling-checkout rule;
- ask for confirmation before ordinary read-only inspection/validation;
- say “need before proceeding” when the needed information can be discovered from the workspace;
- call README.md, CHANGELOG.md, or a physical LICENSE file Store-required without an explicit current source;
- treat changelog-generation schema as evidence that CHANGELOG.md is required;
- turn `store.icon`'s 256x256 schema description into the normal `plugin.png` requirement;
- infer Store requirements from schema fields such as `store.type`, `store.default_locale`, `store.availabilities`, or `store.localizations`;
- invent category, keywords, translation-file, services-config, arbitrary icon-size, or test-coverage requirements;
- label screenshots CLI-enforced unless the CLI actually reports them;
- say `Descriptions pass` based only on local composer metadata;
- say `ready for Store submission` because CLI validation passes;
- imply Store metadata must be in `.shopware-extension.yml`; it is a possible synchronization source while the requirement applies to the resulting Store listing;
- cite a docs home page when the specific claim is not supported by the linked page/section.

## Required response format

Start with **Validation status**:

- PATH CLI binary/version;
- authoritative CLI binary/version;
- normal validation result;
- Store-compliance validation result;
- remote Store listing state: inspected / not inspected;
- files modified: yes/no.

Then provide:

| Finding | Current evidence | Classification | Source | Applies to | Action |
| --- | --- | --- | --- | --- | --- |

The **Source** column is mandatory.

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

End with a compact list of authoritative sources actually used. Include direct clickable links for every official Store documentation page used and identify relevant CLI evidence, for example:

- [Content and translations](https://developer.shopware.com/docs/guides/development/testing/store/content-and-translations.html) — descriptions, translations, favicon, screenshots.
- `<cli> extension validate . --reporter markdown` — current normal validation.
- `<cli> extension validate . --store-compliance --reporter markdown` — current Store-compliance validation.
- `<cli> extension config-schema` — supported local Store synchronization fields.

Do not list sources that were not actually consulted.

## Final source and execution audit

Before sending the answer, verify:

1. The assessment started without unnecessary user confirmation.
2. The CLI was selected from workspace evidence, not by asking the user.
3. If remote state was unavailable, the assessment still completed and marked it remote unverified.
4. Every Store-doc-required/guidance claim has a direct official source link.
5. The cited page/section supports the exact claim and wording strength.
6. Every CLI-enforced claim identifies fresh CLI output/identifier or specific current source/test.
7. Every schema claim names the exact schema field/path.
8. Recommendations are clearly non-authoritative.
9. Local candidate values are not proof of remote listing compliance.
10. No requirement appears in the summary/action column at a stronger evidence level than in the findings table.
11. No files were modified during inspection-only work.

If any check fails, correct the answer before sending it.
