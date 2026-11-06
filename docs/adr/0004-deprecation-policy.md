# ADR: Deprecation policy

**Status:** Proposed
**Related:** Disussion under issue #1407

## Context

shopware-cli is used by many developers, often non-interactively in CI pipelines and build scripts. 
A removed command, flag, env var or config key can break builds or development flow for users who never read our changelog.
We deprecate commands and flags, but have no rule for when they are removed. 

Example: `--store-compliance` and `--full` are deprecated with no removal date.

Constraints:
- No fixed release cadence.
- Telemetry exists for commands, not for flags.

We need a predictable process that gives users clarity and gives maintainers a way to actually retire code.

## Scope

This policy covers the public interface of shopware-cli:

- Commands and subcommands
- Flags
- Environment variables (`SHOPWARE_CLI_*`)
- Config keys in `.shopware-extension.yml` and `.shopware-project.yml`
- Documented output formats that users may parse (e.g. JSON output)

## Decision drivers

- Clarity for users: they know *when* something goes away and *what to use instead*.
- CI safety: no silent breakage. Users get warned in the logs before removal.
- Maintainability: no indefinitely growing "half-dead" code paths.
- Consistency with the Shopware ecosystem, where users are used to deprecations with announced removal.
- Low process overhead: we're a small team.

## Options

**A. Keep deprecated items until they become a maintenance burden**
Never breaks users. Gives no clarity, and deprecated code stays indefinitely.

**B. Remove after a fixed window in calendar months**
Clear for users, works without a release cadence. Requires tracking dates.

**C. Remove in the next (or second next) major release**
Common convention. Open-ended without planned major releases.

## Decision

_To be agreed by the team._

## Questions to answer

1. Which option do we adopt?
2. If B: how long is the window?
3. How do we warn users (help text, runtime warning, changelog)?
4. How do we assess impact for flags without telemetry?
5. Do we allow exceptions, and how are they recorded?

## Proposal

We adopt **Option B**, with an explicit, recorded exception path.

1. **Every deprecation has a removal target from day one.** The warning, the changelog and the docs state the earliest removal date and the replacement.
2. **Minimum window: 3 calendar months** from the first release that emits the warning. Longer windows are allowed for widely used features.
3. **Batched removals.** Removals land together in a release.
4. **Runtime warnings, not only help text.** Using a deprecated item prints a warning to **stderr** (so stdout and JSON output stay clean), with the replacement Deprecated flags or commands are hidden from `--help` output.
5. **Impact check before removal.**
   - Commands: check telemetry before the removal release.
   - Flags, env vars, config keys: no telemetry today. We search public usage (GitHub code search, shopware/github-actions, docs, Shopware's own repos) and fix our own usages first.
   - *Open question:* whether to collect anonymous flag-usage telemetry (flag names only) to make this measurable.
6. **Exceptions are explicit.** If the team decides to keep a deprecated item longer (e.g. maintenance cost is negligible and user impact is high), we record that decision in the tracking issue with a new review date. "Deprecated" never means "undecided".
7. **Tracking.** Each deprecation gets an issue labeled `deprecation` with the removal date and a checklist: warning, docs, changelog, own usages migrated, impact check, removal.