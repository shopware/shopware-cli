package verifier

import (
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/shopware/shopware-cli/internal/tracking"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/logging"
)

type ToolResult = validation.ToolResult

// RunTools collects findings as tools publish them and waits for every tool,
// including when a sibling fails. Finding delivery and verbose output are
// serialized across tools. Tools return ToolSummary metadata; the runner obtains
// HasFindings and Errors from the execution helper after all tool cleanup has
// completed. A tool that returns after ctx is cancelled fails with the
// cancellation, so interrupted commands exit nonzero.
func RunTools[T Tool](
	ctx context.Context,
	tools ToolList[T],
	skipped []ToolResult,
	config ToolConfig,
	progress *Progress,
	run func(T, *ToolExecution) ToolSummary,
) []validation.ToolReport {
	results := make([]validation.ToolReport, len(tools), len(tools)+len(skipped))
	sourceRoot := validation.ResolveSourceRoot(config.RootDir)
	var delivery sync.Mutex
	var group sync.WaitGroup
	for i, tool := range tools {
		group.Go(func() {
			var collected []validation.CheckResult
			execution := NewToolExecution(func(findings []validation.CheckResult) bool {
				delivery.Lock()
				defer delivery.Unlock()
				retained := filterFindings(findings, sourceRoot, config.ValidationIgnores)
				collected = append(collected, retained...)
				return len(retained) > 0
			})
			execution.onSubprocess = func(subprocess SubprocessResult) {
				if subprocess.ProcessState != nil {
					tags := map[string]string{
						tracking.TagTool:        tool.Name(),
						tracking.TagStdoutBytes: strconv.Itoa(len(subprocess.Stdout)),
						tracking.TagStderrBytes: strconv.Itoa(len(subprocess.Stderr)),
					}
					if code := subprocess.ProcessState.ExitCode(); code >= 0 {
						tags[tracking.TagExitCode] = strconv.Itoa(code)
					}
					tracking.Track(ctx, tracking.EventToolSubprocess, tags)
				}
				if logging.IsVerbose(ctx) {
					delivery.Lock()
					defer delivery.Unlock()
					var output strings.Builder
					fmt.Fprintf(&output, "\n%s subprocess: %s\nDirectory: %s\n", tool.Name(), strings.Join(subprocess.Command, " "), subprocess.Dir)
					if subprocess.Stdout != "" {
						fmt.Fprintf(&output, "stdout:\n%s\n", subprocess.Stdout)
					}
					if subprocess.Stderr != "" {
						fmt.Fprintf(&output, "stderr:\n%s\n", subprocess.Stderr)
					}
					_, _ = io.WriteString(os.Stderr, output.String())
				}
			}
			finish := progress.Start(tool.Name())
			summary := run(tool, execution)
			if err := ctx.Err(); err != nil {
				execution.Fail(&ToolError{Err: err})
			}
			result := execution.snapshot()
			result.Name = tool.Name()
			result.Note = summary.Note
			// A tool that reported findings or failures has run, whatever it returned.
			if !result.HasFindings && len(result.Errors) == 0 {
				result.SkipReason = summary.SkipReason
			}
			if len(result.Errors) > 0 {
				finish(result.Errors[0])
			} else {
				finish(nil)
			}
			results[i] = validation.ToolReport{ToolResult: result, Findings: collected}
		})
	}
	group.Wait()
	for _, result := range skipped {
		results = append(results, validation.ToolReport{ToolResult: result})
	}
	return results
}

type Check struct {
	Results    []validation.CheckResult `json:"results"`
	mutex      sync.Mutex
	sourceRoot string
}

func NewCheck() *Check {
	return &Check{
		Results: []validation.CheckResult{},
	}
}

// SetSourceRoot configures the analysis root used to rewrite finding paths
// into stable, input-relative locations before they are stored or reported.
func (c *Check) SetSourceRoot(root string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.sourceRoot = validation.ResolveSourceRoot(root)
}

func (c *Check) AddResult(result validation.CheckResult) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.sourceRoot != "" {
		result = validation.NormalizeResult(result, c.sourceRoot)
	}
	c.Results = append(c.Results, result)
}

func (c *Check) HasErrors() bool {
	for _, r := range c.Results {
		if r.Severity == validation.SeverityError {
			return true
		}
	}

	return false
}

func (c *Check) GetResults() []validation.CheckResult {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.Results
}

func (c *Check) RemoveByIdentifier(ignores []validation.ToolConfigIgnore) validation.Check {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.Results = filterFindings(c.Results, c.sourceRoot, ignores)

	return c
}

func filterFindings(findings []validation.CheckResult, root string, ignores []validation.ToolConfigIgnore) []validation.CheckResult {
	samePath := func(resultPath, ignorePath string) bool {
		return resultPath == ignorePath || (root != "" && ignorePath != "" &&
			validation.NormalizeSourcePath(resultPath, root) == validation.NormalizeSourcePath(ignorePath, root))
	}
	filtered := make([]validation.CheckResult, 0, len(findings))
	for _, finding := range findings {
		if root != "" {
			finding = validation.NormalizeResult(finding, root)
		}
		if !slices.ContainsFunc(ignores, func(ignore validation.ToolConfigIgnore) bool {
			return validation.IgnoreMatches(finding, ignore, samePath)
		}) {
			filtered = append(filtered, finding)
		}
	}
	return filtered
}

func fileChangeNote(count int, action string) string {
	if count == 0 {
		return "no changes"
	}
	if count == 1 {
		return "1 file " + action
	}
	return fmt.Sprintf("%d files %s", count, action)
}
