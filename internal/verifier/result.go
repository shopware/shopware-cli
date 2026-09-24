package verifier

import (
	"slices"
	"strings"
	"sync"

	"github.com/shopware/shopware-cli/internal/validation"
)

type Check struct {
	Results    []validation.CheckResult `json:"results"`
	mutex      sync.Mutex
	sourceRoot string
	target     *validation.Target
	toolRuns   map[string]validation.ToolRun
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

// SetTarget records the Shopware baseline the run is evaluated against.
func (c *Check) SetTarget(target validation.Target) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.target = &target
}

func (c *Check) GetTarget() *validation.Target {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.target
}

// RecordToolRun stores how a tool took part; a later record for the same tool wins.
func (c *Check) RecordToolRun(run validation.ToolRun) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.toolRuns == nil {
		c.toolRuns = map[string]validation.ToolRun{}
	}
	c.toolRuns[run.Name] = run
}

// GetToolRuns returns the recorded runs sorted by tool name.
func (c *Check) GetToolRuns() []validation.ToolRun {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	runs := make([]validation.ToolRun, 0, len(c.toolRuns))
	for _, run := range c.toolRuns {
		runs = append(runs, run)
	}
	slices.SortFunc(runs, func(a, b validation.ToolRun) int {
		return strings.Compare(a.Name, b.Name)
	})
	return runs
}

func (c *Check) hasToolRun(name string) bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	_, ok := c.toolRuns[name]
	return ok
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
