---
name: shopware-cli-extension-store
description: MUST use for Shopware Store readiness/publication/submission/compliance questions. Inspect read-only, automatically prefer a sibling checkout-built CLI, verify current official Store docs, and keep local CLI facts, Store policy, and remote Account state separate.
---

# Shopware Store readiness

Assess the current extension for Store publication. Start immediately. Do not modify files when the user asks for inspection only, and do not ask questions that the workspace can answer.

## 1. Inspect and select the CLI deterministically

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

Then run the CLI checks in one block:

```bash
PATH_CLI="$(command -v shopware-cli)"
CLI="$PATH_CLI"
if [ -x ../shopware-cli/bin/shopware-cli ]; then
  CLI="../shopware-cli/bin/shopware-cli"
fi

printf 'PATH CLI: %s\n' "$PATH_CLI"
[ -n "$PATH_CLI" ] && "$PATH_CLI" --version
printf 'Authoritative CLI: %s\n' "$CLI"
"$CLI" --version
"$CLI" extension validate . --reporter markdown
"$CLI" extension validate --help
if "$CLI" extension validate --help | grep -q -- '--store-compliance'; then
  "$CLI" extension validate . --store-compliance --reporter markdown
fi
```

Hard rule: if `../shopware-cli/bin/shopware-cli` exists, it is authoritative. Every validation/help command must use it. If a later command uses plain `shopware-cli`, rerun it before answering.

Only run `extension config-schema` when the extension already uses Store sync configuration or the user asks where Store metadata can be configured. Schema fields are not readiness requirements.

## 2. Keep four truths separate

Never merge:

1. **Workspace truth** — local files and values.
2. **CLI truth** — what the authoritative CLI reports/enforces.
3. **Store-policy truth** — what current official Shopware Store docs require or recommend.
4. **Remote-listing truth** — what is actually configured in Shopware Account.

A passing CLI run proves only CLI truth. Local metadata does not prove remote listing state.

If Shopware Account was not inspected, every Account/listing condition is **remote unverified**. Do not call it missing, satisfied, passed, compliant, or ready.

## 3. Use the current official Store source as the policy baseline

For ordinary Store listing readiness, read the current official page before answering:

- [Content and translations](https://developer.shopware.com/docs/guides/development/testing/store/content-and-translations.html)

Use the wording on the live page, not memory. When the page says `All extensions must`, every item in that list is **Store-doc-required**. Do not downgrade it because CLI validation passes.

At the time this skill was authored, that page establishes these Store-listing requirements; re-check the live page before reporting them:

| Store condition | Evidence level | Target |
| --- | --- | --- |
| International Store publication | Store-doc-required | Remote listing |
| German/English 1:1 parity, only when German Store is used | Store-doc-required | Remote listing |
| Short description 150–185 characters | Store-doc-required | Remote listing |
| Long description minimum 200 characters | Store-doc-required | Remote listing |
| Meaningful, accurate short/long descriptions and use cases | Store-doc-required | Remote listing |
| Display name avoids `plugin` and `shopware` | Store-doc-required | Remote listing |
| Clear, complete setup/configuration instructions | Store-doc-required | Remote listing |
| `src/Resources/config/plugin.png` is 112×112 | Store-doc-required | Local artifact |
| English fallback for applicable settings/error messages | Store-doc-required | Extension behavior/content |
| Shopware Account license matches `composer.json` license | Store-doc-required | Remote Account |
| Screenshots/images where the docs say `should`/`prefer` | Store-doc-guidance | Remote listing |

Do not turn a `should` into `must`, and do not turn an `All extensions must` item into guidance.

Only consult other Store pages when the current extension contains a feature that makes them relevant or the user asks for a broader policy audit.

## 4. Measure local candidates, but never use them as remote proof

Measure exact local fields when they can serve as Store listing candidates:

```bash
jq -r '.extra.label."en-GB", .extra.label."de-DE"' composer.json
jq -r '.extra.description."en-GB" | length' composer.json
jq -r '.extra.description."de-DE" | length' composer.json
jq -r '.license' composer.json
```

If a local description is 150–185 characters, say:

> local candidate is within the documented length range; remote short description remains unverified

Never say `short description passes` or `descriptions pass` unless the actual remote listing was inspected.

If a local label contains a word prohibited by the current Store docs, report it only as a **local candidate that appears incompatible with the remote display-name rule**; the actual remote display name remains unverified.

For license, compare the remote Account value to the actual `composer.json` license value. Example: if composer says `MIT`, the condition is `Shopware Account license must match MIT`. Never compare the Account license to the package name.

## 5. Field mappings require implementation evidence

Store docs establish the required result. Schema only proves a local field exists. Neither proves how a local field maps to Shopware Account.

If you recommend repository-managed Store metadata, inspect current CLI implementation, especially `internal/account-api/producer_store_push.go`, before mapping fields.

Current implementation must be rechecked before relying on these mappings. In the current code:

- plugin/composer metadata description -> remote `ShortDescription`
- `store.description` -> remote long `Description` when configured
- `store.installation_manual` -> remote `InstallationManual` when configured
- `store.meta_description` -> remote `MetaDescription`
- `store.availabilities` -> remote Store availabilities when configured

Therefore:

- never use `store.meta_description` as the Store short description;
- never make `store.description`, `store.availabilities`, `store.type`, `store.demo_shops`, or other `store.*` fields required local changes merely because schema supports them;
- the Store requirement is the resulting remote listing state; YAML is only an optional synchronization mechanism unless another source explicitly requires it.

## 6. Interpret validation correctly

Normal and Store-compliance validation passing means only that the current CLI found no problems in those modes.

It does not erase Store-doc-required differences. If the Store docs require 112×112 and the local `plugin.png` is 128×128, report a **required local change** even when CLI validation passes.

Do not claim an absent pattern is CLI-enforced merely because validation passed.

## 7. Do not pad the answer

For a normal readiness request, omit unrelated or schema-only items such as:

- `store.type`
- demo shops
- API credential validation when no API feature exists
- install/uninstall hooks merely because the plugin class is minimal
- logging/cross-domain/JavaScript rules when those features are absent
- README/CHANGELOG/physical LICENSE-file pseudo-requirements
- generic compatibility-date advice

Do not say `No findings` when either:

- a current Store-doc-required local mismatch exists; or
- required remote Store conditions remain unverified.

You may say `No CLI findings` when both CLI modes pass.

## 8. Required response format

Start with **Validation status**:

- PATH CLI binary/version
- authoritative CLI binary/version
- normal validation result
- Store-compliance result
- remote Store listing: inspected / not inspected
- files modified: yes/no

Then show current actionable findings:

| Finding | Evidence | Classification | Source | Applies to | Action |
| --- | --- | --- | --- | --- | --- |

Use exactly these classifications:

- **CLI-enforced**
- **Store-doc-required**
- **Store-doc-guidance**
- **Schema-supported, not proven required**
- **Recommendation / inference**

### Required local changes

Only demonstrated local deficiencies that are CLI-enforced or Store-doc-required. A 128×128 `plugin.png` is a required local change when the current Store docs still require 112×112.

### Required Store conditions to verify

When remote state is not inspected, include every applicable required remote condition from the live Store docs. For the baseline listing page this normally includes:

- International Store publication;
- short description 150–185 characters;
- long description minimum 200 characters;
- meaningful/accurate descriptions and use cases;
- compliant display name;
- clear setup/configuration instructions;
- Shopware Account license matching the exact `composer.json` license;
- German/English parity only if German Store publication is intended;
- English fallback only when applicable user-facing settings/error messages exist.

For local candidates, show compatibility/incompatibility separately from remote status.

State:

> Store publication readiness cannot be confirmed from local validation alone because required remote Store listing state was not inspected.

### Guidance / recommendations

Only actual Store-doc-guidance and useful optional recommendations. Screenshots belong here when the current docs use `should`/`prefer`. Optional YAML synchronization fields belong here only when useful; never present them as required changes.

### Sources checked

List only sources actually consulted, with direct clickable Store-doc links and exact CLI/source evidence.

## Final audit

Before answering, verify all of these:

- sibling checkout CLI used everywhere when present;
- no preflight question;
- no file modifications;
- docs `All extensions must` items stayed Store-doc-required;
- no remote `pass`/`missing` claim without remote inspection;
- no `store.meta_description` = short-description mistake;
- no remote Store requirement converted into mandatory YAML;
- `composer.json` license value, not package name, is used for the Account-license comparison;
- 128×128 local icon is reported as a Store-doc-required local mismatch when live docs require 112×112;
- short description local values are candidates, never remote passes;
- long description remains required, not recommended/optional;
- International Store requirement is not omitted;
- screenshots stay guidance when docs say `should`;
- no schema-only noise or hypothetical feature rules.

If any check fails, correct the answer before sending it.
