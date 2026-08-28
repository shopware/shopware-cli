---
name: shopware-cli-extension-store
description: MUST use for Shopware Store publication/readiness/submission/compliance questions, including "prepare this plugin/extension for the Shopware Store", "what needs to change before Store publication", Store listing metadata, Store-compliance validation, packaging, or upload readiness. Start immediately with safe read-only checks, resolve the CLI from the workspace, and base every Store claim and field mapping on current sources exposed to the user.
---

# Shopware Store Readiness

Use this skill whenever the user asks whether a Shopware extension/plugin is ready for the Shopware Store or what must change before publication.

The assessment must be **execution-first, source-first, and traceable**. Do not generate a generic Store checklist from memory. Do not ask preflight questions that the workspace can answer.

## Default execution: proceed immediately

A Store-readiness request provides enough intent for a read-only local assessment.

Do not ask the user to choose local vs. remote scope, PATH vs. checkout CLI, or whether to start. Instead:

1. inspect the live extension;
2. resolve the authoritative CLI from the workspace;
3. run fresh normal and Store-compliance validation;
4. inspect current schema;
5. inspect current official Store docs;
6. inspect current CLI implementation/source when a Store concept is being mapped to a local field or remote Account field;
7. if remote Shopware Account state is unavailable, finish the assessment and mark those conditions **remote unverified**.

Do not request credentials merely to complete the local assessment.

## Core principle 1: no source, no Store requirement

Every Store-policy claim must be tied to a current authoritative source and that source must be visible in the final answer.

Source types:

- **Workspace** — current local file/config inspection.
- **CLI runtime** — fresh output from the selected Shopware CLI binary.
- **CLI source/test** — current `shopware/shopware-cli` implementation or tests.
- **CLI schema** — current `extension config-schema`; proves support/shape only.
- **Official Store docs** — current Shopware developer documentation.
- **Recommendation / inference** — explicitly non-authoritative.

For every **Store-doc-required** or **Store-doc-guidance** finding, include a direct official Shopware documentation link plus page/section label when possible.

If no authoritative Store source can be named and linked, the claim cannot be Store-doc-required/guidance.

## Core principle 2: no implementation source, no field mapping

A Store documentation requirement describes the required Store result. It does **not** establish which `.shopware-extension.yml` field, `composer.json` field, or Account API field represents that result.

Before saying a Store requirement maps to a particular local field, inspect current CLI schema **and current implementation/source** that performs the synchronization.

This rule is strict:

> A docs source can prove the requirement. A schema source can prove a field exists. Only implementation/source can prove how that field maps to the Store/Account model.

Therefore:

- never infer that Store short description = `store.meta_description` because the names sound similar;
- never infer that Store long description must live in `store.description` merely because the schema supports it;
- never infer that Store availability must be configured locally in `store.availabilities` merely because the schema supports it;
- never turn an optional sync field into a required local change unless current docs or CLI semantics explicitly require local configuration.

For the current `account producer extension info push` implementation, verify these mappings before using them. At the time this skill was authored, current source establishes:

- extension metadata description from `composer.json`/plugin metadata -> remote `ShortDescription`;
- `store.description` -> remote long `Description` when configured;
- `store.installation_manual` -> remote `InstallationManual` when configured;
- `store.meta_title` -> remote `MetaTitle` when configured;
- `store.meta_description` -> remote `MetaDescription` when configured;
- `store.availabilities` -> remote Store availabilities when configured;
- `store.default_locale`, `store.localizations`, `store.type`, images/icon and other `store.*` values are applied only when configured.

Re-read current source before relying on these mappings if the implementation may have changed.

**Important:** `store.meta_description` is not the Store short description unless current implementation explicitly proves that mapping. Do not use SEO/meta fields as substitutes for listing short/long description fields.

## Four different truths

Never collapse these into one:

1. **Workspace truth** — local files/values.
2. **CLI truth** — selected CLI validation/enforcement.
3. **Store-policy truth** — official Store requirements/guidance.
4. **Remote-listing truth** — actual Shopware Account/Store listing state.

A clean CLI run proves only CLI truth. A local metadata value that fits a Store constraint is only a candidate/source for remote Store data until the remote listing is inspected.

## Mandatory read-only workflow

### 1. Inspect the live extension

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

Use explicit probes. Do not let failure of an optional secondary command make an existing file appear absent.

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

If literal `../shopware-cli/bin/shopware-cli` exists, it is authoritative unless the user explicitly asks for the installed release. Use that same binary for all subsequent CLI commands. Do not reconstruct an absolute sibling path.

### 3. Run fresh validation

```bash
<cli> extension validate . --reporter markdown
<cli> extension validate --help
```

If current help supports it:

```bash
<cli> extension validate . --store-compliance --reporter markdown
```

Do not edit YAML merely to activate Store compliance during inspection-only work.

### 4. Inspect current schema

```bash
<cli> extension config-schema
```

Schema presence = supported field, not requirement and not field semantics beyond what the schema explicitly says.

### 5. Inspect implementation when mapping Store concepts to local fields

If the answer discusses how local metadata is synchronized to the Shopware Account, inspect current source, especially the account producer Store push implementation. Do not guess mappings from field names.

For example, inspect the current implementation of `PushExtensionStoreInfo` / `updateStoreInfo` before saying where short description, long description, installation manual, meta fields, availabilities, or localization values come from.

### 6. Read current official Store docs

Primary sources include:

- [Content and translations](https://developer.shopware.com/docs/guides/development/testing/store/content-and-translations.html)
- [Quality guidelines](https://developer.shopware.com/docs/guides/development/testing/store/quality-guidelines.html)
- [Code quality](https://developer.shopware.com/docs/guides/development/testing/store/code-quality.html)
- [Functionality and integration](https://developer.shopware.com/docs/guides/development/testing/store/functionality-integration.html)
- [Store review errors](https://developer.shopware.com/docs/guides/development/testing/store/store-review-errors.html)
- [Cookies and privacy](https://developer.shopware.com/docs/guides/development/testing/store/cookies-and-privacy.html)
- [Installation and cleanup](https://developer.shopware.com/docs/guides/development/testing/store/installation-and-cleanup.html)

Preserve wording strength: `must`/`required` may support Store-doc-required; `should`/`prefer`/`recommended` means Store-doc-guidance.

## Evidence classifications

Each actionable finding must be exactly one of:

- **CLI-enforced** — fresh CLI output fails because of it, or current CLI source/test directly proves enforcement.
- **Store-doc-required** — current official docs explicitly establish it.
- **Store-doc-guidance** — current official docs explicitly provide non-mandatory guidance.
- **Schema-supported, not proven required** — schema supports the field/shape, but no stronger source proves it mandatory.
- **Recommendation / inference** — useful advice without authoritative requirement evidence.

Classifications are sticky.

### Source format by classification

- **CLI-enforced:** fresh command/result + validation identifier when available, or exact source/test.
- **Store-doc-required/guidance:** direct official docs link + relevant page/section.
- **Schema-supported:** `extension config-schema -> exact.field.path`.
- **Recommendation / inference:** say there is no authoritative requirement source.

## Passing validation is not proof of absent-rule enforcement

Never classify an item as **CLI-enforced** merely because Store-compliance validation passed on a project that happens not to contain the prohibited pattern.

Examples: passing validation does not prove that `die`, `exit`, `var_dump`, screenshots, logging behavior, external domains, or any other absent feature is enforced by the CLI.

To call something CLI-enforced, require either:

- a fresh failing validation finding/identifier on the relevant condition; or
- current CLI source/test explicitly implementing that check.

If official Store docs prohibit something but CLI enforcement is not established, classify it as Store-doc-required, not CLI-enforced.

## Actionability and applicability

The findings table is for **current actionable findings**.

Do not pad it with hypothetical future requirements such as API credential validation, sales-channel configuration, external-service disclosures, logging rules, message-queue limits, or English fallback when the current extension does not implement the relevant feature.

For conditional docs rules:

- if the triggering feature exists, assess it and cite the source;
- if it clearly does not exist, omit it from actionable findings or mark it `not applicable` only when that distinction helps the user;
- do not tell the user to fix future hypothetical code.

Do not claim a minimal plugin “already satisfies” install/uninstall, cleanup, code-quality, or other runtime-review requirements unless those behaviors were actually tested or directly established.

## Current Store distinctions to preserve

Verify against current sources before reporting:

- International Store publication is required; German Store is optional.
- German/English 1:1 parity applies when publishing in the German Store.
- Store short description is 150–185 characters.
- Store long description is at least 200 characters.
- Store listing text must accurately describe functionality/use cases and include setup/configuration instructions.
- Shopware Account license must match `composer.json`.
- Store docs specify normal `src/Resources/config/plugin.png` as 112x112 px.
- current CLI normal icon validation accepts 112x112 through 256x256, max 30 KB; report CLI vs. docs separately.
- screenshot wording using `should` is Store-doc-guidance, not a blocker.

## Remote Store requirements and local candidates

If remote Account/listing state is unavailable, mark each applicable remote requirement **remote unverified**.

Do not report remote values as missing/satisfied based on local config.

A local value can be described as:

> local candidate/source looks compatible, remote unverified

For short description specifically: if current implementation maps composer/plugin metadata description to remote `ShortDescription`, report the actual local metadata length as a candidate. Do not invent another short-description source field.

For long description: `store.description` may be used as an optional CLI synchronization source if current implementation confirms it, but the Store requirement is the remote long description itself. Do not require local YAML unless the user has chosen repository-managed Store metadata or another authoritative source requires it.

Likewise, `store.availabilities` is an optional sync mechanism unless a source explicitly requires the local field. International Store publication is the requirement; local YAML is one way to synchronize it.

## Consistency of measured facts

Compute a local fact once from the actual current field and keep it consistent throughout the answer.

Do not report one composer description as 45 characters in one row and 165 characters elsewhere. If multiple description-like fields exist, name the exact field (`composer.json extra.description.en-GB`, `description`, `store.meta_description`, etc.) and never conflate them.

## Known traps

Do not:

- ask preflight questions resolvable from the workspace;
- require `store.meta_description` for Store short description;
- require `store.description`, `store.availabilities`, `store.default_locale`, `store.localizations`, or other `store.*` fields solely because docs require the resulting remote content/state;
- call local Store metadata a “standard Store config location” unless a source establishes that claim;
- say composer presence implies International Store availability;
- call a docs prohibition CLI-enforced merely because validation passes;
- call README.md, CHANGELOG.md, or physical LICENSE Store-required without explicit current source;
- treat changelog-generation schema as a CHANGELOG requirement;
- turn `store.icon` 256x256 into the normal `plugin.png` requirement;
- invent category/keywords/arbitrary icon sizes/translation-file/services-config requirements;
- say `Descriptions pass` based only on local candidates;
- say `ready for Store submission` because CLI validation passes;
- list sources that were not actually consulted.

## Required response format

Start with **Validation status**:

- PATH CLI binary/version;
- authoritative CLI binary/version;
- normal validation result;
- Store-compliance validation result;
- remote Store listing state: inspected / not inspected;
- files modified: yes/no.

Then provide only current actionable findings:

| Finding | Current evidence | Classification | Source | Applies to | Action |
| --- | --- | --- | --- | --- | --- |

The `Source` column is mandatory.

If a finding maps a Store requirement to a local field, the Source cell must include both:

1. the official Store docs source for the requirement; and
2. current CLI implementation/schema source for the field mapping.

If only (1) exists, describe the Store requirement without claiming a local field is required.

### Required local changes

Only CLI-enforced or Store-doc-required deficiencies whose **target is actually local**.

A remote Store listing requirement is not a required local YAML change merely because an optional sync field exists.

### Required Store conditions to verify

List every applicable remote Store requirement not inspected. If a local candidate exists, show it as a candidate and keep remote state unverified.

State:

> Store publication readiness cannot be confirmed from local validation alone because the required remote Store listing state was not inspected.

### Guidance / recommendations

Store-doc-guidance, schema-supported items, and recommendations only. Optional CLI synchronization may be suggested with `can` / `consider`, not `must` / `should add`, unless the user selected that workflow.

### Sources checked

List only sources actually used, with direct clickable official docs links and concrete CLI/source evidence.

## Final audit

Before answering, verify:

1. no unnecessary preflight question was asked;
2. sibling checkout CLI was selected when present and used consistently;
3. fresh normal + Store-compliance validation ran read-only;
4. every Store-doc claim has the exact official source;
5. every local-field mapping has implementation evidence, not just schema/name similarity;
6. `store.meta_description` was not mistaken for Store short description;
7. remote requirements were not converted into required local YAML fields;
8. passing validation was not used as proof of enforcement for absent patterns;
9. conditional future requirements were not padded into current actionable findings;
10. all measured local facts are internally consistent and name the exact field;
11. `plugin.png` and `store.icon` stayed distinct;
12. 112x112 Store-doc favicon and CLI 112–256 acceptance stayed distinct;
13. remote values are unverified unless actually inspected;
14. recommendations did not become requirements in summaries/actions;
15. no files were modified during inspection-only work.

If any check fails, correct the answer before sending it.
