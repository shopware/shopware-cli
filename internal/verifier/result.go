package verifier

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/shopware/shopware-cli/internal/tracking"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/logging"
)

type ToolResult = validation.ToolResult

// RunTools collects findings as tools publish them and waits for every tool,
// including when a sibling fails. The callback, when supplied, receives retained
// findings synchronously; delivery and verbose output are serialized across tools.
// Tools return Note and SkipReason; the runner obtains HasFindings and Errors
// from the execution helper after all tool cleanup has completed.
func RunTools[T Tool](ctx context.Context, tools ToolList[T], skipped []ToolResult, config ToolConfig, run func(T, *ToolExecution) ToolResult, live ...func(string, []validation.CheckResult)) []validation.ToolReport {
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
				if len(retained) > 0 {
					for _, callback := range live {
						callback(tool.Name(), slices.Clone(retained))
					}
				}
				return len(retained) > 0
			})
			execution.telemetry = func(tags map[string]string) {
				tags[tracking.TagTool] = tool.Name()
				tracking.Track(ctx, tracking.EventToolSubprocess, tags)
			}
			if logging.IsVerbose(ctx) {
				execution.onSubprocess = func(subprocess SubprocessResult) {
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
			finish := config.Progress.Start(tool.Name())
			summary := run(tool, execution)
			result := execution.Result()
			result.Name = tool.Name()
			result.Note = summary.Note
			result.SkipReason = summary.SkipReason
			if len(result.Errors) > 0 {
				result.SkipReason = ""
				finish(result.Errors[0])
			} else {
				finish(nil)
			}
			results[i] = validation.ToolReport{ToolResult: result, Findings: collected}
		})
	}
	group.Wait()
	if err := ctx.Err(); err != nil && len(results) > 0 {
		results[0].Errors = append(results[0].Errors, ToolError{Err: err})
		results[0].SkipReason = ""
	}
	for _, result := range skipped {
		results = append(results, validation.ToolReport{ToolResult: result})
	}
	slices.SortFunc(results, func(a, b validation.ToolReport) int { return cmp.Compare(a.Name, b.Name) })
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
