# 3. Unified tool results and output

- Status: Proposed
- Date: 2026-10-22

## Context

Checkers, formatters, and fixers currently report findings, invocation errors, selection statuses, and execution context through different mechanisms. This makes consistent output difficult.

Each tool should explain whether it completed, failed, or was skipped, alongside its findings and captured output.

## Why now

Recent requests, including [issue #1695](https://github.com/shopware/shopware-cli/issues/1695), are pushing tool output to explain more than findings alone:
users also need to know what ran, what was skipped, and why an invocation failed.
The current implementation carries these details through separate paths:
tools mutate a shared `Check` or return an error, while selection status is collected and rendered independently.
As transparency grows, each new piece of invocation information adds coordination across those paths and makes future tool additions harder to maintain.
Consolidating the outcome now addresses that accumulated complexity and gives upcoming changes one consistent place to report results.

## Decision

Keep capability interfaces, but return one result instead of mutating a shared `Check` or returning a separate error:

```go
type Tool interface {
    Name() string
}

type CheckTool interface {
    Tool
    Check(ctx context.Context, config ToolConfig) ToolResult
}

type FormatTool interface {
    Tool
    Format(ctx context.Context, config ToolConfig, dryRun bool) ToolResult
}

type FixTool interface {
    Tool
    Fix(ctx context.Context, config ToolConfig) ToolResult
}

type ToolResult struct {
    Name       string
    SkipReason string
    Findings   []validation.CheckResult // using our existing struct
    Note       string
    Stdout     string
    Stderr     string
    Err        *ToolError
}

type ToolError struct {
    Err     error
    Command []string // optional executable and arguments
    Dir     string   // optional working directory
    Stdout  string   // optional output from the failed invocation
    Stderr  string   // optional output from the failed invocation
}
```

The runner attaches the tool name and collects results. It creates skipped results for tools omitted by `--only` or removed by `--exclude`. Selected tools can also return a skip reason when they do not apply.

Derive the displayed outcome in this order:

| Condition | Outcome |
| --- | --- |
| Non-nil `Err` | Failed |
| Nonempty `SkipReason` | Skipped |
| Otherwise | Completed |

Skipped results contain no findings or execution error. Failed results retain captured output and any partial findings.

`Err` means the tool could not complete reliably, including preparation, process execution, output parsing, or cleanup failures. Source problems and formatting violations are findings. Each tool interprets process exit codes internally; neither a nonzero exit code nor stderr alone establishes failure.

A `ToolError` requires a non-nil underlying error. Its `Error()` method returns that error's message, and `Unwrap()` preserves the cause. All other fields are optional, so failures before a command is constructed can omit them. Process and parsing failures include the failing command's context when available.

`ToolError.Stdout` and `Stderr` contain only output captured from the failed invocation. They are optional and can be empty, including for preparation failures. `ToolResult.Stdout` and `Stderr` retain the merged output across all invocations, including the failed one.

When a tool runs multiple commands, it retains the first detected execution error. Other invocations can finish normally and contribute findings and captured output; the tool waits for them before returning its result. A failure does not cancel the other invocations. Which error is detected first can vary when commands run concurrently.

Use one optional note for context or a summary of changes. Capture stdout and stderr as strings, appending output when multiple internal commands run. Do not expose exit codes or individual execution records.

## Output

Show an overview of all tools supporting the requested capability:

```text
Checkers:
  builtin          skipped    excluded by --exclude
  eslint           failed     cannot parse ESLint output
  phpstan          completed  3 errors · Shopware 6.7.0.0 (lowest)
  storefront-twig  skipped    no storefront sources
  stylelint        skipped    not selected by --only
```

Formatters and fixers describe their outcomes similarly:

```text
Formatters:
  php-cs-fixer  completed  no changes
  prettier      completed  4 files formatted

Fixers:
  eslint       completed  fixes applied
  rector       failed     cannot run rector
  stylelint    skipped    no storefront sources
  symfony-xml  completed  no changes
```

Show detailed findings below the overview, grouped by tool. Failed tools explicitly indicate that their findings may be incomplete. Include the failed invocation's stdout and stderr alongside its error; verbose output can also expose merged output for any tool.

Renderers consume the same result collection. Structured output serializes an execution error as an object with `message` and optional `command` (an array of executable and arguments), `dir`, `stdout`, and `stderr` fields, rather than serializing the underlying Go error. Ignore rules apply only to findings. Execution errors and remaining error findings cause a nonzero command exit in every output format. Successfully applied changes alone do not constitute error findings.

### Example: ESLint failure with partial findings

ESLint runs against three source directories. Two invocations complete with three findings between them; the third cannot load its configuration. The other invocations finish normally, and ESLint returns their three findings, merged output, and a `ToolError` containing the first failure's cause, command, working directory, and invocation-specific stdout and stderr.

The output makes the failure and potentially incomplete findings explicit:

```text
Checkers:
  eslint  failed  cannot load ESLint configuration · 3 findings (may be incomplete)

Execution error:
  cannot load ESLint configuration
  Command: node /tools/node_modules/.bin/eslint --format=json .
  Directory: /project/custom/plugins/Example/src/Resources/app/administration
  Stderr:
    [configuration error output from this invocation]

Findings:
  [three findings from the two completed invocations]
```

The command exits nonzero because ESLint failed, even if ignore rules remove all three findings.

## Consequences

Tools produce results without printing directly. Selection and execution share one reporting model, replacing separate invocation-status reporting.

The desired interfaces cover checkers, formatters, and fixers. Implementation can migrate these capabilities separately. The remaining serialized schemas and compatibility changes follow during implementation.
