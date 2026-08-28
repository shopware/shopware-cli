---
name: shopware-cli-extension-store
description: Assess and prepare Shopware extensions for Store distribution using verified workspace facts, fresh CLI behavior, the current CLI schema/source, and authoritative Store documentation. Separate proven requirements from schema support and recommendations; never infer Store blockers from generic conventions.
---

# Shopware Extension Store Distribution

Use this skill for Store-readiness assessments, Store-compliance validation, Store listing configuration, packaging, and submission guidance.

The goal is not to produce a generic Store checklist. The goal is to answer from **current evidence**.

## Non-negotiable rules

1. **Inspect the current working tree before describing it.** Never claim a file is missing, present, invalid, undersized, placeholder, or configured without checking the live project.
2. **Use one authoritative CLI binary.** When an unreleased `shopware/shopware-cli` checkout is available, prefer its built binary over an older executable on `PATH` and use that same binary for help, schema, validation, and packaging discovery.
3. **Fresh runtime output beats saved reports.** Ignore stale `validation.json`, XML, markdown, screenshots, or prior agent output when they conflict with the current tree or a fresh command.
4. **Do not modify files during an inspection-only request.** Use read-only commands, CLI flags, schema/help output, source inspection, official docs, or a disposable fixture.
5. **Schema support is not a requirement.** A field appearing in `extension config-schema` proves only that the CLI supports that shape.
6. **A passing fixture does not prove non-enforcement.** If an item is present and validation passes, that does not show the validator would allow it to be absent. Use source/tests or a controlled fixture before claiming that the CLI does not enforce something.
7. **Strong requirement language needs strong evidence.** `required`, `must`, `blocker`, `critical`, `will fail`, `reject`, and `rejection` require either current CLI enforcement or an explicit current official Store requirement.
8. **Evidence classifications are sticky.** A recommendation cannot become an unconditional action in a later summary or next-step list.

## Mandatory Store-readiness protocol

For a request such as “inspect this extension and tell me what needs to change for Store publication,” follow this sequence before answering.

### 1. Resolve the authoritative CLI

Start by locating the current binary and any nearby checkout-built binary:

```bash
command -v shopware-cli
shopware-cli --version

# If a sibling shopware-cli checkout exists, inspect its built binary too.
if [ -x ../shopware-cli/bin/shopware-cli ]; then
  ../shopware-cli/bin/shopware-cli --version
fi
```

If the task is explicitly testing unreleased CLI or skill behavior from a checkout, use the checkout-built binary consistently. Do not mix its results with a Homebrew/system binary.

Record which binary and version produced the evidence.

### 2. Inspect the live extension

Before stating workspace facts, inspect the current files directly. Keep this read-only.

Typical commands:

```bash
pwd
find . -maxdepth 4 -type f \
  -not -path './vendor/*' \
  -not -path './node_modules/*' \
  -not -path './.git/*' | sort

cat composer.json

if [ -f .shopware-extension.yml ]; then
  cat .shopware-extension.yml
fi

if [ -e src/Resources/config/plugin.png ]; then
  file src/Resources/config/plugin.png
fi
```

Use targeted checks when making a specific claim:

```bash
test -f README.md && echo 'README present' || echo 'README absent'
test -f CHANGELOG.md && echo 'CHANGELOG present' || echo 'CHANGELOG absent'
test -f LICENSE && echo 'LICENSE present' || echo 'LICENSE absent'
test -f src/Resources/config/plugin.png && echo 'plugin icon present' || echo 'plugin icon absent'
```

Do not say “missing” unless a current check shows absence. Do not state image dimensions unless current file metadata or another reliable inspection shows them.

### 3. Run fresh normal validation

Use the chosen binary:

```bash
<cli> extension validate . --reporter markdown
```

Treat the current exit status/output as authoritative for the current tree.

### 4. Run Store-compliance validation without modifying the project

First inspect current help:

```bash
<cli> extension validate --help
```

If `--store-compliance` is supported and Store compliance is not already enabled in YAML, use the flag for a read-only assessment:

```bash
<cli> extension validate . --store-compliance --reporter markdown
```

This is preferable to temporarily editing `.shopware-extension.yml` when the user said not to modify files.

For Store-intended projects, `validation.store_compliance: true` can be recommended as persistent project configuration so ordinary local/CI validation uses the same mode, but do **not** call persistence a Store submission requirement unless current official Store documentation establishes that.

Issue #1407 tracks deprecating the flag. Do not say the flag is already deprecated unless current CLI help or runtime output says so.

### 5. Inspect the current schema only for supported configuration

```bash
<cli> extension config-schema
```

Only claim a schema field exists if it is visible in the current schema output. Do not invent fields from memory.

Fields such as `store.type`, `store.icon`, `store.description`, `store.meta_title`, `store.meta_description`, `store.localizations`, and others may be supported depending on the current schema.

For each field, distinguish:

- **supported by schema**
- **required by schema/local validation**, if the schema or validator actually marks/enforces it
- **required by Store publication**, only if current official Store documentation says so

Do not collapse these into one concept.

### 6. Check current official Store documentation before declaring publication requirements

When the user asks what is needed for Store publication, verify relevant current official Shopware documentation before classifying anything as a Store requirement.

Useful starting points:

- <https://developer.shopware.com/docs/guides/development/testing/store/quality-guidelines.html>
- <https://developer.shopware.com/docs/guides/development/testing/store/content-and-translations.html>
- <https://developer.shopware.com/docs/guides/development/testing/store/code-quality.html>
- <https://developer.shopware.com/docs/guides/development/testing/store/functionality-integration.html>
- <https://developer.shopware.com/docs/guides/development/testing/store/not-allowed-store-behaviors.html>
- <https://developer.shopware.com/docs/guides/development/testing/store/store-review-errors.html>
- <https://developer.shopware.com/docs/guides/development/testing/store/storefront-performance-and-errors.html>
- <https://developer.shopware.com/docs/guides/development/testing/store/seo-and-structured-data.html>
- <https://developer.shopware.com/docs/guides/development/testing/store/cookies-and-privacy.html>
- <https://developer.shopware.com/docs/guides/development/testing/store/installation-and-cleanup.html>

If current official docs cannot be accessed or do not explicitly establish a claim, do not fill the gap from memory. Downgrade it to a recommendation or state that the requirement could not be confirmed.

## Evidence model

Classify every proposed change as exactly one of these:

- **CLI-enforced** — a fresh run fails because of it, or current CLI source/tests directly establish the rule.
- **Store-doc-required** — current official Shopware Store documentation explicitly establishes the publication/review requirement.
- **Schema-supported, not proven required** — current `config-schema` shows the field/shape, but neither CLI enforcement nor Store documentation proves it mandatory.
- **Recommendation / inference** — product-quality advice, likely cleanup, branding guidance, or another sensible suggestion not established as a requirement.

Workspace observations such as “file exists,” “URL equals example.com,” or “icon is 128x128” are **evidence**, not requirement classifications. They must come from current inspection.

### Language by classification

For **CLI-enforced** and **Store-doc-required**, imperative actions are appropriate when the evidence clearly requires a change.

For **Schema-supported, not proven required** and **Recommendation / inference**, keep actions conditional. Prefer:

- “consider ...”
- “verify whether ...”
- “if required by current Store documentation, configure ...”
- “this is supported but not established as mandatory”

Do not use `reject`, `fail`, `flag`, `block`, `needs`, `must`, `required`, or equivalent mandatory wording for an unverified finding.

The classification must remain unchanged in:

- the finding description
- evidence/impact text
- action text
- summary
- checklist
- “next steps”

Do not re-promote a recommendation later in the answer.

## What Store-compliance mode currently means

Verify this against the current source when version accuracy matters.

In the current implementation associated with this skill, `validation.store_compliance` gates `validateAssets`, which checks Administration and Storefront source/build pairing, including configured extra bundles.

A controlled example:

- compiled Administration assets under `Resources/public/administration` without an Administration source entrypoint can pass normal validation;
- the same tree can fail Store-compliance validation with `assets.administration.sources_missing`.

The inverse source-without-build situation is also checked.

Do not describe unrelated normal validators as Store-only checks. Metadata, license value, plugin icon, and other built-in validators may run in normal validation.

Do not conclude “Store compliance has no checks” merely because one fixture produces zero Store-compliance findings.

## Plugin metadata and icons

Keep these separate:

- plugin metadata from `composer.json`
- normal plugin icon, typically `src/Resources/config/plugin.png`
- Store listing metadata/assets such as `.shopware-extension.yml` `store.icon`

Never infer that the normal plugin icon is missing without checking the file.

Never infer that the normal plugin icon must be exactly 256x256 from a `store.icon` schema description. If exact normal-plugin icon constraints matter, inspect current validator output/source.

A physical `LICENSE`, `README.md`, or `CHANGELOG.md` may be useful, but do not call any of them required for Store publication unless current official Store documentation explicitly says so. Likewise, do not claim the CLI does not validate them merely because the current fixture passes; prove non-enforcement from source/tests or a controlled fixture if that distinction matters.

## Store configuration

`extension config-schema` describes configuration the CLI understands. It is not automatically a Store submission checklist.

A Store-intended extension may choose to persist Store validation intent:

```yaml
validation:
  store_compliance: true
```

And may use a supported `store:` block for listing information. Which Store fields are actually mandatory must come from current schema validation semantics and/or current official Store documentation, not from the mere existence of those fields.

If the current extension has no `store:` block, report that as an observed configuration state. Do not say it “needs a Store section” unless evidence proves the section is mandatory for the user's intended submission path.

## Full validation is separate

`--full` controls additional validation tools such as PHPStan, ESLint, and Stylelint. It is separate from Store-compliance mode.

Use only tool names accepted by the current CLI. Inspect:

```bash
<cli> extension validate --help
```

Do not invent wildcard tool names.

## Packaging and account operations

Before suggesting packaging or submission commands, inspect the current command surface:

```bash
<cli> extension package --help
<cli> account producer extension --help
<cli> account producer extension info --help
<cli> account producer extension upload --help
```

Do not invent `shopware-cli extension create` or other commands that are not present.

Do not present upload as an immediate next step merely because validation passes. Upload/push operations are remote side effects and require explicit user intent. Local validation success does not prove Store acceptance or listing completeness.

## Required response shape for readiness assessments

When the user asks what **needs to change**, separate proven requirements from everything else.

Start with a short validation status containing:

- authoritative CLI binary/version
- normal validation result
- Store-compliance validation result, if run
- whether files were modified

Then use a table:

| Finding | Current evidence | Classification | Action |
| --- | --- | --- | --- |
| ... | live file/CLI/docs evidence | one of the four classes | wording consistent with classification |

Then provide exactly these two sections when relevant:

### Required changes

Include only **CLI-enforced** and **Store-doc-required** changes.

If none are demonstrated, say:

> No required changes were established by the current CLI checks or the Store requirements verified in this assessment.

### Recommendations / items to verify

Put **Schema-supported, not proven required** and **Recommendation / inference** items here. Keep them conditional.

Do not end with an unconditional numbered “Next Steps” list that upgrades recommendations into requirements.

## Final self-audit before answering

Before sending the assessment, check all of these:

1. **Missing-file claims:** Did I directly inspect that path in the current tree?
2. **Presence/dimension claims:** Did I inspect the current file rather than infer from a checklist?
3. **Non-enforcement claims:** Am I incorrectly inferring “CLI does not enforce X” merely because X is present and validation passes? If so, remove the claim or verify it from source/tests/controlled fixture.
4. **Schema claims:** Did the exact field appear in the current `config-schema` output?
5. **Store requirement claims:** Did current official Shopware documentation explicitly establish the requirement?
6. **CLI requirement claims:** Did a fresh run or current source/tests establish enforcement?
7. **Icon separation:** Did I keep `src/Resources/config/plugin.png` separate from `store.icon`?
8. **Read-only constraint:** Did I avoid changing project files when asked not to modify them?
9. **Binary consistency:** Did I use one intended CLI binary throughout?
10. **Sticky classification:** Did any recommendation become `add`, `fix`, `replace`, `must`, `required`, `reject`, `fail`, or `block` later in the response?
11. **Required-changes section:** Does it contain only CLI-enforced or Store-doc-required items?
12. **No invented commands:** Did I verify command syntax with the current CLI before suggesting it?

If any check fails, correct the assessment before responding.

## Information priority

When evidence conflicts, use this order:

1. Current live working-tree inspection for facts about the project.
2. Intended/current CLI binary and fresh runtime output.
3. Current `.shopware-extension.yml` plus current `extension config-schema`.
4. Current `shopware/shopware-cli` source/tests for implementation semantics.
5. Current official Shopware Store documentation for publication/review requirements.
6. Historical reports, remembered behavior, and examples only as background.

Use the source that answers the specific question: filesystem inspection establishes whether a file exists; CLI/source establishes local enforcement; official Store docs establish Store publication policy.

## See also

- `shopware-cli` skill for general CLI and validation guidance.
- Issue #1407 for the `--store-compliance` deprecation plan and current mode semantics.
- Shopware Store testing documentation: <https://developer.shopware.com/docs/guides/development/testing/store/>
