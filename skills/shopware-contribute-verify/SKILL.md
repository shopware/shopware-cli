---
name: shopware-contribute-verify
description: Use when someone needs to know which local checks to run before opening a pull request in a Shopware repository (shopware/shopware, shopware/shopware-cli, shopware/deployment-helper), or a CI check on their pull request failed and they want to know what it means and what to do next.
metadata:
  version: "0.0.1"
---

# Verify a change and understand CI

Help the contributor run the checks that apply to their change, report the
results honestly, and work out whether a red CI check is theirs to fix.

## Hard rules

- A check that didn't run is not a pass. Report each one as passed, failed,
  skipped or not run.
- Never dismiss a failure as "pre-existing" or "flaky" without evidence (the
  same failure on the default branch, or a known-flaky issue). Without it, the
  classification is "unknown".
- A similar old issue is a lead, not proof.
- Don't re-run CI over and over to get a green check.
- Never post, push or re-run anything on GitHub without the contributor's
  go-ahead.

## Local checks

Progress:

- [ ] 1. Find the checks in the repository's own files: `CONTRIBUTING.md`,
      `AGENTS.md`, the PR template, `mise.toml` or `composer.json` scripts,
      and the workflows in `.github/workflows/` (CI is the final word on what
      runs).
- [ ] 2. Pick the ones that cover the change; run the full suite only if the
      repository asks for it. Check prerequisites first (Go, PHP, Node,
      Docker, mise).
- [ ] 3. Use the versions CI pins (for example the linter version in the lint
      workflow), not whatever is installed.
- [ ] 4. Record each check, its command and its result in a table the PR
      description can reuse (`shopware-contribute-submit`).

| Repository                 | Start with                                                                                                         |
| -------------------------- | ------------------------------------------------------------------------------------------------------------------ |
| shopware/shopware-cli      | `mise run check` (format, build, tests, vet, Go, Markdown and conventions lint, workflow lint, lint-rule tests; not skills validation or smoke tests). Without mise, `go test ./...` and `golangci-lint run ./...` skip the Markdown, conventions and workflow lints and the lint-rule tests: name them as not run             |
| shopware/deployment-helper | `composer test`, `composer phpstan`, `composer cs-dry`                                                             |
| shopware/shopware          | The tests for the changed area and the linters in `AGENTS.md`'s file linting table; tests must fail without the fix |

Confirm these in the repository before running them; they can change.

## A CI check failed

Progress:

- [ ] 1. Get the evidence. Confirm the number is a pull request (`gh api
      repos/<owner/repo>/issues/<n> --jq '.pull_request != null'`); if it is
      an issue, ask for the PR. Then `gh pr checks <number> -R <owner/repo>`
      and the failing job's log: `gh run view <run-id> -R <owner/repo> --log-failed`.
- [ ] 2. Compare with the default branch: does the same job fail there now
      (`gh run list -R <owner/repo> --branch <default> --workflow <file>`)?
- [ ] 3. Classify, and give the evidence for it:

| Class                  | Evidence                                                                                             | Next step                                                       |
| ---------------------- | ---------------------------------------------------------------------------------------------------- | --------------------------------------------------------------- |
| Likely regression      | Fails in code or tests the change touches, or reproduces locally, and the default branch passes      | Reproduce locally, fix, push a follow-up commit                 |
| Known failure          | A known-flaky issue or label names the test, or the default branch fails the same way; the change doesn't touch it | Flaky: re-run once. Failing on the default branch: link the issue and wait for its fix. Either way, note it in the PR |
| Infrastructure failure | Runner, network, registry or service-container error unrelated to the code (for example an image pull timeout)    | Not the contributor's to fix; re-run later or tell maintainers  |
| Unknown                | None of the above holds                                                                              | Say so; ask maintainers with the log excerpt and what was tried |

A failure can be deterministic and still not the contributor's: if the
default branch and other PRs fail the same way, it's a known failure.

- [ ] 4. Workflows that wait for approval ("action required") on a fork PR
      aren't failures; a maintainer has to approve the run.

## Gotchas

- **A lint finding in a file the change didn't touch:** linters often check
  the whole repository, so a changed type or signature can surface elsewhere,
  or the branch is behind a fix on the default branch. Rebase, run the linter
  at the version CI pins, and compare with the default branch.
- **Markdown is linted in the CLI.** A new `.md` file can fail CI even when
  Go checks pass; `mise run lint-markdown` catches it locally.
- **Long-running checks:** if a check can't run locally (missing Docker,
  services, OS), say "not run" and why, rather than skipping it silently.
