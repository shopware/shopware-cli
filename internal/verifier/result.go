package verifier

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shopware/shopware-cli/internal/tracking"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/logging"
)

type ToolResult = validation.ToolResult

// RunTools collects findings as tools publish them and waits for every tool,
// including when a sibling fails. Finding delivery is serialized per tool, while
// verbose output is serialized across tools. Tools return ToolSummary metadata;
// the runner obtains HasFindings and Errors from the execution helper after all tool cleanup has
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
	// The command event reports how much the tools work on and how much CPU time they use.
	for key, value := range configTelemetryTags(config) {
		tracking.SetCommandTag(key, value)
	}
	// Logical CPUs, not GOMAXPROCS: that is a scheduler setting, which rounds container limits up.
	tracking.SetCommandTag(tracking.TagCPUCount, strconv.Itoa(runtime.NumCPU()))
	var cpuTime atomic.Int64
	results := make([]validation.ToolReport, len(tools), len(tools)+len(skipped))
	sourceRoot := validation.ResolveSourceRoot(config.RootDir)
	var stderrMu sync.Mutex
	var group sync.WaitGroup
	for i, tool := range tools {
		group.Go(func() {
			var collected []validation.CheckResult
			execution := NewToolExecution(func(findings []validation.CheckResult) bool {
				retained := filterFindings(findings, sourceRoot, config.ValidationIgnores)
				collected = append(collected, retained...)
				return len(retained) > 0
			})
			execution.onSubprocess = func(subprocess SubprocessResult) {
				cpuTime.Add(int64(subprocess.cpuTime()))
				tracking.Track(ctx, tracking.EventToolSubprocess, subprocessTelemetryTags(tool.Name(), subprocess))
				if logging.IsVerbose(ctx) {
					stderrMu.Lock()
					defer stderrMu.Unlock()
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
			var summary ToolSummary
			if ctx.Err() == nil {
				summary = run(tool, execution)
			}
			result := execution.finalize(ctx.Err())
			result.Name = tool.Name()
			result.Note = summary.Note
			result.ChangeAction = summary.ChangeAction
			result.ChangedFiles = normalizeChangedFiles(result.ChangedFiles, config.RootDir)
			// A tool that reported changes, findings or failures has run, whatever it returned.
			if !result.HasFindings && len(result.Errors) == 0 && len(result.ChangedFiles) == 0 {
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
	if cpuTimeCoversChildren {
		tracking.SetCommandTag(tracking.TagCPUMS, strconv.FormatInt(time.Duration(cpuTime.Load()).Milliseconds(), 10))
	}
	for _, result := range skipped {
		results = append(results, validation.ToolReport{ToolResult: result})
	}
	return results
}

func subprocessTelemetryTags(tool string, subprocess SubprocessResult) map[string]string {
	tags := map[string]string{
		tracking.TagTool:        tool,
		tracking.TagOperation:   subprocess.Operation,
		tracking.TagStdoutBytes: strconv.Itoa(len(subprocess.Stdout)),
		tracking.TagStderrBytes: strconv.Itoa(len(subprocess.Stderr)),
		tracking.TagDurationMS:  strconv.FormatInt(subprocess.Duration.Milliseconds(), 10),
		tracking.TagStarted:     strconv.FormatBool(subprocess.ProcessState != nil),
	}
	if subprocess.ProcessState != nil {
		if cpuTimeCoversChildren {
			tags[tracking.TagCPUMS] = strconv.FormatInt(subprocess.cpuTime().Milliseconds(), 10)
		}
		if code := subprocess.ProcessState.ExitCode(); code >= 0 {
			tags[tracking.TagExitCode] = strconv.Itoa(code)
		}
	}
	return tags
}

func configTelemetryTags(config ToolConfig) map[string]string {
	return map[string]string{
		tracking.TagExtensionCount:     strconv.Itoa(len(config.Extensions)),
		tracking.TagSourceDirCount:     strconv.Itoa(len(config.SourceDirectories)),
		tracking.TagAdminDirCount:      strconv.Itoa(len(config.AdminDirectories)),
		tracking.TagStorefrontDirCount: strconv.Itoa(len(config.StorefrontDirectories)),
	}
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

// normalizeChangedFiles gives every reporter the same stable, per-tool path list.
func normalizeChangedFiles(paths []string, root string) []string {
	var normalized []string
	resolvedRoot := validation.ResolveSourceRoot(root)
	for _, file := range paths {
		// Preserve the original root alias: converted sources may no longer
		// exist, so resolving a symlink in the file path can fail.
		file = validation.NormalizeSourcePath(file, root)
		file = validation.NormalizeSourcePath(file, resolvedRoot)
		if file != "" {
			normalized = append(normalized, file)
		}
	}
	slices.Sort(normalized)
	return slices.Compact(normalized)
}
