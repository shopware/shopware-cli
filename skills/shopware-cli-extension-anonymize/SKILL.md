---
name: shopware-cli-extension-anonymize
description: Bootstrap the anonymize section in an extension's shopware-extension.yml so project dump --anonymize clears that extension's personal data and secrets. Use when an extension developer asks to declare anonymized tables, columns, system config keys, or dump secrets for a plugin, app, or bundle.
---

# Bootstrap extension anonymize config

Write the `anonymize` section so `shopware-cli project dump --anonymize` rewrites this extension's personal data and integration secrets. Change only that section. Leave the rest of the config file as it is.

Work on one extension at a time, from that extension's root.

## Confirm this is an extension

An extension root has one of:

- `composer.json` with `"type": "shopware-platform-plugin"` or `"shopware-bundle"`
- `manifest.xml` (Shopware app)

If the directory is a Shopware project, stop. Project dump rules belong in `.config/shopware-project.yml` under `dump.rewrite`.

The technical name is the prefix for `system_config` keys:

- Plugin: the part of `extra.shopware-plugin-class` after the last `\`.
- Bundle: `extra.shopware-bundle-name`.
- App: `<meta><name>` in `manifest.xml`.

## Config file to edit

The CLI loads the first file that exists:

1. `.config/shopware-extension.yml`
2. `.shopware-extension.yml`
3. `.shopware-extension.yaml`

Edit that file only.

If none of them exists, search the build scripts for `shopware-extension` first. Some repositories keep the config elsewhere, such as `config/.shopware-extension.yml`, and copy it to the root when the ZIP is built. Edit that file instead, and tell the developer that only packages with the root file carry the rules.

Only when the extension has no config at all, create the recommended one:

```bash
shopware-cli extension config init
```

That writes `.config/shopware-extension.yml` with the schema comment and `compatibility_date`. Add `anonymize` below those lines. Do not create a second config file beside one that already exists.

Confirm the object shape with the current schema instead of inventing keys:

```bash
shopware-cli extension config-schema
```

`anonymize` has two keys: `tables` and `system_config`.

## Find candidates in the extension

Declare only tables, columns, and keys that this extension's code contains.

### system_config

These are configuration keys, not database columns.

- In `config.xml` (plugins usually ship `src/Resources/config/config.xml`, apps `Resources/config/config.xml`): a field `<name>` is stored as `{technicalName}.config.{name}`.
- In PHP that reads or writes system config, such as `systemConfigService->get('SwagExample.settings.clientSecret')`: copy that string. Keys are often built from constants, such as `SYSTEM_CONFIG_DOMAIN . 'clientSecret'`; resolve them to the full key. A `.settings.` key is a different key from `.config.`.

Include a key when it is a live credential or personal data stored in system config:

- `config.xml` field type `password`
- a secret, token, password, private key, API key, client secret, client id, webhook secret, or SMTP password
- an email address or other personal data saved as configuration

For a secret, omit the row. A plain string entry does that:

```yaml
system_config:
  - SwagExample.config.clientSecret
```

This object form is the same thing:

```yaml
system_config:
  - key: SwagExample.config.clientSecret
    omit: true
```

Use `value` when the restored shop needs a specific setting, or when the stored value is personal data:

```yaml
system_config:
  - key: SwagExample.config.environment
    value: sandbox
  - key: SwagExample.config.merchantEmail
    value: faker.Internet.Email()
```

`value` is written to `configuration_value` as `{"_value": value}`. A string that starts with `faker.` is generated for each dumped row. `value: null` keeps the row and stores `{"_value": null}`. Do not set `omit` and `value` on the same key.

Skip settings that are not personal data and not secrets: titles, feature toggles, log level, locales, colors, CSS, snippet keys, and sales channel pickers. Omitting those makes a restored shop unusable. If the extension has a sandbox or test-mode setting, set it to sandbox or test mode with `value` (for example `value: true` or `value: sandbox`), so a restored shop cannot take live payments.

### tables

Use database storage names. The DAL property name is not the column.

- Entity definitions: `ENTITY_NAME` or `getEntityName()` is the table. The first argument of a field constructor is the column. In `new StringField('access_token', 'accessToken')`, the column is `access_token`.
- Migrations: `CREATE TABLE` column lists. When an entity definition and a migration both describe the table, they should agree; use the storage name from the definition.
- Translated fields live in `{entity}_translation`. Mapping entities have their own entity name.

Include a column when it stores personal data or a secret: email, first name, last name, company, title, street, zip code, city, phone, IP address, token, secret, password, API key, refresh token, or private key. Free text that can hold personal data, such as mail bodies, notes, or comments, counts too.

Skip `id`, foreign keys (`*_id`, or camelCase such as `customerId`), `version_id`, timestamps, state technical names, and quantities.

Skip Shopware core tables. The dump already rewrites `customer`, `customer_address`, `log_entry`, `newsletter_recipient`, `order_address`, `order_customer`, and `product_review`.

Custom fields this extension adds to a core entity live in that entity's `custom_fields` JSON. Remove only this extension's keys:

```yaml
tables:
  customer:
    custom_fields: "JSON_REMOVE(custom_fields, '$.swag_example_vat_id')"
```

Use `JSON_REMOVE` on the column itself. The CLI combines it with other extensions' `JSON_REMOVE` rules on the same column. Do not replace the whole `custom_fields` value.

Do not rewrite `system_config.configuration_value` under `tables`. That would wipe every shop setting. List keys under `system_config` instead.

## Choose the expression

`tables` values are SQL expressions, same as project `dump.rewrite`.

| Data | Expression |
|---|---|
| email | `faker.Internet.Email()` |
| first name | `faker.Person.FirstName()` |
| last name | `faker.Person.LastName()` |
| company, title, or a person name | `faker.Person.Name()` |
| street | `faker.Address.StreetAddress()` |
| zip code | `faker.Address.PostCode()` |
| city | `faker.Address.City()` |
| phone | `faker.Phone.Number()` |
| IP address | `faker.Internet.Ipv4()` |
| secret or token on a required or `NOT NULL` column | `"''"` |
| secret or token on a nullable column | `"NULL"` |
| free text, such as a mail body or a note | `"NULL"` on a nullable column, otherwise `"''"` |
| JSON column | `"'{}'"`, or `JSON_OBJECT` with faker templates (see below) |
| secret on a column with a UNIQUE index | `"NULL"` on a nullable column, otherwise a per-row value such as `"HEX(id)"` |

When the whole rewrite is a bare faker call, write it without `{{- -}}`. The dump adds those delimiters.

Quote empty string and NULL as `"''"` and `"NULL"`. A bare `''` is an empty YAML value, and a bare `NULL` is YAML null. The CLI rejects both. The outer double quotes are YAML, and everything inside them is SQL:

```yaml
access_token: "''"
refresh_token: "NULL"
payload: "'{}'"
```

A string column with `->addFlags(new Required())`, or a migration column declared `NOT NULL`, is required. When a string secret's nullability is unclear, use `"''"`. Importing `NULL` into a `NOT NULL` column fails.

Anything other than a bare faker call is SQL. To use faker inside SQL, put the template with its delimiters in a string literal. It is generated when the row is written:

```yaml
sender: "JSON_OBJECT('{{- faker.Internet.Email() -}}', '{{- faker.Person.Name() -}}')"
company: "IF(company IS NULL, NULL, '{{- faker.Person.Name() -}}')"
```

A plain faker call also fills rows that were NULL; the `IF` form keeps them NULL. A JSON column needs a valid JSON value: a plain faker call or text there breaks the import.

Table and column names must match `^[A-Za-z_][A-Za-z0-9_]*$`. Config keys must match `^[A-Za-z0-9_.]+$`.

## Write the section

Keep every other key, comment, and `compatibility_date`. Do not add `compatibility_date` to a file that has none. Add missing tables, columns, and config keys.

When `anonymize` already sets a column, keep that expression. Tell the developer about the difference. Do not overwrite it.

When the extension has no personal data and no secrets, do not add an empty `anonymize` block. Say so.

```yaml
anonymize:
  tables:
    swag_example_token:
      access_token: "''"
      email: faker.Internet.Email()
    customer:
      custom_fields: "JSON_REMOVE(custom_fields, '$.swag_example_vat_id')"
  system_config:
    - SwagExample.config.clientSecret
    - key: SwagExample.config.merchantEmail
      value: faker.Internet.Email()
    - key: SwagExample.config.environment
      value: sandbox
```

## After editing

Re-read the config file and check that:

- the new keys are in the one file the CLI loads, or in the file the build copies there
- keys outside `anonymize` are unchanged
- every new column is a storage name present in the extension
- every new system config key is a `<name>` from `config.xml` or a string passed to SystemConfig

Then run:

```bash
shopware-cli extension validate . --only builtin --format markdown
```

It loads the config and reports anonymize errors without PHP or JavaScript tooling. When the build copies the config to the root, the source tree has no config to load and the command checks nothing. Copy the file to the root path the build uses, run the command, then delete the copy.

Fix the config when the error names the extension config file and `anonymize`. Leave unrelated validation findings alone.

## Tell the developer

Close with two short lists:

- Added: `table.column` or config key, the expression, and the file it came from.
- Left out: fields you saw and did not treat as personal data or secrets, with the reason.

If the extension also stores personal data in files, such as archived mails, say that the dump does not cover them.

Do not print live secret values from config, `.env`, or a database. Key names and column names are fine.
