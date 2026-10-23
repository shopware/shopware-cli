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
    Err        error
}
```

The runner attaches the tool name and collects results. It creates skipped results for tools omitted by `--only` or removed by `--exclude`. Selected tools can also return a skip reason when they do not apply.

Derive the displayed outcome:

| Condition | Outcome |
| --- | --- |
| Nonempty `SkipReason` | Skipped |
| Non-nil `Err` | Failed |
| Otherwise | Completed |

Skipped results contain no findings or execution error. Failed results retain captured output and any partial findings.

`Err` means the tool could not complete reliably, including preparation, process execution, or output parsing failures. Source problems and formatting violations are findings. Each adapter interprets process exit codes internally; neither a nonzero exit code nor stderr alone establishes failure.

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

Show detailed findings below the overview, grouped by tool. Include captured output for failures; verbose output can also expose it for completed tools.

Renderers consume the same result collection. Structured output explicitly serializes execution errors. Ignore rules apply only to findings. Execution errors and remaining error findings cause a nonzero command exit in every output format. Successfully applied changes alone do not constitute error findings.

## Consequences

Tools produce results without printing directly. Selection and execution share one reporting model, replacing separate invocation-status reporting.

The desired interfaces cover checkers, formatters, and fixers. Implementation can migrate these capabilities separately. Exact serialized schemas and compatibility changes follow during implementation.
