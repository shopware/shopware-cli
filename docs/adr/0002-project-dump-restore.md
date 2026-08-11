title: Align project dump and restore with portable shop data
date: 2026-08-11
tags: [project dump, project restore, mysql, database]

## Context

`project dump` currently creates a SQL database dump, with optional gzip or zstd compression and support for Shopware-specific cleaning and anonymization.

[#1243](https://github.com/shopware/shopware-cli/issues/1243) proposed the corresponding `project restore` workflow. Work in [#1326](https://github.com/shopware/shopware-cli/pull/1326) added shared database connection handling and SQL execution infrastructure needed for that workflow, but `project restore` was held back while we aligned with Shopware PaaS and SaaS teams.

The resulting conversation established that transferable shop data can include three distinct components: the database, public files, and private files. It also surfaced two interoperability goals: first, making data produced by Shopware CLI suitable for a future migration into Shopware hosting; and second, allowing data exported from hosted environments to be restored elsewhere.

## Decision

We will proceed with `project restore` and keep the existing database-only `project dump` and its SQL format supported.

To ensure compatibility with other Shopware hosting models, dump and restore will also evolve around the same three independently selectable components: database, public files, and private files.

Whole-shop transfers will use a versioned manifest describing the included components and their formats. The portable structure should match the format expected by Shopware hosting where applicable and practical, so that data produced by Shopware CLI can be accepted by a future PaaS restore flow without conversion.

These compatibility requirements define the interchange contract; they do not require Shopware CLI to adopt hosting-specific implementation details that are unnecessary for interoperability. We commit to frictionless interchange, not to cloning PaaS/SaaS internals.

The database component will continue to support Shopware CLI's existing SQL dumps and will also support reading mysql-shell dumps.

Dump and restore will support partial operation on individual components and optional compression.

The first implementation may read from and write to the local filesystem, but the design must not hard-code local paths as the only source or destination. The dump/restore model must support the storage setup used by Shopware PaaS and SaaS, including S3, while remaining extensible to additional user-controlled storage backends.

This workflow is for transferring shop data. Production backup, disaster recovery, and platform snapshot lifecycle remain separate concerns.

## Consequences

- The pending `project restore` work can proceed for existing Shopware CLI SQL dumps.
- Existing `project dump` behavior remains compatible.
- Public and private files can be added to the same transfer model.
- mysql-shell dumps can be consumed without replacing Shopware CLI's existing database dumper.
- The transfer format should remain compatible with future PaaS import and export workflows where practical.
- Partial transfers, optional compression, and non-local storage can be added without introducing a separate data model.
- Work to define the migration path from/into Shopware PaaS/SaaS hosting is ongoing and supported by this ADR.
