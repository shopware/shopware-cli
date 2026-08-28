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

A local value that happens to satisfy a Store content constraint is only a **candidate/source** for remote Store data unless the relevant Store listing state was actually inspected. For example, a `composer.json` description with 165 characters does not prove the remote Store short description satisfies the 150–185 character requirement.

If the remote Store listing was not inspected, never say **ready for Store submission**, **ready for publication**, **descriptions pass**, or **nothing must change** for remote Store conditions. Say that local CLI validation passes but Store publication readiness remains unverified until required remote listing conditions are checked.

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
```

For the normal plugin icon, avoid ambiguous shell chains. Use an explicit branch:

```bash
if [ -e src/Resources/config/plugin.png ]; then
  file src/Resources/config/plugin.png
else
  echo 'plugin icon absent'
fi
```

Do not write probes such as `[ -e file ] && file file && optional-tool file || echo 'file absent'`: failure of the optional tool can falsely report an existing file as absent.

Use explicit existence checks for any path you plan to call missing. Do not infer remote Store screenshots/listing data from local file absence.

### 2. Resolve one authoritative CLI deterministically

Inspect PATH, then test the **literal sibling path from the current extension directory**:

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

Do **not** reconstruct the sibling path manually from parent directories or hard-code an absolute path. From an extension at `.../test-plugin`, `../shopware-cli/bin/shopware-cli` means the sibling checkout at `.../shopware-cli`.

Selection rule:

- If literal `[ -x ../shopware-cli/bin/shopware-cli ]` succeeds, `../shopware-cli/bin/shopware-cli` is authoritative unless the user explicitly asks to test the installed release.
- From that point onward, every Store-readiness `--version`, `--help`, `extension validate`, `extension config-schema`, packaging, and account-discovery command must start with `../shopware-cli/bin/shopware-cli`.
- If the sibling check succeeds but later commands use plain `shopware-cli`, the assessment is invalid: rerun those commands with the checkout binary before answering.
- Only fall back to the PATH CLI when the literal sibling check fails.

Report PATH and authoritative versions when they differ.

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

- **CLI-enforced** — a fresh validation run fails because of it, or current source/tests directly establish enforcement.
- **Store-doc-required** — current official Store docs clearly establish the requirement.
- **Store-doc-guidance** — current official Store docs use non-mandatory guidance such as `should`, `prefer`, or `recommended`.
- **Schema-supported, not proven required** — the current schema supports it, but neither CLI enforcement nor Store docs prove it mandatory.
- **Recommendation / inference** — sensible advice without authoritative requirement evidence.

The classification is sticky through the entire response. A recommendation must not become an unconditional `Add`, `Fix`, `Replace`, `Must`, or `Critical` step later.

Do not use **locally demonstrated** to mark a remote Store-listing requirement as satisfied. That state is reserved for requirements whose target is genuinely local, such as an extension-artifact file requirement. For remote listing requirements, use **local candidate/source looks compatible, remote unverified** until the Account/listing itself is inspected.

## Current Store documentation facts

When the current official `Content and translations` page still says the following, preserve these distinctions.

### Store-doc-required

- All extensions must be published in the **International Store**; German Store publication is optional.
- If also published in the German Store, English/German content must have 1:1 parity.
- Store **short description**: **150–185 characters**.
- Store **long description**: **minimum 200 characters**.
- Store listing text must be meaningful, accurately describe the extension/use cases, and include clear, complete setup/configuration instructions.
- English must be available as the fallback for settings and error messages.
- The license selected in the Shopware Account must match `composer.json`.
- The docs instruct extensions to store a valid favicon named `plugin.png` at `src/Resources/config/plugin.png` with **112x112px**. Treat this as an extension-artifact Store requirement unless current docs change.

The short/long description constraints above apply to the **Store listing**, not automatically to arbitrary local description fields. A local `composer.json` description can be reported as a compatible candidate/source when appropriate, but it does not satisfy the remote requirement by itself.

### Store-doc-guidance

- Images should show the extension in use in Storefront and Administration, including configuration/how-to detail.
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

If remote Account state was not read, required listing conditions are **unverified**, not `missing`, `satisfied`, `compliant`, or `pass`.

Examples:

- Absence of local `store.description` does not prove the Store long description is missing. Say the long-description requirement exists, remote state is unverified, and local YAML currently does not supply it for CLI synchronization.
- A `composer.json` description whose length fits 150–185 characters is a **local candidate/source that looks compatible** with the Store short-description constraint. It is not an extension-artifact compliance result and does not prove the remote listing contains that value.
- Never write `Descriptions pass` based only on `composer.json`; write `Local short-description candidate is within the documented range; remote short description remains unverified.`

If checking a JSON string length with `jq`, use its string length rather than byte-counting a newline, for example:

```bash
jq -r '.extra.description."en-GB" | length' composer.json
```

## Required Store-condition completeness

When remote Store state is unavailable, review each applicable current Store-doc-required condition and classify its state as one of:

- **locally demonstrated** — local evidence directly satisfies a requirement whose target is the extension artifact/local state;
- **local candidate/source looks compatible, remote unverified** — useful for Store listing synchronization, but not proof of remote state;
- **remote unverified** — must be checked in the Shopware Account/listing;
- **not applicable** — explain why.

At minimum, consider the current documented conditions relevant to the extension:

- International Store availability;
- German/English 1:1 parity if German Store publication is intended;
- short description 150–185 characters;
- long description minimum 200 characters;
- meaningful/accurate short and long descriptions;
- clear setup/configuration instructions in the Store listing;
- English fallback for settings/error messages where the extension has such user-facing text;
- Shopware Account license matching `composer.json`;
- `plugin.png` Store favicon requirement.

**If the remote Store listing was not inspected, the short-description condition must still appear in `Required Store conditions to verify`, even when a local `composer.json` description is within 150–185 characters.** Report the local value as a compatible candidate/source, then state that the remote value is unverified.

Do not omit any required remote condition merely because a local candidate value appears valid.

## Local sync configuration wording

When a required Store listing value is absent from local `.shopware-extension.yml`, do not say it **should be added** there unless the user has chosen YAML/CLI synchronization as their workflow.

Use wording such as:

- `This required Store listing value can be managed in the Shopware Account or supplied through the supported store.* field for CLI synchronization.`
- `If you want the repository to be the source of truth for Store metadata, consider configuring store.description / store.installation_manual.`

Do not imply that `store.description` or `store.installation_manual` is itself the Store requirement. The requirement is the resulting Store listing content.

## Known traps

Never repeat these mistakes:

- Do not say `ready for Store submission` solely because normal/Store-compliance CLI validation passes.
- Do not say `nothing must change` when required remote listing conditions have not been verified.
- Do not say `Descriptions pass` solely because local `composer.json` description lengths fit the Store range.
- Do not classify Store short-description compliance as **locally demonstrated / extension artifact** when only a local candidate value was inspected.
- Do not omit the Store short-description requirement from remote verification merely because the local candidate is 150–185 characters.
- Do not use the PATH/Homebrew CLI after literal `../shopware-cli/bin/shopware-cli` has been shown executable.
- Do not invent an absolute sibling path from guessed parent directories; test `../shopware-cli/bin/shopware-cli` literally first.
- Do not label screenshots **CLI-enforced** merely because Store docs discuss screenshots.
- Do not put `should` screenshot guidance under `Remote Store listing must have`.
- Do not call `README.md`, `CHANGELOG.md`, or a physical `LICENSE` file Store-required unless current official docs explicitly establish that.
- Absence of `README.md`, `CHANGELOG.md`, or `LICENSE` alone is normally not an actionable Store-readiness finding; omit these rows unless directly relevant to a verified requirement or specifically asked about.
- Top-level `changelog` generation configuration does **not** make absence of `CHANGELOG.md` a **Schema-supported** finding.
- Do not call `store.type`, `store.default_locale`, `store.availabilities`, `store.localizations`, or Store metadata fields Store-required local YAML solely because the schema supports them.
- Do not infer remote Store listing data is missing from absence of the local `store:` block.
- Do not turn `store.icon`'s 256x256 schema description into the normal `plugin.png` requirement.
- Do not say a 128x128 normal plugin icon is Store-compliant merely because the CLI accepts it; current Store docs specify 112x112.
- Do not invent arbitrary icon sizes, `category`, `keywords`, translation-file requirements, or service-config requirements.
- Do not add generic `compatibility_date` advice to a Store-readiness report unless current evidence shows a compatibility problem or the user asks about version targeting.
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

For Store listing requirements backed only by local source values, the row must still say `Store listing / Shopware Account` in **Applies to**, and the evidence/action must explicitly preserve `remote unverified`.

Then use these sections:

### Required local changes

Include only demonstrated **CLI-enforced** or **Store-doc-required** deficiencies whose target is the local extension artifact/configuration.

If none exist, say `No other required local changes were established.` Do not translate that into overall Store readiness.

### Required Store conditions to verify

List **every applicable Store-doc-required Store listing / Shopware Account condition** whose remote state was not inspected, including short description even if a local candidate is valid.

If a local source value looks compatible, say so while keeping remote state unverified. Example:

> **Short description** — Store listing requires 150–185 characters. Local `composer.json` EN/DE descriptions are within that range and may be used as source values, but the remote Store short descriptions were not inspected and must still be verified.

If remote state is unverified, explicitly say:

> Store publication readiness cannot be confirmed from local validation alone because the required remote Store listing state was not inspected.

### Guidance / recommendations

Put **Store-doc-guidance**, **Schema-supported, not proven required**, and **Recommendation / inference** here with conditional wording. Include screenshot guidance when relevant, but do not promote it to a required condition.

When discussing YAML metadata synchronization, use `can` / `consider` rather than `should` unless the user has chosen that workflow.

Do not add a generic numbered `Next steps` list that re-promotes recommendations.

## Final self-audit

Before answering, verify:

1. Every local presence/missing claim came from the live tree.
2. File-probing commands cannot print contradictory presence/absence because an optional secondary command failed.
3. Every dimension/size claim came from current inspection or an authoritative source.
4. Literal `../shopware-cli/bin/shopware-cli` was tested before any reconstructed/absolute sibling path.
5. If that literal sibling CLI existed, it was the authoritative binary for every subsequent CLI command unless the user explicitly requested otherwise.
6. One authoritative CLI binary was used consistently after selection.
7. Validation output is fresh.
8. A passing fixture was not used as proof of non-enforcement.
9. `must` vs `should` wording from Store docs was preserved.
10. Remote Store listing values were not declared missing/satisfied/pass/compliant without reading remote state.
11. CLI success was not converted into overall Store readiness.
12. All applicable Store-doc-required remote conditions were considered, including short description even when a valid local candidate exists.
13. A local Store metadata candidate was not mislabeled as an extension-artifact compliance result.
14. Schema-supported fields were not promoted to mandatory without separate evidence.
15. `plugin.png` and `store.icon` stayed separate.
16. The Store-doc 112x112 favicon requirement and CLI 112–256 accepted range were reported distinctly.
17. README/CHANGELOG/LICENSE, category/keywords, translation files, services config, and generic compatibility-date advice were not invented as requirements or noisy pseudo-findings.
18. YAML sync fields were described as optional synchronization choices rather than the Store requirement itself.
19. Guidance stayed guidance in summaries/actions.
20. No files were modified during inspection-only work.

If any check fails, correct the answer before sending it.
