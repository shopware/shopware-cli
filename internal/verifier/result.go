package verifier

import (
	"cmp"
	"fmt"
	"slices"
	"sync"

	"github.com/shopware/shopware-cli/internal/validation"
)

type ToolResult = validation.ToolResult

// RunTools collects independent outcomes without cancelling sibling tools when
// one fails. Each goroutine owns a separate result slot.
func RunTools[T Tool](tools ToolList[T], skipped []ToolResult, config ToolConfig, run func(T) ToolResult) []ToolResult {
	results := make([]ToolResult, len(tools), len(tools)+len(skipped))
	var group sync.WaitGroup
	for i, tool := range tools {
		group.Go(func() {
			result := run(tool)
			result.Name = tool.Name()
			check := NewCheck()
			check.SetSourceRoot(config.RootDir)
			for _, finding := range result.Findings {
				check.AddResult(finding)
			}
			result.Findings = check.RemoveByIdentifier(config.ValidationIgnores).GetResults()
			results[i] = result
		})
	}
	group.Wait()
	results = append(results, skipped...)
	slices.SortFunc(results, func(a, b ToolResult) int { return cmp.Compare(a.Name, b.Name) })
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

	filtered := make([]validation.CheckResult, 0)
	for _, r := range c.Results {
		shouldKeep := true
		for _, ignore := range ignores {
			if validation.IgnoreMatches(r, ignore, c.samePath) {
				shouldKeep = false
				break
			}
		}
		if shouldKeep {
			filtered = append(filtered, r)
		}
	}
	c.Results = filtered

	return c
}

func (c *Check) samePath(resultPath, ignorePath string) bool {
	if resultPath == ignorePath {
		return true
	}
	if ignorePath == "" || c.sourceRoot == "" {
		return false
	}

	return validation.NormalizeSourcePath(resultPath, c.sourceRoot) == validation.NormalizeSourcePath(ignorePath, c.sourceRoot)
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
