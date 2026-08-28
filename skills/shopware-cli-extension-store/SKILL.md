---
name: shopware-cli-extension-store
description: MUST use for Shopware Store readiness/publication/submission/compliance questions. Inspect read-only, prefer a sibling checkout-built CLI automatically, verify current official Store docs, and show sources for every Store requirement.
---

# Shopware Store readiness

Assess the current extension for Store publication. Do not modify files when the user asks for inspection only. Do not ask preflight questions that can be answered from the workspace.

## 1. Run the local checks first

Inspect the live extension:

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

Then select and use the CLI in one deterministic block:

```bash
PATH_CLI="$(command -v shopware-cli)"
CLI="$PATH_CLI"
if [ -x ../shopware-cli/bin/shopware-cli ]; then
  CLI="../shopware-cli/bin/shopware-cli"
fi

printf 'PATH CLI: %s\n' "$PATH_CLI"
"$PATH_CLI" --version
printf 'Authoritative CLI: %s\n' "$CLI"
"$CLI" --version
"$CLI" extension validate . --reporter markdown
"$CLI" extension validate --help
if "$CLI" extension validate --help | grep -q -- '--store-compliance'; then
  "$CLI" extension validate . --store-compliance --reporter markdown
fi
"$CLI" extension config-schema
```

Hard rule: if `../shopware-cli/bin/shopware-cli` exists, it is authoritative. Every validation/help/schema command must use it. A result that falls back to plain `shopware-cli` after detecting the sibling binary is invalid and must be rerun before answering.

## 2. Keep four truths separate

Never merge:

- **Workspace truth**: local files and values.
- **CLI truth**: what the authoritative CLI reports/enforces.
- **Store-policy truth**: what current official Shopware docs require/recommend.
- **Remote-listing truth**: what is actually configured in Shopware Account.

A clean CLI run does not prove Store readiness. Local metadata does not prove remote listing state.

If remote Account state was not inspected, call it **remote unverified**. Never say a remote value is missing, satisfied, compliant, or ready based only on local files.

## 3. Source every Store claim

For Store listing/content requirements, read the current official docs and link the exact page in the answer. Start with:

- [Content and translations](https://developer.shopware.com/docs/guides/development/testing/store/content-and-translations.html)

Only consult other Store pages when the current extension contains a feature that makes them relevant.

Preserve source strength:

- `must` / `required` -> **Store-doc-required**
- `should` / `recommended` / `preferred` -> **Store-doc-guidance**

No direct official source = no Store requirement claim.

## 4. Never infer Store fields from names

Docs establish the required Store result; schema only proves a local field exists. Neither proves how a field maps to Shopware Account.

If discussing local Store metadata synchronization, inspect current CLI implementation (not just schema), especially `internal/account-api/producer_store_push.go`.

Current implementation must be rechecked before relying on mappings. In the current code:

- plugin/composer metadata description -> remote `ShortDescription`
- `store.description` -> remote long `Description` when configured
- `store.installation_manual` -> remote `InstallationManual` when configured
- `store.meta_description` -> remote `MetaDescription`
- `store.availabilities` -> remote Store availabilities when configured

Therefore:

- never use `store.meta_description` as the Store short description unless current implementation changes to prove that mapping;
- never make `store.description`, `store.availabilities`, `store.type`, `store.demo_shops`, or other `store.*` fields required local changes merely because schema supports them;
- optional sync fields belong in recommendations only when useful to the user's workflow.

Do not include schema-only fields as readiness findings unless they correspond to an actual current deficiency or the user asks about repository-managed Store metadata.

## 5. For this kind of readiness request, check these current Store conditions when applicable

Verify each against the current official source before reporting:

- International Store publication requirement
- German/English 1:1 parity only if German Store publication is intended
- Store short description 150-185 characters
- Store long description minimum 200 characters
- meaningful/accurate listing text and setup/configuration instructions
- English fallback for settings/error messages only if such user-facing content exists
- Shopware Account license matches `composer.json`
- `src/Resources/config/plugin.png` Store favicon requirement (current docs specify 112x112)
- screenshot/image guidance, preserving `should` wording as guidance

For local short-description candidates, measure the exact plugin metadata fields, e.g.:

```bash
jq -r '.extra.description."en-GB" | length' composer.json
jq -r '.extra.description."de-DE" | length' composer.json
```

If those values are in range, say only: **local candidate looks compatible; remote short description remains unverified**. Never say `short descriptions pass` without inspecting the remote listing.

The normal CLI may accept `plugin.png` dimensions that differ from the Store docs. Report CLI acceptance and Store-doc requirement separately. Do not downgrade a docs `must` requirement to guidance just because CLI validation passes.

## 6. Do not pad the assessment

For a normal readiness request, omit hypothetical or schema-only items that are not current deficiencies, including:

- `store.type`
- demo shops
- API credential validation when there is no API feature
- install/uninstall hooks merely because the plugin class is minimal
- logging/cross-domain/JavaScript rules when those features are absent
- README/CHANGELOG/LICENSE-file pseudo-requirements
- generic compatibility-date advice

Passing validation does not prove absent patterns are CLI-enforced.

## 7. Required output

Start with **Validation status**:

- PATH CLI binary/version
- authoritative CLI binary/version
- normal validation result
- Store-compliance result
- remote Store listing: inspected / not inspected
- files modified: yes/no

Then show only actionable current findings:

| Finding | Evidence | Classification | Source | Applies to | Action |
| --- | --- | --- | --- | --- | --- |

Classifications:

- **CLI-enforced**: fresh CLI finding or current CLI source/test proves enforcement
- **Store-doc-required**: current official docs explicitly require it
- **Store-doc-guidance**: current official docs use non-mandatory wording
- **Schema-supported, not proven required**: schema support only
- **Recommendation / inference**: no authoritative requirement source

Then:

### Required local changes

Only actual local deficiencies that are CLI-enforced or Store-doc-required.

### Required Store conditions to verify

Every applicable required remote condition not inspected. Local candidates stay remote-unverified.

If remote state was not inspected, state:

> Store publication readiness cannot be confirmed from local validation alone because required remote Store listing state was not inspected.

### Guidance / recommendations

Only Store-doc-guidance and genuinely useful optional recommendations. Do not promote schema-only sync fields.

### Sources checked

List only sources actually consulted, with direct clickable Store-doc links plus the exact CLI/source evidence used.

## Final checks before answering

- sibling checkout CLI used everywhere when present
- no preflight question
- no file modifications
- no remote `pass`/`missing` claim without remote inspection
- no `store.meta_description` = short-description mistake
- no remote Store requirement converted into mandatory YAML
- no schema-only noise (`store.type`, demo shops, etc.)
- no hypothetical feature rules
- docs `must` stays required; docs `should` stays guidance
- 128x128 CLI acceptance does not erase a current docs 112x112 requirement
