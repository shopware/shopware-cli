---
name: shopware-cli-extension-store
description: MUST use for Shopware Store readiness/publication/submission/compliance questions. Inspects read-only, prefers a sibling checkout-built CLI, classifies every finding by table lookup, and cites a re-checkable source for each one so local file state is never reported as remote Store listing state.
---

# Shopware Store readiness

Assess the current extension for Store publication. Start immediately. Never modify files. Never ask a preflight question the workspace can answer.

Every claim in the answer carries a source. A finding without a source is not a finding.

## 1. Collect evidence

Run this script first from the extension directory. Do not answer before it completes.

```bash
./collect-evidence.sh
```

Script notes (collect-evidence.sh):

- Cobra prints its usage block on a non-zero exit. The `sed` strips it; the exit code is the real signal.
- Without `--full`, `extension validate` runs only the `sw-cli` toolset. It does **not** run PHPStan/ESLint/Stylelint. Source: `cmd/extension/extension_validate.go`, the `if !isFull { only = "sw-cli" }` branch. Say "sw-cli checks passed", not "validation passed", unless `--full` was run.
- If `../shopware-cli/bin/shopware-cli` exists it is authoritative for **every** command. Never mix binaries mid-answer.
- Output is captured to a variable before piping: PIPESTATUS is bash-only and $? after a pipe reports the last command, not the CLI. This preserves exit codes.
- Run `extension config-schema` only if the extension already uses Store sync config, or the user asks where Store metadata is configured. Schema fields are never readiness requirements.

## 2. Classification table — the only place classification is decided

Copy `Level`, `Target`, and `Source` from this table **verbatim**. Do not re-derive them from doc prose, CLI output, or reasoning. If a source has changed, report the drift as a separate note; do not silently reclassify.

Doc baseline: `https://developer.shopware.com/docs/guides/development/testing/store/content-and-translations.html`
Code baseline: shopware-cli source, verified against release 0.18.3.

CLI rules are cited by their **result identifier**, not a line number — identifiers appear in the CLI's own output and survive refactors. Re-check any row with:

```bash
grep -rn '"metadata.icon.size"' --include=*.go internal/ | grep -v _test
```

| # | Condition | Level | Target | Source | Emit only if |
|---|---|---|---|---|---|
| L1 | `extra.description` per locale is 150–185 chars | CLI-enforced | Local file | `metadata.description` — `internal/extension/validator.go` | always |
| L2 | `extra.manufacturerLink` per locale present | CLI-enforced | Local file | `metadata.manufacturer` — `internal/extension/platform.go` | always |
| L3 | `extra.supportLink` per locale present | CLI-enforced | Local file | `metadata.support` — `internal/extension/platform.go` | always |
| L4 | `src/Resources/config/plugin.png` is **112–256 px** and ≤30 kB | CLI-enforced | Local artifact | `metadata.icon.size` — `internal/extension/validator.go` | icon present |
| L5 | `authors` key present in `composer.json` | CLI-enforced | Local file | `metadata.author` — `internal/extension/platform.go` (apps: `app.go`) | always |
| R1 | Published in the international Store | Store-doc-required | Remote listing | docs `#store-listing` | always |
| R2 | Short description 150–185 chars | Store-doc-required | Remote listing | docs `#store-listing` | always |
| R3 | Long description ≥200 chars | Store-doc-required | Remote listing | docs `#store-listing` | always |
| R4 | Descriptions and use cases are meaningful and accurate | Store-doc-required | Remote listing | docs `#store-listing` | always |
| R5 | Display name avoids "plugin" and "shopware" | Store-doc-required | Remote listing | docs `#store-listing` | always |
| R6 | Clear, complete setup/configuration instructions | Store-doc-required | Remote listing | docs `#store-listing` | always |
| R7 | Clean HTML, allowed tags only, no ads/contact info/backlinks | Store-doc-required | Remote listing | docs `#store-listing` | always |
| R8 | No blank-space filler text | Store-doc-required | Remote listing | docs `#store-listing` | always |
| R9 | German/English 1:1 parity | Store-doc-required | Remote listing | docs `#store-listing` | user states German Store intent, **or** `de-DE` values exist in `composer.json` (state which triggered it) |
| R10 | English fallback for settings and error messages | Store-doc-required | Extension behavior | docs `#admin-translations` | extension has user-facing settings or error messages |
| A1 | Shopware Account license matches the `composer.json` license **value** | Store-doc-required | Remote Account | docs `#extension-master-data-and-license` | always |
| G1 | ≥1 storefront and ≥1 admin screenshot showing main features | Store-doc-guidance | Remote listing | docs `#images-and-screenshots` (*should*) | always |
| G2 | Mobile and desktop screenshots | Store-doc-guidance | Remote listing | docs `#images-and-screenshots` (*prefer*) | always |
| G3 | Theme preview image in Theme Manager | Store-doc-required | Remote listing | docs `#images-and-screenshots` | extension is a theme |
| G4 | CMS element icon | Store-doc-required | Local artifact | docs `#images-and-screenshots` | extension ships CMS elements |

Nothing outside this table is a finding, because nothing outside it has a source. In particular, never emit: `store.type`, demo shops, API credential checks, install/uninstall hooks, logging or JS rules for absent features, README/CHANGELOG/LICENSE-file pseudo-requirements, or generic compatibility-date advice.

### CLI-vs-docs conflicts — defer to docs

When CLI and docs disagree on a requirement:
- Follow the **docs** reading (canonical Store review standard).
- Flag the CLI gap or mismatch as a note only if useful for understanding why local validation passed but Store review may differ.
- Do not report the CLI reading as correct if docs have been updated.


## 3. Emit rules

Eight invariants. Check each finding against all eight before writing it.

1. **Lookup, not inference.** `Level`, `Target`, and `Source` come from the table verbatim.
2. **Emit every ungated row, not a selection.** Before writing the remote section, walk rows R1–R10, A1, G1–G4 in order and emit each whose precondition holds. Dropping a row because it feels obvious or hard to check is a silent failure. Count them: an extension with no theme, no CMS elements, and no settings UI yields nine remote rows plus A1.
3. **Remote rows can never become local changes.** Any row with `Target` = Remote listing / Remote Account / Extension behavior may appear **only** under "Remote conditions not verified". It can never be a required local change, and it can never be called missing, present, passing, failing, satisfied, or compliant unless the Shopware Account listing was actually inspected.
4. **Preconditions gate emission.** A row whose `Emit only if` is not proven true by evidence from §1 emits nothing at all. Not as "N/A", not as "not applicable", not as a struck-through line. It is absent. R9 and R10 are omitted entirely for an extension with no German Store intent and no settings UI.
5. **Local values are candidates, not proof.** Name the row and the specific rule the value bumps against, not generic advice: a label containing "Plugin" is incompatible with R5's prohibited-words rule, which is a different statement from "rename it to something better".
   - A candidate entry requires a **measured deficiency** — a value you read that conflicts with a named rule. A precondition firing is not a deficiency. If `config.xml` exists and its fields carry unqualified defaults plus `lang="…"` variants, the English fallback is present; say so or say nothing. Never write "verify that…" or "this may not…" about a file you have read: you either found a conflict and can quote it, or you did not.
   - Never propose a change that would delete existing content (translations, locales, fields) to satisfy a rule that content already satisfies. A `composer.json` label or description is a *sync candidate*. It may be described as compatible or incompatible with a remote rule. It never satisfies or fails one. R5 with a local label of "Acme Plugin" yields: remote display name unverified, **plus** a separate note that the local label candidate looks incompatible.
6. **Every local finding needs a quoted artifact.** A required local change must cite either a verbatim CLI output line or a measured value from §1. No line and no measurement means it is not a required local change.
   - A row may be marked **CLI-enforced** only if the CLI output contains a matching error line. If validation printed `No problems found`, the Required local changes section is empty. Write "none". Never infer a CLI finding from reading `composer.json` yourself.
   - Placeholder or example values (`example.com`, `TODO`, lorem text) are **not** CLI findings: L2 and L3 check presence, not plausibility. Mention them under Local candidates if useful, never as a required local change.
8. **Rows are the only vocabulary.** Every finding names a row ID from §2 and must match that row's actual subject. Do not attach a finding to the nearest-looking row. A1 is the license-value comparison and nothing else — an author homepage is not "A1 candidate", it is not in the table, so it is not a finding.
6. **Every emitted row carries its provenance.** For a CLI row: the result identifier and file. For a doc row: the doc URL with its section anchor. Never paraphrase a requirement without naming where it came from.

## 4. Store doc map

`https://developer.shopware.com/docs/guides/development/testing/store/` is the index of all Store review pages. Always link it in Sources so the user can navigate the set.

`content-and-translations` is the **baseline**: read it every run, it backs the table in §2. Read any other page only when its trigger fires, and add its rows to the answer as sourced findings. Never audit the whole set unprompted.

| Page | Covers | Read when |
|---|---|---|
| [`quality-guidelines`](https://developer.shopware.com/docs/guides/development/testing/store/quality-guidelines.html) | Non-negotiable quality, security, compliance rules for every extension | user asks for a full compliance audit or pre-submission review |
| [`content-and-translations`](https://developer.shopware.com/docs/guides/development/testing/store/content-and-translations.html) | Listing text, translations, images, icon, license | **always** |
| [`store-review-errors`](https://developer.shopware.com/docs/guides/development/testing/store/store-review-errors.html) | Common reasons reviewers reject a submission | user asks why a submission failed, or wants rejection risks |
| [`not-allowed-store-behaviors`](https://developer.shopware.com/docs/guides/development/testing/store/not-allowed-store-behaviors.html) | Prohibited patterns | extension touches core internals, filesystem, or DB directly |
| [`functionality-integration`](https://developer.shopware.com/docs/guides/development/testing/store/functionality-integration.html) | Correct integration with core, persistence, public APIs | extension has subscribers, entities, or API endpoints |
| [`code-quality`](https://developer.shopware.com/docs/guides/development/testing/store/code-quality.html) | Code standards reviewers apply | user asks about code quality, or `--full` validation was run |
| [`installation-and-cleanup`](https://developer.shopware.com/docs/guides/development/testing/store/installation-and-cleanup.html) | Install/update/uninstall and data removal | extension implements lifecycle methods or creates tables |
| [`cookies-and-privacy`](https://developer.shopware.com/docs/guides/development/testing/store/cookies-and-privacy.html) | Cookie registration, GDPR, subprocessors | extension sets cookies, tracks, or sends data to third parties |
| [`seo-and-structured-data`](https://developer.shopware.com/docs/guides/development/testing/store/seo-and-structured-data.html) | SEO output and structured data | extension changes storefront markup, URLs, or meta tags |
| [`storefront-performance-and-errors`](https://developer.shopware.com/docs/guides/development/testing/store/storefront-performance-and-errors.html) | Storefront performance and error handling | extension ships storefront JS/CSS or template overrides |
| [`faq`](https://developer.shopware.com/docs/guides/development/testing/store/faq.html) | Preview requirements and misc answers | user asks about previews or something the other pages miss |

Preconditions here work like §2's: a page you had no trigger to read produces no findings, and you say so rather than guessing at its contents.


## 5. Response format

**Validation status**

- CLI binary, version, and source revision if a checkout was used
- inspection timestamp
- sw-cli checks: pass/fail + exit code (state whether `--full` was run)
- store-compliance checks: pass/fail + exit code
- remote Store listing: inspected / not inspected
- files modified: no

**Required local changes** — table rows with `Target` = Local file / Local artifact that failed. Empty section if none; write "none".

| Finding | Evidence | Level | Row | Source |
|---|---|---|---|---|

`Evidence` is the verbatim CLI line or the measured value. `Source` is the identifier and file, or the doc anchor link.

**Remote conditions not verified** — every emitted row with a remote target, each with its doc anchor link. Then, verbatim:

> Store publication readiness cannot be confirmed from local validation alone because the remote Store listing was not inspected.

**Local candidates** — measured local values and whether each looks compatible with its remote rule, with the rule's source. Explicitly not a pass.

**Guidance** — only `Store-doc-guidance` rows (G1, G2), each with its link and the exact modal verb the docs use. Guidance rows appear **here only**. They are not remote conditions and must not be repeated in that table.

**Sources checked** — a flat list the user can re-verify independently:

- CLI path, version, source revision
- every source file path read, with the `grep` command that locates the rule
- the Store docs index, as a clickable link, so the user can reach the whole set
- every doc page actually read, as a clickable full URL with the date read
- the pages **not** read, listed by name with the trigger that would have required them, so the user can see what was out of scope rather than assuming it passed
- anything else not consulted, stated plainly (e.g. "Shopware Account: not accessed")

The response ends here. Do not append a summary, a recap, a next-steps list, or an offer to fix anything: the sections above already say what is wrong and what is unverified, and a summary reintroduces the collapsed local/remote framing the format exists to prevent.

Every source must be a clickable URL, in every section including table rows. `content-and-translations § store-listing` is not a link.

Repeating a 100-character URL across eleven rows is tedious, so a table may instead declare its base once directly above itself and carry only anchors in the rows:

> Sources below are anchors on https://developer.shopware.com/docs/guides/development/testing/store/content-and-translations.html

with rows reading `#store-listing`. Use one form or the other. Bare anchors with no declared base are not acceptable — that is the failure this replaces.

## 6. Self-check

Answer only after all are true:

- every `Level`/`Target`/`Source` was copied from §2, not reasoned out;
- every finding names a source the user can open or grep, as a full URL;
- the Sources section links the docs index and names the pages not read;
- no remote-target row appears as a required local change;
- no ungated precondition row was emitted;
- no icon finding for a 112–256 px icon;
- no CLI-enforced row emitted when the CLI printed `No problems found`;
- no row emitted as "N/A";
- every ungated row emitted, counted against §2 rather than eyeballed;
- every source is a clickable URL, or an anchor under a base declared directly above its table;
- G rows appear in Guidance only, not in the remote table;
- the response ends at Sources checked;
- every finding's row ID matches that row's actual subject;
- every candidate entry quotes a value that conflicts with a rule, rather than speculating about one;
- every required local change quotes a CLI line or a measured value;
- Account-license comparison uses the `license` value, not the package name;
- one CLI binary used throughout, and its version is stated;
- the "Sources checked" section lists what was *not* inspected as well as what was.
