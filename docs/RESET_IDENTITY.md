# Reset a shop's app identity in a database dump

Use `shopware-cli project dump --reset-identity` to remove the source shop's app
identity from a database copy. The source shop is unchanged.

```bash
shopware-cli project dump --reset-identity --output identity.sql
```

## App identity and registrations

Identity reset omits `core.app.shopId` and `core.app.shopIdV2` from system configuration.
The destination generates its own identity on first use. The following records are
also omitted.

- Apps with completed or pending registrations at an app server.
- Integrations belonging to those apps, including their API credentials.
- Registration secrets retained for previously deleted apps.

Use `--clone-shop` to also omit transient data such as pending jobs and webhook
delivery logs. It is shorthand for `--clean --reset-identity`.

```bash
shopware-cli project dump --clone-shop --output staging.sql
```

Local-only apps without registration secrets and unrelated API integrations
retain their records and credentials. Mail delivery and standalone webhooks remain
configured as before.

## Related records and reinstall

Dependent rows follow database foreign key delete rules during export.

- `CASCADE` omits the referencing rows, including transitive dependencies such
  as action-button translations.
- `SET NULL` keeps the record and sets the affected foreign-key columns to NULL.
  Payment and shipping registrations can retain identities and metadata that
  Shopware uses to reconnect them during reinstall.

Reinstall removed apps in the destination shop to register them under its new
identity. Their app files must be available in the destination project.

## Existing dump options

For a clean clone, combine `--clone-shop` with `--anonymize`. This resets app
identity, omits transient data and anonymizes personal and extension-declared
sensitive data.

```bash
shopware-cli project dump --clone-shop --anonymize --output clone.sql
```

- Use `--reset-identity` alone when you need to retain transient data.
- Add `--limit order=100` for a smaller dataset. Limits also filter dependent
  tables and require `CREATE` and `DROP` privileges.
- Add `--compression zstd --output clone.sql.zst` for a compressed dump.

Project `dump.where`, `dump.rewrite`, `dump.nodata` and `dump.ignore` rules also
apply alongside identity reset.

## Restrictions

Custom rewrites of foreign-key columns or referenced keys may only set nullable
columns to SQL `NULL`. Affected foreign keys that share columns are rejected when
their delete actions could conflict, even if the current rows do not overlap.

If filters or database relationships cannot be handled safely, the command fails
before writing the dump and reports the affected table or foreign key.
