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

- Plugin or bundle: the part of `extra.shopware-plugin-class` after the last `\`.
- App: `<meta><name>` in `manifest.xml`.

## Config file to edit

The CLI loads the first file that exists:

1. `.config/shopware-extension.yml`
2. `.shopware-extension.yml`
3. `.shopware-extension.yaml`

Edit that file only. If none exists, create the recommended one:

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
- In PHP that reads or writes system config, such as `systemConfigService->get('SwagExample.settings.clientSecret')`: copy that string. A `.settings.` key is a different key from `.config.`.

Include a key when it is a live credential:

- `config.xml` field type `password`
- a secret, token, password, private key, API key, client secret, client id, webhook secret, or SMTP password

Skip display and behavior settings: titles, feature toggles, sandbox flags, log level, locales, colors, CSS, snippet keys, and sales channel pickers. Clearing those makes a restored shop unusable.

### tables

Use database storage names. The DAL property name is not the column.

- Entity definitions: `ENTITY_NAME` or `getEntityName()` is the table. The first argument of a field constructor is the column. In `new StringField('access_token', 'accessToken')`, the column is `access_token`.
- Migrations: `CREATE TABLE` column lists. When an entity definition and a migration both describe the table, they should agree; use the storage name from the definition.
- Translated fields live in `{entity}_translation`. Mapping entities have their own entity name.

Include a column when it stores personal data or a secret: email, first name, last name, company, title, street, zip code, city, phone, IP address, token, secret, password, API key, refresh token, or private key.

Skip `id`, foreign keys (`*_id`), `version_id`, timestamps, state technical names, and quantities.

Skip Shopware core tables. The dump already rewrites `customer`, `customer_address`, `log_entry`, `newsletter_recipient`, `order_address`, `order_customer`, and `product_review`.

Custom fields this extension adds to a core entity live in that entity's `custom_fields` JSON. Remove only this extension's keys:

```yaml
tables:
  customer:
    custom_fields: "JSON_REMOVE(custom_fields, '$.swag_example_vat_id')"
```

Do not replace the whole `custom_fields` value.

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

Write faker calls without `{{- -}}`. The dump adds those delimiters.

Quote empty string and NULL as `"''"` and `"NULL"`. A bare `''` is an empty YAML value, and a bare `NULL` is YAML null. The CLI rejects both.

A string column with `->addFlags(new Required())`, or a migration column declared `NOT NULL`, is required. When a string secret's nullability is unclear, use `"''"`. Importing `NULL` into a `NOT NULL` column fails.

An expression containing the text `faker.` is inserted as a quoted literal, not executed as SQL. Keep `faker.` out of `JSON_REMOVE` and `JSON_REPLACE`.

`system_config` entries are key names only. The dump stores `{"_value": null}` in `configuration_value` for those keys.

Table and column names must match `^[A-Za-z_][A-Za-z0-9_]*$`. Config keys must match `^[A-Za-z0-9_.]+$`.

## Write the section

Keep every other key, comment, and `compatibility_date`. Add missing tables, columns, and config keys.

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
    - SwagExample.config.clientId
```

## After editing

Re-read the config file and check that:

- the new keys are in the one file the CLI loads
- keys outside `anonymize` are unchanged
- every new column is a storage name present in the extension
- every new system config key is a `<name>` from `config.xml` or a string passed to SystemConfig

Then run:

```bash
shopware-cli extension validate . --format markdown
```

Fix the config when the error names the extension config file and `anonymize`. Leave unrelated validation findings alone.

## Tell the developer

Close with two short lists:

- Added: `table.column` or config key, the expression, and the file it came from.
- Left out: fields you saw and did not treat as personal data or secrets, with the reason.

Do not print live secret values from config, `.env`, or a database. Key names and column names are fine.
