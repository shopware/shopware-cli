# Command conventions

How commands, flags, messages and exit codes are written in `shopware-cli`. Follow these rules for new commands and when touching existing ones, so the CLI keeps one voice. When a rule and existing code disagree, the code gets fixed.

## Commands

**Short**

- Imperative, capitalised, no trailing period, under 80 characters: `Build and install Administration assets`.
- A parent command without `RunE` describes the group, not one child: `Manage the project configuration file`, `Manage installed Shopware extensions`.

**Long**

- Open in the imperative, same voice as the Short. Plain statements of fact are fine after that, for example `Requires a Docker environment`.
- Examples go into the `Example` field, never into `Long`. Cobra prints them under its own heading.
- No empty `Long: ""` fields.

**Use and arguments**

- Required arguments are bare, optional ones are in brackets, variadic ones get three dots: `validate path`, `fix [path]`, `activate name...`, `package path [branch]`. No angle brackets.
- Every leaf command declares `Args`. A command without positional arguments uses `cobra.NoArgs`, so a stray argument fails with `unknown command "x" for "..."` instead of being ignored.
- A command that passes arguments through shows that: `console command [args...]`.
- Aliases mirror their siblings: `ls` for every `list`, `rm` for `delete`, `watch-admin` beside `admin-watch`.
- Deprecate with the same lowercase sentence cobra uses for flags: `use "project upgrade" instead, will be removed in October 2026`. Always name the replacement.

## Flags

- Usage text is capitalised, has no trailing period, and does not repeat the default; pflag prints `(default ...)` itself.
- Enumerated values are listed in parentheses with commas and no "or": `(summary, json, github, gitlab, junit, markdown)`, `(plugin, theme)`.
- Lists are `(comma-separated, e.g. phpstan,eslint)`. The example must be pasteable: lowercase, real tool names.
- Values in help are lowercase. `--format` values are parsed case-insensitively, so `--format JSON` works, but the help keeps `json`. Other enumerated flags are strict.
- A flag that several sibling commands share (`--only`, `--exclude`, `--format`, `--no-copy`, `--allow-non-git`, `--dry-run`) has the same sentence on each of them.
- Register a flag on the commands that read it. A persistent flag that a subcommand ignores is a bug in the help.
- A `NoOptDefVal` sentinel shows up in the help as `[="..."]`, so pick a word that reads well there (`select`), never a space or an internal marker.
- Numbers are numeric flags (`Uint`, `Int`) unless the value is handed through to another tool as a string.
- Flag deprecations: `use --with-elasticsearch=false instead`, lowercase, naming the replacement.

## Errors

- Returned errors are lowercase and start with `cannot <verb>`: `cannot read project config %s: %w`. Older `failed to` and `could not` wording is not rewritten for its own sake, but new and touched messages use `cannot`. Wrap with `%w`, not `%v`.
- No Go identifiers in messages: not `ReadConfig(%s)`, not `cannot InstallExtension`.
- Hints use the existing forms: `, use shopware-cli project config init to create one`, `: run "shopware-cli account login"`, `(pass --force to overwrite)`. Quotes, never backticks.
- Name config keys as they are in the schema (`environments.<name>.admin_api`) and the preferred file (`.config/shopware-project.yml`). Product names as written: `Admin API`.
- Invalid user input (a config value, a pattern, a flag value) returns an error. It never panics.

## Log lines

- Log lines are capitalised sentences with a colon before the value: `Installed %s`, `Cannot parse URL %s: %s`.
- A failed step keeps the established shape: `Activation of %s failed with error: %v`.
- Log a failure or return it, not both. The root command prints every returned error once, apart from the sentinel errors described under exit codes.
- Never log a success line after a failed step. In a loop, `continue` after a failure.

## Exit codes and output

- Any failure exits non-zero, in every output format. `--format json` has the same exit code as the table.
- Nothing succeeds silently on a wrong assumption: an explicit `--project-config` that does not exist is an error, not a fallback.
- When the error was already printed as part of the output (a failed verification check with its hint), return a sentinel error and add it to the exemption list in `cmd/root.go`, so it is not printed twice.
- Without a terminal, or with `--no-interaction`, a command that would prompt fails fast and names the flag or environment variable that replaces the prompt.

## Tests

- New tests target the `internal/` function a command calls, not the cobra layer. Keep `cmd/` thin enough that this is possible.
- Test behaviour: the request a call makes, the value a parser returns, that bad input yields an error instead of a panic. Do not add tests whose only assertion is the wording of a message.
- Table tests with testify, `t.Setenv` for environment variables, `t.Context()` for contexts.

## Checking your change

- Help text: dump `--help` for every command before and after and diff them (recurse through "Available Commands:"). Deprecated commands are hidden from that walk, check them by name.
- Arguments: run each touched command with a stray argument and confirm the exit code.
- Enumerated values: paste the example from the help into a shell and make sure it runs.
- Messages: grep the repository, including tests and testdata, for the old text before changing it.
