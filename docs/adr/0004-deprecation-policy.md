# ADR: Deprecation policy

**Status:** Accepted
**Related:** Discussion under issue #1407, deprecation tracking issue #1859

## Context

shopware-cli is used by many developers, often non-interactively in CI pipelines and build scripts.
A removed command, flag, env var or config key can break builds or development flow for users who never read our changelog.
Until now we deprecated things without a rule for when they are removed.

We need a predictable process that gives users clarity and gives maintainers a way to actually retire code.

## Scope

This policy covers the public interface of shopware-cli:

- Commands and subcommands
- Flags
- Environment variables (`SHOPWARE_CLI_*`)
- Config keys in `.shopware-extension.yml` and `.shopware-project.yml`

## Decision

1. **Every deprecation has a removal date.** The date is set when the deprecation is merged into `main`: merge date + 3 months, falling on a working day. Users get one quarter to adapt.
2. **Users are warned at runtime.** Using a deprecated item prints a warning to stderr (so stdout and JSON output stay clean). The warning states that the item is deprecated, its removal date and its replacement, if one exists. Deprecated commands and flags are hidden from `--help` output.
3. **Usage is tracked via telemetry.** Using a deprecated item is reported via telemetry, so we can see how many users still rely on it.
4. **Extensions are possible.** If a deprecated item is still widely used and users need more time, the removal date can be extended. Each case is assessed individually, based on telemetry, user feedback and our own knowledge. An extension ships in a new release with the updated date, and the sub-issue gets the new due date.
5. **#1859 is the source of truth.** The issue is pinned in the repository as a living document. Every deprecation is a sub-issue of #1859 and must have a due date (the removal date).
6. **Removals are automated reminders.** A scheduled GitHub Action sends a notification to our internal Slack channel for every deprecation due that week.
7. **Changelog has a Deprecations section.** Each release changelog lists new deprecations and removals in a separate section, linking to the sub-issues.
8. **Dev docs describe the process.** A new page in our dev docs explains how deprecation works and the default window, and links to #1859.

## Versioning

This policy applies even though shopware-cli is still below 1.0.0. If we release a major version, we will revisit this policy.

## Consequences

- Users know from day one when something goes away and have a quarter to adapt.
- Maintainers can remove deprecated code on a known date, without re-discussing each case.
- Each deprecation needs a sub-issue with a due date under #1859.