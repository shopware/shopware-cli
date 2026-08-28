---
name: shopware-cli-extension-store
description: MUST use for Shopware Store publication/readiness/submission/compliance questions, including "prepare this plugin/extension for the Shopware Store", "what needs to change before Store publication", Store listing metadata, Store-compliance validation, packaging, or upload readiness. Verify the live workspace, the checkout-built CLI when present, current CLI behavior, remote listing state when available, and current official Store docs before declaring readiness or requirements.
---

# Shopware Store Readiness

Use this skill whenever the user asks whether a Shopware extension/plugin is ready for the Shopware Store or what must change before publication.

The goal is an evidence-backed readiness assessment, not a generic Store checklist.

## Four different truths

Never collapse these into one:

1. **Workspace truth** — files and values that exist in the current local extension.
2. **CLI truth** — what the selected Shopware CLI actually validates/enforces.
3. **Store-policy truth** — what current official Shopware Store documentation requires or recommends.
4. **Remote-listing truth** — what is currently configured in the Shopware Account / Store listing.

A clean CLI run proves only CLI truth. It does **not** prove Store publication readiness.

If the remote Store listing was not inspected, never say **ready for Store submission**, **ready for publication**, or **nothing must change**. Say that local CLI validation passes but Store publication readiness remains unverified until required remote listing conditions are checked.

## Mandatory read-only workflow

When the user asks to inspect only, do not modify files.

### 1. Inspect the live extension

Before saying a file is missing/present or stating image dimensions, verify the current tree:

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

Use explicit existence checks for any path you plan to call missing. Do not infer remote Store screenshots/listing data from local file absence.

### 2. Resolve one authoritative CLI

Inspect both PATH and sibling checkout binaries:

```bash
command -v shopware-cli
shopware-cli --version

if [ -x ../shopware-cli/bin/shopware-cli ]; then
  ../shopware-cli/bin/shopware-cli --version
fi
```

If `../shopware-cli/bin/shopware-cli` exists and is executable, use it for the rest of this Store-readiness assessment unless the user explicitly asks to assess the installed release.

Report PATH and authoritative versions when they differ. Do not silently fall back to Homebrew/PATH.

### 3. Run fresh validation

Using the authoritative binary:

```bash
<cli> extension validate . --reporter markdown
<cli> extension validate --help
```

If `--store-compliance` is supported and Store compliance is not already enabled in YAML, test it read-only:

```bash
<cli> extension validate . --store-compliance --reporter markdown
```

Fresh output outranks saved reports and earlier agent output.

`validation.store_compliance: true` is useful persistent configuration for Store-intended extensions; do not call persistence a Store submission requirement unless current Store docs explicitly establish that.

### 4. Inspect the current schema

```bash
<cli> extension config-schema
```

A field appearing in the schema means **supported**, not automatically **required**.

Important distinction:

- `store.*` fields are a local CLI-supported representation for Store information/synchronization.
- Absence of a local `store:` block does not prove remote Store listing values are missing.
- A Store listing requirement does not prove the value must live in `.shopware-extension.yml`.

Current CLI code may map local metadata and optional `store.*` values to the Shopware Account via account producer commands. Treat YAML as a synchronization mechanism, not as the only possible source of Store listing data.

### 5. Verify current official Store docs

Before using `required`, `must`, `blocker`, `critical`, `reject`, `will fail`, or equivalent wording, verify the current official documentation or current CLI enforcement.

Primary sources:

- <https://developer.shopware.com/docs/guides/development/testing/store/content-and-translations.html>
- <https://developer.shopware.com/docs/guides/development/testing/store/quality-guidelines.html>
- <https://developer.shopware.com/docs/guides/development/testing/store/code-quality.html>
- <https://developer.shopware.com/docs/guides/development/testing/store/functionality-integration.html>
- <https://developer.shopware.com/docs/guides/development/testing/store/store-review-errors.html>
- <https://developer.shopware.com/docs/guides/development/testing/store/cookies-and-privacy.html>
- <https://developer.shopware.com/docs/guides/development/testing/store/installation-and-cleanup.html>

Preserve the strength of the source wording. `Should`/`prefer`/`recommended` is guidance, not a blocker.

## Evidence classifications

Classify every actionable finding as one of:

- **CLI-enforced** — fresh validation fails because of it, or current source/tests directly prove enforcement.
- **Store-doc-required** — current official Store docs clearly establish the requirement.
- **Store-doc-guidance** — current official Store docs use non-mandatory guidance such as `should`, `prefer`, or `recommended`.
- **Schema-supported, not proven required** — the current schema supports it, but neither CLI enforcement nor Store docs prove it mandatory.
- **Recommendation / inference** — sensible advice without authoritative requirement evidence.

The classification is sticky through the entire response. A recommendation must not become an unconditional `Add`, `Fix`, `Replace`, `Must`, or `Critical` step later.

## Current Store documentation facts

When the current official `Content and translations` page still says the following, preserve these distinctions:

### Store-doc-required

- All extensions must be published in the **International Store**; German Store publication is optional.
- If also published in the German Store, English/German content must have 1:1 parity.
- Short description: **150–185 characters**.
- Long description: **minimum 200 characters**.
- Listing text must meaningfully and accurately describe the extension/use cases and include clear, complete setup/configuration instructions.
- English must be available as the fallback for settings and error messages.
- The license selected in the Shopware Account must match `composer.json`.
- The documentation instructs extensions to store a valid favicon named `plugin.png` at `src/Resources/config/plugin.png` with **112x112px**. Treat this as an extension-artifact Store requirement unless current docs change.

### Store-doc-guidance

- Images should show the extension in use in Storefront and Administration.
- At least one Storefront and one Administration screenshot **should** show main features.
- Mobile and desktop screenshots are preferred.

Do not put screenshot guidance inside a section headed `must have` or `required` unless another authoritative source establishes a stronger requirement.

## Current CLI behavior

For the current implementation associated with this skill:

- normal extension validation includes normal metadata and plugin-icon validation;
- a missing normal plugin icon can produce `metadata.icon`;
- the current normal icon validator accepts **112x112 through 256x256**, max **30 KB**;
- therefore a 128x128 `plugin.png` can pass CLI validation while still differing from Store docs specifying 112x112;
- Store-compliance mode gates Administration/Storefront source/build pairing checks such as `assets.administration.sources_missing`;
- `--full` is separate from Store-compliance mode.

A passing fixture is not proof that an existing item is unenforced. Use source/tests or a controlled fixture for negative enforcement claims.

## Requirement location

For each **Store-doc-required** item, state where it applies:

- **extension artifact** — a file/path explicitly required in the extension;
- **Store listing / Shopware Account** — publication content/state that may be remote;
- **CLI configuration** — only when current CLI semantics/docs explicitly require local config.

If remote Account state was not read, required listing conditions are **unverified**, not `missing` and not `satisfied`.

Example: absence of local `store.description` does not prove the Store long description is missing. The correct result is: Store long description is required; remote state was not verified; local YAML currently does not supply it for CLI synchronization.

## Known traps

Never repeat these mistakes:

- Do not say `ready for Store submission` solely because normal/Store-compliance CLI validation passes.
- Do not say `nothing must change` when required remote listing conditions have not been verified.
- Do not label screenshots **CLI-enforced** merely because Store docs discuss screenshots.
- Do not put `should` screenshot guidance under `Remote Store listing must have`.
- Do not call `README.md`, `CHANGELOG.md`, or a physical `LICENSE` file Store-required unless current official docs explicitly establish that.
- Absence of `README.md`, `CHANGELOG.md`, or `LICENSE` alone is normally not an actionable Store-readiness finding; omit these rows unless they are directly relevant to a verified requirement or the user specifically asks about them.
- Top-level `changelog` generation configuration does **not** make absence of `CHANGELOG.md` a **Schema-supported** finding. These are different concepts.
- Do not call `store.type`, `store.default_locale`, `store.availabilities`, `store.localizations`, or Store metadata fields Store-required local YAML solely because the schema supports them.
- Do not infer remote Store listing data is missing from absence of the local `store:` block.
- Do not turn `store.icon`'s 256x256 schema description into the normal `plugin.png` requirement.
- Do not say a 128x128 normal plugin icon is Store-compliant merely because the CLI accepts it; current Store docs specify 112x112.
- Do not invent arbitrary icon sizes, `category`, `keywords`, translation-file requirements, or service-config requirements.
- Do not pad the result with speculative `Compliant` rows unrelated to current findings.
- Do not present packaging/upload as an immediate next step until readiness is established and the user asks for submission actions.
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

Then use these sections:

### Required local changes

Include only demonstrated **CLI-enforced** or **Store-doc-required** deficiencies in the local extension artifact/configuration.

If none exist, say `No other required local changes were established.` Do not translate that into overall Store readiness.

### Required Store conditions to verify

List **Store-doc-required** Store listing / Shopware Account conditions whose remote state was not inspected. Use `verify/ensure`, not `missing`, when state is unknown.

If remote state is unverified, explicitly say:

> Store publication readiness cannot be confirmed from local validation alone because the required remote Store listing state was not inspected.

### Guidance / recommendations

Put **Store-doc-guidance**, **Schema-supported, not proven required**, and **Recommendation / inference** here with conditional wording.

Do not add a generic numbered `Next steps` list that re-promotes recommendations.

## Final self-audit

Before answering, verify:

1. Every local presence/missing claim came from the live tree.
2. Every dimension/size claim came from current inspection or an authoritative source.
3. The sibling checkout CLI was used when present unless the user explicitly requested the installed release.
4. One authoritative CLI binary was used consistently after selection.
5. Validation output is fresh.
6. A passing fixture was not used as proof of non-enforcement.
7. `must` vs `should` wording from Store docs was preserved.
8. Remote Store listing values were not declared missing/satisfied without reading remote state.
9. CLI success was not converted into overall Store readiness.
10. Schema-supported fields were not promoted to mandatory without separate evidence.
11. `plugin.png` and `store.icon` stayed separate.
12. The Store-doc 112x112 favicon requirement and CLI 112–256 accepted range were reported distinctly.
13. README/CHANGELOG/LICENSE, category/keywords, translation files, and services config were not invented as requirements or noisy pseudo-findings.
14. Guidance stayed guidance in summaries/actions.
15. No files were modified during inspection-only work.

If any check fails, correct the answer before sending it.
