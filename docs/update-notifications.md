# Update notifications

This document describes how `shopware-cli` checks for newer CLI releases and
how it presents the result. It is intended for contributors changing the
update-check or TUI startup flow.

## TL;DR

- The root command starts one background update check per invocation.
- Release metadata comes from
  [`version.json`](https://shopware.github.io/shopware-cli/version.json) and is
  cached for 24 hours in `update-check-info.json`.
- A newer release is printed to `stderr` after command execution, at most once
  per 24 hours. The message links to the repository for installation options.
- TUIs display a compact `Update available` hint in their shared header when
  the same check finds a newer release. The TUI hint is not subject to the
  command-line display throttle.
- The check is skipped for dev builds, CI, `--no-update-hint`/`-n`, and when
  `SHOPWARE_CLI_NO_UPDATE_NOTIFICATION=true`.
- A failed check is non-fatal. The command continues, and the failure is only
  logged at debug level.

## User-visible behavior

### Command-line notification

The check starts before the command runs so normal command work can overlap
with the network request. Before the process exits, the root command waits for
the shared check result. If a newer release is available, it prints a bordered
notification to `stderr` after the command output.

The notification is shown only when both conditions are true:

1. The installed version is older than the release reported by `version.json`.
2. The notification cache says that no detailed notification was printed in
   the previous 24 hours.

The timestamp is written only after the notification has been rendered. A
failure to write the timestamp is logged, but does not change the command exit
status.

### TUI notification

The shared TUI header (`internal/tui/header.go`) displays a compact update hint
when the background check reports a newer release. The hint is available in
the development dashboard and the upgrade and plugin-migration wizards because
they all use `tui.Header`.

The TUI does not write or consult `update-notification.json`. Consequently,
the compact hint can appear on every TUI invocation while a newer release is
available. This is intentional: the hint is part of the unobtrusive header,
whereas the command-line message is a detailed notification.

Homebrew users are treated specially. If the executable is below Homebrew's
`<prefix>/bin` and the release was published less than 24 hours ago, neither
surface reports it yet. This gives Homebrew time to publish the new formula.

## When the check runs

`update.ShouldCheckForUpdate` is the single policy gate. It returns false when:

- the version is `dev`;
- the process is detected as running in CI;
- the arguments contain `--no-update-hint` or `-n`; or
- `SHOPWARE_CLI_NO_UPDATE_NOTIFICATION` is exactly `true`.

The `-n` short flag is also used by the existing `--no-interaction` option, so
non-interactive invocations using `-n` disable update checks as a side effect.
Use `--no-interaction` when a script needs non-interactive mode but should not
rely on that behavior being coupled to update notifications.

The root command creates a 900 ms context timeout for the background check.
The HTTP client itself has a 5 s timeout, but the shorter root context normally
ends the request first. If the check does not finish in time, no notification
is shown and the command still completes normally.

## Caching and release comparison

There are two independent cache files below
`system.GetShopwareCliCacheDir()`:

| File | Purpose | Retention policy |
| --- | --- | --- |
| `update-check-info.json` | Latest fetched `ReleaseInfo` | A fetch is reused for 24 hours. |
| `update-notification.json` | `last_printed_at` for the detailed CLI message | Suppresses the detailed message for 24 hours. |

The default cache directory is the user cache directory plus
`shopware-cli`. Tests and controlled environments can override it with
`SHOPWARE_CLI_CACHE_DIR`.

`ReleaseInfo` contains:

```json
{
  "version": "v1.2.3",
  "published_at": "2026-08-18T10:00:00Z",
  "fetched_at": "2026-08-18T10:01:00Z"
}
```

The reported version is compared with the build version using semantic version
comparison. Git-describe suffixes such as `-123-gabcdef12` are removed from
the installed version before comparison. Invalid versions are treated as not
being newer.

The fetch endpoint is deliberately a small, stable JSON document maintained
with each CLI release. Changes to its schema must be coordinated with
`internal/update.ReleaseInfo` and its tests.

## Control flow

The important ownership boundaries are:

1. `cmd.run` creates an `update.CheckHandle` and starts `checkForUpdate` in a
   goroutine.
2. `internal/update.CheckForUpdate` loads the release cache, fetches metadata
   only when the cache is older than 24 hours, and compares versions.
3. `cmd.run` consumes the result after command execution and decides whether
   to print the detailed notification and mark it as printed.
4. `tui.Header.Init` waits on the same handle through
   `tui.NewUpdateCheckCmd`. When the result is positive, Bubble Tea receives
   `UpdateAvailableMsg`; `Header.Update` then rebuilds the header with the
   compact hint.

The handle is important: the CLI and a TUI must not start independent network
requests or race over cache state. `CheckHandle.Complete` is guarded by
`sync.Once`, and `Wait` supports cancellation through its context.

## Tests and change checklist

When changing this feature, update the focused tests before running the full
suite:

- `internal/update/update_test.go`: version comparison, cache reuse, policy
  gates, rendering, and notification throttling;
- `internal/update/handle_test.go`: shared-result completion and cancellation;
- `internal/tui/update_test.go` and `internal/tui/header_test.go`: message
  delivery and header rendering;
- root-command tests, when changing Homebrew detection or command wiring.

Preserve these invariants:

- update failures never fail the user's command;
- at most one update request is started per CLI invocation;
- cache timestamps are recorded only for the event they represent;
- the detailed CLI notification is written to `stderr`, after command
  execution; and
- the TUI consumes the shared result without adding another network request.

For a behavior change, update this document and the user-facing options in
`README.md` together.
