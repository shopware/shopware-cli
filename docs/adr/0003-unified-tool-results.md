# 3. Unified tool results and output

- Status: Implemented
- Date: 2026-10-22

## Context

Checkers, formatters, and fixers currently mutate a shared `Check` or return an error, while tool selection status is collected and rendered separately. This makes consistent output difficult. For example, [issue #1695](https://github.com/shopware/shopware-cli/issues/1695) describes a Prettier preparation failure that appears in the command's error output but is missing from its report.

Users also need to know what ran, what was skipped, and which configuration was used. [Issue #1676](https://github.com/shopware/shopware-cli/issues/1676), for example, requests the Shopware version used by PHPStan. As we report more execution details, coordinating these separate paths makes tool additions and changes harder to maintain.

Each tool invocation should have one reporting model for its outcome, findings, and execution context.

## Vocabulary

- **CLI command:** a user-facing operation such as `shopware-cli project validate`.
- **Tool:** an implementation of a `CheckTool`, `FormatTool`, or `FixTool` interface.
- **Tool runner:** the orchestration code that selects and invokes tools.
- **Subprocess:** an external process spawned by a tool, such as one ESLint invocation.
- **Finding:** a structured source or formatting diagnostic.
- **Renderer:** a component that consumes findings and `ToolResult` summaries to produce output in a specific format, such as JSON or human-readable text.

## Decision

Keep capability interfaces, but give each tool invocation an execution helper. Tools deliver findings through the helper and return an outcome summary instead of mutating a shared `Check` or returning a separate error.

Findings use the existing `validation.CheckResult` struct and are delivered through a callback rather than included in the returned summary. This lets a live renderer receive findings before the tool finishes, while the tool runner owns filtering and storage.

```go
type Tool interface {
    Name() string
}

type CheckTool interface {
    Tool
    Check(ctx context.Context, config ToolConfig, execution *ToolExecution) ToolSummary
}

type FormatTool interface {
    Tool
    Format(ctx context.Context, config ToolConfig, execution *ToolExecution, dryRun bool) ToolSummary
}

type FixTool interface {
    Tool
    Fix(ctx context.Context, config ToolConfig, execution *ToolExecution) ToolSummary
}

type ToolSummary struct {
    Note       string
    SkipReason string
}

type ToolResult struct {
    Name        string
    SkipReason  string
    HasFindings bool
    Note        string
    Errors      []ToolError
}

type ToolError struct {
    Err     error
    Command []string // optional subprocess executable and arguments
    Dir     string   // optional working directory
    Stdout  string   // optional output from the failed invocation
    Stderr  string   // optional output from the failed invocation
}
```

The tool runner attaches the tool name to each result. It creates skipped results for tools omitted by `--only` or removed by `--exclude`. Selected tools can also return a skip reason when they do not apply (e.g. early return).

Derive the displayed outcome in this order:

| Condition             | Outcome   |
| --------------------- | --------- |
| Nonempty `Errors`     | Failed    |
| Nonempty `SkipReason` | Skipped   |
| Otherwise             | Completed |

A completed or failed outcome indicates that the tool ran, without implying that it spawned a subprocess. Skipped tools have `HasFindings == false`, publish no findings, and have no execution errors.

`HasFindings` indicates whether at least one reported finding remains after ignore rules. It can be true for completed or failed tools and stays false if all findings are ignored.

`Note` holds optional context, such as the Shopware version used or a summary of changes.

`Errors` means the tool could not complete reliably, including preparation, process execution, output parsing, or cleanup failures. Source problems and formatting violations are findings. Each tool interprets process exit codes internally; neither a nonzero exit code nor stderr alone establishes failure.

A `ToolError` requires a non-nil underlying error. Its `Error()` method returns that error's message, and `Unwrap()` preserves the cause. All other fields are optional.

A subprocess failure does not cancel sibling work or discard findings already delivered. Tools wait for all their subprocesses before returning. Cancellation of the CLI command still cancels work through its context, and the tool runner checks for cancellation so an interrupted command exits nonzero even if a tool reports no error.

### Responsibilities

| Component        | Responsibility                                                                                                                                                                                                                |
| ---------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Execution helper | Execute subprocesses, capture their output, record subprocess telemetry, track whether findings survived filtering, deliver findings and optional subprocess diagnostics through callbacks, and collect all reported failures |
| Tool             | Configure subprocesses, interpret process errors and exit codes, parse output, and report findings or failures                                                                                                                |
| Tool runner      | Create the helper, manage tool selection and lifecycle, normalize and filter findings, collect them by tool, and coordinate optional live findings and verbose subprocess output                                              |
| Renderer         | Present published findings and final results as interactive, plain-text, or structured output                                                                                                                                 |

Tools and the execution helper do not print directly. Tools control subprocess concurrency.

## Output

The output below is illustrative. Layout, wording, grouping, and ordering depend on the renderer's implementation.

A human-readable renderer could show an overview of all tools supporting the requested capability:

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

Detailed findings could appear below the overview, grouped by tool. Reports must indicate that failed tools' findings may be incomplete and include every reported execution error with its subprocess context and captured stdout and stderr when available.

All renderers consume the same collected findings and execution results. Execution errors must appear in reports rather than only in the command's terminal error output. Format-specific representations and compatibility changes follow during implementation.

Ignore rules apply only to findings. Any execution error or remaining error finding causes a nonzero CLI command exit in every output format. Successfully applied changes alone do not constitute error findings.

### Example: ESLint failure with partial findings

ESLint checks three source directories. Two subprocesses report three findings; the third fails to load its configuration. The report includes the execution error and the three findings, marked as potentially incomplete:

```text
Checkers:
  eslint  failed  cannot load ESLint configuration · 3 findings (may be incomplete)

Execution error:
  cannot load ESLint configuration
  Subprocess: node /tools/node_modules/.bin/eslint --format=json .
  Directory: /project/custom/plugins/Example/src/Resources/app/administration
  Stderr:
    [configuration error output from this invocation]

Findings:
  [three findings from the two completed invocations]
```

The CLI command exits nonzero because ESLint failed, even if ignore rules remove all three findings.

## Execution helper

`ToolExecution` provides three operations for tools, all safe for concurrent use:

```go
func (e *ToolExecution) Run(cmd *exec.Cmd) SubprocessResult
func (e *ToolExecution) AddFindings(findings ...validation.CheckResult)
func (e *ToolExecution) Fail(err *ToolError)
```

### Subprocesses and failures

`Run` waits for one subprocess to finish and returns its executable and arguments, working directory, separate stdout and stderr, and process error in `SubprocessResult`. `SubprocessResult.ToolError(err)` attaches that subprocess's context and captured output to a supplied error. The tool decides whether the process error or parsed output represents a failure; `Run` does not automatically call `Fail`.

Each `Fail` call appends the supplied error under synchronization, including preparation and cleanup failures without subprocess context. Errors are collected in reporting order, which can vary when subprocesses run concurrently.

Each subprocess has its own stdout and stderr buffers for parsing. The helper does not merge output or retain a history of subprocess results. Successful subprocess output can be discarded after parsing; failure output is retained in the corresponding `ToolError`. Its context and output fields remain empty when unavailable, such as for preparation failures. Tools interpret exit codes to classify outcomes; the helper also records them for telemetry. They are not added to `ToolResult`.

The execution helper records telemetry for each subprocess, associated with its tool: the exit code when available, and the number of bytes captured in stdout and stderr separately. Attempts that fail before the process starts are distinguished from processes that exit. This uses the existing telemetry infrastructure and respects its opt-out setting.

Output-size measurements will inform future decisions about buffer limits and memory usage. Telemetry records sizes without transmitting output contents, command arguments, or working directories.

For verbose diagnostics, the tool runner can supply an optional callback receiving each completed `SubprocessResult`. `Run` calls it synchronously before returning. The tool runner binds it to the tool's identity and serializes verbose output and live findings delivery across tools.

The verbose renderer displays each subprocess's output as one complete block labelled with tool, executable and arguments, and working directory. Each block is delivered after the subprocess finishes. Concurrent completions may be displayed in either order; the original interleaving of stdout and stderr is not preserved. Verbose output is delivered through the callback rather than replayed from final results.

### Findings and results

The tool runner creates each helper with a required callback bound to the tool's identity. The callback returns whether any findings in the batch were retained after filtering:

```go
execution := NewToolExecution(func(findings []validation.CheckResult) bool {
    return runner.collectFindings(tool.Name(), findings)
})
```

`AddFindings` synchronously delivers each nonempty batch through this callback and sets `HasFindings` to true if the callback returns true. Empty batches have no effect, and subsequent calls never reset the flag. The helper does not retain the findings themselves.

The tool runner normalizes paths, applies ignore rules, and stores the remaining findings by tool, even without a live renderer. It optionally forwards them to a live renderer. Collection and live delivery are serialized across tools.

Subprocess findings are delivered after that subprocess finishes and its output is parsed. In-process tools can deliver findings as their checks complete. Synchronous callbacks provide backpressure without a channel or background consumer; once all tools finish, all findings have been collected. Final renderers need no live subscription.

Tools return `ToolSummary` containing optional `Note` and `SkipReason` metadata. After the tool and its deferred cleanup finish, the runner uses the private `execution.snapshot()` method to read `HasFindings` and all reported failures in `ToolResult.Errors`, then adds the tool name and returned metadata. A skip reason is applied only when there are no retained findings or failures.

For example, inside an ESLint tool's per-directory function:

```go
subprocess := execution.Run(cmd)
if !isExpectedESLintExit(subprocess.Err) {
    execution.Fail(subprocess.ToolError(
        fmt.Errorf("cannot run ESLint: %w", subprocess.Err)))
    return
}

findings, err := parseESLint(subprocess.Stdout)
if err != nil {
    execution.Fail(subprocess.ToolError(
        fmt.Errorf("cannot parse ESLint output: %w", err)))
    return
}
execution.AddFindings(findings...)
```

Here, `cmd` is an `*exec.Cmd` configured by the tool. The illustrative `isExpectedESLintExit` and `parseESLint` functions belong to the tool; the exit check accepts findings-related exit codes. After all per-directory functions finish, the tool returns its `ToolSummary`.

## Consequences

Selection and execution share one reporting model, replacing separate invocation-status reporting. Consumers needing only tool status can use `ToolResult`; full reports combine it with collected findings.

Synchronous callbacks simplify collection, but slow rendering can delay tool progress. Each tool must explicitly distinguish findings from execution failures and report failures through `Fail`; running a subprocess through the helper alone does not guarantee that its failure is reported.

Changing the capability interfaces requires migrating every tool implementation and its callers.

This decision does not define raw-output streaming, incremental subprocess parsing, output limits, persistent storage, or a TUI implementation.
