title: Project import and export
date: 2026-08-11
tags: [project import, project export, mysql, database]

## Context

`project dump` creates a SQL database dump, with optional gzip or zstd compression and support for Shopware-specific cleaning and anonymization.

Transferable shop data can include three components: the database, public files, and private files. Shopware CLI should interoperate with Shopware hosting: data produced by the CLI should be restorable in hosted environments, and data exported from hosted environments should be restorable elsewhere.

A SQL dump is not enough for that. Files and a declared package layout are also required.

## Decision

`project dump` stays as it is: a database-only SQL stream to `dump.sql` or stdout, with optional gzip or zstd wrapping.

Portable shop-data packages are a new pair of commands:

- `project export` writes a tar archive.
- `project import` reads that tar archive.

Export and import operate on independently selectable components: database, public files, and private files. An archive does not have to include public or private files. `project import` applies every component that is present. Skipping public or private files is opt-out (`--skip-public`, `--skip-private`).

The archive is the interchange unit. Its layout should match the format expected by Shopware hosting where applicable and practical, so a CLI export can be accepted by a future hosted import without conversion, and a hosted export can be imported by the CLI.

These compatibility requirements define the interchange contract. They do not require Shopware CLI to clone PaaS/SaaS internals.

This workflow is for transferring shop data. Production backup, disaster recovery, and platform snapshot lifecycle remain separate concerns.

### Archive

`project export` writes one tar (optionally wrapped with gzip or zstd). Inside:

```text
metadata.json
database.sql          # SQL dump
database/             # complete, opaque MySQL Shell dump
  ...
public.tar.gz         # optional
private.tar.gz        # optional
```

Components are detected from those fixed names. `metadata.json` does not repeat paths or backup types.

`database.sql` and `database/` are alternatives. If both are present, import fails. A missing `public.tar.gz` or `private.tar.gz` means that filesystem is not in the package.

The first CLI export writes `database.sql` using the existing SQL dumper, including `--clean` / `--anonymize` and `ConfigDump` rewrite, where, ignore, and no-data rules. The CLI will not produce MySQL Shell dumps in the first implementation.

`project import` consumes the tar and picks the database payload from the layout:

- `database.sql` — applied to the project database.
- `database/` — loaded as MySQL Shell left it. Any directory that MySQL Shell's load utility can consume (`util.dumpInstance` or `util.dumpSchemas` output) is accepted. The CLI does not re-encode MySQL Shell internals.

Public and private files are optional tar archives of the corresponding Shopware filesystems, not of the project source tree. Export includes a filesystem only when that component is selected. Import applies `public.tar.gz` and `private.tar.gz` when they are present, unless the matching `--skip-public` or `--skip-private` flag is set. A missing file is valid and is not an error. When they are applied, export reads them and import writes them through the project's existing Flysystem configuration in `config/packages` (`shopware.filesystem.public` and `shopware.filesystem.private`). Only the `local` and `amazon-s3` adapters are supported. Any other adapter is rejected with a clear error.

`--anonymize` and `ConfigDump` rules apply only to the SQL dumper. They do not sanitize public or private files. File components are transferred as stored. Private files can include invoices, documents, and other sensitive shop data; that is expected and must be documented.

### metadata.json

`metadata.json` describes the package, not the file layout. Required fields for `format_version` 1:

- `format_version` (integer, currently `1`)
- `created_at` (RFC 3339 timestamp)
- `source` (`shopware_version`, and `database.vendor` / `database.version` when a database component is present)

```json
{
  "format_version": 1,
  "created_at": "2026-08-11T12:00:00Z",
  "source": {
    "shopware_version": "6.7.1.0",
    "database": {
      "vendor": "mysql",
      "version": "8.4.6"
    }
  }
}
```

Validation rules:

- Import rejects a missing `metadata.json` or an unknown `format_version` with a clear error. It does not attempt partial interpretation.
- Import ignores unknown fields (forward compatible).
- The archive must contain at least one of `database.sql`, `database/`, `public.tar.gz`, or `private.tar.gz`.
- If both `database.sql` and `database/` are present, import fails.

Field names and additional hosting-specific metadata may still be aligned with PaaS/SaaS before the first implementation. Adding fields is allowed; removing or renaming a required `format_version` 1 field requires a new `format_version`.

### Compression and storage

Inner payloads may be compressed on their own (`public.tar.gz`, `private.tar.gz`, `database.sql.gz`). The outer file is the tar that export writes and import reads; gzip or zstd wrapping of that tar is optional.

The first implementation writes and reads that tar on the local filesystem. Where the shop's public and private files themselves live is determined only by the Flysystem adapters above.

## Consequences

- `project dump` remains the database-only SQL command. Existing scripts keep working.
- `project export` / `project import` are the portable shop-data commands.
- Public and private files travel with the database in one archive, using the project's Flysystem config (`local` and `amazon-s3` only).
- Hosted MySQL Shell dumps can be imported when they arrive in this package layout, without replacing the CLI SQL dumper.
- SQL-only restore of a raw `dump.sql` is not `project import`. That remains a separate concern.
- Work to align the package with Shopware PaaS/SaaS hosting is ongoing and supported by this ADR.
