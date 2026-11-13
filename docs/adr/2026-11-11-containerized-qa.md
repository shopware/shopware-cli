---
title: Containerized QA execution
status: Proposed
tags: [tooling, qa, containers]
---

- Related: [Issue #1713](https://github.com/shopware/shopware-cli/issues/1713),
  [Issue #1672](https://github.com/shopware/shopware-cli/issues/1672)
- Related: [Execution and runtime model](2026-11-10-execution-runtime-model.md)
- Related: [Unified tool results and output](2026-10-22-unified-tool-results.md)

## Context

Validation, fixing and formatting should run without depending on a working
PHP and Node.js installation on the host. Consistent runtime and tool versions
across development and CI reduce "works on my machine" differences, particularly
for Docker-managed projects whose host tooling may differ from the project.

Most QA tools need neither a running Shopware nor a database. A dedicated
tooling container avoids starting an entire stack or relying on a web image
that may lack the tools. The CLI already handles tool preparation,
configuration generation and reporting, so wrapping its QA commands can reuse
that workflow.

The execution and runtime model defines the shared process API. This decision
uses that API for containerized QA and selects the initial image and dispatch
policy.

## Decision

Run complete QA commands in a disposable Shopware CLI container when
containerized execution is selected. Keep direct execution for local projects
and when the CLI already runs inside the tooling image, including CI.

The orchestration layer handles dispatch and process lifecycle, while the
existing QA workflow runs inside the selected environment.

### Tooling image

Two existing images are suitable starting points:

| Image | Approach |
| --- | --- |
| Shopware CLI image | Includes the CLI and its QA tools, allowing a complete QA command to run inside the container. |
| Development image used by project creation and the local stack (`docker-dev`) | Provides the PHP and Node.js environment, with QA tooling prepared for execution there. |

Start with the Shopware CLI image. Always select the tag
`<cli-version>-php-<php-version>`, matching the currently running CLI version
and the PHP version selected for the QA run. Wrapping the existing QA command
requires few changes and preserves tool preparation and reporting inside the
CLI. The image choice is separate from the executor contract. A possible second
phase could migrate QA to the development image, but that decision remains open.
Such a change would adapt the image contents and target setup while retaining
the wrapped CLI command as the execution boundary.

### Execution policy

Run a complete validate, fix or format operation in the selected CLI image.
Dispatch before host prerequisite checks. The inner CLI runs the workflow
through its local runtime, with a tooling-context marker preventing recursive
container dispatch. One container runs the complete
workflow, including any generated tool configurations. The same approach can
wrap standalone extension commands without introducing container handling into
each tool.

| Operating mode | QA execution |
| --- | --- |
| Local project, CLI on the host | Direct QA execution with host tooling. |
| Docker-managed project, CLI on the host | Disposable CLI tooling container, independent of the development stack. |
| CLI already in the tooling image, including CI | Direct QA execution inside the image. |

Project configuration selects the mode. Engine availability validates that
selection and never silently changes it. Standalone extension commands retain
current behavior until selection is introduced separately. Intended precedence
is an explicit flag, extension configuration, then local execution.

### Dispatch through the orchestration API

The QA command uses the orchestration API to launch the containerized CLI.
The verifier retains tool execution and QA reporting, while application wiring
selects the image and workspace. With a container-backed executor supplied by
that wiring, a wrapped project validation command can look as follows:

```go
args := append([]string{"project", "validate", "/workspace"}, qaFlags...)
command := executor.Command("shopware-cli", args...).
    WithDir("/workspace").
    WithEnv(map[string]string{"SHOPWARE_CLI_TOOLING_CONTEXT": "1"})

result, err := command.Run(ctx, streams)
```

The mounted workspace can be a project or standalone extension directory. This
example runs project validation with the source directory mounted at
`/workspace`. `qaFlags` preserves
the original QA flags, including `--only`, `--exclude` and reporting options, so
tool selection and output behavior remain the same inside the container.
Application wiring maps paths in arguments and flags into the container
workspace. The inner CLI generates tool configurations and runs the existing
workflow. The outer command forwards its output and preserves failure status.

### Delivery and verification

Deliver this after the local and disposable Docker execution capabilities of the
shared API. QA can ship independently of stack, proxy and SSH migration.
Establish image publication, CLI/tool compatibility and supported workspace
layouts before making container dispatch the default for Docker-managed
projects.

Dispatch tests can supply `executor.New(testBackend)` to inspect the wrapped
command, simulate output and failures, and verify that dispatch is not repeated
inside the image.
Verifier tests continue to cover parsing, findings and reporting. Native checks
must verify findings, report formats, exit codes and persisted fixes, including
that dry-run leaves source files unchanged and cancellation cleans up the
owned container.

## Consequences

Developers can run QA without maintaining matching PHP and Node.js installations
on the host. The same tooling image supports development and CI, independently
of the development stack. A complete QA operation shares one container and
reuses the CLI's tool configuration and reporting behavior.

The integration stays small because the command wrapper is the execution
boundary. Image selection and workspace setup live in application wiring, while
the verifier keeps its existing responsibilities. A later image change can
retain that boundary.

The tooling image becomes part of the release workflow. CLI and tool versions
must remain compatible, and mounted files must preserve the expected reports
and changes. Local and containerized execution need verification against the
same QA behavior.
